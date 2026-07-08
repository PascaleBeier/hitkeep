package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleGetSystemReport(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)
	h.ctx.Config.JWTSecret = "report-secret-must-not-leak"

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/report", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetSystemReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Fatalf("expected text/markdown content type, got %q", ct)
	}

	report := w.Body.String()
	for _, section := range []string{
		"# HitKeep System Report",
		"## Instance",
		"## Configuration",
		"## Storage",
		"## DuckDB Memory",
		"## Ingest (24h)",
		"## Features",
	} {
		if !strings.Contains(report, section) {
			t.Fatalf("expected report to contain %q\nreport:\n%s", section, report)
		}
	}
	if !strings.Contains(report, "Go version") {
		t.Fatal("expected report to include the Go runtime version")
	}
	if strings.Contains(report, "report-secret-must-not-leak") {
		t.Fatal("report must not leak the JWT secret")
	}
}
