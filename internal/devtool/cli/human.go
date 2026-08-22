package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"hitkeep/internal/devtool"
)

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
)

type humanStyle struct {
	color bool
}

type humanDetachedRun struct {
	Run devtool.Run
}

type humanFollowedLog struct {
	RunID    string
	Detached bool
}

type runLookup func(string) (devtool.Run, error)

type liveRunOutput struct {
	writer   io.Writer
	style    humanStyle
	mu       sync.Mutex
	previous map[string]string
}

func validateOutputFormat(output string) error {
	switch output {
	case "human", "plain", "json", "ndjson":
		return nil
	default:
		return fmt.Errorf("unknown output format %q (choose human, plain, json, or ndjson)", output)
	}
}

func colorEnabled(writer io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	for _, name := range []string{"CLICOLOR_FORCE", "FORCE_COLOR"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" && value != "0" {
			return true
		}
	}
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (s humanStyle) paint(code, value string) string {
	if !s.color || value == "" {
		return value
	}
	return code + value + ansiReset
}

func (s humanStyle) bold(value string) string    { return s.paint(ansiBold, value) }
func (s humanStyle) dim(value string) string     { return s.paint(ansiDim, value) }
func (s humanStyle) red(value string) string     { return s.paint(ansiRed, value) }
func (s humanStyle) green(value string) string   { return s.paint(ansiGreen, value) }
func (s humanStyle) yellow(value string) string  { return s.paint(ansiYellow, value) }
func (s humanStyle) magenta(value string) string { return s.paint(ansiMagenta, value) }
func (s humanStyle) cyan(value string) string    { return s.paint(ansiCyan, value) }

func renderHumanError(writer io.Writer, message string, color bool) {
	style := humanStyle{color: color}
	_, _ = fmt.Fprintf(writer, "%s %s\n", style.red("✗"), style.bold(message))
}

func renderHuman(writer io.Writer, envelope devtool.Envelope, color bool) error {
	style := humanStyle{color: color}
	switch value := envelope.Data.(type) {
	case devtool.QAPlan:
		renderQAPlan(writer, style, value)
	case devtool.DevStatus:
		renderHumanDevStatus(writer, style, value)
	case devtool.DevStartResult:
		renderHumanDevStatus(writer, style, value.Status)
	case devtool.DevLogBatch:
		for _, event := range value.Events {
			renderHumanDevEvent(writer, style, event)
		}
	case devtool.Run:
		if value.Request.Kind != "qa" {
			return renderPlain(writer, envelope)
		}
		renderTerminalRun(writer, style, value)
	case humanDetachedRun:
		renderDetachedRun(writer, style, value.Run)
	case humanFollowedLog:
		renderFollowedLog(writer, style, value)
	case devtool.LogTail:
		for _, line := range value.Lines {
			renderHumanLogLine(writer, style, line)
		}
	default:
		return renderPlain(writer, envelope)
	}
	return nil
}

func renderHumanDevStatus(writer io.Writer, style humanStyle, status devtool.DevStatus) {
	var icon string
	switch status.State {
	case devtool.DevStateReady:
		icon = style.green("✓")
	case devtool.DevStateDegraded:
		icon = style.yellow("!")
	case devtool.DevStateFailed:
		icon = style.red("✗")
	case devtool.DevStateStopped:
		icon = style.dim("○")
	case devtool.DevStateStarting, devtool.DevStateStopping:
		icon = style.cyan("●")
	}
	_, _ = fmt.Fprintf(writer, "%s %s %s\n", icon, style.bold("HitKeep development"), status.State)
	if status.URLs.Web != "" && status.State != devtool.DevStateStopped {
		_, _ = fmt.Fprintf(writer, "  web   %s\n", style.cyan(status.URLs.Web))
		_, _ = fmt.Fprintf(writer, "  api   %s\n", style.cyan(status.URLs.API))
		_, _ = fmt.Fprintf(writer, "  mail  %s\n", style.cyan(status.URLs.Mailpit))
	}
	if status.Error != "" {
		_, _ = fmt.Fprintf(writer, "  %s %s\n", style.red("error"), status.Error)
	}
}

