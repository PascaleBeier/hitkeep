package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
)

func setupTenantStore(t *testing.T) *Store {
	t.Helper()

	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	return store
}

func TestCreateUserAddsDefaultTenantMembership(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	defaultTenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	ownerID, err := store.CreateUser(ctx, "owner@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	memberID, err := store.CreateUser(ctx, "member@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create member user: %v", err)
	}

	var ownerRole string
	if err := store.DB().QueryRowContext(ctx,
		"SELECT role FROM tenant_members WHERE tenant_id = ? AND user_id = ?",
		defaultTenantID, ownerID,
	).Scan(&ownerRole); err != nil {
		t.Fatalf("query owner tenant role: %v", err)
	}
	if ownerRole != TenantRoleOwner {
		t.Fatalf("expected owner role %q, got %q", TenantRoleOwner, ownerRole)
	}

	var memberRole string
	if err := store.DB().QueryRowContext(ctx,
		"SELECT role FROM tenant_members WHERE tenant_id = ? AND user_id = ?",
		defaultTenantID, memberID,
	).Scan(&memberRole); err != nil {
		t.Fatalf("query member tenant role: %v", err)
	}
	if memberRole != TenantRoleMember {
		t.Fatalf("expected member role %q, got %q", TenantRoleMember, memberRole)
	}

	var isDefault bool
	if err := store.DB().QueryRowContext(ctx, "SELECT is_default FROM tenants WHERE id = ?", defaultTenantID).Scan(&isDefault); err != nil {
		t.Fatalf("query default tenant: %v", err)
	}
	if !isDefault {
		t.Fatal("expected default tenant is_default = true")
	}
}

func TestGetPrimaryTenantIDFallsBackToDefault(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	defaultTenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	userID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO users (id, email, password, created_at) VALUES (?, ?, ?, ?)",
		userID, "nomembership@tenant.test", "hash", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	tenantID, err := store.GetPrimaryTenantID(ctx, userID)
	if err != nil {
		t.Fatalf("get primary tenant: %v", err)
	}
	if tenantID != defaultTenantID {
		t.Fatalf("expected default tenant %s, got %s", defaultTenantID, tenantID)
	}
}

func TestAddSiteMemberRequiresTenantMembership(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "owner@cross-tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	outsiderID, err := store.CreateUser(ctx, "outsider@cross-tenant.test", "hash")
	if err != nil {
		t.Fatalf("create outsider user: %v", err)
	}

	tenantID := uuid.New()
	now := time.Now().UTC()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		tenantID, "Custom Tenant", now,
	); err != nil {
		t.Fatalf("create custom tenant: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenant_members (id, tenant_id, user_id, role, added_at, added_by) VALUES (?, ?, ?, ?, ?, ?)",
		uuid.New(), tenantID, ownerID, TenantRoleOwner, now, ownerID,
	); err != nil {
		t.Fatalf("add owner to custom tenant: %v", err)
	}

	siteID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO sites (id, user_id, domain, created_at) VALUES (?, ?, ?, ?)",
		siteID, ownerID, "cross-tenant.test", now,
	); err != nil {
		t.Fatalf("create tenant site: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO site_tenants (site_id, tenant_id, created_at) VALUES (?, ?, ?)",
		siteID, tenantID, now,
	); err != nil {
		t.Fatalf("create site tenant mapping: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO site_members (id, site_id, user_id, role, added_at, added_by) VALUES (?, ?, ?, ?, ?, ?)",
		uuid.New(), siteID, ownerID, auth.SiteOwner, now, ownerID,
	); err != nil {
		t.Fatalf("create owner site membership: %v", err)
	}

	err = store.AddSiteMember(ctx, siteID, outsiderID, auth.SiteViewer, ownerID)
	if err == nil {
		t.Fatalf("expected tenant membership error")
	}
	if !strings.Contains(err.Error(), "not part of tenant") {
		t.Fatalf("expected tenant membership error, got %v", err)
	}

	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenant_members (id, tenant_id, user_id, role, added_at, added_by) VALUES (?, ?, ?, ?, ?, ?)",
		uuid.New(), tenantID, outsiderID, TenantRoleMember, now, ownerID,
	); err != nil {
		t.Fatalf("add outsider to custom tenant: %v", err)
	}

	if err := store.AddSiteMember(ctx, siteID, outsiderID, auth.SiteViewer, ownerID); err != nil {
		t.Fatalf("add site member after tenant membership: %v", err)
	}
}

