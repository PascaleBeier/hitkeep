package main

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	mrand "math/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

func TestEnsureUserResetsExistingUserPassword(t *testing.T) {
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "seed.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	userID := ensureUser(ctx, store, "demo@example.com", "old-password")
	original, err := store.GetUserByEmail(ctx, "demo@example.com")
	if err != nil {
		t.Fatalf("load original user: %v", err)
	}
	if original == nil {
		t.Fatal("expected original user")
	}
	if !seedPasswordMatches(t, "old-password", original.Password) {
		t.Fatal("expected original password to match")
	}

	reusedID := ensureUser(ctx, store, "demo@example.com", "demo1234")
	if reusedID != userID {
		t.Fatalf("expected existing user id %s, got %s", userID, reusedID)
	}

	updated, err := store.GetUserByEmail(ctx, "demo@example.com")
	if err != nil {
		t.Fatalf("load updated user: %v", err)
	}
	if updated == nil {
		t.Fatal("expected updated user")
	}
	if !seedPasswordMatches(t, "demo1234", updated.Password) {
		t.Fatal("expected reseeded password to match")
	}
	if seedPasswordMatches(t, "old-password", updated.Password) {
		t.Fatal("expected old password to stop matching")
	}
}

func TestSeedActivationFixturesKeepsPrimaryDemoSiteFirst(t *testing.T) {
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "seed.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	userID := ensureUser(ctx, store, "demo@example.com", "demo1234")
	seedTeam(ctx, store, userID)

	primary, err := ensureSiteInActiveTeam(ctx, store, userID, "acme-analytics.io")
	if err != nil {
		t.Fatalf("ensure primary site: %v", err)
	}

	seedActivationFixtures(ctx, store, userID, primary.ID)

	sites, err := store.GetSites(ctx, userID)
	if err != nil {
		t.Fatalf("get sites: %v", err)
	}
	if len(sites) < 3 {
		t.Fatalf("expected primary and activation fixture sites, got %d", len(sites))
	}
	if sites[0].ID != primary.ID {
		t.Fatalf("expected primary demo site first, got %s (%s)", sites[0].Domain, sites[0].ID)
	}
}

func TestSeedGoogleSearchConsoleAndActivationFixturesKeepMappedPrimarySiteFirst(t *testing.T) {
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "seed.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	tenantMgr := database.NewTenantStoreManager(store, t.TempDir())
	defer tenantMgr.Close()

	userID := ensureUser(ctx, store, "demo@example.com", "demo1234")
	seedTeam(ctx, store, userID)
	primary, err := ensureSiteInActiveTeam(ctx, store, userID, "acme-analytics.io")
	if err != nil {
		t.Fatalf("ensure primary site: %v", err)
	}
	if err := tenantMgr.SyncSite(ctx, primary.ID); err != nil {
		t.Fatalf("sync primary site: %v", err)
	}

	seedGoogleSearchConsoleFixtures(ctx, store, tenantMgr, userID, primary.ID, 90)
	seedActivationFixtures(ctx, store, userID, primary.ID)

	sites, err := store.GetSites(ctx, userID)
	if err != nil {
		t.Fatalf("get sites: %v", err)
	}
	if len(sites) < 6 {
		t.Fatalf("expected primary plus Search Console and activation fixture sites, got %d", len(sites))
	}
	if sites[0].ID != primary.ID {
		t.Fatalf("expected mapped primary demo site first, got %s (%s)", sites[0].Domain, sites[0].ID)
	}
}

