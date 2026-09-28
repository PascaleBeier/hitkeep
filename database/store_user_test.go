package database

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDeleteUserBlocksSoleOwnerDeletion(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "owner@delete-user.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	memberID, err := store.CreateUser(ctx, "member@delete-user.test", "hash")
	if err != nil {
		t.Fatalf("create member user: %v", err)
	}

	site, err := store.CreateSite(ctx, ownerID, "delete-user.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	err = store.DeleteUser(ctx, ownerID)
	if err == nil {
		t.Fatalf("expected deletion to be blocked")
	}

	var ownsTeamsErr *UserOwnsTeamsError
	if !errors.As(err, &ownsTeamsErr) {
		t.Fatalf("expected UserOwnsTeamsError, got %v", err)
	}
	if !errors.Is(err, ErrUserOwnsTeams) {
		t.Fatalf("expected ErrUserOwnsTeams, got %v", err)
	}
	if len(ownsTeamsErr.Teams) != 1 {
		t.Fatalf("expected 1 blocking team, got %d", len(ownsTeamsErr.Teams))
	}
	if ownsTeamsErr.Teams[0].Name != defaultTenantName {
		t.Fatalf("expected blocking team %q, got %q", defaultTenantName, ownsTeamsErr.Teams[0].Name)
	}

	if user, err := store.GetUserByID(ctx, ownerID); err != nil || user == nil {
		t.Fatalf("expected blocked user to remain, got user=%v err=%v", user, err)
	}
	if member, err := store.GetUserByID(ctx, memberID); err != nil || member == nil {
		t.Fatalf("expected teammate to remain, got user=%v err=%v", member, err)
	}
	if site, err := store.GetSiteByID(ctx, site.ID); err != nil || site == nil {
		t.Fatalf("expected owned site to remain after blocked deletion, got site=%v err=%v", site, err)
	}
}

func TestDeleteUserAllowsDeletionWhenAnotherOwnerExists(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "owner@delete-user-success.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	coOwnerID, err := store.CreateUser(ctx, "co-owner@delete-user-success.test", "hash")
	if err != nil {
		t.Fatalf("create co-owner user: %v", err)
	}

	defaultTenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	if err := store.AddTeamMember(ctx, defaultTenantID, coOwnerID, TenantRoleOwner, ownerID); err != nil {
		t.Fatalf("promote co-owner: %v", err)
	}

	site, err := store.CreateSite(ctx, ownerID, "delete-user-success.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	if err := store.DeleteUser(ctx, ownerID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	user, err := store.GetUserByID(ctx, ownerID)
	if err != nil {
		t.Fatalf("lookup deleted user: %v", err)
	}
	if user != nil {
		t.Fatalf("expected user to be deleted, got %+v", user)
	}

	loadedSite, err := store.GetSiteByID(ctx, site.ID)
	if err != nil {
		t.Fatalf("lookup deleted site: %v", err)
	}
	if loadedSite != nil {
		t.Fatalf("expected owned site to be deleted with user, got %+v", loadedSite)
	}
}

func TestDeleteUserRemovesLegacyAuthStateRows(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "legacy-auth-owner@delete-user.test", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	coOwnerID, err := store.CreateUser(ctx, "legacy-auth-co-owner@delete-user.test", "hash")
	if err != nil {
		t.Fatalf("create co-owner user: %v", err)
	}

	defaultTenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	if err := store.AddTeamMember(ctx, defaultTenantID, coOwnerID, TenantRoleOwner, ownerID); err != nil {
		t.Fatalf("promote co-owner: %v", err)
	}

	if _, err := store.DB().ExecContext(ctx, `
		CREATE TABLE user_totp_pending_setup (
			user_id UUID PRIMARY KEY REFERENCES users(id),
			secret VARCHAR NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL
		)
	`); err != nil {
		t.Fatalf("create legacy totp table: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		CREATE TABLE user_passkey_challenges (
			user_id UUID PRIMARY KEY REFERENCES users(id),
			challenge VARCHAR NOT NULL,
			requested_name VARCHAR,
			created_at TIMESTAMPTZ NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL
		)
	`); err != nil {
		t.Fatalf("create legacy passkey table: %v", err)
	}

	now := time.Now().UTC()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO user_totp_pending_setup (user_id, secret, created_at, expires_at) VALUES (?, ?, ?, ?)",
		ownerID, "secret", now, now.Add(time.Hour),
	); err != nil {
		t.Fatalf("insert legacy totp row: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO user_passkey_challenges (user_id, challenge, requested_name, created_at, expires_at) VALUES (?, ?, ?, ?, ?)",
		ownerID, "challenge", "Key", now, now.Add(time.Hour),
	); err != nil {
		t.Fatalf("insert legacy passkey row: %v", err)
	}

	if err := store.DeleteUser(ctx, ownerID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	user, err := store.GetUserByID(ctx, ownerID)
	if err != nil {
		t.Fatalf("lookup deleted user: %v", err)
	}
	if user != nil {
		t.Fatalf("expected user to be deleted, got %+v", user)
	}
}
