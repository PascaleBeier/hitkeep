package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"hitkeep/config"
	hitai "hitkeep/internal/ai"
	"hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/opportunities"
	"hitkeep/internal/opportunities/smokegate"
)

type recordingRecorder struct {
	base hitai.StoreRecorder
	runs []hitai.RunRecord
}

func (r *recordingRecorder) RecordAIRun(ctx context.Context, run hitai.RunRecord) (uuid.UUID, error) {
	id, err := r.base.RecordAIRun(ctx, run)
	if err != nil {
		return uuid.Nil, err
	}
	run.ID = id
	r.runs = append(r.runs, run)
	return id, nil
}

func (r *recordingRecorder) ReserveAIRun(ctx context.Context, run hitai.RunRecord, since time.Time, requestLimit, tokenLimit int) (uuid.UUID, error) {
	return r.base.ReserveAIRun(ctx, run, since, requestLimit, tokenLimit)
}

func (r *recordingRecorder) GetAIUsageSince(ctx context.Context, since time.Time) (hitai.BudgetUsage, error) {
	return r.base.GetAIUsageSince(ctx, since)
}

var errSmokeNotReleaseReady = errors.New("opportunities smoke gate did not pass")

type smokeRunner func(context.Context, smokeConfig) (smokegate.Report, error)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(executeSmoke(ctx, os.Args[1:], os.Stdout, os.Stderr, runSmoke))
}

type smokeSyntaxError struct{ err error }

func (e *smokeSyntaxError) Error() string { return e.err.Error() }

func executeSmoke(ctx context.Context, args []string, stdout, stderr io.Writer, run smokeRunner) int {
	cmd := newSmokeCommand(run)
	cmd.SetArgs(normalizeLegacySmokeArgs(args))
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, errSmokeNotReleaseReady) {
			return 2
		}
		var syntaxError *smokeSyntaxError
		if errors.As(err, &syntaxError) {
			fmt.Fprintln(stderr, syntaxError)
			writeSmokeUsage(stderr, cmd)
			return 2
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func writeSmokeUsage(stderr io.Writer, cmd *cobra.Command) {
	fmt.Fprint(stderr, cmd.UsageString())
}

func newSmokeCommand(run smokeRunner) *cobra.Command {
	var v *viper.Viper
	cmd := &cobra.Command{
		Use:           "opportunities-smoke",
		Short:         "Run the opportunities release smoke gate",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			conf, err := smokeConfigFromViper(v)
			if err != nil {
				return err
			}
			report, err := run(cmd.Context(), conf)
			if err != nil {
				return redactSmokeError(err, conf.APIKey)
			}
			if err := writeSmokeReport(conf.OutPath, report); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), conf.OutPath)
			if !smokegate.Evaluate(report).ReleaseReady {
				return errSmokeNotReleaseReady
			}
			return nil
		},
	}
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &smokeSyntaxError{err: err}
	})
	cmd.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		writeSmokeUsage(cmd.ErrOrStderr(), cmd)
	})
	flags := cmd.Flags()
	flags.SetInterspersed(false)
	flags.String("db", "", "restored shared HitKeep database path")
	flags.String("out", "tmp/prod-eu-opportunities-smoke/release-hardening-smoke.md", "markdown report output path")
	flags.String("domains", "hitkeep.com,cloud.hitkeep.eu", "comma-separated site domains to smoke")
	flags.String("provider", "openai-compatible", "AI provider")
	flags.String("model", "openai.gpt-oss-120b", "AI model")
	flags.String("region", "eu-central-1", "AI provider region")
	flags.String("base-url", smokeCatalogSetting("AIBaseURL").Default, "AI provider base URL")
	flags.String("data-path", smokeCatalogSetting("DataPath").Default, "restored HitKeep data directory containing tenant databases")
	flags.Bool("ai", true, "enable AI provider calls")
	flags.Int("window-days", 30, "analysis window in days")
	flags.String("to", "2026-05-09T19:05:42Z", "analysis end timestamp")
	v = newSmokeViper(flags)
	return cmd
}

type smokeCatalogBinding struct {
	key   string
	field string
}

var smokeCatalogBindings = []smokeCatalogBinding{
	{key: "provider", field: "AIProvider"},
	{key: "model", field: "AIModel"},
	{key: "region", field: "AIRegion"},
	{key: "base-url", field: "AIBaseURL"},
	{key: "data-path", field: "DataPath"},
	{key: "api-key", field: "AIAPIKey"},
}

func newSmokeViper(flags *pflag.FlagSet) *viper.Viper {
	v := viper.New()
	if err := v.BindPFlags(flags); err != nil {
		panic(err)
	}
	for _, binding := range smokeCatalogBindings {
		setting := smokeCatalogSetting(binding.field)
		if err := v.BindEnv(binding.key, setting.Environment); err != nil {
			panic(err)
		}
		if binding.key == "api-key" {
			v.SetDefault(binding.key, setting.Default)
		}
	}
	return v
}