func TestSetActiveTenantIDAndScopedSites(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "owner@active-team.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	defaultTenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	defaultSite, err := store.CreateSite(ctx, userID, "default-team.test")
	if err != nil {
		t.Fatalf("create default team site: %v", err)
	}

	customTenantID := uuid.New()
	now := time.Now().UTC()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		customTenantID, "Custom Team", now,
	); err != nil {
		t.Fatalf("create custom tenant: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenant_members (tenant_id, user_id, role, added_by) VALUES (?, ?, ?, ?)",
		customTenantID, userID, TenantRoleAdmin, userID,
	); err != nil {
		t.Fatalf("add user to custom tenant: %v", err)
	}

	otherTenantID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		otherTenantID, "Other Team", now,
	); err != nil {
		t.Fatalf("create other tenant: %v", err)
	}

	if err := store.SetActiveTenantID(ctx, userID, otherTenantID); err == nil {
		t.Fatalf("expected non-member active team assignment to fail")
	}

	if err := store.SetActiveTenantID(ctx, userID, customTenantID); err != nil {
		t.Fatalf("set custom active tenant: %v", err)
	}

	activeTenantID, err := store.GetActiveTenantID(ctx, userID)
	if err != nil {
		t.Fatalf("get active tenant: %v", err)
	}
	if activeTenantID != customTenantID {
		t.Fatalf("expected active tenant %s, got %s", customTenantID, activeTenantID)
	}

	customSite, err := store.CreateSite(ctx, userID, "custom-team.test")
	if err != nil {
		t.Fatalf("create custom team site: %v", err)
	}

	sites, err := store.GetSites(ctx, userID)
	if err != nil {
		t.Fatalf("get scoped sites: %v", err)
	}
	if len(sites) != 1 || sites[0].ID != customSite.ID {
		t.Fatalf("expected only custom team site %s, got %+v", customSite.ID, sites)
	}

	if err := store.SetActiveTenantID(ctx, userID, defaultTenantID); err != nil {
		t.Fatalf("set default active tenant: %v", err)
	}
	sites, err = store.GetSites(ctx, userID)
	if err != nil {
		t.Fatalf("get default scoped sites: %v", err)
	}
	if len(sites) != 1 || sites[0].ID != defaultSite.ID {
		t.Fatalf("expected only default team site %s, got %+v", defaultSite.ID, sites)
	}

	_, activeTeamID, err := store.ListUserTeams(ctx, userID)
	if err != nil {
		t.Fatalf("list user teams: %v", err)
	}
	if activeTeamID != defaultTenantID {
		t.Fatalf("expected active team %s, got %s", defaultTenantID, activeTeamID)
	}
}

