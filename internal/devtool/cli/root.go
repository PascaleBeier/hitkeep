package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"hitkeep/internal/devtool"
	"hitkeep/internal/devtool/devmcp"
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
	}
	root.PersistentFlags().StringVar(&options.workspace, "workspace", ".", "Git worktree to operate on")
	root.PersistentFlags().StringVar(&options.output, "output", "human", "output format: human, plain, json, or ndjson")
	root.AddCommand(
		catalogCommand(options), doctorCommand(options), workspaceCommand(options), setupCommand(options),
		devCommand(options), qaCommand(options), formatCommand(options), fixCommand(options), buildCommand(options), smokeCommand(options), runCommand(options),
		cacheCommand(options), ciCommand(options), docsCommand(options), skillsCommand(options), mcpCommand(options), workerCommand(options),
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
	return sourceChangeCommand(options, "fmt [check|write]", "fmt", "Format repository Go sources", "write", func(_ context.Context, app *devtool.App, write bool) (any, error) {
		return app.FormatGo(write)
	})
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
		data, runErr := handler(cmd.Context(), app)
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
	return &cobra.Command{Use: "catalog", Short: "Show canonical variants, QA profiles, and gates", Args: cobra.NoArgs, RunE: withApp(options, "catalog", func(_ context.Context, app *devtool.App) (any, error) { return app.Catalog(), nil })}
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
	return startCommand(options, "setup", "setup", "Prepare exact dependencies for this worktree", func(*cobra.Command, []string) devtool.RunRequest { return devtool.RunRequest{Kind: "setup"} })
}

func devCommand(options *options) *cobra.Command {
	command := startCommand(options, "dev", "dev start", "Run the isolated development stack", variantRequest("dev-start"))
	command.AddCommand(startCommand(options, "start", "dev start", "Start native/container development services", variantRequest("dev-start")))
	command.AddCommand(startCommand(options, "stop", "dev stop", "Stop this worktree's development services", variantRequest("dev-stop")))
	return command
}

func qaCommand(options *options) *cobra.Command {
	request := func(cmd *cobra.Command, args []string) devtool.RunRequest {
		profile := "changed"
		if len(args) > 0 {
			profile = args[0]
		}
		gates, _ := cmd.Flags().GetStringSlice("gate")
		return devtool.RunRequest{Kind: "qa", Profile: profile, GateIDs: gates}
	}
	configure := func(cmd *cobra.Command) {
		cmd.Args = cobra.MaximumNArgs(1)
		cmd.Flags().StringSlice("gate", nil, "run only canonical gate IDs")
	}
	command := startCommand(options, "qa [changed|pr|full]", "qa start", "Plan and run canonical QA profiles", request, configure)
	var baseRef string
	plan := &cobra.Command{Use: "plan [changed|pr|full]", Args: cobra.MaximumNArgs(1), RunE: withApp(options, "qa plan", func(ctx context.Context, app *devtool.App) (any, error) {
		profile := "changed"
		if len(planArgs(ctx)) > 0 { // populated below through command context
			profile = planArgs(ctx)[0]
		}
		return app.QAPlan(ctx, profile, baseRef)
	})}
	// Cobra does not expose args to a closure nested in withApp, so capture them here.
	plan.RunE = func(cmd *cobra.Command, args []string) error {
		ctx := context.WithValue(cmd.Context(), argsKey{}, args)
		cmd.SetContext(ctx)
		return withApp(options, "qa plan", func(ctx context.Context, app *devtool.App) (any, error) {
			profile := "changed"
			if values := planArgs(ctx); len(values) > 0 {
				profile = values[0]
			}
			return app.QAPlan(ctx, profile, baseRef)
		})(cmd, args)
	}
	plan.Flags().StringVar(&baseRef, "base", "", "base ref for changed planning")
	command.AddCommand(plan)
	command.AddCommand(startCommand(options, "start [changed|pr|full]", "qa start", "Run selected QA profile", request, configure))
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
	logs := &cobra.Command{Use: "logs RUN_ID", Args: cobra.ExactArgs(1), RunE: withArgsApp(options, "run logs", func(_ context.Context, app *devtool.App, args []string) (any, error) {
		if gateID != "" {
			return app.TailGateLogAfter(args[0], gateID, limit, cursor)
		}
		return app.TailLogAfter(args[0], limit, cursor)
	})}
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
	race.Flags().StringVar(&shard, "shard", "", "canonical shard: heavy or rest")
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
	command.AddCommand(
		&cobra.Command{Use: "manifest", Short: "Emit worktree-specific client registration", Args: cobra.NoArgs, RunE: withApp(options, "mcp manifest", func(_ context.Context, app *devtool.App) (any, error) {
			return app.MCPManifest()
		})},
		&cobra.Command{Use: "serve", Short: "Serve MCP over stdio", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := devtool.NewApp(options.workspace)
			if err != nil {
				return err
			}
			return devmcp.RunStdio(cmd.Context(), app, options.version)
		}},
	)
	return command
}

