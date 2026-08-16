package devtool

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const (
	maxSourceResultFiles = 50
	maxSourceFileBytes   = 16 << 20
	maxFixOutputBytes    = 64 << 10
)

type sourceChangesPendingError struct {
	result SourceChangeResult
}

func (e sourceChangesPendingError) Error() string {
	return fmt.Sprintf("%d source file(s) require %s", e.result.ChangedFileCount, e.result.Tool)
}

func (e sourceChangesPendingError) ErrorData() any { return e.result }

// FormatGo checks or rewrites all repository-owned Go sources. It deliberately
// includes untracked files while excluding dependency and VCS directories.
func (a *App) FormatGo(write bool) (SourceChangeResult, error) {
	mode := "check"
	if write {
		mode = "write"
	}
	result := SourceChangeResult{Tool: "gofmt", Mode: mode, Current: true}
	root, err := os.OpenRoot(a.workspace.Root)
	if err != nil {
		return result, fmt.Errorf("open workspace root: %w", err)
	}
	defer root.Close()

	var changed []string
	err = filepath.WalkDir(a.workspace.Root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				if path != a.workspace.Root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".go" {
			return nil
		}
		relative, err := filepath.Rel(a.workspace.Root, path)
		if err != nil {
			return err
		}
		file, err := root.Open(filepath.ToSlash(relative))
		if err != nil {
			return fmt.Errorf("open %s: %w", relative, err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(file, maxSourceFileBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return fmt.Errorf("read %s: %w", relative, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", relative, closeErr)
		}
		if len(raw) > maxSourceFileBytes {
			return fmt.Errorf("source file exceeds %d bytes: %s", maxSourceFileBytes, relative)
		}
		formatted, err := format.Source(raw)
		if err != nil {
			return fmt.Errorf("format %s: %w", relative, err)
		}
		if bytes.Equal(raw, formatted) {
			return nil
		}
		changed = append(changed, filepath.ToSlash(relative))
		if !write {
			return nil
		}
		output, err := root.OpenFile(filepath.ToSlash(relative), os.O_WRONLY|os.O_TRUNC, 0)
		if err != nil {
			return fmt.Errorf("open %s for formatting: %w", relative, err)
		}
		if _, err := output.Write(formatted); err != nil {
			_ = output.Close()
			return fmt.Errorf("write %s: %w", relative, err)
		}
		if err := output.Close(); err != nil {
			return fmt.Errorf("close %s: %w", relative, err)
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	slices.Sort(changed)
	result.ChangedFileCount = len(changed)
	result.ChangedFiles, result.Truncated = boundedStrings(changed, maxSourceResultFiles)
	result.Current = len(changed) == 0 || write
	if len(changed) > 0 && !write {
		return result, sourceChangesPendingError{result: result}
	}
	return result, nil
}

// FormatFrontend checks or rewrites the dashboard with the repository-pinned
// oxfmt binary. Paths returned to callers are always workspace-relative.
func (a *App) FormatFrontend(ctx context.Context, write bool) (SourceChangeResult, error) {
	mode := "check"
	if write {
		mode = "write"
	}
	result := SourceChangeResult{Tool: "oxfmt", Mode: mode, Current: true}
	frontendDir, err := ValidateWorkspacePath(a.workspace, filepath.Join(a.workspace.Root, "frontend", "dashboard"))
	if err != nil {
		return result, err
	}
	formatter := filepath.Join(frontendDir, "node_modules", ".bin", "oxfmt")
	if info, statErr := os.Stat(formatter); statErr != nil || info.IsDir() {
		return result, errors.New("frontend formatter is unavailable; run ./hk setup first")
	}

	output, commandErr := a.runFrontendFormatter(ctx, formatter, frontendDir, "--list-different")
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	changed := frontendChangedFiles(output, a.workspace.Root, frontendDir)
	if commandErr != nil && len(changed) == 0 {
		return result, fmt.Errorf("check frontend formatting: %w: %s", commandErr, boundedFixOutput(output))
	}
	result.ChangedFileCount = len(changed)
	result.ChangedFiles, result.Truncated = boundedStrings(changed, maxSourceResultFiles)
	if len(changed) == 0 {
		return result, nil
	}
	if !write {
		result.Current = false
		return result, sourceChangesPendingError{result: result}
	}
	writeOutput, err := a.runFrontendFormatter(ctx, formatter, frontendDir, "--write")
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, fmt.Errorf("write frontend formatting: %w: %s", err, boundedFixOutput(writeOutput))
	}
	return result, nil
}

func (a *App) runFrontendFormatter(ctx context.Context, formatter, directory, mode string) ([]byte, error) {
	command := exec.CommandContext(ctx, formatter, mode) //nolint:gosec // executable is workspace-pinned and arguments are closed
	command.Dir = directory
	command.Env = a.commandEnvironment([]string{"NO_COLOR=1", "TERM=dumb"})
	buffer := &boundedBuffer{limit: maxFixOutputBytes}
	command.Stdout = buffer
	command.Stderr = buffer
	err := command.Run()
	return buffer.Bytes(), err
}

func frontendChangedFiles(output []byte, workspaceRoot, frontendDir string) []string {
	seen := map[string]bool{}
	var paths []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		value := strings.TrimSpace(scanner.Text())
		if value == "" {
			continue
		}
		path := value
		if !filepath.IsAbs(path) {
			path = filepath.Join(frontendDir, filepath.FromSlash(path))
		}
		path = filepath.Clean(path)
		relative, err := filepath.Rel(workspaceRoot, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		relative = filepath.ToSlash(relative)
		if seen[relative] {
			continue
		}
		seen[relative] = true
		paths = append(paths, relative)
	}
	slices.Sort(paths)
	return paths
}

// FixGo uses the pinned Go toolchain's own fix analyzers. Check mode relies on
// go fix -diff, so it reports drift without modifying the workspace.
func (a *App) FixGo(ctx context.Context, apply bool) (SourceChangeResult, error) {
	mode := "check"
	if apply {
		mode = "apply"
	}
	result := SourceChangeResult{Tool: "go fix", Mode: mode, Current: true}
	output, commandErr := a.runGoFix(ctx, true)
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	changed := goFixChangedFiles(output, a.workspace.Root)
	if commandErr != nil && len(changed) == 0 {
		return result, fmt.Errorf("go fix check: %w: %s", commandErr, boundedFixOutput(output))
	}
	result.ChangedFileCount = len(changed)
	result.ChangedFiles, result.Truncated = boundedStrings(changed, maxSourceResultFiles)
	if len(changed) == 0 {
		return result, nil
	}
	if !apply {
		result.Current = false
		return result, sourceChangesPendingError{result: result}
	}
	applyOutput, err := a.runGoFix(ctx, false)
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, fmt.Errorf("apply go fix: %w: %s", err, boundedFixOutput(applyOutput))
	}
	result.Current = true
	return result, nil
}

func (a *App) runGoFix(ctx context.Context, diff bool) ([]byte, error) {
	args := []string{"fix"}
	if diff {
		args = append(args, "-diff")
	}
	variant, _ := VariantByID("self-hosted")
	packages, err := a.listGoPackages(ctx, variant)
	if err != nil {
		return nil, err
	}
	args = append(args, packages...)
	command := exec.CommandContext(ctx, "go", args...) //nolint:gosec // fixed executable and closed arguments
	command.Dir = a.workspace.Root
	command.Env = a.commandEnvironment([]string{"GOFLAGS=" + goFlagsForTags(variant.BuildTags)})
	buffer := &boundedBuffer{limit: maxFixOutputBytes}
	command.Stdout = buffer
	command.Stderr = buffer
	runErr := command.Run()
	return buffer.Bytes(), runErr
}

func goFixChangedFiles(output []byte, workspaceRoot string) []string {
	seen := map[string]bool{}
	var paths []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "+++ ") {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
		path = strings.TrimSuffix(path, " (new)")
		path = strings.Trim(path, `"`)
		if path == "/dev/null" || path == "" {
			continue
		}
		if filepath.IsAbs(path) {
			if relative, err := filepath.Rel(workspaceRoot, path); err == nil {
				path = relative
			}
		}
		path = filepath.ToSlash(filepath.Clean(path))
		if strings.HasPrefix(path, "../") || path == ".." || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths
}

func boundedFixOutput(output []byte) string {
	value := strings.TrimSpace(redactError(string(output)))
	if value == "" {
		return "no diagnostic output"
	}
	return value
}

type boundedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		_, _ = b.buffer.Write(data[:min(len(data), remaining)])
	}
	return original, nil
}

func (b *boundedBuffer) Bytes() []byte { return bytes.Clone(b.buffer.Bytes()) }

var _ error = sourceChangesPendingError{}
var _ interface{ ErrorData() any } = sourceChangesPendingError{}
