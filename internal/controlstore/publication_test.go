package controlstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublishLegacyConversionRetainsSourceAndPublishesSQLite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "hitkeep.db")
	source := append(make([]byte, 8), []byte("DUCK-post-split-control")...)
	if err := os.WriteFile(path, source, 0o644); err != nil {
		t.Fatal(err)
	}
	sourceSHA, err := checksumFile(path)
	if err != nil {
		t.Fatal(err)
	}
	createPublicationWork(t, ctx, SQLiteWorkPath(path), sourceSHA)

	if err := PublishLegacyConversion(ctx, path); err != nil {
		t.Fatal(err)
	}
	if format, err := InspectFormat(path); err != nil || format != FileSQLite {
		t.Fatalf("published format=%v err=%v", format, err)
	}
	retained, err := os.ReadFile(PreSQLiteEvidencePath(path))
	if err != nil {
		t.Fatal(err)
	}
	if string(retained) != string(source) {
		t.Fatal("retained DuckDB source changed")
	}
	if info, err := os.Stat(PreSQLiteEvidencePath(path)); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("retained evidence permissions=%#o", info.Mode().Perm())
	}
	if err := PublishLegacyConversion(ctx, path); err != nil {
		t.Fatalf("idempotent publish: %v", err)
	}
}

func TestPublishLegacyConversionCompletesAfterSourceRename(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "hitkeep.db")
	evidencePath := PreSQLiteEvidencePath(path)
	source := append(make([]byte, 8), []byte("DUCK-post-split-control")...)
	if err := os.WriteFile(evidencePath, source, 0o600); err != nil {
		t.Fatal(err)
	}
	sourceSHA, err := checksumFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	createPublicationWork(t, ctx, SQLiteWorkPath(path), sourceSHA)

	state, err := InspectPublication(ctx, path)
	if err != nil || state != PublicationCanComplete {
		t.Fatalf("state=%v err=%v", state, err)
	}
	if err := PublishLegacyConversion(ctx, path); err != nil {
		t.Fatal(err)
	}
	if format, _ := InspectFormat(path); format != FileSQLite {
		t.Fatalf("format=%v", format)
	}
}

func TestInspectPublicationRejectsChecksumConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "hitkeep.db")
	if err := os.WriteFile(PreSQLiteEvidencePath(path), append(make([]byte, 8), []byte("DUCK-evidence")...), 0o600); err != nil {
		t.Fatal(err)
	}
	createPublicationWork(t, ctx, SQLiteWorkPath(path), strings.Repeat("a", 64))
	if _, err := InspectPublication(ctx, path); err == nil {
		t.Fatal("checksum conflict was accepted")
	}
}

func TestInspectPublicationRejectsPublishedSQLiteWithChangedEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hitkeep.db")
	source := append(make([]byte, 8), []byte("DUCK-post-split-control")...)
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	sourceSHA, err := checksumFile(path)
	if err != nil {
		t.Fatal(err)
	}
	createPublicationWork(t, ctx, SQLiteWorkPath(path), sourceSHA)
	if err := PublishLegacyConversion(ctx, path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(PreSQLiteEvidencePath(path), append(source, byte('!')), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := InspectPublication(ctx, path); err == nil {
		t.Fatal("published SQLite was accepted with conflicting retained evidence")
	}
}

func TestInspectPublicationRebuildsMissingOrMalformedWorkFromRetainedEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, test := range []struct {
		name     string
		workBody []byte
	}{
		{name: "missing"},
		{name: "malformed", workBody: []byte("not sqlite")},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "hitkeep.db")
			evidencePath := PreSQLiteEvidencePath(path)
			if err := os.WriteFile(evidencePath, append(make([]byte, 8), []byte("DUCK-evidence")...), 0o600); err != nil {
				t.Fatal(err)
			}
			if test.workBody != nil {
				if err := os.WriteFile(SQLiteWorkPath(path), test.workBody, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			state, err := InspectPublication(ctx, path)
			if err != nil || state != PublicationNeedsRebuild {
				t.Fatalf("state=%v err=%v", state, err)
			}
			if err := ResetInvalidLegacyWork(ctx, path); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(SQLiteWorkPath(path)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid work file remains after reset: %v", err)
			}
			if _, err := os.Stat(evidencePath); err != nil {
				t.Fatalf("retained evidence was changed: %v", err)
			}
		})
	}
}

func TestInspectPublicationRebuildsMalformedWorkFromAuthoritativeDuckDBSource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hitkeep.db")
	source := append(make([]byte, 8), []byte("DUCK-authoritative-source")...)
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SQLiteWorkPath(path), []byte("partial sqlite write"), 0o600); err != nil {
		t.Fatal(err)
	}

	state, err := InspectPublication(ctx, path)
	if err != nil || state != PublicationNeedsWorkRebuild {
		t.Fatalf("state=%v err=%v", state, err)
	}
	if err := ResetInvalidLegacyWork(ctx, path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(SQLiteWorkPath(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid work file remains after reset: %v", err)
	}
	retained, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(retained) != string(source) {
		t.Fatal("authoritative DuckDB source changed during work reset")
	}
}

func createPublicationWork(t *testing.T, ctx context.Context, path, sourceSHA string) {
	t.Helper()
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO data_migrations(name, applied_at) VALUES
			('default_tenant_split_v1', ?),
			('default_tenant_split_compacted_v1', ?)
	`, time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO control_imports(name, source_sha256, source_schema_sha256, imported_at)
		VALUES (?, ?, ?, ?)
	`, DuckDBControlImportV1, sourceSHA, strings.Repeat("b", 64), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}
