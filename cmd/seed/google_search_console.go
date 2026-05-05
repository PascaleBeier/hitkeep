package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/searchconsole"
)

type searchConsoleSeedStats struct {
	facts int
	sites int
}

type googleSearchConsoleFixture struct {
	domain   string
	property string
	state    string
	category string
	manual   bool
	mapped   bool
}

func seedGoogleSearchConsoleFixtures(ctx context.Context, store *database.Store, tenantMgr *database.TenantStoreManager, userID, primarySiteID uuid.UUID, days int) searchConsoleSeedStats {
	teamID, err := store.GetSiteTenantID(ctx, primarySiteID)
	if err != nil {
		slog.Warn("Skipping Search Console fixtures; primary site tenant unavailable", "site_id", primarySiteID, "error", err)
		return searchConsoleSeedStats{}
	}

	now := time.Now().UTC().Truncate(time.Minute)
	if !seedGoogleSearchConsoleConnection(ctx, store, teamID, userID, now) {
		return searchConsoleSeedStats{}
	}

	stats := seedPrimaryGoogleSearchConsoleFixture(ctx, store, tenantMgr, primarySiteID, teamID, userID, days, now)
	statusStats := seedGoogleSearchConsoleStatusFixtures(ctx, store, tenantMgr, teamID, userID, now)
	stats.sites += statusStats.sites
	stats.facts += statusStats.facts

	slog.Info("Google Search Console fixtures seeded", "sites", stats.sites, "facts", stats.facts)
	return stats
}

func seedGoogleSearchConsoleConnection(ctx context.Context, store *database.Store, teamID, userID uuid.UUID, now time.Time) bool {
	// #nosec G101 -- these inert demo placeholders are deliberately unusable and keep seed data offline.
	if err := store.UpsertGoogleSearchConsoleConnectionWithAudit(ctx, database.GoogleSearchConsoleConnectionInput{
		TeamID:             teamID,
		ConnectedByUserID:  userID,
		GoogleAccountEmail: "demo-search-console@example.com",
		GoogleAccountID:    "demo-search-console-account",
		AccessToken:        "seeded-demo-access-token",
		RefreshToken:       "seeded-demo-refresh-token",
		TokenType:          "Bearer",
		Scope:              searchconsole.ReadOnlyScope,
		TokenExpiry:        now.AddDate(1, 0, 0),
		ConnectedAt:        now.AddDate(0, 0, -14),
	}, database.AuditEntryParams{
		ActorID:    userID,
		TeamID:     teamID,
		Action:     "google_search_console.connected",
		TargetType: "team",
		TargetID:   teamID.String(),
		Outcome:    "success",
		Details:    "outcome=connected;google_account_email=demo-search-console@example.com;seed_fixture=true",
	}); err != nil {
		slog.Warn("Failed to seed Search Console connection", "team_id", teamID, "error", err)
		return false
	}
	return true
}

func seedPrimaryGoogleSearchConsoleFixture(ctx context.Context, store *database.Store, tenantMgr *database.TenantStoreManager, primarySiteID, teamID, userID uuid.UUID, days int, now time.Time) searchConsoleSeedStats {
	stats := searchConsoleSeedStats{}
	primaryProperty := "sc-domain:acme-analytics.io"
	if err := seedGoogleSearchConsoleProperty(ctx, store, teamID, primaryProperty, now); err != nil {
		slog.Warn("Failed to seed primary Search Console property", "property_uri", primaryProperty, "error", err)
		return stats
	}
	if err := seedGoogleSearchConsoleMappedSite(ctx, store, primarySiteID, teamID, primaryProperty, userID, now); err != nil {
		slog.Warn("Failed to seed primary Search Console mapping", "site_id", primarySiteID, "error", err)
		return stats
	}
	stats.sites++
	stats.facts += seedGoogleSearchConsoleFacts(ctx, tenantMgr, primarySiteID, teamID, primaryProperty, userID, days, now)
	seedGoogleSearchConsoleSyncState(ctx, store, primarySiteID, teamID, userID, "succeeded", "", false, now)

	return stats
}

func seedGoogleSearchConsoleStatusFixtures(ctx context.Context, store *database.Store, tenantMgr *database.TenantStoreManager, teamID, userID uuid.UUID, now time.Time) searchConsoleSeedStats {
	stats := searchConsoleSeedStats{}
	for _, fixture := range googleSearchConsoleStatusFixtures() {
		if seedGoogleSearchConsoleStatusFixture(ctx, store, tenantMgr, teamID, userID, now, fixture) {
			stats.sites++
		}
	}
	return stats
}

