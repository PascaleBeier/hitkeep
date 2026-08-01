package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/aianalytics"
	"hitkeep/internal/api"
)

const (
	uaAIActivityGPT    = "Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)"
	uaAIActivityClaude = "Mozilla/5.0 (compatible; ClaudeBot/1.0; +https://anthropic.com/claudebot)"
	uaAIActivityHuman  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

// aiActivityBase is the fixed end of the reporting range used by every case in
// this file. A 24h window keeps truncUnitForRange on "hour" buckets.
var aiActivityBase = time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)

func createAIActivitySite(t *testing.T, store *Store, userID uuid.UUID, domain string) uuid.UUID {
	t.Helper()
	site, err := store.CreateSite(context.Background(), userID, domain)
	if err != nil {
		t.Fatalf("create site %s: %v", domain, err)
	}
	return site.ID
}

func createAIActivityHits(t *testing.T, store *Store, siteID uuid.UUID, hits []*api.Hit) {
	t.Helper()
	for _, hit := range hits {
		hit.SiteID = siteID
		if hit.SessionID == uuid.Nil {
			hit.SessionID = uuid.New()
		}
		hit.PageID = uuid.New()
		if err := store.CreateHit(context.Background(), hit); err != nil {
			t.Fatalf("CreateHit %s: %v", hit.Path, err)
		}
	}
}

func createAIActivityFetches(t *testing.T, store *Store, siteID uuid.UUID, fetches []*api.AIFetch) {
	t.Helper()
	for _, fetch := range fetches {
		fetch.SiteID = siteID
	}
	if err := store.CreateAIFetchesBulk(context.Background(), fetches); err != nil {
		t.Fatalf("CreateAIFetchesBulk: %v", err)
	}
}

func aiActivityFetch(ts time.Time, name, family, category, path string, status int, responseMs int, bytes int64) *api.AIFetch {
	return &api.AIFetch{
		Timestamp:         ts,
		AssistantName:     name,
		AssistantFamily:   family,
		AssistantCategory: category,
		Path:              path,
		StatusCode:        status,
		ResourceType:      "html",
		ResponseMs:        new(responseMs),
		BytesServed:       new(bytes),
	}
}

// seedAIActivityFixture writes the shared merged-report fixture: tracked AI bot
// hits, one AI-referred session, one organic hit, and five fetch records — one
// of which is a legacy row with no assistant_category.
func seedAIActivityFixture(t *testing.T, store *Store, userID uuid.UUID, domain string) uuid.UUID {
	t.Helper()
	siteID := createAIActivitySite(t, store, userID, domain)

	base := aiActivityBase
	referralSession := uuid.New()
	refChatGPT := "https://chatgpt.com/"
	refOrganic := "https://news.ycombinator.com/"

	createAIActivityHits(t, store, siteID, []*api.Hit{
		{Timestamp: base.Add(-6 * time.Hour), Path: "/docs", UserAgent: new(uaAIActivityGPT)},
		{Timestamp: base.Add(-6*time.Hour + 5*time.Minute), Path: "/docs", UserAgent: new(uaAIActivityGPT)},
		{Timestamp: base.Add(-2 * time.Hour), Path: "/pricing", UserAgent: new(uaAIActivityGPT)},
		{Timestamp: base.Add(-3 * time.Hour), Path: "/docs", UserAgent: new(uaAIActivityClaude)},
		{SessionID: referralSession, Timestamp: base.Add(-4 * time.Hour), Path: "/", UserAgent: new(uaAIActivityHuman), Referrer: &refChatGPT},
		{SessionID: referralSession, Timestamp: base.Add(-4*time.Hour + 2*time.Minute), Path: "/pricing", UserAgent: new(uaAIActivityHuman), Referrer: &refChatGPT},
		{Timestamp: base.Add(-5 * time.Hour), Path: "/", UserAgent: new(uaAIActivityHuman), Referrer: &refOrganic},
		// Comparison window: strictly before the main range.
		{Timestamp: base.Add(-40 * time.Hour), Path: "/docs", UserAgent: new(uaAIActivityGPT)},
		{Timestamp: base.Add(-39 * time.Hour), Path: "/docs", UserAgent: new(uaAIActivityGPT)},
		{Timestamp: base.Add(-38 * time.Hour), Path: "/", UserAgent: new(uaAIActivityHuman), Referrer: &refChatGPT},
	})

	createAIActivityFetches(t, store, siteID, []*api.AIFetch{
		aiActivityFetch(base.Add(-6*time.Hour), "GPTBot", "OpenAI", aianalytics.CategoryTrainingCrawler, "/docs", 200, 100, 1000),
		aiActivityFetch(base.Add(-5*time.Hour), "GPTBot", "OpenAI", aianalytics.CategoryTrainingCrawler, "/docs", 200, 300, 2000),
		aiActivityFetch(base.Add(-4*time.Hour), "GPTBot", "OpenAI", aianalytics.CategoryTrainingCrawler, "/missing", 404, 50, 500),
		// Legacy row: ingested before assistant_category existed.
		aiActivityFetch(base.Add(-3*time.Hour), "ClaudeBot", "Anthropic", "", "/docs", 200, 200, 1500),
		aiActivityFetch(base.Add(-1*time.Hour), "PerplexityBot", "Perplexity", aianalytics.CategorySearchIndexer, "/pricing", 500, 400, 100),
		// Comparison window.
		aiActivityFetch(base.Add(-30*time.Hour), "GPTBot", "OpenAI", aianalytics.CategoryTrainingCrawler, "/docs", 200, 120, 900),
	})

	return siteID
}

