package goals

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/config"
	"hitkeep/internal/controlstore"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/testutil"
	"hitkeep/internal/webhooks"
)

type recordingWebhookEmitter struct {
	events []webhooks.Event
	err    error
}

func (e *recordingWebhookEmitter) Emit(_ context.Context, event webhooks.Event) (webhooks.Emission, error) {
	e.events = append(e.events, event)
	return webhooks.Emission{EventID: uuid.New()}, e.err
}

func setupTenantGoalsTestEnv(t *testing.T) (*handler, *controlstore.Store, *database.Store, uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	store, tenantMgr := testutil.NewControlAndTenantStores(t)

	t.Cleanup(func() {
		_ = tenantMgr.Close()
		_ = store.Close()
	})

	userID, err := store.CreateUser(ctx, "team-owner@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	team, err := store.CreateTenant(ctx, userID, "Acme Analytics", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}

	site, err := store.CreateSite(ctx, userID, "acme-analytics.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := tenantMgr.SyncSite(ctx, site.ID); err != nil {
		t.Fatalf("sync site: %v", err)
	}

	tenantStore, err := tenantMgr.ForTenant(ctx, team.ID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}

	h := &handler{
		ctx: &shared.Context{
			Store:        store,
			TenantStores: tenantMgr,
			Config:       &config.Config{},
		},
	}

	return h, store, tenantStore, site.ID
}

func TestHandleGoalCRUDUsesTenantAnalyticsStore(t *testing.T) {
	h, _, tenantStore, siteID := setupTenantGoalsTestEnv(t)
	ctx := context.Background()
	emitter := &recordingWebhookEmitter{err: errors.New("webhook storage unavailable")}
	h.ctx.Webhooks = emitter

	body, err := json.Marshal(api.Goal{
		Name:  "Signup",
		Type:  "event",
		Value: "signup_completed",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/goals", bytes.NewReader(body))
	createReq.SetPathValue("id", siteID.String())
	createResp := httptest.NewRecorder()
	h.handleCreateGoal().ServeHTTP(createResp, createReq)

	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d: %s", http.StatusCreated, createResp.Code, createResp.Body.String())
	}
	if len(emitter.events) != 1 || emitter.events[0].Type != webhooks.EventGoalCreated || emitter.events[0].SiteID == nil || *emitter.events[0].SiteID != siteID {
		t.Fatalf("expected non-blocking goal.created event, got %+v", emitter.events)
	}

	tenantGoals, err := tenantStore.GetGoals(ctx, siteID)
	if err != nil {
		t.Fatalf("tenant GetGoals: %v", err)
	}
	if len(tenantGoals) != 1 {
		t.Fatalf("expected 1 goal in tenant store, got %d", len(tenantGoals))
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/goals", nil)
	getReq.SetPathValue("id", siteID.String())
	getResp := httptest.NewRecorder()
	h.handleGetGoals().ServeHTTP(getResp, getReq)

	if getResp.Code != http.StatusOK {
		t.Fatalf("expected get status %d, got %d: %s", http.StatusOK, getResp.Code, getResp.Body.String())
	}

	var gotGoals []api.Goal
	if err := json.NewDecoder(getResp.Body).Decode(&gotGoals); err != nil {
		t.Fatalf("decode goals response: %v", err)
	}
	if len(gotGoals) != 1 || gotGoals[0].Name != "Signup" {
		t.Fatalf("expected tenant goal in response, got %+v", gotGoals)
	}

	updateBody, _ := json.Marshal(api.Goal{Name: "Activated", Type: "event", Value: "account_activated"})
	updateReq := httptest.NewRequest(http.MethodPut, "/api/sites/"+siteID.String()+"/goals/"+gotGoals[0].ID.String(), bytes.NewReader(updateBody))
	updateReq.SetPathValue("id", siteID.String())
	updateReq.SetPathValue("goalID", gotGoals[0].ID.String())
	updateResp := httptest.NewRecorder()
	h.handleUpdateGoal().ServeHTTP(updateResp, updateReq)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected update status %d, got %d: %s", http.StatusOK, updateResp.Code, updateResp.Body.String())
	}
	if len(emitter.events) != 2 || emitter.events[1].Type != webhooks.EventGoalUpdated {
		t.Fatalf("expected non-blocking goal.updated event, got %+v", emitter.events)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/sites/"+siteID.String()+"/goals/"+tenantGoals[0].ID.String(), nil)
	deleteReq.SetPathValue("id", siteID.String())
	deleteReq.SetPathValue("goalID", tenantGoals[0].ID.String())
	deleteResp := httptest.NewRecorder()
	h.handleDeleteGoal().ServeHTTP(deleteResp, deleteReq)

	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected delete status %d, got %d: %s", http.StatusOK, deleteResp.Code, deleteResp.Body.String())
	}
	if len(emitter.events) != 3 || emitter.events[2].Type != webhooks.EventGoalDeleted {
		t.Fatalf("expected non-blocking goal.deleted event, got %+v", emitter.events)
	}

	tenantGoals, err = tenantStore.GetGoals(ctx, siteID)
	if err != nil {
		t.Fatalf("tenant GetGoals after delete: %v", err)
	}
	if len(tenantGoals) != 0 {
		t.Fatalf("expected tenant goal to be deleted, got %d remaining", len(tenantGoals))
	}

}

func TestHandleGetFunnelsUsesTenantAnalyticsStore(t *testing.T) {
	h, _, tenantStore, siteID := setupTenantGoalsTestEnv(t)
	ctx := context.Background()

	err := tenantStore.CreateFunnel(ctx, &api.Funnel{
		SiteID: siteID,
		Name:   "Checkout Funnel",
		Steps: []api.FunnelStep{
			{Type: "path", Value: "/pricing"},
			{Type: "event", Value: "signup_completed"},
		},
	})
	if err != nil {
		t.Fatalf("create funnel in tenant store: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/funnels", nil)
	getReq.SetPathValue("id", siteID.String())
	getResp := httptest.NewRecorder()
	h.handleGetFunnels().ServeHTTP(getResp, getReq)

	if getResp.Code != http.StatusOK {
		t.Fatalf("expected get status %d, got %d: %s", http.StatusOK, getResp.Code, getResp.Body.String())
	}

	var gotFunnels []api.Funnel
	if err := json.NewDecoder(getResp.Body).Decode(&gotFunnels); err != nil {
		t.Fatalf("decode funnels response: %v", err)
	}
	if len(gotFunnels) != 1 || gotFunnels[0].Name != "Checkout Funnel" {
		t.Fatalf("expected tenant funnel in response, got %+v", gotFunnels)
	}
}
