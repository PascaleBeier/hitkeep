package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/searchconsole"
)

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
	requireSearchConsoleStartAudit(t, shared, teamID, site.ID)
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
	if err := tenantStore.UpsertSearchConsoleFact(ctx, database.SearchConsoleFactInput{
		SiteID:      fixture.site.ID,
		PropertyURI: fixture.propertyURI,
		Date:        time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Query:       "existing query",
		Page:        "https://gsc-property-loss.example.com/",
		Country:     "USA",
		Device:      "DESKTOP",
		Clicks:      11,
		Impressions: 100,
		DataState:   "final",
		ImportedAt:  time.Now().UTC(),
	}); err != nil {
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
			source := &fakeSearchConsoleSource{err: searchconsole.ClassifiedError(tc.category, errors.New("safe classified auth error"))}
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
			if entries[0].Outcome != "failure" || !strings.Contains(entries[0].Details, "category="+string(tc.category)) {
				t.Fatalf("expected safe category failure audit, got %+v", entries[0])
			}
			if strings.Contains(entries[0].Details, "access-token") || strings.Contains(entries[0].Details, "refresh-token") || strings.Contains(entries[0].Details, "safe classified auth error") {
				t.Fatalf("failure audit leaked token or raw provider error details: %q", entries[0].Details)
			}
		})
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
	if len(source.queries) != 1 {
		t.Fatalf("expected one recurring query, got %+v", source.queries)
	}
	query := source.queries[0].Query
	if query.StartDate.Format(time.DateOnly) != "2026-04-27" || query.EndDate.Format(time.DateOnly) != "2026-05-03" {
		t.Fatalf("expected recent completed recheck window, got %s to %s", query.StartDate.Format(time.DateOnly), query.EndDate.Format(time.DateOnly))
	}
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
	if len(source.queries) != 2 || !queried[first.propertyURI] || !queried[failing.propertyURI] || queried[second.propertyURI] {
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
	if len(source.queries) != 1 {
		t.Fatalf("expected one Search Analytics query, got %+v", source.queries)
	}
	query := source.queries[0]
	if query.Token.RefreshToken != "refresh-token" {
		t.Fatalf("expected worker to use stored team token, got %+v", query.Token)
	}
	if query.Query.SiteURL != propertyURI {
		t.Fatalf("expected mapped property URI %q, got %q", propertyURI, query.Query.SiteURL)
	}
	if query.Query.StartDate.Format(time.DateOnly) != "2026-02-03" || query.Query.EndDate.Format(time.DateOnly) != "2026-05-03" {
		t.Fatalf("expected recent finalized 90 day window, got %s to %s", query.Query.StartDate.Format(time.DateOnly), query.Query.EndDate.Format(time.DateOnly))
	}
	if query.Query.DataState != searchconsole.DataStateFinal {
		t.Fatalf("expected final data state, got %q", query.Query.DataState)
	}
	if strings.Join(query.Query.Dimensions, ",") != "date,query,page,country,device" {
		t.Fatalf("unexpected dimensions: %+v", query.Query.Dimensions)
	}
}

func requireSearchConsoleImportedClicks(t *testing.T, tenantMgr *database.TenantStoreManager, siteID uuid.UUID, expected int) {
	t.Helper()
	ctx := context.Background()
	tenantStore, _, err := tenantMgr.ResolveSiteStore(ctx, siteID)
	if err != nil {
		t.Fatalf("resolve tenant store: %v", err)
	}
	var clicks int
	if err := tenantStore.DB().QueryRowContext(ctx, "SELECT COALESCE(SUM(clicks), 0) FROM search_console_facts WHERE site_id = ?", siteID).Scan(&clicks); err != nil {
		t.Fatalf("sum clicks: %v", err)
	}
	if clicks != expected {
		t.Fatalf("expected imported clicks=%d, got %d", expected, clicks)
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
	if !strings.Contains(entries[0].Details, "imported_rows=2") || strings.Contains(entries[0].Details, "hitkeep analytics") || strings.Contains(entries[0].Details, "privacy analytics") {
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
	if !strings.Contains(entries[0].Details, "prepared_rows=2") || strings.Contains(entries[0].Details, "hitkeep analytics") || strings.Contains(entries[0].Details, "privacy analytics") {
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

type fakeSearchConsoleSource struct {
	rows          []searchconsole.SearchAnalyticsRow
	rowsBySiteURL map[string][]searchconsole.SearchAnalyticsRow
	errBySiteURL  map[string]error
	err           error
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
	f.queries = append(f.queries, fakeSearchConsoleQuery{Token: token, Query: query})
	if f.err != nil {
		return nil, f.err
	}
	if f.errBySiteURL != nil && f.errBySiteURL[query.SiteURL] != nil {
		return nil, f.errBySiteURL[query.SiteURL]
	}
	if f.rowsBySiteURL != nil {
		return f.rowsBySiteURL[query.SiteURL], nil
	}
	return f.rows, nil
}
