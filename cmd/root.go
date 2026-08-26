package hitkeepcmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	runtimeconfig "hitkeep/internal/config"
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
	root := newRootCommand(rootActions{
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
	root.AddCommand(newConfigCommand(afero.NewOsFs(), logger))
	return root
}

func newConfigCommand(fs afero.Fs, logger *slog.Logger) *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: "Create and validate HitKeep configuration files",
		Args:  cobra.NoArgs,
	}
	command.AddCommand(newConfigInitCommand(fs), newConfigValidateCommand(logger))
	return command
}

func newConfigInitCommand(fs afero.Fs) *cobra.Command {
	var outputPath string
	command := &cobra.Command{
		Use:   "init",
		Short: "Write the canonical example configuration",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			file, err := fs.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				return fmt.Errorf("create configuration file %q: %w", outputPath, err)
			}
			removePartial := func() {
				_ = file.Close()
				_ = fs.Remove(outputPath)
			}
			if _, err := file.Write(runtimeconfig.RenderExampleYAML()); err != nil {
				removePartial()
				return fmt.Errorf("write configuration file %q: %w", outputPath, err)
			}
			if err := file.Close(); err != nil {
				_ = fs.Remove(outputPath)
				return fmt.Errorf("close configuration file %q: %w", outputPath, err)
			}
			command.Printf("Created configuration file %s\n", outputPath)
			return nil
		},
	}
	command.Flags().StringVar(&outputPath, "output", "", "configuration file to create")
	_ = command.MarkFlagRequired("output")
	return command
}

func newConfigValidateCommand(logger *slog.Logger) *cobra.Command {
	var configPath string
	command := &cobra.Command{
		Use:   "validate",
		Short: "Validate a configuration file",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if _, err := runtimeconfig.LoadArgs(nil, configPath, logger); err != nil {
				return err
			}
			command.Printf("Configuration file %s is valid\n", configPath)
			return nil
		},
	}
	command.Flags().StringVar(&configPath, "config", "", "configuration file to validate")
	_ = command.MarkFlagRequired("config")
	return command
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
