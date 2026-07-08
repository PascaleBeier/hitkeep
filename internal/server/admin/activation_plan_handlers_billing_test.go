//go:build billing

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hitkeep/internal/database"
)

func TestHandleSetActivationTeamPlanPreservesStripeFields(t *testing.T) {
	h, store, _, ownerUserID, _, _ := setupSystemTestEnv(t)
	ctx := context.Background()

	account, err := store.CreateManagedCloudAccount(ctx, database.CreateManagedCloudAccountInput{
		Email:          "plan-preserve@example.com",
		HashedPassword: "hashed",
		TeamName:       "Plan Preserve Team",
	})
	if err != nil {
		t.Fatalf("create managed cloud account: %v", err)
	}
	if err := store.UpsertCloudBillingAccount(ctx, database.CloudBillingAccount{
		TenantID:             account.TenantID,
		PlanCode:             database.CloudPlanFree,
		PlanName:             "Free",
		SubscriptionStatus:   database.CloudSubscriptionStatusFree,
		StripeCustomerID:     "cus_existing",
		StripeSubscriptionID: "sub_existing",
		StripePriceID:        "price_existing",
	}); err != nil {
		t.Fatalf("seed billing account: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/system/activation/"+account.TenantID.String()+"/plan", strings.NewReader(`{"plan_code":"business"}`)), ownerUserID)
	req.SetPathValue("team_id", account.TenantID.String())
	w := httptest.NewRecorder()
	h.handleSetActivationTeamPlan().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	updated, err := store.GetCloudBillingAccount(ctx, account.TenantID)
	if err != nil {
		t.Fatalf("get billing account: %v", err)
	}
	if updated.PlanCode != database.CloudPlanBusiness {
		t.Fatalf("expected plan_code business, got %q", updated.PlanCode)
	}
	if updated.StripeCustomerID != "cus_existing" || updated.StripeSubscriptionID != "sub_existing" || updated.StripePriceID != "price_existing" {
		t.Fatalf("expected existing Stripe fields preserved, got %+v", updated)
	}
}

func TestHandleSetActivationTeamPlanCreatesFreshAccount(t *testing.T) {
	h, store, _, ownerUserID, _, _ := setupSystemTestEnv(t)
	ctx := context.Background()

	account, err := store.CreateManagedCloudAccount(ctx, database.CreateManagedCloudAccountInput{
		Email:          "plan-fresh@example.com",
		HashedPassword: "hashed",
		TeamName:       "Plan Fresh Team",
	})
	if err != nil {
		t.Fatalf("create managed cloud account: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/system/activation/"+account.TenantID.String()+"/plan", strings.NewReader(`{"plan_code":"business"}`)), ownerUserID)
	req.SetPathValue("team_id", account.TenantID.String())
	w := httptest.NewRecorder()
	h.handleSetActivationTeamPlan().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["plan_code"] != "business" || resp["plan_name"] != "Business" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	updated, err := store.GetCloudBillingAccount(ctx, account.TenantID)
	if err != nil {
		t.Fatalf("get billing account: %v", err)
	}
	if updated.PlanCode != database.CloudPlanBusiness || updated.SubscriptionStatus != database.CloudSubscriptionStatusActive {
		t.Fatalf("unexpected billing account: %+v", updated)
	}
}

