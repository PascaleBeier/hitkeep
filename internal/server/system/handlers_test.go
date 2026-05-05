package system

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

func TestHealthzIsLivenessOnly(t *testing.T) {
	h := &handler{
		ctx: &shared.Context{
			Store: database.NewStore(":memory:"),
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
