package permissions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
	json "hitkeep/jsonapi"
)

func setupTestEnv(t *testing.T) (*shared.Context, *database.Store, uuid.UUID) {
	t.Helper()

	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	userID, err := store.CreateUser(context.Background(), "test@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	ctx := &shared.Context{
		Store:  store,
		Config: &config.Config{},
	}

	return ctx, store, userID
}

func TestHandleGetUserPermissions(t *testing.T) {
	ctx, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(context.Background(), userID, "permissions.example.com")
	if err != nil {
		t.Fatalf("failed to create site: %v", err)
	}

	tests := []struct {
		name           string
		injectAuth     bool
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "Unauthorized",
			injectAuth:     false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Authorized - Returns Permissions",
			injectAuth:     true,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp api.PermissionContext
				if err := json.UnmarshalRead(w.Body, &resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if resp.InstanceRole == "" {
					t.Error("expected instance role, got empty")
				}
				if resp.Permissions == nil {
					t.Fatal("expected site permissions map")
				}
				if resp.Permissions[site.ID.String()] != string(auth.SiteOwner) {
					t.Fatalf("expected site owner role for created site, got %q", resp.Permissions[site.ID.String()])
				}
				if resp.InstancePermissions == nil {
					t.Fatal("expected instance permissions list")
				}
				if resp.InstanceCapabilities == nil {
					t.Fatal("expected instance capabilities list")
				}
				if !contains(resp.SiteCapabilities[site.ID.String()], string(auth.PermSiteManageData)) {
					t.Fatalf("expected site manage-data capability, got %+v", resp.SiteCapabilities[site.ID.String()])
				}
				if resp.ActiveTeamID == nil || *resp.ActiveTeamID == uuid.Nil || resp.ActiveTeamRole != "owner" {
					t.Fatalf("expected active team context, got %+v", resp)
				}
				if !contains(resp.ActiveTeamCapabilities, string(auth.CapTeamManageMembers)) {
					t.Fatalf("expected team manage members capability, got %+v", resp.ActiveTeamCapabilities)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/user/permissions", nil)

			if tc.injectAuth {
				req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
			}

			w := httptest.NewRecorder()
			handler := (&handler{ctx: ctx}).handleGetUserPermissions()
			handler.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, w.Code)
			}

			if tc.checkResponse != nil {
				tc.checkResponse(t, w)
			}
		})
	}
}

func contains(values []string, target string) bool {
	return slices.Contains(values, target)
}
