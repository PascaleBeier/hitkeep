package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
)

// TestCrossTenantHitIsolation verifies that analytics data written to one
// tenant's data plane is invisible from another tenant's data plane.
func TestCrossTenantHitIsolation(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Set up the shared (control plane) store.
	shared := NewStore(filepath.Join(tmpDir, "shared.db"))
	if err := shared.Connect(); err != nil {
		t.Fatalf("connect shared: %v", err)
	}
	t.Cleanup(func() { _ = shared.Close() })
	if err := shared.Migrate(ctx); err != nil {
		t.Fatalf("migrate shared: %v", err)
	}

	mgr := NewTenantStoreManager(shared, tmpDir)
	t.Cleanup(func() { _ = mgr.Close() })

	// Create a user (owner of the default tenant).
	userID, err := shared.CreateUser(ctx, "isolation@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	defaultTenantID, err := shared.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	// Create a second tenant.
	tenantBID := uuid.New()
	now := time.Now().UTC()
	if _, err := shared.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		tenantBID, "Tenant B", now,
	); err != nil {
		t.Fatalf("insert tenant B: %v", err)
	}
	if _, err := shared.DB().ExecContext(ctx,
		"INSERT INTO tenant_members (tenant_id, user_id, role, added_by) VALUES (?, ?, ?, ?)",
		tenantBID, userID, TenantRoleOwner, userID,
	); err != nil {
		t.Fatalf("add user to tenant B: %v", err)
	}

	// Create a site in the default tenant.
	siteA, err := shared.CreateSite(ctx, userID, "site-a.test")
	if err != nil {
		t.Fatalf("create site A: %v", err)
	}

	// Switch to tenant B and create a site there.
	if err := shared.SetActiveTenantID(ctx, userID, tenantBID); err != nil {
		t.Fatalf("set active tenant B: %v", err)
	}
	siteB, err := shared.CreateSite(ctx, userID, "site-b.test")
	if err != nil {
		t.Fatalf("create site B: %v", err)
	}

	// Resolve data plane stores.
	storeA, err := mgr.ForTenant(ctx, defaultTenantID)
	if err != nil {
		t.Fatalf("ForTenant(default): %v", err)
	}
	storeB, err := mgr.ForTenant(ctx, tenantBID)
	if err != nil {
		t.Fatalf("ForTenant(B): %v", err)
	}

	if storeA == storeB {
		t.Fatal("expected different store instances for default and tenant B")
	}

	// Write a hit to site A (default tenant data plane).
	isUnique := true
	hitA := &api.Hit{
		SiteID:    siteA.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: now,
		Path:      "/tenant-a-page",
		IsUnique:  &isUnique,
	}
	if err := storeA.CreateHit(ctx, hitA); err != nil {
		t.Fatalf("create hit A: %v", err)
	}

	// Write a hit to site B (tenant B data plane).
	hitB := &api.Hit{
		SiteID:    siteB.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: now,
		Path:      "/tenant-b-page",
		IsUnique:  &isUnique,
	}
	if err := storeB.CreateHit(ctx, hitB); err != nil {
		t.Fatalf("create hit B: %v", err)
	}

	period := api.AnalyticsParams{
		SiteID: siteA.ID,
		UserID: userID,
		Start:  now.Add(-1 * time.Hour),
		End:    now.Add(1 * time.Hour),
	}

	// --- Verify: storeA sees site A's hit ---
	statsA, err := storeA.GetSiteStats(ctx, period)
	if err != nil {
		t.Fatalf("GetSiteStats on storeA for siteA: %v", err)
	}
	if statsA.TotalPageviews != 1 {
		t.Fatalf("expected 1 pageview on storeA for siteA, got %d", statsA.TotalPageviews)
	}

	// --- Verify: storeA does NOT see site B's hit ---
	period.SiteID = siteB.ID
	statsAforB, err := storeA.GetSiteStats(ctx, period)
	if err != nil {
		t.Fatalf("GetSiteStats on storeA for siteB: %v", err)
	}
	if statsAforB.TotalPageviews != 0 {
		t.Fatalf("expected 0 pageviews on storeA for siteB (cross-tenant leak!), got %d", statsAforB.TotalPageviews)
	}

	// --- Verify: storeB sees site B's hit ---
	statsBforB, err := storeB.GetSiteStats(ctx, period)
	if err != nil {
		t.Fatalf("GetSiteStats on storeB for siteB: %v", err)
	}
	if statsBforB.TotalPageviews != 1 {
		t.Fatalf("expected 1 pageview on storeB for siteB, got %d", statsBforB.TotalPageviews)
	}

	// --- Verify: storeB does NOT see site A's hit ---
	period.SiteID = siteA.ID
	statsBforA, err := storeB.GetSiteStats(ctx, period)
	if err != nil {
		t.Fatalf("GetSiteStats on storeB for siteA: %v", err)
	}
	if statsBforA.TotalPageviews != 0 {
		t.Fatalf("expected 0 pageviews on storeB for siteA (cross-tenant leak!), got %d", statsBforA.TotalPageviews)
	}
}