func TestHandleSetActivationTeamPlanRejectsInvalidPlanCode(t *testing.T) {
	h, store, _, ownerUserID, _, _ := setupSystemTestEnv(t)
	ctx := context.Background()

	account, err := store.CreateManagedCloudAccount(ctx, database.CreateManagedCloudAccountInput{
		Email:          "plan-invalid@example.com",
		HashedPassword: "hashed",
		TeamName:       "Plan Invalid Team",
	})
	if err != nil {
		t.Fatalf("create managed cloud account: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/system/activation/"+account.TenantID.String()+"/plan", strings.NewReader(`{"plan_code":"platinum"}`)), ownerUserID)
	req.SetPathValue("team_id", account.TenantID.String())
	w := httptest.NewRecorder()
	h.handleSetActivationTeamPlan().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSetActivationTeamPlanSyncsRetention(t *testing.T) {
	h, store, _, ownerUserID, _, _ := setupSystemTestEnv(t)
	ctx := context.Background()

	account, err := store.CreateManagedCloudAccount(ctx, database.CreateManagedCloudAccountInput{
		Email:          "plan-retention@example.com",
		HashedPassword: "hashed",
		TeamName:       "Plan Retention Team",
	})
	if err != nil {
		t.Fatalf("create managed cloud account: %v", err)
	}
	if _, err := store.CreateSite(ctx, account.UserID, "plan-retention.example.com"); err != nil {
		t.Fatalf("create site: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/system/activation/"+account.TenantID.String()+"/plan", strings.NewReader(`{"plan_code":"business"}`)), ownerUserID)
	req.SetPathValue("team_id", account.TenantID.String())
	w := httptest.NewRecorder()
	h.handleSetActivationTeamPlan().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	sites, err := store.ListSitesForTenant(ctx, account.TenantID)
	if err != nil {
		t.Fatalf("list sites: %v", err)
	}
	if len(sites) != 1 || sites[0].DataRetentionDays != 1095 {
		t.Fatalf("expected retention synced to 1095, got %+v", sites)
	}
}

func TestHandleSetActivationTeamPlanWritesAuditEntry(t *testing.T) {
	h, store, _, ownerUserID, adminUserID, _ := setupSystemTestEnv(t)
	ctx := context.Background()

	account, err := store.CreateManagedCloudAccount(ctx, database.CreateManagedCloudAccountInput{
		Email:          "plan-audit@example.com",
		HashedPassword: "hashed",
		TeamName:       "Plan Audit Team",
	})
	if err != nil {
		t.Fatalf("create managed cloud account: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/system/activation/"+account.TenantID.String()+"/plan", strings.NewReader(`{"plan_code":"business"}`)), adminUserID)
	req.SetPathValue("team_id", account.TenantID.String())
	w := httptest.NewRecorder()
	h.handleSetActivationTeamPlan().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	entries, total, err := store.ListInstanceAuditEntries(ctx, database.InstanceAuditFilter{
		Action: "cloud_billing.plan_override",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("list audit entries: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one audit entry, total=%d len=%d", total, len(entries))
	}
	entry := entries[0]
	if entry.TeamID == nil || *entry.TeamID != account.TenantID {
		t.Fatalf("expected audit entry team_id %s, got %+v", account.TenantID, entry.TeamID)
	}
	if entry.TargetType != "team" || entry.TargetID != account.TenantID.String() {
		t.Fatalf("unexpected audit entry target: %+v", entry)
	}
	if entry.Outcome != "success" {
		t.Fatalf("expected success outcome, got %q", entry.Outcome)
	}
	if entry.ActorID == nil || *entry.ActorID != adminUserID {
		t.Fatalf("expected actor_id %s, got %+v", adminUserID, entry.ActorID)
	}
	_ = ownerUserID
}

func TestHandleSetActivationTeamPlanSucceedsWithoutTenantStores(t *testing.T) {
	h, store, _, ownerUserID, _, _ := setupSystemTestEnv(t)
	ctx := context.Background()
	h.ctx.TenantStores = nil

	account, err := store.CreateManagedCloudAccount(ctx, database.CreateManagedCloudAccountInput{
		Email:          "plan-no-tenant-stores@example.com",
		HashedPassword: "hashed",
		TeamName:       "No Tenant Stores Team",
	})
	if err != nil {
		t.Fatalf("create managed cloud account: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/system/activation/"+account.TenantID.String()+"/plan", strings.NewReader(`{"plan_code":"business"}`)), ownerUserID)
	req.SetPathValue("team_id", account.TenantID.String())
	w := httptest.NewRecorder()
	h.handleSetActivationTeamPlan().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even without TenantStores, got %d: %s", w.Code, w.Body.String())
	}
}
