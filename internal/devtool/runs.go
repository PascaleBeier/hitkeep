package devtool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const maxLogLines = 200

func (a *App) StartRun(ctx context.Context, request RunRequest) (RunStart, error) {
	request = normalizeRunRequest(request)
	if err := ValidateRunRequest(request); err != nil {
		return RunStart{}, err
	}
	if request.Kind == "qa" && len(request.GateIDs) == 0 {
		plan, err := a.QAPlan(ctx, request.Profile, "")
		if err != nil {
			return RunStart{}, err
		}
		request.GateIDs = plan.GateIDs
	}
	startLock, err := lockStateRoot(a.workspace.StateDir, "run-start")
	if err != nil {
		return RunStart{}, fmt.Errorf("lock run creation: %w", err)
	}
	defer unlockStateRoot(startLock)
	runs, err := a.ListRuns(100)
	if err != nil {
		return RunStart{}, fmt.Errorf("inspect active runs: %w", err)
	}
	for _, existing := range runs {
		if isTerminal(existing.Status) {
			continue
		}
		if runRequestsEqual(existing.Request, request) {
			start := runStart(existing)
			start.Reused = true
			return start, nil
		}
	}
	id := time.Now().UTC().Format("20060102T150405") + "-" + uuid.NewString()[:8]
	logPath := filepath.Join(a.workspace.StateDir, "runs", id+".log")
	run := Run{
		ID: id, WorkspaceID: a.workspace.ID, Request: request, Status: "queued", LogPath: logPath, StartedAt: time.Now().UTC(),
	}
	if request.Kind == "qa" {
		for _, gateID := range request.GateIDs {
			run.GateResults = append(run.GateResults, GateResult{GateID: gateID, Status: "queued", LogPath: filepath.Join(a.workspace.StateDir, "runs", id+"."+gateID+".log")})
		}
	}
	if err := a.writeRun(run); err != nil {
		return RunStart{}, err
	}

	// The executable and worktree are resolved internally, and the run ID is generated above.
	command := exec.CommandContext(context.WithoutCancel(ctx), a.executable, "__run", "--workspace", a.workspace.Root, "--run-id", id) //nolint:gosec
	command.Dir = a.workspace.Root
	stateRoot := filepath.Dir(filepath.Dir(a.workspace.StateDir))
	childEnvironment := []string{"HK_CHILD_RUN=1", "HK_STATE_DIR=" + stateRoot}
	if agentOutputEnabled(ctx) {
		childEnvironment = append(childEnvironment, "HK_CHILD_OUTPUT=json")
	}
	if slots := validatedQASlotOverride(); slots != "" {
		childEnvironment = append(childEnvironment, "HK_QA_SLOTS="+slots)
	}
	command.Env = a.commandEnvironment(childEnvironment)
	command.Stdin = nil
	command.Stdout = nil
	command.Stderr = nil
	// A detached run must outlive the CLI or MCP process that started it.
	// Give the worker its own session so a launcher terminal closing does not
	// deliver SIGHUP to the worker and every gate process beneath it.
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		finished := time.Now().UTC()
		exitCode := 1
		run.Status = "failed"
		run.Error = redactError(err.Error())
		run.ExitCode = &exitCode
		run.FinishedAt = &finished
		_ = a.writeRun(run)
		return RunStart{}, fmt.Errorf("start run worker: %w", err)
	}
	run.Status = "running"
	run.PID = command.Process.Pid
	if err := a.writeRun(run); err != nil {
		_ = command.Process.Kill()
		return RunStart{}, err
	}
	if err := os.WriteFile(a.runReadyPath(run.ID), []byte("ready\n"), 0o600); err != nil {
		_ = command.Process.Kill()
		finished := time.Now().UTC()
		exitCode := 1
		run.Status = "failed"
		run.Error = "failed to release run worker"
		run.PID = 0
		run.ExitCode = &exitCode
		run.FinishedAt = &finished
		_ = a.writeRun(run)
		return RunStart{}, fmt.Errorf("release run worker: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return RunStart{}, fmt.Errorf("release run worker: %w", err)
	}
	return runStart(run), nil
}

func runStart(run Run) RunStart {
	return RunStart{RunID: run.ID, Status: run.Status, StatusURI: "hitkeep-dev://runs/" + run.ID + "/summary", LogURI: "hitkeep-dev://runs/" + run.ID + "/logs/all"}
}

