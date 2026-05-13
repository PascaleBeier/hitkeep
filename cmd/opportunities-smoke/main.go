package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

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

func main() {
	var dbPath string
	var outPath string
	var domains string
	var provider string
	var model string
	var region string
	var dataPath string
	var aiEnabled bool
	var windowDays int
	var toValue string
	flag.StringVar(&dbPath, "db", "", "restored shared HitKeep database path")
	flag.StringVar(&outPath, "out", "tmp/prod-eu-opportunities-smoke/release-hardening-smoke.md", "markdown report output path")
	flag.StringVar(&domains, "domains", "hitkeep.com,cloud.hitkeep.eu", "comma-separated site domains to smoke")
	flag.StringVar(&provider, "provider", envOrDefault("HITKEEP_AI_PROVIDER", "bedrock"), "AI provider")
	flag.StringVar(&model, "model", envOrDefault("HITKEEP_AI_MODEL", "eu.amazon.nova-2-lite-v1:0"), "AI model")
	flag.StringVar(&region, "region", envOrDefault("HITKEEP_AI_REGION", "eu-central-1"), "AI provider region")
	flag.StringVar(&dataPath, "data-path", envOrDefault("HITKEEP_DATA_PATH", "data"), "restored HitKeep data directory containing tenant databases")
	flag.BoolVar(&aiEnabled, "ai", true, "enable AI provider calls")
	flag.IntVar(&windowDays, "window-days", 30, "analysis window in days")
	flag.StringVar(&toValue, "to", "2026-05-09T19:05:42Z", "analysis end timestamp")
	flag.Parse()

	if strings.TrimSpace(dbPath) == "" {
		fatalf("-db is required")
	}
	to, err := time.Parse(time.RFC3339, toValue)
	if err != nil {
		fatalf("parse -to: %v", err)
	}

	report, err := runSmoke(context.Background(), smokeConfig{
		DBPath:     dbPath,
		OutPath:    outPath,
		Domains:    splitCSV(domains),
		Provider:   provider,
		Model:      model,
		Region:     region,
		DataPath:   dataPath,
		AIEnabled:  aiEnabled,
		WindowDays: windowDays,
		To:         to,
	})
	if err != nil {
		fatalf("%v", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fatalf("create output dir: %v", err)
	}
	if err := os.WriteFile(outPath, []byte(smokegate.RenderMarkdown(report)), 0o600); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Println(outPath)
	if !smokegate.Evaluate(report).ReleaseReady {
		os.Exit(2)
	}
}

type smokeConfig struct {
	DBPath     string
	OutPath    string
	Domains    []string
	Provider   string
	Model      string
	Region     string
	DataPath   string
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

	store := database.NewStore(workingDB)
	if err := store.Connect(); err != nil {
		return smokegate.Report{}, fmt.Errorf("connect restored db: %w", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return smokegate.Report{}, fmt.Errorf("migrate restored db: %w", err)
	}
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
			Region:              conf.Region,
			APIKey:              strings.TrimSpace(os.Getenv("HITKEEP_AI_API_KEY")),
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
	store := database.NewStore(targetPath)
	if err := store.Connect(); err != nil {
		return nil, fmt.Errorf("connect tenant db %s: %w", tenantID, err)
	}
	if err := store.MigrateTenant(ctx); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("migrate tenant db %s: %w", tenantID, err)
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

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
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

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
