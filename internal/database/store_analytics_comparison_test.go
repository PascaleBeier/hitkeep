package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func setupComparisonStore(t *testing.T) (*Store, uuid.UUID) {
	t.Helper()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	userID, err := store.CreateUser(context.Background(), "cmp@example.com", "hashed")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return store, userID
}

func TestGetSiteStatsComparison(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "comparison.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	currentStart := now.AddDate(0, 0, -7)
	currentEnd := now
	previousStart := now.AddDate(0, 0, -14)
	previousEnd := now.AddDate(0, 0, -7)

	sessionCurrent := uuid.New()
	sessionPrevious := uuid.New()

	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: sessionCurrent,
		PageID:    uuid.New(),
		Timestamp: now.AddDate(0, 0, -3),
		Path:      "/current",
	}); err != nil {
		t.Fatalf("create current hit: %v", err)
	}

	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: sessionPrevious,
		PageID:    uuid.New(),
		Timestamp: now.AddDate(0, 0, -10),
		Path:      "/previous",
	}); err != nil {
		t.Fatalf("create previous hit: %v", err)
	}

	params := api.AnalyticsParams{
		SiteID:       site.ID,
		UserID:       userID,
		Start:        currentStart,
		End:          currentEnd,
		CompareStart: previousStart,
		CompareEnd:   previousEnd,
	}

	result, err := store.GetSiteStats(ctx, params)
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}

	if result.Comparison == nil {
		t.Fatal("expected comparison stats, got nil")
	}
	if result.TotalPageviews != 1 {
		t.Errorf("expected 1 current pageview, got %d", result.TotalPageviews)
	}
	if result.Comparison.TotalPageviews != 1 {
		t.Errorf("expected 1 comparison pageview, got %d", result.Comparison.TotalPageviews)
	}
}

func TestGetSiteStatsNoComparison(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "nocompare.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	params := api.AnalyticsParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  now.AddDate(0, 0, -7),
		End:    now,
	}

	result, err := store.GetSiteStats(ctx, params)
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}
	if result.Comparison != nil {
		t.Error("expected nil comparison when CompareStart is zero")
	}
}

func TestGetSiteStatsComparisonCurrentPeriodEmpty(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "cmp-empty.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	currentStart := now.AddDate(0, 0, -7)
	currentEnd := now
	previousStart := now.AddDate(0, 0, -14)
	previousEnd := now.AddDate(0, 0, -7)

	// Only a hit in the previous window; current period should show zero pageviews.
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: now.AddDate(0, 0, -10),
		Path:      "/old-page",
	}); err != nil {
		t.Fatalf("create previous hit: %v", err)
	}

	params := api.AnalyticsParams{
		SiteID:       site.ID,
		UserID:       userID,
		Start:        currentStart,
		End:          currentEnd,
		CompareStart: previousStart,
		CompareEnd:   previousEnd,
	}

	result, err := store.GetSiteStats(ctx, params)
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}

	if result.Comparison == nil {
		t.Fatal("expected comparison stats even when current period is empty")
	}
	if result.TotalPageviews != 0 {
		t.Errorf("expected 0 current pageviews, got %d", result.TotalPageviews)
	}
	if result.Comparison.TotalPageviews != 1 {
		t.Errorf("expected 1 comparison pageview, got %d", result.Comparison.TotalPageviews)
	}
}

func TestGetSiteStatsComparisonBothPeriodsPopulated(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "cmp-both.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	currentStart := now.AddDate(0, 0, -7)
	currentEnd := now
	previousStart := now.AddDate(0, 0, -14)
	previousEnd := now.AddDate(0, 0, -7)

	sessionA := uuid.New()
	sessionB := uuid.New()

	// Two hits in the current period across the same session.
	for _, path := range []string{"/home", "/about"} {
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID:    site.ID,
			SessionID: sessionA,
			PageID:    uuid.New(),
			Timestamp: now.AddDate(0, 0, -2),
			Path:      path,
		}); err != nil {
			t.Fatalf("create current hit %s: %v", path, err)
		}
	}

	// One hit in the previous period on a different session.
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: sessionB,
		PageID:    uuid.New(),
		Timestamp: now.AddDate(0, 0, -9),
		Path:      "/landing",
	}); err != nil {
		t.Fatalf("create previous hit: %v", err)
	}

	params := api.AnalyticsParams{
		SiteID:       site.ID,
		UserID:       userID,
		Start:        currentStart,
		End:          currentEnd,
		CompareStart: previousStart,
		CompareEnd:   previousEnd,
	}

	result, err := store.GetSiteStats(ctx, params)
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}

	if result.Comparison == nil {
		t.Fatal("expected comparison stats, got nil")
	}
	if result.TotalPageviews != 2 {
		t.Errorf("expected 2 current pageviews, got %d", result.TotalPageviews)
	}
	if result.Comparison.TotalPageviews != 1 {
		t.Errorf("expected 1 comparison pageview, got %d", result.Comparison.TotalPageviews)
	}
}

