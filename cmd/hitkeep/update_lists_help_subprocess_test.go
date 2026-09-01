package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"

	hitkeepcmd "hitkeep/cmd"
)

func TestExecuteUpdateListHelpSubprocessParity(t *testing.T) {
	if commandName := os.Getenv("HITKEEP_UPDATE_LIST_HELP_SUBPROCESS"); commandName != "" {
		os.Args = []string{os.Args[0], commandName, os.Getenv("HITKEEP_UPDATE_LIST_HELP_FLAG")}
		root := hitkeepcmd.NewRootCommand(slog.New(slog.NewTextHandler(io.Discard, nil)))
		root.SetArgs([]string{commandName, os.Getenv("HITKEEP_UPDATE_LIST_HELP_FLAG")})
		root.SetOut(os.Stdout)
		root.SetErr(os.Stderr)
		os.Exit(execute(context.Background(), root, slog.New(slog.NewTextHandler(io.Discard, nil))))
	}

	tests := []struct {
		commandName string
		helpFlag    string
	}{
		{"update-spam-lists", "-h"},
		{"update-spam-lists", "--help"},
		{"update-ai-agent-lists", "-h"},
		{"update-ai-agent-lists", "--help"},
	}
	for _, tt := range tests {
		t.Run(tt.commandName+"/"+tt.helpFlag, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestExecuteUpdateListHelpSubprocessParity$")
			command.Env = append(os.Environ(), "HITKEEP_UPDATE_LIST_HELP_SUBPROCESS="+tt.commandName, "HITKEEP_UPDATE_LIST_HELP_FLAG="+tt.helpFlag)
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			if err := command.Run(); err != nil {
				t.Fatalf("help subprocess error = %v", err)
			}
			if got := stdout.String(); got != "" {
				t.Errorf("stdout = %q, want empty", got)
			}
			expectedUsage := "Usage of " + tt.commandName + ":\n"
			if got := stderr.String(); !strings.HasPrefix(got, expectedUsage) || strings.Count(got, expectedUsage) != 1 || strings.Contains(got, "\nError:") {
				t.Errorf("stderr = %q, want one usage block without command error", got)
			}
		})
	}
}
