package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/api/googleapi"

	"hitkeep/api"
	"hitkeep/database"
	"hitkeep/hklog"
	"hitkeep/searchconsole"
)

func TestSearchConsoleSyncWorkerRunDueFailureUsesStructuredDiagnostics(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := hklog.WithLogger(context.Background(), logger)
	worker := NewSearchConsoleSyncWorker(nil, nil)

	worker.runDueAndLog(ctx, 10, "Search Console sync run")

	if strings.Contains(logs.String(), "error=") {
		t.Fatalf("run-level sync failure logged raw error: %q", logs.String())
	}
	if !strings.Contains(logs.String(), "stage=orchestration") || !strings.Contains(logs.String(), "category=unknown") {
		t.Fatalf("expected structured Search Console failure diagnostics, got %q", logs.String())
	}
}

func TestSearchConsoleSyncWorkerInitialSyncImportsRecentFinalizedRows(t *testing.T) {
	ctx := context.Background()
	fixture := newSearchConsoleWorkerFixture(t, "gsc-worker@test.dev", "gsc-worker.example.com")
	defer fixture.shared.Close()
	if err := fixture.shared.UpsertGoogleSearchConsoleSyncState(ctx, database.GoogleSearchConsoleSyncStateInput{
		SiteID: fixture.site.ID,
		TeamID: fixture.teamID,
		State:  "pending",
		Manual: true,
	}); err != nil {
		t.Fatalf("seed sync state: %v", err)
	}

	source := &fakeSearchConsoleSource{rows: initialSearchConsoleRows()}
	worker := NewSearchConsoleSyncWorker(fixture.tenantMgr, source)
	worker.now = func() time.Time { return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC) }

	if err := worker.ImportSite(ctx, fixture.site.ID); err != nil {
		t.Fatalf("import site: %v", err)
	}
	requireInitialSearchConsoleQuery(t, source, fixture.propertyURI)
	requireSearchConsoleImportedClicks(t, fixture.tenantMgr, fixture.site.ID, 10)
	requireSearchConsoleSucceededState(t, fixture.shared, fixture.site.ID, "2026-02-03", "2026-05-03")
	if state, err := fixture.shared.GetGoogleSearchConsoleSyncState(ctx, fixture.site.ID); err != nil || state.TotalsBackfilledPropertyURI != fixture.propertyURI {
		t.Fatalf("expected the first sync to record its property as backfilled, got state=%+v err=%v", state, err)
	}
	requireSearchConsoleStartAudit(t, fixture.shared, fixture.teamID, fixture.site.ID)
	requireSearchConsolePreparedAudit(t, fixture.shared, fixture.teamID, fixture.site.ID)
	requireSearchConsoleImportAudit(t, fixture.shared, fixture.teamID, fixture.site.ID)
}

