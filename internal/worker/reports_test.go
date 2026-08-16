package worker

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/mailables"
	"hitkeep/internal/mailer"
	"hitkeep/internal/reporting"
	"hitkeep/internal/testutil/testdb"
)

func TestReportMailFailureFieldsAreGrouped(t *testing.T) {
	attr := reportMailFailureLogAttr("external", "message-123", 2, "smtp_send_failed", mailer.ErrorDetails{
		Stage: "transport", Kind: "temporary_rejection", Message: "mail transport failed", SMTPCode: "421",
	})
	if attr.Key != "mail" || attr.Value.Kind() != slog.KindGroup {
		t.Fatalf("expected mail group, got %#v", attr)
	}
	fields := make(map[string]slog.Value)
	for _, field := range attr.Value.Group() {
		fields[field.Key] = field.Value
	}
	for _, key := range []string{"recipient_kind", "message_id", "attempt", "error_code", "error_stage", "error_kind", "error_message", "smtp_code"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("mail group is missing %q: %#v", key, fields)
		}
	}
}

type capturedScheduledReport struct {
	to        []string
	messageID string
	headers   map[string]string
}

type scheduledReportDriver struct {
	messages      []capturedScheduledReport
	failRemaining int
}

func (d *scheduledReportDriver) Send(to []string, _ string, _ string, _ string) error {
	return d.SendWithHeaders(to, "", "", "", "", nil)
}

func (d *scheduledReportDriver) SendWithHeaders(to []string, _ string, _ string, _ string, messageID string, headers map[string]string) error {
	copiedHeaders := make(map[string]string, len(headers))
	maps.Copy(copiedHeaders, headers)
	d.messages = append(d.messages, capturedScheduledReport{to: append([]string(nil), to...), messageID: messageID, headers: copiedHeaders})
	if d.failRemaining > 0 {
		d.failRemaining--
		return errors.New("smtp unavailable")
	}
	return nil
}

func (d *scheduledReportDriver) Close() error { return nil }

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

func TestReportWorkerDeliversDueReportOnceWithStableDeliveryIdentity(t *testing.T) {
	store := setupReportContentStore(t)
	report, now := setupDueReportWorkerFixture(t, store, "scheduled-report@example.test")
	driver := &scheduledReportDriver{}
	worker := NewReportWorker(newTestTenantMgr(t, store), mailer.NewWithDriver(driver, nil), "https://hitkeep.example", "worker-test-secret")

	worker.RunAt(context.Background(), now)

	if len(driver.messages) != 1 {
		t.Fatalf("mail attempts = %d, want exactly one", len(driver.messages))
	}
	message := driver.messages[0]
	if len(message.to) != 1 || message.to[0] != "scheduled-report@example.test" {
		t.Fatalf("recipients = %v", message.to)
	}
	if !strings.HasPrefix(message.messageID, "<report.") || !strings.HasSuffix(message.messageID, "@hitkeep>") {
		t.Fatalf("message ID = %q", message.messageID)
	}
	if message.headers["Auto-Submitted"] != "auto-generated" || message.headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" || !strings.Contains(message.headers["List-Unsubscribe"], "/api/reports/unsubscribe/") {
		t.Fatalf("delivery headers = %#v", message.headers)
	}

	runs, err := store.ListReportRuns(context.Background(), report.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "completed" || len(runs[0].Deliveries) != 1 || runs[0].Deliveries[0].Status != "accepted" || runs[0].Deliveries[0].AttemptCount != 1 || runs[0].Deliveries[0].SMTPAcceptedAt == nil {
		t.Fatalf("accepted run = %+v", runs)
	}
	var storedMessageID string
	if err := store.DB().QueryRowContext(context.Background(), "SELECT message_id FROM report_deliveries WHERE id = ?", runs[0].Deliveries[0].ID).Scan(&storedMessageID); err != nil {
		t.Fatal(err)
	}
	if storedMessageID != message.messageID {
		t.Fatalf("stored message ID = %q, sent = %q", storedMessageID, message.messageID)
	}
	updated, err := store.GetReportDefinition(context.Background(), report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.NextRunAt == nil || !updated.NextRunAt.After(now) {
		t.Fatalf("next run = %v, want after %v", updated.NextRunAt, now)
	}

	worker.RunAt(context.Background(), now)
	if len(driver.messages) != 1 {
		t.Fatalf("repeated occurrence sent %d messages, want one", len(driver.messages))
	}
	runs, err = store.ListReportRuns(context.Background(), report.ID, 10)
	if err != nil || len(runs) != 1 {
		t.Fatalf("repeated occurrence runs = %+v, err = %v", runs, err)
	}
}

func TestReportWorkerReusesMessageIDAfterSMTPFailure(t *testing.T) {
	store := setupReportContentStore(t)
	report, now := setupDueReportWorkerFixture(t, store, "retry-report@example.test")
	driver := &scheduledReportDriver{failRemaining: 1}
	worker := NewReportWorker(newTestTenantMgr(t, store), mailer.NewWithDriver(driver, nil), "https://hitkeep.example", "worker-test-secret")

	worker.RunAt(context.Background(), now)
	runs, err := store.ListReportRuns(context.Background(), report.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "running" || len(runs[0].Deliveries) != 1 || runs[0].Deliveries[0].Status != "failed" || runs[0].Deliveries[0].AttemptCount != 1 || runs[0].Deliveries[0].NextAttemptAt == nil {
		t.Fatalf("failed attempt = %+v", runs)
	}

	worker.RunAt(context.Background(), now.Add(5*time.Minute))
	if len(driver.messages) != 2 || driver.messages[0].messageID == "" || driver.messages[1].messageID != driver.messages[0].messageID {
		t.Fatalf("retry message identities = %+v", driver.messages)
	}
	runs, err = store.ListReportRuns(context.Background(), report.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if runs[0].Status != "completed" || runs[0].Deliveries[0].Status != "accepted" || runs[0].Deliveries[0].AttemptCount != 2 || runs[0].Deliveries[0].SMTPAcceptedAt == nil {
		t.Fatalf("successful retry = %+v", runs[0])
	}
}

func setupDueReportWorkerFixture(t *testing.T, store *database.Store, email string) (*api.ReportDefinition, time.Time) {
	t.Helper()
	ctx := context.Background()
	userID, err := store.CreateUser(ctx, email, "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "delivery-"+uuid.NewString()+".example.test")
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.CreateReportDefinition(ctx, userID, api.CreateReportRequest{
		Name: "Daily delivery", Scope: api.ReportScopePersonal, Preset: api.ReportPresetSiteSummary,
		SiteMode: api.ReportSiteModeSelected, SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{userID},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"},
		Status:   api.ReportStatusActive,
	}, "worker-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	if _, err := store.DB().ExecContext(ctx, "UPDATE report_definitions SET next_run_at = ? WHERE id = ?", time.Date(2026, 7, 22, 8, 0, 0, 0, time.UTC), report.ID); err != nil {
		t.Fatal(err)
	}
	return report, now
}

func setupReportContentStore(t *testing.T) *database.Store {
	t.Helper()
	return testdb.Shared(t)
}

func reportingPeriod(schedule api.ReportSchedule, scheduledFor time.Time) (time.Time, time.Time, time.Time, time.Time, error) {
	return reporting.PeriodBounds(schedule, scheduledFor)
}
