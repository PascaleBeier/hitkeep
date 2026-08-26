package cli

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	runtimeconfig "hitkeep/internal/config"
	"hitkeep/internal/devtool"
	"hitkeep/internal/devtool/devmcp"
	json "hitkeep/jsonapi"
)

type options struct {
	workspace string
	output    string
	version   string
	stdout    io.Writer
	stderr    io.Writer
}

type reportedError struct{ cause error }

func (e reportedError) Error() string { return e.cause.Error() }
func (e reportedError) Unwrap() error { return e.cause }

func IsReported(err error) bool {
	var reported reportedError
	return errors.As(err, &reported)
}

type exitError struct {
	cause error
	code  int
}

func (e exitError) Error() string { return e.cause.Error() }
func (e exitError) Unwrap() error { return e.cause }

func ExitCode(err error) int {
	if coded, ok := errors.AsType[exitError](err); ok {
		return coded.code
	}
	return 1
}

func Execute(ctx context.Context, version string, args []string, stdout, stderr io.Writer) error {
	options := &options{version: version, output: "human", stdout: stdout, stderr: stderr}
	root := newRoot(options)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	return root.ExecuteContext(ctx)
}

func newRoot(options *options) *cobra.Command {
	root := &cobra.Command{
		Use:           "hk",
		Short:         "Reproducible HitKeep development workflows",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       options.version,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return validateOutputFormat(options.output)
		},
	}
	root.PersistentFlags().StringVar(&options.workspace, "workspace", ".", "Git worktree to operate on")
	root.PersistentFlags().StringVar(&options.output, "output", "human", "output format: human (default), plain, json, or ndjson")
	root.AddCommand(
		catalogCommand(options), doctorCommand(options), workspaceCommand(options), setupCommand(options),
		devCommand(options), screenshotCommand(options), qaCommand(options), formatCommand(options), fixCommand(options), buildCommand(options), smokeCommand(options), runCommand(options),
		cacheCommand(options), ciCommand(options), docsCommand(options), skillsCommand(options), mcpCommand(options), runWorkerCommand(options), devWorkerCommand(options),
	)
	return root
}

func cacheCommand(options *options) *cobra.Command {
	command := &cobra.Command{Use: "cache", Short: "Inspect and safely prune hk-managed shared caches"}
	command.AddCommand(&cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: withApp(options, "cache status", func(_ context.Context, app *devtool.App) (any, error) {
		return app.CacheStatus()
	})})
	var apply bool
	var olderThan time.Duration
	prune := &cobra.Command{Use: "prune", Short: "Preview or remove unused managed cache entries", Args: cobra.NoArgs, RunE: withApp(options, "cache prune", func(_ context.Context, app *devtool.App) (any, error) {
		return app.PruneCache(olderThan, apply)
	})}
	prune.Flags().BoolVar(&apply, "apply", false, "remove listed entries (default is dry run)")
	prune.Flags().DurationVar(&olderThan, "older-than", 30*24*time.Hour, "minimum unused age")
	command.AddCommand(prune)
	return command
}

func formatCommand(options *options) *cobra.Command {
	var scope string
	command := sourceChangeCommand(options, "fmt [check|write]", "fmt", "Format repository sources", "write", func(ctx context.Context, app *devtool.App, write bool) (any, error) {
		switch scope {
		case "go":
			return app.FormatGo(write)
		case "frontend":
			return app.FormatFrontend(ctx, write)
		default:
			return nil, errors.New("scope must be go or frontend")
		}
	})
	command.Flags().StringVar(&scope, "scope", "go", "source scope: go or frontend")
	return command
}

func fixCommand(options *options) *cobra.Command {
	return sourceChangeCommand(options, "fix [check|apply]", "fix", "Apply pinned Go source migrations", "apply", func(ctx context.Context, app *devtool.App, apply bool) (any, error) {
		return app.FixGo(ctx, apply)
	})
}

func sourceChangeCommand(options *options, use, command, short, writeMode string, handler func(context.Context, *devtool.App, bool) (any, error)) *cobra.Command {
	return &cobra.Command{
		Use: use, Short: short, Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := writeMode
			if len(args) > 0 {
				mode = args[0]
			}
			valid := mode == "check" || mode == writeMode
			if !valid {
				return fmt.Errorf("mode must be check or %s", writeMode)
			}
			return withApp(options, command+" "+mode, func(ctx context.Context, app *devtool.App) (any, error) {
				return handler(ctx, app, mode == writeMode)
			})(cmd, args)
		},
	}
}

func withApp(options *options, command string, handler func(context.Context, *devtool.App) (any, error)) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		app, err := devtool.NewApp(options.workspace)
		if err != nil {
			if renderErr := render(options, devtool.ErrorEnvelope(command, "", err)); renderErr != nil {
				return renderErr
			}
			return reportedError{cause: err}
		}
		ctx := cmd.Context()
		if options.output == "json" || options.output == "ndjson" {
			ctx = devtool.WithAgentOutput(ctx)
		}
		data, runErr := handler(ctx, app)
		if runErr != nil {
			if renderErr := render(options, devtool.ErrorEnvelope(command, app.WorkspaceID(), runErr)); renderErr != nil {
				return renderErr
			}
			return reportedError{cause: runErr}
		}
		return render(options, devtool.SuccessEnvelope(command, app.WorkspaceID(), data))
	}
}

