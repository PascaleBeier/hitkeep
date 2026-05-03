package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/server/shared"
)

func TestHandleGetUserBootstrap(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	if _, err := store.CreateSite(context.Background(), userID, "bootstrap.example.com"); err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.UpsertUserPreferences(context.Background(), userID, api.UserPreferences{DefaultLocale: "de"}); err != nil {
		t.Fatalf("upsert preferences: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/user/bootstrap", nil)
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
	req = req.WithContext(context.WithValue(req.Context(), shared.AuthSessionKey, shared.AuthSessionContext{
		ExpiresAt: time.Now().Add(30 * time.Minute).UTC(),
		IssuedAt:  time.Now().UTC(),
	}))
	w := httptest.NewRecorder()

	h.handleGetUserBootstrap().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp api.UserBootstrap
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Session.ExpiresAt.IsZero() {
		t.Fatalf("expected session details")
	}
	if resp.Profile.ID != userID || resp.Profile.Email == "" {
		t.Fatalf("expected profile for user %s, got %+v", userID, resp.Profile)
	}
	if resp.Preferences.DefaultLocale != "de" {
		t.Fatalf("expected locale de, got %q", resp.Preferences.DefaultLocale)
	}
	if resp.Teams.ActiveTeamID == uuid.Nil || len(resp.Teams.Teams) == 0 {
		t.Fatalf("expected team context, got %+v", resp.Teams)
	}
	if resp.Permissions.InstanceRole == "" {
		t.Fatalf("expected permission context")
	}
	if len(resp.Sites) != 1 || resp.Sites[0].Domain != "bootstrap.example.com" {
		t.Fatalf("expected bootstrap site, got %+v", resp.Sites)
	}
	if resp.Status.NeedsSetup {
		t.Fatalf("expected setup to be complete")
	}
}

func TestHandleGetUserBootstrapRequiresSession(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/bootstrap", nil), userID)
	w := httptest.NewRecorder()

	h.handleGetUserBootstrap().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}
