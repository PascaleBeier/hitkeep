package controlstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func openAtomicMutationTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestUpdateUserProfileDoesNotRewriteForeignKeysThroughShadowRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := openAtomicMutationTestStore(t)

	userID, err := store.CreateUserWithNamesAndDefaultTenantName(ctx, "before@example.test", "hash", "Before", "Name", "Team")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		CREATE TRIGGER reject_tenant_member_user_rewrite
		BEFORE UPDATE OF user_id ON tenant_members
		BEGIN
			SELECT RAISE(ABORT, 'foreign key rewrite is forbidden');
		END
	`); err != nil {
		t.Fatal(err)
	}

	if err := store.UpdateUserProfile(ctx, userID, "after@example.test", "After", "Name"); err != nil {
		t.Fatalf("update profile: %v", err)
	}

	var email string
	if err := store.db.QueryRowContext(ctx, "SELECT email FROM users WHERE id = ?", userID).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email != "after@example.test" {
		t.Fatalf("email=%q, want after@example.test", email)
	}
	var userCount int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount); err != nil {
		t.Fatal(err)
	}
	if userCount != 1 {
		t.Fatalf("users=%d, want only the original user", userCount)
	}
}

func TestUpdateSiteDomainDoesNotCreateShadowSites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := openAtomicMutationTestStore(t)

	userID, err := store.CreateUserWithNamesAndDefaultTenantName(ctx, "site-owner@example.test", "hash", "Site", "Owner", "Team")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "before.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		CREATE TRIGGER reject_shadow_site_cleanup
		BEFORE DELETE ON sites
		WHEN OLD.domain LIKE '__shadow_%'
		BEGIN
			SELECT RAISE(ABORT, 'shadow sites are forbidden');
		END
	`); err != nil {
		t.Fatal(err)
	}

	if err := store.UpdateSiteDomain(ctx, site.ID, "after.example.test"); err != nil {
		t.Fatalf("update site domain: %v", err)
	}

	var domain string
	if err := store.db.QueryRowContext(ctx, "SELECT domain FROM sites WHERE id = ?", site.ID).Scan(&domain); err != nil {
		t.Fatal(err)
	}
	if domain != "after.example.test" {
		t.Fatalf("domain=%q, want after.example.test", domain)
	}
	var siteCount int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sites").Scan(&siteCount); err != nil {
		t.Fatal(err)
	}
	if siteCount != 1 {
		t.Fatalf("sites=%d, want only the original site", siteCount)
	}
}

func TestDeleteUserRollsBackCleanupWhenParentDeleteFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := openAtomicMutationTestStore(t)
	userID := uuid.New()
	if _, err := store.db.ExecContext(ctx,
		"INSERT INTO users (id, email, password, created_at) VALUES (?, ?, ?, ?)",
		userID, "delete@example.test", "hash", time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx,
		"INSERT INTO remember_me_tokens (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)",
		"keep-on-failure", userID, time.Now().UTC().Add(time.Hour), time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		CREATE TRIGGER reject_user_delete
		BEFORE DELETE ON users
		BEGIN
			SELECT RAISE(ABORT, 'user delete rejected');
		END
	`); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteUser(ctx, userID); err == nil {
		t.Fatal("DeleteUser succeeded despite rejecting trigger")
	}

	var tokenCount int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM remember_me_tokens WHERE user_id = ?", userID).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	if tokenCount != 1 {
		t.Fatalf("remember-me tokens=%d, want cleanup rolled back", tokenCount)
	}
}

func TestDeleteSiteRollsBackCleanupWhenParentDeleteFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := openAtomicMutationTestStore(t)

	userID, err := store.CreateUserWithNamesAndDefaultTenantName(ctx, "site-delete@example.test", "hash", "Site", "Delete", "Team")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "delete.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		CREATE TRIGGER reject_site_delete
		BEFORE DELETE ON sites
		BEGIN
			SELECT RAISE(ABORT, 'site delete rejected');
		END
	`); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteSite(ctx, site.ID); err == nil {
		t.Fatal("DeleteSite succeeded despite rejecting trigger")
	}

	var mappingCount int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM site_tenants WHERE site_id = ?", site.ID).Scan(&mappingCount); err != nil {
		t.Fatal(err)
	}
	if mappingCount != 1 {
		t.Fatalf("site tenant mappings=%d, want cleanup rolled back", mappingCount)
	}
}
