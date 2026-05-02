package user

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/mailer"
)

type teamTestMailDriver struct {
	subject  string
	htmlBody string
	textBody string
}

func (d *teamTestMailDriver) Send(_ []string, subject, htmlBody, textBody string) error {
	d.subject = subject
	d.htmlBody = htmlBody
	d.textBody = textBody
	return nil
}

func (d *teamTestMailDriver) Close() error { return nil }

func TestHandleGetTeams(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	activeTeamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active tenant: %v", err)
	}
	if _, err := store.CreateSite(context.Background(), userID, "usage-api.test"); err != nil {
		t.Fatalf("create site: %v", err)
	}
	if _, err := store.CreateTeamInvite(context.Background(), activeTeamID, "pending-team-usage@test.dev", database.TenantRoleMember, nil, userID); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams", nil), userID)
	w := httptest.NewRecorder()

	h.handleGetTeams().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp struct {
		ActiveTeamID  uuid.UUID   `json:"active_team_id"`
		RecentTeamIDs []uuid.UUID `json:"recent_team_ids"`
		Teams         []api.Team  `json:"teams"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ActiveTeamID == uuid.Nil {
		t.Fatalf("expected active_team_id")
	}
	if len(resp.Teams) == 0 {
		t.Fatalf("expected at least one team")
	}
	if len(resp.RecentTeamIDs) == 0 || resp.RecentTeamIDs[0] != resp.ActiveTeamID {
		t.Fatalf("expected recent_team_ids to start with active team, got %+v", resp.RecentTeamIDs)
	}
	if resp.Teams[0].Usage == nil {
		t.Fatalf("expected usage summary on team payload")
	}
	if resp.Teams[0].Entitlements == nil {
		t.Fatalf("expected entitlements on team payload")
	}
	if resp.Teams[0].Usage.CurrentSites != 1 {
		t.Fatalf("expected 1 team site, got %d", resp.Teams[0].Usage.CurrentSites)
	}
	if resp.Teams[0].Usage.CurrentMembers != 1 {
		t.Fatalf("expected 1 team member, got %d", resp.Teams[0].Usage.CurrentMembers)
	}
	if resp.Teams[0].Usage.CurrentPendingInvites != 1 {
		t.Fatalf("expected 1 pending invite, got %d", resp.Teams[0].Usage.CurrentPendingInvites)
	}
}

func TestHandleSetActiveTeam(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	customTenantID := uuid.New()
	if _, err := store.DB().ExecContext(context.Background(),
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		customTenantID, "Custom Active", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert custom tenant: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"team_id": customTenantID.String()})
	req := withTestUser(httptest.NewRequest(http.MethodPut, "/api/user/teams/active", bytes.NewReader(body)), userID)
	w := httptest.NewRecorder()
	h.handleSetActiveTeam().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	if err := store.AddTeamMember(context.Background(), customTenantID, userID, database.TenantRoleAdmin, userID); err != nil {
		t.Fatalf("add team member: %v", err)
	}

	req = withTestUser(httptest.NewRequest(http.MethodPut, "/api/user/teams/active", bytes.NewReader(body)), userID)
	w = httptest.NewRecorder()
	h.handleSetActiveTeam().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var setResp struct {
		Status        string      `json:"status"`
		ActiveTeamID  uuid.UUID   `json:"active_team_id"`
		RecentTeamIDs []uuid.UUID `json:"recent_team_ids"`
	}
	if err := json.NewDecoder(w.Body).Decode(&setResp); err != nil {
		t.Fatalf("decode set active response: %v", err)
	}
	if setResp.Status != "ok" {
		t.Fatalf("expected status %q, got %q", "ok", setResp.Status)
	}
	if setResp.ActiveTeamID != customTenantID {
		t.Fatalf("expected active team %s, got %s", customTenantID, setResp.ActiveTeamID)
	}
	if len(setResp.RecentTeamIDs) == 0 || setResp.RecentTeamIDs[0] != customTenantID {
		t.Fatalf("expected recent_team_ids to start with %s, got %+v", customTenantID, setResp.RecentTeamIDs)
	}
}

func TestHandleAddTeamMemberAdminCannotAssignOwner(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	adminID, err := store.CreateUser(context.Background(), "admin@team.test", "hash")
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), defaultTenantID, adminID, database.TenantRoleAdmin, ownerID); err != nil {
		t.Fatalf("add admin user to team: %v", err)
	}

	targetID, err := store.CreateUser(context.Background(), "target-owner@team.test", "hash")
	if err != nil {
		t.Fatalf("create target user: %v", err)
	}
	if err := store.RemoveTeamMember(context.Background(), defaultTenantID, targetID); err != nil {
		t.Fatalf("remove default team membership for target: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"email": "target-owner@team.test",
		"role":  database.TenantRoleOwner,
	})

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+defaultTenantID.String()+"/members", bytes.NewReader(body)), adminID)
	req.SetPathValue("id", defaultTenantID.String())
	w := httptest.NewRecorder()

	h.handleAddTeamMember().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestHandleUpdateTeamOwnerSuccess(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"name":     "Updated Team Name",
		"logo_url": "https://example.com/logo.png",
	})
	req := withTestUser(httptest.NewRequest(http.MethodPatch, "/api/user/teams/"+defaultTenantID.String(), bytes.NewReader(body)), ownerID)
	req.SetPathValue("id", defaultTenantID.String())
	w := httptest.NewRecorder()

	h.handleUpdateTeam().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	team, err := store.GetTenant(context.Background(), defaultTenantID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if team.Name != "Updated Team Name" {
		t.Fatalf("expected name %q, got %q", "Updated Team Name", team.Name)
	}
	if team.LogoURL != "https://example.com/logo.png" {
		t.Fatalf("expected logo_url %q, got %q", "https://example.com/logo.png", team.LogoURL)
	}
}

func TestHandleUpdateTeamOwnerSuccessViaDeprecatedPutAlias(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"name":     "Legacy Update Alias",
		"logo_url": "https://example.com/legacy-logo.png",
	})
	req := withTestUser(httptest.NewRequest(http.MethodPut, "/api/user/teams/"+defaultTenantID.String(), bytes.NewReader(body)), ownerID)
	req.SetPathValue("id", defaultTenantID.String())
	w := httptest.NewRecorder()

	h.handleUpdateTeam().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestHandleUpdateTeamMemberForbidden(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	memberID, err := store.CreateUser(context.Background(), "member@team.test", "hash")
	if err != nil {
		t.Fatalf("create member user: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), defaultTenantID, memberID, database.TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add member to team: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"name": "Should Fail"})
	req := withTestUser(httptest.NewRequest(http.MethodPatch, "/api/user/teams/"+defaultTenantID.String(), bytes.NewReader(body)), memberID)
	req.SetPathValue("id", defaultTenantID.String())
	w := httptest.NewRecorder()

	h.handleUpdateTeam().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestHandleUpdateTeamEmptyNameRejected(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"name": ""})
	req := withTestUser(httptest.NewRequest(http.MethodPatch, "/api/user/teams/"+defaultTenantID.String(), bytes.NewReader(body)), ownerID)
	req.SetPathValue("id", defaultTenantID.String())
	w := httptest.NewRecorder()

	h.handleUpdateTeam().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestSendTeamInviteEmailFallsBackToInviterLocaleForNewRecipient(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	ctx := context.Background()
	if err := store.UpsertUserPreferences(ctx, ownerID, api.UserPreferences{DefaultLocale: "de"}); err != nil {
		t.Fatalf("set owner locale: %v", err)
	}

	teamID, err := store.GetActiveTenantID(ctx, ownerID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}

	invite, err := store.CreateTeamInvite(ctx, teamID, "neu@example.com", database.TenantRoleMember, nil, ownerID)
	if err != nil {
		t.Fatalf("create team invite: %v", err)
	}

	drv := &teamTestMailDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(drv, h.ctx.Config)

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+teamID.String()+"/members", nil), ownerID)
	h.sendTeamInviteEmail(req, teamID, ownerID, invite)

	if !strings.Contains(drv.subject, "Du wurdest eingeladen") {
		t.Fatalf("expected localized German subject, got %q", drv.subject)
	}
	if !strings.Contains(drv.textBody, "Du wurdest zu einem Team eingeladen") {
		t.Fatalf("expected localized German text body, got:\n%s", drv.textBody)
	}
	if !strings.Contains(drv.textBody, "Passwort festlegen und Team beitreten") {
		t.Fatalf("expected localized German CTA, got:\n%s", drv.textBody)
	}
}

func TestHandleAddTeamMemberSelfInviteBlocked(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	// The owner is already a member — inviting themselves should be rejected.
	ownerUser, err := store.GetUserByID(context.Background(), ownerID)
	if err != nil || ownerUser == nil {
		t.Fatalf("get owner user: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"email": ownerUser.Email,
		"role":  database.TenantRoleMember,
	})

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+defaultTenantID.String()+"/members", bytes.NewReader(body)), ownerID)
	req.SetPathValue("id", defaultTenantID.String())
	w := httptest.NewRecorder()

	h.handleAddTeamMember().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, w.Code, w.Body.String())
	}
}

func TestHandleAddTeamMemberRoleChangeAllowed(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	// Create a user and add them as a member.
	memberID, err := store.CreateUser(context.Background(), "existing-member@team.test", "hash")
	if err != nil {
		t.Fatalf("create member user: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), defaultTenantID, memberID, database.TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add member to team: %v", err)
	}

	// Changing an existing member's role should succeed.
	body, _ := json.Marshal(map[string]string{
		"email": "existing-member@team.test",
		"role":  database.TenantRoleAdmin,
	})

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+defaultTenantID.String()+"/members", bytes.NewReader(body)), ownerID)
	req.SetPathValue("id", defaultTenantID.String())
	w := httptest.NewRecorder()

	h.handleAddTeamMember().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	// Verify the role was actually updated.
	updatedRole, err := store.GetTenantRole(context.Background(), defaultTenantID, memberID)
	if err != nil {
		t.Fatalf("get updated role: %v", err)
	}
	if updatedRole != database.TenantRoleAdmin {
		t.Fatalf("expected role %q, got %q", database.TenantRoleAdmin, updatedRole)
	}
}

func TestHandleAddTeamMemberCreatesPendingInvite(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	customTenantID := uuid.New()
	if _, err := store.DB().ExecContext(context.Background(),
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		customTenantID, "Invites", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert custom tenant: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), customTenantID, ownerID, database.TenantRoleOwner, ownerID); err != nil {
		t.Fatalf("add owner to custom tenant: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"email": "pending-invite@team.test",
		"role":  database.TenantRoleAdmin,
	})
	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+customTenantID.String()+"/members", bytes.NewReader(body)), ownerID)
	req.SetPathValue("id", customTenantID.String())
	w := httptest.NewRecorder()

	h.handleAddTeamMember().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Status   string         `json:"status"`
		IsInvite bool           `json:"is_invite"`
		Invite   api.TeamInvite `json:"invite"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.IsInvite {
		t.Fatalf("expected pending invite response")
	}
	if resp.Invite.Status != database.TeamInviteStatusPending {
		t.Fatalf("expected invite status %q, got %q", database.TeamInviteStatusPending, resp.Invite.Status)
	}

	invites, err := store.ListTeamInvites(context.Background(), customTenantID)
	if err != nil {
		t.Fatalf("list team invites: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("expected 1 pending invite, got %d", len(invites))
	}

	invitee, err := store.GetUserByEmail(context.Background(), "pending-invite@team.test")
	if err != nil || invitee == nil {
		t.Fatalf("get invitee user: %v", err)
	}
	isMember, err := store.IsTenantMember(context.Background(), customTenantID, invitee.ID)
	if err != nil {
		t.Fatalf("check tenant membership: %v", err)
	}
	if isMember {
		t.Fatalf("expected pending invite not to create active team membership")
	}
}

func TestHandleGetAndRevokeTeamInvites(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	customTenantID := uuid.New()
	if _, err := store.DB().ExecContext(context.Background(),
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		customTenantID, "Invite Admin", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert custom tenant: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), customTenantID, ownerID, database.TenantRoleOwner, ownerID); err != nil {
		t.Fatalf("add owner to custom tenant: %v", err)
	}

	invite, err := store.CreateTeamInvite(context.Background(), customTenantID, "revoke-invite@team.test", database.TenantRoleMember, nil, ownerID)
	if err != nil {
		t.Fatalf("create team invite: %v", err)
	}

	listReq := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams/"+customTenantID.String()+"/invites", nil), ownerID)
	listReq.SetPathValue("id", customTenantID.String())
	listW := httptest.NewRecorder()
	h.handleGetTeamInvites().ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, listW.Code, listW.Body.String())
	}

	var invites []api.TeamInvite
	if err := json.NewDecoder(listW.Body).Decode(&invites); err != nil {
		t.Fatalf("decode invites response: %v", err)
	}
	if len(invites) != 1 || invites[0].ID != invite.ID {
		t.Fatalf("expected invite %s, got %+v", invite.ID, invites)
	}

	revokeReq := withTestUser(httptest.NewRequest(http.MethodDelete, "/api/user/teams/"+customTenantID.String()+"/invites/"+invite.ID.String(), nil), ownerID)
	revokeReq.SetPathValue("id", customTenantID.String())
	revokeReq.SetPathValue("inviteId", invite.ID.String())
	revokeW := httptest.NewRecorder()
	h.handleRevokeTeamInvite().ServeHTTP(revokeW, revokeReq)
	if revokeW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, revokeW.Code, revokeW.Body.String())
	}

	invites, err = store.ListTeamInvites(context.Background(), customTenantID)
	if err != nil {
		t.Fatalf("list team invites after revoke: %v", err)
	}
	if len(invites) != 0 {
		t.Fatalf("expected no pending invites after revoke, got %d", len(invites))
	}
}

