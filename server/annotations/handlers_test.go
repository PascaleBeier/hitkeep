package annotations

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	authcore "hitkeep/auth"
	"hitkeep/config"
	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

func setupAnnotationsTestEnv(t *testing.T) (*handler, uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "hitkeep.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	userID, err := store.CreateUser(ctx, "owner@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "annotations.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	other, err := store.CreateSite(ctx, userID, "other-annotations.test")
	if err != nil {
		t.Fatalf("create other site: %v", err)
	}
	return &handler{ctx: &shared.Context{Store: store, Config: &config.Config{}}}, site.ID, other.ID
}

func serve(t *testing.T, fn http.HandlerFunc, method, target string, body any, pathValues map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.MarshalWrite(&buf, body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, target, &buf)
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()
	fn(rec, req)
	return rec
}

func TestAnnotationHandlersCRUD(t *testing.T) {
	h, siteID, otherSiteID := setupAnnotationsTestEnv(t)
	site := map[string]string{"id": siteID.String()}
	start := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)

	rec := serve(t, h.handleCreate(), http.MethodPost, "/", api.AnnotationInput{StartsAt: start, Body: " Launch "}, site)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", rec.Code, rec.Body)
	}
	var created api.Annotation
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.Body != "Launch" || created.SiteID != siteID {
		t.Fatalf("created = %+v", created)
	}
	if strings.Contains(rec.Body.String(), "created_by") {
		t.Fatalf("response must not expose created_by: %s", rec.Body)
	}

	rec = serve(t, h.handleList(), http.MethodGet, "/?from=2026-09-01T00:00:00Z&to=2026-09-30T00:00:00Z", nil, site)
	var listed []api.Annotation
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || len(listed) != 1 {
		t.Fatalf("list = %d %s (%v)", rec.Code, rec.Body, err)
	}

	end := start.AddDate(0, 0, 3)
	ids := map[string]string{"id": siteID.String(), "annotationID": created.ID.String()}
	rec = serve(t, h.handleUpdate(), http.MethodPut, "/", api.AnnotationInput{StartsAt: start, EndsAt: &end, Body: "Campaign"}, ids)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", rec.Code, rec.Body)
	}

	// The note is invisible through another site's path.
	foreign := map[string]string{"id": otherSiteID.String(), "annotationID": created.ID.String()}
	if rec := serve(t, h.handleUpdate(), http.MethodPut, "/", api.AnnotationInput{StartsAt: start, Body: "x"}, foreign); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-site update status = %d", rec.Code)
	}
	if rec := serve(t, h.handleDelete(), http.MethodDelete, "/", nil, foreign); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-site delete status = %d", rec.Code)
	}

	if rec := serve(t, h.handleDelete(), http.MethodDelete, "/", nil, ids); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
}

func TestAnnotationHandlersRejectInvalidInput(t *testing.T) {
	h, siteID, _ := setupAnnotationsTestEnv(t)
	site := map[string]string{"id": siteID.String()}
	now := time.Now().UTC()
	before := now.Add(-time.Hour)

	for name, input := range map[string]api.AnnotationInput{
		"missing start":    {Body: "x"},
		"blank body":       {StartsAt: now, Body: "   "},
		"too long":         {StartsAt: now, Body: strings.Repeat("é", maxBodyRunes+1)},
		"end before start": {StartsAt: now, EndsAt: &before, Body: "x"},
		"far future":       {StartsAt: now.AddDate(2, 0, 0), Body: "x"},
	} {
		if rec := serve(t, h.handleCreate(), http.MethodPost, "/", input, site); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d", name, rec.Code)
		}
	}
	// Exactly the limit is fine, counted in characters rather than bytes.
	if rec := serve(t, h.handleCreate(), http.MethodPost, "/", api.AnnotationInput{StartsAt: now, Body: strings.Repeat("é", maxBodyRunes)}, site); rec.Code != http.StatusCreated {
		t.Fatalf("limit body status = %d: %s", rec.Code, rec.Body)
	}
}

func TestOnlyEditorsAndAboveManageAnnotations(t *testing.T) {
	for role, want := range map[authcore.SiteRole]bool{
		authcore.SiteOwner:  true,
		authcore.SiteAdmin:  true,
		authcore.SiteEditor: true,
		authcore.SiteViewer: false,
	} {
		if got := role.HasPermission(authcore.PermSiteManageAnnotations); got != want {
			t.Fatalf("%s manage annotations = %v, want %v", role, got, want)
		}
		if !role.HasPermission(authcore.PermSiteView) {
			t.Fatalf("%s must be able to read annotations", role)
		}
	}
}