func TestTeamAdminsGetEffectiveSiteAccessForActiveTeam(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "team-site-owner@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	adminID, err := store.CreateUser(ctx, "team-site-admin@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	memberID, err := store.CreateUser(ctx, "team-site-member@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}

	team, err := store.CreateTenant(ctx, ownerID, "Managed Sites", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := store.AddTeamMember(ctx, team.ID, adminID, TenantRoleAdmin, ownerID); err != nil {
		t.Fatalf("add admin: %v", err)
	}
	if err := store.AddTeamMember(ctx, team.ID, memberID, TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, ownerID, team.ID); err != nil {
		t.Fatalf("set owner active team: %v", err)
	}

	firstSite, err := store.CreateSite(ctx, ownerID, "team-admin-first.test")
	if err != nil {
		t.Fatalf("create first site: %v", err)
	}
	secondSite, err := store.CreateSite(ctx, ownerID, "team-admin-second.test")
	if err != nil {
		t.Fatalf("create second site: %v", err)
	}
	if err := store.AddSiteMember(ctx, firstSite.ID, memberID, auth.SiteViewer, ownerID); err != nil {
		t.Fatalf("add explicit member site access: %v", err)
	}

	if err := store.SetActiveTenantID(ctx, adminID, team.ID); err != nil {
		t.Fatalf("set admin active team: %v", err)
	}
	adminRole, err := store.GetSiteRole(ctx, adminID, firstSite.ID)
	if err != nil {
		t.Fatalf("get admin site role: %v", err)
	}
	if adminRole != auth.SiteAdmin {
		t.Fatalf("expected team admin to resolve as site admin, got %q", adminRole)
	}

	adminSites, err := store.GetSites(ctx, adminID)
	if err != nil {
		t.Fatalf("get admin sites: %v", err)
	}
	if len(adminSites) != 2 {
		t.Fatalf("expected team admin to see both team sites, got %+v", adminSites)
	}

	if err := store.SetActiveTenantID(ctx, memberID, team.ID); err != nil {
		t.Fatalf("set member active team: %v", err)
	}
	memberSites, err := store.GetSites(ctx, memberID)
	if err != nil {
		t.Fatalf("get member sites: %v", err)
	}
	if len(memberSites) != 1 || memberSites[0].ID != firstSite.ID {
		t.Fatalf("expected team member to see only explicitly assigned site %s, got %+v", firstSite.ID, memberSites)
	}
	if _, err := store.GetSiteRole(ctx, memberID, secondSite.ID); err == nil {
		t.Fatal("expected team member without site role to be denied")
	}
}

func TestArchiveTenantHidesArchivedTeamAndResetsActiveTeam(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "archive-owner@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	team, err := store.CreateTenant(ctx, ownerID, "Archive Ready", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, ownerID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}

	if err := store.ArchiveTenant(ctx, team.ID, ownerID); err != nil {
		t.Fatalf("archive tenant: %v", err)
	}

	teams, activeTenantID, err := store.ListUserTeams(ctx, ownerID)
	if err != nil {
		t.Fatalf("list user teams: %v", err)
	}
	for _, listedTeam := range teams {
		if listedTeam.ID == team.ID {
			t.Fatalf("expected archived team %s to be omitted from team list", team.ID)
		}
	}

	defaultTenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	if activeTenantID != defaultTenantID {
		t.Fatalf("expected active tenant to fall back to default %s, got %s", defaultTenantID, activeTenantID)
	}

	var archivedAt time.Time
	if err := store.DB().QueryRowContext(ctx, "SELECT archived_at FROM tenant_archives WHERE tenant_id = ?", team.ID).Scan(&archivedAt); err != nil {
		t.Fatalf("query tenant_archives: %v", err)
	}
	if archivedAt.IsZero() {
		t.Fatalf("expected archived_at to be set")
	}
}

func TestArchiveTenantFailsWhenTeamStillOwnsSites(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "archive-sites-owner@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	team, err := store.CreateTenant(ctx, ownerID, "Archive Blocked", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, ownerID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	if _, err := store.CreateSite(ctx, ownerID, "archive-blocked.test"); err != nil {
		t.Fatalf("create site: %v", err)
	}

	err = store.ArchiveTenant(ctx, team.ID, ownerID)
	if !errors.Is(err, ErrTeamArchiveHasSites) {
		t.Fatalf("expected ErrTeamArchiveHasSites, got %v", err)
	}
}

func TestDeleteArchivedTenantMetadataRemovesArchivedTeamRows(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "purge-owner@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	team, err := store.CreateTenant(ctx, ownerID, "Purge Ready", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := store.ArchiveTenant(ctx, team.ID, ownerID); err != nil {
		t.Fatalf("archive tenant: %v", err)
	}

	deleted, err := store.DeleteArchivedTenantMetadata(ctx, team.ID)
	if err != nil {
		t.Fatalf("delete archived tenant metadata: %v", err)
	}
	if deleted == nil || deleted.ID != team.ID {
		t.Fatalf("expected deleted team %s, got %+v", team.ID, deleted)
	}

	remaining, err := store.GetTenant(ctx, team.ID)
	if err != nil {
		t.Fatalf("get tenant after delete: %v", err)
	}
	if remaining != nil {
		t.Fatalf("expected tenant metadata to be deleted, got %+v", remaining)
	}
}