func (a *App) ExecuteRun(ctx context.Context, runID string) error {
	if os.Getenv("HK_CHILD_OUTPUT") == "json" {
		ctx = WithAgentOutput(ctx)
	}
	if err := a.waitUntilRunReady(ctx, runID); err != nil {
		return err
	}
	defer os.Remove(a.runReadyPath(runID))
	run, err := a.GetRun(runID)
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(run.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open run log: %w", err)
	}
	defer logFile.Close()
	run.Status = "running"
	run.PID = os.Getpid()
	if err := a.writeRun(run); err != nil {
		return err
	}

	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	cancelWatchDone := make(chan struct{})
	defer close(cancelWatchDone)
	go a.watchRunCancellation(runContext, run.ID, cancel, cancelWatchDone)

	gateResults, execErr := a.executeRequest(runContext, run.ID, run.Request, logFile)
	finished := time.Now().UTC()
	run.FinishedAt = &finished
	run.PID = 0
	exitCode := 0
	if execErr != nil {
		run.Status = "failed"
		run.Error = redactError(execErr.Error())
		exitCode = 1
		if errors.Is(execErr, context.Canceled) {
			run.Status = "cancelled"
			exitCode = 130
		}
	} else {
		run.Status = "passed"
	}
	run.Artifacts = a.runArtifacts(run)
	run.GateResults = gateResults
	run.ExitCode = &exitCode
	if err := a.writeRun(run); err != nil {
		return err
	}
	_ = os.Remove(a.runCancellationPath(run.ID))
	return execErr
}

func (a *App) GetRun(runID string) (Run, error) {
	if err := validateRunID(runID); err != nil {
		return Run{}, err
	}
	path := filepath.Join(a.workspace.StateDir, "runs", runID+".json")
	var run Run
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return run, fmt.Errorf("run %q not found", runID)
		}
		return run, err
	}
	if err := json.Unmarshal(raw, &run); err != nil {
		return run, fmt.Errorf("decode run: %w", err)
	}
	if !finiteRunKind(run.Request.Kind) {
		return Run{}, fmt.Errorf("run %q is not a finite operation", runID)
	}
	run = a.reconcileExitedWorker(run)
	return run, nil
}

func (a *App) ListRuns(limit int) ([]Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	paths, err := filepath.Glob(filepath.Join(a.workspace.StateDir, "runs", "*.json"))
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	slices.Reverse(paths)
	runs := make([]Run, 0, min(limit, len(paths)))
	for _, path := range paths {
		var run Run
		raw, readErr := os.ReadFile(path)
		if readErr == nil && json.Unmarshal(raw, &run) == nil && finiteRunKind(run.Request.Kind) {
			run = a.reconcileExitedWorker(run)
			runs = append(runs, run)
			if len(runs) >= limit {
				break
			}
		}
	}
	return runs, nil
}

func (a *App) reconcileExitedWorker(run Run) Run {
	if isTerminal(run.Status) || run.PID <= 0 || time.Since(run.StartedAt) < 2*time.Second {
		return run
	}
	if err := syscall.Kill(run.PID, 0); err == nil || !errors.Is(err, syscall.ESRCH) {
		return run
	}
	finished := time.Now().UTC()
	exitCode := 1
	if run.Status == "cancelling" {
		run.Status = "cancelled"
		run.Error = "run worker exited after cancellation was requested"
		exitCode = 130
		_ = os.Remove(a.runCancellationPath(run.ID))
	} else {
		run.Status = "failed"
		run.Error = "run worker exited before recording completion"
	}
	run.PID = 0
	run.ExitCode = &exitCode
	run.FinishedAt = &finished
	_ = a.writeRun(run)
	return run
}

func (a *App) TailLog(runID string, limit int) (LogTail, error) {
	return a.TailLogAfter(runID, limit, 0)
}