func TestGetSiteStatsIncludesLandingAndExitPages(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "pages.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	sessionA := uuid.New()
	sessionB := uuid.New()
	sessionC := uuid.New()

	for _, hit := range []struct {
		sessionID uuid.UUID
		path      string
		timestamp time.Time
		language  string
	}{
		{sessionID: sessionA, path: "/home", timestamp: base.Add(-6 * time.Hour), language: "de-DE"},
		{sessionID: sessionA, path: "/pricing", timestamp: base.Add(-5 * time.Hour), language: "de-DE"},
		{sessionID: sessionA, path: "/signup", timestamp: base.Add(-4 * time.Hour), language: "de-DE"},
		{sessionID: sessionB, path: "/blog", timestamp: base.Add(-3 * time.Hour), language: "en-US"},
		{sessionID: sessionB, path: "/pricing", timestamp: base.Add(-2 * time.Hour), language: "en-US"},
		{sessionID: sessionC, path: "/home", timestamp: base.Add(-90 * time.Minute), language: "de-AT"},
	} {
		language := hit.language
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID:    site.ID,
			SessionID: hit.sessionID,
			PageID:    uuid.New(),
			Timestamp: hit.timestamp,
			Path:      hit.path,
			Language:  &language,
		}); err != nil {
			t.Fatalf("create hit %s: %v", hit.path, err)
		}
	}

	result, err := store.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  base.Add(-24 * time.Hour),
		End:    base,
	})
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}

	if len(result.TopPages) < 2 {
		t.Fatalf("expected top pages, got %v", result.TopPages)
	}
	if !containsMetric(result.TopPages, "/home", 2) {
		t.Fatalf("expected /home with 2 pageviews in top pages, got %+v", result.TopPages)
	}
	if !containsMetric(result.TopPages, "/pricing", 2) {
		t.Fatalf("expected /pricing with 2 pageviews in top pages, got %+v", result.TopPages)
	}
	if result.TopLandingPages[0].Name != "/home" || result.TopLandingPages[0].Value != 2 {
		t.Fatalf("expected /home as top landing page with 2 sessions, got %+v", result.TopLandingPages[0])
	}
	if result.TopLandingPages[1].Name != "/blog" || result.TopLandingPages[1].Value != 1 {
		t.Fatalf("expected /blog as second landing page, got %+v", result.TopLandingPages[1])
	}
	if result.TopExitPages[0].Name != "/home" || result.TopExitPages[0].Value != 1 {
		t.Fatalf("expected /home as first exit page by alphabetical tiebreak, got %+v", result.TopExitPages[0])
	}
	if result.TopExitPages[1].Name != "/pricing" || result.TopExitPages[1].Value != 1 {
		t.Fatalf("expected /pricing as second exit page, got %+v", result.TopExitPages[1])
	}
	if result.TopExitPages[2].Name != "/signup" || result.TopExitPages[2].Value != 1 {
		t.Fatalf("expected /signup as third exit page, got %+v", result.TopExitPages[2])
	}
	if len(result.TopLanguages) < 2 {
		t.Fatalf("expected top languages, got %+v", result.TopLanguages)
	}
	if result.TopLanguages[0].Name != "de" || result.TopLanguages[0].Value != 4 {
		t.Fatalf("expected top language de with 4 hits, got %+v", result.TopLanguages[0])
	}
	if result.TopLanguages[1].Name != "en" || result.TopLanguages[1].Value != 2 {
		t.Fatalf("expected second language en with 2 hits, got %+v", result.TopLanguages[1])
	}
}