func TestHandleRemoveTeamMemberLastOwnerBlocked(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	req := withTestUser(
		httptest.NewRequest(
			http.MethodDelete,
			"/api/user/teams/"+defaultTenantID.String()+"/members/"+ownerID.String(),
			nil,
		),
		ownerID,
	)
	req.SetPathValue("id", defaultTenantID.String())
	req.SetPathValue("userId", ownerID.String())
	w := httptest.NewRecorder()

	h.handleRemoveTeamMember().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleCreateTeamSuccess(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	body, _ := json.Marshal(map[string]string{
		"name":     "My New Team",
		"logo_url": "https://example.com/logo.png",
	})

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams", bytes.NewReader(body)), userID)
	w := httptest.NewRecorder()

	h.handleCreateTeam().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp struct {
		Team api.Team `json:"team"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Team.Name != "My New Team" {
		t.Fatalf("expected name %q, got %q", "My New Team", resp.Team.Name)
	}
	if resp.Team.Role != database.TenantRoleOwner {
		t.Fatalf("expected role %q, got %q", database.TenantRoleOwner, resp.Team.Role)
	}

	// Verify creator is an owner member.
	role, err := store.GetTenantRole(context.Background(), resp.Team.ID, userID)
	if err != nil {
		t.Fatalf("get tenant role: %v", err)
	}
	if role != database.TenantRoleOwner {
		t.Fatalf("expected role %q, got %q", database.TenantRoleOwner, role)
	}
}

func TestHandleCreateTeamEmptyNameRejected(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	body, _ := json.Marshal(map[string]string{"name": ""})

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams", bytes.NewReader(body)), userID)
	w := httptest.NewRecorder()

	h.handleCreateTeam().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestHandleLeaveTeamSuccess(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	customTenantID := uuid.New()
	if _, err := store.DB().ExecContext(context.Background(),
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		customTenantID, "Leave Team", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert custom tenant: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), customTenantID, userID, database.TenantRoleAdmin, userID); err != nil {
		t.Fatalf("add team member: %v", err)
	}
	if err := store.SetActiveTenantID(context.Background(), userID, customTenantID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodDelete, "/api/user/teams/"+customTenantID.String()+"/leave", nil), userID)
	req.SetPathValue("id", customTenantID.String())
	w := httptest.NewRecorder()

	h.handleLeaveTeam().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	isMember, err := store.IsTenantMember(context.Background(), customTenantID, userID)
	if err != nil {
		t.Fatalf("check membership: %v", err)
	}
	if isMember {
		t.Fatalf("expected user to be removed from team")
	}

	var resp struct {
		Status        string      `json:"status"`
		ActiveTeamID  uuid.UUID   `json:"active_team_id"`
		RecentTeamIDs []uuid.UUID `json:"recent_team_ids"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode leave response: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected leave status ok, got %q", resp.Status)
	}
	if len(resp.RecentTeamIDs) == 0 || resp.RecentTeamIDs[0] != resp.ActiveTeamID {
		t.Fatalf("expected recent_team_ids to start with active team, got %+v", resp.RecentTeamIDs)
	}
}

func TestHandleLeaveTeamReturnsStructuredError(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodDelete, "/api/user/teams/"+defaultTenantID.String()+"/leave", nil), ownerID)
	req.SetPathValue("id", defaultTenantID.String())
	w := httptest.NewRecorder()

	h.handleLeaveTeam().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode leave error response: %v", err)
	}
	if resp["code"] != "user_only_team" {
		t.Fatalf("expected code %q, got %q", "user_only_team", resp["code"])
	}
}