func catalogCommand(options *options) *cobra.Command {
	command := &cobra.Command{Use: "catalog", Short: "Show canonical variants, QA profiles, and gates", Args: cobra.NoArgs, RunE: withApp(options, "catalog", func(_ context.Context, app *devtool.App) (any, error) { return app.Catalog(), nil })}
	commands := &cobra.Command{Use: "commands", Short: "List the complete machine-readable hk command surface", Args: cobra.NoArgs}
	commands.RunE = withApp(options, "catalog commands", func(_ context.Context, _ *devtool.App) (any, error) {
		return buildCommandCatalog(commands.Root()), nil
	})
	configuration := &cobra.Command{Use: "configuration", Aliases: []string{"config"}, Short: "Export the runtime configuration documentation contract", Args: cobra.NoArgs}
	configuration.RunE = withApp(options, "catalog configuration", func(_ context.Context, _ *devtool.App) (any, error) {
		return runtimeconfig.Catalog(), nil
	})
	command.AddCommand(commands, configuration)
	return command
}

type commandCatalog struct {
	SchemaVersion string               `json:"schema_version"`
	OutputFormats []string             `json:"output_formats"`
	GlobalFlags   []commandCatalogFlag `json:"global_flags"`
	Commands      []commandCatalogItem `json:"commands"`
}

type commandCatalogItem struct {
	Path             string               `json:"path"`
	Use              string               `json:"use"`
	Description      string               `json:"description"`
	Flags            []commandCatalogFlag `json:"flags,omitempty"`
	StructuredOutput bool                 `json:"structured_output"`
}

type commandCatalogFlag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
	Usage     string `json:"usage"`
}

func buildCommandCatalog(root *cobra.Command) commandCatalog {
	catalog := commandCatalog{
		SchemaVersion: devtool.SchemaVersion,
		OutputFormats: []string{"human", "plain", "json", "ndjson"},
		GlobalFlags:   commandFlags(root.PersistentFlags()),
	}
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		if command.Hidden {
			return
		}
		if command != root && (command.Run != nil || command.RunE != nil) {
			catalog.Commands = append(catalog.Commands, commandCatalogItem{
				Path:             command.CommandPath(),
				Use:              command.Use,
				Description:      command.Short,
				Flags:            commandFlags(command.NonInheritedFlags()),
				StructuredOutput: true,
			})
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
	slices.SortFunc(catalog.Commands, func(left, right commandCatalogItem) int {
		return cmp.Compare(left.Path, right.Path)
	})
	return catalog
}

func commandFlags(flags *pflag.FlagSet) []commandCatalogFlag {
	var result []commandCatalogFlag
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Name == "help" {
			return
		}
		result = append(result, commandCatalogFlag{
			Name: flag.Name, Shorthand: flag.Shorthand, Type: flag.Value.Type(), Default: flag.DefValue, Usage: flag.Usage,
		})
	})
	slices.SortFunc(result, func(left, right commandCatalogFlag) int {
		return cmp.Compare(left.Name, right.Name)
	})
	return result
}

func doctorCommand(options *options) *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Diagnose required developer tools", Args: cobra.NoArgs, RunE: withApp(options, "doctor", func(ctx context.Context, app *devtool.App) (any, error) { return app.Doctor(ctx), nil })}
}

func workspaceCommand(options *options) *cobra.Command {
	command := &cobra.Command{Use: "workspace", Short: "Inspect isolated worktree state"}
	command.AddCommand(
		&cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: withApp(options, "workspace status", func(ctx context.Context, app *devtool.App) (any, error) { return app.Workspace(ctx) })},
		&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: withApp(options, "workspace list", func(ctx context.Context, app *devtool.App) (any, error) { return app.Workspaces(ctx) })},
		&cobra.Command{Use: "handoff", Args: cobra.NoArgs, RunE: withApp(options, "workspace handoff", func(ctx context.Context, app *devtool.App) (any, error) { return app.Handoff(ctx) })},
	)
	return command
}

func setupCommand(options *options) *cobra.Command {
	return startCommand(options, "setup", "setup", "Prepare pinned development containers for this worktree", func(*cobra.Command, []string) devtool.RunRequest { return devtool.RunRequest{Kind: "setup"} })
}

func screenshotCommand(options *options) *cobra.Command {
	var request devtool.ScreenshotRequest
	command := &cobra.Command{
		Use:   "screenshot [ROUTE...]",
		Short: "Capture local dashboard routes for visual QA",
		Args:  cobra.MaximumNArgs(devtool.MaxScreenshotRoutes),
		RunE: withArgsApp(options, "screenshot", func(ctx context.Context, app *devtool.App, args []string) (any, error) {
			request.Routes = args
			return app.CaptureScreenshots(ctx, request)
		}),
	}
	command.Flags().StringVar(&request.Viewport, "viewport", "desktop", "viewport preset: desktop or mobile")
	command.Flags().StringVar(&request.Theme, "theme", "light", "color scheme: light or dark")
	command.Flags().IntVar(&request.Scale, "scale", 1, "device pixel ratio: 1 or 2")
	command.Flags().IntVar(&request.WaitMS, "wait-ms", 200, "bounded visual settle time after route readiness")
	command.Flags().BoolVar(&request.FullPage, "full-page", false, "capture the complete document instead of the viewport")
	command.Flags().StringVar(&request.Selector, "selector", "", "capture one visible CSS selector on a single route")
	command.Flags().BoolVar(&request.Anonymous, "anonymous", false, "capture without signing in to the seeded development account")
	return command
}

func devCommand(options *options) *cobra.Command {
	var variant string
	var seed bool
	var detach bool
	command := &cobra.Command{
		Use: "dev", Short: "Run a workspace-scoped HitKeep development session", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDevStart(cmd, options, "dev", devtool.DevRequest{Variant: variant, Seed: seed}, detach)
		},
	}
	command.Flags().StringVar(&variant, "variant", "self-hosted", "self-hosted or cloud")
	command.Flags().BoolVar(&seed, "seed", false, "seed isolated demo data before starting")
	command.Flags().BoolVar(&detach, "detach", false, "run in the background and wait until ready")
	command.AddCommand(
		&cobra.Command{Use: "status", Short: "Inspect the current development session", Args: cobra.NoArgs, RunE: withApp(options, "dev status", func(ctx context.Context, app *devtool.App) (any, error) {
			return app.DevStatus(ctx)
		})},
		devStopCommand(options),
		devRestartCommand(options),
		devResetCommand(options),
		devLogsCommand(options),
	)
	return command
}