func (a *App) TailLogAfter(runID string, limit, cursor int) (LogTail, error) {
	run, err := a.GetRun(runID)
	if err != nil {
		return LogTail{}, err
	}
	if limit <= 0 {
		limit = 40
	}
	limit = min(limit, maxLogLines)
	file, err := os.Open(run.LogPath)
	if err != nil {
		if os.IsNotExist(err) {
			return LogTail{RunID: runID, Complete: isTerminal(run.Status), SourcePath: run.LogPath, NextCursor: cursor}, nil
		}
		return LogTail{}, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	count := 0
	for scanner.Scan() {
		count++
		if count <= cursor {
			continue
		}
		lines = append(lines, redactError(scanner.Text()))
		if len(lines) > limit {
			lines = lines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return LogTail{}, err
	}
	return LogTail{RunID: runID, Lines: lines, LineCount: count, Truncated: count-cursor > len(lines), Complete: isTerminal(run.Status), SourcePath: run.LogPath, NextCursor: count}, nil
}

func (a *App) TailGateLog(runID, gateID string, limit int) (LogTail, error) {
	return a.TailGateLogAfter(runID, gateID, limit, 0)
}

func (a *App) TailGateLogAfter(runID, gateID string, limit, cursor int) (LogTail, error) {
	if _, err := GateByID(gateID); err != nil {
		return LogTail{}, err
	}
	run, err := a.GetRun(runID)
	if err != nil {
		return LogTail{}, err
	}
	path := filepath.Join(a.workspace.StateDir, "runs", runID+"."+gateID+".log")
	if limit <= 0 {
		limit = 40
	}
	limit = min(limit, maxLogLines)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return LogTail{RunID: run.ID, Complete: isTerminal(run.Status), SourcePath: path, NextCursor: cursor}, nil
		}
		return LogTail{}, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	count := 0
	for scanner.Scan() {
		count++
		if count <= cursor {
			continue
		}
		lines = append(lines, redactError(scanner.Text()))
		if len(lines) > limit {
			lines = lines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return LogTail{}, err
	}
	return LogTail{RunID: run.ID, Lines: lines, LineCount: count, Truncated: count-cursor > len(lines), Complete: isTerminal(run.Status), SourcePath: path, NextCursor: count}, nil
}

func (a *App) CancelRun(runID string) (Run, error) {
	run, err := a.GetRun(runID)
	if err != nil {
		return Run{}, err
	}
	if isTerminal(run.Status) {
		return run, nil
	}
	if run.PID <= 0 {
		return Run{}, errors.New("run has no active process")
	}
	marker := a.runCancellationPath(run.ID)
	if err := os.WriteFile(marker, []byte("cancel requested\n"), 0o600); err != nil {
		return Run{}, fmt.Errorf("record cancellation: %w", err)
	}
	run.Status = "cancelling"
	if err := a.writeRun(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (a *App) runCancellationPath(runID string) string {
	return filepath.Join(a.workspace.StateDir, "runs", runID+".cancel")
}

func (a *App) runReadyPath(runID string) string {
	return filepath.Join(a.workspace.StateDir, "runs", runID+".ready")
}

func (a *App) waitUntilRunReady(ctx context.Context, runID string) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		if _, err := os.Stat(a.runReadyPath(runID)); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return errors.New("timed out waiting for run launcher state")
		case <-ticker.C:
		}
	}
}

func (a *App) watchRunCancellation(ctx context.Context, runID string, cancel context.CancelFunc, done <-chan struct{}) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(a.runCancellationPath(runID)); err == nil {
			cancel()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
		}
	}
}

func (a *App) WaitRun(ctx context.Context, runID string) (Run, error) {
	return a.WaitRunObserved(ctx, runID, nil)
}

