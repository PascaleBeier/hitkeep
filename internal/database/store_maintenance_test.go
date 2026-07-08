package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func maintenanceRunning(s *Store) bool {
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	return s.maintenanceCancel != nil
}

func insertTestTenant(t *testing.T, store *Store) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	if _, err := store.DB().ExecContext(context.Background(),
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		tenantID, "Maintenance Tenant", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	return tenantID
}

func TestTenantStoreOpenedAfterStartMaintenanceRunsCheckpoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	shared := newSharedTestStore(t)
	mgr := NewTenantStoreManager(shared, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })
	mgr.StartMaintenance(ctx)

	tenantStore, err := mgr.ForTenant(ctx, insertTestTenant(t, shared))
	if err != nil {
		t.Fatalf("ForTenant: %v", err)
	}
	if !maintenanceRunning(tenantStore) {
		t.Fatal("expected tenant store opened after StartMaintenance to run checkpoint maintenance")
	}
}

func TestTenantStoreOpenedBeforeStartMaintenanceRunsCheckpoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	shared := newSharedTestStore(t)
	mgr := NewTenantStoreManager(shared, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	tenantStore, err := mgr.ForTenant(ctx, insertTestTenant(t, shared))
	if err != nil {
		t.Fatalf("ForTenant: %v", err)
	}
	if maintenanceRunning(tenantStore) {
		t.Fatal("expected no maintenance before StartMaintenance")
	}

	mgr.StartMaintenance(ctx)
	if !maintenanceRunning(tenantStore) {
		t.Fatal("expected StartMaintenance to cover already-open tenant stores")
	}
}

func TestCloseStopsMaintenance(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "maint.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	store.StartMaintenance(context.Background())
	if !maintenanceRunning(store) {
		t.Fatal("expected maintenance to be running after StartMaintenance")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if maintenanceRunning(store) {
		t.Fatal("expected Close to stop maintenance")
	}
}
