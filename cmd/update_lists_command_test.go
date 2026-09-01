package hitkeepcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestUpdateListCommandsUseCobraContextAndStreams(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(t.Context(), contextKey{}, "value")

	tests := []struct {
		name string
		args []string
	}{
		{name: "spam", args: []string{"update-spam-lists", "--output", "spam.json"}},
		{name: "AI agents", args: []string{"update-ai-agent-lists", "--output", "agents.json"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			called := false
			actions := rootActions{
				updateSpamLists: func(got context.Context, args []string, out, errOut io.Writer, configFile string) error {
					called = true
					if got.Value(contextKey{}) != "value" {
						t.Fatal("spam command did not receive the Cobra context")
					}
					if out != &stdout || errOut != &stderr {
						t.Fatal("spam command did not receive command streams")
					}
					if want := "--output spam.json"; strings.Join(args, " ") != want {
						t.Fatalf("spam args = %q, want %q", args, want)
					}
					_, _ = io.WriteString(out, "spam output\n")
					return nil
				},
				updateAIAgentLists: func(got context.Context, args []string, out, errOut io.Writer, configFile string) error {
					called = true
					if got.Value(contextKey{}) != "value" {
						t.Fatal("AI-agent command did not receive the Cobra context")
					}
					if out != &stdout || errOut != &stderr {
						t.Fatal("AI-agent command did not receive command streams")
					}
					if want := "--output agents.json"; strings.Join(args, " ") != want {
						t.Fatalf("AI-agent args = %q, want %q", args, want)
					}
					_, _ = io.WriteString(out, "AI-agent output\n")
					return nil
				},
			}
			root := newRootCommand(actions)
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs(tt.args)
			if err := root.ExecuteContext(ctx); err != nil {
				t.Fatalf("ExecuteContext() error = %v", err)
			}
			if !called {
				t.Fatal("update command was not called")
			}
		})
	}
}

func TestUpdateListCommandsPreserveFlagExitCode(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tests := []struct {
		name string
		run  func(context.Context, []string, io.Writer, io.Writer, string, *slog.Logger) error
	}{
		{name: "spam", run: UpdateSpamLists},
		{name: "AI agents", run: UpdateAIAgentLists},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := tt.run(t.Context(), []string{"--unknown"}, &stdout, &stderr, "", logger)
			exitErr, ok := errors.AsType[*ExitError](err)
			if !ok || exitErr.Code != 2 {
				t.Fatalf("error = %v, want ExitError code 2", err)
			}
			if got := stdout.String(); got != "" {
				t.Errorf("stdout = %q, want empty", got)
			}
			if !strings.Contains(stderr.String(), "flag provided but not defined: -unknown") {
				t.Errorf("stderr = %q, want stdlib flag error", stderr.String())
			}
		})
	}
}

func TestUpdateListCommandsReturnActionErrors(t *testing.T) {
	want := errors.New("update failed")
	root := newRootCommand(rootActions{
		updateSpamLists:    func(context.Context, []string, io.Writer, io.Writer, string) error { return want },
		updateAIAgentLists: func(context.Context, []string, io.Writer, io.Writer, string) error { return want },
	})
	root.SetArgs([]string{"update-spam-lists"})
	if err := root.ExecuteContext(t.Context()); !errors.Is(err, want) {
		t.Fatalf("ExecuteContext() error = %v, want %v", err, want)
	}
}
