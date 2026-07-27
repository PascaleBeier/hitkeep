package controlstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectFormat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if got, err := InspectFormat(filepath.Join(dir, "missing.db")); err != nil || got != FileMissing {
		t.Fatalf("missing: format=%v err=%v", got, err)
	}

	duckPath := filepath.Join(dir, "duck.db")
	duckHeader := append(make([]byte, 8), []byte("DUCK")...)
	if err := os.WriteFile(duckPath, duckHeader, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := InspectFormat(duckPath); err != nil || got != FileDuckDB {
		t.Fatalf("duckdb: format=%v err=%v", got, err)
	}

	badPath := filepath.Join(dir, "bad.db")
	if err := os.WriteFile(badPath, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := InspectFormat(badPath); err != nil || got != FileUnknown {
		t.Fatalf("unknown: format=%v err=%v", got, err)
	}
}

func TestOpenCreatesPureSQLiteControlStoreWithRequiredPragmas(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if got, err := InspectFormat(path); err != nil || got != FileSQLite {
		t.Fatalf("format=%v err=%v", got, err)
	}

	checks := map[string]string{
		"journal_mode": "wal",
		"synchronous":  "2",
		"foreign_keys": "1",
		"temp_store":   "1",
		"mmap_size":    "0",
		"cache_size":   "-8192",
	}
	for pragma, want := range checks {
		var got string
		if err := store.db.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&got); err != nil {
			t.Fatalf("read %s: %v", pragma, err)
		}
		if got != want {
			t.Errorf("PRAGMA %s=%q, want %q", pragma, got, want)
		}
	}

	var migrationsCount int
	if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM control_migrations").Scan(&migrationsCount); err != nil {
		t.Fatal(err)
	}
	if migrationsCount != 2 {
		t.Fatalf("control_migrations count=%d, want 2", migrationsCount)
	}
	var dataMigrationsTable int
	if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'data_migrations'").Scan(&dataMigrationsTable); err != nil {
		t.Fatal(err)
	}
	if dataMigrationsTable != 1 {
		rows, queryErr := store.db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		defer rows.Close()
		var names []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			names = append(names, name)
		}
		t.Fatalf("data_migrations table was not created; tables=%v", names)
	}
	if stats := store.db.Stats(); stats.MaxOpenConnections != maxOpenConnections {
		t.Fatalf("max connections=%d, want %d", stats.MaxOpenConnections, maxOpenConnections)
	}
}

func TestOpenRestrictsSQLiteControlArtifacts(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "control.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("control database permissions=%#o, want 0600", got)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if sidecar, statErr := os.Stat(path + suffix); statErr == nil && sidecar.Mode().Perm() != 0o600 {
			t.Fatalf("control database %s permissions=%#o, want 0600", suffix, sidecar.Mode().Perm())
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			t.Fatal(statErr)
		}
	}
}

func TestEnsureDefaultTenantMarksFreshControlPlaneAsSplitComplete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.EnsureDefaultTenant(ctx); err != nil {
		t.Fatal(err)
	}
	complete, err := store.DefaultTenantSplitComplete(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !complete {
		t.Fatal("fresh SQLite control plane is missing default-tenant split markers")
	}
	imported, err := store.LegacyControlImportComplete(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if imported {
		t.Fatal("fresh SQLite control plane was marked as a legacy import")
	}
}

func TestSQLiteSchemaEnforcesCaseInsensitiveUserEmailUniqueness(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.CreateUser(ctx, "CaseSensitive@example.test", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUser(ctx, "casesensitive@example.test", "hash"); err == nil {
		t.Fatal("created two users whose emails differ only by case")
	}
}

func TestOpenRejectsUnknownAndDuckDBWithoutModification(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{name: "unknown", body: []byte("client material")},
		{name: "duckdb", body: append(make([]byte, 8), []byte("DUCK-source")...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hitkeep.db")
			if err := os.WriteFile(path, tc.body, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(ctx, path); err == nil {
				t.Fatal("Open succeeded")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(tc.body) {
				t.Fatal("Open modified a rejected input")
			}
		})
	}
}

func TestOpenRejectsChangedMigrationChecksum(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE control_migrations SET checksum = 'tampered'"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(ctx, path); err == nil {
		t.Fatal("Open succeeded with a changed migration checksum")
	}
}