func TestDeleteArchivedTenantMetadataRemovesCloudAndAPIClientRows(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "purge-cloud-owner@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	team, err := store.CreateTenant(ctx, ownerID, "Purge Cloud Rows", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	now := time.Now().UTC()
	clientID := uuid.New()
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO api_clients (id, tenant_id, name, secret_hash, instance_role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, clientID, team.ID, "Team client", "purge-cloud-secret", "user", now, now); err != nil {
		t.Fatalf("insert team api client: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO cloud_billing_accounts (
			tenant_id, plan_code, plan_name, subscription_status, stripe_customer_id, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, team.ID, "pro", "Pro", "active", "cus_purge_cloud", now, now); err != nil {
		t.Fatalf("insert cloud billing account: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO cloud_billing_events (
			stripe_event_id, tenant_id, event_type, livemode, processing_status, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "evt_purge_cloud", team.ID, "customer.subscription.deleted", false, "processed", now, now); err != nil {
		t.Fatalf("insert cloud billing event: %v", err)
	}

	if err := store.ArchiveTenant(ctx, team.ID, ownerID); err != nil {
		t.Fatalf("archive tenant: %v", err)
	}

	deleted, err := store.DeleteArchivedTenantMetadata(ctx, team.ID)
	if err != nil {
		t.Fatalf("delete archived tenant metadata: %v", err)
	}
	if deleted == nil || deleted.ID != team.ID {
		t.Fatalf("expected deleted team %s, got %+v", team.ID, deleted)
	}

	assertTenantPurgeCount := func(table string) {
		t.Helper()
		var count int
		if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE tenant_id = ?", team.ID).Scan(&count); err != nil {
			t.Fatalf("count %s rows: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("expected %s tenant rows to be purged, got %d", table, count)
		}
	}
	assertTenantPurgeCount("api_clients")
	assertTenantPurgeCount("cloud_billing_accounts")
	assertTenantPurgeCount("cloud_billing_events")
}

func TestDeleteArchivedTenantMetadataRequiresArchivedTeam(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "purge-active-owner@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	team, err := store.CreateTenant(ctx, ownerID, "Purge Blocked", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	_, err = store.DeleteArchivedTenantMetadata(ctx, team.ID)
	if !errors.Is(err, ErrTeamPurgeNotArchived) {
		t.Fatalf("expected ErrTeamPurgeNotArchived, got %v", err)
	}
}

func TestLeaveTeamReassignsActiveTenant(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "leave-team@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	customTenantID := uuid.New()
	now := time.Now().UTC()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		customTenantID, "Leave Team", now,
	); err != nil {
		t.Fatalf("create custom tenant: %v", err)
	}
	if err := store.AddTeamMember(ctx, customTenantID, userID, TenantRoleAdmin, userID); err != nil {
		t.Fatalf("add user to custom tenant: %v", err)
	}

	if err := store.SetActiveTenantID(ctx, userID, customTenantID); err != nil {
		t.Fatalf("set active custom tenant: %v", err)
	}

	site, err := store.CreateSite(ctx, userID, "leave-team-site.test")
	if err != nil {
		t.Fatalf("create custom tenant site: %v", err)
	}
	if err := store.UpsertSiteReportSubscription(ctx, userID, site.ID, api.ReportFrequencyDaily, true); err != nil {
		t.Fatalf("upsert site report subscription: %v", err)
	}

	defaultTenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	nextActiveTenantID, err := store.LeaveTeam(ctx, customTenantID, userID)
	if err != nil {
		t.Fatalf("leave team: %v", err)
	}
	if nextActiveTenantID != defaultTenantID {
		t.Fatalf("expected next active tenant %s, got %s", defaultTenantID, nextActiveTenantID)
	}

	isMember, err := store.IsTenantMember(ctx, customTenantID, userID)
	if err != nil {
		t.Fatalf("check custom tenant membership: %v", err)
	}
	if isMember {
		t.Fatalf("expected user to be removed from custom tenant")
	}

	var siteMemberCount int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM site_members WHERE site_id = ? AND user_id = ?",
		site.ID, userID,
	).Scan(&siteMemberCount); err != nil {
		t.Fatalf("count remaining site memberships: %v", err)
	}
	if siteMemberCount != 0 {
		t.Fatalf("expected tenant-scoped site memberships to be removed, got %d", siteMemberCount)
	}

	var siteSubCount int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM site_report_subscriptions WHERE site_id = ? AND user_id = ?",
		site.ID, userID,
	).Scan(&siteSubCount); err != nil {
		t.Fatalf("count remaining site report subscriptions: %v", err)
	}
	if siteSubCount != 0 {
		t.Fatalf("expected tenant-scoped report subscriptions to be removed, got %d", siteSubCount)
	}
}

