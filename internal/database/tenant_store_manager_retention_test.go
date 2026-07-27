package database

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/controlstore"
)

func seedRetentionTestTeam(t *testing.T, ctx context.Context, store *controlstore.Store) uuid.UUID {
	t.Helper()
	return newManagerTestTenant(t, store, "Retention Test Team")
}

func seedRetentionTestSite(t *testing.T, ctx context.Context, store *controlstore.Store, tenantID uuid.UUID, days int, syncedFromPlan bool) uuid.UUID {
	t.Helper()
	userID, err := store.CreateUser(ctx, uuid.NewString()+"@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.AddTeamMember(ctx, tenantID, userID, controlstore.TenantRoleMember, userID); err != nil {
		t.Fatalf("add team member: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, tenantID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, uuid.NewString()+".example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.UpdateSiteRetention(ctx, site.ID, userID, days, syncedFromPlan); err != nil {
		t.Fatalf("update site retention: %v", err)
	}
	return site.ID
}

func getRetentionTestSite(t *testing.T, ctx context.Context, store *controlstore.Store, tenantID, siteID uuid.UUID) api.Site {
	t.Helper()
	sites, err := store.ListSitesForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("list sites for tenant: %v", err)
	}
	for _, s := range sites {
		if s.ID == siteID {
			return s
		}
	}
	t.Fatalf("site %s not found for tenant %s", siteID, tenantID)
	return api.Site{}
}

func TestSyncTeamRetentionClampsPlanManagedSiteAboveCap(t *testing.T) {
	ctx := context.Background()
	store := newControlTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir(), nil)
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := seedRetentionTestTeam(t, ctx, store)
	siteID := seedRetentionTestSite(t, ctx, store, tenantID, 365, true)

	updated, err := mgr.SyncTeamRetention(ctx, tenantID, 60)
	if err != nil {
		t.Fatalf("SyncTeamRetention: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 site updated, got %d", updated)
	}

	got := getRetentionTestSite(t, ctx, store, tenantID, siteID)
	if got.DataRetentionDays != 60 || !got.RetentionSyncedFromPlan {
		t.Fatalf("expected days=60 synced=true, got days=%d synced=%v", got.DataRetentionDays, got.RetentionSyncedFromPlan)
	}
}

func TestSyncTeamRetentionLeavesPlanManagedSiteAtCapUnchanged(t *testing.T) {
	ctx := context.Background()
	store := newControlTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir(), nil)
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := seedRetentionTestTeam(t, ctx, store)
	seedRetentionTestSite(t, ctx, store, tenantID, 60, true)

	updated, err := mgr.SyncTeamRetention(ctx, tenantID, 60)
	if err != nil {
		t.Fatalf("SyncTeamRetention: %v", err)
	}
	if updated != 0 {
		t.Fatalf("expected 0 sites updated, got %d", updated)
	}
}

func TestSyncTeamRetentionRaisesPlanManagedSiteOnUpgrade(t *testing.T) {
	ctx := context.Background()
	store := newControlTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir(), nil)
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := seedRetentionTestTeam(t, ctx, store)
	siteID := seedRetentionTestSite(t, ctx, store, tenantID, 60, true)

	updated, err := mgr.SyncTeamRetention(ctx, tenantID, 365)
	if err != nil {
		t.Fatalf("SyncTeamRetention: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 site updated, got %d", updated)
	}

	got := getRetentionTestSite(t, ctx, store, tenantID, siteID)
	if got.DataRetentionDays != 365 {
		t.Fatalf("expected days=365 after upgrade, got %d", got.DataRetentionDays)
	}
}

func TestSyncTeamRetentionClampsManuallyCustomizedSiteAboveCap(t *testing.T) {
	ctx := context.Background()
	store := newControlTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir(), nil)
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := seedRetentionTestTeam(t, ctx, store)
	siteID := seedRetentionTestSite(t, ctx, store, tenantID, 1095, false)

	updated, err := mgr.SyncTeamRetention(ctx, tenantID, 60)
	if err != nil {
		t.Fatalf("SyncTeamRetention: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 site updated, got %d", updated)
	}

	got := getRetentionTestSite(t, ctx, store, tenantID, siteID)
	if got.DataRetentionDays != 60 {
		t.Fatalf("expected clamped to 60, got %d", got.DataRetentionDays)
	}
	if got.RetentionSyncedFromPlan {
		t.Fatal("expected manually-customized flag to remain false after clamp")
	}
}

