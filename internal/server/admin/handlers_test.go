package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/mailer"
	"hitkeep/internal/server/shared"
)

type adminTestMailDriver struct {
	recipients []string
	subject    string
	htmlBody   string
	textBody   string
}

func (d *adminTestMailDriver) Send(recipients []string, subject, htmlBody, textBody string) error {
	d.recipients = recipients
	d.subject = subject
	d.htmlBody = htmlBody
	d.textBody = textBody
	return nil
}

func (d *adminTestMailDriver) Close() error { return nil }

func setupAdminTestEnv(t *testing.T) (*handler, *database.Store, *database.TenantStoreManager, string, uuid.UUID, uuid.UUID) {
	t.Helper()

	basePath := t.TempDir()
	store := database.NewStore(filepath.Join(basePath, "shared.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	targetUserID, err := store.CreateUser(context.Background(), "target-owner@example.com", "hash")
	if err != nil {
		t.Fatalf("create target user: %v", err)
	}
	actorUserID, err := store.CreateUser(context.Background(), "admin@example.com", "hash")
	if err != nil {
		t.Fatalf("create actor user: %v", err)
	}

	tenantStores := database.NewTenantStoreManager(store, basePath)
	t.Cleanup(func() { _ = tenantStores.Close() })

	ctx := &shared.Context{
		Store:        store,
		TenantStores: tenantStores,
		Config: &config.Config{
			PublicURL: "http://localhost:8080",
			JWTSecret: "test-secret",
		},
	}

	return &handler{ctx: ctx}, store, tenantStores, basePath, actorUserID, targetUserID
}

func withAdminTestUser(req *http.Request, userID uuid.UUID) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
}

func TestHandleCreateInstanceCountryExclusion(t *testing.T) {
	h, store, _, _, actorUserID, _ := setupAdminTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/exclusions", strings.NewReader(`{"type":"country","country_code":"us","description":"United States"}`)), actorUserID)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("User-Agent", "global-exclusion-test")
	req.Header.Set("X-Request-Id", "req-global-exclusion-create")
	w := httptest.NewRecorder()

	h.handleCreateInstanceExclusion().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var created api.IPExclusion
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode created exclusion: %v", err)
	}
	if created.Type != "country" || created.CountryCode != "US" || created.CIDR != "" {
		t.Fatalf("unexpected created exclusion: %+v", created)
	}
	assertGlobalExclusionAuditEntry(t, store, "site.exclusion_created", created.ID.String(), "US", "203.0.113.10", "global-exclusion-test", "req-global-exclusion-create")

	listReq := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/exclusions", nil), actorUserID)
	listW := httptest.NewRecorder()
	h.handleListInstanceExclusions().ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d: %s", http.StatusOK, listW.Code, listW.Body.String())
	}
	var rules []api.IPExclusion
	if err := json.NewDecoder(listW.Body).Decode(&rules); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(rules) != 1 || rules[0].Type != "country" || rules[0].CountryCode != "US" {
		t.Fatalf("unexpected listed rules: %+v", rules)
	}

	badReq := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/exclusions", strings.NewReader(`{"type":"country","country_code":"usa"}`)), actorUserID)
	badW := httptest.NewRecorder()
	h.handleCreateInstanceExclusion().ServeHTTP(badW, badReq)
	if badW.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid country status %d, got %d: %s", http.StatusBadRequest, badW.Code, badW.Body.String())
	}

	deleteReq := withAdminTestUser(httptest.NewRequest(http.MethodDelete, "/api/admin/exclusions/"+created.ID.String(), nil), actorUserID)
	deleteReq.SetPathValue("ruleID", created.ID.String())
	deleteW := httptest.NewRecorder()

	h.handleDeleteInstanceExclusion().ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusNoContent {
		t.Fatalf("expected delete status %d, got %d: %s", http.StatusNoContent, deleteW.Code, deleteW.Body.String())
	}
	assertGlobalExclusionAuditEntry(t, store, "site.exclusion_deleted", created.ID.String(), created.ID.String(), "", "", "")
}