func smokeCatalogSetting(field string) config.ConfigurationSetting {
	for _, setting := range config.Catalog().Settings {
		if setting.Field == field {
			return setting
		}
	}
	panic("missing configuration catalog setting " + field)
}

type smokeViperConfig struct {
	DBPath     string `mapstructure:"db"`
	OutPath    string `mapstructure:"out"`
	Domains    string `mapstructure:"domains"`
	Provider   string `mapstructure:"provider"`
	Model      string `mapstructure:"model"`
	Region     string `mapstructure:"region"`
	BaseURL    string `mapstructure:"base-url"`
	DataPath   string `mapstructure:"data-path"`
	APIKey     string `mapstructure:"api-key"`
	AIEnabled  bool   `mapstructure:"ai"`
	WindowDays int    `mapstructure:"window-days"`
	To         string `mapstructure:"to"`
}

func smokeConfigFromViper(v *viper.Viper) (smokeConfig, error) {
	var values smokeViperConfig
	if err := v.Unmarshal(&values); err != nil {
		return smokeConfig{}, fmt.Errorf("decode command configuration: %w", err)
	}
	if strings.TrimSpace(values.DBPath) == "" {
		return smokeConfig{}, errors.New("-db is required")
	}
	to, err := time.Parse(time.RFC3339, values.To)
	if err != nil {
		return smokeConfig{}, fmt.Errorf("parse -to: %w", err)
	}
	return smokeConfig{
		DBPath:     values.DBPath,
		OutPath:    values.OutPath,
		Domains:    splitCSV(values.Domains),
		Provider:   values.Provider,
		Model:      values.Model,
		Region:     values.Region,
		BaseURL:    resolveSmokeAIBaseURL(values.Provider, values.Region, values.BaseURL),
		DataPath:   values.DataPath,
		APIKey:     strings.TrimSpace(values.APIKey),
		AIEnabled:  values.AIEnabled,
		WindowDays: values.WindowDays,
		To:         to,
	}, nil
}

func redactSmokeError(err error, apiKey string) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), apiKey, "[REDACTED]"))
}

func writeSmokeReport(path string, report smokegate.Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(smokegate.RenderMarkdown(report)), 0o600); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func normalizeLegacySmokeArgs(args []string) []string {
	legacyLongFlags := map[string]bool{
		"db": true, "out": true, "domains": true, "provider": true, "model": true, "region": true, "base-url": true, "data-path": true, "ai": false, "window-days": true, "to": true,
	}
	normalized := make([]string, 0, len(args))
	expectsValue := false
	for index, arg := range args {
		if expectsValue {
			normalized = append(normalized, arg)
			expectsValue = false
			continue
		}
		if arg == "--" || !strings.HasPrefix(arg, "-") {
			return append(normalized, args[index:]...)
		}
		name, _, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if name == "h" || name == "help" {
			return append(normalized, "--help")
		}
		takesValue, known := legacyLongFlags[name]
		if !strings.HasPrefix(arg, "--") && known {
			arg = "-" + arg
		}
		normalized = append(normalized, arg)
		expectsValue = known && takesValue && !hasValue
	}
	return normalized
}

type smokeConfig struct {
	DBPath     string
	OutPath    string
	Domains    []string
	Provider   string
	Model      string
	Region     string
	BaseURL    string
	DataPath   string
	APIKey     string
	AIEnabled  bool
	WindowDays int
	To         time.Time
}