func TestSyncTeamRetentionLeavesManuallyCustomizedSiteUnchangedOnUpgrade(t *testing.T) {
	ctx := context.Background()
	store := newControlTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir(), nil)
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := seedRetentionTestTeam(t, ctx, store)
	siteID := seedRetentionTestSite(t, ctx, store, tenantID, 30, false)

	updated, err := mgr.SyncTeamRetention(ctx, tenantID, 365)
	if err != nil {
		t.Fatalf("SyncTeamRetention: %v", err)
	}
	if updated != 0 {
		t.Fatalf("expected 0 sites updated (manual value under new cap), got %d", updated)
	}

	got := getRetentionTestSite(t, ctx, store, tenantID, siteID)
	if got.DataRetentionDays != 30 {
		t.Fatalf("expected manual value 30 preserved, got %d", got.DataRetentionDays)
	}
}

func TestSyncTeamRetentionUnlimitedCapSetsPlanManagedSiteUnlimited(t *testing.T) {
	ctx := context.Background()
	store := newControlTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir(), nil)
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := seedRetentionTestTeam(t, ctx, store)
	siteID := seedRetentionTestSite(t, ctx, store, tenantID, 60, true)

	updated, err := mgr.SyncTeamRetention(ctx, tenantID, 0)
	if err != nil {
		t.Fatalf("SyncTeamRetention: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 site updated, got %d", updated)
	}

	got := getRetentionTestSite(t, ctx, store, tenantID, siteID)
	if got.DataRetentionDays != 0 {
		t.Fatalf("expected unlimited (0), got %d", got.DataRetentionDays)
	}
}

func TestSyncTeamRetentionUnlimitedCapLeavesManuallyCustomizedSiteUnchanged(t *testing.T) {
	ctx := context.Background()
	store := newControlTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir(), nil)
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := seedRetentionTestTeam(t, ctx, store)
	seedRetentionTestSite(t, ctx, store, tenantID, 30, false)

	updated, err := mgr.SyncTeamRetention(ctx, tenantID, 0)
	if err != nil {
		t.Fatalf("SyncTeamRetention: %v", err)
	}
	if updated != 0 {
		t.Fatalf("expected 0 sites updated on unlimited plan, got %d", updated)
	}
}

func TestSyncTeamRetentionIdempotentReRun(t *testing.T) {
	ctx := context.Background()
	store := newControlTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir(), nil)
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := seedRetentionTestTeam(t, ctx, store)
	seedRetentionTestSite(t, ctx, store, tenantID, 365, true)

	if _, err := mgr.SyncTeamRetention(ctx, tenantID, 60); err != nil {
		t.Fatalf("first SyncTeamRetention: %v", err)
	}
	updated, err := mgr.SyncTeamRetention(ctx, tenantID, 60)
	if err != nil {
		t.Fatalf("second SyncTeamRetention: %v", err)
	}
	if updated != 0 {
		t.Fatalf("expected second run to be a no-op, got %d updated", updated)
	}
}

func TestSyncTeamRetentionUnknownTeamReturnsZero(t *testing.T) {
	ctx := context.Background()
	store := newControlTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir(), nil)
	t.Cleanup(func() { _ = mgr.Close() })

	updated, err := mgr.SyncTeamRetention(ctx, uuid.New(), 60)
	if err != nil {
		t.Fatalf("SyncTeamRetention: %v", err)
	}
	if updated != 0 {
		t.Fatalf("expected 0 for unknown team, got %d", updated)
	}
}

func TestSyncTeamRetentionMultiSiteTeamOnlyTouchesSitesAboveCap(t *testing.T) {
	ctx := context.Background()
	store := newControlTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir(), nil)
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := seedRetentionTestTeam(t, ctx, store)
	aboveCap := seedRetentionTestSite(t, ctx, store, tenantID, 365, true)
	atCap := seedRetentionTestSite(t, ctx, store, tenantID, 60, true)

	updated, err := mgr.SyncTeamRetention(ctx, tenantID, 60)
	if err != nil {
		t.Fatalf("SyncTeamRetention: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 site updated, got %d", updated)
	}

	if got := getRetentionTestSite(t, ctx, store, tenantID, aboveCap); got.DataRetentionDays != 60 {
		t.Fatalf("expected aboveCap site clamped to 60, got %d", got.DataRetentionDays)
	}
	if got := getRetentionTestSite(t, ctx, store, tenantID, atCap); got.DataRetentionDays != 60 {
		t.Fatalf("expected atCap site to remain 60, got %d", got.DataRetentionDays)
	}
}
