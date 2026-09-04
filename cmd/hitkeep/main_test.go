package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"testing"

	"github.com/spf13/cobra"

	hitkeepcmd "hitkeep/cmd"
)

func TestExecuteHealthcheckSubprocess(t *testing.T) {
	if os.Getenv("HITKEEP_EXECUTE_HEALTHCHECK_SUBPROCESS") == "1" {
		root := &cobra.Command{
			Use:           "hitkeep",
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE: func(*cobra.Command, []string) error {
				return &hitkeepcmd.HealthcheckError{Err: errors.New("unhealthy")}
			},
		}
		root.SetOut(os.Stdout)
		root.SetErr(os.Stderr)
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		os.Exit(execute(context.Background(), root, logger))
	}

	command := exec.Command(os.Args[0], "-test.run=^TestExecuteHealthcheckSubprocess$")
	command.Env = append(os.Environ(), "HITKEEP_EXECUTE_HEALTHCHECK_SUBPROCESS=1")
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err == nil {
		t.Fatal("healthcheck subprocess succeeded, want exit code 1")
	}
	if got := err.(*exec.ExitError).ExitCode(); got != 1 {
		t.Errorf("healthcheck subprocess exit code = %d, want 1", got)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("healthcheck subprocess stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "Healthcheck failed: unhealthy\n" {
		t.Errorf("healthcheck subprocess stderr = %q, want %q", got, "Healthcheck failed: unhealthy\n")
	}
}

func TestExecuteExitErrorDoesNotEmitAgain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	root := &cobra.Command{
		Use:           "hitkeep",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(*cobra.Command, []string) error {
			return &hitkeepcmd.ExitError{Code: 2}
		},
	}
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if got := execute(t.Context(), root, logger); got != 2 {
		t.Fatalf("execute exit code = %d, want 2", got)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
}

func TestExecuteHealthcheckFailureUsesCommandStreams(t *testing.T) {
	var stdout, stderr bytes.Buffer
	root := &cobra.Command{
		Use:           "hitkeep",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(*cobra.Command, []string) error {
			return &hitkeepcmd.HealthcheckError{Err: errors.New("unhealthy")}
		},
	}
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if got := execute(t.Context(), root, logger); got != 1 {
		t.Fatalf("execute exit code = %d, want 1", got)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "Healthcheck failed: unhealthy\n" {
		t.Errorf("stderr = %q, want %q", got, "Healthcheck failed: unhealthy\n")
	}
}
