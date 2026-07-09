package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestResolveSiteStoreMemoizesSiteSync(t *testing.T) {
	ctx := context.Background()
	shared := newSharedTestStore(t)
	mgr := NewTenantStoreManager(shared, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := shared.CreateUser(ctx, "sync-memo@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := shared.CreateSite(ctx, userID, "sync-memo.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	customTenantID := uuid.New()
	if _, err := shared.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		customTenantID, "Sync Memo Team", time.Now().UTC(),
	); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := shared.UpdateSiteTenant(ctx, site.ID, customTenantID); err != nil {
		t.Fatalf("map site to tenant: %v", err)
	}

	tenantStore, tenantID, err := mgr.ResolveSiteStore(ctx, site.ID)
	if err != nil {
		t.Fatalf("resolve site store: %v", err)
	}
	if tenantID != customTenantID {
		t.Fatalf("expected tenant %s, got %s", customTenantID, tenantID)
	}

	mirroredRetention := func() int {
		t.Helper()
		var days int
		if err := tenantStore.DB().QueryRowContext(ctx,
			"SELECT data_retention_days FROM sites WHERE id = ?", site.ID,
		).Scan(&days); err != nil {
			t.Fatalf("query mirrored site: %v", err)
		}
		return days
	}

	before := mirroredRetention()
	if _, err := shared.DB().ExecContext(ctx,
		"UPDATE sites SET data_retention_days = ? WHERE id = ?", before+7, site.ID,
	); err != nil {
		t.Fatalf("update shared site retention: %v", err)
	}

	// A second resolve inside the memoization window must skip the mirror sync.
	if _, _, err := mgr.ResolveSiteStore(ctx, site.ID); err != nil {
		t.Fatalf("resolve site store again: %v", err)
	}
	if got := mirroredRetention(); got != before {
		t.Fatalf("expected memoized resolve to skip mirror sync (retention %d), got %d", before, got)
	}

	// An explicit SyncSite must bypass the memo and refresh the mirror.
	if err := mgr.SyncSite(ctx, site.ID); err != nil {
		t.Fatalf("forced sync: %v", err)
	}
	if got := mirroredRetention(); got != before+7 {
		t.Fatalf("expected forced sync to refresh mirror to %d, got %d", before+7, got)
	}
}
