package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/reporting"
)

func TestReportDefinitionMigrationPreservesLegacyUTC0800Subscriptions(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := applyMigrationsThrough(t, store, "2026_07_18_000000_unify_traffic_exclusions.sql"); err != nil {
		t.Fatalf("apply baseline migrations: %v", err)
	}

	userID, err := store.CreateUser(ctx, "legacy-report@example.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "legacy-report.example.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	now := time.Date(2026, 7, 18, 4, 0, 0, 0, time.UTC)
	for _, subscription := range []struct {
		table     string
		frequency api.ReportFrequency
		enabled   bool
	}{
		{table: "digest_subscriptions", frequency: api.ReportFrequencyDaily, enabled: true},
		{table: "digest_subscriptions", frequency: api.ReportFrequencyWeekly, enabled: true},
		{table: "digest_subscriptions", frequency: api.ReportFrequencyMonthly, enabled: false},
	} {
		if _, err := store.DB().ExecContext(ctx, `
			INSERT INTO digest_subscriptions (id, user_id, frequency, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, uuid.New(), userID, subscription.frequency, subscription.enabled, now, now); err != nil {
			t.Fatalf("insert %s %s: %v", subscription.table, subscription.frequency, err)
		}
	}
	for _, frequency := range []api.ReportFrequency{api.ReportFrequencyDaily, api.ReportFrequencyMonthly} {
		if _, err := store.DB().ExecContext(ctx, `
			INSERT INTO site_report_subscriptions (id, user_id, site_id, frequency, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, true, ?, ?)
		`, uuid.New(), userID, site.ID, frequency, now, now); err != nil {
			t.Fatalf("insert site %s: %v", frequency, err)
		}
	}

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate report definitions: %v", err)
	}
	reports, err := store.ListReportDefinitions(ctx, userID)
	if err != nil {
		t.Fatalf("list reports: %v", err)
	}
	if len(reports) != 4 {
		t.Fatalf("migrated report count = %d, want 4", len(reports))
	}

	digestCount := 0
	siteCount := 0
	for _, report := range reports {
		if report.Source != "legacy" || report.Scope != api.ReportScopePersonal || report.OwnerUserID == nil || *report.OwnerUserID != userID {
			t.Fatalf("legacy ownership not preserved: %+v", report)
		}
		if report.Schedule.Timezone != "UTC" || report.Schedule.LocalTime != "08:00" {
			t.Fatalf("legacy schedule = %s %s, want UTC 08:00", report.Schedule.Timezone, report.Schedule.LocalTime)
		}
		if report.NextRunAt == nil || report.NextRunAt.Location() != time.UTC || report.NextRunAt.Hour() != 8 {
			t.Fatalf("next run = %v, want 08:00 UTC", report.NextRunAt)
		}
		if len(report.Recipients) != 1 || report.Recipients[0].UserID == nil || *report.Recipients[0].UserID != userID {
			t.Fatalf("legacy recipient not preserved: %+v", report.Recipients)
		}
		switch report.Preset {
		case api.ReportPresetPortfolioDigest:
			digestCount++
			if report.SiteMode != api.ReportSiteModeAllAccessible || len(report.Sites) != 0 {
				t.Fatalf("portfolio migration has unexpected sites: %+v", report.Sites)
			}
		case api.ReportPresetSiteSummary:
			siteCount++
			if len(report.Sites) != 1 || report.Sites[0].ID != site.ID {
				t.Fatalf("site migration has unexpected sites: %+v", report.Sites)
			}
		default:
			t.Fatalf("unexpected migrated preset %q", report.Preset)
		}
	}
	if digestCount != 2 || siteCount != 2 {
		t.Fatalf("migrated presets digest=%d site=%d, want 2 each", digestCount, siteCount)
	}
}

func TestExternalRecipientMigrationPreservesMemberDeliveryIdentity(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := applyMigrationsThrough(t, store, "2026_07_18_010000_create_report_definitions.sql"); err != nil {
		t.Fatalf("apply named-report migration: %v", err)
	}
	rebuildNamedReportTablesWithHistoricalForeignKeys(t, store)

	userID, err := store.CreateUser(ctx, "delivery-migration@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "delivery-migration.example.test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 18, 8, 0, 0, 0, time.UTC)
	reportID, recipientID, runID, deliveryID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO report_definitions (
			id, owner_user_id, created_by, name, scope, preset, site_mode, frequency,
			timezone, local_time, status, source, next_run_at, created_at, updated_at
		) VALUES (?, ?, ?, 'Migration history', 'personal', 'site_summary', 'selected',
		          'daily', 'UTC', '08:00', 'active', 'v2', ?, ?, ?)
	`, reportID, userID, userID, now.Add(24*time.Hour), now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO report_definition_sites (id, report_id, site_id, created_at)
		VALUES (?, ?, ?, ?)
	`, uuid.New(), reportID, site.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO report_recipients (id, report_id, user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, recipientID, reportID, userID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO report_runs (
			id, report_id, scheduled_for, period_start, period_end, status, completed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 'completed', ?, ?, ?)
	`, runID, reportID, now, now.Add(-24*time.Hour), now, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO report_deliveries (
			id, report_id, run_id, recipient_user_id, status, message_id,
			attempt_count, smtp_accepted_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'accepted', 'legacy-message', 1, ?, ?, ?)
	`, deliveryID, reportID, runID, userID, now, now, now); err != nil {
		t.Fatal(err)
	}

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("apply external-recipient migration: %v", err)
	}
	var mappedRecipientID uuid.UUID
	var recipientKind string
	if err := store.DB().QueryRowContext(ctx,
		"SELECT recipient_id, recipient_kind FROM report_deliveries WHERE id = ?",
		deliveryID,
	).Scan(&mappedRecipientID, &recipientKind); err != nil {
		t.Fatal(err)
	}
	if mappedRecipientID != recipientID || recipientKind != string(api.ReportRecipientKindMember) {
		t.Fatalf("migrated delivery identity = %s/%s, want %s/member", mappedRecipientID, recipientKind, recipientID)
	}
	var confirmedAt time.Time
	if err := store.DB().QueryRowContext(ctx,
		"SELECT confirmed_at FROM report_recipients WHERE id = ?",
		recipientID,
	).Scan(&confirmedAt); err != nil || confirmedAt.IsZero() {
		t.Fatalf("member consent not preserved: confirmed_at=%v err=%v", confirmedAt, err)
	}
	var legacyColumnCount int
	if err := store.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'report_deliveries' AND column_name = 'recipient_user_id'
	`).Scan(&legacyColumnCount); err != nil || legacyColumnCount != 0 {
		t.Fatalf("legacy delivery identity remains: count=%d err=%v", legacyColumnCount, err)
	}
}

func rebuildNamedReportTablesWithHistoricalForeignKeys(t *testing.T, store *Store) {
	t.Helper()
	const historicalSchema = `
		DROP TABLE report_deliveries;
		DROP TABLE report_definition_sites;
		DROP TABLE report_recipients;
		DROP TABLE report_runs;
		DROP TABLE report_definitions;

		CREATE TABLE report_definitions (
			id UUID PRIMARY KEY, tenant_id UUID REFERENCES tenants(id), owner_user_id UUID,
			created_by UUID, name VARCHAR NOT NULL, scope VARCHAR NOT NULL, preset VARCHAR NOT NULL,
			site_mode VARCHAR NOT NULL, frequency VARCHAR NOT NULL, timezone VARCHAR NOT NULL,
			local_time VARCHAR NOT NULL, weekly_day SMALLINT, monthly_day SMALLINT,
			status VARCHAR NOT NULL, source VARCHAR NOT NULL, legacy_key VARCHAR UNIQUE,
			next_run_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL
		);
		CREATE TABLE report_definition_sites (
			id UUID PRIMARY KEY, report_id UUID NOT NULL REFERENCES report_definitions(id),
			site_id UUID NOT NULL REFERENCES sites(id), tenant_id UUID REFERENCES tenants(id),
			created_at TIMESTAMPTZ NOT NULL, UNIQUE (report_id, site_id)
		);
		CREATE TABLE report_recipients (
			id UUID PRIMARY KEY, report_id UUID NOT NULL REFERENCES report_definitions(id),
			tenant_id UUID REFERENCES tenants(id), user_id UUID NOT NULL REFERENCES users(id),
			unsubscribe_token_hash VARCHAR UNIQUE, opted_out_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
			UNIQUE (report_id, user_id)
		);
		CREATE TABLE report_runs (
			id UUID PRIMARY KEY, report_id UUID NOT NULL REFERENCES report_definitions(id),
			tenant_id UUID REFERENCES tenants(id), scheduled_for TIMESTAMPTZ NOT NULL,
			period_start TIMESTAMPTZ NOT NULL, period_end TIMESTAMPTZ NOT NULL,
			status VARCHAR NOT NULL, safe_error_code VARCHAR, started_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL, UNIQUE (report_id, scheduled_for)
		);
		CREATE TABLE report_deliveries (
			id UUID PRIMARY KEY, report_id UUID NOT NULL REFERENCES report_definitions(id),
			run_id UUID NOT NULL REFERENCES report_runs(id), tenant_id UUID REFERENCES tenants(id),
			recipient_user_id UUID NOT NULL REFERENCES users(id), status VARCHAR NOT NULL,
			message_id VARCHAR NOT NULL, attempt_count INTEGER NOT NULL DEFAULT 0,
			next_attempt_at TIMESTAMPTZ, safe_error_code VARCHAR, smtp_accepted_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
			UNIQUE (run_id, recipient_user_id)
		);
	`
	if _, err := store.DB().Exec(historicalSchema); err != nil {
		t.Fatalf("rebuild historical named-report schema: %v", err)
	}
}

func TestClaimDueReportRunsIsIdempotentAcrossConcurrentWorkers(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	userID, err := store.CreateUser(ctx, "concurrent-report@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "concurrent-report.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.CreateReportDefinition(ctx, userID, api.CreateReportRequest{
		Name: "Concurrent delivery", Scope: api.ReportScopePersonal, Preset: api.ReportPresetSiteSummary,
		SiteMode: api.ReportSiteModeSelected, SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{userID},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"},
		Status:   api.ReportStatusActive,
	}, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	if _, err := store.DB().ExecContext(ctx, "UPDATE report_definitions SET next_run_at = ? WHERE id = ?", time.Date(2026, 7, 18, 8, 0, 0, 0, time.UTC), report.ID); err != nil {
		t.Fatal(err)
	}

	type claimResult struct {
		runs []uuid.UUID
		err  error
	}
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	claim := func() {
		go func() {
			<-start
			runs, claimErr := store.ClaimDueReportRuns(ctx, now)
			results <- claimResult{runs: runs, err: claimErr}
		}()
	}
	claim()
	claim()
	close(start)

	claimed := 0
	for _, result := range []claimResult{<-results, <-results} {
		if result.err != nil {
			t.Fatalf("concurrent claim failed: %v", result.err)
		}
		claimed += len(result.runs)
	}
	if claimed != 1 {
		t.Fatalf("concurrent workers claimed %d runs, want 1", claimed)
	}
	runs, err := store.ListReportRuns(ctx, report.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || len(runs[0].Deliveries) != 1 {
		t.Fatalf("durable concurrent result = %+v, want one run and delivery", runs)
	}
}

func TestExternalReportRecipientConsentLifecycleAndScopeVersion(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	managerID, err := store.CreateUser(ctx, "external-report-manager@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, managerID, "external-consent.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.CreateReportDefinition(ctx, managerID, api.CreateReportRequest{
		Name: "Client pulse", Scope: api.ReportScopeTeam, TenantID: &tenantID,
		Preset: api.ReportPresetSiteSummary, SiteMode: api.ReportSiteModeSelected,
		SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{managerID},
		ExternalRecipientEmails: []string{" Client@Example.Test ", "client@example.test"},
		Schedule:                api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"},
		Status:                  api.ReportStatusActive,
	}, "test-secret")
	if err != nil {
		t.Fatalf("create external report: %v", err)
	}
	if report.ConsentVersion != 1 || len(report.Recipients) != 2 {
		t.Fatalf("created report = %+v", report)
	}
	external := report.Recipients[0]
	if external.Kind != api.ReportRecipientKindExternal {
		external = report.Recipients[1]
	}
	if external.Email != "client@example.test" || external.Status != api.ReportRecipientStatusPending {
		t.Fatalf("external recipient = %+v", external)
	}

	now := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	prepared, err := store.PrepareReportRecipientConfirmation(ctx, report.ID, external.ID, "de", now, false)
	if err != nil {
		t.Fatalf("prepare confirmation: %v", err)
	}
	if len(prepared.Token) < 40 || prepared.Metadata.TeamName == "" || len(prepared.Metadata.Sites) != 1 {
		t.Fatalf("prepared confirmation = %+v", prepared)
	}
	var storedHash, storedLocale string
	if err := store.DB().QueryRowContext(ctx, `SELECT confirmation_token_hash, external_locale FROM report_recipients WHERE id = ?`, external.ID).Scan(&storedHash, &storedLocale); err != nil {
		t.Fatal(err)
	}
	if storedHash != reporting.ConfirmationTokenHash(prepared.Token) || storedHash == prepared.Token || storedLocale != "de" {
		t.Fatalf("stored token state hash=%q locale=%q", storedHash, storedLocale)
	}
	if _, err := store.GetReportRecipientConfirmation(ctx, prepared.Token, now.Add(time.Hour)); err != nil {
		t.Fatalf("first metadata GET: %v", err)
	}
	if _, err := store.GetReportRecipientConfirmation(ctx, prepared.Token, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("second metadata GET mutated consent: %v", err)
	}
	if err := store.ConfirmReportRecipient(ctx, prepared.Token, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := store.ConfirmReportRecipient(ctx, prepared.Token, now.Add(2*time.Hour)); !errors.Is(err, ErrReportConfirmationInvalid) {
		t.Fatalf("second confirmation error = %v", err)
	}

	localTime := "09:15"
	timeOnly := report.Schedule
	timeOnly.LocalTime = localTime
	report, err = store.UpdateReportDefinition(ctx, managerID, report.ID, api.UpdateReportRequest{Schedule: &timeOnly}, "test-secret")
	if err != nil {
		t.Fatalf("time-only update: %v", err)
	}
	if report.ConsentVersion != 1 || externalRecipientStatus(report) != api.ReportRecipientStatusConfirmed {
		t.Fatalf("time-only update invalidated consent: %+v", report)
	}

	weeklyDay := 1
	material := api.ReportSchedule{Frequency: api.ReportFrequencyWeekly, Timezone: "UTC", LocalTime: localTime, WeeklyDay: &weeklyDay}
	report, err = store.UpdateReportDefinition(ctx, managerID, report.ID, api.UpdateReportRequest{Schedule: &material}, "test-secret")
	if err != nil {
		t.Fatalf("material update: %v", err)
	}
	if report.ConsentVersion != 2 || externalRecipientStatus(report) != api.ReportRecipientStatusPending {
		t.Fatalf("material update did not invalidate consent: %+v", report)
	}
	externalID := uuid.Nil
	for _, recipient := range report.Recipients {
		if recipient.Kind == api.ReportRecipientKindExternal {
			externalID = recipient.ID
		}
	}
	rotated, err := store.PrepareReportRecipientConfirmation(ctx, report.ID, externalID, "de", now.Add(3*time.Hour), false)
	if err != nil {
		t.Fatalf("prepare rotated confirmation: %v", err)
	}
	if _, err := store.PrepareReportRecipientConfirmation(ctx, report.ID, externalID, "de", now.Add(3*time.Hour+time.Minute), true); !errors.Is(err, ErrReportConfirmationCooldown) {
		t.Fatalf("resend cooldown error = %v", err)
	}
	if err := store.DeclineReportRecipient(ctx, rotated.Token, now.Add(4*time.Hour)); err != nil {
		t.Fatalf("decline: %v", err)
	}
	report, err = store.GetReportDefinition(ctx, report.ID)
	if err != nil || externalRecipientStatus(report) != api.ReportRecipientStatusOptedOut {
		t.Fatalf("declined recipient = %+v err=%v", report, err)
	}
	fresh, err := store.PrepareReportRecipientConfirmation(ctx, report.ID, externalID, "de", now.Add(4*time.Hour+16*time.Minute), true)
	if err != nil {
		t.Fatalf("prepare fresh confirmation after opt-out: %v", err)
	}
	if err := store.ConfirmReportRecipient(ctx, fresh.Token, now.Add(4*time.Hour+17*time.Minute)); err != nil {
		t.Fatalf("fresh confirm: %v", err)
	}
	report, err = store.GetReportDefinition(ctx, report.ID)
	if err != nil || externalRecipientStatus(report) != api.ReportRecipientStatusConfirmed {
		t.Fatalf("fresh confirmation did not restore delivery: %+v err=%v", report, err)
	}
	daily := api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: localTime}
	report, err = store.UpdateReportDefinition(ctx, managerID, report.ID, api.UpdateReportRequest{Schedule: &daily}, "test-secret")
	if err != nil {
		t.Fatalf("second material update: %v", err)
	}
	expiring, err := store.PrepareReportRecipientConfirmation(ctx, report.ID, externalID, "de", now.Add(5*time.Hour), false)
	if err != nil {
		t.Fatalf("prepare expiring confirmation: %v", err)
	}
	if _, err := store.GetReportRecipientConfirmation(ctx, expiring.Token, now.Add(8*24*time.Hour)); !errors.Is(err, ErrReportConfirmationExpired) {
		t.Fatalf("expired confirmation error = %v", err)
	}
	removedToken, err := store.PrepareReportRecipientConfirmation(ctx, report.ID, externalID, "de", now.Add(8*24*time.Hour+time.Minute), true)
	if err != nil {
		t.Fatalf("rotate expired confirmation: %v", err)
	}
	noExternalRecipients := []string{}
	if _, err := store.UpdateReportDefinition(ctx, managerID, report.ID, api.UpdateReportRequest{
		ExternalRecipientEmails: &noExternalRecipients,
	}, "test-secret"); err != nil {
		t.Fatalf("remove external recipient: %v", err)
	}
	if err := store.ConfirmReportRecipient(ctx, removedToken.Token, now.Add(8*24*time.Hour+2*time.Minute)); !errors.Is(err, ErrReportConfirmationInvalid) {
		t.Fatalf("removed recipient confirmation error = %v", err)
	}
}

func TestPendingExternalRecipientIsNotBackfilledAfterConfirmation(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	managerID, err := store.CreateUser(ctx, "no-catchup-manager@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, managerID, "no-catchup.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.CreateReportDefinition(ctx, managerID, api.CreateReportRequest{
		Name: "No catch-up", Scope: api.ReportScopeTeam, TenantID: &tenantID,
		Preset: api.ReportPresetSiteSummary, SiteMode: api.ReportSiteModeSelected,
		SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{managerID}, ExternalRecipientEmails: []string{"client@example.test"},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"}, Status: api.ReportStatusActive,
	}, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	if _, err := store.DB().ExecContext(ctx, `UPDATE report_definitions SET next_run_at = ? WHERE id = ?`, now.Add(-time.Hour), report.ID); err != nil {
		t.Fatal(err)
	}
	runIDs, err := store.ClaimDueReportRuns(ctx, now)
	if err != nil || len(runIDs) != 1 {
		t.Fatalf("first run = %v err=%v", runIDs, err)
	}
	first, err := store.ListReportRuns(ctx, report.ID, 10)
	if err != nil || len(first) != 1 || len(first[0].Deliveries) != 1 || first[0].Deliveries[0].RecipientKind != api.ReportRecipientKindMember {
		t.Fatalf("pending recipient got a delivery: %+v err=%v", first, err)
	}
	external := report.Recipients[0]
	if external.Kind != api.ReportRecipientKindExternal {
		external = report.Recipients[1]
	}
	prepared, err := store.PrepareReportRecipientConfirmation(ctx, report.ID, external.ID, "en", now, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ConfirmReportRecipient(ctx, prepared.Token, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	first, err = store.ListReportRuns(ctx, report.ID, 10)
	if err != nil || len(first[0].Deliveries) != 1 {
		t.Fatalf("confirmation backfilled old run: %+v err=%v", first, err)
	}
}

func TestRemovedExternalRecipientDoesNotLeaveEmailSnapshotInDeliveryHistory(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	managerID, err := store.CreateUser(ctx, "history-manager@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, managerID, "history-external.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.CreateReportDefinition(ctx, managerID, api.CreateReportRequest{
		Name: "History privacy", Scope: api.ReportScopeTeam, TenantID: &tenantID,
		Preset: api.ReportPresetSiteSummary, SiteMode: api.ReportSiteModeSelected,
		SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{managerID},
		ExternalRecipientEmails: []string{"history-client@example.test"},
		Schedule:                api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"},
		Status:                  api.ReportStatusActive,
	}, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	var external api.ReportRecipient
	for _, recipient := range report.Recipients {
		if recipient.Kind == api.ReportRecipientKindExternal {
			external = recipient
		}
	}
	prepared, err := store.PrepareReportRecipientConfirmation(ctx, report.ID, external.ID, "en", time.Now().UTC(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ConfirmReportRecipient(ctx, prepared.Token, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(2 * time.Hour)
	if _, err := store.DB().ExecContext(ctx, "UPDATE report_definitions SET next_run_at = ? WHERE id = ?", now.Add(-time.Minute), report.ID); err != nil {
		t.Fatal(err)
	}
	if runs, err := store.ClaimDueReportRuns(ctx, now); err != nil || len(runs) != 1 {
		t.Fatalf("claim delivery run = %v err=%v", runs, err)
	}
	emptyExternalRecipients := []string{}
	if _, err := store.UpdateReportDefinition(ctx, managerID, report.ID, api.UpdateReportRequest{
		ExternalRecipientEmails: &emptyExternalRecipients,
	}, "test-secret"); err != nil {
		t.Fatal(err)
	}
	history, err := store.ListReportRuns(ctx, report.ID, 10)
	if err != nil || len(history) != 1 {
		t.Fatalf("delivery history = %+v err=%v", history, err)
	}
	for _, delivery := range history[0].Deliveries {
		if delivery.RecipientID != external.ID {
			continue
		}
		if delivery.RecipientKind != api.ReportRecipientKindExternal || delivery.RecipientEmail != "" || delivery.RecipientUserID != nil {
			t.Fatalf("removed external metadata retained in delivery: %+v", delivery)
		}
		return
	}
	t.Fatal("external delivery identity was not preserved")
}

func TestScheduleReportOnboardingRequiresDeliverableNamedRecipient(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	managerID, err := store.CreateUser(ctx, "onboarding-report-manager@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, managerID, "onboarding-report.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.CreateReportDefinition(ctx, managerID, api.CreateReportRequest{
		Name: "Pending onboarding", Scope: api.ReportScopeTeam, TenantID: &tenantID,
		Preset: api.ReportPresetSiteSummary, SiteMode: api.ReportSiteModeSelected,
		SiteIDs: []uuid.UUID{site.ID}, ExternalRecipientEmails: []string{"onboarding-client@example.test"},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"},
		Status:   api.ReportStatusActive,
	}, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	onboarding, err := store.GetUserOnboarding(ctx, managerID)
	if err != nil {
		t.Fatal(err)
	}
	if onboardingStepComplete(onboarding, "schedule_report") {
		t.Fatal("pending-only named report completed onboarding")
	}
	prepared, err := store.PrepareReportRecipientConfirmation(ctx, report.ID, report.Recipients[0].ID, "en", time.Now().UTC(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ConfirmReportRecipient(ctx, prepared.Token, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	onboarding, err = store.GetUserOnboarding(ctx, managerID)
	if err != nil || !onboardingStepComplete(onboarding, "schedule_report") {
		t.Fatalf("confirmed named report onboarding = %+v err=%v", onboarding, err)
	}
}

func onboardingStepComplete(onboarding *api.UserOnboarding, key string) bool {
	if onboarding == nil {
		return false
	}
	for _, step := range onboarding.Steps {
		if step.Key == key {
			return step.Complete
		}
	}
	return false
}

func TestExternalReportScopeRequiresActiveTeamAndCurrentSiteOwnership(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	ownerID, err := store.CreateUser(ctx, "external-scope-owner@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.CreateTenant(ctx, ownerID, "External Scope Team", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetActiveTenantID(ctx, ownerID, team.ID); err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, ownerID, "external-scope.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report := &api.ReportDefinition{
		Scope: api.ReportScopeTeam, TenantID: &team.ID, SiteMode: api.ReportSiteModeSelected,
		Sites: []api.ReportSite{{ID: site.ID, Domain: site.Domain}},
	}
	if allowed, err := store.ExternalRecipientCanAccessReportSites(ctx, report); err != nil || !allowed {
		t.Fatalf("active external scope allowed=%v err=%v", allowed, err)
	}
	foreignTeam, err := store.CreateTenant(ctx, ownerID, "Foreign Scope Team", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.TransferSiteTeamWithAudit(ctx, site.ID, foreignTeam.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	if allowed, _ := store.ExternalRecipientCanAccessReportSites(ctx, report); allowed {
		t.Fatal("external scope survived site tenant change")
	}
	if err := store.TransferSiteTeamWithAudit(ctx, site.ID, team.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenant_archives (tenant_id, archived_at, archived_by) VALUES (?, ?, ?)",
		team.ID, time.Now().UTC(), ownerID,
	); err != nil {
		t.Fatal(err)
	}
	if allowed, _ := store.ExternalRecipientCanAccessReportSites(ctx, report); allowed {
		t.Fatal("external scope survived team archival")
	}
}

func externalRecipientStatus(report *api.ReportDefinition) api.ReportRecipientStatus {
	for _, recipient := range report.Recipients {
		if recipient.Kind == api.ReportRecipientKindExternal {
			return recipient.Status
		}
	}
	return ""
}

func TestNamedReportOptOutCanOnlyBeClearedForSameRecipient(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	userID, err := store.CreateUser(ctx, "report-owner@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "named-report.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.CreateReportDefinition(ctx, userID, api.CreateReportRequest{
		Name: "Morning summary", Scope: api.ReportScopePersonal, Preset: api.ReportPresetSiteSummary,
		SiteMode: api.ReportSiteModeSelected, SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{uuid.New()},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "Europe/Berlin", LocalTime: "08:15"},
		Status:   api.ReportStatusDraft,
	}, "test-secret")
	if err != nil {
		t.Fatalf("create report: %v", err)
	}
	if len(report.Recipients) != 1 || report.Recipients[0].UserID == nil || *report.Recipients[0].UserID != userID {
		t.Fatalf("personal report recipients = %+v, want owner only", report.Recipients)
	}

	recipientID := report.Recipients[0].ID
	token := reportingTokenForTest(report.ID, recipientID)
	if err := store.OptOutReportRecipient(ctx, report.ID, recipientID, token.hash, time.Now().UTC()); err != nil {
		t.Fatalf("opt out: %v", err)
	}
	if err := store.ResubscribeReportRecipient(ctx, report.ID, uuid.New(), time.Now().UTC()); err != nil {
		t.Fatalf("unrelated resubscribe should remain a no-op: %v", err)
	}
	after, err := store.GetReportDefinition(ctx, report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Recipients[0].OptedOutAt == nil {
		t.Fatal("another user cleared the recipient opt-out")
	}
	if err := store.ResubscribeReportRecipient(ctx, report.ID, userID, time.Now().UTC()); err != nil {
		t.Fatalf("recipient resubscribe: %v", err)
	}
	after, err = store.GetReportDefinition(ctx, report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Recipients[0].OptedOutAt != nil {
		t.Fatal("recipient opt-out was not cleared")
	}
}

func TestClaimDueReportRunsCatchesUpLatestOccurrenceOnceAndRetriesStably(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	userID, err := store.CreateUser(ctx, "report-delivery@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "delivery-report.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.CreateReportDefinition(ctx, userID, api.CreateReportRequest{
		Name: "Daily delivery", Scope: api.ReportScopePersonal, Preset: api.ReportPresetSiteSummary,
		SiteMode: api.ReportSiteModeSelected, SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{userID},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"},
		Status:   api.ReportStatusActive,
	}, "test-secret")
	if err != nil {
		t.Fatalf("create report: %v", err)
	}
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	oldestMissed := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	if _, err := store.DB().ExecContext(ctx, "UPDATE report_definitions SET next_run_at = ? WHERE id = ?", oldestMissed, report.ID); err != nil {
		t.Fatal(err)
	}

	runs, err := store.ClaimDueReportRuns(ctx, now)
	if err != nil {
		t.Fatalf("claim due runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("claimed runs = %d, want 1", len(runs))
	}
	secondClaim, err := store.ClaimDueReportRuns(ctx, now)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if len(secondClaim) != 0 {
		t.Fatalf("second worker claimed duplicate runs: %v", secondClaim)
	}

	reportRuns, err := store.ListReportRuns(ctx, report.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reportRuns) != 1 || !reportRuns[0].ScheduledFor.Equal(time.Date(2026, 7, 18, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("scheduled run = %+v, want latest missed occurrence", reportRuns)
	}
	if len(reportRuns[0].Deliveries) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(reportRuns[0].Deliveries))
	}
	deliveryID := reportRuns[0].Deliveries[0].ID
	claimed, err := store.ClaimReportDelivery(ctx, deliveryID, now)
	if err != nil || !claimed {
		t.Fatalf("claim delivery = %v, %v", claimed, err)
	}
	first, err := store.GetPendingReportDelivery(ctx, deliveryID)
	if err != nil {
		t.Fatal(err)
	}
	if first.AttemptCount != 1 {
		t.Fatalf("first attempt count = %d", first.AttemptCount)
	}
	if err := store.MarkReportDeliveryFailed(ctx, deliveryID, first.AttemptCount, "smtp_send_failed", now); err != nil {
		t.Fatal(err)
	}
	dueEarly, err := store.ListDueReportDeliveries(ctx, now.Add(4*time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(dueEarly) != 0 {
		t.Fatalf("delivery retried before five minutes: %v", dueEarly)
	}
	retryAt := now.Add(5 * time.Minute)
	claimed, err = store.ClaimReportDelivery(ctx, deliveryID, retryAt)
	if err != nil || !claimed {
		t.Fatalf("claim retry = %v, %v", claimed, err)
	}
	second, err := store.GetPendingReportDelivery(ctx, deliveryID)
	if err != nil {
		t.Fatal(err)
	}
	if second.AttemptCount != 2 || second.MessageID != first.MessageID {
		t.Fatalf("retry attempt/message = %d/%q, want 2/%q", second.AttemptCount, second.MessageID, first.MessageID)
	}
	if err := store.MarkReportDeliveryFailed(ctx, deliveryID, second.AttemptCount, "smtp_send_failed", retryAt); err != nil {
		t.Fatal(err)
	}
	if due, err := store.ListDueReportDeliveries(ctx, retryAt.Add(29*time.Minute), 10); err != nil || len(due) != 0 {
		t.Fatalf("delivery retried before thirty minutes: %v, %v", due, err)
	}
	thirdAt := retryAt.Add(30 * time.Minute)
	claimed, err = store.ClaimReportDelivery(ctx, deliveryID, thirdAt)
	if err != nil || !claimed {
		t.Fatalf("claim third attempt = %v, %v", claimed, err)
	}
	third, err := store.GetPendingReportDelivery(ctx, deliveryID)
	if err != nil {
		t.Fatal(err)
	}
	if third.AttemptCount != 3 || third.MessageID != first.MessageID {
		t.Fatalf("third attempt/message = %d/%q, want 3/%q", third.AttemptCount, third.MessageID, first.MessageID)
	}
	if err := store.MarkReportDeliveryFailed(ctx, deliveryID, third.AttemptCount, "smtp_send_failed", thirdAt); err != nil {
		t.Fatal(err)
	}
	if due, err := store.ListDueReportDeliveries(ctx, thirdAt.Add(119*time.Minute), 10); err != nil || len(due) != 0 {
		t.Fatalf("delivery retried before two hours: %v, %v", due, err)
	}
	fourthAt := thirdAt.Add(2 * time.Hour)
	claimed, err = store.ClaimReportDelivery(ctx, deliveryID, fourthAt)
	if err != nil || !claimed {
		t.Fatalf("claim final attempt = %v, %v", claimed, err)
	}
	fourth, err := store.GetPendingReportDelivery(ctx, deliveryID)
	if err != nil {
		t.Fatal(err)
	}
	if fourth.AttemptCount != 4 || fourth.MessageID != first.MessageID {
		t.Fatalf("final attempt/message = %d/%q, want 4/%q", fourth.AttemptCount, fourth.MessageID, first.MessageID)
	}
	if err := store.MarkReportDeliveryFailed(ctx, deliveryID, fourth.AttemptCount, "smtp_send_failed", fourthAt); err != nil {
		t.Fatal(err)
	}
	if due, err := store.ListDueReportDeliveries(ctx, fourthAt.Add(24*time.Hour), 10); err != nil || len(due) != 0 {
		t.Fatalf("exhausted delivery remained retryable: %v, %v", due, err)
	}
	if err := store.FinalizeReportRun(ctx, reportRuns[0].ID, fourthAt); err != nil {
		t.Fatal(err)
	}
	finalRuns, err := store.ListReportRuns(ctx, report.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(finalRuns) != 1 || finalRuns[0].Status != "failed" || finalRuns[0].CompletedAt == nil {
		t.Fatalf("exhausted run = %+v, want completed failure", finalRuns)
	}
}

func TestTeamReportVisibilityRequiresManagementRoleOrRecipient(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	managerID, err := store.CreateUser(ctx, "report-manager@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	recipientID, err := store.CreateUser(ctx, "report-recipient@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	outsiderID, err := store.CreateUser(ctx, "report-outsider@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, managerID, "team-report-visibility.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddSiteMember(ctx, site.ID, recipientID, auth.SiteViewer, managerID); err != nil {
		t.Fatalf("add report recipient to site: %v", err)
	}
	report, err := store.CreateReportDefinition(ctx, managerID, api.CreateReportRequest{
		Name: "Team summary", Scope: api.ReportScopeTeam, TenantID: &tenantID,
		Preset: api.ReportPresetSiteSummary, SiteMode: api.ReportSiteModeSelected,
		SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{recipientID},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyWeekly, Timezone: "Europe/Berlin", LocalTime: "08:00", WeeklyDay: new(1)},
		Status:   api.ReportStatusDraft,
	}, "test-secret")
	if err != nil {
		t.Fatalf("create team report: %v", err)
	}

	recipientReports, err := store.ListReportDefinitions(ctx, recipientID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipientReports) != 1 || recipientReports[0].ID != report.ID {
		t.Fatalf("recipient reports = %+v, want team report", recipientReports)
	}
	outsiderReports, err := store.ListReportDefinitions(ctx, outsiderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(outsiderReports) != 0 {
		t.Fatalf("non-recipient member could see team reports: %+v", outsiderReports)
	}
	managerReports, err := store.ListReportDefinitions(ctx, managerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(managerReports) != 1 || managerReports[0].ID != report.ID {
		t.Fatalf("team manager reports = %+v, want team report", managerReports)
	}
}

func TestTeamReportRemainsManageableAfterCreatorDeletion(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	creatorID, err := store.CreateUser(ctx, "report-creator@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	successorID, err := store.CreateUser(ctx, "report-successor@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTeamMember(ctx, tenantID, successorID, TenantRoleOwner, creatorID); err != nil {
		t.Fatalf("promote successor: %v", err)
	}
	site, err := store.CreateSite(ctx, successorID, "surviving-team-report.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.CreateReportDefinition(ctx, creatorID, api.CreateReportRequest{
		Name: "Surviving team summary", Scope: api.ReportScopeTeam, TenantID: &tenantID,
		Preset: api.ReportPresetSiteSummary, SiteMode: api.ReportSiteModeSelected,
		SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{successorID},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"},
		Status:   api.ReportStatusDraft,
	}, "test-secret")
	if err != nil {
		t.Fatalf("create team report: %v", err)
	}
	if err := store.DeleteUser(ctx, creatorID); err != nil {
		t.Fatalf("delete report creator: %v", err)
	}

	after, err := store.GetReportDefinition(ctx, report.ID)
	if err != nil {
		t.Fatalf("load surviving team report: %v", err)
	}
	if after.TenantID == nil || *after.TenantID != tenantID || after.OwnerUserID != nil || after.CreatedBy != nil {
		t.Fatalf("team ownership after creator deletion = %+v", after)
	}
	manageable, err := store.CanManageReport(ctx, after.ID, successorID)
	if err != nil || !manageable {
		t.Fatalf("successor cannot manage surviving report: manageable=%v err=%v", manageable, err)
	}
	if len(after.Recipients) != 1 || after.Recipients[0].UserID == nil || *after.Recipients[0].UserID != successorID {
		t.Fatalf("surviving recipients = %+v", after.Recipients)
	}
}

type reportTokenTestValue struct {
	hash string
}

func reportingTokenForTest(reportID, userID uuid.UUID) reportTokenTestValue {
	// CreateReportDefinition stores this deterministic hash at creation time.
	token := reporting.UnsubscribeToken("test-secret", reportID, userID)
	return reportTokenTestValue{hash: reporting.UnsubscribeTokenHash(token)}
}
