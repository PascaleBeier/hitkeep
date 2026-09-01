package sites

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/config"
	"hitkeep/internal/realtime"
	"hitkeep/internal/server/shared"
)

func TestHandleGetSiteRealtimeStreamsSiteEvents(t *testing.T) {
	siteID := uuid.New()
	broker := realtime.NewBroker()
	h := &handler{ctx: &shared.Context{Config: &config.Config{}, Realtime: broker}}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/sites/{id}/realtime", h.handleGetSiteRealtime())
	server := httptest.NewServer(mux)
	defer server.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/api/sites/"+siteID.String()+"/realtime", nil)
	if err != nil {
		t.Fatalf("build realtime request: %v", err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("failed to connect to realtime stream: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	waitForSubscribers(t, broker, siteID)
	broker.Publish(realtime.Event{SiteID: siteID, Kinds: []string{realtime.KindHits}, Counts: map[string]int{realtime.KindHits: 1}})
	body := waitForBody(t, reader, `"kinds":["hits"]`)

	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expected event-stream content type, got %q", contentType)
	}
	if !strings.Contains(body, "event: analytics.changed") {
		t.Fatalf("expected analytics.changed event in SSE body, got %q", body)
	}
}

func TestHandleGetSiteRealtimeRejectsInvalidSiteID(t *testing.T) {
	h := &handler{ctx: &shared.Context{Config: &config.Config{}, Realtime: realtime.NewBroker()}}
	req := httptest.NewRequest(http.MethodGet, "/api/sites/bad/realtime", nil)
	req.SetPathValue("id", "bad")
	rec := httptest.NewRecorder()

	h.handleGetSiteRealtime().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func waitForSubscribers(t *testing.T, broker *realtime.Broker, siteID uuid.UUID) {
	t.Helper()
	deadline := time.After(time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for realtime subscriber")
		case <-ticker.C:
			if broker.SubscriberCount(siteID) > 0 {
				return
			}
		}
	}
}

func waitForBody(t *testing.T, reader *bufio.Reader, fragment string) string {
	t.Helper()

	type result struct {
		body string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		var body strings.Builder
		for {
			line, err := reader.ReadString('\n')
			body.WriteString(line)
			if strings.Contains(body.String(), fragment) {
				done <- result{body: body.String()}
				return
			}
			if err != nil {
				done <- result{body: body.String(), err: err}
				return
			}
		}
	}()

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("failed reading response while waiting for %q in %q: %v", fragment, result.body, result.err)
		}
		return result.body
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for response fragment %q", fragment)
		return ""
	}
}