func devRestartCommand(options *options) *cobra.Command {
	var variant string
	var detach bool
	command := &cobra.Command{
		Use: "restart", Short: "Restart development without deleting workspace data", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := devtool.NewApp(options.workspace)
			if err != nil {
				return renderDevError(options, "dev restart", "", err)
			}
			status, err := app.DevStatus(cmd.Context())
			if err != nil {
				return renderDevError(options, "dev restart", app.WorkspaceID(), err)
			}
			if variant == "" {
				variant = status.Variant
				if variant == "" {
					variant = "self-hosted"
				}
			}
			if _, err := app.StopDev(cmd.Context()); err != nil {
				return renderDevError(options, "dev restart", app.WorkspaceID(), err)
			}
			return runDevStartWithApp(cmd, options, app, "dev restart", devtool.DevRequest{Variant: variant}, detach)
		},
	}
	command.Flags().StringVar(&variant, "variant", "", "variant override; defaults to the current session")
	command.Flags().BoolVar(&detach, "detach", false, "run in the background and wait until ready")
	return command
}

func devStopCommand(options *options) *cobra.Command {
	return &cobra.Command{Use: "stop", Short: "Stop the selected workspace's development session", Args: cobra.NoArgs, RunE: withApp(options, "dev stop", func(ctx context.Context, app *devtool.App) (any, error) {
		return app.StopDev(ctx)
	})}
}

func devResetCommand(options *options) *cobra.Command {
	var variant string
	var seed bool
	var detach bool
	command := &cobra.Command{
		Use: "reset", Short: "Delete isolated development data and start a fresh session", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if options.output == "json" && !detach {
				return renderDevError(options, "dev reset", "", errors.New("--output json requires --detach for development sessions"))
			}
			app, err := devtool.NewApp(options.workspace)
			if err != nil {
				return renderDevError(options, "dev reset", "", err)
			}
			if _, err := app.StopDev(cmd.Context()); err != nil {
				return renderDevError(options, "dev reset", app.WorkspaceID(), err)
			}
			if err := app.ResetDevData(cmd.Context()); err != nil {
				return renderDevError(options, "dev reset", app.WorkspaceID(), err)
			}
			return runDevStartWithApp(cmd, options, app, "dev reset", devtool.DevRequest{Variant: variant, Seed: seed}, detach)
		},
	}
	command.Flags().StringVar(&variant, "variant", "self-hosted", "self-hosted or cloud")
	command.Flags().BoolVar(&seed, "seed", false, "seed isolated demo data after reset")
	command.Flags().BoolVar(&detach, "detach", false, "run the fresh session in the background and wait until ready")
	return command
}

func devLogsCommand(options *options) *cobra.Command {
	var limit int
	var cursor int64
	var follow bool
	logs := &cobra.Command{Use: "logs", Short: "Read or follow the current development events", Args: cobra.NoArgs}
	logs.RunE = func(cmd *cobra.Command, _ []string) error {
		app, err := devtool.NewApp(options.workspace)
		if err != nil {
			return renderDevError(options, "dev logs", "", err)
		}
		followLogs := options.output == "human"
		if cmd.Flags().Changed("follow") {
			followLogs = follow
		}
		if followLogs && options.output == "json" {
			return renderDevError(options, "dev logs", app.WorkspaceID(), errors.New("--follow requires --output ndjson, plain, or human"))
		}
		batch, err := app.DevLogs(cursor, limit)
		if err != nil {
			return renderDevError(options, "dev logs", app.WorkspaceID(), err)
		}
		if !followLogs {
			return render(options, devtool.SuccessEnvelope("dev logs", app.WorkspaceID(), batch))
		}
		for _, event := range batch.Events {
			renderDevStreamEvent(options, app.WorkspaceID(), event)
		}
		final, followErr := app.FollowDevEvents(cmd.Context(), batch.NextCursor, limit, func(event devtool.DevEvent) {
			renderDevStreamEvent(options, app.WorkspaceID(), event)
		})
		if followErr != nil && errors.Is(followErr, context.Canceled) {
			return nil
		}
		if followErr != nil {
			return renderDevError(options, "dev logs", app.WorkspaceID(), followErr)
		}
		return render(options, devtool.SuccessEnvelope("dev logs", app.WorkspaceID(), final))
	}
	logs.Flags().IntVar(&limit, "limit", 40, "maximum recent events (up to 200)")
	logs.Flags().Int64Var(&cursor, "cursor", 0, "next event cursor for incremental reads")
	logs.Flags().BoolVar(&follow, "follow", true, "continue streaming new events")
	return logs
}

func runDevStart(cmd *cobra.Command, options *options, command string, request devtool.DevRequest, detach bool) error {
	app, err := devtool.NewApp(options.workspace)
	if err != nil {
		return renderDevError(options, command, "", err)
	}
	return runDevStartWithApp(cmd, options, app, command, request, detach)
}