func TestSearchConsoleSyncWorkerQuotaErrorBacksOff(t *testing.T) {
	ctx := context.Background()
	shared := newTestStore(t)
	tenantMgr := newTestTenantMgr(t, shared)
	userID, err := shared.CreateUser(ctx, "gsc-quota@test.dev", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := shared.CreateSite(ctx, userID, "gsc-quota.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	teamID, err := shared.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get site team: %v", err)
	}
	if err := shared.UpsertGoogleSearchConsoleConnection(ctx, database.GoogleSearchConsoleConnectionInput{
		TeamID:       teamID,
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		Scope:        searchconsole.ReadOnlyScope,
		TokenExpiry:  time.Now().UTC().Add(time.Hour),
		ConnectedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if err := shared.UpsertGoogleSearchConsoleSiteMapping(ctx, database.GoogleSearchConsoleSiteMappingInput{
		SiteID:      site.ID,
		TeamID:      teamID,
		PropertyURI: "sc-domain:gsc-quota.example.com",
		MappedBy:    userID,
		MappedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed mapping: %v", err)
	}

	source := &fakeSearchConsoleSource{err: searchconsole.ClassifiedError(searchconsole.CategoryQuotaLimited, errors.New("daily quota exceeded"))}
	worker := NewSearchConsoleSyncWorker(tenantMgr, source)
	worker.now = func() time.Time { return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC) }

	if err := worker.ImportSite(ctx, site.ID); err == nil {
		t.Fatalf("expected quota error")
	}
	state, err := shared.GetGoogleSearchConsoleSyncState(ctx, site.ID)
	if err != nil {
		t.Fatalf("get sync state: %v", err)
	}
	if state == nil || state.State != "failed" || state.LastErrorCategory != string(searchconsole.CategoryQuotaLimited) {
		t.Fatalf("expected quota-limited failed state, got %+v", state)
	}
	if state.NextRetryAt == nil || !state.NextRetryAt.After(time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected quota retry time, got %+v", state.NextRetryAt)
	}
	entries, total, err := shared.ListTeamAuditEntries(ctx, teamID, "google_search_console.sync_failed", 5, 0)
	if err != nil {
		t.Fatalf("list sync failure audit: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one sync failure audit entry, got total=%d entries=%+v", total, entries)
	}
	if entries[0].Outcome != "failure" || !strings.Contains(entries[0].Details, "category=quota_limited") {
		t.Fatalf("unexpected failure audit: %+v", entries[0])
	}
	if !strings.Contains(entries[0].Details, "stage=query") || !strings.Contains(entries[0].Details, `error_message="daily quota exceeded"`) {
		t.Fatalf("expected query-stage failure diagnostic, got %+v", entries[0])
	}
	requireSearchConsoleStartAudit(t, shared, teamID, site.ID)
}

func TestSearchConsoleSyncWorkerPrunesExpiredErrorPayloadsWithoutRetryingAttentionState(t *testing.T) {
	ctx := context.Background()
	fixture := newSearchConsoleWorkerFixture(t, "gsc-retention@test.dev", "gsc-retention.example.com")
	defer fixture.shared.Close()
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	oldAttempt := now.Add(-database.GoogleSearchConsoleErrorMessageRetention - time.Hour)
	if err := fixture.shared.UpsertGoogleSearchConsoleSyncState(ctx, database.GoogleSearchConsoleSyncStateInput{
		SiteID: fixture.site.ID, TeamID: fixture.teamID, State: "needs_attention", LastAttemptAt: &oldAttempt,
		LastErrorCategory: string(searchconsole.CategoryCredentialsInvalid),
	}); err != nil {
		t.Fatalf("seed expired sync error: %v", err)
	}
	if err := fixture.shared.AppendAuditEntry(ctx, database.AuditEntryParams{
		TeamID: fixture.teamID, Action: "google_search_console.sync_failed", TargetType: "site", TargetID: fixture.site.ID.String(), Outcome: "failure",
		Details: `outcome=failed;stage=query;category=credentials_invalid;http_status=400;error_message="expired provider payload"`,
	}); err != nil {
		t.Fatalf("seed expired audit error: %v", err)
	}
	if _, err := fixture.shared.DB().ExecContext(ctx, "UPDATE instance_audit_log SET created_at = ? WHERE target_id = ?", oldAttempt, fixture.site.ID.String()); err != nil {
		t.Fatalf("age audit error: %v", err)
	}

	worker := NewSearchConsoleSyncWorker(fixture.tenantMgr, &fakeSearchConsoleSource{})
	worker.now = func() time.Time { return now }
	summary, err := worker.RunDue(ctx, 10)
	if err != nil {
		t.Fatalf("run due: %v", err)
	}
	if summary.Attempted != 0 {
		t.Fatalf("expected attention state not to retry, got %+v", summary)
	}
	entries, _, err := fixture.shared.ListTeamAuditEntries(ctx, fixture.teamID, "google_search_console.sync_failed", 5, 0)
	if err != nil {
		t.Fatalf("list pruned audit: %v", err)
	}
	if len(entries) != 1 || strings.Contains(entries[0].Details, ";error_message=") || !strings.Contains(entries[0].Details, "http_status=400") {
		t.Fatalf("expected audit metadata without expired payload, got %+v", entries)
	}
}

func TestSearchConsoleSyncWorkerContinuesWhenErrorRetentionMaintenanceFails(t *testing.T) {
	ctx := context.Background()
	fixture := newSearchConsoleWorkerFixture(t, "gsc-retention-failure@test.dev", "gsc-retention-failure.example.com")
	defer fixture.shared.Close()
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	if err := fixture.shared.UpsertGoogleSearchConsoleSyncState(ctx, database.GoogleSearchConsoleSyncStateInput{
		SiteID: fixture.site.ID, TeamID: fixture.teamID, State: "needs_attention", LastAttemptAt: new(now),
	}); err != nil {
		t.Fatalf("seed attention state: %v", err)
	}
	if _, err := fixture.shared.DB().ExecContext(ctx, "DROP TABLE instance_audit_log"); err != nil {
		t.Fatalf("break retention audit storage: %v", err)
	}

	worker := NewSearchConsoleSyncWorker(fixture.tenantMgr, &fakeSearchConsoleSource{})
	worker.now = func() time.Time { return now }
	summary, err := worker.RunDue(ctx, 10)
	if err != nil {
		t.Fatalf("retention maintenance must not stop sync scheduling: %v", err)
	}
	if summary.Attempted != 0 {
		t.Fatalf("expected attention state not to retry, got %+v", summary)
	}
}

func TestSearchConsoleSyncWorkerPropertyAccessLossKeepsImportedFacts(t *testing.T) {
	ctx := context.Background()
	fixture := newSearchConsoleWorkerFixture(t, "gsc-property-loss@test.dev", "gsc-property-loss.example.com")
	defer fixture.shared.Close()
	lastSuccess := time.Date(2026, 5, 4, 2, 0, 0, 0, time.UTC)
	importedStart := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	importedEnd := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	if err := fixture.shared.UpsertGoogleSearchConsoleSyncState(ctx, database.GoogleSearchConsoleSyncStateInput{
		SiteID:            fixture.site.ID,
		TeamID:            fixture.teamID,
		State:             "succeeded",
		ImportedStartDate: &importedStart,
		ImportedEndDate:   &importedEnd,
		LastSuccessAt:     &lastSuccess,
	}); err != nil {
		t.Fatalf("seed sync state: %v", err)
	}

	tenantStore, _, err := fixture.tenantMgr.ResolveSiteStore(ctx, fixture.site.ID)
	if err != nil {
		t.Fatalf("resolve tenant store: %v", err)
	}
	existingDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if err := tenantStore.ReplaceSearchConsoleFacts(ctx, database.SearchConsoleFactScope{
		SiteID: fixture.site.ID, PropertyURI: fixture.propertyURI, StartDate: existingDate, EndDate: existingDate, DataState: "final",
	}, []database.SearchConsoleFactInput{{
		SiteID:      fixture.site.ID,
		PropertyURI: fixture.propertyURI,
		Date:        existingDate,
		Query:       "existing query",
		Page:        "https://gsc-property-loss.example.com/",
		Country:     "USA",
		Device:      "DESKTOP",
		Clicks:      11,
		Impressions: 100,
		DataState:   "final",
		ImportedAt:  time.Now().UTC(),
	}}); err != nil {
		t.Fatalf("seed existing fact: %v", err)
	}

	source := &fakeSearchConsoleSource{err: searchconsole.ClassifiedError(searchconsole.CategoryPropertyAccessLost, errors.New("property permission denied"))}
	worker := NewSearchConsoleSyncWorker(fixture.tenantMgr, source)
	worker.now = func() time.Time { return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC) }

	if err := worker.ImportSite(ctx, fixture.site.ID); err == nil {
		t.Fatalf("expected property access loss error")
	}
	var clicks int
	if err := tenantStore.DB().QueryRowContext(ctx, "SELECT COALESCE(SUM(clicks), 0) FROM search_console_facts WHERE site_id = ?", fixture.site.ID).Scan(&clicks); err != nil {
		t.Fatalf("sum existing clicks: %v", err)
	}
	if clicks != 11 {
		t.Fatalf("expected existing imported facts to remain, got clicks=%d", clicks)
	}
	state, err := fixture.shared.GetGoogleSearchConsoleSyncState(ctx, fixture.site.ID)
	if err != nil {
		t.Fatalf("get sync state: %v", err)
	}
	if state == nil || state.State != "needs_attention" || state.LastErrorCategory != string(searchconsole.CategoryPropertyAccessLost) {
		t.Fatalf("expected property access needs-attention state, got %+v", state)
	}
	if state.LastSuccessAt == nil || !state.LastSuccessAt.Equal(lastSuccess) || state.ImportedStartDate == nil || state.ImportedEndDate == nil {
		t.Fatalf("expected failure status to preserve last successful import metadata, got %+v", state)
	}
	mapping, err := fixture.shared.GetGoogleSearchConsoleSiteMappingForTeam(ctx, fixture.site.ID, fixture.teamID)
	if err != nil {
		t.Fatalf("get mapping: %v", err)
	}
	if mapping == nil {
		t.Fatalf("expected mapping to remain after property access loss")
	}
}

func TestSearchConsoleSyncWorkerAuthFailuresNeedAttentionAndAuditSafeCategories(t *testing.T) {
	cases := []struct {
		name     string
		category searchconsole.ErrorCategory
	}{
		{name: "authorization revoked", category: searchconsole.CategoryAuthorizationRevoked},
		{name: "token refresh failed", category: searchconsole.CategoryTokenRefreshFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			fixture := newSearchConsoleWorkerFixture(t, "gsc-"+string(tc.category)+"@test.dev", "gsc-"+string(tc.category)+".example.com")
			defer fixture.shared.Close()
			source := &fakeSearchConsoleSource{err: searchconsole.ClassifiedError(tc.category, errors.New("provider auth failed"))}
			worker := NewSearchConsoleSyncWorker(fixture.tenantMgr, source)
			worker.now = func() time.Time { return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC) }

			if err := worker.ImportSite(ctx, fixture.site.ID); err == nil {
				t.Fatalf("expected auth error")
			}
			state, err := fixture.shared.GetGoogleSearchConsoleSyncState(ctx, fixture.site.ID)
			if err != nil {
				t.Fatalf("get sync state: %v", err)
			}
			if state == nil || state.State != "needs_attention" || state.LastErrorCategory != string(tc.category) || state.NextRetryAt != nil {
				t.Fatalf("expected needs-attention auth state without retry, got %+v", state)
			}
			entries, total, err := fixture.shared.ListTeamAuditEntries(ctx, fixture.teamID, "google_search_console.sync_failed", 5, 0)
			if err != nil {
				t.Fatalf("list sync failure audit: %v", err)
			}
			if total != 1 || len(entries) != 1 {
				t.Fatalf("expected one sync failure audit entry, got total=%d entries=%+v", total, entries)
			}
			if entries[0].Outcome != "failure" || !strings.Contains(entries[0].Details, "category="+string(tc.category)) || !strings.Contains(entries[0].Details, "stage=query") {
				t.Fatalf("expected safe category failure audit, got %+v", entries[0])
			}
			if !strings.Contains(entries[0].Details, `error_message="provider auth failed"`) {
				t.Fatalf("expected provider error in failure audit: %q", entries[0].Details)
			}
		})
	}
}