func TestHandleTransferTeamOwnership(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	targetID, err := store.CreateUser(context.Background(), "transfer-target@team.test", "hash")
	if err != nil {
		t.Fatalf("create target user: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), defaultTenantID, targetID, database.TenantRoleAdmin, ownerID); err != nil {
		t.Fatalf("add target user to team: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"target_user_id": targetID.String()})
	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+defaultTenantID.String()+"/transfer-ownership", bytes.NewReader(body)), ownerID)
	req.SetPathValue("id", defaultTenantID.String())
	w := httptest.NewRecorder()

	h.handleTransferTeamOwnership().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	newOwnerRole, err := store.GetTenantRole(context.Background(), defaultTenantID, targetID)
	if err != nil {
		t.Fatalf("get new owner role: %v", err)
	}
	if newOwnerRole != database.TenantRoleOwner {
		t.Fatalf("expected transferred owner role %q, got %q", database.TenantRoleOwner, newOwnerRole)
	}

	previousOwnerRole, err := store.GetTenantRole(context.Background(), defaultTenantID, ownerID)
	if err != nil {
		t.Fatalf("get previous owner role: %v", err)
	}
	if previousOwnerRole != database.TenantRoleAdmin {
		t.Fatalf("expected previous owner role %q, got %q", database.TenantRoleAdmin, previousOwnerRole)
	}
}

func TestHandleArchiveTeam(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	team, err := store.CreateTenant(context.Background(), ownerID, "Archive API", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := store.SetActiveTenantID(context.Background(), ownerID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+team.ID.String()+"/archive", nil), ownerID)
	req.SetPathValue("id", team.ID.String())
	w := httptest.NewRecorder()

	h.handleArchiveTeam().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Status        string      `json:"status"`
		ActiveTeamID  uuid.UUID   `json:"active_team_id"`
		RecentTeamIDs []uuid.UUID `json:"recent_team_ids"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode archive response: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status ok, got %q", resp.Status)
	}
	if resp.ActiveTeamID == team.ID {
		t.Fatalf("expected archived team not to remain active")
	}

	teams, _, err := store.ListUserTeams(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list user teams: %v", err)
	}
	for _, listedTeam := range teams {
		if listedTeam.ID == team.ID {
			t.Fatalf("expected archived team %s to be omitted from team list", team.ID)
		}
	}
}

func TestHandleArchiveTeamReturnsStructuredErrorWhenSitesRemain(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	team, err := store.CreateTenant(context.Background(), ownerID, "Archive Blocked API", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := store.SetActiveTenantID(context.Background(), ownerID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	if _, err := store.CreateSite(context.Background(), ownerID, "archive-api-blocked.test"); err != nil {
		t.Fatalf("create site: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+team.ID.String()+"/archive", nil), ownerID)
	req.SetPathValue("id", team.ID.String())
	w := httptest.NewRecorder()

	h.handleArchiveTeam().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode archive error response: %v", err)
	}
	if resp["code"] != "team_archive_has_sites" {
		t.Fatalf("expected code %q, got %q", "team_archive_has_sites", resp["code"])
	}
}

func TestHandleGetTeamAudit(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	memberID, err := store.CreateUser(context.Background(), "audit-member@team.test", "hash")
	if err != nil {
		t.Fatalf("create member user: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), defaultTenantID, memberID, database.TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add member to team: %v", err)
	}

	// Trigger an auditable action.
	body, _ := json.Marshal(map[string]string{
		"email": "audit-member@team.test",
		"role":  database.TenantRoleAdmin,
	})
	updateReq := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+defaultTenantID.String()+"/members", bytes.NewReader(body)), ownerID)
	updateReq.RemoteAddr = "203.0.113.11:1234"
	updateReq.Header.Set("User-Agent", "team-audit-test")
	updateReq.Header.Set("X-Request-Id", "req-team-audit")
	updateReq.SetPathValue("id", defaultTenantID.String())
	updateW := httptest.NewRecorder()
	h.handleAddTeamMember().ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, updateW.Code)
	}

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams/"+defaultTenantID.String()+"/audit?limit=5&action=member.role_updated&outcome=success&target_type=user&query=audit-member%40team.test", nil), ownerID)
	req.SetPathValue("id", defaultTenantID.String())
	w := httptest.NewRecorder()
	h.handleGetTeamAudit().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var response api.TeamAuditListResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}
	if len(response.Entries) == 0 {
		t.Fatalf("expected at least one audit entry")
	}
	if response.Action != "member.role_updated" {
		t.Fatalf("expected action filter to be echoed, got %q", response.Action)
	}
	if response.Limit != 5 {
		t.Fatalf("expected limit 5, got %d", response.Limit)
	}
	entry := response.Entries[0]
	if entry.TargetType != "user" {
		t.Fatalf("expected target type user, got %q", entry.TargetType)
	}
	if entry.TargetLabel != "audit-member@team.test" {
		t.Fatalf("expected target label, got %q", entry.TargetLabel)
	}
	if entry.Outcome != "success" {
		t.Fatalf("expected success outcome, got %q", entry.Outcome)
	}
	if entry.TargetUserID == nil || *entry.TargetUserID != memberID {
		t.Fatalf("expected target user %s, got %v", memberID, entry.TargetUserID)
	}
	if entry.IPAddress != "203.0.113.11" {
		t.Fatalf("expected request IP, got %q", entry.IPAddress)
	}
	if entry.UserAgent != "team-audit-test" || entry.RequestID != "req-team-audit" {
		t.Fatalf("expected request evidence, got user_agent=%q request_id=%q", entry.UserAgent, entry.RequestID)
	}
}

func TestHandleGetTeamAuditAllowsAdminAndRejectsMembers(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	adminID, err := store.CreateUser(context.Background(), "audit-admin@team.test", "hash")
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), defaultTenantID, adminID, database.TenantRoleAdmin, ownerID); err != nil {
		t.Fatalf("add admin to team: %v", err)
	}

	memberID, err := store.CreateUser(context.Background(), "audit-basic@team.test", "hash")
	if err != nil {
		t.Fatalf("create member user: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), defaultTenantID, memberID, database.TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add member to team: %v", err)
	}

	if err := store.AppendTeamAuditEntry(context.Background(), defaultTenantID, ownerID, "team.updated", "Updated logo", nil); err != nil {
		t.Fatalf("append team audit entry: %v", err)
	}

	adminReq := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams/"+defaultTenantID.String()+"/audit", nil), adminID)
	adminReq.SetPathValue("id", defaultTenantID.String())
	adminW := httptest.NewRecorder()
	h.handleGetTeamAudit().ServeHTTP(adminW, adminReq)
	if adminW.Code != http.StatusOK {
		t.Fatalf("expected admin status %d, got %d: %s", http.StatusOK, adminW.Code, adminW.Body.String())
	}

	memberReq := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams/"+defaultTenantID.String()+"/audit", nil), memberID)
	memberReq.SetPathValue("id", defaultTenantID.String())
	memberW := httptest.NewRecorder()
	h.handleGetTeamAudit().ServeHTTP(memberW, memberReq)
	if memberW.Code != http.StatusForbidden {
		t.Fatalf("expected member status %d, got %d: %s", http.StatusForbidden, memberW.Code, memberW.Body.String())
	}
}

func TestHandleTeamAPIClientLifecycle(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	team, err := store.CreateTenant(context.Background(), ownerID, "Team Keys", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := store.SetActiveTenantID(context.Background(), ownerID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	site, err := store.CreateSite(context.Background(), ownerID, "team-api-handler.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	createBody, _ := json.Marshal(map[string]any{
		"name":        "Shared automation",
		"description": "Runs team syncs",
		"site_roles": []map[string]string{
			{"site_id": site.ID.String(), "role": "admin"},
		},
	})
	createReq := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+team.ID.String()+"/api-clients", bytes.NewReader(createBody)), ownerID)
	createReq.SetPathValue("id", team.ID.String())
	createW := httptest.NewRecorder()
	h.handleCreateTeamAPIClient().ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, createW.Code, createW.Body.String())
	}

	var createResp struct {
		Client api.APIClient `json:"client"`
		Token  string        `json:"token"`
	}
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Token == "" {
		t.Fatalf("expected one-time token")
	}
	if createResp.Client.OwnerType != database.APIClientOwnerTeam {
		t.Fatalf("expected team owner type, got %q", createResp.Client.OwnerType)
	}
	if createResp.Client.TenantID == nil || *createResp.Client.TenantID != team.ID {
		t.Fatalf("expected tenant id %s, got %+v", team.ID, createResp.Client.TenantID)
	}

	listReq := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams/"+team.ID.String()+"/api-clients", nil), ownerID)
	listReq.SetPathValue("id", team.ID.String())
	listW := httptest.NewRecorder()
	h.handleListTeamAPIClients().ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, listW.Code, listW.Body.String())
	}

	var clients []api.APIClient
	if err := json.NewDecoder(listW.Body).Decode(&clients); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(clients) != 1 || clients[0].ID != createResp.Client.ID {
		t.Fatalf("expected one matching team api client, got %+v", clients)
	}

	updateBody, _ := json.Marshal(map[string]any{
		"name":        "Shared automation updated",
		"description": "Runs team syncs safely",
		"revoked":     true,
		"site_roles": []map[string]string{
			{"site_id": site.ID.String(), "role": "viewer"},
		},
	})
	updateReq := withTestUser(httptest.NewRequest(http.MethodPut, "/api/user/teams/"+team.ID.String()+"/api-clients/"+createResp.Client.ID.String(), bytes.NewReader(updateBody)), ownerID)
	updateReq.SetPathValue("id", team.ID.String())
	updateReq.SetPathValue("clientId", createResp.Client.ID.String())
	updateW := httptest.NewRecorder()
	h.handleUpdateTeamAPIClient().ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, updateW.Code, updateW.Body.String())
	}

	var updated api.APIClient
	if err := json.NewDecoder(updateW.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.RevokedAt == nil {
		t.Fatalf("expected revoked timestamp after update")
	}

	deleteReq := withTestUser(httptest.NewRequest(http.MethodDelete, "/api/user/teams/"+team.ID.String()+"/api-clients/"+createResp.Client.ID.String(), nil), ownerID)
	deleteReq.SetPathValue("id", team.ID.String())
	deleteReq.SetPathValue("clientId", createResp.Client.ID.String())
	deleteW := httptest.NewRecorder()
	h.handleDeleteTeamAPIClient().ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNoContent, deleteW.Code, deleteW.Body.String())
	}
}

func TestHandleTeamAPIClientRequiresTeamAdmin(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	team, err := store.CreateTenant(context.Background(), ownerID, "Restricted Team Keys", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	memberID, err := store.CreateUser(context.Background(), "member-api-client@team.test", "hash")
	if err != nil {
		t.Fatalf("create member user: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), team.ID, memberID, database.TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add member: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"name": "Should fail"})
	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+team.ID.String()+"/api-clients", bytes.NewReader(body)), memberID)
	req.SetPathValue("id", team.ID.String())
	w := httptest.NewRecorder()
	h.handleCreateTeamAPIClient().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
}
