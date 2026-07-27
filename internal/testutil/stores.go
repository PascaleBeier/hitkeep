package testutil

import (
	"context"
	"path/filepath"
	"testing"

	"hitkeep/internal/controlstore"
	"hitkeep/internal/database"
)

// NewControlStore opens an isolated SQLite control plane with its mandatory
// default tenant. Tests should use this instead of the historical combined
// DuckDB fixture.
func NewControlStore(tb testing.TB) *controlstore.Store {
	tb.Helper()
	ctx := context.Background()
	store, err := controlstore.Open(ctx, filepath.Join(tb.TempDir(), "control.db"))
	if err != nil {
		tb.Fatalf("open test control store: %v", err)
	}
	if _, err := store.EnsureDefaultTenant(ctx); err != nil {
		_ = store.Close()
		tb.Fatalf("ensure test default tenant: %v", err)
	}
	return store
}

// NewControlAndTenantStores opens an isolated SQLite control plane and its
// default-rooted, shared DuckDB tenant data plane.
func NewControlAndTenantStores(tb testing.TB) (*controlstore.Store, *database.TenantStoreManager) {
	tb.Helper()
	control := NewControlStore(tb)
	manager := database.NewTenantStoreManager(control, filepath.Join(tb.TempDir(), "data"), nil)
	return control, manager
}
