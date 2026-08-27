package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"

	hitkeepcmd "hitkeep/cmd"
)

func TestExecuteUpdateListFlagSubprocessParity(t *testing.T) {
	if commandName := os.Getenv("HITKEEP_UPDATE_LIST_SUBPROCESS"); commandName != "" {
		os.Args = []string{os.Args[0], commandName, "--unknown"}
		root := hitkeepcmd.NewRootCommand(slog.New(slog.NewTextHandler(io.Discard, nil)))
		root.SetArgs([]string{commandName, "--unknown"})
		root.SetOut(os.Stdout)
		root.SetErr(os.Stderr)
		os.Exit(execute(context.Background(), root, slog.New(slog.NewTextHandler(io.Discard, nil))))
	}

	for _, commandName := range []string{"update-spam-lists", "update-ai-agent-lists"} {
		t.Run(commandName, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestExecuteUpdateListFlagSubprocessParity$")
			command.Env = append(os.Environ(), "HITKEEP_UPDATE_LIST_SUBPROCESS="+commandName)
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			if err == nil {
				t.Fatal("update-list subprocess succeeded, want exit code 2")
			}
			exitErr, ok := errors.AsType[*exec.ExitError](err)
			if !ok || exitErr.ExitCode() != 2 {
				t.Fatalf("subprocess error = %v, want exit code 2", err)
			}
			if got := stdout.String(); got != "" {
				t.Errorf("stdout = %q, want empty", got)
			}
			if got := stderr.String(); !strings.HasPrefix(got, "flag provided but not defined: -unknown\nUsage of "+commandName+":\n") {
				t.Errorf("stderr = %q, want stdlib flag failure without duplicate command error", got)
			}
		})
	}
}