func aiActivityParams(siteID uuid.UUID, filters ...api.Filter) api.AnalyticsParams {
	return api.AnalyticsParams{
		SiteID:  siteID,
		Start:   aiActivityBase.Add(-24 * time.Hour),
		End:     aiActivityBase,
		Filters: filters,
	}
}

func findAIActivityStat(t *testing.T, rows []api.AIActivityStat, name string) api.AIActivityStat {
	t.Helper()
	for _, row := range rows {
		if row.Name == name {
			return row
		}
	}
	t.Fatalf("expected a row named %q, got %+v", name, rows)
	return api.AIActivityStat{}
}

func requireAIActivityStat(t *testing.T, rows []api.AIActivityStat, name string, wantValue, wantTracked, wantFetched int) {
	t.Helper()
	row := findAIActivityStat(t, rows, name)
	if row.Value != wantValue || row.TrackedHits != wantTracked || row.FetchCount != wantFetched {
		t.Fatalf("row %q = {value:%d tracked:%d fetched:%d}, want {value:%d tracked:%d fetched:%d}",
			name, row.Value, row.TrackedHits, row.FetchCount, wantValue, wantTracked, wantFetched)
	}
}

func TestGetAIActivityMergesTrackedHitsAndFetches(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()
	siteID := seedAIActivityFixture(t, store, userID, "ai-activity-merged.example.com")

	report, err := store.GetAIActivity(ctx, aiActivityParams(siteID))
	if err != nil {
		t.Fatalf("GetAIActivity: %v", err)
	}

	if report.TrackedHits != 4 {
		t.Fatalf("tracked_hits = %d, want 4", report.TrackedHits)
	}
	if report.FetchCount != 5 {
		t.Fatalf("fetch_count = %d, want 5", report.FetchCount)
	}
	if report.AIRequests != 9 {
		t.Fatalf("ai_requests = %d, want 9 (tracked hits + fetch records)", report.AIRequests)
	}
	if report.ReferralVisits != 1 {
		t.Fatalf("referral_visits = %d, want 1 distinct AI-referred session", report.ReferralVisits)
	}
	if report.PathsCrawled != 3 {
		t.Fatalf("paths_crawled = %d, want 3 (/docs, /pricing, /missing)", report.PathsCrawled)
	}
	if report.UniqueAgents != 3 {
		t.Fatalf("unique_agents = %d, want 3", report.UniqueAgents)
	}
	if report.Pageviews != 7 {
		t.Fatalf("pageviews = %d, want 7 hits in range", report.Pageviews)
	}

	// Fetch-only depth scalars.
	if report.ErrorRate4xx != 20 {
		t.Fatalf("error_rate_4xx = %v, want 20", report.ErrorRate4xx)
	}
	if report.ErrorRate5xx != 20 {
		t.Fatalf("error_rate_5xx = %v, want 20", report.ErrorRate5xx)
	}
	if report.MedianResponseMs != 200 {
		t.Fatalf("median_response_ms = %d, want 200", report.MedianResponseMs)
	}
	if report.TotalBytes != 5100 {
		t.Fatalf("total_bytes = %d, want 5100", report.TotalBytes)
	}

	// Agents merge both provenances into one row.
	requireAIActivityStat(t, report.TopAgents, "GPTBot", 6, 3, 3)
	requireAIActivityStat(t, report.TopAgents, "ClaudeBot", 2, 1, 1)
	requireAIActivityStat(t, report.TopAgents, "PerplexityBot", 1, 0, 1)

	// The legacy NULL-category fetch row still lands in a category through the
	// hk_ai_bot_category_from_name fallback: 4 fetched, not 3.
	requireAIActivityStat(t, report.TopCategories, aianalytics.CategoryTrainingCrawler, 8, 4, 4)
	requireAIActivityStat(t, report.TopCategories, aianalytics.CategorySearchIndexer, 1, 0, 1)

	requireAIActivityStat(t, report.TopPaths, "/docs", 6, 3, 3)
	requireAIActivityStat(t, report.TopPaths, "/pricing", 2, 1, 1)
	requireAIActivityStat(t, report.TopPaths, "/missing", 1, 0, 1)

	requireAIActivityStat(t, report.TopSources, "ChatGPT", 1, 1, 0)
	requireAIActivityStat(t, report.TopFamilies, "OpenAI", 3, 0, 3)
	requireAIActivityStat(t, report.TopResourceTypes, "html", 5, 0, 5)
	requireAIActivityStat(t, report.TopErrorPaths, "/missing", 1, 0, 1)
	requireAIActivityStat(t, report.TopErrorPaths, "/pricing", 1, 0, 1)

	// Per-category agent lists merge both sides too.
	training := report.TopAgentsByCategory[aianalytics.CategoryTrainingCrawler]
	requireAIActivityStat(t, training, "GPTBot", 6, 3, 3)
	requireAIActivityStat(t, training, "ClaudeBot", 2, 1, 1)
	indexers := report.TopAgentsByCategory[aianalytics.CategorySearchIndexer]
	requireAIActivityStat(t, indexers, "PerplexityBot", 1, 0, 1)

	if report.Comparison != nil {
		t.Fatalf("comparison must be absent without CompareStart, got %+v", report.Comparison)
	}
}