func renderHumanDevEvent(writer io.Writer, style humanStyle, event devtool.DevEvent) {
	component := event.Component
	if component == "" {
		component = event.Type
	}
	label := style.magenta(fmt.Sprintf("%-10s", component))
	switch event.Level {
	case "error":
		_, _ = fmt.Fprintf(writer, "%s %s\n", label, style.red(event.Message))
	case "warn", "warning":
		_, _ = fmt.Fprintf(writer, "%s %s\n", label, style.yellow(event.Message))
	default:
		if event.Type == "phase" {
			_, _ = fmt.Fprintf(writer, "%s %s\n", style.cyan(fmt.Sprintf("%-10s", event.Phase)), style.bold(event.Message))
			return
		}
		_, _ = fmt.Fprintf(writer, "%s %s\n", label, event.Message)
	}
}

func renderQAPlan(writer io.Writer, style humanStyle, plan devtool.QAPlan) {
	_, _ = fmt.Fprintf(writer, "%s %s\n", style.bold("QA"), style.cyan(plan.Profile))
	if plan.Escalated {
		_, _ = fmt.Fprintf(writer, "  %s %s\n", style.yellow("escalated:"), plan.EscalationWhy)
	}
	_, _ = fmt.Fprintf(writer, "  %s\n", strings.Join(plan.GateIDs, ", "))
}

func renderTerminalRun(writer io.Writer, style humanStyle, run devtool.Run) {
	duration := time.Since(run.StartedAt)
	if run.FinishedAt != nil {
		duration = run.FinishedAt.Sub(run.StartedAt)
	}
	_, _ = fmt.Fprintf(writer, "%s %s %s\n", terminalStatus(style, run.Status), style.bold(runName(run.Request)), style.dim(duration.Round(time.Millisecond).String()))
	if run.Status == "failed" {
		_, _ = fmt.Fprintf(writer, "  log %s\n", run.LogPath)
	}
	for _, gate := range run.GateResults {
		if gate.Status == "failed" {
			_, _ = fmt.Fprintf(writer, "  %s %s\n", style.red("✗"), gate.GateID)
		}
	}
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		_ = appendGitHubRunSummary(run, duration)
	}
}

func appendGitHubRunSummary(run devtool.Run, duration time.Duration) error {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" || run.Request.Kind != "qa" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // GitHub provides the step-summary path to the runner process.
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, "### hk QA: %s\n\n**Profile:** `%s` · **Duration:** %s\n\n| Gate | Status | Duration |\n| --- | --- | ---: |\n", githubMarkdown(run.Status), githubMarkdown(run.Request.Profile), duration.Round(time.Millisecond))
	if err != nil {
		return err
	}
	for _, gate := range run.GateResults {
		gateDuration := time.Duration(gate.DurationMS) * time.Millisecond
		if _, err = fmt.Fprintf(file, "| `%s` | %s | %s |\n", githubMarkdown(gate.GateID), githubMarkdown(gate.Status), gateDuration.Round(time.Millisecond)); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(file)
	return err
}

func githubMarkdown(value string) string {
	return strings.NewReplacer("|", "\\|", "\r", " ", "\n", " ").Replace(value)
}

func renderDetachedRun(writer io.Writer, style humanStyle, run devtool.Run) {
	_, _ = fmt.Fprintf(writer, "%s %s\n", style.yellow("○"), style.bold(runName(run.Request)+" continues in the background"))
	_, _ = fmt.Fprintf(writer, "  %s\n", run.ID)
}

func renderFollowedLog(writer io.Writer, style humanStyle, log humanFollowedLog) {
	if log.Detached {
		_, _ = fmt.Fprintf(writer, "%s %s\n", style.yellow("○"), style.bold("Log viewer detached"))
		_, _ = fmt.Fprintf(writer, "  reattach with: ./hk run logs %s\n", log.RunID)
		return
	}
	_, _ = fmt.Fprintf(writer, "%s %s\n", style.green("✓"), style.bold("Log stream complete"))
}

func terminalStatus(style humanStyle, status string) string {
	switch status {
	case "passed":
		return style.green("✓")
	case "failed":
		return style.red("✗")
	case "cancelled":
		return style.dim("○")
	default:
		return style.cyan("●")
	}
}

func runName(request devtool.RunRequest) string {
	switch request.Kind {
	case "qa":
		return "QA " + request.Profile
	default:
		return request.Kind
	}
}