func TestSeedGoogleSearchConsoleFixturesCreatesMappedReportRows(t *testing.T) {
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "seed.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	tenantMgr := database.NewTenantStoreManager(store, t.TempDir())
	defer tenantMgr.Close()

	userID := ensureUser(ctx, store, "demo@example.com", "demo1234")
	seedTeam(ctx, store, userID)
	primary, err := ensureSiteInActiveTeam(ctx, store, userID, "acme-analytics.io")
	if err != nil {
		t.Fatalf("ensure primary site: %v", err)
	}
	if err := tenantMgr.SyncSite(ctx, primary.ID); err != nil {
		t.Fatalf("sync primary site: %v", err)
	}

	stats := seedGoogleSearchConsoleFixtures(ctx, store, tenantMgr, userID, primary.ID, 90)
	if stats.facts == 0 {
		t.Fatalf("expected seeded Search Console facts")
	}

	teamID, err := store.GetSiteTenantID(ctx, primary.ID)
	if err != nil {
		t.Fatalf("GetSiteTenantID: %v", err)
	}
	conn, err := store.GetGoogleSearchConsoleConnection(ctx, teamID)
	if err != nil {
		t.Fatalf("GetGoogleSearchConsoleConnection: %v", err)
	}
	if conn == nil || !conn.Connected || conn.GoogleAccountEmail != "demo-search-console@example.com" {
		t.Fatalf("expected connected demo Search Console account, got %+v", conn)
	}
	mapping, err := store.GetGoogleSearchConsoleSiteMappingForTeam(ctx, primary.ID, teamID)
	if err != nil {
		t.Fatalf("GetGoogleSearchConsoleSiteMappingForTeam: %v", err)
	}
	if mapping == nil || mapping.PropertyURI != "sc-domain:acme-analytics.io" {
		t.Fatalf("expected primary Search Console mapping, got %+v", mapping)
	}
	state, err := store.GetGoogleSearchConsoleSyncState(ctx, primary.ID)
	if err != nil {
		t.Fatalf("GetGoogleSearchConsoleSyncState: %v", err)
	}
	if state == nil || state.State != "succeeded" || state.LastSuccessAt == nil {
		t.Fatalf("expected successful primary sync state, got %+v", state)
	}

	tenantStore, _, err := tenantMgr.ResolveSiteStore(ctx, primary.ID)
	if err != nil {
		t.Fatalf("ResolveSiteStore: %v", err)
	}
	overview, err := tenantStore.GetSearchConsoleOverview(ctx, api.SearchConsoleReportParams{
		SiteID:      primary.ID,
		PropertyURI: "sc-domain:acme-analytics.io",
		Start:       time.Now().UTC().AddDate(0, 0, -30),
		End:         time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("GetSearchConsoleOverview: %v", err)
	}
	if overview.Clicks == 0 || overview.Impressions == 0 || overview.DataSource != "google_search_console" {
		t.Fatalf("expected useful Search Console overview, got %+v", overview)
	}
}

func TestSeedWebVitalsCreatesReportableSamples(t *testing.T) {
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "seed.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	userID := ensureUser(ctx, store, "demo@example.com", "demo1234")
	site, err := store.CreateSite(ctx, userID, "web-vitals-seed.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	count, err := seedWebVitals(ctx, store, site.ID, 7, mrand.New(mrand.NewSource(147))) // #nosec G404 -- deterministic demo fixture test.
	if err != nil {
		t.Fatalf("seedWebVitals: %v", err)
	}
	if count == 0 {
		t.Fatal("expected seeded Web Vitals samples")
	}

	summary, err := store.GetWebVitalsSummary(ctx, api.WebVitalsParams{
		SiteID: site.ID,
		Start:  time.Now().UTC().AddDate(0, 0, -8),
		End:    time.Now().UTC().AddDate(0, 0, 1),
	})
	if err != nil {
		t.Fatalf("GetWebVitalsSummary: %v", err)
	}
	if len(summary) != 5 {
		t.Fatalf("expected all five Web Vitals metrics, got %+v", summary)
	}
}

func TestSeedQRCampaignsCreatesDefinitionsAndAttribution(t *testing.T) {
	ctx, store, tenantMgr, site, userID, analyticsStore := newSeedQRCampaignTestContext(t)
	defer store.Close()
	defer tenantMgr.Close()

	deleteSiteQRCampaignData(ctx, store, analyticsStore, site.ID)
	stats, err := seedQRCampaigns(ctx, store, analyticsStore, site.ID, userID, site.Domain, 30, t.TempDir(), mrand.New(mrand.NewSource(2112))) // #nosec G404 -- deterministic fixture test.
	if err != nil {
		t.Fatalf("seedQRCampaigns: %v", err)
	}
	requireSeedQRCampaignStats(t, stats)
	qrs := requireSeedQRCampaignDefinitions(t, ctx, store, site.ID, stats.qrCodes)
	requireSeedQRCodeAttribution(t, ctx, analyticsStore, site.ID, qrs)
}

func TestSeedQRCampaignsCreateDefaultRangeAnalytics(t *testing.T) {
	ctx, store, tenantMgr, site, userID, analyticsStore := newSeedQRCampaignTestContext(t)
	defer store.Close()
	defer tenantMgr.Close()

	stats, err := seedQRCampaigns(ctx, store, analyticsStore, site.ID, userID, site.Domain, 30, t.TempDir(), mrand.New(mrand.NewSource(2112))) // #nosec G404 -- deterministic fixture test.
	if err != nil {
		t.Fatalf("seedQRCampaigns: %v", err)
	}
	requireSeedQRCampaignStats(t, stats)
	qrs := requireSeedQRCampaignDefinitions(t, ctx, store, site.ID, stats.qrCodes)

	defaultRangeStart := time.Now().UTC().Truncate(24 * time.Hour)
	defaultRangeEnd := time.Now().UTC().Add(5 * time.Minute)
	for _, qr := range qrs {
		opens, err := analyticsStore.CountQRCodeOpens(ctx, site.ID, qr.ID, defaultRangeStart, defaultRangeEnd)
		if err != nil {
			t.Fatalf("CountQRCodeOpens(%s): %v", qr.Name, err)
		}
		if opens == 0 {
			t.Fatalf("expected default today range to show QR opens for %s", qr.Name)
		}

		report, err := analyticsStore.GetSiteStats(ctx, api.AnalyticsParams{
			SiteID: site.ID,
			Start:  defaultRangeStart,
			End:    defaultRangeEnd,
			Filters: []api.Filter{{
				Type:  "qr_code_id",
				Value: qr.ID.String(),
			}},
		})
		if err != nil {
			t.Fatalf("GetSiteStats(%s): %v", qr.Name, err)
		}
		if report.TotalPageviews == 0 || report.UniqueSessions == 0 {
			t.Fatalf("expected default today range to show QR-attributed analytics for %s, got %+v", qr.Name, report)
		}
	}
}

func TestSeedTrafficCreatesDefaultRangeUTMData(t *testing.T) {
	ctx, store, tenantMgr, site, _, analyticsStore := newSeedQRCampaignTestContext(t)
	defer store.Close()
	defer tenantMgr.Close()

	stats, err := seedTraffic(ctx, analyticsStore, site.ID, goalIDs{}, 30, mrand.New(mrand.NewSource(4242))) // #nosec G404 -- deterministic demo fixture test.
	if err != nil {
		t.Fatalf("seedTraffic: %v", err)
	}
	if stats.hits == 0 || stats.sessions == 0 {
		t.Fatalf("expected seeded traffic, got %+v", stats)
	}

	defaultRangeStart := time.Now().UTC().Truncate(24 * time.Hour)
	defaultRangeEnd := time.Now().UTC().Add(5 * time.Minute)
	report, err := analyticsStore.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		Start:  defaultRangeStart,
		End:    defaultRangeEnd,
	})
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}
	if report.UTMCampaignHits == 0 || report.UTMMediumHits == 0 || report.UTMSourceHits == 0 {
		t.Fatalf("expected default today range to show seeded UTM KPIs, got campaign=%d medium=%d source=%d", report.UTMCampaignHits, report.UTMMediumHits, report.UTMSourceHits)
	}
	if !hasSpecifiedSeedMetric(report.TopUTMCampaigns) || !hasSpecifiedSeedMetric(report.TopUTMMediums) || !hasSpecifiedSeedMetric(report.TopUTMSources) {
		t.Fatalf("expected default today range to show seeded UTM top lists, got campaigns=%+v mediums=%+v sources=%+v", report.TopUTMCampaigns, report.TopUTMMediums, report.TopUTMSources)
	}
}