func runSmoke(ctx context.Context, conf smokeConfig) (smokegate.Report, error) {
	workingDB, cleanup, err := prepareWorkingDB(conf.DBPath)
	if err != nil {
		return smokegate.Report{}, err
	}
	defer cleanup()

	store, err := database.OpenMigratedStore(ctx, workingDB)
	if err != nil {
		return smokegate.Report{}, fmt.Errorf("connect restored db: %w", err)
	}
	defer store.Close()
	workingDataPath, cleanupDataPath, err := prepareWorkingDataPath(conf.DataPath)
	if err != nil {
		return smokegate.Report{}, err
	}
	defer cleanupDataPath()
	tenantStores := map[uuid.UUID]*database.Store{}
	defer closeTenantStores(tenantStores)

	recorder := &recordingRecorder{base: hitai.StoreRecorder{Store: store}}
	service := opportunities.Service{
		Shared:  store,
		Catalog: opportunities.NewDefaultDetectorCatalog(),
	}
	if conf.AIEnabled {
		ai, err := hitai.NewService(hitai.Config{
			Enabled:             true,
			Provider:            conf.Provider,
			Model:               conf.Model,
			BaseURL:             conf.BaseURL,
			Region:              conf.Region,
			APIKey:              conf.APIKey,
			Timeout:             45 * time.Second,
			RequestLimit:        80,
			TokenLimit:          240000,
			BudgetWindowMinutes: 60,
			ConfigMode:          "cloud_managed",
		}, recorder)
		if err != nil {
			return smokegate.Report{}, fmt.Errorf("configure ai: %w", err)
		}
		service.AI = ai
	}

	from := conf.To.AddDate(0, 0, -conf.WindowDays)
	report := smokegate.Report{
		GeneratedAt: time.Now().UTC(),
		Source:      conf.DBPath,
		Provider:    conf.Provider,
		Model:       conf.Model,
	}
	actorID := uuid.MustParse("cce03cbc-a88a-451c-92aa-3381def5713b")
	for _, domain := range conf.Domains {
		target := smokegate.TargetResult{Domain: domain, From: from, To: conf.To}
		site, err := store.FindSiteByDomain(ctx, domain)
		if err != nil {
			target.Error = err.Error()
			report.Targets = append(report.Targets, target)
			continue
		}
		teamID, err := store.GetSiteTenantID(ctx, site.ID)
		if err != nil {
			target.Error = err.Error()
			report.Targets = append(report.Targets, target)
			continue
		}
		analyticsStore, err := tenantAnalyticsStore(ctx, store, tenantStores, conf.DataPath, workingDataPath, teamID)
		if err != nil {
			target.Error = err.Error()
			report.Targets = append(report.Targets, target)
			continue
		}
		generated, _, status, err := service.Generate(ctx, opportunities.GenerateInput{
			TeamID:                teamID,
			Site:                  *site,
			Store:                 analyticsStore,
			From:                  from,
			To:                    conf.To,
			ActorID:               actorID,
			ActorType:             "ai_smoke_gate",
			EffectiveUserID:       actorID,
			EffectiveInstanceRole: auth.InstanceOwner,
			EffectiveSiteRole:     auth.SiteOwner,
		})
		target.Status = status
		target.Opportunities = generated
		if err != nil {
			target.Error = err.Error()
		}
		report.Targets = append(report.Targets, target)
	}
	for _, run := range recorder.runs {
		report.AIRuns = append(report.AIRuns, smokegate.AIRun{
			ID:            run.ID,
			Provider:      run.Provider,
			Model:         run.Model,
			Status:        run.Status,
			ErrorCategory: run.ErrorCategory,
			OutputJSON:    run.OutputJSON,
			TotalTokens:   run.Usage.TotalTokens,
			ToolCalls:     run.Usage.ToolCallCount,
			EvidenceIDs:   append([]string(nil), run.EvidenceIDs...),
		})
	}
	return report, nil
}

func prepareWorkingDataPath(source string) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "hitkeep-opportunities-smoke-data-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create working data dir: %w", err)
	}
	return tmp, func() { _ = os.RemoveAll(tmp) }, nil
}

func tenantAnalyticsStore(ctx context.Context, shared *database.Store, cache map[uuid.UUID]*database.Store, sourceDataPath, workingDataPath string, tenantID uuid.UUID) (*database.Store, error) {
	defaultID, err := shared.GetDefaultTenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve default tenant: %w", err)
	}
	if tenantID == uuid.Nil || tenantID == defaultID {
		return shared, nil
	}
	if store, ok := cache[tenantID]; ok {
		return store, nil
	}
	sourcePath := filepath.Join(sourceDataPath, "tenants", tenantID.String(), "hitkeep.db")
	targetPath := filepath.Join(workingDataPath, "tenants", tenantID.String(), "hitkeep.db")
	if err := copyFile(sourcePath, targetPath); err != nil {
		return nil, fmt.Errorf("copy tenant db %s: %w", tenantID, err)
	}
	store, err := database.OpenMigratedTenantStore(ctx, targetPath)
	if err != nil {
		return nil, fmt.Errorf("connect tenant db %s: %w", tenantID, err)
	}
	cache[tenantID] = store
	return store, nil
}

func closeTenantStores(stores map[uuid.UUID]*database.Store) {
	for _, store := range stores {
		_ = store.Close()
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func resolveSmokeAIBaseURL(provider, region, baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL != "" || !strings.EqualFold(strings.TrimSpace(provider), "openai-compatible") {
		return baseURL
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = "eu-central-1"
	}
	return "https://bedrock-mantle." + region + ".api.aws/v1"
}

func prepareWorkingDB(source string) (string, func(), error) {
	tmp, err := os.CreateTemp("", "hitkeep-opportunities-smoke-*.db")
	if err != nil {
		return "", func() {}, fmt.Errorf("create working db: %w", err)
	}
	cleanup := func() { _ = os.Remove(tmp.Name()) }
	target := tmp.Name()
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close working db: %w", err)
	}
	if err := copyFile(source, target); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("copy working db: %w", err)
	}
	return target, cleanup, nil
}

func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create target dir: %w", err)
	}
	output, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create target: %w", err)
	}
	defer output.Close()
	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}
