//go:build billing

package user

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	serverauth "hitkeep/internal/server/auth"
)

func TestHandleCreateTeamAllowsHostedCloudInstanceOwner(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	h.ctx.Config.CloudHosted = true

	body, err := json.Marshal(map[string]string{
		"name": "Second Team",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams", bytes.NewReader(body)), ownerID)
	w := httptest.NewRecorder()
	h.handleCreateTeam().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	teams, _, err := store.ListUserTeams(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list teams: %v", err)
	}
	if len(teams) != 2 {
		t.Fatalf("expected operator to have two teams, got %d", len(teams))
	}
}

func TestHandleCreateTeamRejectsHostedCloudUser(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	h.ctx.Config.CloudHosted = true
	// Mirrors the managed-cloud default of HITKEEP_CLOUD_MAX_TEAMS=1.
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{MaxTeams: 1}, entitlements.PlanInfo{})

	hashed, err := serverauth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "cloud-user@team.test", hashed)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.UpdateInstanceRole(context.Background(), userID, auth.InstanceUser, ownerID); err != nil {
		t.Fatalf("set user role: %v", err)
	}

	body, err := json.Marshal(map[string]string{
		"name": "Second Team",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams", bytes.NewReader(body)), userID)
	w := httptest.NewRecorder()
	h.handleCreateTeam().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
}

func TestHandleGetTeamsReturnsOperatorPlanForHostedCloudOwnerTeams(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	h.ctx.Config.CloudHosted = true
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{
		MaxSitesPerTeam:  3,
		MaxTeamMembers:   3,
		MaxRetentionDays: 60,
	}, entitlements.PlanInfo{
		Code: "free",
		Name: "Free",
	})

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetTeams().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Teams []api.Team `json:"teams"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Teams) == 0 {
		t.Fatal("expected at least one team")
	}
	if resp.Teams[0].Plan == nil || resp.Teams[0].Plan.Code != operatorCloudPlanCode {
		t.Fatalf("expected operator plan, got %+v", resp.Teams[0].Plan)
	}
	if resp.Teams[0].Entitlements == nil || resp.Teams[0].Entitlements.MaxTeamMembers != 0 || !resp.Teams[0].Entitlements.AllowSSO {
		t.Fatalf("expected unlimited operator entitlements, got %+v", resp.Teams[0].Entitlements)
	}
}

func TestHandleGetTeamsDoesNotReturnOperatorPlanForHostedCloudNonOwner(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	h.ctx.Config.CloudHosted = true
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{
		MaxSitesPerTeam:  3,
		MaxTeamMembers:   3,
		MaxRetentionDays: 60,
	}, entitlements.PlanInfo{
		Code: "free",
		Name: "Free",
	})

	hashed, err := serverauth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	adminID, err := store.CreateUser(context.Background(), "cloud-admin@team.test", hashed)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := store.UpdateInstanceRole(context.Background(), adminID, auth.InstanceOwner, ownerID); err != nil {
		t.Fatalf("set instance owner role: %v", err)
	}
	defaultTeamID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default team: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), defaultTeamID, adminID, database.TenantRoleAdmin, ownerID); err != nil {
		t.Fatalf("add admin to team: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams", nil), adminID)
	w := httptest.NewRecorder()
	h.handleGetTeams().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Teams []api.Team `json:"teams"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Teams) == 0 {
		t.Fatal("expected at least one team")
	}
	if resp.Teams[0].Plan == nil || resp.Teams[0].Plan.Code == operatorCloudPlanCode {
		t.Fatalf("expected non-operator plan, got %+v", resp.Teams[0].Plan)
	}
}

func TestHandleArchiveTeamRejectsHostedCloud(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	h.ctx.Config.CloudHosted = true

	team, err := store.CreateTenant(context.Background(), userID, "Cloud Team", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+team.ID.String()+"/archive", nil), userID)
	req.SetPathValue("id", team.ID.String())
	w := httptest.NewRecorder()

	h.handleArchiveTeam().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
}