func workerCommand(options *options) *cobra.Command {
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

func variantRequest(kind string) func(*cobra.Command, []string) devtool.RunRequest {
	return func(cmd *cobra.Command, _ []string) devtool.RunRequest {
		variant, _ := cmd.Flags().GetString("variant")
		runtimeID, _ := cmd.Flags().GetString("runtime")
		seed, _ := cmd.Flags().GetBool("seed")
		return devtool.RunRequest{Kind: kind, Variant: variant, Runtime: runtimeID, Seed: seed}
	}
}

type startConfig func(*cobra.Command)

func startCommand(options *options, use, command, short string, request func(*cobra.Command, []string) devtool.RunRequest, configs ...startConfig) *cobra.Command {
	var detach bool
	cmd := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs}
	cmd.Flags().BoolVar(&detach, "detach", false, "return immediately with a run ID")
	if strings.Contains(command, "dev") || strings.Contains(command, "smoke") {
		cmd.Flags().String("variant", "self-hosted", "self-hosted or cloud")
	}
	if strings.Contains(command, "dev") {
		cmd.Flags().String("runtime", "native", "native or container")
		if strings.Contains(command, "start") {
			cmd.Flags().Bool("seed", false, "seed isolated demo data before starting")
		}
	}
	for _, configure := range configs {
		configure(cmd)
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return withApp(options, command, func(ctx context.Context, app *devtool.App) (any, error) {
			start, err := app.StartRun(ctx, request(cmd, args))
			if err != nil || detach {
				return start, err
			}
			previous := map[string]string{}
			observer := func(run devtool.Run) {
				switch options.output {
				case "human":
					if len(run.GateResults) == 0 {
						if previous["run"] != run.Status {
							_, _ = fmt.Fprintf(options.stderr, "%s  %s\n", run.ID, run.Status)
							previous["run"] = run.Status
						}
						return
					}
					for _, gate := range run.GateResults {
						if previous[gate.GateID] == gate.Status || gate.Status == "queued" || gate.Status == "waiting" {
							continue
						}
						_, _ = fmt.Fprintf(options.stderr, "%-24s %s\n", gate.GateID, gate.Status)
						previous[gate.GateID] = gate.Status
					}
				case "ndjson":
					_ = render(options, devtool.SuccessEnvelope("run progress", app.WorkspaceID(), map[string]any{"run_id": run.ID, "status": run.Status, "gates": run.GateResults}))
				}
			}
			run, waitErr := app.WaitRunObserved(ctx, start.RunID, observer)
			if waitErr != nil {
				if options.output == "human" {
					renderFailedRunLogs(options.stderr, app, run)
				}
				return run, devtool.WithErrorData(waitErr, run)
			}
			return run, nil
		})(cmd, args)
	}
	return cmd
}

func renderFailedRunLogs(writer io.Writer, app *devtool.App, run devtool.Run) {
	printed := 0
	for _, gate := range run.GateResults {
		if gate.Status != "failed" || printed >= 3 {
			continue
		}
		_, _ = fmt.Fprintf(writer, "\n--- %s (last 40 lines; full log: %s) ---\n", gate.GateID, gate.LogPath)
		if tail, err := app.TailGateLog(run.ID, gate.GateID, 40); err == nil {
			for _, line := range tail.Lines {
				_, _ = fmt.Fprintln(writer, line)
			}
		}
		printed++
	}
	if printed > 0 {
		return
	}
	_, _ = fmt.Fprintf(writer, "\n--- run log (last 60 lines; full log: %s) ---\n", run.LogPath)
	if tail, err := app.TailLog(run.ID, 60); err == nil {
		for _, line := range tail.Lines {
			_, _ = fmt.Fprintln(writer, line)
		}
	}
}

func withArgsApp(options *options, command string, handler func(context.Context, *devtool.App, []string) (any, error)) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return withApp(options, command, func(ctx context.Context, app *devtool.App) (any, error) { return handler(ctx, app, args) })(cmd, args)
	}
}

func render(options *options, envelope devtool.Envelope) error {
	switch options.output {
	case "json":
		encoder := json.NewEncoder(options.stdout)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(envelope)
	case "ndjson":
		return json.NewEncoder(options.stdout).Encode(envelope)
	case "plain", "human":
		if envelope.Status == "error" {
			if envelope.Data != nil {
				if err := renderHuman(options.stdout, envelope); err != nil {
					return err
				}
			}
			_, _ = fmt.Fprintln(options.stderr, "error:", envelope.Error)
			return nil
		}
		return renderHuman(options.stdout, envelope)
	default:
		return fmt.Errorf("unknown output format %q", options.output)
	}
}

func renderHuman(writer io.Writer, envelope devtool.Envelope) error {
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
		_, _ = fmt.Fprintf(writer, "native %t  container %t  pr-qa %t  full-qa %t\n", value.Capabilities.NativeDevelopment, value.Capabilities.ContainerDevelopment, value.Capabilities.PRQA, value.Capabilities.FullQA)
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
		_, _ = fmt.Fprintln(writer, "Project-scoped MCP (model-agnostic):")
		for _, client := range value.ProjectClients {
			trust := ""
			if client.RequiresTrustedProject {
				trust = "; approve the worktree once"
			}
			_, _ = fmt.Fprintf(writer, "  %-12s automatic via %s%s\n", client.ClientName, client.ConfigPath, trust)
		}
		_, _ = fmt.Fprintln(writer, "Generic registration for any other stdio MCP host:")
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
