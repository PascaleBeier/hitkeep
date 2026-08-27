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

func TestExecuteImportSubprocessParity(t *testing.T) {
	if mode := os.Getenv("HITKEEP_IMPORT_SUBPROCESS"); mode != "" {
		root := hitkeepcmd.NewRootCommand(slog.New(slog.NewTextHandler(io.Discard, nil)))
		args := []string{"import", "list", "--help"}
		if mode == "missing-token" {
			args = []string{"import", "list", "--site", "site"}
		}
		root.SetArgs(args)
		root.SetOut(os.Stdout)
		root.SetErr(os.Stderr)
		os.Exit(execute(context.Background(), root, slog.New(slog.NewTextHandler(io.Discard, nil))))
	}

	tests := []struct {
		name       string
		mode       string
		wantCode   int
		wantPrefix string
	}{
		{name: "help", mode: "help", wantCode: 0, wantPrefix: "Usage of import:\n"},
		{name: "missing token", mode: "missing-token", wantCode: 2, wantPrefix: "--token or HITKEEP_API_TOKEN is required\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestExecuteImportSubprocessParity$")
			command.Env = append(os.Environ(), "HITKEEP_IMPORT_SUBPROCESS="+tt.mode, "HITKEEP_API_TOKEN=", "HITKEEP_API_URL=", "HITKEEP_URL=")
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			if tt.wantCode == 0 {
				if err != nil {
					t.Fatalf("subprocess error = %v", err)
				}
			} else if exitErr, ok := errors.AsType[*exec.ExitError](err); !ok || exitErr.ExitCode() != tt.wantCode {
				t.Fatalf("subprocess error = %v, want exit code %d", err, tt.wantCode)
			}
			if got := stdout.String(); got != "" {
				t.Errorf("stdout = %q, want empty", got)
			}
			if got := stderr.String(); !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("stderr = %q, want prefix %q", got, tt.wantPrefix)
			}
		})
	}
}