func TestHandleDeleteUserReturnsConflictForSoleOwner(t *testing.T) {
	h, store, _, _, actorUserID, targetUserID := setupAdminTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodDelete, "/api/admin/users/"+targetUserID.String(), nil), actorUserID)
	req.SetPathValue("id", targetUserID.String())
	w := httptest.NewRecorder()

	h.handleDeleteUser().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, w.Code, w.Body.String())
	}

	var resp api.AdminDeleteUserBlockedResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != "user_owns_teams" {
		t.Fatalf("expected code %q, got %q", "user_owns_teams", resp.Code)
	}
	if len(resp.Teams) != 1 || resp.Teams[0].Name != "Default Tenant" {
		t.Fatalf("expected default tenant blocking payload, got %+v", resp.Teams)
	}

	user, err := store.GetUserByID(context.Background(), targetUserID)
	if err != nil {
		t.Fatalf("lookup blocked user: %v", err)
	}
	if user == nil {
		t.Fatal("expected blocked user to remain")
	}
}

func TestHandleDeleteUserDeletesWhenTeamHasAnotherOwner(t *testing.T) {
	h, store, _, _, actorUserID, targetUserID := setupAdminTestEnv(t)

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), defaultTenantID, actorUserID, database.TenantRoleOwner, targetUserID); err != nil {
		t.Fatalf("promote actor to owner: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodDelete, "/api/admin/users/"+targetUserID.String(), nil), actorUserID)
	req.SetPathValue("id", targetUserID.String())
	w := httptest.NewRecorder()

	h.handleDeleteUser().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	user, err := store.GetUserByID(context.Background(), targetUserID)
	if err != nil {
		t.Fatalf("lookup deleted user: %v", err)
	}
	if user != nil {
		t.Fatalf("expected user to be deleted, got %+v", user)
	}
}

func TestHandleDeleteUserRemovesTenantLocalOwnedSites(t *testing.T) {
	h, store, tenantStores, _, actorUserID, targetUserID := setupAdminTestEnv(t)
	ctx := context.Background()

	team, err := store.CreateTenant(ctx, targetUserID, "Tenant Local Cleanup", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	defaultTenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	if err := store.AddTeamMember(ctx, defaultTenantID, actorUserID, database.TenantRoleOwner, targetUserID); err != nil {
		t.Fatalf("promote actor to default owner: %v", err)
	}
	if err := store.AddTeamMember(ctx, team.ID, actorUserID, database.TenantRoleOwner, targetUserID); err != nil {
		t.Fatalf("add actor as team owner: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, targetUserID, team.ID); err != nil {
		t.Fatalf("set target active team: %v", err)
	}

	site, err := store.CreateSite(ctx, targetUserID, "tenant-local-delete.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := tenantStores.SyncSite(ctx, site.ID); err != nil {
		t.Fatalf("sync tenant site: %v", err)
	}
	tenantStore, err := tenantStores.ForTenant(ctx, team.ID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}
	now := time.Now().UTC()
	if _, err := tenantStore.DB().ExecContext(ctx,
		"INSERT INTO hits (id, site_id, session_id, page_id, timestamp, path) VALUES (?, ?, ?, ?, ?, ?)",
		uuid.New(), site.ID, uuid.New(), uuid.New(), now, "/",
	); err != nil {
		t.Fatalf("insert tenant hit: %v", err)
	}
	if _, err := tenantStore.DB().ExecContext(ctx,
		"INSERT INTO web_vitals (id, site_id, session_id, page_id, metric, value, rating, path, timestamp) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		uuid.New(), site.ID, uuid.New(), uuid.New(), "LCP", 2600, "needs_improvement", "/pricing", now,
	); err != nil {
		t.Fatalf("insert tenant web vital: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodDelete, "/api/admin/users/"+targetUserID.String(), nil), actorUserID)
	req.SetPathValue("id", targetUserID.String())
	w := httptest.NewRecorder()

	h.handleDeleteUser().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var hitCount int
	if err := tenantStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", site.ID).Scan(&hitCount); err != nil {
		t.Fatalf("count tenant hit rows: %v", err)
	}
	if hitCount != 0 {
		t.Fatalf("expected tenant hit rows to be deleted, got %d", hitCount)
	}

	var webVitalsCount int
	if err := tenantStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM web_vitals WHERE site_id = ?", site.ID).Scan(&webVitalsCount); err != nil {
		t.Fatalf("count tenant web vital rows: %v", err)
	}
	if webVitalsCount != 0 {
		t.Fatalf("expected tenant web vital rows to be deleted, got %d", webVitalsCount)
	}

	var siteCount int
	if err := tenantStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM sites WHERE id = ?", site.ID).Scan(&siteCount); err != nil {
		t.Fatalf("count tenant site rows: %v", err)
	}
	if siteCount != 0 {
		t.Fatalf("expected tenant site rows to be deleted, got %d", siteCount)
	}
}

func TestHandleDisableUser2FARequiresOwner(t *testing.T) {
	h, _, _, _, actorUserID, targetUserID := setupAdminTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/users/"+targetUserID.String()+"/disable-2fa", nil), actorUserID)
	req.SetPathValue("id", targetUserID.String())
	w := httptest.NewRecorder()

	h.handleDisableUser2FA().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
}

func TestHandleDisableUser2FADisablesTargetFactors(t *testing.T) {
	h, store, _, _, actorUserID, targetUserID := setupAdminTestEnv(t)
	ctx := context.Background()

	if err := store.UpdateInstanceRole(ctx, actorUserID, auth.InstanceOwner, targetUserID); err != nil {
		t.Fatalf("promote actor to owner: %v", err)
	}

	if err := store.EnableUserTOTP(ctx, targetUserID, "totp-secret"); err != nil {
		t.Fatalf("enable target totp: %v", err)
	}
	if _, err := store.CreateUserPasskey(ctx, targetUserID, "Recovery key", "credential-1", "public-key", nil); err != nil {
		t.Fatalf("create target passkey: %v", err)
	}
	token, err := store.CreateRememberMeToken(ctx, targetUserID)
	if err != nil {
		t.Fatalf("create remember me token: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/users/"+targetUserID.String()+"/disable-2fa", nil), actorUserID)
	req.SetPathValue("id", targetUserID.String())
	w := httptest.NewRecorder()

	h.handleDisableUser2FA().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp api.AdminDisableUserMFAResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status ok, got %q", resp.Status)
	}
	if !resp.TOTPDisabled {
		t.Fatal("expected TOTP to be disabled")
	}
	if resp.PasskeysDeleted != 1 {
		t.Fatalf("expected 1 deleted passkey, got %d", resp.PasskeysDeleted)
	}
	if resp.SessionsInvalidated != 1 {
		t.Fatalf("expected 1 invalidated session, got %d", resp.SessionsInvalidated)
	}

	hasTOTP, err := store.HasEnabledTOTP(ctx, targetUserID)
	if err != nil {
		t.Fatalf("check target totp: %v", err)
	}
	if hasTOTP {
		t.Fatal("expected target totp to be disabled")
	}

	passkeys, err := store.ListUserPasskeys(ctx, targetUserID)
	if err != nil {
		t.Fatalf("list target passkeys: %v", err)
	}
	if len(passkeys) != 0 {
		t.Fatalf("expected target passkeys to be deleted, got %d", len(passkeys))
	}

	rememberedUserID, err := store.ValidateRememberMeToken(ctx, token)
	if err != nil {
		t.Fatalf("validate remember me token: %v", err)
	}
	if rememberedUserID != uuid.Nil {
		t.Fatalf("expected target remember me token to be invalidated, got %s", rememberedUserID)
	}
}

func TestHandleDeleteTeamPurgesArchivedTenantData(t *testing.T) {
	h, store, tenantStores, basePath, actorUserID, _ := setupAdminTestEnv(t)
	ctx := context.Background()

	team, err := store.CreateTenant(ctx, actorUserID, "Archived Team", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if _, err := tenantStores.ForTenant(ctx, team.ID); err != nil {
		t.Fatalf("open tenant store: %v", err)
	}

	dbPath := filepath.Join(basePath, "tenants", team.ID.String(), "hitkeep.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected tenant db to exist before purge: %v", err)
	}

	if err := store.ArchiveTenant(ctx, team.ID, actorUserID); err != nil {
		t.Fatalf("archive team: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodDelete, "/api/admin/teams/"+team.ID.String(), nil), actorUserID)
	req.SetPathValue("id", team.ID.String())
	w := httptest.NewRecorder()

	h.handleAdminDeleteTeam().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp api.AdminDeleteTeamResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.TeamID != team.ID {
		t.Fatalf("expected deleted team id %s, got %s", team.ID, resp.TeamID)
	}

	remaining, err := store.GetTenant(ctx, team.ID)
	if err != nil {
		t.Fatalf("get tenant after purge: %v", err)
	}
	if remaining != nil {
		t.Fatalf("expected tenant to be deleted, got %+v", remaining)
	}

	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("expected tenant db to be removed, stat err=%v", err)
	}
}

func TestHandleDeleteTeamRequiresArchiveFirst(t *testing.T) {
	h, store, _, _, actorUserID, _ := setupAdminTestEnv(t)
	ctx := context.Background()

	team, err := store.CreateTenant(ctx, actorUserID, "Active Team", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodDelete, "/api/admin/teams/"+team.ID.String(), nil), actorUserID)
	req.SetPathValue("id", team.ID.String())
	w := httptest.NewRecorder()

	h.handleAdminDeleteTeam().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestHandleAdminArchiveTeamRejectsHostedCloudWithoutDefaultFallback(t *testing.T) {
	h, store, _, _, actorUserID, _ := setupAdminTestEnv(t)
	h.ctx.Config.CloudHosted = true
	ctx := context.Background()

	customerID, err := store.CreateUserWithoutDefaultTenant(ctx, "cloud-customer@example.com", "hash")
	if err != nil {
		t.Fatalf("create cloud customer: %v", err)
	}
	team, err := store.CreateTenant(ctx, customerID, "Customer Cloud Team", "")
	if err != nil {
		t.Fatalf("create customer team: %v", err)
	}
	defaultTenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/teams/"+team.ID.String()+"/archive", nil), actorUserID)
	req.SetPathValue("id", team.ID.String())
	w := httptest.NewRecorder()

	h.handleAdminArchiveTeam().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}

	isDefaultMember, err := store.IsTenantMember(ctx, defaultTenantID, customerID)
	if err != nil {
		t.Fatalf("check default membership: %v", err)
	}
	if isDefaultMember {
		t.Fatal("expected rejected cloud archive not to add customer to default team")
	}
}

func TestHandleAdminForceDeleteTeamRejectsHostedCloudWithoutDefaultFallback(t *testing.T) {
	h, store, _, _, actorUserID, _ := setupAdminTestEnv(t)
	h.ctx.Config.CloudHosted = true
	ctx := context.Background()

	customerID, err := store.CreateUserWithoutDefaultTenant(ctx, "cloud-force-delete@example.com", "hash")
	if err != nil {
		t.Fatalf("create cloud customer: %v", err)
	}
	team, err := store.CreateTenant(ctx, customerID, "Force Delete Cloud Team", "")
	if err != nil {
		t.Fatalf("create customer team: %v", err)
	}
	defaultTenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodDelete, "/api/admin/teams/"+team.ID.String()+"?force=true", nil), actorUserID)
	req.SetPathValue("id", team.ID.String())
	w := httptest.NewRecorder()

	h.handleAdminDeleteTeam().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}

	isDefaultMember, err := store.IsTenantMember(ctx, defaultTenantID, customerID)
	if err != nil {
		t.Fatalf("check default membership: %v", err)
	}
	if isDefaultMember {
		t.Fatal("expected rejected cloud force delete not to add customer to default team")
	}
}