func TestSearchConsoleSyncLogValuesDoNotIncludeUpstreamResponseBody(t *testing.T) {
	responseBody := `{"error":{"code":403,"message":"Property access denied","status":"PERMISSION_DENIED"}}`
	err := &searchConsoleSyncError{stage: searchConsoleSyncStageQuery, err: fmt.Errorf("query failed: %w", &googleapi.Error{
		Code:    http.StatusForbidden,
		Message: "Property access denied fallback",
		Body:    responseBody,
		Errors:  []googleapi.ErrorItem{{Reason: "forbidden"}},
	})}

	values := SearchConsoleSyncLogValues(err)
	fields := make(map[string]any, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			t.Fatalf("expected string log key at %d, got %T", i, values[i])
		}
		fields[key] = values[i+1]
	}
	if fields["stage"] != searchConsoleSyncStageQuery || fields["category"] != searchconsole.CategoryPropertyAccessLost {
		t.Fatalf("unexpected structured failure fields: %+v", fields)
	}
	if fields["http_status"] != http.StatusForbidden || fields["provider_reason"] != "forbidden" {
		t.Fatalf("expected provider status and reason, got %+v", fields)
	}
	message, _ := fields["error_message"].(string)
	if message == responseBody || message != "Property access denied fallback" {
		t.Fatalf("expected safe provider message without upstream response body, got %q", message)
	}
}

func TestSearchConsoleSyncWorkerRecurringSyncRechecksRecentCompletedDays(t *testing.T) {
	ctx := context.Background()
	fixture := newSearchConsoleWorkerFixture(t, "gsc-recurring@test.dev", "gsc-recurring.example.com")
	defer fixture.shared.Close()
	lastSuccess := time.Date(2026, 5, 4, 2, 0, 0, 0, time.UTC)
	importedStart := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	importedEnd := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	if err := fixture.shared.UpsertGoogleSearchConsoleSyncState(ctx, database.GoogleSearchConsoleSyncStateInput{
		SiteID:            fixture.site.ID,
		TeamID:            fixture.teamID,
		State:             "succeeded",
		ImportedStartDate: &importedStart,
		ImportedEndDate:   &importedEnd,
		LastSuccessAt:     &lastSuccess,
	}); err != nil {
		t.Fatalf("seed sync state: %v", err)
	}

	source := &fakeSearchConsoleSource{}
	worker := NewSearchConsoleSyncWorker(fixture.tenantMgr, source)
	worker.now = func() time.Time { return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC) }

	if err := worker.ImportSite(ctx, fixture.site.ID); err != nil {
		t.Fatalf("import site: %v", err)
	}
	if len(source.queries) != 21 {
		t.Fatalf("expected three requests for each of seven daily recurring windows, got %d queries", len(source.queries))
	}
	firstQuery := source.queries[0].Query
	lastQuery := source.queries[len(source.queries)-1].Query
	if firstQuery.StartDate.Format(time.DateOnly) != "2026-05-03" || firstQuery.EndDate.Format(time.DateOnly) != "2026-05-03" ||
		lastQuery.StartDate.Format(time.DateOnly) != "2026-04-27" || lastQuery.EndDate.Format(time.DateOnly) != "2026-04-27" {
		t.Fatalf("expected newest-first daily recheck window, got first=%s to %s last=%s to %s",
			firstQuery.StartDate.Format(time.DateOnly), firstQuery.EndDate.Format(time.DateOnly),
			lastQuery.StartDate.Format(time.DateOnly), lastQuery.EndDate.Format(time.DateOnly))
	}
}

