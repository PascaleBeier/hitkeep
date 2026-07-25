package sites

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/server/shared"
)

func setupAIActivityHandlerEnv(t *testing.T) (*handler, uuid.UUID, uuid.UUID, time.Time) {
	t.Helper()

	h, store, userID := setupTestEnv(t)
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "ai-activity-handler.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	uaGPT := "Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)"
	uaHuman := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
	refChatGPT := "https://chatgpt.com/"
	for _, hit := range []*api.Hit{
		{SessionID: uuid.New(), PageID: uuid.New(), Path: "/docs", Timestamp: base.Add(-2 * time.Hour), UserAgent: &uaGPT},
		{SessionID: uuid.New(), PageID: uuid.New(), Path: "/", Timestamp: base.Add(-time.Hour), UserAgent: &uaHuman, Referrer: &refChatGPT},
		// Comparison window.
		{SessionID: uuid.New(), PageID: uuid.New(), Path: "/docs", Timestamp: base.Add(-26 * time.Hour), UserAgent: &uaGPT},
	} {
		hit.SiteID = site.ID
		if err := store.CreateHit(ctx, hit); err != nil {
			t.Fatalf("create hit %s: %v", hit.Path, err)
		}
	}

	responseMs := 120
	bytesServed := int64(2048)
	if err := store.CreateAIFetch(ctx, &api.AIFetch{
		SiteID:            site.ID,
		Timestamp:         base.Add(-3 * time.Hour),
		AssistantName:     "GPTBot",
		AssistantFamily:   "OpenAI",
		AssistantCategory: "ai_training_crawler",
		Path:              "/docs",
		StatusCode:        200,
		ResourceType:      "html",
		ResponseMs:        &responseMs,
		BytesServed:       &bytesServed,
	}); err != nil {
		t.Fatalf("create ai fetch: %v", err)
	}

	return h, site.ID, userID, base
}

func serveAIActivity(t *testing.T, h *handler, siteID uuid.UUID, userID uuid.UUID, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/ai-activity"+query, nil)
	req.SetPathValue("id", siteID.String())
	if userID != uuid.Nil {
		req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
	}

	w := httptest.NewRecorder()
	h.handleGetSiteAIActivity().ServeHTTP(w, req)
	return w
}

func TestHandleGetSiteAIActivity(t *testing.T) {
	h, siteID, userID, base := setupAIActivityHandlerEnv(t)

	from := base.Add(-24 * time.Hour).Format(time.RFC3339)
	to := base.Format(time.RFC3339)
	query := "?from=" + url.QueryEscape(from) + "&to=" + url.QueryEscape(to)

	t.Run("success", func(t *testing.T) {
		w := serveAIActivity(t, h, siteID, userID, query)
		if w.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var report api.AIActivityReport
		if err := json.NewDecoder(w.Body).Decode(&report); err != nil {
			t.Fatalf("decode report: %v", err)
		}
		if report.TrackedHits != 1 || report.FetchCount != 1 || report.AIRequests != 2 {
			t.Fatalf("tracked %d fetched %d ai_requests %d, want 1/1/2", report.TrackedHits, report.FetchCount, report.AIRequests)
		}
		if report.ReferralVisits != 1 {
			t.Fatalf("referral_visits = %d, want 1", report.ReferralVisits)
		}
		if len(report.TopAgents) == 0 || report.TopAgents[0].Name != "GPTBot" {
			t.Fatalf("expected GPTBot to lead top_agents, got %+v", report.TopAgents)
		}
		if len(report.Series) == 0 {
			t.Fatalf("expected a non-empty series")
		}
		if report.Comparison != nil {
			t.Fatalf("comparison must be absent without compare_from, got %+v", report.Comparison)
		}
	})

	t.Run("includes comparison when requested", func(t *testing.T) {
		compare := "&compare_from=" + url.QueryEscape(base.Add(-48*time.Hour).Format(time.RFC3339)) +
			"&compare_to=" + url.QueryEscape(base.Add(-24*time.Hour).Format(time.RFC3339))
		w := serveAIActivity(t, h, siteID, userID, query+compare)
		if w.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var report api.AIActivityReport
		if err := json.NewDecoder(w.Body).Decode(&report); err != nil {
			t.Fatalf("decode report: %v", err)
		}
		if report.Comparison == nil {
			t.Fatal("expected comparison scalars for the previous window")
		}
		if report.Comparison.TrackedHits != 1 || report.Comparison.AIRequests != 1 {
			t.Fatalf("comparison tracked %d ai_requests %d, want 1/1", report.Comparison.TrackedHits, report.Comparison.AIRequests)
		}
	})

	t.Run("honors repeatable filters", func(t *testing.T) {
		w := serveAIActivity(t, h, siteID, userID, query+"&filter=ai_bot:GPTBot&filter=path:/docs")
		if w.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var report api.AIActivityReport
		if err := json.NewDecoder(w.Body).Decode(&report); err != nil {
			t.Fatalf("decode report: %v", err)
		}
		if report.TrackedHits != 1 || report.FetchCount != 1 {
			t.Fatalf("filtered tracked %d fetched %d, want 1/1", report.TrackedHits, report.FetchCount)
		}
		if report.ReferralVisits != 0 {
			t.Fatalf("referral_visits = %d, want 0 under an ai_bot filter", report.ReferralVisits)
		}
	})

	t.Run("rejects invalid filter", func(t *testing.T) {
		w := serveAIActivity(t, h, siteID, userID, "?filter=ai_bogus:x")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
		}
	})

	t.Run("rejects invalid from", func(t *testing.T) {
		w := serveAIActivity(t, h, siteID, userID, "?from=not-a-date")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
		}
	})

	t.Run("rejects invalid site id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/sites/not-a-uuid/ai-activity", nil)
		req.SetPathValue("id", "not-a-uuid")
		req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))

		w := httptest.NewRecorder()
		h.handleGetSiteAIActivity().ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
		}
	})

	t.Run("rejects unauthenticated", func(t *testing.T) {
		w := serveAIActivity(t, h, siteID, uuid.Nil, query)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status %d, got %d: %s", http.StatusUnauthorized, w.Code, w.Body.String())
		}
	})
}
