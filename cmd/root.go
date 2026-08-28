package hitkeepcmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	runtimeconfig "hitkeep/config"
)

// ExitError tells the executable that a command already wrote its user-facing error.
type ExitError struct{ Code int }

func (*ExitError) Error() string { return "command failed" }

type rootConfigFileContextKey struct{}
type rootLegacyServerArgsContextKey struct{}

func rootConfigFile(ctx context.Context) string {
	configFile, _ := ctx.Value(rootConfigFileContextKey{}).(string)
	return configFile
}

// ExecuteRoot applies only the legacy leading --config grammar before Cobra routes a command.
func ExecuteRoot(ctx context.Context, root *cobra.Command, args []string) error {
	configFile, args, err := splitRootConfig(args)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if configFile != "" {
		ctx = context.WithValue(ctx, rootConfigFileContextKey{}, configFile)
	} else if len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "--version" {
		// Cobra scans past unknown flags when finding children. Preserve the legacy
		// first-token grammar by forcing these server arguments to remain at root.
		ctx = context.WithValue(ctx, rootLegacyServerArgsContextKey{}, true)
		args = append([]string{"--"}, args...)
	}
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}

type rootActions struct {
	run                func([]string, string) error
	runContext         func(context.Context, []string, string) error
	recover            func(context.Context, []string, io.Reader, io.Writer, io.Writer) error
	updateSpamLists    func(context.Context, []string, io.Writer, io.Writer, string) error
	updateAIAgentLists func(context.Context, []string, io.Writer, io.Writer, string) error
	importData         func(context.Context, []string, io.Reader, io.Writer, io.Writer, string) error
}

func (actions rootActions) runWithContext(ctx context.Context, args []string, configFile string) error {
	if actions.runContext != nil {
		return actions.runContext(ctx, args, configFile)
	}
	return actions.run(args, configFile)
}

// NewRootCommand routes production commands while their existing parsers remain authoritative.
func NewRootCommand(logger *slog.Logger) *cobra.Command {
	if logger == nil {
		logger = slog.Default()
	}
	actions := rootActions{
		run: func(args []string, configFile string) error {
			return run(logger, args, configFile)
		},
		runContext: func(ctx context.Context, args []string, configFile string) error {
			return runContext(ctx, logger, args, configFile)
		},
		recover: func(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
			return Recover(ctx, args, in, out, errOut, logger)
		},
		updateSpamLists: func(ctx context.Context, args []string, out, errOut io.Writer, configFile string) error {
			return UpdateSpamLists(ctx, args, out, errOut, configFile, logger)
		},
		updateAIAgentLists: func(ctx context.Context, args []string, out, errOut io.Writer, configFile string) error {
			return UpdateAIAgentLists(ctx, args, out, errOut, configFile, logger)
		},
		importData: func(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, configFile string) error {
			return Import(ctx, args, in, out, errOut, configFile, logger)
		},
	}
	root := newRootCommand(actions)
	root.AddCommand(newConfigCommand(afero.NewOsFs(), logger, func(command *cobra.Command, args []string) error {
		return actions.runWithContext(command.Context(), args, rootConfigFile(command.Context()))
	}))
	return root
}

func newImportCommandRoute(run func(context.Context, []string, io.Reader, io.Writer, io.Writer, string) error) *cobra.Command {
	return &cobra.Command{
		Use:                "import",
		Short:              "Import historical analytics data",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(command *cobra.Command, args []string) error {
			return run(command.Context(), args, command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr(), rootConfigFile(command.Context()))
		},
	}
}

func newUpdateSpamListsCommand(run func(context.Context, []string, io.Writer, io.Writer, string) error) *cobra.Command {
	return &cobra.Command{
		Use:                "update-spam-lists",
		Short:              "Update spam filter lists",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(command *cobra.Command, args []string) error {
			return run(command.Context(), args, command.OutOrStdout(), command.ErrOrStderr(), rootConfigFile(command.Context()))
		},
	}
}

func newUpdateAIAgentListsCommand(run func(context.Context, []string, io.Writer, io.Writer, string) error) *cobra.Command {
	return &cobra.Command{
		Use:                "update-ai-agent-lists",
		Short:              "Update AI agent lists",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(command *cobra.Command, args []string) error {
			return run(command.Context(), args, command.OutOrStdout(), command.ErrOrStderr(), rootConfigFile(command.Context()))
		},
	}
}

func newHealthcheckCommand(run func(context.Context, []string, string) error) *cobra.Command {
	return &cobra.Command{
		Use:                "healthcheck",
		Short:              "Check whether HitKeep is healthy",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(command *cobra.Command, args []string) error {
			if err := command.Context().Err(); err != nil {
				return err
			}
			return run(command.Context(), append(args, "--healthcheck"), rootConfigFile(command.Context()))
		},
	}
}

func newConfigCommand(fs afero.Fs, logger *slog.Logger, fallback func(*cobra.Command, []string) error) *cobra.Command {
	command := &cobra.Command{
		Use:                "config",
		Short:              "Create and validate HitKeep configuration files",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(command *cobra.Command, args []string) error {
			return fallback(command, append([]string{"config"}, args...))
		},
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
			closeFile := func() {
				_ = file.Close()
			}
			if _, err := file.Write(runtimeconfig.RenderExampleYAML()); err != nil {
				closeFile()
				return fmt.Errorf("write configuration file %q: %w", outputPath, err)
			}
			if err := file.Close(); err != nil {
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
		RunE: func(command *cobra.Command, args []string) error {
			if escaped, _ := command.Context().Value(rootLegacyServerArgsContextKey{}).(bool); escaped && len(args) > 0 && args[0] == "--" {
				args = args[1:]
			}
			configFile := rootConfigFile(command.Context())
			if len(args) == 0 {
				return actions.runWithContext(command.Context(), args, configFile)
			}
			if args[0] == "recover" {
				return actions.recover(command.Context(), args[1:], command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr())
			}
			if len(args) == 1 && args[0] == "--version" {
				_, err := fmt.Fprintln(command.OutOrStdout(), Version)
				return err
			}
			return actions.runWithContext(command.Context(), args, configFile)
		},
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.AddCommand(
		newHealthcheckCommand(actions.runWithContext),
		newImportCommandRoute(actions.importData),
		newUpdateSpamListsCommand(actions.updateSpamLists),
		newUpdateAIAgentListsCommand(actions.updateAIAgentLists),
	)
	root.SetHelpCommand(&cobra.Command{
		Use:                "help",
		Hidden:             true,
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(command *cobra.Command, args []string) error {
			return actions.run(append([]string{"help"}, args...), rootConfigFile(command.Context()))
		},
	})
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