func (a *App) WaitRunObserved(ctx context.Context, runID string, observer func(Run)) (Run, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	lastState := ""
	for {
		run, err := a.GetRun(runID)
		if err != nil {
			return Run{}, err
		}
		var state strings.Builder
		state.WriteString(run.Status)
		for _, gate := range run.GateResults {
			state.WriteString("\x00" + gate.GateID + "=" + gate.Status)
		}
		if observer != nil && state.String() != lastState {
			observer(run)
			lastState = state.String()
		}
		if isTerminal(run.Status) {
			if run.Status != "passed" {
				return run, fmt.Errorf("run %s: %s", run.Status, run.Error)
			}
			return run, nil
		}
		select {
		case <-ctx.Done():
			return run, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (a *App) writeRun(run Run) error {
	path := filepath.Join(a.workspace.StateDir, "runs", run.ID+".json")
	return writeJSONAtomic(path, run)
}

func writeJSONAtomic(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".hk-state-*")
	if err != nil {
		return fmt.Errorf("create temporary state: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary state: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}

func (a *App) executeRequest(ctx context.Context, runID string, request RunRequest, logWriter io.Writer) ([]GateResult, error) {
	switch request.Kind {
	case "setup":
		return nil, a.executeSetup(ctx, logWriter)
	case "qa":
		return a.executeQA(ctx, runID, request, logWriter)
	case "build":
		return nil, a.executeBuild(ctx, request, logWriter)
	case "smoke":
		return nil, a.executeSmoke(ctx, request.Variant, logWriter)
	default:
		return nil, fmt.Errorf("unsupported run kind %q", request.Kind)
	}
}

func (a *App) executeSetup(ctx context.Context, writer io.Writer) error {
	doctor := a.Doctor(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if !doctor.Capabilities.ContainerDevelopment {
		return errors.New("container development is unavailable; run ./hk doctor and fix Docker Compose")
	}
	_, _ = fmt.Fprintln(writer, "preparing pinned container development dependencies")
	if err := a.executeContainerSetup(ctx, writer); err != nil {
		return err
	}
	statuses := map[string]bool{}
	for _, check := range doctor.Checks {
		statuses[check.Name] = check.Status == "ok"
	}
	if !statuses["node"] || !statuses["npm"] {
		_, _ = fmt.Fprintln(writer, "host frontend QA dependencies: skipped (exact Node and npm are not available)")
		return nil
	}
	_, _ = fmt.Fprintln(writer, "preparing host frontend QA dependencies")
	if err := a.prepareHostFrontendDependencies(ctx, writer); err != nil {
		return err
	}
	return a.runCommand(ctx, writer, commandSpec{Args: []string{"npx", "playwright", "install", "chromium"}, Dir: "frontend/dashboard", Display: "npx playwright install chromium [shared browser cache]"})
}

func (a *App) executeContainerSetup(ctx context.Context, writer io.Writer) error {
	variant, _ := VariantByID("self-hosted")
	for _, suffix := range [][]string{
		{"pull", "backend", "mailpit"},
		{"build", "frontend"},
		{"run", "--rm", "--no-deps", "backend", "go", "mod", "download"},
		{"run", "--rm", "--no-deps", "frontend", "npm", "ci", "--no-audit", "--no-fund"},
	} {
		if err := a.runCommand(ctx, writer, commandSpec{Args: a.composeArgs(suffix...), Env: a.ComposeEnvironment(variant)}); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) prepareHostFrontendDependencies(ctx context.Context, writer io.Writer) error {
	return a.runCommand(ctx, writer, commandSpec{Args: []string{"npm", "ci", "--no-audit", "--no-fund"}, Dir: "frontend/dashboard"})
}

func (a *App) resetDevData(ctx context.Context, writer io.Writer) error {
	volumeNames, err := a.containerDevDataVolumes(ctx)
	if err != nil {
		return err
	}
	if len(volumeNames) == 0 {
		_, _ = fmt.Fprintln(writer, "$ isolated development data is already empty")
		return nil
	}
	args := append([]string{"docker", "volume", "rm"}, volumeNames...)
	return a.runCommand(ctx, writer, commandSpec{Args: args, Env: a.ComposeEnvironment(variants[0]), Display: "docker volume rm [isolated development data]"})
}

func (a *App) containerDevDataVolumes(ctx context.Context) ([]string, error) {
	command := exec.CommandContext( //nolint:gosec
		ctx,
		"docker",
		"volume",
		"ls",
		"--quiet",
		"--filter", "label=com.docker.compose.project="+a.workspace.ComposeProject,
		"--filter", "label=com.docker.compose.volume=hitkeep-dev-data",
	)
	command.Dir = a.workspace.Root
	command.Env = a.commandEnvironment(a.ComposeEnvironment(variants[0]))
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("find container development data: %w", err)
	}
	return compactSorted(strings.Split(string(output), "\n")), nil
}

func (a *App) executeBuild(ctx context.Context, request RunRequest, writer io.Writer) error {
	variant, err := VariantByID(request.Variant)
	if err != nil {
		return err
	}
	if err := a.ValidateProductionBoundary(ctx); err != nil {
		return err
	}
	tags := strings.Join(variant.BuildTags, " ")
	if request.Target == "binary" {
		if err := a.prepareHostFrontendDependencies(ctx, writer); err != nil {
			return err
		}
		if err := a.runCommand(ctx, writer, commandSpec{Args: []string{"npm", "run", "build:prod"}, Dir: "frontend/dashboard"}); err != nil {
			return err
		}
		if err := copyTree(filepath.Join(a.workspace.Root, "frontend", "dashboard", "dist", "dashboard", "browser"), filepath.Join(a.workspace.Root, "public")); err != nil {
			return err
		}
		output := filepath.Join(a.workspace.StateDir, "artifacts", "hitkeep-"+variant.ID)
		if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
			return err
		}
		return a.runCommand(ctx, writer, commandSpec{Args: []string{"go", "build", "-tags", tags, "-ldflags=-w -s -X hitkeep/cmd.Version=snapshot", "-o", output, "./cmd/hitkeep/main.go"}})
	}
	toolchain, err := a.ToolchainConfig()
	if err != nil {
		return err
	}
	args := []string{
		"docker", "buildx", "build", ".", "--target", "local-image",
		"--build-arg", "GOLANG_VERSION=" + toolchain.Go,
		"--build-arg", "NODE_VERSION=" + toolchain.Node,
		"--build-arg", "NPM_VERSION=" + toolchain.NPM,
		"--build-arg", "GO_BUILD_TAGS=" + tags,
		"--build-arg", "HITKEEP_VARIANT=" + variant.ID,
		"--tag", a.localImageRef(variant), "--load",
	}
	return a.runCommand(ctx, writer, commandSpec{Args: args})
}

func copyTree(source, destination string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	sourceRoot, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer sourceRoot.Close()
	destinationRoot, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer destinationRoot.Close()
	return fs.WalkDir(sourceRoot.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse dashboard symlink %s", path)
		}
		if entry.IsDir() {
			return destinationRoot.MkdirAll(path, 0o755)
		}
		data, err := sourceRoot.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return destinationRoot.WriteFile(path, data, info.Mode().Perm())
	})
}

func (a *App) executeSmoke(ctx context.Context, variantID string, writer io.Writer) error {
	variant, err := VariantByID(variantID)
	if err != nil {
		return err
	}
	if err := a.executeBuild(ctx, RunRequest{Kind: "build", Variant: variant.ID, Target: "image"}, writer); err != nil {
		return err
	}
	args := []string{"scripts/docker-smoke.sh", a.localImageRef(variant), variant.ID}
	if variant.ID == "cloud" {
		args = append(args, "--cloud")
	}
	return a.runCommand(ctx, writer, commandSpec{Args: args})
}

func (a *App) VerifyVariantBuild(ctx context.Context, variantID string, writer io.Writer) error {
	variant, err := VariantByID(variantID)
	if err != nil {
		return err
	}
	directory, err := os.MkdirTemp(a.workspace.StateDir, "build-check-")
	if err != nil {
		return fmt.Errorf("create build-check directory: %w", err)
	}
	defer os.RemoveAll(directory)
	output := filepath.Join(directory, "hitkeep-"+variant.ID)
	return a.runCommand(ctx, writer, commandSpec{
		Args:    []string{"go", "build", "-tags", strings.Join(variant.BuildTags, " "), "-o", output, "./cmd/hitkeep/main.go"},
		Display: "go build [" + variant.ID + " variant; temporary workspace-state output]",
	})
}

func (a *App) executeQA(ctx context.Context, runID string, request RunRequest, writer io.Writer) ([]GateResult, error) {
	gateIDs := request.GateIDs
	if len(gateIDs) == 0 {
		gateIDs = profileGateIDs(request.Profile)
	}
	type gateExecution struct {
		id  string
		err error
	}
	jobs := make(chan string)
	results := make(chan gateExecution, len(gateIDs))
	var writerMu sync.Mutex
	var progressMu sync.Mutex
	progress := make(map[string]GateResult, len(gateIDs))
	for _, gateID := range gateIDs {
		progress[gateID] = GateResult{GateID: gateID, Status: "queued", LogPath: filepath.Join(a.workspace.StateDir, "runs", runID+"."+gateID+".log")}
	}
	persistProgress := func(gateID, status string, gateErr error, startedAt, finishedAt *time.Time) {
		progressMu.Lock()
		defer progressMu.Unlock()
		value := progress[gateID]
		value.Status = status
		if gateErr != nil {
			value.Error = redactError(gateErr.Error())
		}
		if startedAt != nil {
			value.StartedAt = startedAt
		}
		if finishedAt != nil {
			value.FinishedAt = finishedAt
			if value.StartedAt != nil {
				value.DurationMS = finishedAt.Sub(*value.StartedAt).Milliseconds()
			}
		}
		progress[gateID] = value
		snapshot := make([]GateResult, 0, len(progress))
		for _, result := range progress {
			snapshot = append(snapshot, result)
		}
		slices.SortFunc(snapshot, func(a, b GateResult) int { return strings.Compare(a.GateID, b.GateID) })
		if run, err := a.GetRun(runID); err == nil {
			run.GateResults = snapshot
			_ = a.writeRun(run)
		}
	}
	worker := func() {
		for id := range jobs {
			gate, err := GateByID(id)
			if err == nil {
				gateLogPath := filepath.Join(a.workspace.StateDir, "runs", runID+"."+gate.ID+".log")
				gateLog, openErr := os.OpenFile(gateLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
				if openErr != nil {
					finished := time.Now().UTC()
					persistProgress(id, "failed", openErr, nil, &finished)
					results <- gateExecution{id: id, err: openErr}
					continue
				}
				writerMu.Lock()
				_, _ = fmt.Fprintf(writer, "\n[%s] %s\n", gate.ID, gate.Description)
				writerMu.Unlock()
				target := io.MultiWriter(&lockedWriter{writer: writer, mu: &writerMu}, gateLog)
				environment := append(a.ComposeEnvironment(variants[0]), "GOFLAGS=-tags="+strings.Join(variants[0].BuildTags, ","))
				if strings.HasPrefix(gate.ID, "cloud-") {
					environment = append(a.ComposeEnvironment(variants[1]), "GOFLAGS=-tags="+strings.Join(variants[1].BuildTags, ","))
				}
				gateContext := ctx
				cancel := func() {}
				if timeout, parseErr := time.ParseDuration(gate.Timeout); parseErr == nil {
					gateContext, cancel = context.WithTimeout(ctx, timeout)
				}
				persistProgress(id, "waiting", nil, nil, nil)
				releaseSlots, acquireErr := a.acquireQASlots(gateContext, gate.Weight)
				if acquireErr != nil {
					cancel()
					_ = gateLog.Close()
					finished := time.Now().UTC()
					status := "failed"
					recordedErr := acquireErr
					if errors.Is(ctx.Err(), context.Canceled) {
						status = "cancelled"
						recordedErr = nil
					}
					persistProgress(id, status, recordedErr, nil, &finished)
					results <- gateExecution{id: id, err: acquireErr}
					continue
				}
				started := time.Now().UTC()
				persistProgress(id, "running", nil, &started, nil)
				switch gate.ID {
				case "go-format":
					var result SourceChangeResult
					result, err = a.FormatGo(false)
					writeSourceResult(target, result)
				case "go-fix":
					var result SourceChangeResult
					result, err = a.FixGo(gateContext, false)
					writeSourceResult(target, result)
				case "go-race-database":
					_, err = a.RunRaceShard(gateContext, "database", target)
				case "go-race-server":
					_, err = a.RunRaceShard(gateContext, "server", target)
				case "go-race-rest":
					_, err = a.RunRaceShard(gateContext, "rest", target)
				case "cloud-test":
					_, err = a.RunCloudTests(gateContext, target)
				case "cloud-build":
					err = a.VerifyVariantBuild(gateContext, "cloud", target)
				case "self-hosted-image":
					err = a.executeSmoke(gateContext, "self-hosted", target)
				case "cloud-image":
					err = a.executeSmoke(gateContext, "cloud", target)
				default:
					err = a.runCommand(gateContext, target, commandSpec{Args: gate.Command, Dir: gate.WorkingDir, Env: environment})
				}
				releaseSlots()
				cancel()
				_ = gateLog.Close()
				finished := time.Now().UTC()
				status := "passed"
				recordedErr := err
				if errors.Is(ctx.Err(), context.Canceled) {
					status = "cancelled"
					recordedErr = nil
				} else if err != nil {
					status = "failed"
				}
				persistProgress(id, status, recordedErr, &started, &finished)
			} else {
				finished := time.Now().UTC()
				persistProgress(id, "failed", err, nil, &finished)
			}
			results <- gateExecution{id: id, err: err}
		}
	}
	workers := min(maxParallelism(), max(1, len(gateIDs)))
	for range workers {
		go worker()
	}
	go func() {
		defer close(jobs)
		for _, id := range gateIDs {
			jobs <- id
		}
	}()
	var failures []string
	for range gateIDs {
		result := <-results
		if result.err != nil {
			failures = append(failures, result.id+": "+result.err.Error())
		}
	}
	progressMu.Lock()
	gateResults := make([]GateResult, 0, len(progress))
	for _, result := range progress {
		gateResults = append(gateResults, result)
	}
	progressMu.Unlock()
	slices.SortFunc(gateResults, func(a, b GateResult) int { return strings.Compare(a.GateID, b.GateID) })
	if err := ctx.Err(); err != nil {
		return gateResults, err
	}
	if len(failures) > 0 {
		return gateResults, errors.New(strings.Join(failures, "; "))
	}
	return gateResults, nil
}

func writeSourceResult(writer io.Writer, result SourceChangeResult) {
	if result.ChangedFileCount == 0 {
		_, _ = fmt.Fprintf(writer, "%s: current\n", result.Tool)
		return
	}
	_, _ = fmt.Fprintf(writer, "%s: %d changed file(s)\n", result.Tool, result.ChangedFileCount)
	for _, path := range result.ChangedFiles {
		_, _ = fmt.Fprintln(writer, path)
	}
	if result.Truncated {
		_, _ = fmt.Fprintln(writer, "[additional paths omitted]")
	}
}

func (a *App) acquireQASlots(ctx context.Context, weight int) (func(), error) {
	total := maxParallelism()
	needed := min(max(1, weight), total)
	directory := filepath.Join(filepath.Dir(filepath.Dir(a.workspace.StateDir)), "scheduler")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	queueDirectory := filepath.Join(directory, "queue")
	if err := os.MkdirAll(queueDirectory, 0o700); err != nil {
		return nil, err
	}
	waiterName := fmt.Sprintf("%020d-%d-%s", time.Now().UnixNano(), os.Getpid(), uuid.NewString())
	waiterPath := filepath.Join(queueDirectory, waiterName)
	if err := os.WriteFile(waiterPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		return nil, err
	}
	defer os.Remove(waiterPath)
	for {
		queueLock, lockErr := lockSchedulerQueue(directory)
		if lockErr != nil {
			return nil, lockErr
		}
		first, queueErr := firstLiveQAWaiter(queueDirectory)
		if queueErr != nil {
			unlockStateRoot(queueLock)
			return nil, queueErr
		}
		if first == waiterName {
			files := tryAcquireQASlotFiles(directory, total, needed)
			if len(files) == needed {
				_ = os.Remove(waiterPath)
				unlockStateRoot(queueLock)
				return func() {
					for _, file := range files {
						_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
						_ = file.Close()
					}
				}, nil
			}
			releaseQASlotFiles(files)
		}
		unlockStateRoot(queueLock)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func lockSchedulerQueue(directory string) (*os.File, error) {
	file, err := os.OpenFile(filepath.Join(directory, "queue.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func firstLiveQAWaiter(directory string) (string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		raw, readErr := os.ReadFile(path)
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
		if readErr != nil || parseErr != nil || pid <= 0 || processExited(pid) {
			_ = os.Remove(path)
			continue
		}
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	if len(names) == 0 {
		return "", nil
	}
	return names[0], nil
}

func processExited(pid int) bool {
	err := syscall.Kill(pid, 0)
	return errors.Is(err, syscall.ESRCH)
}

func tryAcquireQASlotFiles(directory string, total, needed int) []*os.File {
	var files []*os.File
	for index := 0; index < total && len(files) < needed; index++ {
		file, err := os.OpenFile(filepath.Join(directory, fmt.Sprintf("slot-%02d.lock", index)), os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			continue
		}
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			_ = file.Close()
			continue
		}
		files = append(files, file)
	}
	return files
}

func releaseQASlotFiles(files []*os.File) {
	for _, file := range files {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}
}

type commandSpec struct {
	Args    []string
	Dir     string
	Env     []string
	Display string
}

func (a *App) runCommand(ctx context.Context, writer io.Writer, spec commandSpec) error {
	if len(spec.Args) == 0 {
		return errors.New("empty command")
	}
	args := spec.Args
	environment := spec.Env
	if agentOutputEnabled(ctx) {
		args = agentOptimizedCommand(args)
		environment = append(slices.Clone(environment), "HK_AGENT_OUTPUT=json", "NO_COLOR=1", "TERM=dumb")
	}
	display := spec.Display
	if display == "" {
		display = strings.Join(args, " ")
	} else if agentOutputEnabled(ctx) && !slices.Equal(args, spec.Args) {
		display += " [agent-json]"
	}
	_, _ = fmt.Fprintln(writer, "$ "+display)
	// Commands are selected from the closed catalog; user-controlled fields are enum or pattern validated.
	command := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	command.WaitDelay = 5 * time.Second
	command.Dir = a.workspace.Root
	if spec.Dir != "" {
		resolved, err := ValidateWorkspacePath(a.workspace, filepath.Join(a.workspace.Root, spec.Dir))
		if err != nil {
			return err
		}
		command.Dir = resolved
	}
	command.Env = a.commandEnvironment(environment)
	redacted := &redactingWriter{writer: writer}
	command.Stdout = redacted
	command.Stderr = redacted
	runErr := command.Run()
	flushErr := redacted.Flush()
	if runErr != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return fmt.Errorf("command failed with exit code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("run command: %w", runErr)
	}
	if flushErr != nil {
		return fmt.Errorf("write command log: %w", flushErr)
	}
	return nil
}

func (a *App) runArtifacts(run Run) []string {
	var artifacts []string
	if run.Request.Kind == "build" {
		if run.Request.Target == "binary" {
			artifacts = append(artifacts, filepath.Join(a.workspace.StateDir, "artifacts", "hitkeep-"+run.Request.Variant))
		} else if variant, err := VariantByID(run.Request.Variant); err == nil {
			artifacts = append(artifacts, "image://"+a.localImageRef(variant))
		}
	}
	if run.Request.Kind == "qa" {
		for _, gateID := range run.Request.GateIDs {
			artifacts = append(artifacts, filepath.Join(a.workspace.StateDir, "runs", run.ID+"."+gateID+".log"))
		}
	}
	return artifacts
}

type lockedWriter struct {
	writer io.Writer
	mu     *sync.Mutex
}

func (w *lockedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(data)
}

func normalizeRunRequest(request RunRequest) RunRequest {
	if request.Variant == "" && slices.Contains([]string{"build", "smoke"}, request.Kind) {
		request.Variant = "self-hosted"
	}
	if request.Profile == "" && request.Kind == "qa" {
		request.Profile = "changed"
	}
	if request.Target == "" && request.Kind == "build" {
		request.Target = "binary"
	}
	request.GateIDs = compactSorted(request.GateIDs)
	return request
}

func runRequestsEqual(left, right RunRequest) bool {
	return left.Kind == right.Kind && left.Variant == right.Variant && left.Profile == right.Profile && left.Target == right.Target && slices.Equal(left.GateIDs, right.GateIDs)
}

func summarizeRun(run Run) RunSummary {
	summary := RunSummary{ID: run.ID, Request: run.Request, Status: run.Status, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt}
	if run.FinishedAt != nil {
		summary.DurationMS = run.FinishedAt.Sub(run.StartedAt).Milliseconds()
	}
	for _, gate := range run.GateResults {
		if gate.Status == "failed" {
			summary.FailedGateIDs = append(summary.FailedGateIDs, gate.GateID)
		}
	}
	return summary
}

func runSummariesFromState(stateDir string, activeOnly bool, limit int) []RunSummary {
	paths, _ := filepath.Glob(filepath.Join(stateDir, "runs", "*.json"))
	slices.Sort(paths)
	slices.Reverse(paths)
	var summaries []RunSummary
	for _, path := range paths {
		var run Run
		raw, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(raw, &run) != nil || !finiteRunKind(run.Request.Kind) || activeOnly && isTerminal(run.Status) {
			continue
		}
		summaries = append(summaries, summarizeRun(run))
		if len(summaries) >= limit {
			break
		}
	}
	return summaries
}

func mergedCommandEnvironment(overrides []string) []string {
	allowed := map[string]bool{
		"PATH": true, "HOME": true, "TMPDIR": true, "TMP": true, "TEMP": true, "USER": true, "LOGNAME": true, "SHELL": true,
		"LANG": true, "LC_ALL": true, "LC_CTYPE": true, "TERM": true, "NO_COLOR": true, "CI": true,
		"GOENV": true, "GOCACHE": true, "GOMODCACHE": true, "GOPATH": true, "GOTOOLCHAIN": true, "GOPROXY": true, "GONOSUMDB": true, "GOPRIVATE": true,
		"CGO_ENABLED": true, "CC": true, "CXX": true, "AR": true, "PKG_CONFIG_PATH": true, "SDKROOT": true, "DEVELOPER_DIR": true,
		"NPM_CONFIG_CACHE": true, "PLAYWRIGHT_BROWSERS_PATH": true, "DOCKER_HOST": true, "DOCKER_CONTEXT": true, "BUILDKIT_PROGRESS": true,
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true, "ALL_PROXY": true, "http_proxy": true, "https_proxy": true, "no_proxy": true, "all_proxy": true,
	}
	values := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, found := strings.Cut(entry, "=")
		if found && allowed[key] {
			values[key] = value
		}
	}
	for _, entry := range overrides {
		key, value, found := strings.Cut(entry, "=")
		if found {
			values[key] = value
		}
	}
	environment := make([]string, 0, len(values))
	for key, value := range values {
		environment = append(environment, key+"="+value)
	}
	slices.Sort(environment)
	return environment
}

type redactingWriter struct {
	writer  io.Writer
	mu      sync.Mutex
	pending []byte
}

func (w *redactingWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	original := len(data)
	w.pending = append(w.pending, data...)
	for {
		index := bytes.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		line := w.pending[:index+1]
		if _, err := w.writer.Write([]byte(redactError(string(line)))); err != nil {
			return 0, err
		}
		w.pending = w.pending[index+1:]
	}
	const maxPending = 1 << 20
	const retainedSuffix = 512
	if len(w.pending) > maxPending {
		flushLength := len(w.pending) - retainedSuffix
		if _, err := w.writer.Write([]byte(redactError(string(w.pending[:flushLength])))); err != nil {
			return 0, err
		}
		w.pending = w.pending[flushLength:]
	}
	return original, nil
}

func (w *redactingWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) == 0 {
		return nil
	}
	_, err := w.writer.Write([]byte(redactError(string(w.pending))))
	w.pending = nil
	return err
}

func validateRunID(runID string) error {
	if runID == "" || strings.ContainsAny(runID, `/\\`) || strings.Contains(runID, "..") {
		return errors.New("invalid run ID")
	}
	return nil
}

func isTerminal(status string) bool {
	return status == "passed" || status == "failed" || status == "cancelled"
}

func redactError(value string) string {
	value = ansiEscapePattern.ReplaceAllString(value, "")
	value = bearerPattern.ReplaceAllString(value, "Bearer [redacted]")
	value = secretAssignmentPattern.ReplaceAllString(value, `${1}${2}[redacted]`)
	return value
}

var secretAssignmentPattern = regexp.MustCompile(`(?i)(token|secret|password|api[_-]?key|authorization)(["']?\s*[:=]\s*["']?)([^"'\s,}]+)`)
var bearerPattern = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/=-]+`)
var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