func TestSearchConsoleSyncWorkerManualSyncBackfillsAnonymizedClicks(t *testing.T) {
	ctx := context.Background()
	fixture := newSearchConsoleWorkerFixture(t, "gsc-anonymized@test.dev", "gsc-anonymized.example.com")
	defer fixture.shared.Close()
	lastSuccess := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	if err := fixture.shared.UpsertGoogleSearchConsoleSyncState(ctx, database.GoogleSearchConsoleSyncStateInput{
		SiteID: fixture.site.ID, TeamID: fixture.teamID, State: "pending", Manual: true, LastSuccessAt: &lastSuccess,
	}); err != nil {
		t.Fatalf("request manual sync: %v", err)
	}
	tenantStore, _, err := fixture.tenantMgr.ResolveSiteStore(ctx, fixture.site.ID)
	if err != nil {
		t.Fatalf("resolve tenant store: %v", err)
	}
	// Every click came from anonymized searches, so the named query has none.
	// The date is outside the seven-day recurring window.
	date := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	page := "https://gsc-anonymized.example.com/landing"
	detail := searchconsole.SearchAnalyticsRow{
		Date: date, Query: "visible query", Page: page, Country: "USA", Device: "DESKTOP",
		Clicks: 0, Impressions: 40, Position: 20, DataState: searchconsole.DataStateFinal,
	}
	if err := tenantStore.ReplaceSearchConsoleFacts(ctx, database.SearchConsoleFactScope{
		SiteID: fixture.site.ID, PropertyURI: fixture.propertyURI, StartDate: date, EndDate: date,
	}, []database.SearchConsoleFactInput{{
		Date: date, Query: detail.Query, Page: detail.Page, Country: detail.Country, Device: detail.Device,
		Clicks: detail.Clicks, Impressions: detail.Impressions, Position: detail.Position, ImportedAt: lastSuccess,
	}}); err != nil {
		t.Fatalf("seed legacy query-only import: %v", err)
	}
	total := detail
	total.Query = ""
	total.Clicks, total.Impressions, total.CTR, total.Position = 6, 100, 0.06, 12
	// Every search on this day was anonymized, so the old import stored nothing.
	anonymousOnly := total
	anonymousOnly.Date, anonymousOnly.Clicks = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), 4
	// Days before March 15 have no traffic, but still get completion checkpoints.
	source := &fakeSearchConsoleSource{
		rows:   []searchconsole.SearchAnalyticsRow{detail},
		totals: []searchconsole.SearchAnalyticsRow{total, anonymousOnly},
	}
	worker := NewSearchConsoleSyncWorker(fixture.tenantMgr, source)
	worker.now = func() time.Time { return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC) }

	if err := worker.ImportSite(ctx, fixture.site.ID); err != nil {
		t.Fatalf("manual sync: %v", err)
	}
	if len(source.queries) != 270 {
		t.Fatalf("expected the seven-day window plus 83 backfilled days, got %d queries", len(source.queries))
	}
	params := api.SearchConsoleReportParams{SiteID: fixture.site.ID, PropertyURI: fixture.propertyURI, Start: date, End: date}
	overview, err := tenantStore.GetSearchConsoleOverview(ctx, params)
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}
	if overview.Clicks != 6 || overview.Impressions != 100 || overview.CTR != 0.06 || overview.AveragePosition != 12 {
		t.Fatalf("expected query-free totals, got %+v", overview)
	}
	queries, err := tenantStore.GetSearchConsoleDimension(ctx, params, "query")
	if err != nil {
		t.Fatalf("get query report: %v", err)
	}
	if len(queries.Rows) != 1 || queries.Rows[0].Value != detail.Query || queries.Rows[0].Clicks != 0 || queries.Rows[0].Impressions != 40 {
		t.Fatalf("expected only the named query row, got %+v", queries)
	}
	anonymousDay := params
	anonymousDay.Start, anonymousDay.End = anonymousOnly.Date, anonymousOnly.Date
	if overview, err := tenantStore.GetSearchConsoleOverview(ctx, anonymousDay); err != nil || overview.Clicks != 4 {
		t.Fatalf("expected a day without named queries to gain its clicks, got overview=%+v err=%v", overview, err)
	}
	pages, err := tenantStore.GetSearchConsoleDimension(ctx, params, "page")
	if err != nil {
		t.Fatalf("get page report: %v", err)
	}
	if len(pages.Rows) != 1 || pages.Rows[0].Value != page || pages.Rows[0].Clicks != 6 {
		t.Fatalf("expected page report to keep anonymized clicks, got %+v", pages)
	}
	state, err := fixture.shared.GetGoogleSearchConsoleSyncState(ctx, fixture.site.ID)
	if err != nil {
		t.Fatalf("get sync state: %v", err)
	}
	if state == nil || state.State != "succeeded" || state.Manual || state.TotalsBackfilledPropertyURI != fixture.propertyURI {
		t.Fatalf("expected successful backfill to clear the manual request and record the property, got %+v", state)
	}

	// The backfill is recorded, so the next manual sync skips the older days,
	// including the empty ones.
	if err := fixture.shared.UpsertGoogleSearchConsoleSyncState(ctx, database.GoogleSearchConsoleSyncStateInput{
		SiteID: fixture.site.ID, TeamID: fixture.teamID, State: "pending", Manual: true, LastSuccessAt: state.LastSuccessAt,
		ImportedStartDate: state.ImportedStartDate, ImportedEndDate: state.ImportedEndDate,
	}); err != nil {
		t.Fatalf("request another manual sync: %v", err)
	}
	source.queries = nil
	if err := worker.ImportSite(ctx, fixture.site.ID); err != nil {
		t.Fatalf("second manual sync: %v", err)
	}
	if len(source.queries) != 21 {
		t.Fatalf("expected only the seven-day window after backfill, got %d queries", len(source.queries))
	}
}