func runDevStartWithApp(cmd *cobra.Command, options *options, app *devtool.App, command string, request devtool.DevRequest, detach bool) error {
	if options.output == "json" && !detach {
		return renderDevError(options, command, app.WorkspaceID(), errors.New("--output json requires --detach for development sessions"))
	}
	var result devtool.DevStartResult
	var err error
	ctx := cmd.Context()
	if options.output == "json" || options.output == "ndjson" {
		ctx = devtool.WithAgentOutput(ctx)
	}
	if detach {
		result, err = app.StartDevDetached(ctx, request)
	} else {
		result, err = app.StartDevForeground(ctx, request, func(event devtool.DevEvent) {
			renderDevStreamEvent(options, app.WorkspaceID(), event)
		})
	}
	if err != nil && errors.Is(err, context.Canceled) && !detach {
		_ = render(options, devtool.SuccessEnvelope(command, app.WorkspaceID(), result))
		return reportedError{cause: exitError{cause: err, code: 130}}
	}
	if err != nil {
		return renderDevErrorWithData(options, command, app.WorkspaceID(), err, result)
	}
	return render(options, devtool.SuccessEnvelope(command, app.WorkspaceID(), result))
}

func renderDevStreamEvent(options *options, workspaceID string, event devtool.DevEvent) {
	switch options.output {
	case "ndjson":
		_ = render(options, devtool.SuccessEnvelope("dev event", workspaceID, event))
	case "human":
		renderHumanDevEvent(options.stdout, humanStyle{color: colorEnabled(options.stdout)}, event)
	default:
		_, _ = fmt.Fprintln(options.stdout, plainDevEvent(event))
	}
}

func renderDevError(options *options, command, workspaceID string, err error) error {
	return renderDevErrorWithData(options, command, workspaceID, err, nil)
}

func renderDevErrorWithData(options *options, command, workspaceID string, err error, data any) error {
	wrapped := err
	if data != nil {
		wrapped = devtool.WithErrorData(err, data)
	}
	if renderErr := render(options, devtool.ErrorEnvelope(command, workspaceID, wrapped)); renderErr != nil {
		return renderErr
	}
	return reportedError{cause: err}
}

func attachRunLog(ctx context.Context, options *options, app *devtool.App, runID string, limit, cursor int) (any, error) {
	tail, err := app.TailLogAfter(runID, limit, cursor)
	if err != nil || options.output != "human" {
		return tail, err
	}
	live := newLiveRunOutput(options.stdout, colorEnabled(options.stdout))
	for _, line := range tail.Lines {
		live.LogLine(line)
	}
	if tail.Complete {
		return humanFollowedLog{RunID: runID}, nil
	}
	live.Follow(runID)
	if err := followRunLog(ctx, runID, app.GetRun, tail.NextCursor, live.LogLine); err != nil {
		if ctx.Err() != nil {
			return humanFollowedLog{RunID: runID, Detached: true}, nil
		}
		return nil, err
	}
	return humanFollowedLog{RunID: runID}, nil
}

func qaCommand(options *options) *cobra.Command {
	request := func(cmd *cobra.Command, args []string) devtool.RunRequest {
		profile := "complete"
		if len(args) > 0 {
			profile = args[0]
		}
		gates, _ := cmd.Flags().GetStringSlice("gate")
		planID, _ := cmd.Flags().GetString("plan-id")
		return devtool.RunRequest{Kind: "qa", Profile: profile, PlanID: planID, GateIDs: gates}
	}
	configure := func(cmd *cobra.Command) {
		cmd.Args = cobra.MaximumNArgs(1)
		cmd.Flags().StringSlice("gate", nil, "run only canonical gate IDs")
		cmd.Flags().String("plan-id", "", "required source-bound QA plan identifier")
	}
	command := startCommand(options, "qa [changed|complete|pr|full]", "qa start", "Run one persisted source-bound QA plan", request, configure)
	var baseRef string
	plan := &cobra.Command{Use: "plan [changed|complete|pr|full]", Args: cobra.MaximumNArgs(1)}
	plan.RunE = func(cmd *cobra.Command, args []string) error {
		ctx := context.WithValue(cmd.Context(), argsKey{}, args)
		cmd.SetContext(ctx)
		return withApp(options, "qa plan", func(ctx context.Context, app *devtool.App) (any, error) {
			profile := "complete"
			if values := planArgs(ctx); len(values) > 0 {
				profile = values[0]
			}
			return app.QAPlan(ctx, profile, baseRef)
		})(cmd, args)
	}
	plan.Flags().StringVar(&baseRef, "base", "", "base ref for change-aware planning")
	command.AddCommand(plan)
	command.AddCommand(startCommand(options, "start [changed|complete|pr|full]", "qa start", "Run one persisted source-bound QA plan", request, configure))
	return command
}

type argsKey struct{}

func planArgs(ctx context.Context) []string {
	values, _ := ctx.Value(argsKey{}).([]string)
	return values
}

func buildCommand(options *options) *cobra.Command {
	return startCommand(options, "build [binary|image]", "build", "Build a binary or local image", func(cmd *cobra.Command, args []string) devtool.RunRequest {
		variant, _ := cmd.Flags().GetString("variant")
		target := "binary"
		if len(args) > 0 {
			target = args[0]
		}
		return devtool.RunRequest{Kind: "build", Variant: variant, Target: target}
	}, func(cmd *cobra.Command) {
		cmd.Flags().String("variant", "self-hosted", "self-hosted or cloud")
		cmd.Args = cobra.MaximumNArgs(1)
	})
}

func smokeCommand(options *options) *cobra.Command {
	return startCommand(options, "smoke", "smoke", "Build and smoke-test a local production image", variantRequest("smoke"))
}