func googleSearchConsoleStatusFixtures() []googleSearchConsoleFixture {
	return []googleSearchConsoleFixture{
		{domain: "search-pending.example.com", property: "sc-domain:search-pending.example.com", state: "pending", manual: true, mapped: true},
		{domain: "search-quota.example.com", property: "sc-domain:search-quota.example.com", state: "failed", category: string(searchconsole.CategoryQuotaLimited), mapped: true},
		{domain: "search-reconnect.example.com", property: "sc-domain:search-reconnect.example.com", state: "needs_attention", category: string(searchconsole.CategoryAuthorizationRevoked), mapped: true},
		{domain: "search-unmapped.example.com", property: "sc-domain:search-unmapped.example.com", mapped: false},
	}
}

func seedGoogleSearchConsoleStatusFixture(ctx context.Context, store *database.Store, tenantMgr *database.TenantStoreManager, teamID, userID uuid.UUID, now time.Time, fixture googleSearchConsoleFixture) bool {
	site, ok := ensureGoogleSearchConsoleFixtureSite(ctx, store, tenantMgr, userID, fixture.domain)
	if !ok {
		return false
	}
	if err := seedGoogleSearchConsoleProperty(ctx, store, teamID, fixture.property, now); err != nil {
		slog.Warn("Failed to seed Search Console fixture property", "property_uri", fixture.property, "error", err)
		return false
	}
	return seedGoogleSearchConsoleFixtureMappingIfNeeded(ctx, store, site.ID, teamID, userID, now, fixture)
}

func ensureGoogleSearchConsoleFixtureSite(ctx context.Context, store *database.Store, tenantMgr *database.TenantStoreManager, userID uuid.UUID, domain string) (*api.Site, bool) {
	site, err := ensureSiteInActiveTeam(ctx, store, userID, domain)
	if err != nil {
		slog.Warn("Failed to ensure Search Console fixture site", "domain", domain, "error", err)
		return nil, false
	}
	if err := tenantMgr.SyncSite(ctx, site.ID); err != nil {
		slog.Warn("Failed to sync Search Console fixture site", "domain", domain, "site_id", site.ID, "error", err)
		return nil, false
	}
	return site, true
}

func seedGoogleSearchConsoleMappedFixture(ctx context.Context, store *database.Store, siteID, teamID, userID uuid.UUID, now time.Time, fixture googleSearchConsoleFixture) bool {
	if err := seedGoogleSearchConsoleMappedSite(ctx, store, siteID, teamID, fixture.property, userID, now); err != nil {
		slog.Warn("Failed to seed Search Console fixture mapping", "domain", fixture.domain, "error", err)
		return false
	}
	seedGoogleSearchConsoleSyncState(ctx, store, siteID, teamID, userID, fixture.state, fixture.category, fixture.manual, now)
	return true
}

func seedGoogleSearchConsoleFixtureMappingIfNeeded(ctx context.Context, store *database.Store, siteID, teamID, userID uuid.UUID, now time.Time, fixture googleSearchConsoleFixture) bool {
	if fixture.mapped {
		return seedGoogleSearchConsoleMappedFixture(ctx, store, siteID, teamID, userID, now, fixture)
	}
	return true
}

func seedGoogleSearchConsoleProperty(ctx context.Context, store *database.Store, teamID uuid.UUID, propertyURI string, seenAt time.Time) error {
	return store.UpsertGoogleSearchConsoleProperty(ctx, database.GoogleSearchConsolePropertyInput{
		TeamID:          teamID,
		URI:             propertyURI,
		PermissionLevel: "siteOwner",
		SeenAt:          seenAt,
	})
}

func seedGoogleSearchConsoleMappedSite(ctx context.Context, store *database.Store, siteID, teamID uuid.UUID, propertyURI string, userID uuid.UUID, mappedAt time.Time) error {
	siteLabel := siteID.String()
	if site, err := store.GetSiteByID(ctx, siteID); err == nil && site != nil {
		siteLabel = site.Domain
	}
	return store.UpsertGoogleSearchConsoleSiteMappingWithAudit(ctx, database.GoogleSearchConsoleSiteMappingInput{
		SiteID:      siteID,
		TeamID:      teamID,
		PropertyURI: propertyURI,
		MappedBy:    userID,
		MappedAt:    mappedAt,
	}, database.AuditEntryParams{
		ActorID:     userID,
		TeamID:      teamID,
		Action:      "google_search_console.property_mapped",
		TargetType:  "site",
		TargetID:    siteID.String(),
		TargetLabel: siteLabel,
		Outcome:     "success",
		Details:     fmt.Sprintf("old_property_uri=;new_property_uri=%s;seed_fixture=true", propertyURI),
	})
}