func TestSearchConsoleSyncWorkerInitialSyncResumesAfterFailure(t *testing.T) {
	ctx := context.Background()
	fixture := newSearchConsoleWorkerFixture(t, "gsc-resume@test.dev", "gsc-resume.example.com")
	defer fixture.shared.Close()
	// One row of traffic on every day of the initial range, 2026-02-03 to 2026-05-03.
	var rows []searchconsole.SearchAnalyticsRow
	for day := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC); !day.After(time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)); day = day.AddDate(0, 0, 1) {
		rows = append(rows, searchconsole.SearchAnalyticsRow{
			Date: day, Query: "q", Page: "https://gsc-resume.example.com/", Country: "USA", Device: "DESKTOP",
			Clicks: 1, Impressions: 10, DataState: searchconsole.DataStateFinal,
		})
	}
	source := &fakeSearchConsoleSource{
		rows:      rows,
		errOnDate: map[string]error{"2026-04-01": searchconsole.ClassifiedError(searchconsole.CategoryGoogleUnavailable, errors.New("unavailable"))},
	}
	worker := NewSearchConsoleSyncWorker(fixture.tenantMgr, source)
	worker.now = func() time.Time { return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC) }

	if err := worker.ImportSite(ctx, fixture.site.ID); err == nil {
		t.Fatal("expected the first sync to fail on 2026-04-01")
	}
	// 2026-05-03 back to 2026-04-02 committed before the failure.
	source.errOnDate = nil
	source.queries = nil
	if err := worker.ImportSite(ctx, fixture.site.ID); err != nil {
		t.Fatalf("retry first sync: %v", err)
	}
	// The retry fetches the seven recent days and the 58 older days that never
	// committed, not the 25 older days that did.
	if len(source.queries) != 65*3 {
		t.Fatalf("expected the retry to skip committed days, got %d queries", len(source.queries))
	}
	requireSearchConsoleImportedClicks(t, fixture.tenantMgr, fixture.site.ID, 90)
	requireSearchConsoleSucceededState(t, fixture.shared, fixture.site.ID, "2026-02-03", "2026-05-03")
}

func TestSearchConsoleSyncWorkerInitialSyncResumesEmptyDaysAfterFailure(t *testing.T) {
	ctx := t.Context()
	fixture := newSearchConsoleWorkerFixture(t, "gsc-empty-resume@test.dev", "gsc-empty-resume.example.com")
	defer fixture.shared.Close()
	source := &fakeSearchConsoleSource{
		rows: []searchconsole.SearchAnalyticsRow{{
			Date: time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC), Query: "q",
			Page: "https://gsc-empty-resume.example.com/", Country: "USA", Device: "DESKTOP",
			Clicks: 1, Impressions: 10, DataState: searchconsole.DataStateFinal,
		}},
		errOnDate: map[string]error{"2026-04-01": context.DeadlineExceeded},
	}
	worker := NewSearchConsoleSyncWorker(fixture.tenantMgr, source)
	worker.now = func() time.Time { return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC) }
	if err := worker.ImportSite(ctx, fixture.site.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected first sync to time out, got %v", err)
	}

	// Empty days through April 2 committed before the timeout. Retrying one
	// of those older days must not prevent reaching the oldest day's traffic.
	source.errOnDate = map[string]error{"2026-04-02": errors.New("completed empty day was fetched again")}
	source.queries = nil
	worker = NewSearchConsoleSyncWorker(fixture.tenantMgr, source)
	worker.now = func() time.Time { return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC) }
	if err := worker.ImportSite(ctx, fixture.site.ID); err != nil {
		t.Fatalf("resume past completed empty days: %v", err)
	}
	if len(source.queries) != 65*3 {
		t.Fatalf("expected seven recent days plus 58 unfinished days, got %d queries", len(source.queries))
	}
	requireSearchConsoleImportedClicks(t, fixture.tenantMgr, fixture.site.ID, 1)
	requireSearchConsoleSucceededState(t, fixture.shared, fixture.site.ID, "2026-02-03", "2026-05-03")
}