func TestGetSiteStatsIncludesConfiguredFunnels(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "funnels.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	if err := store.CreateFunnel(ctx, &api.Funnel{
		SiteID: site.ID,
		Name:   "Signup funnel",
		Steps: []api.FunnelStep{
			{Type: "path", Value: "/pricing"},
			{Type: "event", Value: "signup_completed"},
		},
	}); err != nil {
		t.Fatalf("create funnel: %v", err)
	}

	base := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	stats, err := store.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  base.Add(-24 * time.Hour),
		End:    base.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}

	if len(stats.Funnels) != 1 {
		t.Fatalf("expected configured funnel in site stats, got %+v", stats.Funnels)
	}
	if stats.Funnels[0].Name != "Signup funnel" {
		t.Fatalf("expected Signup funnel in site stats, got %+v", stats.Funnels[0])
	}
	if len(stats.Funnels[0].Steps) != 2 {
		t.Fatalf("expected funnel steps in site stats, got %+v", stats.Funnels[0].Steps)
	}
}

func TestGetSiteStatsLandingAndExitUseFullSessionBoundaries(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "session-boundaries.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	sessionID := uuid.New()

	for _, hit := range []struct {
		path      string
		timestamp time.Time
	}{
		{path: "/campaign", timestamp: base.Add(-49 * time.Hour)},
		{path: "/pricing", timestamp: base.Add(-2 * time.Hour)},
		{path: "/checkout", timestamp: base.Add(2 * time.Hour)},
	} {
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID:    site.ID,
			SessionID: sessionID,
			PageID:    uuid.New(),
			Timestamp: hit.timestamp,
			Path:      hit.path,
		}); err != nil {
			t.Fatalf("create hit %s: %v", hit.path, err)
		}
	}

	result, err := store.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  base.Add(-24 * time.Hour),
		End:    base,
	})
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}

	if len(result.TopPages) != 1 || result.TopPages[0].Name != "/pricing" {
		t.Fatalf("expected only in-range top page /pricing, got %+v", result.TopPages)
	}
	if len(result.TopLandingPages) != 1 || result.TopLandingPages[0].Name != "/campaign" {
		t.Fatalf("expected landing page from full session boundary, got %+v", result.TopLandingPages)
	}
	if len(result.TopExitPages) != 1 || result.TopExitPages[0].Name != "/checkout" {
		t.Fatalf("expected exit page from full session boundary, got %+v", result.TopExitPages)
	}
}

func TestGetSiteStatsIncludesAIDimensions(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "ai-dimensions.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	sessionA := uuid.New()
	sessionB := uuid.New()
	sessionC := uuid.New()
	sessionD := uuid.New()
	sessionE := uuid.New()

	chatGPTRef := "https://chatgpt.com/share/abc"
	perplexityRef := "https://www.perplexity.ai/page/example"
	falsePositiveYouRef := "https://thankyou.com/pricing"
	falsePositiveArcRef := "https://sparc.net/docs"
	gptBotUA := "Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)"
	claudeBotUA := "Mozilla/5.0 (compatible; ClaudeBot/1.0; +https://www.anthropic.com/claudebot)"
	falsePositiveDeepSeekUA := "Mozilla/5.0 (compatible; DeepSeekBrowser/1.0; +https://example.com/bot)"
	chromeUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"

	for _, hit := range []api.Hit{
		{
			SiteID:    site.ID,
			SessionID: sessionA,
			PageID:    uuid.New(),
			Timestamp: base.Add(-6 * time.Hour),
			Path:      "/docs/ai",
			UserAgent: &gptBotUA,
		},
		{
			SiteID:    site.ID,
			SessionID: sessionA,
			PageID:    uuid.New(),
			Timestamp: base.Add(-5 * time.Hour),
			Path:      "/docs/ai/setup",
			UserAgent: &gptBotUA,
		},
		{
			SiteID:    site.ID,
			SessionID: sessionB,
			PageID:    uuid.New(),
			Timestamp: base.Add(-4 * time.Hour),
			Path:      "/pricing",
			UserAgent: &claudeBotUA,
		},
		{
			SiteID:    site.ID,
			SessionID: sessionC,
			PageID:    uuid.New(),
			Timestamp: base.Add(-3 * time.Hour),
			Path:      "/blog/hitkeep-ai",
			Referrer:  &chatGPTRef,
			UserAgent: &chromeUA,
		},
		{
			SiteID:    site.ID,
			SessionID: sessionC,
			PageID:    uuid.New(),
			Timestamp: base.Add(-2 * time.Hour),
			Path:      "/docs/case-study",
			Referrer:  &chatGPTRef,
			UserAgent: &chromeUA,
		},
		{
			SiteID:    site.ID,
			SessionID: uuid.New(),
			PageID:    uuid.New(),
			Timestamp: base.Add(-90 * time.Minute),
			Path:      "/compare",
			Referrer:  &perplexityRef,
			UserAgent: &chromeUA,
		},
		{
			SiteID:    site.ID,
			SessionID: sessionD,
			PageID:    uuid.New(),
			Timestamp: base.Add(-70 * time.Minute),
			Path:      "/thank-you",
			Referrer:  &falsePositiveYouRef,
			UserAgent: &chromeUA,
		},
		{
			SiteID:    site.ID,
			SessionID: sessionD,
			PageID:    uuid.New(),
			Timestamp: base.Add(-65 * time.Minute),
			Path:      "/thank-you/details",
			Referrer:  &falsePositiveArcRef,
			UserAgent: &chromeUA,
		},
		{
			SiteID:    site.ID,
			SessionID: sessionE,
			PageID:    uuid.New(),
			Timestamp: base.Add(-50 * time.Minute),
			Path:      "/deepseek-browser",
			UserAgent: &falsePositiveDeepSeekUA,
		},
	} {
		if err := store.CreateHit(ctx, &hit); err != nil {
			t.Fatalf("create hit %s: %v", hit.Path, err)
		}
	}

	result, err := store.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  base.Add(-24 * time.Hour),
		End:    base,
	})
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}

	if result.AIBotHits != 3 {
		t.Fatalf("expected 3 AI bot hits, got %d", result.AIBotHits)
	}
	if result.AISourceVisits != 2 {
		t.Fatalf("expected 2 AI source visits, got %d", result.AISourceVisits)
	}
	if !containsMetric(result.TopAIBots, "GPTBot", 2) {
		t.Fatalf("expected GPTBot with 2 hits, got %+v", result.TopAIBots)
	}
	if !containsMetric(result.TopAIBots, "ClaudeBot", 1) {
		t.Fatalf("expected ClaudeBot with 1 hit, got %+v", result.TopAIBots)
	}
	if !containsMetric(result.TopAISources, "ChatGPT", 1) {
		t.Fatalf("expected ChatGPT with 1 session, got %+v", result.TopAISources)
	}
	if !containsMetric(result.TopAISources, "Perplexity", 1) {
		t.Fatalf("expected Perplexity with 1 session, got %+v", result.TopAISources)
	}
}

