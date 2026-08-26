package hitkeepcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoverUsesCommandArgumentsAndStreams(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	root := NewRootCommand(logger)
	var stdout, stderr bytes.Buffer
	root.SetIn(strings.NewReader(""))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"recover", "unknown"})

	err := root.ExecuteContext(t.Context())
	if code := recoveryExitCode(err); code != 1 {
		t.Fatalf("recoveryExitCode() = %d, want 1 (err = %v)", code, err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	want := "Unknown recover subcommand: \"unknown\"\n\n" + recoverUsage + "\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRecoverUnknownSubcommandProcessParity(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "hitkeep")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = "hitkeep"
	build.Env = append(os.Environ(), "GOROOT=")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build hitkeep: %v\n%s", err, output)
	}

	command := exec.Command(binary, "recover", "unknown")
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if code := recoveryExitCode(err); code != 1 {
		t.Fatalf("process exit code = %d, want 1 (err = %v)", code, err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	want := "Unknown recover subcommand: \"unknown\"\n\n" + recoverUsage + "\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRecoverCommandHelpAndNoSubcommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, subcommand := range []string{"disable-2fa", "restore-backup", "restore-database-bundle", "rebuild-default-tenant", "import-archives"} {
		t.Run(subcommand, func(t *testing.T) {
			root := NewRootCommand(logger)
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs([]string{"recover", subcommand, "--help"})
			if code := recoveryExitCode(root.ExecuteContext(t.Context())); code != 0 {
				t.Fatalf("help exit code = %d, want 0", code)
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), "Usage of "+subcommand+":") {
				t.Fatalf("help output stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}

	root := NewRootCommand(logger)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"recover"})
	if code := recoveryExitCode(root.ExecuteContext(t.Context())); code != 1 {
		t.Fatalf("no-subcommand exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 || stderr.String() != recoverUsage+"\n" {
		t.Fatalf("no-subcommand output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRecoverFlagParseProcessParity(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	root := NewRootCommand(logger)
	var expectedOut, expectedErr bytes.Buffer
	root.SetOut(&expectedOut)
	root.SetErr(&expectedErr)
	root.SetArgs([]string{"recover", "disable-2fa", "--unknown"})
	if code := recoveryExitCode(root.ExecuteContext(t.Context())); code != 2 {
		t.Fatalf("in-memory exit code = %d, want 2", code)
	}

	binary := filepath.Join(t.TempDir(), "hitkeep")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = "hitkeep"
	build.Env = append(os.Environ(), "GOROOT=")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build hitkeep: %v\n%s", err, output)
	}
	command := exec.Command(binary, "recover", "disable-2fa", "--unknown")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if code := recoveryExitCode(command.Run()); code != 2 {
		t.Fatalf("process exit code = %d, want 2", code)
	}
	if stdout.String() != expectedOut.String() || stderr.String() != expectedErr.String() {
		t.Fatalf("process output differs from in-memory command\nstdout: %q != %q\nstderr: %q != %q", stdout.String(), expectedOut.String(), stderr.String(), expectedErr.String())
	}
}

func TestRecoverRootPassesContextAndStreams(t *testing.T) {
	type contextKey struct{}
	input := strings.NewReader("confirm")
	var stdout, stderr bytes.Buffer
	ctx := context.WithValue(t.Context(), contextKey{}, "recovery")
	var gotContext context.Context
	var gotInput io.Reader
	var gotOut, gotErr io.Writer

	root := newRootCommand(rootActions{
		recover: func(ctx context.Context, _ []string, in io.Reader, out, errOut io.Writer) error {
			gotContext, gotInput, gotOut, gotErr = ctx, in, out, errOut
			return nil
		},
	})
	root.SetIn(input)
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"recover"})
	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if got := gotContext.Value(contextKey{}); got != "recovery" {
		t.Errorf("context value = %v, want recovery", got)
	}
	if gotInput != input || gotOut != &stdout || gotErr != &stderr {
		t.Error("recovery did not receive the command streams")
	}
}

func recoveryExitCode(err error) int {
	if err == nil {
		return 0
	}
	var recoveryErr *RecoveryError
	if errors.As(err, &recoveryErr) {
		return recoveryErr.Code
	}
	var processErr *exec.ExitError
	if errors.As(err, &processErr) {
		return processErr.ExitCode()
	}
	return -1
}

func TestRecoverReceivesCanceledCommandContext(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	root := NewRootCommand(logger)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	root.SetArgs([]string{"recover", "unknown"})
	if code := recoveryExitCode(root.ExecuteContext(ctx)); code != 1 {
		t.Fatalf("recoveryExitCode() = %d, want 1", code)
	}
}