func TestGetAIActivityFilters(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()
	siteID := seedAIActivityFixture(t, store, userID, "ai-activity-filters.example.com")

	t.Run("ai_bot maps to assistant_name", func(t *testing.T) {
		report, err := store.GetAIActivity(ctx, aiActivityParams(siteID, api.Filter{Type: "ai_bot", Value: "GPTBot"}))
		if err != nil {
			t.Fatalf("GetAIActivity: %v", err)
		}
		if report.TrackedHits != 3 || report.FetchCount != 3 || report.AIRequests != 6 {
			t.Fatalf("GPTBot filter = tracked %d fetched %d ai_requests %d, want 3/3/6", report.TrackedHits, report.FetchCount, report.AIRequests)
		}
		if report.ReferralVisits != 0 {
			t.Fatalf("referral_visits = %d, want 0 under an ai_bot filter", report.ReferralVisits)
		}
		if len(report.TopAgents) != 1 {
			t.Fatalf("expected only GPTBot in top_agents, got %+v", report.TopAgents)
		}
	})

	t.Run("ai_bot_category maps through the fetch fallback", func(t *testing.T) {
		report, err := store.GetAIActivity(ctx, aiActivityParams(siteID, api.Filter{Type: "ai_bot_category", Value: aianalytics.CategoryTrainingCrawler}))
		if err != nil {
			t.Fatalf("GetAIActivity: %v", err)
		}
		if report.TrackedHits != 4 || report.FetchCount != 4 {
			t.Fatalf("training-crawler filter = tracked %d fetched %d, want 4/4", report.TrackedHits, report.FetchCount)
		}
	})

	t.Run("path narrows both sides", func(t *testing.T) {
		report, err := store.GetAIActivity(ctx, aiActivityParams(siteID, api.Filter{Type: "path", Value: "/docs"}))
		if err != nil {
			t.Fatalf("GetAIActivity: %v", err)
		}
		if report.TrackedHits != 3 || report.FetchCount != 3 {
			t.Fatalf("/docs filter = tracked %d fetched %d, want 3/3", report.TrackedHits, report.FetchCount)
		}
		if report.PathsCrawled != 1 {
			t.Fatalf("paths_crawled = %d, want 1 under a path filter", report.PathsCrawled)
		}
	})

	t.Run("ai_source zeroes the fetch side", func(t *testing.T) {
		report, err := store.GetAIActivity(ctx, aiActivityParams(siteID, api.Filter{Type: "ai_source", Value: "ChatGPT"}))
		if err != nil {
			t.Fatalf("GetAIActivity: %v", err)
		}
		if report.FetchCount != 0 {
			t.Fatalf("fetch_count = %d, want 0: ai_fetches carry no referrer dimension", report.FetchCount)
		}
		if report.ReferralVisits != 1 {
			t.Fatalf("referral_visits = %d, want 1", report.ReferralVisits)
		}
		if report.TotalBytes != 0 || report.MedianResponseMs != 0 {
			t.Fatalf("fetch depth scalars must be zero when the fetch side is excluded, got bytes %d median %d", report.TotalBytes, report.MedianResponseMs)
		}
	})

	t.Run("pageviews ignore AI filters but honor others", func(t *testing.T) {
		report, err := store.GetAIActivity(ctx, aiActivityParams(siteID, api.Filter{Type: "ai_bot", Value: "GPTBot"}))
		if err != nil {
			t.Fatalf("GetAIActivity: %v", err)
		}
		if report.Pageviews != 7 {
			t.Fatalf("pageviews = %d, want 7: the AI dimension must not narrow the denominator", report.Pageviews)
		}

		scoped, err := store.GetAIActivity(ctx, aiActivityParams(siteID, api.Filter{Type: "path", Value: "/docs"}))
		if err != nil {
			t.Fatalf("GetAIActivity: %v", err)
		}
		if scoped.Pageviews != 3 {
			t.Fatalf("pageviews = %d, want 3: non-AI filters still apply to the denominator", scoped.Pageviews)
		}
	})
}