// TestCrossTenantGetHitsIsolation verifies that the GetHits query (paginated
// hit list) is isolated per data plane.
func TestCrossTenantGetHitsIsolation(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	shared := NewStore(filepath.Join(tmpDir, "shared.db"))
	if err := shared.Connect(); err != nil {
		t.Fatalf("connect shared: %v", err)
	}
	t.Cleanup(func() { _ = shared.Close() })
	if err := shared.Migrate(ctx); err != nil {
		t.Fatalf("migrate shared: %v", err)
	}

	mgr := NewTenantStoreManager(shared, tmpDir)
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := shared.CreateUser(ctx, "hits-isolation@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	defaultTenantID, err := shared.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	tenantBID := uuid.New()
	now := time.Now().UTC()
	if _, err := shared.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		tenantBID, "Tenant B Hits", now,
	); err != nil {
		t.Fatalf("insert tenant B: %v", err)
	}
	if _, err := shared.DB().ExecContext(ctx,
		"INSERT INTO tenant_members (tenant_id, user_id, role, added_by) VALUES (?, ?, ?, ?)",
		tenantBID, userID, TenantRoleOwner, userID,
	); err != nil {
		t.Fatalf("add user to tenant B: %v", err)
	}

	siteA, err := shared.CreateSite(ctx, userID, "hits-a.test")
	if err != nil {
		t.Fatalf("create site A: %v", err)
	}

	if err := shared.SetActiveTenantID(ctx, userID, tenantBID); err != nil {
		t.Fatalf("set active tenant B: %v", err)
	}
	siteB, err := shared.CreateSite(ctx, userID, "hits-b.test")
	if err != nil {
		t.Fatalf("create site B: %v", err)
	}

	storeA, err := mgr.ForTenant(ctx, defaultTenantID)
	if err != nil {
		t.Fatalf("ForTenant(default): %v", err)
	}
	storeB, err := mgr.ForTenant(ctx, tenantBID)
	if err != nil {
		t.Fatalf("ForTenant(B): %v", err)
	}

	isUnique := true
	// Write 3 hits to site A.
	for i := range 3 {
		if err := storeA.CreateHit(ctx, &api.Hit{
			SiteID:    siteA.ID,
			SessionID: uuid.New(),
			PageID:    uuid.New(),
			Timestamp: now,
			Path:      "/a",
			IsUnique:  &isUnique,
		}); err != nil {
			t.Fatalf("create hit A[%d]: %v", i, err)
		}
	}

	// Write 2 hits to site B.
	for i := range 2 {
		if err := storeB.CreateHit(ctx, &api.Hit{
			SiteID:    siteB.ID,
			SessionID: uuid.New(),
			PageID:    uuid.New(),
			Timestamp: now,
			Path:      "/b",
			IsUnique:  &isUnique,
		}); err != nil {
			t.Fatalf("create hit B[%d]: %v", i, err)
		}
	}

	params := api.HitQueryParams{
		SiteID: siteA.ID,
		Start:  now.Add(-1 * time.Hour),
		End:    now.Add(1 * time.Hour),
		Limit:  100,
	}

	// storeA should return 3 hits for siteA.
	hitsA, err := storeA.GetHits(ctx, params)
	if err != nil {
		t.Fatalf("GetHits storeA siteA: %v", err)
	}
	if hitsA.Total != 3 {
		t.Fatalf("expected 3 hits on storeA for siteA, got %d", hitsA.Total)
	}

	// storeB should return 0 hits for siteA (different data plane).
	hitsBforA, err := storeB.GetHits(ctx, params)
	if err != nil {
		t.Fatalf("GetHits storeB siteA: %v", err)
	}
	if hitsBforA.Total != 0 {
		t.Fatalf("expected 0 hits on storeB for siteA (cross-tenant leak!), got %d", hitsBforA.Total)
	}

	// storeB should return 2 hits for siteB.
	params.SiteID = siteB.ID
	hitsB, err := storeB.GetHits(ctx, params)
	if err != nil {
		t.Fatalf("GetHits storeB siteB: %v", err)
	}
	if hitsB.Total != 2 {
		t.Fatalf("expected 2 hits on storeB for siteB, got %d", hitsB.Total)
	}
}