func TestSearchConsoleSyncWorkerRunDueImportsReadySitesAndContinuesAfterFailure(t *testing.T) {
	ctx := context.Background()
	first := newSearchConsoleWorkerFixture(t, "gsc-run-due-one@test.dev", "gsc-run-due-one.example.com")
	defer first.shared.Close()
	second, err := addSearchConsoleWorkerFixtureSite(t, first.shared, first.tenantMgr, "gsc-run-due-two@test.dev", "gsc-run-due-two.example.com")
	if err != nil {
		t.Fatalf("add second fixture site: %v", err)
	}
	failing, err := addSearchConsoleWorkerFixtureSite(t, first.shared, first.tenantMgr, "gsc-run-due-failing@test.dev", "gsc-run-due-failing.example.com")
	if err != nil {
		t.Fatalf("add failing fixture site: %v", err)
	}
	futureRetry := time.Date(2026, 5, 5, 18, 0, 0, 0, time.UTC)
	if err := first.shared.UpsertGoogleSearchConsoleSyncState(ctx, database.GoogleSearchConsoleSyncStateInput{
		SiteID:            second.site.ID,
		TeamID:            second.teamID,
		State:             "failed",
		LastErrorCategory: "quota_limited",
		NextRetryAt:       &futureRetry,
	}); err != nil {
		t.Fatalf("seed future retry state: %v", err)
	}
	if err := first.shared.UpsertGoogleSearchConsoleSyncState(ctx, database.GoogleSearchConsoleSyncStateInput{
		SiteID: failing.site.ID,
		TeamID: failing.teamID,
		State:  "pending",
		Manual: true,
	}); err != nil {
		t.Fatalf("seed failing pending state: %v", err)
	}

	source := &fakeSearchConsoleSource{
		rowsBySiteURL: map[string][]searchconsole.SearchAnalyticsRow{
			first.propertyURI: {
				{
					Date:        time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
					Query:       "ready site",
					Page:        "https://gsc-run-due-one.example.com/",
					Country:     "USA",
					Device:      "DESKTOP",
					Clicks:      4,
					Impressions: 40,
					DataState:   searchconsole.DataStateFinal,
				},
			},
		},
		errBySiteURL: map[string]error{
			failing.propertyURI: searchconsole.ClassifiedError(searchconsole.CategoryGoogleUnavailable, errors.New("temporary Google outage")),
		},
	}
	worker := NewSearchConsoleSyncWorker(first.tenantMgr, source)
	worker.now = func() time.Time { return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC) }

	summary, err := worker.RunDue(ctx, 25)
	if err != nil {
		t.Fatalf("run due: %v", err)
	}
	if summary.Attempted != 2 || summary.Succeeded != 1 || summary.Failed != 1 {
		t.Fatalf("unexpected run summary: %+v", summary)
	}
	queried := map[string]bool{}
	for _, query := range source.queries {
		queried[query.Query.SiteURL] = true
	}
	if !queried[first.propertyURI] || !queried[failing.propertyURI] || queried[second.propertyURI] {
		t.Fatalf("expected due sites to be queried and future retry skipped, got %+v", source.queries)
	}
	state, err := first.shared.GetGoogleSearchConsoleSyncState(ctx, first.site.ID)
	if err != nil {
		t.Fatalf("get first sync state: %v", err)
	}
	if state == nil || state.State != "succeeded" {
		t.Fatalf("expected first due site to sync, got %+v", state)
	}
	secondState, err := first.shared.GetGoogleSearchConsoleSyncState(ctx, second.site.ID)
	if err != nil {
		t.Fatalf("get second sync state: %v", err)
	}
	if secondState == nil || secondState.NextRetryAt == nil || !secondState.NextRetryAt.Equal(futureRetry) {
		t.Fatalf("expected future retry site to be skipped unchanged, got %+v", secondState)
	}
	failingState, err := first.shared.GetGoogleSearchConsoleSyncState(ctx, failing.site.ID)
	if err != nil {
		t.Fatalf("get failing sync state: %v", err)
	}
	if failingState == nil || failingState.State != "failed" || failingState.LastErrorCategory != string(searchconsole.CategoryGoogleUnavailable) {
		t.Fatalf("expected failing due site to record safe failure state, got %+v", failingState)
	}
}

type searchConsoleWorkerFixture struct {
	shared      *database.Store
	tenantMgr   *database.TenantStoreManager
	site        *api.Site
	teamID      uuid.UUID
	propertyURI string
}