func TestGetAIActivityAppliesConversionCohortsToTrackedRowsOnly(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()
	siteID := createAIActivitySite(t, store, userID, "ai-activity-cohort.example.com")
	base := aiActivityBase

	matchedSession := uuid.New()
	unmatchedSession := uuid.New()
	createAIActivityHits(t, store, siteID, []*api.Hit{
		{SessionID: matchedSession, Timestamp: base.Add(-2 * time.Hour), Path: "/signup", UserAgent: new(uaAIActivityGPT)},
		{SessionID: matchedSession, Timestamp: base.Add(-time.Hour), Path: "/after", UserAgent: new(uaAIActivityHuman)},
		{SessionID: unmatchedSession, Timestamp: base.Add(-90 * time.Minute), Path: "/other", UserAgent: new(uaAIActivityClaude)},
		// A matching session in the comparison window proves that the cohort is
		// rebuilt for the comparison range rather than copied from the current one.
		{SessionID: matchedSession, Timestamp: base.Add(-30 * time.Hour), Path: "/signup", UserAgent: new(uaAIActivityGPT)},
		{SessionID: unmatchedSession, Timestamp: base.Add(-31 * time.Hour), Path: "/other", UserAgent: new(uaAIActivityClaude)},
	})
	createAIActivityFetches(t, store, siteID, []*api.AIFetch{
		aiActivityFetch(base.Add(-2*time.Hour), "GPTBot", "OpenAI", aianalytics.CategoryTrainingCrawler, "/signup", 200, 100, 100),
		aiActivityFetch(base.Add(-30*time.Hour), "ClaudeBot", "Anthropic", aianalytics.CategoryTrainingCrawler, "/other", 200, 100, 100),
	})
	if err := store.CreateEvent(ctx, &api.Event{SiteID: siteID, SessionID: matchedSession, Name: "signup_completed", Timestamp: base.Add(-30 * time.Minute)}); err != nil {
		t.Fatalf("create current goal event: %v", err)
	}
	if err := store.CreateEvent(ctx, &api.Event{SiteID: siteID, SessionID: matchedSession, Name: "signup_completed", Timestamp: base.Add(-29 * time.Hour)}); err != nil {
		t.Fatalf("create comparison goal event: %v", err)
	}
	goal := api.Goal{SiteID: siteID, Name: "Signup", Type: "event", Value: "signup_completed"}
	if err := store.CreateGoal(ctx, &goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	params := aiActivityParams(siteID)
	params.GoalIDs = []uuid.UUID{goal.ID}
	params.CompareStart = base.Add(-48 * time.Hour)
	params.CompareEnd = base.Add(-24 * time.Hour)
	report, err := store.GetAIActivity(ctx, params)
	if err != nil {
		t.Fatalf("GetAIActivity: %v", err)
	}
	if report.TrackedHits != 1 || report.FetchCount != 1 || report.Pageviews != 2 {
		t.Fatalf("current cohort totals tracked %d fetched %d pageviews %d, want 1/1/2", report.TrackedHits, report.FetchCount, report.Pageviews)
	}
	var seriesTracked, seriesFetched int
	for _, point := range report.Series {
		seriesTracked += point.TrackedHits
		seriesFetched += point.FetchCount
	}
	if seriesTracked != 1 || seriesFetched != 1 {
		t.Fatalf("current series totals tracked %d fetched %d, want 1/1", seriesTracked, seriesFetched)
	}
	if report.Comparison == nil || report.Comparison.TrackedHits != 1 || report.Comparison.FetchCount != 1 || report.Comparison.Pageviews != 1 {
		t.Fatalf("comparison cohort = %+v, want tracked 1 fetched 1 pageviews 1", report.Comparison)
	}

	funnel := api.Funnel{SiteID: siteID, Name: "Signup funnel", Steps: []api.FunnelStep{{Type: "event", Value: "signup_completed"}}}
	if err := store.CreateFunnel(ctx, &funnel); err != nil {
		t.Fatalf("create funnel: %v", err)
	}
	funnelParams := aiActivityParams(siteID)
	funnelParams.FunnelIDs = []uuid.UUID{funnel.ID}
	funnelReport, err := store.GetAIActivity(ctx, funnelParams)
	if err != nil {
		t.Fatalf("GetAIActivity funnel cohort: %v", err)
	}
	if funnelReport.TrackedHits != 1 || funnelReport.FetchCount != 1 || funnelReport.Pageviews != 2 {
		t.Fatalf("funnel cohort totals tracked %d fetched %d pageviews %d, want 1/1/2", funnelReport.TrackedHits, funnelReport.FetchCount, funnelReport.Pageviews)
	}
}

func TestGetAIActivityPathsCrawledCountsEveryMergedPath(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()
	siteID := createAIActivitySite(t, store, userID, "ai-activity-paths.example.com")

	base := aiActivityBase
	fetches := make([]*api.AIFetch, 0, 12)
	for i := range 12 {
		path := "/crawled/" + string(rune('a'+i))
		fetches = append(fetches, aiActivityFetch(base.Add(-time.Duration(i+1)*time.Hour), "GPTBot", "OpenAI", aianalytics.CategoryTrainingCrawler, path, 200, 100, 100))
	}
	createAIActivityFetches(t, store, siteID, fetches)
	createAIActivityHits(t, store, siteID, []*api.Hit{
		{Timestamp: base.Add(-2 * time.Hour), Path: "/tracked-only", UserAgent: new(uaAIActivityGPT)},
	})

	report, err := store.GetAIActivity(ctx, aiActivityParams(siteID))
	if err != nil {
		t.Fatalf("GetAIActivity: %v", err)
	}
	if report.PathsCrawled != 13 {
		t.Fatalf("paths_crawled = %d, want 13: the count must not be a top-10 artifact", report.PathsCrawled)
	}
	if len(report.TopPaths) != 10 {
		t.Fatalf("top_paths must stay capped at 10, got %d", len(report.TopPaths))
	}
}

func TestGetAIActivityWithoutFetchesZeroesDepthScalars(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()
	siteID := createAIActivitySite(t, store, userID, "ai-activity-hits-only.example.com")

	createAIActivityHits(t, store, siteID, []*api.Hit{
		{Timestamp: aiActivityBase.Add(-2 * time.Hour), Path: "/docs", UserAgent: new(uaAIActivityGPT)},
	})

	report, err := store.GetAIActivity(ctx, aiActivityParams(siteID))
	if err != nil {
		t.Fatalf("GetAIActivity: %v", err)
	}
	if report.TrackedHits != 1 || report.FetchCount != 0 || report.AIRequests != 1 {
		t.Fatalf("tracked %d fetched %d ai_requests %d, want 1/0/1", report.TrackedHits, report.FetchCount, report.AIRequests)
	}
	if report.ErrorRate4xx != 0 || report.ErrorRate5xx != 0 || report.MedianResponseMs != 0 || report.TotalBytes != 0 {
		t.Fatalf("expected zeroed fetch depth scalars, got %+v", report)
	}
	if report.TopFamilies == nil || report.TopResourceTypes == nil || report.TopErrorPaths == nil {
		t.Fatalf("fetch-only top lists must be empty slices rather than nil: %+v", report)
	}
	if len(report.TopFamilies) != 0 || len(report.TopResourceTypes) != 0 {
		t.Fatalf("expected empty fetch-only top lists, got families %+v resource types %+v", report.TopFamilies, report.TopResourceTypes)
	}
}

func TestGetAIActivityWithFetchesOnlyKeepsUnifiedRows(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()
	siteID := createAIActivitySite(t, store, userID, "ai-activity-fetches-only.example.com")
	createAIActivityFetches(t, store, siteID, []*api.AIFetch{
		aiActivityFetch(aiActivityBase.Add(-2*time.Hour), "GPTBot", "OpenAI", aianalytics.CategoryTrainingCrawler, "/docs", 200, 100, 100),
	})

	report, err := store.GetAIActivity(ctx, aiActivityParams(siteID))
	if err != nil {
		t.Fatalf("GetAIActivity: %v", err)
	}
	if report.TrackedHits != 0 || report.FetchCount != 1 || report.AIRequests != 1 || report.Pageviews != 0 {
		t.Fatalf("fetch-only totals tracked %d fetched %d ai_requests %d pageviews %d, want 0/1/1/0", report.TrackedHits, report.FetchCount, report.AIRequests, report.Pageviews)
	}
	requireAIActivityStat(t, report.TopAgents, "GPTBot", 1, 0, 1)
}

func TestGetAIActivitySeriesMergesBothSides(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()
	siteID := seedAIActivityFixture(t, store, userID, "ai-activity-series.example.com")

	params := aiActivityParams(siteID)
	report, err := store.GetAIActivity(ctx, params)
	if err != nil {
		t.Fatalf("GetAIActivity: %v", err)
	}

	truncUnit := truncUnitForRange(params.Start, params.End)
	wantBuckets := buildSeriesBuckets(params.Start, params.End, truncUnit)
	if len(report.Series) != len(wantBuckets) {
		t.Fatalf("expected %d gap-filled buckets, got %d", len(wantBuckets), len(report.Series))
	}
	for i, bucket := range wantBuckets {
		if !report.Series[i].Time.Equal(bucket) {
			t.Fatalf("bucket %d = %s, want %s", i, report.Series[i].Time, bucket)
		}
	}

	var tracked, fetched, requests, referrals int
	for _, point := range report.Series {
		tracked += point.TrackedHits
		fetched += point.FetchCount
		requests += point.AIRequests
		referrals += point.ReferralVisits
	}
	if tracked != 4 || fetched != 5 || requests != 9 {
		t.Fatalf("series totals tracked %d fetched %d ai_requests %d, want 4/5/9", tracked, fetched, requests)
	}
	if referrals != 1 {
		t.Fatalf("series referral_visits total = %d, want 1", referrals)
	}
}

func TestGetAIActivityComparison(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()
	siteID := seedAIActivityFixture(t, store, userID, "ai-activity-comparison.example.com")

	params := aiActivityParams(siteID)
	params.CompareStart = aiActivityBase.Add(-48 * time.Hour)
	params.CompareEnd = aiActivityBase.Add(-24 * time.Hour)

	report, err := store.GetAIActivity(ctx, params)
	if err != nil {
		t.Fatalf("GetAIActivity: %v", err)
	}
	if report.Comparison == nil {
		t.Fatal("expected comparison scalars when CompareStart is set")
	}
	if report.Comparison.TrackedHits != 2 || report.Comparison.FetchCount != 1 || report.Comparison.AIRequests != 3 {
		t.Fatalf("comparison tracked %d fetched %d ai_requests %d, want 2/1/3", report.Comparison.TrackedHits, report.Comparison.FetchCount, report.Comparison.AIRequests)
	}
	if report.Comparison.ReferralVisits != 1 {
		t.Fatalf("comparison referral_visits = %d, want 1", report.Comparison.ReferralVisits)
	}
	if report.Comparison.PathsCrawled != 1 || report.Comparison.UniqueAgents != 1 {
		t.Fatalf("comparison paths_crawled %d unique_agents %d, want 1/1", report.Comparison.PathsCrawled, report.Comparison.UniqueAgents)
	}
	if report.Comparison.Pageviews != 3 {
		t.Fatalf("comparison pageviews = %d, want 3", report.Comparison.Pageviews)
	}
	// The current-range scalars must not shift when a comparison is requested.
	if report.AIRequests != 9 {
		t.Fatalf("ai_requests = %d, want 9 with a comparison window set", report.AIRequests)
	}
}

// TestGetAIActivityComparisonNeedsBothEnds guards the half-specified window:
// compare_from/compare_to are parsed leniently, so a malformed compare_to leaves
// a zero end behind. Measuring against [start, zero-time] yields an empty
// baseline that reads as a 100% drop, which is worse than no baseline at all.
func TestGetAIActivityComparisonNeedsBothEnds(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()
	siteID := seedAIActivityFixture(t, store, userID, "ai-activity-half-window.example.com")

	params := aiActivityParams(siteID)
	params.CompareStart = aiActivityBase.Add(-48 * time.Hour)

	report, err := store.GetAIActivity(ctx, params)
	if err != nil {
		t.Fatalf("GetAIActivity: %v", err)
	}
	if report.Comparison != nil {
		t.Fatalf("comparison must be absent when CompareEnd is zero, got %+v", report.Comparison)
	}
}