func TestRemoveTeamMemberRemovesTenantScopedSiteAccessAndSubscriptions(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "remove-team-owner@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	memberID, err := store.CreateUser(ctx, "remove-team-member@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create member user: %v", err)
	}

	team, err := store.CreateTenant(ctx, ownerID, "Cleanup Team", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := store.AddTeamMember(ctx, team.ID, memberID, TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add member to team: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, ownerID, team.ID); err != nil {
		t.Fatalf("set active team: %v", err)
	}

	site, err := store.CreateSite(ctx, ownerID, "cleanup-team-site.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.AddSiteMember(ctx, site.ID, memberID, auth.SiteViewer, ownerID); err != nil {
		t.Fatalf("add site member: %v", err)
	}
	if err := store.UpsertSiteReportSubscription(ctx, memberID, site.ID, api.ReportFrequencyWeekly, true); err != nil {
		t.Fatalf("upsert site report subscription: %v", err)
	}

	if err := store.RemoveTeamMember(ctx, team.ID, memberID); err != nil {
		t.Fatalf("remove team member: %v", err)
	}

	var siteMemberCount int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM site_members WHERE site_id = ? AND user_id = ?",
		site.ID, memberID,
	).Scan(&siteMemberCount); err != nil {
		t.Fatalf("count remaining site memberships: %v", err)
	}
	if siteMemberCount != 0 {
		t.Fatalf("expected tenant-scoped site memberships to be removed, got %d", siteMemberCount)
	}

	var siteSubCount int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM site_report_subscriptions WHERE site_id = ? AND user_id = ?",
		site.ID, memberID,
	).Scan(&siteSubCount); err != nil {
		t.Fatalf("count remaining site report subscriptions: %v", err)
	}
	if siteSubCount != 0 {
		t.Fatalf("expected tenant-scoped report subscriptions to be removed, got %d", siteSubCount)
	}
}