func runCommand(options *options) *cobra.Command {
	command := &cobra.Command{Use: "run", Short: "Observe and control asynchronous runs"}
	var listLimit int
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: withApp(options, "run list", func(_ context.Context, app *devtool.App) (any, error) {
		return app.RecentRuns(listLimit)
	})}
	list.Flags().IntVar(&listLimit, "limit", 20, "maximum recent runs (up to 100)")
	command.AddCommand(
		list,
		&cobra.Command{Use: "status RUN_ID", Args: cobra.ExactArgs(1), RunE: withArgsApp(options, "run status", func(_ context.Context, app *devtool.App, args []string) (any, error) { return app.GetRun(args[0]) })},
		&cobra.Command{Use: "cancel RUN_ID", Args: cobra.ExactArgs(1), RunE: withArgsApp(options, "run cancel", func(_ context.Context, app *devtool.App, args []string) (any, error) { return app.CancelRun(args[0]) })},
	)
	var limit int
	var cursor int
	var gateID string
	logs := &cobra.Command{Use: "logs RUN_ID", Args: cobra.ExactArgs(1)}
	logs.RunE = func(cmd *cobra.Command, args []string) error {
		return withApp(options, "run logs", func(ctx context.Context, app *devtool.App) (any, error) {
			if gateID != "" {
				return app.TailGateLogAfter(args[0], gateID, limit, cursor)
			}
			return attachRunLog(ctx, options, app, args[0], limit, cursor)
		})(cmd, args)
	}
	logs.Flags().IntVar(&limit, "limit", 40, "maximum log lines (up to 200)")
	logs.Flags().IntVar(&cursor, "cursor", 0, "previous next_cursor for incremental reads")
	logs.Flags().StringVar(&gateID, "gate", "", "canonical gate ID for gate-specific logs")
	command.AddCommand(logs)
	return command
}

func skillsCommand(options *options) *cobra.Command {
	command := &cobra.Command{Use: "skills", Short: "Validate product and contributor skill packs"}
	command.AddCommand(
		&cobra.Command{Use: "check", Args: cobra.NoArgs, RunE: withApp(options, "skills check", func(_ context.Context, app *devtool.App) (any, error) {
			return map[string]bool{"current": true}, devtool.ValidateSkillLayout(app.Root())
		})},
	)
	return command
}

func docsCommand(options *options) *cobra.Command {
	command := &cobra.Command{Use: "docs", Short: "Validate development documentation against hk"}
	command.AddCommand(&cobra.Command{Use: "check", Args: cobra.NoArgs, RunE: withApp(options, "docs check", func(_ context.Context, app *devtool.App) (any, error) {
		return map[string]bool{"current": true}, devtool.ValidateDevelopmentDocs(app.Root())
	})})
	return command
}

func ciCommand(options *options) *cobra.Command {
	command := &cobra.Command{Use: "ci", Short: "Deterministic CI build operations without publication credentials"}
	var matrixProfile string
	var matrixPrefix string
	var matrixGroup string
	matrix := &cobra.Command{Use: "qa-matrix", Short: "Derive stable CI gate IDs from the canonical catalog", Args: cobra.NoArgs, RunE: withApp(options, "ci qa-matrix", func(_ context.Context, app *devtool.App) (any, error) {
		return app.CIQAMatrix(matrixProfile, matrixPrefix, matrixGroup)
	})}
	matrix.Flags().StringVar(&matrixProfile, "profile", "pr", "canonical profile: pr or full")
	matrix.Flags().StringVar(&matrixPrefix, "prefix", "", "optional stable gate ID prefix")
	matrix.Flags().StringVar(&matrixGroup, "group", "", "optional stable CI execution group")
	var configVariant string
	var extraTags []string
	goConfig := &cobra.Command{Use: "go-config [tags|csv|goflags|golangci]", Short: "Read canonical Go build configuration", Args: cobra.MaximumNArgs(1), RunE: withArgsApp(options, "ci go-config", func(_ context.Context, app *devtool.App, args []string) (any, error) {
		config, err := app.GoBuildConfig(configVariant, extraTags)
		if err != nil || len(args) == 0 {
			return config, err
		}
		switch args[0] {
		case "tags":
			return strings.Join(config.Tags, " "), nil
		case "csv":
			return config.TagsCSV, nil
		case "goflags":
			return config.GOFLAGS, nil
		case "golangci":
			return config.GolangCIArgs, nil
		default:
			return nil, errors.New("field must be tags, csv, goflags, or golangci")
		}
	})}
	goConfig.Flags().StringVar(&configVariant, "variant", "self-hosted", "self-hosted or cloud")
	goConfig.Flags().StringSliceVar(&extraTags, "extra-tag", nil, "additional validated build tag")
	toolchain := &cobra.Command{Use: "toolchain [go|node|npm]", Short: "Read exact repository toolchain versions", Args: cobra.MaximumNArgs(1), RunE: withArgsApp(options, "ci toolchain", func(_ context.Context, app *devtool.App, args []string) (any, error) {
		config, err := app.ToolchainConfig()
		if err != nil || len(args) == 0 {
			return config, err
		}
		switch args[0] {
		case "go":
			return config.Go, nil
		case "node":
			return config.Node, nil
		case "npm":
			return config.NPM, nil
		default:
			return nil, errors.New("tool must be go, node, or npm")
		}
	})}
	var request devtool.ReleaseBuildRequest
	build := &cobra.Command{Use: "build-binaries", Args: cobra.NoArgs, RunE: withApp(options, "ci build-binaries", func(ctx context.Context, app *devtool.App) (any, error) {
		return app.BuildReleaseBinaries(ctx, request, options.stderr)
	})}
	build.Flags().StringVar(&request.Version, "version", "", "release version embedded in both binaries")
	build.Flags().StringVar(&request.GOOS, "goos", "linux", "release operating system")
	build.Flags().StringVar(&request.GOARCH, "goarch", "", "release architecture: amd64 or arm64")
	_ = build.MarkFlagRequired("version")
	_ = build.MarkFlagRequired("goarch")
	var shard string
	race := &cobra.Command{Use: "race", Short: "Run one canonical Go race shard", Args: cobra.NoArgs, RunE: withApp(options, "ci race", func(ctx context.Context, app *devtool.App) (any, error) {
		return app.RunRaceShard(ctx, shard, options.stderr)
	})}
	race.Flags().StringVar(&shard, "shard", "", "canonical shard: database, server, or rest")
	_ = race.MarkFlagRequired("shard")
	cloudTest := &cobra.Command{Use: "cloud-test", Short: "Test cloud-tagged packages outside the developer platform", Args: cobra.NoArgs, RunE: withApp(options, "ci cloud-test", func(ctx context.Context, app *devtool.App) (any, error) {
		return app.RunCloudTests(ctx, options.stderr)
	})}
	var verifyBuildVariant string
	verifyBuild := &cobra.Command{Use: "verify-build", Short: "Compile one canonical variant into temporary workspace state", Args: cobra.NoArgs, RunE: withApp(options, "ci verify-build", func(ctx context.Context, app *devtool.App) (any, error) {
		err := app.VerifyVariantBuild(ctx, verifyBuildVariant, options.stderr)
		return map[string]any{"variant": verifyBuildVariant, "current": err == nil}, err
	})}
	verifyBuild.Flags().StringVar(&verifyBuildVariant, "variant", "self-hosted", "self-hosted or cloud")
	var dashboardArchive string
	restoreDashboard := &cobra.Command{Use: "restore-dashboard", Short: "Restore a validated dashboard archive into the Go embed tree", Args: cobra.NoArgs, RunE: withApp(options, "ci restore-dashboard", func(_ context.Context, app *devtool.App) (any, error) {
		return app.RestoreDashboardArchive(dashboardArchive)
	})}
	restoreDashboard.Flags().StringVar(&dashboardArchive, "archive", "public-assets.tar.gz", "workspace-confined dashboard archive")
	command.AddCommand(
		matrix,
		goConfig,
		toolchain,
		build,
		race,
		cloudTest,
		verifyBuild,
		&cobra.Command{Use: "build-dashboard", Short: "Build a deterministic dashboard asset archive", Args: cobra.NoArgs, RunE: withApp(options, "ci build-dashboard", func(ctx context.Context, app *devtool.App) (any, error) {
			return app.BuildDashboardArchive(ctx, options.stderr)
		})},
		restoreDashboard,
		&cobra.Command{Use: "prepare-public-image", Short: "Relocate cloud binaries and verify the public image context", Args: cobra.NoArgs, RunE: withApp(options, "ci prepare-public-image", func(_ context.Context, app *devtool.App) (any, error) { return app.PreparePublicImageContext() })},
		&cobra.Command{Use: "release-checksums", Short: "Generate deterministic checksums for the release artifact contract", Args: cobra.NoArgs, RunE: withApp(options, "ci release-checksums", func(_ context.Context, app *devtool.App) (any, error) { return app.GenerateReleaseChecksums() })},
		&cobra.Command{Use: "verify-release", Short: "Verify release artifacts and checksums", Args: cobra.NoArgs, RunE: withApp(options, "ci verify-release", func(_ context.Context, app *devtool.App) (any, error) { return app.VerifyReleaseArtifacts() })},
	)
	return command
}

