package worker

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/mailables"
	"hitkeep/internal/reporting"
)

func TestReportPeriodLabelDailyUsesSingleDay(t *testing.T) {
	start := time.Date(2026, time.March, 30, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC)

	if got := reportPeriodLabel("en", api.ReportFrequencyDaily, start, end); got != "Mar 30, 2026" {
		t.Fatalf("expected daily label Mar 30, 2026, got %q", got)
	}
}

func TestReportPeriodLabelWeeklyUsesRange(t *testing.T) {
	start := time.Date(2026, time.March, 23, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.March, 30, 0, 0, 0, 0, time.UTC)

	if got := reportPeriodLabel("en", api.ReportFrequencyWeekly, start, end); got != "Mar 23–29, 2026" {
		t.Fatalf("expected weekly label Mar 23–29, 2026, got %q", got)
	}
}

func TestReportPeriodLabelMonthlyUsesMonthAndYear(t *testing.T) {
	start := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)

	if got := reportPeriodLabel("en", api.ReportFrequencyMonthly, start, end); got != "March 2026" {
		t.Fatalf("expected monthly label March 2026, got %q", got)
	}
}

func TestInclusivePeriodEndUsesExclusiveBoundary(t *testing.T) {
	start := time.Date(2026, time.March, 30, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC)

	got := inclusivePeriodEnd(start, end)
	want := end.Add(-time.Nanosecond)
	if !got.Equal(want) {
		t.Fatalf("expected inclusive end %s, got %s", want.Format(time.RFC3339Nano), got.Format(time.RFC3339Nano))
	}
}

