package database

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type databaseFixture struct {
	once sync.Once
	data []byte
	err  error
}

var (
	sharedDatabaseFixture databaseFixture
	tenantDatabaseFixture databaseFixture
)

func (f *databaseFixture) build(t *testing.T, migrate func(*Store) error) {
	t.Helper()
	f.once.Do(func() {
		dir, err := os.MkdirTemp("", "hitkeep-database-fixture-")
		if err != nil {
			f.err = err
			return
		}
		defer os.RemoveAll(dir)

		path := filepath.Join(dir, "fixture.db")
		store := NewStore(path, WithThreads(1), WithCheckpointInterval(0))
		if err := store.Connect(); err != nil {
			f.err = err
			return
		}
		if err := migrate(store); err != nil {
			_ = store.Close()
			f.err = err
			return
		}
		if err := store.Close(); err != nil {
			f.err = err
			return
		}
		f.data, f.err = os.ReadFile(path)
	})
}

func (f *databaseFixture) clone(t *testing.T, name string, opts ...StoreOption) *Store {
	t.Helper()
	if f.err != nil {
		t.Fatalf("build %s database fixture: %v", name, f.err)
	}
	path := filepath.Join(t.TempDir(), name+".db")
	if err := os.WriteFile(path, f.data, 0o600); err != nil {
		t.Fatalf("clone %s database fixture: %v", name, err)
	}
	storeOptions := append([]StoreOption{WithThreads(1), WithCheckpointInterval(0)}, opts...)
	store := NewStore(path, storeOptions...)
	store.skipCheckpointOnClose = true
	if err := store.Connect(); err != nil {
		t.Fatalf("connect cloned %s database fixture: %v", name, err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newSharedTestFixtureStore(t *testing.T) *Store {
	return newSharedTestFixtureStoreWithOptions(t)
}

func newSharedTestFixtureStoreWithOptions(t *testing.T, opts ...StoreOption) *Store {
	t.Helper()
	sharedDatabaseFixture.build(t, func(store *Store) error {
		return store.Migrate(context.Background())
	})
	return sharedDatabaseFixture.clone(t, "shared", opts...)
}

func newSharedMaintenanceTestStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore(filepath.Join(t.TempDir(), "maintenance.db"), WithThreads(1))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect maintenance store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate maintenance store: %v", err)
	}
	return store
}

func newTenantTestFixtureStore(t *testing.T) *Store {
	return newTenantTestFixtureStoreWithOptions(t)
}

func newTenantTestFixtureStoreWithOptions(t *testing.T, opts ...StoreOption) *Store {
	t.Helper()
	tenantDatabaseFixture.build(t, func(store *Store) error {
		return store.MigrateTenant(context.Background())
	})
	return tenantDatabaseFixture.clone(t, "tenant", opts...)
}

func TestDatabaseFixturesBuildOnceAndCloneIsolatedFiles(t *testing.T) {
	first := newSharedTestFixtureStore(t)
	second := newSharedTestFixtureStore(t)
	if first.path == second.path {
		t.Fatalf("fixture clones share path %q", first.path)
	}
	for _, store := range []*Store{first, second} {
		var migrationCount int
		if err := store.DB().QueryRowContext(context.Background(), "SELECT count(*) FROM migrations").Scan(&migrationCount); err != nil {
			t.Fatalf("read fixture migrations: %v", err)
		}
		if migrationCount == 0 {
			t.Fatal("fixture clone has no migrations")
		}
	}
	if _, err := first.DB().ExecContext(context.Background(), "CREATE TABLE fixture_isolation (value INTEGER)"); err != nil {
		t.Fatalf("write first fixture clone: %v", err)
	}
	var tableCount int
	if err := second.DB().QueryRowContext(context.Background(), "SELECT count(*) FROM information_schema.tables WHERE table_name = 'fixture_isolation'").Scan(&tableCount); err != nil {
		t.Fatalf("read second fixture clone: %v", err)
	}
	if tableCount != 0 {
		t.Fatal("fixture clones are not isolated")
	}
}

func TestDatabaseFixturesUseSingleThreadAndDisablePeriodicCheckpoint(t *testing.T) {
	store := newSharedTestFixtureStore(t)
	var threads int
	if err := store.DB().QueryRowContext(context.Background(), "SELECT current_setting('threads')::INTEGER").Scan(&threads); err != nil {
		t.Fatalf("read DuckDB thread setting: %v", err)
	}
	if threads != 1 {
		t.Fatalf("fixture threads = %d, want 1", threads)
	}
	store.maintenanceMu.Lock()
	maintenanceRunning := store.maintenanceCancel != nil
	store.maintenanceMu.Unlock()
	if maintenanceRunning {
		t.Fatal("fixture started periodic checkpoint maintenance")
	}
	if !store.skipCheckpointOnClose {
		t.Fatal("fixture clone should skip shutdown checkpoints")
	}
}

func TestDatabaseFixtureCloneOptionsOverrideDefaults(t *testing.T) {
	store := newSharedTestFixtureStoreWithOptions(t, WithThreads(2), WithCheckpointInterval(time.Minute))
	var threads int
	if err := store.DB().QueryRowContext(context.Background(), "SELECT current_setting('threads')::INTEGER").Scan(&threads); err != nil {
		t.Fatalf("read overridden DuckDB thread setting: %v", err)
	}
	if threads != 2 {
		t.Fatalf("fixture clone threads = %d, want 2", threads)
	}
	if store.checkpointInterval != time.Minute {
		t.Fatalf("fixture clone checkpoint interval = %s, want %s", store.checkpointInterval, time.Minute)
	}
	store.maintenanceMu.Lock()
	maintenanceRunning := store.maintenanceCancel != nil
	store.maintenanceMu.Unlock()
	if maintenanceRunning {
		t.Fatal("fixture clone started periodic checkpoint maintenance before StartMaintenance")
	}
}

func TestTenantDatabaseFixtureHasCurrentSchema(t *testing.T) {
	first := newTenantTestFixtureStore(t)
	second := newTenantTestFixtureStore(t)
	if first.path == second.path {
		t.Fatalf("tenant fixture clones share path %q", first.path)
	}
	var migrationCount int
	if err := first.DB().QueryRowContext(context.Background(), "SELECT count(*) FROM migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("read tenant fixture migrations: %v", err)
	}
	if migrationCount == 0 {
		t.Fatal("tenant fixture has no migrations")
	}
	if _, err := first.DB().ExecContext(context.Background(), "CREATE TABLE tenant_fixture_isolation (value INTEGER)"); err != nil {
		t.Fatalf("write first tenant fixture clone: %v", err)
	}
	var tableCount int
	if err := second.DB().QueryRowContext(context.Background(), "SELECT count(*) FROM information_schema.tables WHERE table_name = 'tenant_fixture_isolation'").Scan(&tableCount); err != nil {
		t.Fatalf("read second tenant fixture clone: %v", err)
	}
	if tableCount != 0 {
		t.Fatal("tenant fixture clones are not isolated")
	}
}
