package share

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/testutil/testdb"
)

func setupShareAIActivityTestEnv(t *testing.T) (*handler, *database.Store, string, uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	store := testdb.Shared(t)

	userID, err := store.CreateUser(ctx, "share-ai-activity@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	site, err := store.CreateSite(ctx, userID, "share-ai-activity.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	uaGPT := "Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)"
	uaHuman := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
	refChatGPT := "https://chatgpt.com/"
	for _, hit := range []*api.Hit{
		{SessionID: uuid.New(), PageID: uuid.New(), Path: "/docs", Timestamp: now.Add(-2 * time.Hour), UserAgent: &uaGPT},
		{SessionID: uuid.New(), PageID: uuid.New(), Path: "/", Timestamp: now.Add(-time.Hour), UserAgent: &uaHuman, Referrer: &refChatGPT},
	} {
		hit.SiteID = site.ID
		if err := store.CreateHit(ctx, hit); err != nil {
			t.Fatalf("create hit %s: %v", hit.Path, err)
		}
	}

	responseMs := 90
	bytesServed := int64(4096)
	if err := store.CreateAIFetch(ctx, &api.AIFetch{
		SiteID:            site.ID,
		Timestamp:         now.Add(-3 * time.Hour),
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

	_, token, err := store.CreateShareLink(ctx, site.ID, userID)
	if err != nil {
		t.Fatalf("create share link: %v", err)
	}

	h := &handler{ctx: newShareTestContext(t, store)}
	return h, store, token, site.ID
}

func TestHandleGetShareAIActivity(t *testing.T) {
	h, store, token, siteID := setupShareAIActivityTestEnv(t)
	t.Cleanup(func() { _ = store.Close() })

	t.Run("valid token includes fetch counts", func(t *testing.T) {
		w := serveShareWebVitals(t, h.handleGetShareAIActivity(), token, siteID, "/ai-activity")
		requireShareStatus(t, w, http.StatusOK)

		var report api.AIActivityReport
		decodeShareResponse(t, w, &report)
		if report.TrackedHits != 1 || report.FetchCount != 1 || report.AIRequests != 2 {
			t.Fatalf("tracked %d fetched %d ai_requests %d, want 1/1/2", report.TrackedHits, report.FetchCount, report.AIRequests)
		}
		if report.ReferralVisits != 1 {
			t.Fatalf("referral_visits = %d, want 1", report.ReferralVisits)
		}
		if len(report.Series) == 0 {
			t.Fatal("expected a non-empty series")
		}
	})

	t.Run("honors ai filter", func(t *testing.T) {
		w := serveShareWebVitals(t, h.handleGetShareAIActivity(), token, siteID, "/ai-activity?filter=ai_bot:GPTBot")
		requireShareStatus(t, w, http.StatusOK)

		var report api.AIActivityReport
		decodeShareResponse(t, w, &report)
		if report.ReferralVisits != 0 {
			t.Fatalf("referral_visits = %d, want 0 under an ai_bot filter", report.ReferralVisits)
		}
		if report.FetchCount != 1 {
			t.Fatalf("fetch_count = %d, want 1", report.FetchCount)
		}
	})

	t.Run("accepts conversion cohort UUIDs", func(t *testing.T) {
		w := serveShareWebVitals(t, h.handleGetShareAIActivity(), token, siteID, "/ai-activity?goal_id="+uuid.NewString()+"&funnel_id="+uuid.NewString())
		requireShareStatus(t, w, http.StatusOK)
	})

	t.Run("rejects invalid conversion cohort UUIDs", func(t *testing.T) {
		for _, key := range []string{"goal_id", "funnel_id"} {
			w := serveShareWebVitals(t, h.handleGetShareAIActivity(), token, siteID, "/ai-activity?"+key+"=not-a-uuid")
			requireShareStatus(t, w, http.StatusBadRequest)
		}
	})

	t.Run("invalid filter", func(t *testing.T) {
		w := serveShareWebVitals(t, h.handleGetShareAIActivity(), token, siteID, "/ai-activity?filter=ai_bogus:x")
		requireShareStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid from", func(t *testing.T) {
		w := serveShareWebVitals(t, h.handleGetShareAIActivity(), token, siteID, "/ai-activity?from=not-a-date")
		requireShareStatus(t, w, http.StatusBadRequest)
	})

	t.Run("site mismatch", func(t *testing.T) {
		w := serveShareWebVitals(t, h.handleGetShareAIActivity(), token, uuid.New(), "/ai-activity")
		requireShareStatus(t, w, http.StatusNotFound)
	})

	t.Run("unknown token", func(t *testing.T) {
		w := serveShareWebVitals(t, h.handleGetShareAIActivity(), "not-a-token", siteID, "/ai-activity")
		requireShareStatus(t, w, http.StatusNotFound)
	})
}