func mcpCommand(options *options) *cobra.Command {
	command := &cobra.Command{Use: "mcp", Short: "Expose hk over local Model Context Protocol"}
	var fallbackWorkspace string
	serve := &cobra.Command{Use: "serve", Short: "Serve stateless MCP over stdio", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if cmd.Root().PersistentFlags().Changed("workspace") {
			app, err := devtool.NewApp(options.workspace)
			if err != nil {
				return err
			}
			return devmcp.RunStdio(cmd.Context(), app, options.version)
		}
		fallback := fallbackWorkspace
		if fallback == "" {
			fallback, _ = os.Getwd()
		}
		return devmcp.RunCentralStdio(cmd.Context(), fallback, options.version)
	}}
	serve.Flags().StringVar(&fallbackWorkspace, "fallback-workspace", "", "Fallback HitKeep worktree used when a workspace selector is omitted")
	command.AddCommand(
		&cobra.Command{Use: "manifest", Short: "Emit the central client registration", Args: cobra.NoArgs, RunE: withApp(options, "mcp manifest", func(_ context.Context, app *devtool.App) (any, error) {
			return app.MCPManifest()
		})},
		serve,
	)
	return command
}

func runWorkerCommand(options *options) *cobra.Command {
	var runID string
	command := &cobra.Command{Use: "__run", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if os.Getenv("HK_CHILD_RUN") != "1" {
			return errors.New("run worker is internal")
		}
		app, err := devtool.NewApp(options.workspace)
		if err != nil {
			return err
		}
		return app.ExecuteRun(cmd.Context(), runID)
	}}
	command.Flags().StringVar(&runID, "run-id", "", "internal run identifier")
	_ = command.MarkFlagRequired("run-id")
	return command
}

func devWorkerCommand(options *options) *cobra.Command {
	var generationID string
	var variant string
	var seed bool
	command := &cobra.Command{Use: "__dev", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if os.Getenv("HK_CHILD_DEV") != "1" {
			return errors.New("development worker is internal")
		}
		app, err := devtool.NewApp(options.workspace)
		if err != nil {
			return err
		}
		return app.ExecuteDevSession(cmd.Context(), generationID, devtool.DevRequest{Variant: variant, Seed: seed})
	}}
	command.Flags().StringVar(&generationID, "generation-id", "", "internal development generation")
	command.Flags().StringVar(&variant, "variant", "self-hosted", "internal development variant")
	command.Flags().BoolVar(&seed, "seed", false, "internal development seed request")
	_ = command.MarkFlagRequired("generation-id")
	return command
}

func variantRequest(kind string) func(*cobra.Command, []string) devtool.RunRequest {
	return func(cmd *cobra.Command, _ []string) devtool.RunRequest {
		variant, _ := cmd.Flags().GetString("variant")
		return devtool.RunRequest{Kind: kind, Variant: variant}
	}
}

