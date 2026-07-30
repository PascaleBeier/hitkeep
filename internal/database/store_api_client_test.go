package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
)

func setupAPIClientStore(t *testing.T) (*Store, uuid.UUID, uuid.UUID) {
	t.Helper()
	store := newSharedTestFixtureStore(t)

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

func TestDeleteAPIClientDoesNotRemoveGrantsForAnotherOwner(t *testing.T) {
	store, userID, _ := setupAPIClientStore(t)
	defer store.Close()

	ctx := context.Background()
	otherUserID, err := store.CreateUser(ctx, "other-owner@example.com", "hashed-secret")
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}
	otherSite, err := store.CreateSite(ctx, otherUserID, "other-owner.example.com")
	if err != nil {
		t.Fatalf("create other site: %v", err)
	}
	otherClient, _, err := store.CreateAPIClient(ctx, otherUserID, "Other owner token", "", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		otherSite.ID: auth.SiteViewer,
	}, nil)
	if err != nil {
		t.Fatalf("create other api client: %v", err)
	}

	if err := store.DeleteAPIClient(ctx, userID, otherClient.ID); !errors.Is(err, ErrAPIClientNotFound) {
		t.Fatalf("expected cross-owner delete to return ErrAPIClientNotFound, got %v", err)
	}

	remaining, err := store.GetAPIClient(ctx, otherUserID, otherClient.ID)
	if err != nil {
		t.Fatalf("get other api client after failed delete: %v", err)
	}
	if remaining == nil {
		t.Fatalf("expected other api client to remain")
	}
	if len(remaining.SiteRoles) != 1 || remaining.SiteRoles[0].SiteID != otherSite.ID || remaining.SiteRoles[0].Role != string(auth.SiteViewer) {
		t.Fatalf("expected failed cross-owner delete to preserve site grants, got %+v", remaining.SiteRoles)
	}
}

func TestUpdateAPIClientPreservesExistingRevokedAtWhenAlreadyRevoked(t *testing.T) {
	store, userID, siteID := setupAPIClientStore(t)
	defer store.Close()

	ctx := context.Background()
	client, _, err := store.CreateAPIClient(ctx, userID, "Revoked metadata", "", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		siteID: auth.SiteViewer,
	}, nil)
	if err != nil {
		t.Fatalf("create api client: %v", err)
	}

	revoked, err := store.UpdateAPIClient(ctx, userID, client.ID, client.Name, client.Description, auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		siteID: auth.SiteViewer,
	}, nil, true)
	if err != nil {
		t.Fatalf("revoke api client: %v", err)
	}
	if revoked == nil || revoked.RevokedAt == nil {
		t.Fatalf("expected revoked timestamp")
	}
	firstRevokedAt := *revoked.RevokedAt
	time.Sleep(5 * time.Millisecond)

	edited, err := store.UpdateAPIClient(ctx, userID, client.ID, "Revoked metadata edited", "updated", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		siteID: auth.SiteViewer,
	}, nil, true)
	if err != nil {
		t.Fatalf("edit revoked api client: %v", err)
	}
	if edited == nil || edited.RevokedAt == nil {
		t.Fatalf("expected edited client to stay revoked")
	}
	if !edited.RevokedAt.Equal(firstRevokedAt) {
		t.Fatalf("expected revoked_at to remain %s, got %s", firstRevokedAt, *edited.RevokedAt)
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

func TestRotateAPIClientInvalidatesOldTokenAndPreservesGrants(t *testing.T) {
	store, userID, siteID := setupAPIClientStore(t)
	defer store.Close()

	ctx := context.Background()
	expiresAt := time.Now().UTC().Add(2 * time.Hour)
	client, oldToken, err := store.CreateAPIClient(ctx, userID, "Rotating Client", "pipeline", auth.InstanceAdmin, map[uuid.UUID]auth.SiteRole{
		siteID: auth.SiteEditor,
	}, &expiresAt)
	if err != nil {
		t.Fatalf("create api client: %v", err)
	}
	if _, err := store.GetAPIClientAuth(ctx, oldToken); err != nil {
		t.Fatalf("prime api client auth cache: %v", err)
	}

	rotated, newToken, err := store.RotateAPIClient(ctx, userID, client.ID)
	if err != nil {
		t.Fatalf("rotate api client: %v", err)
	}
	assertRotatedAPIClient(t, rotated, client, oldToken, newToken, siteID)
	assertRotatedTokenAuth(t, store, ctx, oldToken, newToken, siteID)
}

func assertRotatedAPIClient(t *testing.T, rotated, original *api.APIClient, oldToken, newToken string, siteID uuid.UUID) {
	t.Helper()

	if rotated == nil {
		t.Fatalf("expected rotated client")
	}
	if newToken == "" || newToken == oldToken {
		t.Fatalf("expected new one-time token, got old=%q new=%q", oldToken, newToken)
	}
	if rotated.Name != original.Name || rotated.InstanceRole != string(auth.InstanceAdmin) || rotated.ExpiresAt == nil {
		t.Fatalf("expected rotation to preserve metadata, got %+v", rotated)
	}
	if len(rotated.SiteRoles) != 1 || rotated.SiteRoles[0].SiteID != siteID || rotated.SiteRoles[0].Role != string(auth.SiteEditor) {
		t.Fatalf("expected rotation to preserve site grants, got %+v", rotated.SiteRoles)
	}
}

func assertRotatedTokenAuth(t *testing.T, store *Store, ctx context.Context, oldToken, newToken string, siteID uuid.UUID) {
	t.Helper()

	oldAuth, err := store.GetAPIClientAuth(ctx, oldToken)
	if err != nil {
		t.Fatalf("get old token auth: %v", err)
	}
	if oldAuth != nil {
		t.Fatalf("expected old token to be invalidated")
	}

	newAuth, err := store.GetAPIClientAuth(ctx, newToken)
	if err != nil {
		t.Fatalf("get new token auth: %v", err)
	}
	if newAuth == nil || newAuth.SiteRoles[siteID] != auth.SiteEditor {
		t.Fatalf("expected new token auth with preserved grant, got %+v", newAuth)
	}
}

func TestRotateAPIClientRejectsRevokedAndExpiredClients(t *testing.T) {
	store, userID, siteID := setupAPIClientStore(t)
	defer store.Close()

	ctx := context.Background()
	expiredAt := time.Now().UTC().Add(-time.Minute)
	expired, _, err := store.CreateAPIClient(ctx, userID, "Expired Rotate", "", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		siteID: auth.SiteViewer,
	}, &expiredAt)
	if err != nil {
		t.Fatalf("create expired api client: %v", err)
	}
	if _, _, err := store.RotateAPIClient(ctx, userID, expired.ID); !errors.Is(err, ErrAPIClientInactive) {
		t.Fatalf("expected expired rotate to return ErrAPIClientInactive, got %v", err)
	}

	active, _, err := store.CreateAPIClient(ctx, userID, "Revoked Rotate", "", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		siteID: auth.SiteViewer,
	}, nil)
	if err != nil {
		t.Fatalf("create active api client: %v", err)
	}
	if _, err := store.UpdateAPIClient(ctx, userID, active.ID, active.Name, active.Description, auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		siteID: auth.SiteViewer,
	}, nil, true); err != nil {
		t.Fatalf("revoke api client: %v", err)
	}
	if _, _, err := store.RotateAPIClient(ctx, userID, active.ID); !errors.Is(err, ErrAPIClientInactive) {
		t.Fatalf("expected revoked rotate to return ErrAPIClientInactive, got %v", err)
	}
}