func TestSeedAIFetchesCreatesDefaultRangeGPTBotData(t *testing.T) {
	ctx, store, tenantMgr, site, _, analyticsStore := newSeedQRCampaignTestContext(t)
	defer store.Close()
	defer tenantMgr.Close()

	stats, err := seedAIVisibility(ctx, analyticsStore, site.ID, 30, mrand.New(mrand.NewSource(4242))) // #nosec G404 -- deterministic demo fixture test.
	if err != nil {
		t.Fatalf("seedAIVisibility: %v", err)
	}
	if stats.fetches == 0 {
		t.Fatalf("expected seeded AI fetches, got %+v", stats)
	}

	defaultRangeStart := time.Now().UTC().Truncate(24 * time.Hour)
	defaultRangeEnd := time.Now().UTC().Add(5 * time.Minute)
	overview, err := analyticsStore.GetAIFetchOverview(ctx, api.AIFetchQueryParams{
		SiteID: site.ID,
		Start:  defaultRangeStart,
		End:    defaultRangeEnd,
	})
	if err != nil {
		t.Fatalf("GetAIFetchOverview: %v", err)
	}
	if overview.TotalRequests == 0 {
		t.Fatalf("expected default today range to show seeded AI fetch requests, got %+v", overview)
	}
	if !hasSeedMetricNamed(overview.TopAssistants, "GPTBot") {
		t.Fatalf("expected default today range to show GPTBot, got %+v", overview.TopAssistants)
	}
}

