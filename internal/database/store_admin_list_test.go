package database

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/auth"
)

func setupAdminListStore(t *testing.T) *Store {
	t.Helper()
	return newSharedTestFixtureStore(t)
}

func TestListUsersIncludesInstanceRole(t *testing.T) {
	store := setupAdminListStore(t)
	defer store.Close()

	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "owner@example.com", "hashed-secret")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	adminID, err := store.CreateUser(ctx, "admin@example.com", "hashed-secret")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	userID, err := store.CreateUser(ctx, "user@example.com", "hashed-secret")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := store.UpdateInstanceRole(ctx, adminID, auth.InstanceAdmin, ownerID); err != nil {
		t.Fatalf("update admin role: %v", err)
	}

	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("expected 3 users, got %d", len(users))
	}

	rolesByUserID := make(map[uuid.UUID]string, len(users))
	for _, user := range users {
		rolesByUserID[user.ID] = user.InstanceRole
	}

	if got := rolesByUserID[ownerID]; got != string(auth.InstanceOwner) {
		t.Fatalf("expected owner role %q, got %q", auth.InstanceOwner, got)
	}
	if got := rolesByUserID[adminID]; got != string(auth.InstanceAdmin) {
		t.Fatalf("expected admin role %q, got %q", auth.InstanceAdmin, got)
	}
	if got := rolesByUserID[userID]; got != string(auth.InstanceUser) {
		t.Fatalf("expected default role %q, got %q", auth.InstanceUser, got)
	}
}

func TestListAllSitesIncludesOwnerEmail(t *testing.T) {
	store := setupAdminListStore(t)
	defer store.Close()

	ctx := context.Background()

	ownerOneID, err := store.CreateUser(ctx, "owner.one@example.com", "hashed-secret")
	if err != nil {
		t.Fatalf("create owner one: %v", err)
	}
	ownerTwoID, err := store.CreateUser(ctx, "owner.two@example.com", "hashed-secret")
	if err != nil {
		t.Fatalf("create owner two: %v", err)
	}

	siteOne, err := store.CreateSite(ctx, ownerOneID, "site-one.example.com")
	if err != nil {
		t.Fatalf("create site one: %v", err)
	}
	siteTwo, err := store.CreateSite(ctx, ownerTwoID, "site-two.example.com")
	if err != nil {
		t.Fatalf("create site two: %v", err)
	}

	sites, err := store.ListAllSites(ctx)
	if err != nil {
		t.Fatalf("list all sites: %v", err)
	}
	if len(sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(sites))
	}

	ownerEmailsBySiteID := make(map[uuid.UUID]string, len(sites))
	for _, site := range sites {
		ownerEmailsBySiteID[site.ID] = site.OwnerEmail
	}

	if got := ownerEmailsBySiteID[siteOne.ID]; got != "owner.one@example.com" {
		t.Fatalf("expected site one owner email, got %q", got)
	}
	if got := ownerEmailsBySiteID[siteTwo.ID]; got != "owner.two@example.com" {
		t.Fatalf("expected site two owner email, got %q", got)
	}
}
