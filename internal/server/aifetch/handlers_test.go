package aifetch

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/blocking"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

func setupAIFetchTestEnv(t *testing.T) (*database.Store, *shared.Context, uuid.UUID, uuid.UUID, string) {
	t.Helper()

	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	userID, err := store.CreateUser(context.Background(), "ai-fetch@example.com", "hashed")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "aifetch.example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}
	_, token, err := store.CreateAPIClient(context.Background(), userID, "AI Fetch", "test", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		site.ID: auth.SiteOwner,
	}, nil)
	if err != nil {
		t.Fatalf("CreateAPIClient: %v", err)
	}

	ctx := &shared.Context{
		Store:          store,
		Config:         &config.Config{},
		SystemCounters: &database.SystemCounter{},
	}

	return store, ctx, userID, site.ID, token
}

func TestHandleCreateAIFetchAcceptsKnownBot(t *testing.T) {
	store, ctx, _, siteID, token := setupAIFetchTestEnv(t)

	mux := http.NewServeMux()
	Register(mux, ctx)

	body := map[string]any{
		"path":         "https://docs.example.com/guides/ai?from=bot#fragment",
		"status_code":  200,
		"content_type": "application/pdf",
		"response_ms":  184,
		"bytes_served": 4096,
		"user_agent":   "Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/ingest/ai-fetch", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", token)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}

	overview, err := store.GetAIFetchOverview(context.Background(), api.AIFetchQueryParams{
		SiteID: siteID,
		Start:  time.Now().UTC().Add(-time.Hour),
		End:    time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetAIFetchOverview: %v", err)
	}
	if overview.TotalRequests != 1 {
		t.Fatalf("expected 1 stored fetch, got %d", overview.TotalRequests)
	}
	// Fragments are client-side only, so normalizeFetchTarget strips them while preserving query strings.
	if !containsMetric(overview.TopPaths, "/guides/ai?from=bot", 1) {
		t.Fatalf("expected normalized path in top paths, got %+v", overview.TopPaths)
	}
	if !containsMetric(overview.TopAssistants, "GPTBot", 1) {
		t.Fatalf("expected GPTBot in top assistants, got %+v", overview.TopAssistants)
	}
}

func TestHandleCreateAIFetchRejectsUnknownBot(t *testing.T) {
	_, ctx, _, siteID, token := setupAIFetchTestEnv(t)

	mux := http.NewServeMux()
	Register(mux, ctx)

	body := map[string]any{
		"path":        "/docs",
		"status_code": 200,
		"user_agent":  "Mozilla/5.0 (compatible; GenericCrawler/1.0)",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/ingest/ai-fetch", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", token)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
	if got := ctx.SystemCounters.Rejections.Load(); got != 1 {
		t.Fatalf("expected rejection counter 1, got %d", got)
	}
}

func TestHandleCreateAIFetchRejectsDeletedSite(t *testing.T) {
	store, ctx, _, siteID, _ := setupAIFetchTestEnv(t)

	if err := store.DeleteSite(context.Background(), siteID); err != nil {
		t.Fatalf("DeleteSite: %v", err)
	}

	h := &handler{ctx: ctx}

	body := map[string]any{
		"path":        "/docs",
		"status_code": 200,
		"user_agent":  "Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/ingest/ai-fetch", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", siteID.String())

	rec := httptest.NewRecorder()
	h.handleCreateAIFetch().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d body=%s", http.StatusNotFound, rec.Code, rec.Body.String())
	}
	if got := ctx.SystemCounters.Rejections.Load(); got != 1 {
		t.Fatalf("expected rejection counter 1, got %d", got)
	}
}

func TestHandleGetOverviewRejectsInvalidDate(t *testing.T) {
	_, ctx, _, siteID, token := setupAIFetchTestEnv(t)

	mux := http.NewServeMux()
	Register(mux, ctx)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/ai-fetch/overview?from=not-a-date", nil)
	req.Header.Set("X-API-Key", token)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d body=%s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestHandleCreateAIFetchRejectsOversizedBody(t *testing.T) {
	_, ctx, _, siteID, token := setupAIFetchTestEnv(t)

	mux := http.NewServeMux()
	Register(mux, ctx)

	oversizedUA := strings.Repeat("A", 70<<10)
	body := map[string]any{
		"path":        "/docs",
		"status_code": 200,
		"user_agent":  oversizedUA,
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/ingest/ai-fetch", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", token)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d body=%s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestHandleCreateAIFetchDropsBlockedSiteExclusion(t *testing.T) {
	store, ctx, userID, siteID, token := setupAIFetchTestEnv(t)

	if _, err := store.CreateSiteExclusion(context.Background(), siteID, "198.51.100.0/24", "blocked", userID); err != nil {
		t.Fatalf("CreateSiteExclusion: %v", err)
	}
	ipFilter := blocking.NewIPFilter(store)
	if err := ipFilter.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh ip filter: %v", err)
	}
	ctx.IPFilter = ipFilter

	mux := http.NewServeMux()
	Register(mux, ctx)

	body := map[string]any{
		"path":        "/docs",
		"status_code": 200,
		"user_agent":  "Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/ingest/ai-fetch", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", token)
	req.RemoteAddr = "198.51.100.42:1234"

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}

	overview, err := store.GetAIFetchOverview(context.Background(), api.AIFetchQueryParams{
		SiteID: siteID,
		Start:  time.Now().UTC().Add(-time.Hour),
		End:    time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetAIFetchOverview: %v", err)
	}
	if overview.TotalRequests != 0 {
		t.Fatalf("expected 0 stored fetches, got %d", overview.TotalRequests)
	}
	if got := ctx.SystemCounters.Rejections.Load(); got != 1 {
		t.Fatalf("expected rejection counter 1, got %d", got)
	}
}

func TestHandleCreateAIFetchDropsCountryExclusion(t *testing.T) {
	store, ctx, userID, siteID, token := setupAIFetchTestEnv(t)

	if _, err := store.CreateInstanceCountryExclusion(context.Background(), "US", "United States", userID); err != nil {
		t.Fatalf("CreateInstanceCountryExclusion: %v", err)
	}
	ipFilter := blocking.NewIPFilter(store)
	if err := ipFilter.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh ip filter: %v", err)
	}
	ctx.IPFilter = ipFilter

	mux := http.NewServeMux()
	Register(mux, ctx)

	body := map[string]any{
		"path":        "/docs",
		"status_code": 200,
		"user_agent":  "Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/ingest/ai-fetch", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", token)
	req.RemoteAddr = "8.8.8.8:1234"

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}

	overview, err := store.GetAIFetchOverview(context.Background(), api.AIFetchQueryParams{
		SiteID: siteID,
		Start:  time.Now().UTC().Add(-time.Hour),
		End:    time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetAIFetchOverview: %v", err)
	}
	if overview.TotalRequests != 0 {
		t.Fatalf("expected 0 stored fetches, got %d", overview.TotalRequests)
	}
}

func TestHandleCreateAIFetchDropsSpamNetwork(t *testing.T) {
	store, ctx, _, siteID, token := setupAIFetchTestEnv(t)

	ctx.SpamFilter = mustNewAIFetchSpamFilter(t, blocking.SpamFeedData{
		NetworkDenylist: []string{"203.0.113.0/24"},
	})

	mux := http.NewServeMux()
	Register(mux, ctx)

	body := map[string]any{
		"path":        "/docs",
		"status_code": 200,
		"user_agent":  "Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/ingest/ai-fetch", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", token)
	req.RemoteAddr = "203.0.113.42:1234"

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}

	overview, err := store.GetAIFetchOverview(context.Background(), api.AIFetchQueryParams{
		SiteID: siteID,
		Start:  time.Now().UTC().Add(-time.Hour),
		End:    time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetAIFetchOverview: %v", err)
	}
	if overview.TotalRequests != 0 {
		t.Fatalf("expected 0 stored fetches, got %d", overview.TotalRequests)
	}
	if got := ctx.SystemCounters.Spam.Load(); got != 1 {
		t.Fatalf("expected spam counter 1, got %d", got)
	}
	if got := ctx.SystemCounters.Rejections.Load(); got != 0 {
		t.Fatalf("expected rejection counter 0, got %d", got)
	}
}

func TestHandleGetOverviewAndTimeseries(t *testing.T) {
	store, ctx, _, siteID, token := setupAIFetchTestEnv(t)

	if err := store.CreateAIFetch(context.Background(), &api.AIFetch{
		SiteID:          siteID,
		Timestamp:       time.Now().UTC().Add(-2 * time.Hour),
		AssistantName:   "GPTBot",
		AssistantFamily: "OpenAI",
		Path:            "/docs",
		StatusCode:      200,
		ResourceType:    "html",
	}); err != nil {
		t.Fatalf("CreateAIFetch: %v", err)
	}

	mux := http.NewServeMux()
	Register(mux, ctx)

	overviewReq := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/ai-fetch/overview", nil)
	overviewReq.Header.Set("X-API-Key", token)
	overviewRec := httptest.NewRecorder()
	mux.ServeHTTP(overviewRec, overviewReq)
	if overviewRec.Code != http.StatusOK {
		t.Fatalf("overview expected %d, got %d body=%s", http.StatusOK, overviewRec.Code, overviewRec.Body.String())
	}

	var overview api.AIFetchOverview
	if err := json.NewDecoder(overviewRec.Body).Decode(&overview); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if overview.TotalRequests != 1 {
		t.Fatalf("expected 1 request in overview, got %d", overview.TotalRequests)
	}

	seriesReq := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/ai-fetch/timeseries", nil)
	seriesReq.Header.Set("X-API-Key", token)
	seriesRec := httptest.NewRecorder()
	mux.ServeHTTP(seriesRec, seriesReq)
	if seriesRec.Code != http.StatusOK {
		t.Fatalf("timeseries expected %d, got %d body=%s", http.StatusOK, seriesRec.Code, seriesRec.Body.String())
	}

	var points []api.AIFetchSeriesPoint
	if err := json.NewDecoder(seriesRec.Body).Decode(&points); err != nil {
		t.Fatalf("decode timeseries: %v", err)
	}
	if len(points) == 0 {
		t.Fatal("expected non-empty timeseries")
	}
}

func TestHandleGetCorrelation(t *testing.T) {
	store, ctx, _, siteID, token := setupAIFetchTestEnv(t)
	base := time.Now().UTC()
	aiReferrer := "https://chatgpt.com/c/example"
	isUnique := true

	if err := store.CreateAIFetch(context.Background(), &api.AIFetch{
		SiteID:          siteID,
		Timestamp:       base.Add(-4 * time.Hour),
		AssistantName:   "GPTBot",
		AssistantFamily: "OpenAI",
		Path:            "/docs",
		StatusCode:      200,
		ResourceType:    "html",
	}); err != nil {
		t.Fatalf("CreateAIFetch: %v", err)
	}
	if err := store.CreateHit(context.Background(), &api.Hit{
		SiteID:    siteID,
		SessionID: uuid.Must(uuid.NewV7()),
		PageID:    uuid.Must(uuid.NewV7()),
		Timestamp: base.Add(-2 * time.Hour),
		Path:      "/docs",
		Referrer:  &aiReferrer,
		IsUnique:  &isUnique,
	}); err != nil {
		t.Fatalf("CreateHit: %v", err)
	}

	mux := http.NewServeMux()
	Register(mux, ctx)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/ai-fetch/correlation?window_days=14", nil)
	req.Header.Set("X-API-Key", token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("correlation expected %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var report api.AIFetchCorrelationReport
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatalf("decode correlation: %v", err)
	}
	if report.Summary.TotalFetches != 1 {
		t.Fatalf("expected 1 total fetch, got %d", report.Summary.TotalFetches)
	}
	if len(report.CitationYield) == 0 {
		t.Fatal("expected citation yield rows")
	}
}

func TestHandleGetCorrelationRejectsInvalidWindowDays(t *testing.T) {
	_, ctx, _, siteID, token := setupAIFetchTestEnv(t)

	mux := http.NewServeMux()
	Register(mux, ctx)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/ai-fetch/correlation?window_days=0", nil)
	req.Header.Set("X-API-Key", token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d body=%s", http.StatusBadRequest, rec.Code, rec.Body.String())
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

func mustNewAIFetchSpamFilter(t *testing.T, data blocking.SpamFeedData) *blocking.SpamFilter {
	t.Helper()

	path := t.TempDir() + "/spam-filter.json"
	if err := blocking.SaveSpamFeedData(path, data); err != nil {
		t.Fatalf("save spam filter data: %v", err)
	}

	filter := blocking.NewSpamFilter(path)
	if err := filter.RefreshFromDisk(); err != nil {
		t.Fatalf("refresh spam filter from disk: %v", err)
	}

	return filter
}