func TestSeedAIFetchesCreatesTrackedAIBotHits(t *testing.T) {
	ctx, store, tenantMgr, site, _, analyticsStore := newSeedQRCampaignTestContext(t)
	defer store.Close()
	defer tenantMgr.Close()

	if _, err := seedAIVisibility(ctx, analyticsStore, site.ID, 30, mrand.New(mrand.NewSource(4242))); err != nil { // #nosec G404 -- deterministic demo fixture test.
		t.Fatalf("seedAIVisibility: %v", err)
	}

	report, err := analyticsStore.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		Start:  time.Now().UTC().AddDate(0, 0, -30),
		End:    time.Now().UTC().Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}
	if report.AIBotHits == 0 {
		t.Fatalf("expected seeded AI bot hits in tracked stats, got %d", report.AIBotHits)
	}

	distinct := map[string]struct{}{}
	for _, row := range report.TopAIBots {
		if row.Name != "" {
			distinct[row.Name] = struct{}{}
		}
	}
	if len(distinct) < 3 {
		t.Fatalf("expected at least 3 distinct seeded AI agent names, got %d: %+v", len(distinct), report.TopAIBots)
	}
}

func TestSeedAIVisibilityFeedsTheMergedAIActivityReport(t *testing.T) {
	ctx, store, tenantMgr, site, _, analyticsStore := newSeedQRCampaignTestContext(t)
	defer store.Close()
	defer tenantMgr.Close()

	if _, err := seedAIVisibility(ctx, analyticsStore, site.ID, 30, mrand.New(mrand.NewSource(4242))); err != nil { // #nosec G404 -- deterministic demo fixture test.
		t.Fatalf("seedAIVisibility: %v", err)
	}

	report, err := analyticsStore.GetAIActivity(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		Start:  time.Now().UTC().AddDate(0, 0, -30),
		End:    time.Now().UTC().Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("GetAIActivity: %v", err)
	}
	if report.TrackedHits == 0 || report.FetchCount == 0 {
		t.Fatalf("expected both provenances seeded, got tracked %d fetched %d", report.TrackedHits, report.FetchCount)
	}

	merged := false
	for _, row := range report.TopPaths {
		if row.TrackedHits > 0 && row.FetchCount > 0 {
			merged = true
			break
		}
	}
	if !merged {
		t.Fatalf("expected at least one path with both tracked hits and fetch records, got %+v", report.TopPaths)
	}

	// Exactly one seeded fetch row is legacy-style: no assistant_category, so
	// the demo data exercises the query-time COALESCE fallback.
	legacyRows := 0
	for _, spec := range pinnedAIFetches {
		row := seedPinnedAIFetch(site.ID, time.Now().UTC(), time.Now().UTC(), spec)
		if spec.legacy {
			legacyRows++
			if row.AssistantCategory != "" {
				t.Fatalf("expected the legacy seed row to carry no category, got %q", row.AssistantCategory)
			}
			continue
		}
		if row.AssistantCategory == "" {
			t.Fatalf("expected the pinned %s seed row to carry a classified category", spec.name)
		}
	}
	if legacyRows != 1 {
		t.Fatalf("expected exactly one legacy pinned seed row, got %d", legacyRows)
	}

	// Every fetch record must land in a category — including the legacy row,
	// which only gets one through hk_ai_bot_category_from_name.
	categorizedFetches := 0
	for _, row := range report.TopCategories {
		categorizedFetches += row.FetchCount
	}
	if categorizedFetches != report.FetchCount {
		t.Fatalf("categorized fetch records = %d, want all %d: the NULL-category row must fall back to its agent's category", categorizedFetches, report.FetchCount)
	}
}