func TestHandleUpsertTeamMemberRejectsExistingHostedCloudUserFromOtherTeam(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	h.ctx.Config.CloudHosted = true
	// Mirrors the managed-cloud default of HITKEEP_CLOUD_MAX_TEAMS=1.
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{MaxTeams: 1}, entitlements.PlanInfo{})

	activeTeamID, err := store.GetActiveTenantID(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("get active tenant: %v", err)
	}

	existingHash, err := serverauth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	existingUserID, err := store.CreateUserWithoutDefaultTenant(context.Background(), "member@cloud.test", existingHash)
	if err != nil {
		t.Fatalf("create hosted user: %v", err)
	}

	_, err = store.CreateTenant(context.Background(), existingUserID, "Existing Hosted Team", "")
	if err != nil {
		t.Fatalf("create hosted team: %v", err)
	}
	body, err := json.Marshal(map[string]string{
		"email": "member@cloud.test",
		"role":  database.TenantRoleMember,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+activeTeamID.String()+"/members", bytes.NewReader(body)), ownerID)
	req.SetPathValue("id", activeTeamID.String())
	w := httptest.NewRecorder()
	h.handleAddTeamMember().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, w.Code, w.Body.String())
	}
}

func TestHandleUpsertTeamMemberAllowsOperatorOwnedTeamPastMemberLimitForNewUsers(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	h.ctx.Config.CloudHosted = true
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{
		MaxTeamMembers: 1,
	}, entitlements.PlanInfo{
		Code: "free",
		Name: "Free",
	})

	activeTeamID, err := store.GetActiveTenantID(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("get active tenant: %v", err)
	}

	body, err := json.Marshal(map[string]string{
		"email": "new-operator-member@cloud.test",
		"role":  database.TenantRoleMember,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+activeTeamID.String()+"/members", bytes.NewReader(body)), ownerID)
	req.SetPathValue("id", activeTeamID.String())
	w := httptest.NewRecorder()
	h.handleAddTeamMember().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestHandleUpsertTeamMemberRejectsHostedCloudUserPastMemberLimit(t *testing.T) {
	h, store, _ := setupUserSecurityTestEnv(t)
	defer store.Close()

	h.ctx.Config.CloudHosted = true
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{
		MaxTeamMembers: 1,
	}, entitlements.PlanInfo{
		Code: "free",
		Name: "Free",
	})

	hashed, err := serverauth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	customerID, err := store.CreateUserWithoutDefaultTenant(context.Background(), "customer-owner@cloud.test", hashed)
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}
	team, err := store.CreateTenant(context.Background(), customerID, "Customer Team", "")
	if err != nil {
		t.Fatalf("create customer team: %v", err)
	}

	body, err := json.Marshal(map[string]string{
		"email": "blocked-new-member@cloud.test",
		"role":  database.TenantRoleMember,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+team.ID.String()+"/members", bytes.NewReader(body)), customerID)
	req.SetPathValue("id", team.ID.String())
	w := httptest.NewRecorder()
	h.handleAddTeamMember().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
	if user, err := store.GetUserByEmail(context.Background(), "blocked-new-member@cloud.test"); err != nil || user != nil {
		t.Fatalf("expected rejected invite not to create user, user=%+v err=%v", user, err)
	}
}

func TestHandleUpsertTeamMemberBypassesLimitForOperatorOnAnyTeam(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	h.ctx.Config.CloudHosted = true
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{
		MaxTeamMembers: 2,
	}, entitlements.PlanInfo{
		Code: "free",
		Name: "Free",
	})

	hashed, err := serverauth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	customerID, err := store.CreateUserWithoutDefaultTenant(context.Background(), "customer-owner-operator-admin@cloud.test", hashed)
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}
	team, err := store.CreateTenant(context.Background(), customerID, "Customer Team", "")
	if err != nil {
		t.Fatalf("create customer team: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), team.ID, ownerID, database.TenantRoleAdmin, customerID); err != nil {
		t.Fatalf("add operator as team admin: %v", err)
	}

	body, err := json.Marshal(map[string]string{
		"email": "blocked-operator-admin-member@cloud.test",
		"role":  database.TenantRoleMember,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+team.ID.String()+"/members", bytes.NewReader(body)), ownerID)
	req.SetPathValue("id", team.ID.String())
	w := httptest.NewRecorder()
	h.handleAddTeamMember().ServeHTTP(w, req)

	// Instance staff bypass cloud limits in any context, including teams they
	// administer without owning.
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}
