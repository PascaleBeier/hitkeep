package hitkeepcmd

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestUpdateListCommandsExposeOutputFlag(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, command := range []*cobra.Command{
		newUpdateSpamListsCommand(logger),
		newUpdateAIAgentListsCommand(logger),
	} {
		if command.Flags().Lookup("output") == nil {
			t.Fatalf("%s has no --output flag", command.Name())
		}
	}
}

func TestUpdateListCommandMetadata(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tests := []struct {
		command           *cobra.Command
		use               string
		short             string
		outputDefault     string
		outputDescription string
	}{
		{
			command:           newUpdateSpamListsCommand(logger),
			use:               "update-spam-lists",
			short:             "Update spam filter lists",
			outputDescription: "Output path for the compiled spam filter cache",
		},
		{
			command:           newUpdateAIAgentListsCommand(logger),
			use:               "update-ai-agent-lists",
			short:             "Update AI agent lists",
			outputDefault:     "internal/aianalytics/default_ai_agents.json",
			outputDescription: "Output path for the assembled AI agent master list",
		},
	}

	for _, test := range tests {
		if got := test.command.Use; got != test.use {
			t.Errorf("Use = %q, want %q", got, test.use)
		}
		if got := test.command.Short; got != test.short {
			t.Errorf("Short = %q, want %q", got, test.short)
		}
		if !test.command.SilenceUsage {
			t.Errorf("%s SilenceUsage = false, want true", test.use)
		}

		output := test.command.Flags().Lookup("output")
		if output == nil {
			t.Fatalf("%s has no --output flag", test.use)
		}
		if got := output.DefValue; got != test.outputDefault {
			t.Errorf("%s --output default = %q, want %q", test.use, got, test.outputDefault)
		}
		if got := output.Usage; got != test.outputDescription {
			t.Errorf("%s --output usage = %q, want %q", test.use, got, test.outputDescription)
		}
		if got := output.Shorthand; got != "" {
			t.Errorf("%s --output shorthand = %q, want empty", test.use, got)
		}
	}
}

func TestUpdateListCommandsPreserveFlagExitCode(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, command := range []*cobra.Command{
		newUpdateSpamListsCommand(logger),
		newUpdateAIAgentListsCommand(logger),
	} {
		var stdout, stderr bytes.Buffer
		command.SetOut(&stdout)
		command.SetErr(&stderr)
		command.SetArgs([]string{"--unknown"})
		err := command.ExecuteContext(t.Context())
		exitErr, ok := errors.AsType[*ExitError](err)
		if !ok || exitErr.Code != 2 {
			t.Fatalf("error = %v, want ExitError code 2", err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout = %q, want empty", got)
		}
		if !strings.Contains(stderr.String(), "unknown flag: --unknown") {
			t.Errorf("stderr = %q, want Cobra flag error", stderr.String())
		}
	}
}

func TestUpdateListCommandsRetainPositionalCompatibility(t *testing.T) {
	for _, command := range []*cobra.Command{
		newUpdateSpamListsCommand(slog.Default()),
		newUpdateAIAgentListsCommand(slog.Default()),
	} {
		if err := command.Args(command, []string{"legacy-positional"}); err != nil {
			t.Fatalf("%s rejected a legacy positional argument: %v", command.Name(), err)
		}
	}
}