func TestHandleAdminForceDeletePurgesAlreadyArchivedHostedCloudTeam(t *testing.T) {
	h, store, _, _, actorUserID, _ := setupAdminTestEnv(t)
	h.ctx.Config.CloudHosted = true
	ctx := context.Background()

	customerID, err := store.CreateUserWithoutDefaultTenant(ctx, "archived-cloud-customer@example.com", "hash")
	if err != nil {
		t.Fatalf("create cloud customer: %v", err)
	}
	team, err := store.CreateTenant(ctx, customerID, "Already Archived Cloud Team", "")
	if err != nil {
		t.Fatalf("create customer team: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, "INSERT INTO tenant_archives (tenant_id, archived_at, archived_by) VALUES (?, ?, ?)", team.ID, time.Now().UTC(), actorUserID); err != nil {
		t.Fatalf("archive team directly: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodDelete, "/api/admin/teams/"+team.ID.String()+"?force=true", nil), actorUserID)
	req.SetPathValue("id", team.ID.String())
	w := httptest.NewRecorder()

	h.handleAdminDeleteTeam().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	deleted, err := store.GetTenant(ctx, team.ID)
	if err != nil {
		t.Fatalf("get deleted team: %v", err)
	}
	if deleted != nil {
		t.Fatalf("expected archived hosted cloud team to be purged, got %+v", deleted)
	}
}

func TestHandleAddSiteMemberUsesInviterLocaleForNewUserInvite(t *testing.T) {
	h, store, _, _, actorUserID, _ := setupAdminTestEnv(t)
	ctx := context.Background()

	if err := store.UpsertUserPreferences(ctx, actorUserID, api.UserPreferences{DefaultLocale: "de"}); err != nil {
		t.Fatalf("set actor locale: %v", err)
	}

	site, err := store.CreateSite(ctx, actorUserID, "beispiel.de")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	drv := &adminTestMailDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(drv, h.ctx.Config)

	body := strings.NewReader(`{"email":"new-user@example.com","role":"viewer"}`)
	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/sites/"+site.ID.String()+"/members", body), actorUserID)
	req.SetPathValue("id", site.ID.String())
	w := httptest.NewRecorder()

	h.handleAddSiteMember().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	if !strings.Contains(drv.subject, "Du wurdest eingeladen") {
		t.Fatalf("expected localized German subject, got %q", drv.subject)
	}
	if !strings.Contains(drv.textBody, "Du wurdest eingeladen!") {
		t.Fatalf("expected localized German text body, got:\n%s", drv.textBody)
	}
	if !strings.Contains(drv.textBody, "Einladung annehmen") {
		t.Fatalf("expected localized German CTA, got:\n%s", drv.textBody)
	}
}

func TestHandleAddSiteMemberInHostedCloudRejectsNewUserInsteadOfJoiningDefaultTeam(t *testing.T) {
	h, store, _, _, actorUserID, _ := setupAdminTestEnv(t)
	h.ctx.Config.CloudHosted = true
	ctx := context.Background()

	site, err := store.CreateSite(ctx, actorUserID, "cloud-admin-invite.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	drv := &adminTestMailDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(drv, h.ctx.Config)

	body := strings.NewReader(`{"email":"cloud-site-member@example.com","role":"viewer"}`)
	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/sites/"+site.ID.String()+"/members", body), actorUserID)
	req.SetPathValue("id", site.ID.String())
	w := httptest.NewRecorder()

	h.handleAddSiteMember().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, w.Code, w.Body.String())
	}

	user, err := store.GetUserByEmail(ctx, "cloud-site-member@example.com")
	if err != nil {
		t.Fatalf("get rejected user: %v", err)
	}
	if user != nil {
		t.Fatalf("expected hosted-cloud site member path not to create a user, got %+v", user)
	}
}

func TestHandleSiteMemberPermissionMutationsAppendCentralAudit(t *testing.T) {
	h, store, _, _, actorUserID, targetUserID := setupAdminTestEnv(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, actorUserID, "audit-permissions.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	teamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get site tenant: %v", err)
	}

	addBody := strings.NewReader(`{"email":"target-owner@example.com","role":"viewer"}`)
	addReq := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/sites/"+site.ID.String()+"/members", addBody), actorUserID)
	addReq.RemoteAddr = "203.0.113.10:1234"
	addReq.Header.Set("User-Agent", "audit-permission-test")
	addReq.Header.Set("X-Request-Id", "req-permission-grant")
	addReq.SetPathValue("id", site.ID.String())
	addW := httptest.NewRecorder()

	h.handleAddSiteMember().ServeHTTP(addW, addReq)
	if addW.Code != http.StatusOK {
		t.Fatalf("expected add status %d, got %d: %s", http.StatusOK, addW.Code, addW.Body.String())
	}
	assertPermissionAuditEntry(t, store, "permission.site_member_granted", teamID, targetUserID, site.ID, "203.0.113.10", "audit-permission-test", "req-permission-grant")

	updateBody := strings.NewReader(`{"email":"target-owner@example.com","role":"admin"}`)
	updateReq := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/sites/"+site.ID.String()+"/members", updateBody), actorUserID)
	updateReq.SetPathValue("id", site.ID.String())
	updateW := httptest.NewRecorder()

	h.handleAddSiteMember().ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Fatalf("expected update status %d, got %d: %s", http.StatusOK, updateW.Code, updateW.Body.String())
	}
	assertPermissionAuditEntry(t, store, "permission.site_member_role_updated", teamID, targetUserID, site.ID, "", "", "")

	removeReq := withAdminTestUser(httptest.NewRequest(http.MethodDelete, "/api/admin/sites/"+site.ID.String()+"/members/"+targetUserID.String(), nil), actorUserID)
	removeReq.SetPathValue("id", site.ID.String())
	removeReq.SetPathValue("userId", targetUserID.String())
	removeW := httptest.NewRecorder()

	h.handleRemoveSiteMember().ServeHTTP(removeW, removeReq)
	if removeW.Code != http.StatusOK {
		t.Fatalf("expected remove status %d, got %d: %s", http.StatusOK, removeW.Code, removeW.Body.String())
	}
	assertPermissionAuditEntry(t, store, "permission.site_member_revoked", teamID, targetUserID, site.ID, "", "", "")
}

