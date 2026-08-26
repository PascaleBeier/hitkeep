package hitkeepcmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

type rootActions struct {
	run                func([]string, string) error
	recover            func([]string)
	updateSpamLists    func([]string)
	updateAIAgentLists func([]string)
	importData         func([]string)
}

// NewRootCommand routes production commands while their existing parsers remain authoritative.
func NewRootCommand(logger *slog.Logger) *cobra.Command {
	if logger == nil {
		logger = slog.Default()
	}
	return newRootCommand(rootActions{
		run: func(args []string, configFile string) error {
			return run(logger, args, configFile)
		},
		recover: func(_ []string) {
			Recover(logger)
		},
		updateSpamLists: func(args []string) {
			UpdateSpamLists(args, logger)
		},
		updateAIAgentLists: func(args []string) {
			UpdateAIAgentLists(args, logger)
		},
		importData: func(args []string) {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			Import(ctx, args)
		},
	})
}

func newRootCommand(actions rootActions) *cobra.Command {
	root := &cobra.Command{
		Use:                "hitkeep",
		SilenceErrors:      true,
		SilenceUsage:       true,
		DisableFlagParsing: true,
		DisableSuggestions: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return actions.run(args, "")
			}
			switch args[0] {
			case "recover":
				actions.recover(args[1:])
			case "update-spam-lists":
				actions.updateSpamLists(args[1:])
			case "update-ai-agent-lists":
				actions.updateAIAgentLists(args[1:])
			case "import":
				actions.importData(args[1:])
			default:
				configFile, serverArgs, err := splitRootConfig(args)
				if err != nil {
					return err
				}
				return actions.run(serverArgs, configFile)
			}
			return nil
		},
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	return root
}

func splitRootConfig(args []string) (string, []string, error) {
	if len(args) == 0 {
		return "", args, nil
	}
	if args[0] == "--config" {
		if len(args) < 2 || args[1] == "" {
			return "", nil, fmt.Errorf("--config requires a path")
		}
		return args[1], args[2:], nil
	}
	if configFile, ok := strings.CutPrefix(args[0], "--config="); ok {
		if configFile == "" {
			return "", nil, fmt.Errorf("--config requires a path")
		}
		return configFile, args[1:], nil
	}
	return "", args, nil
}