func TestAppendAndListTeamAuditEntries(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	actorID, err := store.CreateUser(ctx, "audit-actor@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create actor user: %v", err)
	}
	targetID, err := store.CreateUser(ctx, "audit-target@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create target user: %v", err)
	}

	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	if err := store.AppendTeamAuditEntry(ctx, tenantID, actorID, "member.added", "Added target user", &targetID); err != nil {
		t.Fatalf("append team audit entry: %v", err)
	}

	entries, total, err := store.ListTeamAuditEntries(ctx, tenantID, "", 20, 0)
	if err != nil {
		t.Fatalf("list team audit entries: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected at least one team audit entry")
	}
	if total == 0 {
		t.Fatalf("expected audit total to be populated")
	}

	entry := entries[0]
	if entry.Action != "member.added" {
		t.Fatalf("expected action %q, got %q", "member.added", entry.Action)
	}
	if entry.TeamID != tenantID {
		t.Fatalf("expected team_id %s, got %s", tenantID, entry.TeamID)
	}
	if entry.ActorUserID == nil || *entry.ActorUserID != actorID {
		t.Fatalf("expected actor %s, got %v", actorID, entry.ActorUserID)
	}
	if entry.TargetUserID == nil || *entry.TargetUserID != targetID {
		t.Fatalf("expected target %s, got %v", targetID, entry.TargetUserID)
	}

	var centralCount int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM instance_audit_log WHERE team_id = ? AND action = ?",
		tenantID, "member.added",
	).Scan(&centralCount); err != nil {
		t.Fatalf("count central audit rows: %v", err)
	}
	if centralCount != 1 {
		t.Fatalf("expected team audit wrapper to write one central audit row, got %d", centralCount)
	}
}

func TestAppendCentralAuditPersistsNetworkContext(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	actorID, err := store.CreateUser(ctx, "central-audit-actor@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create actor user: %v", err)
	}
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	if err := store.AppendAuditEntry(ctx, AuditEntryParams{
		ActorID:       actorID,
		TeamID:        tenantID,
		Action:        "team.updated",
		TargetType:    "team",
		TargetID:      tenantID.String(),
		TargetLabel:   "Default Tenant",
		Outcome:       "success",
		IPAddress:     "203.0.113.10",
		IPCountryCode: "de",
		UserAgent:     "audit-test",
		RequestID:     "req-audit-test",
		Details:       "Updated settings",
	}); err != nil {
		t.Fatalf("append central audit entry: %v", err)
	}

	entries, total, err := store.ListInstanceAuditEntries(ctx, InstanceAuditFilter{Action: "team.updated", Limit: 10})
	if err != nil {
		t.Fatalf("list instance audit entries: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one audit entry, total=%d len=%d", total, len(entries))
	}
	entry := entries[0]
	if entry.IPAddress != "203.0.113.10" {
		t.Fatalf("expected IP address to persist, got %q", entry.IPAddress)
	}
	if entry.IPCountryCode != "DE" {
		t.Fatalf("expected country code DE, got %q", entry.IPCountryCode)
	}
	if entry.RequestID != "req-audit-test" {
		t.Fatalf("expected request ID to persist, got %q", entry.RequestID)
	}
}