func terminalRunStatus(status string) bool {
	return status == "passed" || status == "failed" || status == "cancelled"
}

func newLiveRunOutput(writer io.Writer, color bool) *liveRunOutput {
	return &liveRunOutput{
		writer:   writer,
		style:    humanStyle{color: color},
		previous: map[string]string{},
	}
}

func (output *liveRunOutput) Start(runID string, request devtool.RunRequest, workspace devtool.Workspace, reused bool) {
	output.mu.Lock()
	defer output.mu.Unlock()
	_, _ = fmt.Fprintf(output.writer, "%s %s\n", output.style.cyan("◆"), output.style.bold(runName(request)))
	_, _ = fmt.Fprintf(output.writer, "  run %s\n", runID)
	_, _ = fmt.Fprintln(output.writer, output.style.dim("  Ctrl+C detaches"))
	_, _ = fmt.Fprintln(output.writer)
}

func (output *liveRunOutput) Follow(runID string) {
	output.mu.Lock()
	defer output.mu.Unlock()
	_, _ = fmt.Fprintln(output.writer)
	_, _ = fmt.Fprintf(output.writer, "%s %s\n", output.style.cyan("●"), output.style.bold("Following logs"))
	_, _ = fmt.Fprintf(output.writer, "  run %s\n", runID)
	_, _ = fmt.Fprintln(output.writer)
}

func (output *liveRunOutput) Observe(run devtool.Run) {
	if run.Request.Kind != "qa" {
		return
	}
	output.mu.Lock()
	defer output.mu.Unlock()
	for _, gate := range run.GateResults {
		if output.previous[gate.GateID] == gate.Status {
			continue
		}
		output.previous[gate.GateID] = gate.Status
		switch gate.Status {
		case "running":
			_, _ = fmt.Fprintf(output.writer, "%s %s\n", output.style.cyan("●"), gate.GateID)
		case "passed":
			_, _ = fmt.Fprintf(output.writer, "%s %s %s\n", output.style.green("✓"), gate.GateID, output.style.dim(humanDuration(gate.DurationMS)))
		case "failed":
			_, _ = fmt.Fprintf(output.writer, "%s %s %s\n", output.style.red("✗"), gate.GateID, output.style.dim(humanDuration(gate.DurationMS)))
		case "cancelled":
			_, _ = fmt.Fprintf(output.writer, "%s %s\n", output.style.dim("○"), gate.GateID)
		}
	}
}

func (output *liveRunOutput) LogLine(line string) {
	output.mu.Lock()
	defer output.mu.Unlock()
	renderHumanLogLine(output.writer, output.style, line)
}

func renderHumanLogLine(writer io.Writer, style humanStyle, line string) {
	switch {
	case strings.HasPrefix(line, "$ "):
		_, _ = fmt.Fprintln(writer, style.cyan(line))
	case strings.HasPrefix(line, "[") && strings.Contains(line, "] "):
		_, _ = fmt.Fprintln(writer, style.magenta(line))
	default:
		_, _ = fmt.Fprintln(writer, line)
	}
}

func followRunLog(ctx context.Context, runID string, getRun runLookup, skipLines int, emit func(string)) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var file *os.File
	var reader *bufio.Reader
	var pending string
	lineNumber := 0
	for {
		run, err := getRun(runID)
		if err != nil {
			return err
		}
		if file == nil {
			file, err = os.Open(run.LogPath)
			if err != nil {
				if !os.IsNotExist(err) {
					return err
				}
				if terminalRunStatus(run.Status) {
					return nil
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
					continue
				}
			}
			defer file.Close()
			reader = bufio.NewReader(file)
		}

		for {
			part, readErr := reader.ReadString('\n')
			pending += part
			if strings.HasSuffix(pending, "\n") {
				lineNumber++
				if lineNumber > skipLines {
					emit(strings.TrimSuffix(pending, "\n"))
				}
				pending = ""
			}
			if readErr == nil {
				continue
			}
			if !errors.Is(readErr, io.EOF) {
				return readErr
			}
			break
		}
		if terminalRunStatus(run.Status) {
			if pending != "" {
				lineNumber++
				if lineNumber > skipLines {
					emit(pending)
				}
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
