//go:build billing

package system

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"hitkeep/internal/config"
	"hitkeep/internal/database"
	json "hitkeep/internal/jsonapi"
	"hitkeep/internal/server/shared"
)

func TestHandleGetStatusIncludesCloudMetadata(t *testing.T) {
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	h := &handler{
		ctx: &shared.Context{
			Store: store,
			Config: &config.Config{
				Version:            "v2.0.0",
				CloudHosted:        true,
				CloudSignupEnabled: true,
				CloudJurisdiction:  "EU",
				CloudRegion:        "eu-central-1",
				CloudUpgradeURL:    "https://hitkeep.com/cloud/upgrade",
				CloudSupportURL:    "https://hitkeep.com/cloud/support",
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	h.handleGetStatus().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp struct {
		NeedsSetup bool            `json:"needs_setup"`
		Version    string          `json:"version"`
		Cloud      *map[string]any `json:"cloud"`
	}
	if err := json.UnmarshalRead(w.Body, &resp); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if resp.Cloud == nil {
		t.Fatalf("expected cloud metadata in status response")
	}
	if resp.NeedsSetup {
		t.Fatalf("expected managed cloud status to suppress setup bootstrap")
	}
	if got := (*resp.Cloud)["jurisdiction"]; got != "EU" {
		t.Fatalf("expected jurisdiction EU, got %#v", got)
	}
}
