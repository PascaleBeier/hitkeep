package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"
	"golang.org/x/time/rate"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/blocking"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

func TestIngestCORSPreflightAddsCachingHeaders(t *testing.T) {
	handler := newIngestCORS().Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/ingest", nil)
	req.Header.Set("Origin", "https://app.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("expected echoed origin, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials header, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "86400" {
		t.Fatalf("expected max age header, got %q", got)
	}
	if got := rec.Header().Values("Vary"); len(got) == 0 {
		t.Fatal("expected vary header to be set")
	}
}

func TestIngestCORSActualRequestEchoesOrigin(t *testing.T) {
	handler := newIngestCORS().Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPost, "/ingest", nil)
	req.Header.Set("Origin", "https://app.example")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("expected echoed origin, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials header, got %q", got)
	}
}

func TestHandleIngestLeaderDropsBlockedReferrerBeforePublish(t *testing.T) {
	h, cleanup := setupIngestHandler(t, func(ctx *shared.Context) {
		filter := mustNewTestSpamFilter(t, blocking.SpamFeedData{
			ReferrerHostDenylist: []string{"buttons-for-website.example"},
		})
		ctx.SpamFilter = filter
	})
	defer cleanup()

	req := newIngestRequest(t, "https://example.com", "198.51.100.22:1234", map[string]any{
		"path":       "/docs",
		"referrer":   "https://www.buttons-for-website.example/landing",
		"session_id": uuid.New(),
		"page_id":    uuid.New(),
	})

	rec := httptest.NewRecorder()
	h.handleIngestLeader(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if got := h.ctx.SystemCounters.Spam.Load(); got != 1 {
		t.Fatalf("expected spam counter 1, got %d", got)
	}
}

func TestHandleIngestLeaderDropsBlockedNetworkBeforePublish(t *testing.T) {
	h, cleanup := setupIngestHandler(t, func(ctx *shared.Context) {
		filter := mustNewTestSpamFilter(t, blocking.SpamFeedData{
			NetworkDenylist: []string{"203.0.113.0/24"},
		})
		ctx.SpamFilter = filter
	})
	defer cleanup()

	req := newIngestRequest(t, "https://example.com", "203.0.113.42:1234", map[string]any{
		"path":       "/pricing",
		"session_id": uuid.New(),
		"page_id":    uuid.New(),
	})

	rec := httptest.NewRecorder()
	h.handleIngestLeader(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if got := h.ctx.SystemCounters.Spam.Load(); got != 1 {
		t.Fatalf("expected spam counter 1, got %d", got)
	}
}

func TestHandleIngestLeaderCountsUnknownSiteRejection(t *testing.T) {
	h, cleanup := setupIngestHandler(t, nil)
	defer cleanup()

	req := newIngestRequest(t, "https://unknown.example", "198.51.100.22:1234", map[string]any{
		"path":       "/docs",
		"session_id": uuid.New(),
		"page_id":    uuid.New(),
	})

	rec := httptest.NewRecorder()
	h.handleIngestLeader(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if got := h.ctx.SystemCounters.Rejections.Load(); got != 1 {
		t.Fatalf("expected rejection counter 1, got %d", got)
	}
}

func TestHandleIngestLeaderCountsBadBodyRejection(t *testing.T) {
	h, cleanup := setupIngestHandler(t, nil)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewBufferString("{"))
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.22:1234"

	rec := httptest.NewRecorder()
	h.handleIngestLeader(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
	if got := h.ctx.SystemCounters.Rejections.Load(); got != 1 {
		t.Fatalf("expected rejection counter 1, got %d", got)
	}
}

func TestHandleIngestLeaderAllowsSameSiteReferrerToContinueToPublish(t *testing.T) {
	producer, err := newTestProducer()
	if err != nil {
		t.Fatalf("create nsq producer: %v", err)
	}
	defer producer.Stop()

	h, cleanup := setupIngestHandler(t, func(ctx *shared.Context) {
		filter := mustNewTestSpamFilter(t, blocking.SpamFeedData{
			ReferrerHostDenylist: []string{"example.com"},
		})
		ctx.SpamFilter = filter
		ctx.Producer = producer
	})
	defer cleanup()

	req := newIngestRequest(t, "https://www.example.com", "198.51.100.33:1234", map[string]any{
		"path":       "/docs/start",
		"referrer":   "https://www.example.com/blog/post",
		"session_id": uuid.New(),
		"page_id":    uuid.New(),
	})

	rec := httptest.NewRecorder()
	h.handleIngestLeader(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected publish attempt to fail with status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestHandleIngestEventLeaderDropsBlockedReferrerBeforePublish(t *testing.T) {
	h, cleanup := setupIngestHandler(t, func(ctx *shared.Context) {
		filter := mustNewTestSpamFilter(t, blocking.SpamFeedData{
			ReferrerHostDenylist: []string{"buttons-for-website.example"},
		})
		ctx.SpamFilter = filter
	})
	defer cleanup()

	req := newIngestEventRequest(t, "https://example.com", "198.51.100.22:1234", map[string]any{
		"n":   "signup",
		"p":   map[string]any{"plan": "pro"},
		"r":   "https://www.buttons-for-website.example/landing",
		"sid": uuid.New(),
	})

	rec := httptest.NewRecorder()
	h.handleIngestEventLeader(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if got := h.ctx.SystemCounters.Spam.Load(); got != 1 {
		t.Fatalf("expected spam counter 1, got %d", got)
	}
	if got := h.ctx.SystemCounters.Rejections.Load(); got != 0 {
		t.Fatalf("expected rejection counter 0, got %d", got)
	}
}

func TestHandleIngestEventLeaderCountsUnknownSiteRejection(t *testing.T) {
	h, cleanup := setupIngestHandler(t, nil)
	defer cleanup()

	req := newIngestEventRequest(t, "https://unknown.example", "198.51.100.22:1234", map[string]any{
		"n":   "signup",
		"sid": uuid.New(),
	})

	rec := httptest.NewRecorder()
	h.handleIngestEventLeader(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if got := h.ctx.SystemCounters.Rejections.Load(); got != 1 {
		t.Fatalf("expected rejection counter 1, got %d", got)
	}
}

func TestHandleIngestEventLeaderAllowsSameSiteReferrerToContinueToPublish(t *testing.T) {
	producer, err := newTestProducer()
	if err != nil {
		t.Fatalf("create nsq producer: %v", err)
	}
	defer producer.Stop()

	h, cleanup := setupIngestHandler(t, func(ctx *shared.Context) {
		filter := mustNewTestSpamFilter(t, blocking.SpamFeedData{
			ReferrerHostDenylist: []string{"example.com"},
		})
		ctx.SpamFilter = filter
		ctx.Producer = producer
	})
	defer cleanup()

	req := newIngestEventRequest(t, "https://www.example.com", "198.51.100.33:1234", map[string]any{
		"n":   "signup",
		"p":   map[string]any{"plan": "pro"},
		"r":   "https://www.example.com/blog/post",
		"sid": uuid.New(),
	})

	rec := httptest.NewRecorder()
	h.handleIngestEventLeader(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected publish attempt to fail with status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestHandleServerPageviewIngestPublishesCanonicalTimestamp(t *testing.T) {
	producer := &capturingProducer{}
	_, ctx, _, siteID, token := setupServerIngestTestEnv(t, auth.SiteOwner, func(ctx *shared.Context) {
		ctx.Producer = producer
	})

	mux := http.NewServeMux()
	Register(mux, ctx)

	canonical := time.Date(2026, 4, 3, 12, 30, 45, 0, time.UTC)
	sessionID := uuid.New()
	pageID := uuid.New()
	body := map[string]any{
		"url":        "https://www.example.com/docs/start?utm_source=newsletter&utm_medium=email&utm_campaign=spring&utm_term=analytics&utm_content=hero#intro",
		"timestamp":  canonical.Format(time.RFC3339),
		"visitor_ip": "198.51.100.22",
		"user_agent": "Mozilla/5.0 server-side replay",
		"session_id": sessionID,
		"page_id":    pageID,
		"utm_source": "ignored-request-field",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/server/pageview", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", token)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty response body, got %q", rec.Body.String())
	}

	msg := producer.onlyMessage(t, "hits")
	var got api.Hit
	if err := json.Unmarshal(msg, &got); err != nil {
		t.Fatalf("unmarshal published hit: %v", err)
	}
	if got.SiteID != siteID {
		t.Fatalf("expected site_id %s, got %s", siteID, got.SiteID)
	}
	if !got.Timestamp.Equal(canonical) {
		t.Fatalf("expected timestamp %s, got %s", canonical, got.Timestamp)
	}
	if got.SessionID != sessionID {
		t.Fatalf("expected session_id %s, got %s", sessionID, got.SessionID)
	}
	if got.PageID != pageID {
		t.Fatalf("expected page_id %s, got %s", pageID, got.PageID)
	}
	if got.Path != "/docs/start?utm_source=newsletter&utm_medium=email&utm_campaign=spring&utm_term=analytics&utm_content=hero" {
		t.Fatalf("expected normalized path with query, got %q", got.Path)
	}
	if got.Hostname == nil || *got.Hostname != "example.com" {
		t.Fatalf("expected normalized hostname example.com, got %v", got.Hostname)
	}
	if got.UserAgent == nil || *got.UserAgent != "Mozilla/5.0 server-side replay" {
		t.Fatalf("expected stored user agent, got %v", got.UserAgent)
	}
	if got.UTMSource == nil || *got.UTMSource != "newsletter" {
		t.Fatalf("expected utm_source from url, got %v", got.UTMSource)
	}
	if got.UTMMedium == nil || *got.UTMMedium != "email" {
		t.Fatalf("expected utm_medium from url, got %v", got.UTMMedium)
	}
	if got.UTMCampaign == nil || *got.UTMCampaign != "spring" {
		t.Fatalf("expected utm_campaign from url, got %v", got.UTMCampaign)
	}
	if got.UTMTerm == nil || *got.UTMTerm != "analytics" {
		t.Fatalf("expected utm_term from url, got %v", got.UTMTerm)
	}
	if got.UTMContent == nil || *got.UTMContent != "hero" {
		t.Fatalf("expected utm_content from url, got %v", got.UTMContent)
	}
}

func TestHandleServerEventIngestPublishesCanonicalTimestamp(t *testing.T) {
	producer := &capturingProducer{}
	_, ctx, _, siteID, token := setupServerIngestTestEnv(t, auth.SiteOwner, func(ctx *shared.Context) {
		ctx.Producer = producer
	})

	mux := http.NewServeMux()
	Register(mux, ctx)

	canonical := time.Date(2026, 4, 4, 8, 15, 0, 0, time.UTC)
	sessionID := uuid.New()
	body := map[string]any{
		"url":        "https://www.example.com/pricing",
		"timestamp":  canonical.Format(time.RFC3339),
		"visitor_ip": "198.51.100.23",
		"user_agent": "Mozilla/5.0 server-side event",
		"name":       "signup_started",
		"properties": map[string]any{"plan": "pro"},
		"session_id": sessionID,
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/server/event", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", token)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty response body, got %q", rec.Body.String())
	}

	msg := producer.onlyMessage(t, "events")
	var got api.Event
	if err := json.Unmarshal(msg, &got); err != nil {
		t.Fatalf("unmarshal published event: %v", err)
	}
	if got.SiteID != siteID {
		t.Fatalf("expected site_id %s, got %s", siteID, got.SiteID)
	}
	if got.SessionID != sessionID {
		t.Fatalf("expected session_id %s, got %s", sessionID, got.SessionID)
	}
	if !got.Timestamp.Equal(canonical) {
		t.Fatalf("expected timestamp %s, got %s", canonical, got.Timestamp)
	}
	if got.Name != "signup_started" {
		t.Fatalf("expected event name signup_started, got %q", got.Name)
	}
	if got.Properties["plan"] != "pro" {
		t.Fatalf("expected properties to include plan, got %+v", got.Properties)
	}
}

func TestHandleServerIngestRequiresAPIClientManageDataForResolvedURL(t *testing.T) {
	_, ctx, _, _, token := setupServerIngestTestEnv(t, auth.SiteViewer, func(ctx *shared.Context) {
		ctx.Producer = &capturingProducer{}
	})

	mux := http.NewServeMux()
	Register(mux, ctx)

	body := map[string]any{
		"url":        "https://example.com/docs",
		"timestamp":  "2026-04-03T12:30:45Z",
		"visitor_ip": "198.51.100.22",
		"user_agent": "Mozilla/5.0",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/server/pageview", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", token)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d body=%s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
}

func TestHandleServerPageviewIngestDNTDropsWithoutStoring(t *testing.T) {
	producer := &capturingProducer{}
	_, ctx, _, _, token := setupServerIngestTestEnv(t, auth.SiteOwner, func(ctx *shared.Context) {
		ctx.Producer = producer
	})

	mux := http.NewServeMux()
	Register(mux, ctx)

	canonical := time.Date(2026, 4, 3, 12, 30, 45, 0, time.UTC)
	body := map[string]any{
		"url":        "https://example.com/private",
		"timestamp":  canonical.Format(time.RFC3339),
		"visitor_ip": "198.51.100.22",
		"user_agent": "Mozilla/5.0",
		"dnt":        true,
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/server/pageview", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", token)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if got := producer.messageCount("hits"); got != 0 {
		t.Fatalf("expected DNT request to publish no hits, got %d", got)
	}
}

func TestHandleServerPageviewIngestGeneratesIDsAndDoesNotExposeIsUnique(t *testing.T) {
	producer := &capturingProducer{}
	_, ctx, _, _, token := setupServerIngestTestEnv(t, auth.SiteOwner, func(ctx *shared.Context) {
		ctx.Producer = producer
	})

	mux := http.NewServeMux()
	Register(mux, ctx)

	for _, path := range []string{"/first", "/second"} {
		body := map[string]any{
			"url":        "https://example.com" + path,
			"timestamp":  "2026-04-03T12:30:45Z",
			"visitor_ip": "198.51.100.22",
			"user_agent": "Mozilla/5.0",
			"is_unique":  true,
		}
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/ingest/server/pageview", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", token)

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
		}
	}

	messages := producer.messagesForTopic("hits")
	if len(messages) != 2 {
		t.Fatalf("expected 2 hit messages, got %d", len(messages))
	}
	var first api.Hit
	var second api.Hit
	if err := json.Unmarshal(messages[0], &first); err != nil {
		t.Fatalf("unmarshal first hit: %v", err)
	}
	if err := json.Unmarshal(messages[1], &second); err != nil {
		t.Fatalf("unmarshal second hit: %v", err)
	}
	if first.SessionID == uuid.Nil || second.SessionID == uuid.Nil {
		t.Fatalf("expected generated session IDs, got %s and %s", first.SessionID, second.SessionID)
	}
	if first.SessionID == second.SessionID {
		t.Fatalf("expected omitted session_id to generate standalone sessions, both got %s", first.SessionID)
	}
	if first.PageID == uuid.Nil || second.PageID == uuid.Nil {
		t.Fatalf("expected generated page IDs, got %s and %s", first.PageID, second.PageID)
	}
	if first.IsUnique != nil || second.IsUnique != nil {
		t.Fatalf("expected server ingest not to expose is_unique, got %v and %v", first.IsUnique, second.IsUnique)
	}
}

func TestHandleServerPageviewIngestUsesVisitorIPForExclusionsBeforePublish(t *testing.T) {
	producer := &capturingProducer{}
	store, ctx, userID, siteID, token := setupServerIngestTestEnv(t, auth.SiteOwner, func(ctx *shared.Context) {
		ctx.Producer = producer
	})
	if _, err := store.CreateSiteExclusion(context.Background(), siteID, "198.51.100.0/24", "replay block", userID); err != nil {
		t.Fatalf("CreateSiteExclusion: %v", err)
	}
	filter := blocking.NewIPFilter(store)
	if err := filter.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh IP filter: %v", err)
	}
	ctx.IPFilter = filter

	mux := http.NewServeMux()
	Register(mux, ctx)

	body := map[string]any{
		"url":        "https://example.com/blocked",
		"timestamp":  "2026-04-03T12:30:45Z",
		"visitor_ip": "198.51.100.22",
		"user_agent": "Mozilla/5.0",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/server/pageview", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", token)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if got := producer.messageCount("hits"); got != 0 {
		t.Fatalf("expected blocked visitor_ip to publish no hits, got %d", got)
	}
	if got := ctx.SystemCounters.Rejections.Load(); got != 1 {
		t.Fatalf("expected rejection counter 1, got %d", got)
	}
}

func TestHandleServerIngestFollowerForwardsToLeader(t *testing.T) {
	for _, tc := range []struct {
		name    string
		path    string
		payload map[string]any
	}{
		{
			name: "pageview",
			path: "/api/ingest/server/pageview",
			payload: map[string]any{
				"url":        "https://example.com/docs",
				"timestamp":  "2026-04-03T12:30:45Z",
				"visitor_ip": "198.51.100.22",
				"user_agent": "Mozilla/5.0",
			},
		},
		{
			name: "event",
			path: "/api/ingest/server/event",
			payload: map[string]any{
				"url":        "https://example.com/docs",
				"timestamp":  "2026-04-03T12:30:45Z",
				"visitor_ip": "198.51.100.22",
				"user_agent": "Mozilla/5.0",
				"name":       "signup_started",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			leader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Fatalf("expected forwarded path %s, got %q", tc.path, r.URL.Path)
				}
				if got := r.Header.Get("X-API-Key"); got != "hk_test_token" {
					t.Fatalf("expected forwarded API key, got %q", got)
				}
				w.WriteHeader(http.StatusAccepted)
			}))
			defer leader.Close()

			ctx := &shared.Context{
				Cluster:        testClusterState{leader: false, leaderAddr: leader.Listener.Addr().String()},
				Config:         &config.Config{HTTPAddr: leader.Listener.Addr().String()},
				SystemCounters: &database.SystemCounter{},
			}

			mux := http.NewServeMux()
			Register(mux, ctx)

			payload, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-API-Key", "hk_test_token")

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusAccepted {
				t.Fatalf("expected follower to forward and return %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleServerPageviewIngestUsesIngestLimiter(t *testing.T) {
	producer := &capturingProducer{}
	_, ctx, _, _, token := setupServerIngestTestEnv(t, auth.SiteOwner, func(ctx *shared.Context) {
		ctx.Producer = producer
		ctx.ApiLimiter = shared.NewIPRateLimiter(rate.Limit(0), 0)
		ctx.IngestLimiter = shared.NewIPRateLimiter(rate.Inf, 1)
	})

	mux := http.NewServeMux()
	Register(mux, ctx)

	payload, _ := json.Marshal(map[string]any{
		"url":        "https://example.com/docs",
		"timestamp":  "2026-04-03T12:30:45Z",
		"visitor_ip": "198.51.100.22",
		"user_agent": "Mozilla/5.0",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/server/pageview", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", token)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected server ingest to use ingest limiter and return %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if got := producer.messageCount("hits"); got != 1 {
		t.Fatalf("expected accepted request to publish 1 hit, got %d", got)
	}
}

func setupIngestHandler(t *testing.T, mutateCtx func(*shared.Context)) (*handler, func()) {
	t.Helper()

	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	userID, err := store.CreateUser(context.Background(), "ingest-test@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	if _, err := store.CreateSite(context.Background(), userID, "example.com"); err != nil {
		t.Fatalf("create test site: %v", err)
	}

	ctx := &shared.Context{
		Store:          store,
		Config:         &config.Config{},
		SystemCounters: &database.SystemCounter{},
	}
	if mutateCtx != nil {
		mutateCtx(ctx)
	}

	return &handler{ctx: ctx}, func() {
		store.Close()
	}
}

func setupServerIngestTestEnv(t *testing.T, siteRole auth.SiteRole, mutateCtx func(*shared.Context)) (*database.Store, *shared.Context, uuid.UUID, uuid.UUID, string) {
	t.Helper()

	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	userID, err := store.CreateUser(context.Background(), "server-ingest-test@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "example.com")
	if err != nil {
		t.Fatalf("create test site: %v", err)
	}
	_, token, err := store.CreateAPIClient(context.Background(), userID, "Server Ingest", "test", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		site.ID: siteRole,
	}, nil)
	if err != nil {
		t.Fatalf("create api client: %v", err)
	}

	ctx := &shared.Context{
		Store:          store,
		Config:         &config.Config{},
		SystemCounters: &database.SystemCounter{},
	}
	if mutateCtx != nil {
		mutateCtx(ctx)
	}
	return store, ctx, userID, site.ID, token
}

type publishedMessage struct {
	topic string
	body  []byte
}

type capturingProducer struct {
	mu       sync.Mutex
	messages []publishedMessage
	err      error
}

type testClusterState struct {
	leader     bool
	leaderAddr string
}

func (c testClusterState) IsLeader() bool {
	return c.leader
}

func (c testClusterState) GetLeaderAddr() string {
	return c.leaderAddr
}

func (p *capturingProducer) Publish(topic string, body []byte) error {
	if p.err != nil {
		return p.err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, publishedMessage{
		topic: topic,
		body:  append([]byte(nil), body...),
	})
	return nil
}

func (p *capturingProducer) Ping() error {
	if p.err != nil {
		return p.err
	}
	return nil
}

func (p *capturingProducer) onlyMessage(t *testing.T, topic string) []byte {
	t.Helper()
	matches := p.messagesForTopic(topic)
	if len(matches) != 1 {
		t.Fatalf("expected 1 message for topic %q, got %d all=%+v", topic, len(matches), p.messages)
	}
	return matches[0]
}

func (p *capturingProducer) messagesForTopic(topic string) [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	var matches [][]byte
	for _, msg := range p.messages {
		if msg.topic == topic {
			matches = append(matches, msg.body)
		}
	}
	return matches
}

func (p *capturingProducer) messageCount(topic string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	var count int
	for _, msg := range p.messages {
		if msg.topic == topic {
			count++
		}
	}
	return count
}

func newIngestRequest(t *testing.T, origin, remoteAddr string, payload map[string]any) *http.Request {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	return req
}

func newIngestEventRequest(t *testing.T, origin, remoteAddr string, payload map[string]any) *http.Request {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/ingest/event", bytes.NewReader(body))
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	return req
}

func newTestProducer() (*nsq.Producer, error) {
	cfg := nsq.NewConfig()
	cfg.DialTimeout = 100 * time.Millisecond
	cfg.ReadTimeout = 35 * time.Second
	cfg.WriteTimeout = 35 * time.Second
	return nsq.NewProducer("127.0.0.1:1", cfg)
}

func mustNewTestSpamFilter(t *testing.T, data blocking.SpamFeedData) *blocking.SpamFilter {
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
