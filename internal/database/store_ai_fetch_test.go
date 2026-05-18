package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestAIFetchOverviewAndTimeseries(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "ai-fetch.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	htmlType := "text/html; charset=utf-8"
	pdfType := "application/pdf"
	imageType := "image/png"
	response120 := 120
	response400 := 400
	bytes1200 := int64(1200)
	bytes2200 := int64(2200)
	bytes512 := int64(512)
	uaGPT := "Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)"
	uaClaude := "Mozilla/5.0 (compatible; ClaudeBot/1.0; +https://anthropic.com/claudebot)"

	records := []*api.AIFetch{
		{
			SiteID:          site.ID,
			Timestamp:       base.Add(-6 * time.Hour),
			AssistantName:   "GPTBot",
			AssistantFamily: "OpenAI",
			Path:            "/docs",
			StatusCode:      200,
			ContentType:     &htmlType,
			ResourceType:    "html",
			ResponseMs:      &response120,
			BytesServed:     &bytes1200,
			UserAgent:       &uaGPT,
		},
		{
			SiteID:          site.ID,
			Timestamp:       base.Add(-5 * time.Hour),
			AssistantName:   "GPTBot",
			AssistantFamily: "OpenAI",
			Path:            "/docs/setup",
			StatusCode:      404,
			ContentType:     &pdfType,
			ResourceType:    "document",
			ResponseMs:      &response400,
			BytesServed:     &bytes2200,
			UserAgent:       &uaGPT,
		},
		{
			SiteID:          site.ID,
			Timestamp:       base.Add(-2 * time.Hour),
			AssistantName:   "ClaudeBot",
			AssistantFamily: "Anthropic",
			Path:            "/images/hero.png",
			StatusCode:      502,
			ContentType:     &imageType,
			ResourceType:    "image",
			BytesServed:     &bytes512,
			UserAgent:       &uaClaude,
		},
	}

	for _, record := range records {
		if err := store.CreateAIFetch(ctx, record); err != nil {
			t.Fatalf("CreateAIFetch: %v", err)
		}
	}

	params := api.AIFetchQueryParams{
		SiteID: site.ID,
		Start:  base.Add(-24 * time.Hour),
		End:    base,
	}

	overview, err := store.GetAIFetchOverview(ctx, params)
	if err != nil {
		t.Fatalf("GetAIFetchOverview: %v", err)
	}

	if overview.TotalRequests != 3 {
		t.Fatalf("expected 3 requests, got %d", overview.TotalRequests)
	}
	if overview.UniquePaths != 3 {
		t.Fatalf("expected 3 unique paths, got %d", overview.UniquePaths)
	}
	if overview.UniqueAssistants != 2 {
		t.Fatalf("expected 2 unique assistants, got %d", overview.UniqueAssistants)
	}
	if !containsMetric(overview.TopAssistants, "GPTBot", 2) {
		t.Fatalf("expected GPTBot top assistant, got %+v", overview.TopAssistants)
	}
	if !containsMetric(overview.TopFamilies, "OpenAI", 2) {
		t.Fatalf("expected OpenAI top family, got %+v", overview.TopFamilies)
	}
	if !containsMetric(overview.TopErrorPaths, "/docs/setup", 1) || !containsMetric(overview.TopErrorPaths, "/images/hero.png", 1) {
		t.Fatalf("expected error paths, got %+v", overview.TopErrorPaths)
	}
	if !containsMetric(overview.ResourceTypeSplit, "html", 1) || !containsMetric(overview.ResourceTypeSplit, "document", 1) || !containsMetric(overview.ResourceTypeSplit, "image", 1) {
		t.Fatalf("expected resource type split, got %+v", overview.ResourceTypeSplit)
	}

	points, err := store.GetAIFetchTimeseries(ctx, params)
	if err != nil {
		t.Fatalf("GetAIFetchTimeseries: %v", err)
	}
	if len(points) == 0 {
		t.Fatal("expected non-empty timeseries")
	}

	filtered, err := store.GetAIFetchOverview(ctx, api.AIFetchQueryParams{
		SiteID:          site.ID,
		Start:           params.Start,
		End:             params.End,
		AssistantFamily: "OpenAI",
	})
	if err != nil {
		t.Fatalf("GetAIFetchOverview filtered: %v", err)
	}
	if filtered.TotalRequests != 2 {
		t.Fatalf("expected 2 filtered requests, got %d", filtered.TotalRequests)
	}

	pathFiltered, err := store.GetAIFetchOverview(ctx, api.AIFetchQueryParams{
		SiteID: site.ID,
		Start:  params.Start,
		End:    params.End,
		Path:   "/docs",
	})
	if err != nil {
		t.Fatalf("GetAIFetchOverview path filtered: %v", err)
	}
	if pathFiltered.TotalRequests != 1 || pathFiltered.UniquePaths != 1 {
		t.Fatalf("expected /docs path filter to return 1 request and 1 path, got requests=%d paths=%d", pathFiltered.TotalRequests, pathFiltered.UniquePaths)
	}
}