// TestAnalyticsStoreResolution verifies the AnalyticsStore helper on
// TenantStoreManager correctly routes site IDs to their tenant's data plane.
func TestAnalyticsStoreResolution(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	shared := NewStore(filepath.Join(tmpDir, "shared.db"))
	if err := shared.Connect(); err != nil {
		t.Fatalf("connect shared: %v", err)
	}
	t.Cleanup(func() { _ = shared.Close() })
	if err := shared.Migrate(ctx); err != nil {
		t.Fatalf("migrate shared: %v", err)
	}

	mgr := NewTenantStoreManager(shared, tmpDir)
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := shared.CreateUser(ctx, "resolve@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	defaultTenantID, err := shared.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	// Default tenant site resolves to shared store.
	siteDefault, err := shared.CreateSite(ctx, userID, "default-resolve.test")
	if err != nil {
		t.Fatalf("create default site: %v", err)
	}

	tenantIDDefault, err := shared.GetSiteTenantID(ctx, siteDefault.ID)
	if err != nil {
		t.Fatalf("GetSiteTenantID(default): %v", err)
	}
	if tenantIDDefault != defaultTenantID {
		t.Fatalf("expected default tenant %s, got %s", defaultTenantID, tenantIDDefault)
	}

	storeForDefault, err := mgr.ForTenant(ctx, tenantIDDefault)
	if err != nil {
		t.Fatalf("ForTenant(default): %v", err)
	}
	if storeForDefault != shared {
		t.Fatal("expected default tenant site to resolve to shared store")
	}

	// Custom tenant site resolves to a separate store.
	customTenantID := uuid.New()
	now := time.Now().UTC()
	if _, err := shared.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		customTenantID, "Custom Resolve", now,
	); err != nil {
		t.Fatalf("insert custom tenant: %v", err)
	}
	if _, err := shared.DB().ExecContext(ctx,
		"INSERT INTO tenant_members (tenant_id, user_id, role, added_by) VALUES (?, ?, ?, ?)",
		customTenantID, userID, TenantRoleOwner, userID,
	); err != nil {
		t.Fatalf("add user to custom tenant: %v", err)
	}

	if err := shared.SetActiveTenantID(ctx, userID, customTenantID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	siteCustom, err := shared.CreateSite(ctx, userID, "custom-resolve.test")
	if err != nil {
		t.Fatalf("create custom site: %v", err)
	}

	tenantIDCustom, err := shared.GetSiteTenantID(ctx, siteCustom.ID)
	if err != nil {
		t.Fatalf("GetSiteTenantID(custom): %v", err)
	}
	if tenantIDCustom != customTenantID {
		t.Fatalf("expected custom tenant %s, got %s", customTenantID, tenantIDCustom)
	}

	storeForCustom, err := mgr.ForTenant(ctx, tenantIDCustom)
	if err != nil {
		t.Fatalf("ForTenant(custom): %v", err)
	}
	if storeForCustom == shared {
		t.Fatal("expected custom tenant site to resolve to a different store")
	}
}
