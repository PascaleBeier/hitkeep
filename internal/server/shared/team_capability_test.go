package shared

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/auth"
	"hitkeep/internal/controlstore"
	"hitkeep/internal/database"
	"hitkeep/internal/testutil"
)

func TestHandlerConfigAuthHelpers(t *testing.T) {
	tests := []struct {
		name         string
		config       HandlerConfig
		requiresAuth bool
		allowsAPIKey bool
	}{
		{"explicit auth", HandlerConfig{RequireAuth: true}, true, false},
		{"instance permission", HandlerConfig{InstancePerm: auth.PermInstanceViewSystem}, true, true},
		{"site permission", HandlerConfig{SitePerm: auth.PermSiteView}, true, true},
		{"human-only permission", HandlerConfig{SitePerm: auth.PermSiteManageWebhooks, HumanOnly: true}, true, false},
		{"team capability", HandlerConfig{TeamCap: auth.CapTeamManageSettings}, true, false},
		{"api client only", HandlerConfig{APIClientOnly: true}, false, false},
		{"open route", HandlerConfig{}, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.config.requiresUserAuth(); got != tc.requiresAuth {
				t.Fatalf("requiresUserAuth = %v, want %v", got, tc.requiresAuth)
			}
			if got := tc.config.allowsAPIKey(); got != tc.allowsAPIKey {
				t.Fatalf("allowsAPIKey = %v, want %v", got, tc.allowsAPIKey)
			}
		})
	}
}

func TestRequireTeamCapability(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	defer store.Close()

	ownerID, err := store.CreateUser(ctx, "team-cap-owner@example.test", "hash")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	memberID, err := store.CreateUser(ctx, "team-cap-member@example.test", "hash")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	teamID, err := store.GetActiveTenantID(ctx, ownerID)
	if err != nil {
		t.Fatalf("active tenant: %v", err)
	}
	if err := store.AddTeamMember(ctx, teamID, memberID, database.TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add member: %v", err)
	}

	app := &Context{Store: store}
	nextCalled := false
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}

	t.Run("allows matching team capability", func(t *testing.T) {
		nextCalled = false
		w := httptest.NewRecorder()
		req := teamRequest(ownerID, teamID.String())

		app.RequireTeamCapability(auth.CapTeamManageSettings)(next).ServeHTTP(w, req)

		if w.Code != http.StatusNoContent || !nextCalled {
			t.Fatalf("expected next handler, got status %d called=%v body=%q", w.Code, nextCalled, w.Body.String())
		}
	})

	t.Run("rejects missing auth context", func(t *testing.T) {
		nextCalled = false
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/user/teams/"+teamID.String(), nil)
		req.SetPathValue("id", teamID.String())

		app.RequireTeamCapability(auth.CapTeamViewMembers)(next).ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized || nextCalled {
			t.Fatalf("expected unauthorized without next handler, got status %d called=%v", w.Code, nextCalled)
		}
	})

	t.Run("rejects invalid team id", func(t *testing.T) {
		nextCalled = false
		w := httptest.NewRecorder()
		req := teamRequest(ownerID, "not-a-uuid")

		app.RequireTeamCapability(auth.CapTeamViewMembers)(next).ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest || nextCalled {
			t.Fatalf("expected bad request without next handler, got status %d called=%v", w.Code, nextCalled)
		}
	})

	t.Run("rejects missing capability", func(t *testing.T) {
		nextCalled = false
		w := httptest.NewRecorder()
		req := teamRequest(memberID, teamID.String())

		app.RequireTeamCapability(auth.CapTeamArchive)(next).ServeHTTP(w, req)

		if w.Code != http.StatusForbidden || nextCalled {
			t.Fatalf("expected forbidden without next handler, got status %d called=%v", w.Code, nextCalled)
		}
	})
}

func TestRequirePermissionRequiresAPIClientSiteGrantBeforeInstanceBypass(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	defer store.Close()

	userID, err := store.CreateUser(ctx, "api-client-site-grant@example.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "api-client-site-grant.example.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &Context{Store: store}
	next := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}

	t.Run("human owner can still use instance site permission", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := sitePermissionRequest(userID, site.ID)

		app.RequirePermission(auth.PermSiteView)(next).ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected human owner to pass, got status %d body=%q", w.Code, w.Body.String())
		}
	})

	t.Run("api client owner role without site grant is denied", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := sitePermissionRequest(userID, site.ID)
		req = req.WithContext(context.WithValue(req.Context(), APIClientAuthKey, &database.APIClientAuth{
			ClientID:     uuid.New(),
			UserID:       userID,
			InstanceRole: auth.InstanceOwner,
			SiteRoles:    map[uuid.UUID]auth.SiteRole{},
		}))

		app.RequirePermission(auth.PermSiteView)(next).ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected api client without site grant to be denied, got status %d body=%q", w.Code, w.Body.String())
		}
	})

	t.Run("api client with site grant is allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := sitePermissionRequest(userID, site.ID)
		req = req.WithContext(context.WithValue(req.Context(), APIClientAuthKey, &database.APIClientAuth{
			ClientID:     uuid.New(),
			UserID:       userID,
			InstanceRole: auth.InstanceOwner,
			SiteRoles: map[uuid.UUID]auth.SiteRole{
				site.ID: auth.SiteViewer,
			},
		}))

		app.RequirePermission(auth.PermSiteView)(next).ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected api client with site grant to pass, got status %d body=%q", w.Code, w.Body.String())
		}
	})
}

func teamRequest(userID uuid.UUID, teamID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/user/teams/"+teamID, nil)
	req.SetPathValue("id", teamID)
	return req.WithContext(context.WithValue(req.Context(), UserIDKey, userID))
}

func sitePermissionRequest(userID, siteID uuid.UUID) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/stats", nil)
	req.SetPathValue("id", siteID.String())
	return req.WithContext(context.WithValue(req.Context(), UserIDKey, userID))
}

func newSharedTestStore(t *testing.T) *controlstore.Store {
	t.Helper()
	return testutil.NewControlStore(t)
}
