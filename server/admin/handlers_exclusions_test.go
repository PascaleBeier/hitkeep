package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	authcore "hitkeep/auth"
)

func TestGlobalExclusionRoutesRequireInstanceExclusionPermission(t *testing.T) {
	h, _, _, _, adminUserID, regularUserID := setupSystemTestEnv(t)
	mux := http.NewServeMux()
	Register(mux, h.ctx)

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/admin/exclusions"},
		{http.MethodPost, "/api/admin/exclusions"},
		{http.MethodDelete, "/api/admin/exclusions/" + uuid.NewString()},
	}
	serve := func(t *testing.T, method, path string, userID uuid.UUID) int {
		t.Helper()
		token, _, err := authcore.GenerateTokenWithDuration(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, userID, time.Hour)
		if err != nil {
			t.Fatalf("GenerateTokenWithDuration: %v", err)
		}
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(&http.Cookie{Name: authcore.CookieName, Value: token})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			if code := serve(t, route.method, route.path, regularUserID); code != http.StatusForbidden {
				t.Fatalf("instance user: status = %d, want %d", code, http.StatusForbidden)
			}
			if code := serve(t, route.method, route.path, adminUserID); code == http.StatusUnauthorized || code == http.StatusForbidden {
				t.Fatalf("instance admin: status = %d, want access", code)
			}
		})
	}
}
