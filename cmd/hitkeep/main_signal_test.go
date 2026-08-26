package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"

	"github.com/spf13/cobra"

	hitkeepcmd "hitkeep/cmd"
)

func TestExecuteSignalCancelsRecoveryAction(t *testing.T) {
	if os.Getenv("HITKEEP_EXECUTE_SIGNAL_SUBPROCESS") == "1" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		root := &cobra.Command{Use: "hitkeep", SilenceErrors: true, SilenceUsage: true}
		root.AddCommand(&cobra.Command{
			Use: "recover",
			RunE: func(command *cobra.Command, _ []string) error {
				_, _ = fmt.Fprintln(command.OutOrStdout(), "ready")
				<-command.Context().Done()
				return &hitkeepcmd.ExitError{Code: 1}
			},
		})
		root.SetArgs([]string{"recover"})
		root.SetOut(os.Stdout)
		root.SetErr(os.Stderr)
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		os.Exit(execute(ctx, root, logger))
	}

	command := exec.Command(os.Args[0], "-test.run=^TestExecuteSignalCancelsRecoveryAction$")
	command.Env = append(os.Environ(), "HITKEEP_EXECUTE_SIGNAL_SUBPROCESS=1")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(stdout)
	if line, err := reader.ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("recovery readiness = %q, %v; want ready", line, err)
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("signal subprocess succeeded, want exit code 1")
	} else if exitErr, ok := errors.AsType[*exec.ExitError](err); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("signal subprocess error = %v, want exit code 1", err)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
}