func TestListTeamAuditEntriesFilteredReturnsCentralEvidence(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	actorID, err := store.CreateUser(ctx, "audit-filter-actor@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create actor user: %v", err)
	}
	targetID, err := store.CreateUser(ctx, "audit-filter-target@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create target user: %v", err)
	}
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	if err := store.AppendAuditEntry(ctx, AuditEntryParams{
		ActorID:       actorID,
		ActorEmail:    "audit-filter-actor@tenant.test",
		ActorRole:     string(TenantRoleOwner),
		TeamID:        tenantID,
		TargetUserID:  targetID,
		Action:        "permission.site_member_granted",
		TargetType:    "permission",
		TargetID:      "site-1",
		TargetLabel:   "example.com",
		Outcome:       "success",
		IPAddress:     "203.0.113.99",
		IPCountryCode: "us",
		UserAgent:     "audit-filter-agent",
		RequestID:     "req-team-filter",
		Details:       "Granted site access",
	}); err != nil {
		t.Fatalf("append matching audit entry: %v", err)
	}
	if err := store.AppendAuditEntry(ctx, AuditEntryParams{
		ActorID:    actorID,
		TeamID:     tenantID,
		Action:     "permission.site_member_revoked",
		TargetType: "permission",
		TargetID:   "site-1",
		Outcome:    "success",
		Details:    "Revoked site access",
	}); err != nil {
		t.Fatalf("append non-matching audit entry: %v", err)
	}

	from := time.Now().Add(-time.Minute)
	to := time.Now().Add(time.Minute)
	entries, total, err := store.ListTeamAuditEntriesFiltered(ctx, tenantID, TeamAuditFilter{
		Action:     "permission.site_member_granted",
		Outcome:    "success",
		TargetType: "permission",
		Query:      "203.0.113.99",
		From:       from,
		To:         to,
		Limit:      10,
		Offset:     0,
	})
	if err != nil {
		t.Fatalf("list filtered team audit entries: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one filtered audit entry, total=%d len=%d", total, len(entries))
	}

	entry := entries[0]
	if entry.ActorEmailSnapshot != "audit-filter-actor@tenant.test" {
		t.Fatalf("expected actor email snapshot, got %q", entry.ActorEmailSnapshot)
	}
	if entry.ActorRoleSnapshot != string(TenantRoleOwner) {
		t.Fatalf("expected actor role snapshot, got %q", entry.ActorRoleSnapshot)
	}
	if entry.TargetType != "permission" || entry.TargetID != "site-1" || entry.TargetLabel != "example.com" {
		t.Fatalf("expected permission target fields, got type=%q id=%q label=%q", entry.TargetType, entry.TargetID, entry.TargetLabel)
	}
	if entry.TargetUserID == nil || *entry.TargetUserID != targetID {
		t.Fatalf("expected target user %s, got %v", targetID, entry.TargetUserID)
	}
	if entry.Outcome != "success" {
		t.Fatalf("expected success outcome, got %q", entry.Outcome)
	}
	if entry.IPAddress != "203.0.113.99" || entry.IPCountryCode != "US" {
		t.Fatalf("expected network evidence, got ip=%q country=%q", entry.IPAddress, entry.IPCountryCode)
	}
	if entry.UserAgent != "audit-filter-agent" || entry.RequestID != "req-team-filter" {
		t.Fatalf("expected request evidence, got user_agent=%q request_id=%q", entry.UserAgent, entry.RequestID)
	}
}

func TestCreateAndListTeamInvites(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "owner-invite@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	teamID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	invite, err := store.CreateTeamInvite(ctx, teamID, "invitee@tenant.test", TenantRoleAdmin, nil, ownerID)
	if err != nil {
		t.Fatalf("create team invite: %v", err)
	}
	if invite.Status != TeamInviteStatusPending {
		t.Fatalf("expected pending invite status, got %q", invite.Status)
	}

	invites, err := store.ListTeamInvites(ctx, teamID)
	if err != nil {
		t.Fatalf("list team invites: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("expected 1 team invite, got %d", len(invites))
	}
	if invites[0].Email != "invitee@tenant.test" {
		t.Fatalf("expected invite email %q, got %q", "invitee@tenant.test", invites[0].Email)
	}
}

func TestCreateTeamInviteRejectsDuplicatePendingInvite(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "owner-dup@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	teamID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	if _, err := store.CreateTeamInvite(ctx, teamID, "dup@tenant.test", TenantRoleMember, nil, ownerID); err != nil {
		t.Fatalf("create first team invite: %v", err)
	}
	if _, err := store.CreateTeamInvite(ctx, teamID, "dup@tenant.test", TenantRoleMember, nil, ownerID); !errors.Is(err, ErrTeamInviteAlreadyPending) {
		t.Fatalf("expected ErrTeamInviteAlreadyPending, got %v", err)
	}
}

func TestRevokeTeamInviteRemovesPendingInvite(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "owner-revoke@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	teamID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	invite, err := store.CreateTeamInvite(ctx, teamID, "revoke@tenant.test", TenantRoleMember, nil, ownerID)
	if err != nil {
		t.Fatalf("create team invite: %v", err)
	}
	if err := store.RevokeTeamInvite(ctx, teamID, invite.ID); err != nil {
		t.Fatalf("revoke team invite: %v", err)
	}

	invites, err := store.ListTeamInvites(ctx, teamID)
	if err != nil {
		t.Fatalf("list team invites: %v", err)
	}
	if len(invites) != 0 {
		t.Fatalf("expected no pending invites after revoke, got %d", len(invites))
	}
}

