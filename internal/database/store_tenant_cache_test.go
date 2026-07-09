package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func createTestTenant(t *testing.T, store *Store, name string) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	if _, err := store.DB().ExecContext(context.Background(),
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		tenantID, name, time.Now().UTC(),
	); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return tenantID
}

func TestGetSiteTenantIDIsCachedUntilInvalidated(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "tenant-cache@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "tenant-cache.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	defaultTenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	got, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get site tenant: %v", err)
	}
	if got != defaultTenantID {
		t.Fatalf("expected default tenant %s, got %s", defaultTenantID, got)
	}

	customTenantID := createTestTenant(t, store, "Tenant Cache Team")
	// Bypass the store API so the cache is not invalidated.
	if _, err := store.DB().ExecContext(ctx,
		"UPDATE site_tenants SET tenant_id = ? WHERE site_id = ?",
		customTenantID, site.ID,
	); err != nil {
		t.Fatalf("move site tenant: %v", err)
	}

	got, err = store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get site tenant after raw move: %v", err)
	}
	if got != defaultTenantID {
		t.Fatalf("expected cached default tenant %s, got %s", defaultTenantID, got)
	}

	store.invalidateSiteTenantID(site.ID)

	got, err = store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get site tenant after invalidation: %v", err)
	}
	if got != customTenantID {
		t.Fatalf("expected custom tenant %s after invalidation, got %s", customTenantID, got)
	}
}

func TestUpdateSiteTenantInvalidatesSiteTenantIDCache(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "tenant-cache-update@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "tenant-cache-update.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	if _, err := store.GetSiteTenantID(ctx, site.ID); err != nil {
		t.Fatalf("prime site tenant cache: %v", err)
	}

	customTenantID := createTestTenant(t, store, "Tenant Cache Update Team")
	if err := store.UpdateSiteTenant(ctx, site.ID, customTenantID); err != nil {
		t.Fatalf("update site tenant: %v", err)
	}

	got, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get site tenant after update: %v", err)
	}
	if got != customTenantID {
		t.Fatalf("expected custom tenant %s after update, got %s", customTenantID, got)
	}
}

func TestTransferSiteTeamWithAuditInvalidatesSiteTenantIDCache(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "tenant-cache-transfer@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "tenant-cache-transfer.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	if _, err := store.GetSiteTenantID(ctx, site.ID); err != nil {
		t.Fatalf("prime site tenant cache: %v", err)
	}

	customTenantID := createTestTenant(t, store, "Tenant Cache Transfer Team")
	if err := store.TransferSiteTeamWithAudit(ctx, site.ID, customTenantID, false, nil); err != nil {
		t.Fatalf("transfer site team: %v", err)
	}

	got, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get site tenant after transfer: %v", err)
	}
	if got != customTenantID {
		t.Fatalf("expected custom tenant %s after transfer, got %s", customTenantID, got)
	}
}

func TestDeleteSiteInvalidatesSiteTenantIDCache(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "tenant-cache-delete@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "tenant-cache-delete.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	if _, err := store.GetSiteTenantID(ctx, site.ID); err != nil {
		t.Fatalf("prime site tenant cache: %v", err)
	}

	if err := store.DeleteSite(ctx, site.ID); err != nil {
		t.Fatalf("delete site: %v", err)
	}

	if _, err := store.GetSiteTenantID(ctx, site.ID); err == nil {
		t.Fatal("expected site tenant lookup to fail after site deletion")
	}
}
