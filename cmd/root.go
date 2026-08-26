package hitkeepcmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

type rootActions struct {
	run                func([]string)
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
		run: func(_ []string) {
			Run(logger)
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
		Run: func(_ *cobra.Command, args []string) {
			if len(args) == 0 {
				actions.run(args)
				return
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
				actions.run(args)
			}
		},
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	return root
}