func hasSpecifiedSeedMetric(rows []api.MetricStat) bool {
	for _, row := range rows {
		if row.Name != "" && row.Name != "(Unspecified)" {
			return true
		}
	}
	return false
}

func hasSeedMetricNamed(rows []api.MetricStat, name string) bool {
	for _, row := range rows {
		if row.Name == name {
			return true
		}
	}
	return false
}

func newSeedQRCampaignTestContext(t *testing.T) (context.Context, *database.Store, *database.TenantStoreManager, *api.Site, uuid.UUID, *database.Store) {
	t.Helper()
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "seed.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	tenantMgr := database.NewTenantStoreManager(store, t.TempDir())

	userID := ensureUser(ctx, store, "demo@example.com", "demo1234")
	seedTeam(ctx, store, userID)
	site, err := ensureSiteInActiveTeam(ctx, store, userID, "acme-analytics.io")
	if err != nil {
		t.Fatalf("ensure site: %v", err)
	}
	if err := tenantMgr.SyncSite(ctx, site.ID); err != nil {
		t.Fatalf("sync site: %v", err)
	}
	analyticsStore, _, err := tenantMgr.ResolveSiteStore(ctx, site.ID)
	if err != nil {
		t.Fatalf("resolve tenant store: %v", err)
	}
	return ctx, store, tenantMgr, site, userID, analyticsStore
}

func requireSeedQRCampaignStats(t *testing.T, stats seedStats) {
	t.Helper()
	if stats.qrCodes != 3 || stats.qrOpens == 0 || stats.hits == 0 || stats.sessions == 0 || stats.events == 0 {
		t.Fatalf("expected QR definitions, opens, hits, sessions, and events, got %+v", stats)
	}
}

func requireSeedQRCampaignDefinitions(t *testing.T, ctx context.Context, store *database.Store, siteID uuid.UUID, expected int) []api.QRCode {
	t.Helper()
	qrs, err := store.ListQRCodes(ctx, siteID, false)
	if err != nil {
		t.Fatalf("ListQRCodes: %v", err)
	}
	if len(qrs) != expected {
		t.Fatalf("expected %d QR codes, got %d", expected, len(qrs))
	}

	conference := requireSeedQRCodeByName(t, qrs, "Conference booth poster")
	if conference.UTMSource != "conference" || conference.UTMMedium != "qr" || conference.UTMCampaign != "berlin-analytics-summit" {
		t.Fatalf("expected conference QR UTM attribution, got %+v", conference)
	}
	if conference.CustomParams["placement"] != "booth-wall" || conference.CustomParams["segment"] != "enterprise" {
		t.Fatalf("expected conference QR custom params, got %+v", conference.CustomParams)
	}
	asset, err := store.GetQRCodeAsset(ctx, siteID, conference.ID)
	if err != nil {
		t.Fatalf("GetQRCodeAsset: %v", err)
	}
	if asset == nil || asset.ContentType != "image/png" || asset.ByteSize == 0 || !strings.HasPrefix(asset.Checksum, "sha256:") {
		t.Fatalf("expected persisted PNG QR asset, got %+v", asset)
	}
	shares, err := store.ListQRCodeShareLinks(ctx, siteID, conference.ID)
	if err != nil {
		t.Fatalf("ListQRCodeShareLinks: %v", err)
	}
	if len(shares) == 0 {
		t.Fatal("expected seeded QR-only share link")
	}
	return qrs
}