func TestAcceptTeamInvitesByEmailCreatesMembership(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "owner-accept@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	inviteeID, err := store.CreateUser(ctx, "accept@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create invitee user: %v", err)
	}

	teamID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		teamID, "Invite Accept", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	if _, err := store.CreateTeamInvite(ctx, teamID, "accept@tenant.test", TenantRoleAdmin, &inviteeID, ownerID); err != nil {
		t.Fatalf("create team invite: %v", err)
	}

	accepted, err := store.AcceptTeamInvitesByEmail(ctx, "accept@tenant.test", inviteeID)
	if err != nil {
		t.Fatalf("accept team invites: %v", err)
	}
	if len(accepted) != 1 {
		t.Fatalf("expected 1 accepted invite, got %d", len(accepted))
	}
	if accepted[0].Status != TeamInviteStatusAccepted {
		t.Fatalf("expected accepted invite status, got %q", accepted[0].Status)
	}

	isMember, err := store.IsTenantMember(ctx, teamID, inviteeID)
	if err != nil {
		t.Fatalf("check tenant membership: %v", err)
	}
	if !isMember {
		t.Fatalf("expected invitee to become a team member")
	}

	role, err := store.GetTenantRole(ctx, teamID, inviteeID)
	if err != nil {
		t.Fatalf("get tenant role: %v", err)
	}
	if role != TenantRoleAdmin {
		t.Fatalf("expected invitee role %q, got %q", TenantRoleAdmin, role)
	}
}

func TestTransferTeamOwnership(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "owner-transfer@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	adminID, err := store.CreateUser(ctx, "admin-transfer@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	teamID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		teamID, "Ownership Transfer", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	if err := store.AddTeamMember(ctx, teamID, ownerID, TenantRoleOwner, ownerID); err != nil {
		t.Fatalf("add owner to tenant: %v", err)
	}
	if err := store.AddTeamMember(ctx, teamID, adminID, TenantRoleAdmin, ownerID); err != nil {
		t.Fatalf("add admin to tenant: %v", err)
	}

	if err := store.TransferTeamOwnership(ctx, teamID, ownerID, adminID); err != nil {
		t.Fatalf("transfer team ownership: %v", err)
	}

	ownerRole, err := store.GetTenantRole(ctx, teamID, ownerID)
	if err != nil {
		t.Fatalf("get previous owner role: %v", err)
	}
	if ownerRole != TenantRoleAdmin {
		t.Fatalf("expected previous owner role %q, got %q", TenantRoleAdmin, ownerRole)
	}

	newOwnerRole, err := store.GetTenantRole(ctx, teamID, adminID)
	if err != nil {
		t.Fatalf("get new owner role: %v", err)
	}
	if newOwnerRole != TenantRoleOwner {
		t.Fatalf("expected new owner role %q, got %q", TenantRoleOwner, newOwnerRole)
	}
}

func TestCanAssignTenantRole(t *testing.T) {
	cases := []struct {
		name      string
		actor     string
		requested string
		want      bool
	}{
		{name: "owner_to_owner", actor: TenantRoleOwner, requested: TenantRoleOwner, want: true},
		{name: "owner_to_admin", actor: TenantRoleOwner, requested: TenantRoleAdmin, want: true},
		{name: "owner_to_member", actor: TenantRoleOwner, requested: TenantRoleMember, want: true},
		{name: "admin_to_owner", actor: TenantRoleAdmin, requested: TenantRoleOwner, want: false},
		{name: "admin_to_admin", actor: TenantRoleAdmin, requested: TenantRoleAdmin, want: true},
		{name: "admin_to_member", actor: TenantRoleAdmin, requested: TenantRoleMember, want: true},
		{name: "member_to_admin", actor: TenantRoleMember, requested: TenantRoleAdmin, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CanAssignTenantRole(tc.actor, tc.requested)
			if got != tc.want {
				t.Fatalf("CanAssignTenantRole(%q,%q) = %v, want %v", tc.actor, tc.requested, got, tc.want)
			}
		})
	}
}
