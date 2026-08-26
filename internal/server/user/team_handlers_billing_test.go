//go:build billing

package user

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/testutil/testdb"
	json "hitkeep/jsonapi"
)

func TestHandleGetTeamsIncludesPlanMetadata(t *testing.T) {
	store := testdb.Shared(t)
	defer store.Close()

	userID, err := store.CreateUser(context.Background(), "plan@test.dev", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.UpdateInstanceRole(context.Background(), userID, authcore.InstanceUser, userID); err != nil {
		t.Fatalf("demote user: %v", err)
	}
	teams, _, err := store.ListUserTeams(context.Background(), userID)
	if err != nil {
		t.Fatalf("list user teams: %v", err)
	}
	if len(teams) == 0 {
		t.Fatal("expected at least one team after user creation")
	}
	if err := store.UpsertCloudBillingAccount(context.Background(), database.CloudBillingAccount{
		TenantID:           teams[0].ID,
		PlanCode:           database.CloudPlanPro,
		PlanName:           "Pro",
		SubscriptionStatus: "active",
	}); err != nil {
		t.Fatalf("seed cloud billing account: %v", err)
	}

	h := &handler{
		ctx: &shared.Context{
			Store:        store,
			TenantStores: database.NewTenantStoreManager(store, t.TempDir()),
			Config:       &config.Config{CloudHosted: true},
			Entitlements: entitlements.NewProvider(&config.Config{
				CloudHosted:          true,
				CloudPlanCode:        "free",
				CloudPlanName:        "Free",
				CloudUpgradeURL:      "https://hitkeep.com/cloud/upgrade",
				CloudSupportURL:      "https://hitkeep.com/cloud/support",
				CloudMaxTeams:        1,
				CloudMaxSitesPerTeam: 3,

				CloudMaxRetentionDays:    60,
				CloudMaxTeamMembers:      3,
				CloudAllowSSO:            false,
				CloudAllowCustomBranding: false,
			}),
		},
	}

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams", nil), userID)
	w := httptest.NewRecorder()
	h.handleGetTeams().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp struct {
		Teams []api.Team `json:"teams"`
	}
	if err := json.UnmarshalRead(w.Body, &resp); err != nil {
		t.Fatalf("decode teams response: %v", err)
	}
	if len(resp.Teams) == 0 {
		t.Fatalf("expected at least one team")
	}
	if resp.Teams[0].Plan == nil {
		t.Fatalf("expected plan metadata on team payload")
	}
	if resp.Teams[0].Plan.Code != "pro" || resp.Teams[0].Plan.Name != "Pro" {
		t.Fatalf("unexpected plan metadata: %+v", resp.Teams[0].Plan)
	}
	if resp.Teams[0].Entitlements == nil || resp.Teams[0].Entitlements.MaxRetentionDays != 365 || !resp.Teams[0].Entitlements.AllowExternalReportRecipients {
		t.Fatalf("expected cloud entitlements on team payload")
	}
}

func TestHandleGetTeamsTreatsPendingCheckoutAsFreePlan(t *testing.T) {
	store := testdb.Shared(t)
	defer store.Close()

	userID, err := store.CreateUser(context.Background(), "pending@test.dev", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.UpdateInstanceRole(context.Background(), userID, authcore.InstanceUser, userID); err != nil {
		t.Fatalf("demote user: %v", err)
	}
	teams, _, err := store.ListUserTeams(context.Background(), userID)
	if err != nil {
		t.Fatalf("list user teams: %v", err)
	}
	if len(teams) == 0 {
		t.Fatal("expected at least one team after user creation")
	}
	if err := store.UpsertCloudBillingAccount(context.Background(), database.CloudBillingAccount{
		TenantID:           teams[0].ID,
		PlanCode:           database.CloudPlanPro,
		PlanName:           "Pro",
		SubscriptionStatus: "pending_checkout",
		StripeCustomerID:   "cus_pending",
		StripePriceID:      "price_pending",
	}); err != nil {
		t.Fatalf("seed cloud billing account: %v", err)
	}

	h := &handler{
		ctx: &shared.Context{
			Store:        store,
			TenantStores: database.NewTenantStoreManager(store, t.TempDir()),
			Config:       &config.Config{CloudHosted: true},
			Entitlements: entitlements.NewProvider(&config.Config{
				CloudHosted:          true,
				CloudPlanCode:        "free",
				CloudPlanName:        "Free",
				CloudUpgradeURL:      "https://hitkeep.com/cloud/upgrade",
				CloudSupportURL:      "https://hitkeep.com/cloud/support",
				CloudMaxTeams:        1,
				CloudMaxSitesPerTeam: 3,

				CloudMaxRetentionDays:    60,
				CloudMaxTeamMembers:      3,
				CloudAllowSSO:            false,
				CloudAllowCustomBranding: false,
			}),
		},
	}

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams", nil), userID)
	w := httptest.NewRecorder()
	h.handleGetTeams().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp struct {
		Teams []api.Team `json:"teams"`
	}
	if err := json.UnmarshalRead(w.Body, &resp); err != nil {
		t.Fatalf("decode teams response: %v", err)
	}
	if len(resp.Teams) == 0 {
		t.Fatalf("expected at least one team")
	}
	if resp.Teams[0].Plan == nil {
		t.Fatalf("expected plan metadata on team payload")
	}
	if resp.Teams[0].Plan.Code != "free" || resp.Teams[0].Plan.Name != "Free" {
		t.Fatalf("unexpected plan metadata: %+v", resp.Teams[0].Plan)
	}
	if resp.Teams[0].Entitlements == nil || resp.Teams[0].Entitlements.MaxRetentionDays != 60 || resp.Teams[0].Entitlements.AllowExternalReportRecipients {
		t.Fatalf("expected free entitlements on pending checkout payload")
	}
}