func requireSeedQRCodeAttribution(t *testing.T, ctx context.Context, analyticsStore *database.Store, siteID uuid.UUID, qrs []api.QRCode) {
	t.Helper()
	start := time.Now().UTC().AddDate(0, 0, -31)
	end := time.Now().UTC().AddDate(0, 0, 1)
	for _, qr := range qrs {
		opens, err := analyticsStore.CountQRCodeOpens(ctx, siteID, qr.ID, start, end)
		if err != nil {
			t.Fatalf("CountQRCodeOpens(%s): %v", qr.Name, err)
		}
		if opens == 0 {
			t.Fatalf("expected QR opens for %s", qr.Name)
		}

		filter := []api.Filter{{Type: "qr_code_id", Value: qr.ID.String()}}
		filteredStats, err := analyticsStore.GetSiteStats(ctx, api.AnalyticsParams{
			SiteID:  siteID,
			Start:   start,
			End:     end,
			Filters: filter,
		})
		if err != nil {
			t.Fatalf("GetSiteStats(%s): %v", qr.Name, err)
		}
		if filteredStats.TotalPageviews == 0 || filteredStats.UniqueSessions == 0 {
			t.Fatalf("expected QR-scoped analytics for %s, got %+v", qr.Name, filteredStats)
		}

		hits, err := analyticsStore.GetHits(ctx, api.HitQueryParams{
			SiteID:  siteID,
			Start:   start,
			End:     end,
			Limit:   10,
			Filters: filter,
		})
		if err != nil {
			t.Fatalf("GetHits(%s): %v", qr.Name, err)
		}
		if hits.Total == 0 || len(hits.Data) == 0 {
			t.Fatalf("expected QR-attributed hits for %s, got %+v", qr.Name, hits)
		}
		if hits.Data[0].QRCodeID == nil || *hits.Data[0].QRCodeID != qr.ID {
			t.Fatalf("expected hit attribution %s, got %+v", qr.ID, hits.Data[0].QRCodeID)
		}
	}
}

func TestSeedOpportunitiesCreatesWebVitalsPerformanceOpportunity(t *testing.T) {
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "seed.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	userID := ensureUser(ctx, store, "demo@example.com", "demo1234")
	site, err := store.CreateSite(ctx, userID, "web-vitals-opportunity.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if _, err := seedWebVitals(ctx, store, site.ID, 30, mrand.New(mrand.NewSource(147))); err != nil { // #nosec G404 -- deterministic demo fixture test.
		t.Fatalf("seedWebVitals: %v", err)
	}

	count, err := seedOpportunities(ctx, store, store, *site, userID, time.Now().UTC().AddDate(0, 0, -31), time.Now().UTC())
	if err != nil {
		t.Fatalf("seedOpportunities: %v", err)
	}
	if count == 0 {
		t.Fatal("expected generated opportunities")
	}

	items, err := store.ListOpportunities(ctx, site.ID)
	if err != nil {
		t.Fatalf("ListOpportunities: %v", err)
	}
	webVitals := findSeedOpportunity(items, "opportunities.types.web_vitals_performance")
	if webVitals == nil {
		t.Fatalf("expected Web Vitals performance opportunity, got %+v", items)
	}
	if webVitals.Kind != "performance" || webVitals.RouteLabelKey != "opportunities.routes.web_vitals" {
		t.Fatalf("expected Web Vitals opportunity routing, got kind=%q route=%q", webVitals.Kind, webVitals.RouteLabelKey)
	}
	if !seedOpportunityHasEvidence(*webVitals, "web_vital_metric") || !seedOpportunityHasEvidence(*webVitals, "web_vital_top_page") {
		t.Fatalf("expected metric and top-page evidence, got %+v", webVitals.Evidence)
	}
}