func TestAIFetchCorrelation(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "ai-correlation.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	aiReferrer := "https://chatgpt.com/c/abc123"
	isUnique := true

	records := []*api.AIFetch{
		{SiteID: site.ID, Timestamp: base.Add(-48 * time.Hour), AssistantName: "GPTBot", AssistantFamily: "OpenAI", Path: "/docs", StatusCode: 200, ResourceType: "html"},
		{SiteID: site.ID, Timestamp: base.Add(-36 * time.Hour), AssistantName: "GPTBot", AssistantFamily: "OpenAI", Path: "/docs", StatusCode: 404, ResourceType: "html"},
		{SiteID: site.ID, Timestamp: base.Add(-24 * time.Hour), AssistantName: "ClaudeBot", AssistantFamily: "Anthropic", Path: "/pricing", StatusCode: 502, ResourceType: "html"},
		{SiteID: site.ID, Timestamp: base.Add(-12 * time.Hour), AssistantName: "ClaudeBot", AssistantFamily: "Anthropic", Path: "/orphan", StatusCode: 200, ResourceType: "html"},
	}
	for _, record := range records {
		if err := store.CreateAIFetch(ctx, record); err != nil {
			t.Fatalf("CreateAIFetch: %v", err)
		}
	}

	hits := []*api.Hit{
		{SiteID: site.ID, SessionID: mustUUID(t), PageID: mustUUID(t), Timestamp: base.Add(-18 * time.Hour), Path: "/docs", Referrer: &aiReferrer, IsUnique: &isUnique},
		{SiteID: site.ID, SessionID: mustUUID(t), PageID: mustUUID(t), Timestamp: base.Add(-6 * time.Hour), Path: "/pricing", Referrer: &aiReferrer, IsUnique: &isUnique},
		{SiteID: site.ID, SessionID: mustUUID(t), PageID: mustUUID(t), Timestamp: base.Add(-4 * time.Hour), Path: "/docs", Referrer: &aiReferrer, IsUnique: &isUnique},
	}
	for _, hit := range hits {
		if err := store.CreateHit(ctx, hit); err != nil {
			t.Fatalf("CreateHit: %v", err)
		}
	}

	report, err := store.GetAIFetchCorrelation(ctx, api.AIFetchCorrelationParams{
		SiteID:     site.ID,
		Start:      base.Add(-72 * time.Hour),
		End:        base,
		WindowDays: 30,
	})
	if err != nil {
		t.Fatalf("GetAIFetchCorrelation: %v", err)
	}

	if report.Summary.TotalFetches != 4 {
		t.Fatalf("expected 4 total fetches, got %d", report.Summary.TotalFetches)
	}
	if report.Summary.CorrelatedPaths != 2 {
		t.Fatalf("expected 2 correlated paths, got %d", report.Summary.CorrelatedPaths)
	}
	if report.Summary.AIReferredVisits != 3 {
		t.Fatalf("expected 3 AI referred visits, got %d", report.Summary.AIReferredVisits)
	}
	if report.Summary.UncorrelatedFetches != 1 {
		t.Fatalf("expected 1 uncorrelated fetch, got %d", report.Summary.UncorrelatedFetches)
	}
	if len(report.CitationYield) == 0 {
		t.Fatal("expected citation yield rows")
	}
	if report.CitationYield[0].Path != "/docs" || report.CitationYield[0].AssistantName != "GPTBot" {
		t.Fatalf("expected /docs GPTBot to lead citation yield, got %+v", report.CitationYield[0])
	}
	if !containsOpportunity(report.OpportunityPages, "/orphan", 1, 0) {
		t.Fatalf("expected /orphan opportunity page, got %+v", report.OpportunityPages)
	}
	if !containsHotspot(report.FailureHotspots, "ClaudeBot", "/pricing", 1, 1) {
		t.Fatalf("expected ClaudeBot /pricing hotspot, got %+v", report.FailureHotspots)
	}

	pathReport, err := store.GetAIFetchCorrelation(ctx, api.AIFetchCorrelationParams{
		SiteID:     site.ID,
		Start:      base.Add(-72 * time.Hour),
		End:        base,
		Path:       "/docs",
		WindowDays: 30,
	})
	if err != nil {
		t.Fatalf("GetAIFetchCorrelation path filtered: %v", err)
	}
	if pathReport.Summary.TotalFetches != 2 || pathReport.Summary.CorrelatedPaths != 1 {
		t.Fatalf("expected /docs path filter to scope correlation, got summary %+v", pathReport.Summary)
	}
	if len(pathReport.OpportunityPages) != 1 || pathReport.OpportunityPages[0].Path != "/docs" {
		t.Fatalf("expected only /docs opportunity rows after path filter, got %+v", pathReport.OpportunityPages)
	}
}

func containsOpportunity(rows []api.AIFetchOpportunityRow, path string, fetchCount, visits int64) bool {
	for _, row := range rows {
		if row.Path == path && row.FetchCount == fetchCount && row.AIReferredVisits == visits {
			return true
		}
	}
	return false
}

func containsHotspot(rows []api.AIFetchFailureHotspot, assistantName, pathPrefix string, totalRequests, errorRequests int64) bool {
	for _, row := range rows {
		if row.AssistantName == assistantName && row.PathPrefix == pathPrefix && row.TotalRequests == totalRequests && row.ErrorRequests == errorRequests {
			return true
		}
	}
	return false
}

func mustUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	return id
}
