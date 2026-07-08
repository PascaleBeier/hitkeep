package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func querySetting(t *testing.T, store *Store, name string) string {
	t.Helper()
	var value string
	if err := WithPinnedConn(context.Background(), store.DB(), func(conn *sql.Conn) error {
		return conn.QueryRowContext(context.Background(), "SELECT CAST(current_setting(?) AS VARCHAR)", name).Scan(&value)
	}); err != nil {
		t.Fatalf("query setting %s: %v", name, err)
	}
	return value
}

func TestConnectAppliesMemoryLimitAndThreads(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "settings.db"), WithMemoryLimit("1GiB"), WithThreads(2))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if got := querySetting(t, store, "memory_limit"); got != "1.0 GiB" {
		t.Fatalf("expected memory_limit 1.0 GiB, got %q", got)
	}
	if got := querySetting(t, store, "threads"); got != "2" {
		t.Fatalf("expected threads 2, got %q", got)
	}
}

func TestConnectWithoutOptionsKeepsDuckDBDefaults(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "defaults.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if got := querySetting(t, store, "memory_limit"); got == "1.0 GiB" || got == "" {
		t.Fatalf("expected DuckDB default memory_limit, got %q", got)
	}
}

func TestConnectRejectsInvalidMemoryLimit(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "invalid.db"), WithMemoryLimit("512MB'; DROP TABLE hits;--"))
	if err := store.Connect(); err == nil {
		_ = store.Close()
		t.Fatal("expected Connect to reject an invalid memory limit")
	}
}

func TestConnectRejectsNegativeThreads(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "threads.db"), WithThreads(-3))
	if err := store.Connect(); err == nil {
		_ = store.Close()
		t.Fatal("expected Connect to reject negative threads")
	}
}

func TestTenantStoreInheritsDuckDBSettings(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	shared := NewStore(filepath.Join(tmpDir, "shared.db"), WithMemoryLimit("1GiB"))
	if err := shared.Connect(); err != nil {
		t.Fatalf("connect shared: %v", err)
	}
	t.Cleanup(func() { _ = shared.Close() })
	if err := shared.Migrate(ctx); err != nil {
		t.Fatalf("migrate shared: %v", err)
	}

	mgr := NewTenantStoreManager(shared, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := uuid.New()
	if _, err := shared.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		tenantID, "Settings Tenant", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	tenantStore, err := mgr.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ForTenant: %v", err)
	}
	if got := querySetting(t, tenantStore, "memory_limit"); got != "1.0 GiB" {
		t.Fatalf("expected tenant store to inherit memory_limit 1.0 GiB, got %q", got)
	}
}
