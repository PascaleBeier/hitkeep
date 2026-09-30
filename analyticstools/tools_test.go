package analyticstools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/analyticscatalog"
	"hitkeep/api"
	"hitkeep/database"
	json "hitkeep/jsonapi"
)

func TestAnnotationsToolIsOfferedOnlyWithANotesStore(t *testing.T) {
	store, siteID, authorID := setupAnnotationsStore(t)
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	for _, input := range []api.AnnotationInput{
		{StartsAt: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), Body: "Pricing launch"},
		{StartsAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Body: "Outside the window"},
	} {
		if _, err := store.CreateAnnotation(context.Background(), siteID, input, authorID); err != nil {
			t.Fatalf("create annotation: %v", err)
		}
	}

	// Opportunities builds the bridge without a notes store: notes must never become evidence there.
	withoutNotes := NewBridge(Config{Analytics: store, SiteID: siteID, From: from, To: to})
	if findTool(withoutNotes, analyticscatalog.ToolAnnotations) {
		t.Fatalf("annotations tool offered without a notes store")
	}

	askAI := NewBridge(Config{Analytics: store, Annotations: store, SiteID: siteID, From: from, To: to})
	var raw string
	for _, tool := range askAI.Tools() {
		if tool.Name == analyticscatalog.ToolAnnotations {
			out, err := tool.Execute(context.Background(), nil)
			if err != nil {
				t.Fatalf("execute annotations tool: %v", err)
			}
			raw = out
		}
	}
	if raw == "" {
		t.Fatalf("annotations tool missing from Ask AI bridge")
	}

	var body struct {
		EvidenceID string `json:"evidence_id"`
		SiteID     string `json:"site_id"`
		Data       []struct {
			Body string `json:"body"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("decode tool output: %v", err)
	}
	if body.EvidenceID != analyticscatalog.ToolAnnotations || body.SiteID != siteID.String() {
		t.Fatalf("envelope = %+v", body)
	}
	if len(body.Data) != 1 || body.Data[0].Body != "Pricing launch" {
		t.Fatalf("notes = %+v, want only the in-range note", body.Data)
	}
	if strings.Contains(raw, "created_by") || strings.Contains(raw, authorID.String()) {
		t.Fatalf("tool output exposes the author: %s", raw)
	}
}

func setupAnnotationsStore(t *testing.T) (*database.Store, uuid.UUID, uuid.UUID) {
	t.Helper()
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "notes@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "notes.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	return store, site.ID, userID
}

func findTool(bridge Bridge, name string) bool {
	for _, tool := range bridge.Tools() {
		if tool.Name == name {
			return true
		}
	}
	return false
}