func seedGoogleSearchConsoleFacts(ctx context.Context, tenantMgr *database.TenantStoreManager, siteID, teamID uuid.UUID, propertyURI string, userID uuid.UUID, days int, now time.Time) int {
	tenantStore, _, err := tenantMgr.ResolveSiteStore(ctx, siteID)
	if err != nil {
		slog.Warn("Failed to resolve tenant store for Search Console facts", "site_id", siteID, "error", err)
		return 0
	}
	if !prepareGoogleSearchConsoleFactSeed(ctx, tenantMgr.Shared(), tenantStore, siteID, teamID, propertyURI, userID) {
		return 0
	}
	rows := buildGoogleSearchConsoleFactInputs(siteID, propertyURI, days, now)
	return upsertGoogleSearchConsoleSeedFacts(ctx, tenantStore, siteID, rows)
}

func prepareGoogleSearchConsoleFactSeed(ctx context.Context, shared *database.Store, tenantStore *database.Store, siteID, teamID uuid.UUID, propertyURI string, userID uuid.UUID) bool {
	if err := auditSeededGoogleSearchConsoleFacts(ctx, shared, siteID, teamID, propertyURI, userID); err != nil {
		slog.Warn("Failed to audit seeded Search Console fact refresh", "site_id", siteID, "error", err)
		return false
	}
	if _, err := tenantStore.DB().ExecContext(ctx, "DELETE FROM search_console_facts WHERE site_id = ?", siteID); err != nil {
		slog.Warn("Failed to reset seeded Search Console facts", "site_id", siteID, "error", err)
		return false
	}
	return true
}

func upsertGoogleSearchConsoleSeedFacts(ctx context.Context, tenantStore *database.Store, siteID uuid.UUID, rows []database.SearchConsoleFactInput) int {
	if err := tenantStore.UpsertSearchConsoleFacts(ctx, rows); err != nil {
		slog.Warn("Failed to seed Search Console facts", "site_id", siteID, "error", err)
		return 0
	}
	return len(rows)
}

func auditSeededGoogleSearchConsoleFacts(ctx context.Context, store *database.Store, siteID, teamID uuid.UUID, propertyURI string, userID uuid.UUID) error {
	return store.AppendAuditEntry(ctx, database.AuditEntryParams{
		ActorID:    userID,
		TeamID:     teamID,
		Action:     "google_search_console.sync_import_prepared",
		TargetType: "site",
		TargetID:   siteID.String(),
		Outcome:    "success",
		Details:    fmt.Sprintf("outcome=prepared;property_uri=%s;prepared_rows=seed_fixture;seed_fixture=true", propertyURI),
	})
}

func buildGoogleSearchConsoleFactInputs(siteID uuid.UUID, propertyURI string, days int, now time.Time) []database.SearchConsoleFactInput {
	seedDays := googleSearchConsoleSeedDays(days)
	rows := make([]database.SearchConsoleFactInput, 0, seedDays*len(googleSearchConsoleSeedQueries()))
	for day := 0; day < seedDays; day++ {
		date := searchConsoleSeedDate(now.AddDate(0, 0, -(day + 2)))
		for index, query := range googleSearchConsoleSeedQueries() {
			rows = append(rows, googleSearchConsoleSeedFact(siteID, propertyURI, date, day, index, seedDays, query, now))
		}
	}
	return rows
}

func googleSearchConsoleSeedDays(days int) int {
	switch {
	case days <= 0:
		return 90
	case days > 60:
		return 60
	default:
		return days
	}
}

type googleSearchConsoleSeedQuery struct {
	query    string
	path     string
	country  string
	device   string
	clicks   int
	position float64
}

func googleSearchConsoleSeedQueries() []googleSearchConsoleSeedQuery {
	return []googleSearchConsoleSeedQuery{
		{"privacy friendly analytics", "/", "US", "DESKTOP", 18, 2.8},
		{"self hosted web analytics", "/docs", "DE", "DESKTOP", 11, 4.1},
		{"cookie free analytics", "/pricing", "GB", "MOBILE", 9, 5.4},
		{"google analytics alternative", "/compare/google-analytics", "CA", "MOBILE", 7, 6.2},
	}
}