func TestGetSiteStatsIncludesGeoNetworkDimensions(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "geo-network.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	region := "California"
	city := "Mountain View"
	provider := "Google LLC"
	asnOrg := "Google LLC"
	asn := 15169
	otherCity := "Berlin"
	otherProvider := "Deutsche Telekom AG"
	otherASNOrg := "Deutsche Telekom AG"
	otherASN := 3320

	for _, hit := range []api.Hit{
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: base.Add(-2 * time.Hour), Path: "/a", Region: &region, City: &city, Provider: &provider, ASN: &asn, ASNOrg: &asnOrg},
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: base.Add(-time.Hour), Path: "/b", Region: &region, City: &city, Provider: &provider, ASN: &asn, ASNOrg: &asnOrg},
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: base.Add(-30 * time.Minute), Path: "/c", City: &otherCity, Provider: &otherProvider, ASN: &otherASN, ASNOrg: &otherASNOrg},
	} {
		if err := store.CreateHit(ctx, &hit); err != nil {
			t.Fatalf("create hit %s: %v", hit.Path, err)
		}
	}

	result, err := store.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  base.Add(-24 * time.Hour),
		End:    base,
	})
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}

	if !containsMetric(result.TopCities, "Mountain View", 2) {
		t.Fatalf("expected Mountain View with 2 hits, got %+v", result.TopCities)
	}
	if !containsMetric(result.TopProviders, "Google LLC", 2) {
		t.Fatalf("expected Google LLC with 2 hits, got %+v", result.TopProviders)
	}
	if !containsMetric(result.TopASNs, "AS15169 Google LLC", 2) {
		t.Fatalf("expected AS15169 Google LLC with 2 hits, got %+v", result.TopASNs)
	}

	filtered, err := store.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID:  site.ID,
		UserID:  userID,
		Start:   base.Add(-24 * time.Hour),
		End:     base,
		Filters: []api.Filter{{Type: "provider", Value: "Google LLC"}},
	})
	if err != nil {
		t.Fatalf("GetSiteStats with provider filter: %v", err)
	}
	if filtered.TotalPageviews != 2 {
		t.Fatalf("expected provider filter to return 2 pageviews, got %d", filtered.TotalPageviews)
	}
}

func containsMetric(metrics []api.MetricStat, name string, value int) bool {
	for _, metric := range metrics {
		if metric.Name == name && metric.Value == value {
			return true
		}
	}
	return false
}
