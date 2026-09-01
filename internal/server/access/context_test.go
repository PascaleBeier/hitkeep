package access

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"hitkeep/config"
	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/testutil/testdb"
)

func TestBuilderForUserBuildsDerivedAccessContext(t *testing.T) {
	ctx := context.Background()
	store := newAccessTestStore(t)
	defer store.Close()

	userID, err := store.CreateUser(ctx, "access-owner@example.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "access-owner.example.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	resp, err := (Builder{Store: store}).ForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ForUser: %v", err)
	}

	if resp.InstanceRole != string(auth.InstanceOwner) {
		t.Fatalf("expected instance owner, got %q", resp.InstanceRole)
	}
	if resp.Permissions[site.ID.String()] != string(auth.SiteOwner) {
		t.Fatalf("expected site owner permission, got %+v", resp.Permissions)
	}
	if !hasAccessValue(resp.InstanceCapabilities, string(auth.PermInstanceViewSystem)) {
		t.Fatalf("expected instance capabilities, got %+v", resp.InstanceCapabilities)
	}
	if !hasAccessValue(resp.SiteCapabilities[site.ID.String()], string(auth.PermSiteDelete)) {
		t.Fatalf("expected site delete capability, got %+v", resp.SiteCapabilities)
	}
	if resp.ActiveTeamID == nil || *resp.ActiveTeamID == uuid.Nil || resp.ActiveTeamRole != database.TenantRoleOwner {
		t.Fatalf("expected active owner team context, got %+v", resp)
	}
	if !hasAccessValue(resp.ActiveTeamCapabilities, string(auth.CapTeamManageSettings)) {
		t.Fatalf("expected team settings capability, got %+v", resp.ActiveTeamCapabilities)
	}
}

func TestBuilderForUserSitesAllowsInstanceWideSiteVisibility(t *testing.T) {
	ctx := context.Background()
	store := newAccessTestStore(t)
	defer store.Close()

	ownerID, err := store.CreateUser(ctx, "site-owner@example.test", "hash")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	adminID, err := store.CreateUser(ctx, "instance-admin@example.test", "hash")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := store.UpdateInstanceRole(ctx, adminID, auth.InstanceAdmin, ownerID); err != nil {
		t.Fatalf("update instance role: %v", err)
	}
	site, err := store.CreateSite(ctx, ownerID, "admin-visible.example.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	resp, err := (Builder{Store: store}).ForUserSites(ctx, adminID, []api.Site{{ID: site.ID, Domain: site.Domain}})
	if err != nil {
		t.Fatalf("ForUserSites: %v", err)
	}
	if resp.InstanceRole != string(auth.InstanceAdmin) {
		t.Fatalf("expected instance admin, got %q", resp.InstanceRole)
	}
	if len(resp.Permissions) != 0 || len(resp.SiteCapabilities) != 0 {
		t.Fatalf("expected no direct site grants for instance admin, got %+v / %+v", resp.Permissions, resp.SiteCapabilities)
	}
	if !hasAccessValue(resp.InstanceCapabilities, string(auth.PermInstanceViewAllSites)) {
		t.Fatalf("expected instance all-sites capability, got %+v", resp.InstanceCapabilities)
	}
}

func TestBuilderForUserSitesRejectsUnrelatedSiteWithoutInstanceVisibility(t *testing.T) {
	ctx := context.Background()
	store := newAccessTestStore(t)
	defer store.Close()

	ownerID, err := store.CreateUser(ctx, "other-site-owner@example.test", "hash")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	memberID, err := store.CreateUser(ctx, "plain-member@example.test", "hash")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	site, err := store.CreateSite(ctx, ownerID, "private-site.example.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	_, err = (Builder{Store: store}).ForUserSites(ctx, memberID, []api.Site{{ID: site.ID, Domain: site.Domain}})
	if err == nil || !strings.Contains(err.Error(), "resolve site role") {
		t.Fatalf("expected site role resolution error, got %v", err)
	}
}

func TestBuilderForUserDerivesCanCreateTeams(t *testing.T) {
	ctx := context.Background()
	store := newAccessTestStore(t)
	defer store.Close()

	ownerID, err := store.CreateUser(ctx, "limits-owner@example.test", "hash")
	if err != nil {
		t.Fatalf("create instance owner: %v", err)
	}
	memberID, err := store.CreateUser(ctx, "limits-member@example.test", "hash")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}

	// Mirrors the managed-cloud default of HITKEEP_CLOUD_MAX_TEAMS=1.
	cloudProvider := entitlements.NewStaticProvider(entitlements.Entitlements{MaxTeams: 1}, entitlements.PlanInfo{})
	cloudLimits := entitlements.NewService(store, cloudProvider, &config.Config{CloudHosted: true})
	cloudBuilder := Builder{Store: store, Limits: cloudLimits}

	if resp, err := cloudBuilder.ForUser(ctx, ownerID); err != nil || !resp.CanCreateTeams {
		t.Fatalf("expected instance owner to create teams on managed cloud, got %+v (err %v)", resp, err)
	}
	if resp, err := cloudBuilder.ForUser(ctx, memberID); err != nil || resp.CanCreateTeams {
		t.Fatalf("expected cloud user at the team cap not to create teams, got %+v (err %v)", resp, err)
	}

	selfHostedLimits := entitlements.NewService(store, entitlements.NewDefaultProvider(), &config.Config{})
	if resp, err := (Builder{Store: store, Limits: selfHostedLimits}).ForUser(ctx, memberID); err != nil || !resp.CanCreateTeams {
		t.Fatalf("expected self-hosted user to create teams, got %+v (err %v)", resp, err)
	}

	// Builders assembled without a limits service stay permissive.
	if resp, err := (Builder{Store: store}).ForUser(ctx, memberID); err != nil || !resp.CanCreateTeams {
		t.Fatalf("expected builder without limits to allow team creation, got %+v (err %v)", resp, err)
	}
}

func TestActiveTeamContextHandlesMissingMembership(t *testing.T) {
	store := newAccessTestStore(t)
	defer store.Close()

	teamID, role := (Builder{Store: store}).activeTeamContext(context.Background(), uuid.New())
	if teamID == uuid.Nil || role != "" {
		t.Fatalf("expected fallback tenant without role, got %s/%q", teamID, role)
	}
}

func TestBuilderHandlesStoreErrors(t *testing.T) {
	ctx := context.Background()
	store := newAccessTestStore(t)
	userID, err := store.CreateUser(ctx, "closed-store@example.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "closed-store.example.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	store.Close()

	builder := Builder{Store: store}
	if _, err := builder.ForUser(ctx, userID); err == nil {
		t.Fatalf("expected ForUser to return store error")
	}
	if _, err := builder.ForUserSites(ctx, userID, []api.Site{{ID: site.ID, Domain: site.Domain}}); err == nil {
		t.Fatalf("expected ForUserSites to return instance-role store error")
	}
	if teamID, role := builder.activeTeamContext(ctx, userID); teamID != uuid.Nil || role != "" {
		t.Fatalf("expected empty active team context on store error, got %s/%q", teamID, role)
	}
}

func newAccessTestStore(t *testing.T) *database.Store {
	t.Helper()
	return testdb.Shared(t)
}

func hasAccessValue(values []string, target string) bool {
	return slices.Contains(values, target)
}
