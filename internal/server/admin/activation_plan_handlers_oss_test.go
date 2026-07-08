//go:build !billing

package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleSetActivationTeamPlanNotImplementedOnOSSBuild(t *testing.T) {
	h := &handler{}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/system/activation/00000000-0000-0000-0000-000000000000/plan", nil)
	req.SetPathValue("team_id", "00000000-0000-0000-0000-000000000000")
	w := httptest.NewRecorder()
	h.handleSetActivationTeamPlan().ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", w.Code, w.Body.String())
	}
}