func TestReportContentBuilderBuildsTheScheduledSiteSummaryContent(t *testing.T) {
	store := setupReportContentStore(t)
	ctx := context.Background()
	userID, err := store.CreateUser(ctx, "report-content@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "content-builder.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report := &api.ReportDefinition{
		ID: uuid.New(), Name: "Morning summary", Scope: api.ReportScopePersonal,
		Preset: api.ReportPresetSiteSummary, SiteMode: api.ReportSiteModeSelected,
		Sites:    []api.ReportSite{{ID: site.ID, Domain: site.Domain}},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "Europe/Berlin", LocalTime: "08:00"},
	}
	scheduledFor := time.Date(2026, time.March, 30, 6, 0, 0, 0, time.UTC)
	start, end, _, _, err := reportingPeriod(report.Schedule, scheduledFor)
	if err != nil {
		t.Fatal(err)
	}
	builder := NewReportContentBuilder(store, func(context.Context, uuid.UUID) (*database.Store, error) { return store, nil }, "https://hitkeep.example")
	email, shouldSend, err := builder.Build(ctx, ReportContentRequest{
		Report: report, RecipientUserID: userID, RecipientLocale: "en",
		ScheduledFor: scheduledFor, PeriodStart: start, PeriodEnd: end,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !shouldSend || email == nil {
		t.Fatal("site summary content was unexpectedly suppressed")
	}
	if !strings.Contains(email.Subject(), site.Domain) {
		t.Fatalf("subject = %q, want site domain", email.Subject())
	}
	content, ok := email.(*mailables.SiteAnalyticsReport)
	if !ok {
		t.Fatalf("content type = %T, want site summary", email)
	}
	for _, want := range []string{"site=" + site.ID.String(), "from=2026-03-29", "to=2026-03-29", "report=true"} {
		if !strings.Contains(content.DashURL, want) {
			t.Fatalf("dashboard URL = %q, want %q", content.DashURL, want)
		}
	}
}

func TestReportContentBuilderMakesExternalSiteSummarySelfContained(t *testing.T) {
	store := setupReportContentStore(t)
	ctx := context.Background()
	userID, err := store.CreateUser(ctx, "external-content@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "external-content.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report := &api.ReportDefinition{
		ID: uuid.New(), Name: "External summary", Scope: api.ReportScopeTeam,
		Preset: api.ReportPresetSiteSummary, SiteMode: api.ReportSiteModeSelected,
		Sites:    []api.ReportSite{{ID: site.ID, Domain: site.Domain}},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"},
	}
	scheduledFor := time.Date(2026, time.March, 30, 8, 0, 0, 0, time.UTC)
	start, end, _, _, err := reportingPeriod(report.Schedule, scheduledFor)
	if err != nil {
		t.Fatal(err)
	}
	builder := NewReportContentBuilder(store, func(context.Context, uuid.UUID) (*database.Store, error) { return store, nil }, "https://private.example")
	email, shouldSend, err := builder.Build(ctx, ReportContentRequest{
		Report: report, RecipientLocale: "en", SelfContained: true,
		ScheduledFor: scheduledFor, PeriodStart: start, PeriodEnd: end,
	})
	if err != nil || !shouldSend {
		t.Fatalf("external content shouldSend=%v err=%v", shouldSend, err)
	}
	content, ok := email.(*mailables.SiteAnalyticsReport)
	if !ok {
		t.Fatalf("content type = %T", email)
	}
	if content.DashURL != "" || content.SettingsURL != "" || content.SiteDomain != site.Domain {
		t.Fatalf("external content leaked deep links or lost content: %+v", content)
	}
}

func TestReportWorkerExternalDeliveryRequiresPaidCloudPlan(t *testing.T) {
	ctx := context.Background()
	teamID := uuid.New()
	report := &api.ReportDefinition{TenantID: &teamID}
	cloudConfig := &config.Config{CloudHosted: true}

	free := entitlements.NewStaticProvider(
		entitlements.Entitlements{},
		entitlements.PlanInfo{Code: entitlements.PlanCodeFree, Name: "Free"},
	)
	worker := (&ReportWorker{}).WithEntitlements(entitlements.NewService(nil, free, cloudConfig))
	if worker.allowsExternalReportRecipient(ctx, report) {
		t.Fatal("expected the worker to block external delivery after a downgrade to Free")
	}

	pro := entitlements.NewStaticProvider(
		entitlements.Entitlements{AllowExternalReportRecipients: true},
		entitlements.PlanInfo{Code: "pro", Name: "Pro"},
	)
	worker.WithEntitlements(entitlements.NewService(nil, pro, cloudConfig))
	if !worker.allowsExternalReportRecipient(ctx, report) {
		t.Fatal("expected the worker to allow external delivery on Pro")
	}
	if worker.allowsExternalReportRecipient(ctx, &api.ReportDefinition{}) {
		t.Fatal("expected the worker to reject external delivery without a team")
	}

	cloudConfig.CloudHosted = false
	worker.WithEntitlements(entitlements.NewService(nil, free, cloudConfig))
	if !worker.allowsExternalReportRecipient(ctx, report) {
		t.Fatal("expected self-hosted delivery to remain unrestricted")
	}
}

func TestReportContentBuilderSuppressesAnEmptyOpportunityBrief(t *testing.T) {
	store := setupReportContentStore(t)
	ctx := context.Background()
	userID, err := store.CreateUser(ctx, "empty-opportunity-report@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "empty-opportunity.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report := &api.ReportDefinition{
		ID: uuid.New(), Scope: api.ReportScopePersonal, Preset: api.ReportPresetOpportunityBrief,
		SiteMode: api.ReportSiteModeSelected, Sites: []api.ReportSite{{ID: site.ID, Domain: site.Domain}},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyWeekly, Timezone: "UTC", LocalTime: "08:00", WeeklyDay: new(1)},
	}
	scheduledFor := time.Date(2026, time.March, 30, 8, 0, 0, 0, time.UTC)
	start, end, _, _, err := reportingPeriod(report.Schedule, scheduledFor)
	if err != nil {
		t.Fatal(err)
	}
	builder := NewReportContentBuilder(store, func(context.Context, uuid.UUID) (*database.Store, error) { return store, nil }, "https://hitkeep.example")
	email, shouldSend, err := builder.Build(ctx, ReportContentRequest{
		Report: report, RecipientUserID: userID, RecipientLocale: "en",
		ScheduledFor: scheduledFor, PeriodStart: start, PeriodEnd: end,
	})
	if err != nil {
		t.Fatal(err)
	}
	if shouldSend || email != nil {
		t.Fatalf("empty opportunity content = %T/%v, want suppressed", email, shouldSend)
	}
}

func setupReportContentStore(t *testing.T) *database.Store {
	t.Helper()
	store := database.NewStore(filepath.Join(t.TempDir(), "report-content.db"))
	if err := store.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func reportingPeriod(schedule api.ReportSchedule, scheduledFor time.Time) (time.Time, time.Time, time.Time, time.Time, error) {
	return reporting.PeriodBounds(schedule, scheduledFor)
}
