package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hitkeep/internal/server/shared"
	"hitkeep/internal/testutil"
)

func TestHealthzIsLivenessOnly(t *testing.T) {
	store := testutil.NewControlStore(t)
	defer store.Close()
	h := &handler{
		ctx: &shared.Context{
			Store: store,
		},
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	h.handleHealthz().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Fatalf("expected body ok, got %q", w.Body.String())
	}
}

func TestReadyzReturnsStructuredRetryResponseWhenDatabaseUnavailable(t *testing.T) {
	h := &handler{ctx: &shared.Context{}}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	h.handleReadyz().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") != "5" {
		t.Fatalf("expected Retry-After 5, got %q", w.Header().Get("Retry-After"))
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode readiness response: %v", err)
	}
	if body["reason"] != "database_unavailable" {
		t.Fatalf("unexpected readiness reason: %v", body["reason"])
	}
}