func assertPermissionAuditEntry(t *testing.T, store *database.Store, action string, teamID, targetUserID, siteID uuid.UUID, expectedIP, expectedUserAgent, expectedRequestID string) {
	t.Helper()

	entries, total, err := store.ListInstanceAuditEntries(context.Background(), database.InstanceAuditFilter{
		Action: action,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("list audit entries for %s: %v", action, err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one %s audit entry, total=%d len=%d", action, total, len(entries))
	}

	entry := entries[0]
	if entry.TeamID == nil || *entry.TeamID != teamID {
		t.Fatalf("expected team_id %s, got %v", teamID, entry.TeamID)
	}
	if entry.TargetUserID == nil || *entry.TargetUserID != targetUserID {
		t.Fatalf("expected target_user_id %s, got %v", targetUserID, entry.TargetUserID)
	}
	if entry.TargetType != "permission" || entry.TargetID != siteID.String() || entry.TargetLabel != "audit-permissions.example" {
		t.Fatalf("expected permission target fields, got type=%q id=%q label=%q", entry.TargetType, entry.TargetID, entry.TargetLabel)
	}
	if expectedIP != "" && entry.IPAddress != expectedIP {
		t.Fatalf("expected IP %q, got %q", expectedIP, entry.IPAddress)
	}
	if expectedUserAgent != "" && entry.UserAgent != expectedUserAgent {
		t.Fatalf("expected user agent %q, got %q", expectedUserAgent, entry.UserAgent)
	}
	if expectedRequestID != "" && entry.RequestID != expectedRequestID {
		t.Fatalf("expected request ID %q, got %q", expectedRequestID, entry.RequestID)
	}
}

func assertGlobalExclusionAuditEntry(t *testing.T, store *database.Store, action string, targetID string, targetLabel string, expectedIP string, expectedUserAgent string, expectedRequestID string) {
	t.Helper()

	entries, total, err := store.ListInstanceAuditEntries(context.Background(), database.InstanceAuditFilter{
		Action: action,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("list audit entries for %s: %v", action, err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one %s audit entry, total=%d len=%d", action, total, len(entries))
	}

	entry := entries[0]
	if entry.TargetType != "site_exclusion" || entry.TargetID != targetID || entry.TargetLabel != targetLabel {
		t.Fatalf("expected exclusion target fields, got type=%q id=%q label=%q", entry.TargetType, entry.TargetID, entry.TargetLabel)
	}
	if expectedIP != "" && entry.IPAddress != expectedIP {
		t.Fatalf("expected IP %q, got %q", expectedIP, entry.IPAddress)
	}
	if expectedUserAgent != "" && entry.UserAgent != expectedUserAgent {
		t.Fatalf("expected user agent %q, got %q", expectedUserAgent, entry.UserAgent)
	}
	if expectedRequestID != "" && entry.RequestID != expectedRequestID {
		t.Fatalf("expected request ID %q, got %q", expectedRequestID, entry.RequestID)
	}
}