type startConfig func(*cobra.Command)

func startCommand(options *options, use, command, short string, request func(*cobra.Command, []string) devtool.RunRequest, configs ...startConfig) *cobra.Command {
	var detach bool
	cmd := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs}
	cmd.Flags().BoolVar(&detach, "detach", false, "return immediately with a run ID")
	if strings.Contains(command, "smoke") {
		cmd.Flags().String("variant", "self-hosted", "self-hosted or cloud")
	}
	for _, configure := range configs {
		configure(cmd)
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return withApp(options, command, func(ctx context.Context, app *devtool.App) (any, error) {
			runRequest := request(cmd, args)
			start, err := app.StartRun(ctx, runRequest)
			if err != nil || detach {
				return start, err
			}
			if options.output == "human" {
				live := newLiveRunOutput(options.stderr, colorEnabled(options.stderr))
				workspace, _ := app.Workspace(ctx)
				live.Start(start.RunID, runRequest, workspace, start.Reused)
				skipLines := 0
				if start.Reused {
					tail, tailErr := app.TailLogAfter(start.RunID, 40, 0)
					if tailErr == nil {
						for _, line := range tail.Lines {
							live.LogLine(line)
						}
						skipLines = tail.NextCursor
					}
				}
				streamDone := make(chan error, 1)
				go func() {
					streamDone <- followRunLog(ctx, start.RunID, app.GetRun, skipLines, live.LogLine)
				}()
				run, waitErr := app.WaitRunObserved(ctx, start.RunID, live.Observe)
				streamErr := <-streamDone
				if waitErr != nil && ctx.Err() != nil && !terminalRunStatus(run.Status) {
					return humanDetachedRun{Run: run}, nil
				}
				if waitErr != nil {
					return run, devtool.WithErrorData(waitErr, run)
				}
				if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
					return run, devtool.WithErrorData(streamErr, run)
				}
				return run, nil
			}
			observer := func(run devtool.Run) {
				switch options.output {
				case "ndjson":
					_ = render(options, devtool.SuccessEnvelope("run progress", app.WorkspaceID(), map[string]any{"run_id": run.ID, "status": run.Status, "gates": run.GateResults}))
				}
			}
			run, waitErr := app.WaitRunObserved(ctx, start.RunID, observer)
			if waitErr != nil {
				return run, devtool.WithErrorData(waitErr, run)
			}
			return run, nil
		})(cmd, args)
	}
	return cmd
}

func withArgsApp(options *options, command string, handler func(context.Context, *devtool.App, []string) (any, error)) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return withApp(options, command, func(ctx context.Context, app *devtool.App) (any, error) { return handler(ctx, app, args) })(cmd, args)
	}
}

func render(options *options, envelope devtool.Envelope) error {
	switch options.output {
	case "json":
		return json.MarshalEncode(jsontext.NewEncoder(options.stdout), envelope, jsontext.EscapeForHTML(false))
	case "ndjson":
		return json.MarshalEncode(jsontext.NewEncoder(options.stdout), envelope)
	case "plain":
		if envelope.Status == "error" {
			if envelope.Data != nil {
				if err := renderPlain(options.stdout, envelope); err != nil {
					return err
				}
			}
			_, _ = fmt.Fprintln(options.stderr, "error:", envelope.Error)
			return nil
		}
		return renderPlain(options.stdout, envelope)
	case "human":
		if envelope.Status == "error" {
			if envelope.Data != nil {
				if err := renderHuman(options.stdout, envelope, colorEnabled(options.stdout)); err != nil {
					return err
				}
			}
			renderHumanError(options.stderr, envelope.Error, colorEnabled(options.stderr))
			return nil
		}
		return renderHuman(options.stdout, envelope, colorEnabled(options.stdout))
	default:
		return fmt.Errorf("unknown output format %q", options.output)
	}
}