func googleSearchConsoleSeedFact(siteID uuid.UUID, propertyURI string, date time.Time, day int, index int, seedDays int, query googleSearchConsoleSeedQuery, importedAt time.Time) database.SearchConsoleFactInput {
	seasonality := (seedDays - day) + (index * 3)
	clicks := query.clicks + seasonality%9
	impressions := clicks*14 + 60 + (day % 11)
	return database.SearchConsoleFactInput{
		SiteID:          siteID,
		PropertyURI:     propertyURI,
		Date:            date,
		Query:           query.query,
		Page:            "https://acme-analytics.io" + query.path,
		Country:         query.country,
		Device:          query.device,
		Clicks:          clicks,
		Impressions:     impressions,
		CTR:             float64(clicks) / float64(impressions),
		Position:        query.position + float64(day%5)/10,
		AggregationType: "auto",
		DataState:       "final",
		ImportedAt:      importedAt,
	}
}

func seedGoogleSearchConsoleSyncState(ctx context.Context, store *database.Store, siteID, teamID, userID uuid.UUID, state string, category string, manual bool, now time.Time) {
	input := googleSearchConsoleSyncStateInput(siteID, teamID, state, category, manual, now)
	err := store.UpsertGoogleSearchConsoleSyncStateWithAudit(ctx, input, seedGoogleSearchConsoleSyncAudit(siteID, teamID, userID, input.State, category))
	logGoogleSearchConsoleSyncSeedError(err, siteID, state)
}

func googleSearchConsoleSyncStateInput(siteID, teamID uuid.UUID, state string, category string, manual bool, now time.Time) database.GoogleSearchConsoleSyncStateInput {
	importedStart, importedEnd, lastSuccess := googleSearchConsoleSyncMilestones(state, now)
	lastAttempt := now.Add(-20 * time.Minute)
	return database.GoogleSearchConsoleSyncStateInput{
		SiteID:            siteID,
		TeamID:            teamID,
		State:             strings.TrimSpace(state),
		ImportedStartDate: importedStart,
		ImportedEndDate:   importedEnd,
		LastSuccessAt:     lastSuccess,
		LastAttemptAt:     &lastAttempt,
		LastErrorCategory: strings.TrimSpace(category),
		NextRetryAt:       googleSearchConsoleNextRetry(category, now),
		Manual:            manual,
	}
}

func googleSearchConsoleSyncMilestones(state string, now time.Time) (*time.Time, *time.Time, *time.Time) {
	start := searchConsoleSeedDate(now.AddDate(0, 0, -61))
	end := searchConsoleSeedDate(now.AddDate(0, 0, -2))
	success := now.AddDate(0, 0, -1)
	switch strings.TrimSpace(state) {
	case "pending":
		return nil, nil, nil
	case "needs_attention":
		return &start, &end, nil
	default:
		return &start, &end, &success
	}
}

func googleSearchConsoleNextRetry(category string, now time.Time) *time.Time {
	if category != string(searchconsole.CategoryQuotaLimited) {
		return nil
	}
	retry := now.Add(2 * time.Hour)
	return &retry
}

func logGoogleSearchConsoleSyncSeedError(err error, siteID uuid.UUID, state string) {
	if err != nil {
		slog.Warn("Failed to seed Search Console sync state", "site_id", siteID, "state", state, "error", err)
	}
}

func seedGoogleSearchConsoleSyncAudit(siteID, teamID, userID uuid.UUID, state string, category string) database.AuditEntryParams {
	action := "google_search_console.sync_imported"
	outcome := "success"
	details := "outcome=imported;imported_rows=seed_fixture;seed_fixture=true"
	switch state {
	case "pending":
		action = "google_search_console.sync_requested"
		details = "outcome=requested;seed_fixture=true"
	case "failed", "needs_attention":
		action = "google_search_console.sync_failed"
		outcome = "failure"
		details = fmt.Sprintf("outcome=failed;category=%s;seed_fixture=true", strings.TrimSpace(category))
	}
	return database.AuditEntryParams{
		ActorID:    userID,
		TeamID:     teamID,
		Action:     action,
		TargetType: "site",
		TargetID:   siteID.String(),
		Outcome:    outcome,
		Details:    details,
	}
}

func searchConsoleSeedDate(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
