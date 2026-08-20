package share

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/config"
	"hitkeep/internal/realtime"
)

func TestHandleGetShareRealtimeScopesToShareTokenSite(t *testing.T) {
	h, store, token, siteID := setupShareExportTestEnv(t)
	defer store.Close()
	broker := realtime.NewBroker()
	h.ctx.Realtime = broker
	h.ctx.Config = &config.Config{}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/share/{token}/sites/{id}/realtime", h.handleGetShareRealtime())
	server := httptest.NewServer(mux)
	defer server.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/api/share/"+token+"/sites/"+siteID.String()+"/realtime", nil)
	if err != nil {
		t.Fatalf("build share realtime request: %v", err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("failed to connect to share realtime stream: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	waitForShareSubscribers(t, broker, siteID)
	broker.Publish(realtime.Event{SiteID: siteID, Kinds: []string{realtime.KindEvents}, Counts: map[string]int{realtime.KindEvents: 1}})
	body := waitForShareBody(t, reader, `"kinds":["events"]`)

	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expected event-stream content type, got %q", contentType)
	}
	if !strings.Contains(body, "event: analytics.changed") {
		t.Fatalf("expected analytics.changed event in SSE body, got %q", body)
	}
}

func TestHandleGetShareRealtimeRejectsWrongSite(t *testing.T) {
	h, store, token, _ := setupShareExportTestEnv(t)
	defer store.Close()
	h.ctx.Realtime = realtime.NewBroker()

	req := httptest.NewRequest(http.MethodGet, "/api/share/"+token+"/sites/not-the-site/realtime", nil)
	req.SetPathValue("token", token)
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000001")
	rec := httptest.NewRecorder()

	h.handleGetShareRealtime().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func waitForShareSubscribers(t *testing.T, broker *realtime.Broker, siteID uuid.UUID) {
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

func waitForShareBody(t *testing.T, reader *bufio.Reader, fragment string) string {
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