func renderPlain(writer io.Writer, envelope devtool.Envelope) error {
	switch value := envelope.Data.(type) {
	case devtool.Workspace:
		_, _ = fmt.Fprintf(writer, "%s  %s\nweb %s  api %s\n", value.ID, value.Root, value.URLs.Web, value.URLs.API)
		for _, service := range value.Services {
			state := "stopped"
			if service.Reachable {
				state = "reachable"
			}
			_, _ = fmt.Fprintf(writer, "%-10s %-9s %s\n", service.Name, state, service.Address)
		}
	case []devtool.Workspace:
		for _, workspace := range value {
			branch := workspace.Branch
			if branch == "" {
				branch = "detached"
			}
			_, _ = fmt.Fprintf(writer, "%s  %-18s %s\n", workspace.ID, branch, workspace.Root)
		}
	case devtool.Handoff:
		_, _ = fmt.Fprintf(writer, "%s  %s\n", value.Workspace.ID, value.Workspace.Root)
		_, _ = fmt.Fprintf(writer, "changes %d  recent runs %d\n", value.Workspace.DirtyCount, len(value.RecentRuns))
		for _, path := range value.Workspace.ChangedPaths {
			_, _ = fmt.Fprintln(writer, path)
		}
		if value.Truncated {
			_, _ = fmt.Fprintln(writer, "[additional paths omitted]")
		}
	case devtool.RunStart:
		_, _ = fmt.Fprintf(writer, "%s  %s\n", value.RunID, value.Status)
	case devtool.DevStatus:
		renderPlainDevStatus(writer, value)
	case devtool.DevStartResult:
		renderPlainDevStatus(writer, value.Status)
	case devtool.DevLogBatch:
		for _, event := range value.Events {
			_, _ = fmt.Fprintln(writer, plainDevEvent(event))
		}
	case devtool.Run:
		_, _ = fmt.Fprintf(writer, "%s  %s  %s\n", value.ID, value.Status, value.Request.Kind)
		for _, gate := range value.GateResults {
			if gate.Status == "failed" {
				_, _ = fmt.Fprintf(writer, "failed %-22s %s\n", gate.GateID, gate.LogPath)
			}
		}
	case []devtool.RunSummary:
		for _, run := range value {
			detail := run.Request.Kind
			if run.Request.Profile != "" {
				detail += ":" + run.Request.Profile
			} else if run.Request.Variant != "" {
				detail += ":" + run.Request.Variant
			}
			_, _ = fmt.Fprintf(writer, "%s  %-10s %-24s %s\n", run.ID, run.Status, detail, humanDuration(run.DurationMS))
		}
	case devtool.QAPlan:
		_, _ = fmt.Fprintf(writer, "%s: %s\n", value.Profile, strings.Join(value.GateIDs, ", "))
	case devtool.DoctorReport:
		for _, check := range value.Checks {
			_, _ = fmt.Fprintf(writer, "%-10s %s  %s\n", check.Name, check.Status, check.Detected)
		}
		_, _ = fmt.Fprintf(writer, "development %t  pr-qa %t  full-qa %t\n", value.Capabilities.ContainerDevelopment, value.Capabilities.PRQA, value.Capabilities.FullQA)
	case devtool.Catalog:
		_, _ = fmt.Fprintln(writer, "variants")
		for _, variant := range value.Variants {
			publication := "local-only"
			if variant.Publishable {
				publication = "publishable"
			}
			_, _ = fmt.Fprintf(writer, "  %-12s %-11s %s\n", variant.ID, publication, variant.Description)
		}
		_, _ = fmt.Fprintln(writer, "qa gates")
		for _, gate := range value.Gates {
			_, _ = fmt.Fprintf(writer, "  %-24s %-7s %s\n", gate.ID, gate.Timeout, gate.Description)
		}
	case devtool.DeveloperMCPManifest:
		_, _ = fmt.Fprintln(writer, "One-time central MCP registration (model-agnostic; stateless server-catalog routing):")
		raw, err := json.MarshalIndent(value.ClientConfig, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(writer, string(raw))
		return err
	case devtool.LogTail:
		for _, line := range value.Lines {
			_, _ = fmt.Fprintln(writer, line)
		}
	case devtool.SourceChangeResult:
		state := "current"
		if value.ChangedFileCount > 0 {
			state = fmt.Sprintf("%d changed file(s)", value.ChangedFileCount)
		}
		_, _ = fmt.Fprintf(writer, "%s %s: %s\n", value.Tool, value.Mode, state)
		for _, path := range value.ChangedFiles {
			_, _ = fmt.Fprintln(writer, path)
		}
		if value.Truncated {
			_, _ = fmt.Fprintln(writer, "[additional paths omitted]")
		}
	case devtool.CacheReport:
		prunableBytes := int64(0)
		prunableEntries := 0
		for _, entry := range value.Entries {
			if entry.Prunable {
				prunableBytes += entry.Bytes
				prunableEntries++
			}
		}
		_, _ = fmt.Fprintf(writer, "%s\ntotal %s  prunable %s in %d entries\n", value.Root, humanBytes(value.TotalBytes), humanBytes(prunableBytes), prunableEntries)
		for _, entry := range value.Entries {
			if entry.Prunable {
				_, _ = fmt.Fprintf(writer, "%-24s %9s  %s\n", entry.Kind+":"+entry.Key, humanBytes(entry.Bytes), entry.Path)
			}
		}
	case devtool.CachePruneResult:
		action := "would remove"
		count := len(value.Candidates)
		bytes := value.CandidateBytes
		if !value.DryRun {
			action = "removed"
			count = len(value.Removed)
			bytes = value.RemovedBytes
		}
		_, _ = fmt.Fprintf(writer, "%s %d entries (%s), older than %s\n", action, count, humanBytes(bytes), value.OlderThan)
		if value.DryRun && count > 0 {
			_, _ = fmt.Fprintln(writer, "rerun with --apply to remove only these hk-managed entries")
		}
	case devtool.RaceShardResult:
		_, _ = fmt.Fprintf(writer, "%s race shard: %d packages\n", value.Shard, value.PackageCount)
	case string:
		_, _ = fmt.Fprintln(writer, value)
	default:
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(writer, string(raw))
		return err
	}
	return nil
}

func renderPlainDevStatus(writer io.Writer, status devtool.DevStatus) {
	_, _ = fmt.Fprintf(writer, "%s  %s  %s\n", status.State, status.Variant, status.Owner)
	if status.URLs.Web != "" {
		_, _ = fmt.Fprintf(writer, "web %s  api %s  mail %s\n", status.URLs.Web, status.URLs.API, status.URLs.Mailpit)
	}
	for _, service := range status.Services {
		state := "unavailable"
		if service.Reachable {
			state = "ready"
		}
		_, _ = fmt.Fprintf(writer, "%-10s %-11s %s\n", service.Name, state, service.Address)
	}
	if status.Error != "" {
		_, _ = fmt.Fprintln(writer, "error", status.Error)
	}
}

func plainDevEvent(event devtool.DevEvent) string {
	label := event.Component
	if label == "" {
		label = event.Type
	}
	if event.Phase != "" {
		return fmt.Sprintf("[%s] %s: %s", label, event.Phase, event.Message)
	}
	return fmt.Sprintf("[%s] %s", label, event.Message)
}

func humanBytes(bytes int64) string {
	const unit = int64(1024)
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	for _, suffix := range units {
		value /= float64(unit)
		if value < float64(unit) || suffix == units[len(units)-1] {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%d B", bytes)
}

func humanDuration(milliseconds int64) string {
	if milliseconds <= 0 {
		return "active"
	}
	return (time.Duration(milliseconds) * time.Millisecond).Round(time.Millisecond).String()
}