func newSearchConsoleWorkerFixture(t *testing.T, email string, domain string) searchConsoleWorkerFixture {
	t.Helper()
	ctx := context.Background()
	shared := newTestStore(t)
	tenantMgr := newTestTenantMgr(t, shared)
	userID, err := shared.CreateUser(ctx, email, "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := shared.CreateSite(ctx, userID, domain)
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	teamID, err := shared.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get site team: %v", err)
	}
	propertyURI := "sc-domain:" + domain
	if err := shared.UpsertGoogleSearchConsoleConnection(ctx, database.GoogleSearchConsoleConnectionInput{
		TeamID:       teamID,
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		Scope:        searchconsole.ReadOnlyScope,
		TokenExpiry:  time.Now().UTC().Add(time.Hour),
		ConnectedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if err := shared.UpsertGoogleSearchConsoleSiteMapping(ctx, database.GoogleSearchConsoleSiteMappingInput{
		SiteID:      site.ID,
		TeamID:      teamID,
		PropertyURI: propertyURI,
		MappedBy:    userID,
		MappedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed mapping: %v", err)
	}
	return searchConsoleWorkerFixture{
		shared:      shared,
		tenantMgr:   tenantMgr,
		site:        site,
		teamID:      teamID,
		propertyURI: propertyURI,
	}
}

func addSearchConsoleWorkerFixtureSite(t *testing.T, shared *database.Store, tenantMgr *database.TenantStoreManager, email string, domain string) (searchConsoleWorkerFixture, error) {
	t.Helper()
	ctx := context.Background()
	userID, err := shared.CreateUser(ctx, email, "hash")
	if err != nil {
		return searchConsoleWorkerFixture{}, err
	}
	team, err := shared.CreateTenant(ctx, userID, domain+" Team", "")
	if err != nil {
		return searchConsoleWorkerFixture{}, err
	}
	if err := shared.SetActiveTenantID(ctx, userID, team.ID); err != nil {
		return searchConsoleWorkerFixture{}, err
	}
	site, err := shared.CreateSite(ctx, userID, domain)
	if err != nil {
		return searchConsoleWorkerFixture{}, err
	}
	propertyURI := "sc-domain:" + domain
	if err := shared.UpsertGoogleSearchConsoleConnection(ctx, database.GoogleSearchConsoleConnectionInput{
		TeamID:       team.ID,
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		Scope:        searchconsole.ReadOnlyScope,
		TokenExpiry:  time.Now().UTC().Add(time.Hour),
		ConnectedAt:  time.Now().UTC(),
	}); err != nil {
		return searchConsoleWorkerFixture{}, err
	}
	if err := shared.UpsertGoogleSearchConsoleSiteMapping(ctx, database.GoogleSearchConsoleSiteMappingInput{
		SiteID:      site.ID,
		TeamID:      team.ID,
		PropertyURI: propertyURI,
		MappedBy:    userID,
		MappedAt:    time.Now().UTC(),
	}); err != nil {
		return searchConsoleWorkerFixture{}, err
	}
	if _, _, err := tenantMgr.ResolveSiteStore(ctx, site.ID); err != nil {
		return searchConsoleWorkerFixture{}, err
	}
	return searchConsoleWorkerFixture{
		shared:      shared,
		tenantMgr:   tenantMgr,
		site:        site,
		teamID:      team.ID,
		propertyURI: propertyURI,
	}, nil
}

func initialSearchConsoleRows() []searchconsole.SearchAnalyticsRow {
	return []searchconsole.SearchAnalyticsRow{
		{
			Date:        time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			Query:       "hitkeep analytics",
			Page:        "https://gsc-worker.example.com/",
			Country:     "USA",
			Device:      "DESKTOP",
			Clicks:      7,
			Impressions: 70,
			CTR:         0.1,
			Position:    2.4,
			DataState:   "final",
		},
		{
			Date:        time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
			Query:       "privacy analytics",
			Page:        "https://gsc-worker.example.com/privacy",
			Country:     "USA",
			Device:      "MOBILE",
			Clicks:      3,
			Impressions: 20,
			CTR:         0.15,
			Position:    4.2,
			DataState:   "final",
		},
	}
}

func requireInitialSearchConsoleQuery(t *testing.T, source *fakeSearchConsoleSource, propertyURI string) {
	t.Helper()
	if len(source.queries) != 270 {
		t.Fatalf("expected three requests for each of 90 daily windows, got %d queries", len(source.queries))
	}
	// Each day's requests run concurrently, so they may arrive in any order.
	var dimensionSets []string
	for _, q := range source.queries[:3] {
		dimensionSets = append(dimensionSets, strings.Join(q.Query.Dimensions, ","))
	}
	slices.Sort(dimensionSets)
	if got := strings.Join(dimensionSets, " | "); got != "date,country,device | date,page | date,query,page,country,device" {
		t.Fatalf("expected query rows plus page and audience totals, got %s", got)
	}
	query := source.queries[0]
	if query.Token.RefreshToken != "refresh-token" {
		t.Fatalf("expected worker to use stored team token, got %+v", query.Token)
	}
	if query.Query.SiteURL != propertyURI {
		t.Fatalf("expected mapped property URI %q, got %q", propertyURI, query.Query.SiteURL)
	}
	lastQuery := source.queries[len(source.queries)-1].Query
	if query.Query.StartDate.Format(time.DateOnly) != "2026-05-03" || query.Query.EndDate.Format(time.DateOnly) != "2026-05-03" ||
		lastQuery.StartDate.Format(time.DateOnly) != "2026-02-03" || lastQuery.EndDate.Format(time.DateOnly) != "2026-02-03" {
		t.Fatalf("expected newest-first finalized daily windows, got first=%s to %s last=%s to %s",
			query.Query.StartDate.Format(time.DateOnly), query.Query.EndDate.Format(time.DateOnly),
			lastQuery.StartDate.Format(time.DateOnly), lastQuery.EndDate.Format(time.DateOnly))
	}
	if query.Query.DataState != searchconsole.DataStateFinal {
		t.Fatalf("expected final data state, got %q", query.Query.DataState)
	}
}

func requireSearchConsoleImportedClicks(t *testing.T, tenantMgr *database.TenantStoreManager, siteID uuid.UUID, expected int) {
	t.Helper()
	ctx := context.Background()
	tenantStore, _, err := tenantMgr.ResolveSiteStore(ctx, siteID)
	if err != nil {
		t.Fatalf("resolve tenant store: %v", err)
	}
	mapping, err := tenantMgr.Shared().GetGoogleSearchConsoleSiteMapping(ctx, siteID)
	if err != nil {
		t.Fatalf("get mapping: %v", err)
	}
	overview, err := tenantStore.GetSearchConsoleOverview(ctx, api.SearchConsoleReportParams{
		SiteID: siteID, PropertyURI: mapping.PropertyURI,
		Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("get imported overview: %v", err)
	}
	if overview.Clicks != expected {
		t.Fatalf("expected imported clicks=%d, got %d", expected, overview.Clicks)
	}
}

func requireSearchConsoleSucceededState(t *testing.T, shared *database.Store, siteID uuid.UUID, startDate, endDate string) {
	t.Helper()
	state, err := shared.GetGoogleSearchConsoleSyncState(context.Background(), siteID)
	if err != nil {
		t.Fatalf("get sync state: %v", err)
	}
	if state == nil || state.State != "succeeded" || state.LastSuccessAt == nil {
		t.Fatalf("expected succeeded sync state, got %+v", state)
	}
	if state.ImportedStartDate == nil || state.ImportedStartDate.Format(time.DateOnly) != startDate {
		t.Fatalf("expected imported start date %s, got %+v", startDate, state.ImportedStartDate)
	}
	if state.ImportedEndDate == nil || state.ImportedEndDate.Format(time.DateOnly) != endDate {
		t.Fatalf("expected imported end date %s, got %+v", endDate, state.ImportedEndDate)
	}
	if state.LastErrorCategory != "" || state.NextRetryAt != nil {
		t.Fatalf("expected successful sync to clear error status, got category=%q retry=%+v", state.LastErrorCategory, state.NextRetryAt)
	}
}

func requireSearchConsoleImportAudit(t *testing.T, shared *database.Store, teamID, siteID uuid.UUID) {
	t.Helper()
	entries, total, err := shared.ListTeamAuditEntries(context.Background(), teamID, "google_search_console.sync_imported", 5, 0)
	if err != nil {
		t.Fatalf("list sync import audit: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one sync import audit entry, got total=%d entries=%+v", total, entries)
	}
	if entries[0].Outcome != "success" || entries[0].TargetType != "site" || entries[0].TargetID != siteID.String() {
		t.Fatalf("unexpected sync import audit entry: %+v", entries[0])
	}
	if !strings.Contains(entries[0].Details, "imported_rows=6") || strings.Contains(entries[0].Details, "hitkeep analytics") || strings.Contains(entries[0].Details, "privacy analytics") {
		t.Fatalf("expected aggregate import audit without query payloads, got %q", entries[0].Details)
	}
}

func requireSearchConsolePreparedAudit(t *testing.T, shared *database.Store, teamID, siteID uuid.UUID) {
	t.Helper()
	entries, total, err := shared.ListTeamAuditEntries(context.Background(), teamID, "google_search_console.sync_import_prepared", 5, 0)
	if err != nil {
		t.Fatalf("list sync import prepared audit: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one sync import prepared audit entry, got total=%d entries=%+v", total, entries)
	}
	if entries[0].Outcome != "success" || entries[0].TargetType != "site" || entries[0].TargetID != siteID.String() {
		t.Fatalf("unexpected sync import prepared audit entry: %+v", entries[0])
	}
	if !strings.Contains(entries[0].Details, "prepared_rows=6") || strings.Contains(entries[0].Details, "hitkeep analytics") || strings.Contains(entries[0].Details, "privacy analytics") {
		t.Fatalf("expected aggregate prepared audit without query payloads, got %q", entries[0].Details)
	}
}

func requireSearchConsoleStartAudit(t *testing.T, shared *database.Store, teamID, siteID uuid.UUID) {
	t.Helper()
	entries, total, err := shared.ListTeamAuditEntries(context.Background(), teamID, "google_search_console.sync_started", 5, 0)
	if err != nil {
		t.Fatalf("list sync start audit: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one sync start audit entry, got total=%d entries=%+v", total, entries)
	}
	if entries[0].Outcome != "success" || entries[0].TargetType != "site" || entries[0].TargetID != siteID.String() {
		t.Fatalf("unexpected sync start audit entry: %+v", entries[0])
	}
	if strings.Contains(entries[0].Details, "access-token") || strings.Contains(entries[0].Details, "refresh-token") {
		t.Fatalf("sync start audit leaked token material: %q", entries[0].Details)
	}
}

type fakeSearchConsoleQuery struct {
	Token searchconsole.Token
	Query searchconsole.SearchAnalyticsQuery
}

func TestSearchConsoleSyncWorkerStartStopsBlockedSyncWhenContextCanceled(t *testing.T) {
	fixture := newSearchConsoleWorkerFixture(t, "gsc-worker-cancel@test.dev", "gsc-worker-cancel.example.com")
	source := &blockingSearchConsoleSource{started: make(chan struct{}, 1)}
	worker := NewSearchConsoleSyncWorker(fixture.tenantMgr, source)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()

	select {
	case <-source.started:
		cancel()
	case <-time.After(10 * time.Second):
		t.Fatal("blocked Search Console sync did not start")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Search Console worker did not stop after its parent context was canceled")
	}
}

type blockingSearchConsoleSource struct {
	fakeSearchConsoleSource
	started chan struct{}
}

func (s *blockingSearchConsoleSource) QuerySearchAnalytics(ctx context.Context, token searchconsole.Token, query searchconsole.SearchAnalyticsQuery) ([]searchconsole.SearchAnalyticsRow, error) {
	select {
	case s.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

type fakeSearchConsoleSource struct {
	rows          []searchconsole.SearchAnalyticsRow
	totals        []searchconsole.SearchAnalyticsRow // rows for query-free requests; defaults to rows
	rowsBySiteURL map[string][]searchconsole.SearchAnalyticsRow
	errBySiteURL  map[string]error
	err           error
	queryErr      error // fails only query-grouped requests
	errOnDate     map[string]error
	queriesMu     sync.Mutex
	queries       []fakeSearchConsoleQuery
}

func (f *fakeSearchConsoleSource) AuthCodeURL(state, redirectURL string) (string, error) {
	return "", nil
}

func (f *fakeSearchConsoleSource) ExchangeCode(ctx context.Context, code, redirectURL string) (searchconsole.Token, error) {
	return searchconsole.Token{}, nil
}

func (f *fakeSearchConsoleSource) ListProperties(ctx context.Context, token searchconsole.Token) ([]searchconsole.Property, error) {
	return nil, nil
}

func (f *fakeSearchConsoleSource) QuerySearchAnalytics(ctx context.Context, token searchconsole.Token, query searchconsole.SearchAnalyticsQuery) ([]searchconsole.SearchAnalyticsRow, error) {
	f.queriesMu.Lock()
	f.queries = append(f.queries, fakeSearchConsoleQuery{Token: token, Query: query})
	f.queriesMu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if f.errBySiteURL != nil && f.errBySiteURL[query.SiteURL] != nil {
		return nil, f.errBySiteURL[query.SiteURL]
	}
	if err := f.errOnDate[query.StartDate.Format(time.DateOnly)]; err != nil {
		return nil, err
	}
	byQuery := slices.Contains(query.Dimensions, "query")
	if byQuery && f.queryErr != nil {
		return nil, f.queryErr
	}
	rows := f.rows
	if f.rowsBySiteURL != nil {
		rows = f.rowsBySiteURL[query.SiteURL]
	}
	if !byQuery && f.totals != nil {
		rows = f.totals
	}
	filtered := searchConsoleRowsInQueryRange(rows, query)
	for i := range filtered {
		clearUngroupedSearchConsoleDimensions(&filtered[i], query.Dimensions)
	}
	return filtered, nil
}

// clearUngroupedSearchConsoleDimensions mirrors Google, which leaves out the
// dimensions a request does not group by.
func clearUngroupedSearchConsoleDimensions(row *searchconsole.SearchAnalyticsRow, dimensions []string) {
	if !slices.Contains(dimensions, "query") {
		row.Query = ""
	}
	if !slices.Contains(dimensions, "page") {
		row.Page = ""
	}
	if !slices.Contains(dimensions, "country") {
		row.Country = ""
	}
	if !slices.Contains(dimensions, "device") {
		row.Device = ""
	}
}

func searchConsoleRowsInQueryRange(rows []searchconsole.SearchAnalyticsRow, query searchconsole.SearchAnalyticsQuery) []searchconsole.SearchAnalyticsRow {
	filtered := make([]searchconsole.SearchAnalyticsRow, 0, len(rows))
	for _, row := range rows {
		if row.Date.Before(query.StartDate) || row.Date.After(query.EndDate) {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}
