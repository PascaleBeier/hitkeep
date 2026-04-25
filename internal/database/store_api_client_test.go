package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/auth"
)

func setupAPIClientStore(t *testing.T) (*Store, uuid.UUID, uuid.UUID) {
	t.Helper()

	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	userID, err := store.CreateUser(context.Background(), "owner@example.com", "hashed-secret")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	return store, userID, site.ID
}

func TestAPIClientLifecycle(t *testing.T) {
	store, userID, siteID := setupAPIClientStore(t)
	defer store.Close()

	ctx := context.Background()
	expiresAt := time.Now().UTC().Add(2 * time.Hour)

	client, token, err := store.CreateAPIClient(ctx, userID, "CI Sync", "pipeline", auth.InstanceAdmin, map[uuid.UUID]auth.SiteRole{
		siteID: auth.SiteOwner,
	}, &expiresAt)
	if err != nil {
		t.Fatalf("create api client: %v", err)
	}
	if token == "" {
		t.Fatalf("expected token")
	}
	if client.ID == uuid.Nil {
		t.Fatalf("expected client id")
	}
	if len(client.SiteRoles) != 1 {
		t.Fatalf("expected one site role, got %d", len(client.SiteRoles))
	}

	list, err := store.ListAPIClients(ctx, userID)
	if err != nil {
		t.Fatalf("list api clients: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one api client, got %d", len(list))
	}

	authz, err := store.GetAPIClientAuth(ctx, token)
	if err != nil {
		t.Fatalf("get api client auth: %v", err)
	}
	if authz == nil {
		t.Fatalf("expected api client auth")
	}
	if authz.UserID != userID {
		t.Fatalf("expected user id %s, got %s", userID, authz.UserID)
	}
	if authz.InstanceRole != auth.InstanceAdmin {
		t.Fatalf("expected instance role %s, got %s", auth.InstanceAdmin, authz.InstanceRole)
	}
	if authz.SiteRoles[siteID] != auth.SiteOwner {
		t.Fatalf("expected site owner role for %s", siteID)
	}
	if client.UserID == nil || *client.UserID != userID {
		t.Fatalf("expected personal client user id %s, got %+v", userID, client.UserID)
	}
	if client.OwnerType != APIClientOwnerPersonal {
		t.Fatalf("expected owner type %q, got %q", APIClientOwnerPersonal, client.OwnerType)
	}

	updated, err := store.UpdateAPIClient(ctx, userID, client.ID, "CI Sync Updated", "pipeline-updated", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		siteID: auth.SiteViewer,
	}, &expiresAt, true)
	if err != nil {
		t.Fatalf("update api client: %v", err)
	}
	if updated == nil {
		t.Fatalf("expected updated client")
	}
	if updated.RevokedAt == nil {
		t.Fatalf("expected client to be revoked")
	}
	if updated.InstanceRole != string(auth.InstanceUser) {
		t.Fatalf("expected updated instance role %s, got %s", auth.InstanceUser, updated.InstanceRole)
	}

	revokedAuth, err := store.GetAPIClientAuth(ctx, token)
	if err != nil {
		t.Fatalf("get revoked api client auth: %v", err)
	}
	if revokedAuth != nil {
		t.Fatalf("expected revoked client token to be invalid")
	}

	if err := store.DeleteAPIClient(ctx, userID, client.ID); err != nil {
		t.Fatalf("delete api client: %v", err)
	}
	remaining, err := store.ListAPIClients(ctx, userID)
	if err != nil {
		t.Fatalf("list api clients after delete: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected no api clients after delete, got %d", len(remaining))
	}
}

func TestTeamAPIClientLifecycle(t *testing.T) {
	store, userID, _ := setupAPIClientStore(t)
	defer store.Close()

	ctx := context.Background()

	team, err := store.CreateTenant(ctx, userID, "Automation Team", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	tenantID := team.ID
	if err := store.SetActiveTenantID(ctx, userID, tenantID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}

	site, err := store.CreateSite(ctx, userID, "team-api-client.example")
	if err != nil {
		t.Fatalf("create team site: %v", err)
	}
	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	client, token, err := store.CreateTeamAPIClient(ctx, tenantID, "Team Sync", "shared integration", map[uuid.UUID]auth.SiteRole{
		site.ID: auth.SiteAdmin,
	}, &expiresAt)
	if err != nil {
		t.Fatalf("create team api client: %v", err)
	}
	if token == "" {
		t.Fatalf("expected team token")
	}
	if client.TenantID == nil || *client.TenantID != tenantID {
		t.Fatalf("expected team client tenant id %s, got %+v", tenantID, client.TenantID)
	}
	if client.UserID != nil {
		t.Fatalf("expected team client to have no user owner, got %+v", client.UserID)
	}
	if client.OwnerType != APIClientOwnerTeam {
		t.Fatalf("expected owner type %q, got %q", APIClientOwnerTeam, client.OwnerType)
	}
	if client.InstanceRole != string(auth.InstanceUser) {
		t.Fatalf("expected team client instance role %q, got %q", auth.InstanceUser, client.InstanceRole)
	}

	list, err := store.ListTeamAPIClients(ctx, tenantID)
	if err != nil {
		t.Fatalf("list team api clients: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one team api client, got %d", len(list))
	}

	authz, err := store.GetAPIClientAuth(ctx, token)
	if err != nil {
		t.Fatalf("get team api client auth: %v", err)
	}
	if authz == nil {
		t.Fatalf("expected team api client auth")
	}
	if authz.UserID != uuid.Nil {
		t.Fatalf("expected no backing user id, got %s", authz.UserID)
	}
	if authz.TenantID != tenantID {
		t.Fatalf("expected tenant id %s, got %s", tenantID, authz.TenantID)
	}
	if authz.SiteRoles[site.ID] != auth.SiteAdmin {
		t.Fatalf("expected delegated team site role %s", auth.SiteAdmin)
	}

	updated, err := store.UpdateTeamAPIClient(ctx, tenantID, client.ID, "Team Sync Updated", "shared integration updated", map[uuid.UUID]auth.SiteRole{
		site.ID: auth.SiteViewer,
	}, &expiresAt, true)
	if err != nil {
		t.Fatalf("update team api client: %v", err)
	}
	if updated == nil || updated.RevokedAt == nil {
		t.Fatalf("expected revoked team client")
	}

	revokedAuth, err := store.GetAPIClientAuth(ctx, token)
	if err != nil {
		t.Fatalf("get revoked team api client auth: %v", err)
	}
	if revokedAuth != nil {
		t.Fatalf("expected revoked team api client token to be invalid")
	}

	if err := store.DeleteTeamAPIClient(ctx, tenantID, client.ID); err != nil {
		t.Fatalf("delete team api client: %v", err)
	}
	remaining, err := store.ListTeamAPIClients(ctx, tenantID)
	if err != nil {
		t.Fatalf("list team api clients after delete: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected no team api clients after delete, got %d", len(remaining))
	}
}

func TestAPIClientAuthHonorsExpiration(t *testing.T) {
	store, userID, _ := setupAPIClientStore(t)
	defer store.Close()

	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	_, token, err := store.CreateAPIClient(context.Background(), userID, "Expired", "", auth.InstanceUser, nil, &expiredAt)
	if err != nil {
		t.Fatalf("create expired api client: %v", err)
	}

	authz, err := store.GetAPIClientAuth(context.Background(), token)
	if err != nil {
		t.Fatalf("get api client auth for expired token: %v", err)
	}
	if authz != nil {
		t.Fatalf("expected expired token to be invalid")
	}
}
