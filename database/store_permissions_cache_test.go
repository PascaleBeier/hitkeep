package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/auth"
)

func TestGetInstanceRoleCacheInvalidatedOnUpdate(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "role-cache@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	role, err := store.GetInstanceRole(ctx, userID)
	if err != nil {
		t.Fatalf("get initial instance role: %v", err)
	}
	if role != auth.InstanceOwner {
		t.Fatalf("expected initial instance role %q, got %q", auth.InstanceOwner, role)
	}

	if err := store.UpdateInstanceRole(ctx, userID, auth.InstanceAdmin, userID); err != nil {
		t.Fatalf("update instance role: %v", err)
	}

	role, err = store.GetInstanceRole(ctx, userID)
	if err != nil {
		t.Fatalf("get updated instance role: %v", err)
	}
	if role != auth.InstanceAdmin {
		t.Fatalf("expected updated instance role %q, got %q", auth.InstanceAdmin, role)
	}
}

func TestGetSiteRoleCacheInvalidatedOnActiveTenantSwitch(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "site-role-cache@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	defaultSite, err := store.CreateSite(ctx, userID, "default-role-cache.test")
	if err != nil {
		t.Fatalf("create default site: %v", err)
	}

	role, err := store.GetSiteRole(ctx, userID, defaultSite.ID)
	if err != nil {
		t.Fatalf("get default site role: %v", err)
	}
	if role != auth.SiteOwner {
		t.Fatalf("expected default site owner role, got %q", role)
	}

	customTenantID := uuid.New()
	now := time.Now().UTC()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		customTenantID, "Role Cache Team", now,
	); err != nil {
		t.Fatalf("create custom tenant: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenant_members (tenant_id, user_id, role, added_by) VALUES (?, ?, ?, ?)",
		customTenantID, userID, TenantRoleOwner, userID,
	); err != nil {
		t.Fatalf("add user to custom tenant: %v", err)
	}

	if err := store.SetActiveTenantID(ctx, userID, customTenantID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}

	customSite, err := store.CreateSite(ctx, userID, "custom-role-cache.test")
	if err != nil {
		t.Fatalf("create custom site: %v", err)
	}

	if _, err := store.GetSiteRole(ctx, userID, defaultSite.ID); err == nil {
		t.Fatal("expected default tenant site access to be invalid after tenant switch")
	}

	role, err = store.GetSiteRole(ctx, userID, customSite.ID)
	if err != nil {
		t.Fatalf("get custom tenant site role: %v", err)
	}
	if role != auth.SiteOwner {
		t.Fatalf("expected custom tenant site owner role, got %q", role)
	}
}

func TestInvalidateAllSiteRolesForUserClearsOnlyTargetUser(t *testing.T) {
	store := setupTenantStore(t)

	userID := uuid.New()
	otherUserID := uuid.New()
	firstSiteID := uuid.New()
	secondSiteID := uuid.New()
	otherSiteID := uuid.New()

	store.cacheSiteRole(userID, firstSiteID, auth.SiteOwner)
	store.cacheSiteRole(userID, secondSiteID, auth.SiteViewer)
	store.cacheSiteRole(otherUserID, otherSiteID, auth.SiteAdmin)

	if _, ok := store.getCachedSiteRole(userID, firstSiteID); !ok {
		t.Fatal("expected first cached site role for user")
	}
	if _, ok := store.getCachedSiteRole(userID, secondSiteID); !ok {
		t.Fatal("expected second cached site role for user")
	}
	if _, ok := store.getCachedSiteRole(otherUserID, otherSiteID); !ok {
		t.Fatal("expected cached site role for other user")
	}

	store.invalidateAllSiteRolesForUser(userID)

	if _, ok := store.getCachedSiteRole(userID, firstSiteID); ok {
		t.Fatal("expected first site role to be invalidated for user")
	}
	if _, ok := store.getCachedSiteRole(userID, secondSiteID); ok {
		t.Fatal("expected second site role to be invalidated for user")
	}
	if role, ok := store.getCachedSiteRole(otherUserID, otherSiteID); !ok {
		t.Fatal("expected other user's site role to remain cached")
	} else if role != auth.SiteAdmin {
		t.Fatalf("expected other user's site role %q, got %q", auth.SiteAdmin, role)
	}
}