func findSeedOpportunity(items []api.Opportunity, typeKey string) *api.Opportunity {
	for i := range items {
		if items[i].TypeKey == typeKey {
			return &items[i]
		}
	}
	return nil
}

func requireSeedQRCodeByName(t *testing.T, qrs []api.QRCode, name string) api.QRCode {
	t.Helper()
	for _, qr := range qrs {
		if qr.Name == name {
			return qr
		}
	}
	t.Fatalf("expected seeded QR code %q, got %+v", name, qrs)
	return api.QRCode{}
}

func seedOpportunityHasEvidence(item api.Opportunity, evidenceID string) bool {
	for _, evidence := range item.Evidence {
		if evidence.ID == evidenceID {
			return true
		}
	}
	return false
}

func TestSeedGoogleSearchConsoleFixturesCreatesStatusExamples(t *testing.T) {
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "seed.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	tenantMgr := database.NewTenantStoreManager(store, t.TempDir())
	defer tenantMgr.Close()

	userID := ensureUser(ctx, store, "demo@example.com", "demo1234")
	seedTeam(ctx, store, userID)
	primary, err := ensureSiteInActiveTeam(ctx, store, userID, "acme-analytics.io")
	if err != nil {
		t.Fatalf("ensure primary site: %v", err)
	}

	seedGoogleSearchConsoleFixtures(ctx, store, tenantMgr, userID, primary.ID, 90)

	expectedStates := map[string]string{
		"search-pending.example.com":   "pending",
		"search-quota.example.com":     "failed",
		"search-reconnect.example.com": "needs_attention",
	}
	for domain, expectedState := range expectedStates {
		site := requireSeedSiteByDomain(t, ctx, store, domain)
		state, err := store.GetGoogleSearchConsoleSyncState(ctx, site.ID)
		if err != nil {
			t.Fatalf("GetGoogleSearchConsoleSyncState(%s): %v", domain, err)
		}
		if state == nil || state.State != expectedState {
			t.Fatalf("expected %s state %q, got %+v", domain, expectedState, state)
		}
	}

	unmapped := requireSeedSiteByDomain(t, ctx, store, "search-unmapped.example.com")
	mapping, err := store.GetGoogleSearchConsoleSiteMapping(ctx, unmapped.ID)
	if err != nil {
		t.Fatalf("GetGoogleSearchConsoleSiteMapping: %v", err)
	}
	if mapping != nil {
		t.Fatalf("expected unmapped fixture site, got %+v", mapping)
	}
}

func requireSeedSiteByDomain(t *testing.T, ctx context.Context, store *database.Store, domain string) *api.Site {
	t.Helper()
	var site api.Site
	if err := store.DB().QueryRowContext(ctx, `
		SELECT id, user_id, domain, data_retention_days, created_at
		FROM sites
		WHERE lower(domain) = lower(?)
		LIMIT 1
	`, domain).Scan(&site.ID, &site.UserID, &site.Domain, &site.DataRetentionDays, &site.CreatedAt); err != nil {
		t.Fatalf("load seed site %s: %v", domain, err)
	}
	return &site
}

func seedPasswordMatches(t *testing.T, password string, encoded string) bool {
	t.Helper()

	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		t.Fatalf("invalid password hash format: %q", encoded)
	}
	if parts[1] != "argon2id" {
		t.Fatalf("unexpected password algorithm: %q", parts[1])
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		t.Fatalf("parse argon2 version: %v", err)
	}
	if version != argon2.Version {
		t.Fatalf("unexpected argon2 version: %d", version)
	}

	var memory uint32
	var timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		t.Fatalf("parse argon2 params: %v", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		t.Fatalf("decode salt: %v", err)
	}
	decodedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		t.Fatalf("decode hash: %v", err)
	}

	comparisonHash := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(decodedHash)))
	return subtle.ConstantTimeCompare(decodedHash, comparisonHash) == 1
}
