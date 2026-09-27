// Package testdb provides isolated, already-migrated DuckDB fixtures for tests
// outside the database package. Tests that exercise migrations themselves must
// continue to create a fresh Store explicitly.
package testdb

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"hitkeep/internal/database"
)

type fixture struct {
	once sync.Once
	data []byte
	err  error
}

var (
	shared fixture
	tenant fixture
)

func build(f *fixture, migrate func(*database.Store) error) {
	f.once.Do(func() {
		dir, err := os.MkdirTemp("", "hitkeep-testdb-")
		if err != nil {
			f.err = err
			return
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "fixture.db")
		store := database.NewStore(path, database.WithThreads(1), database.WithCheckpointInterval(0))
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

func clone(t testing.TB, f *fixture, name, path string, opts ...database.StoreOption) *database.Store {
	t.Helper()
	if f.err != nil {
		t.Fatalf("build %s test database fixture: %v", name, f.err)
	}
	if path == "" {
		path = filepath.Join(t.TempDir(), name+".db")
	} else if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create %s test database directory: %v", name, err)
	}
	if err := os.WriteFile(path, f.data, 0o600); err != nil {
		t.Fatalf("clone %s test database fixture: %v", name, err)
	}
	storeOptions := append([]database.StoreOption{database.WithThreads(1), database.WithCheckpointInterval(0)}, opts...)
	store := database.NewStore(path, storeOptions...)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect cloned %s test database fixture: %v", name, err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// Shared returns an isolated shared/control database with all shared migrations applied.
func Shared(t testing.TB) *database.Store {
	build(&shared, func(store *database.Store) error { return store.Migrate(context.Background()) })
	return clone(t, &shared, "shared", "")
}

// SharedWithOptions clones the shared fixture with explicit Store options for
// tests that assert configurable runtime settings.
func SharedWithOptions(t testing.TB, opts ...database.StoreOption) *database.Store {
	build(&shared, func(store *database.Store) error { return store.Migrate(context.Background()) })
	return clone(t, &shared, "shared", "", opts...)
}

// SharedAt clones the shared fixture at an explicit path for tests that inspect
// the configured database path as part of their contract.
func SharedAt(t testing.TB, path string) *database.Store {
	build(&shared, func(store *database.Store) error { return store.Migrate(context.Background()) })
	return clone(t, &shared, "shared", path)
}

// SharedAtWithOptions clones the shared fixture at an explicit path with
// explicit Store options.
func SharedAtWithOptions(t testing.TB, path string, opts ...database.StoreOption) *database.Store {
	build(&shared, func(store *database.Store) error { return store.Migrate(context.Background()) })
	return clone(t, &shared, "shared", path, opts...)
}

// Tenant returns an isolated tenant analytics database with all tenant migrations applied.
func Tenant(t testing.TB) *database.Store {
	build(&tenant, func(store *database.Store) error { return store.MigrateTenant(context.Background()) })
	return clone(t, &tenant, "tenant", "")
}
