package controlstore

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeLegacySource struct {
	tables  []string
	columns map[string][]string
	types   map[string][]string
	rows    map[string][][]any
	counts  map[string]int64
}

func (f *fakeLegacySource) BaseTables(context.Context) ([]string, error) {
	return append([]string(nil), f.tables...), nil
}

func (f *fakeLegacySource) Columns(_ context.Context, table string) ([]LegacyColumn, error) {
	columns, ok := f.columns[table]
	if !ok {
		return nil, errors.New("missing fake columns")
	}
	result := make([]LegacyColumn, len(columns))
	for i, name := range columns {
		result[i] = LegacyColumn{Name: name}
		if i < len(f.types[table]) {
			result[i].LogicalType = f.types[table][i]
		}
	}
	return result, nil
}

func (f *fakeLegacySource) Rows(_ context.Context, table string, _ []LegacyColumn) (LegacyRows, error) {
	return &fakeLegacyRows{rows: f.rows[table]}, nil
}

func (f *fakeLegacySource) RowCount(_ context.Context, table string) (int64, error) {
	if count, ok := f.counts[table]; ok {
		return count, nil
	}
	return int64(len(f.rows[table])), nil
}

type fakeLegacyRows struct {
	rows  [][]any
	index int
}

func TestCanonicalColumnValueConvertsDuckDBUUIDBytesToTextWithoutChangingBlobs(t *testing.T) {
	id := uuid.MustParse("7a869d3b-08cf-46d1-a194-3fb89db02a2f")
	normalized, tag, _, err := canonicalColumnValue(id[:], LegacyColumn{Name: "id", LogicalType: "UUID"})
	if err != nil {
		t.Fatal(err)
	}
	if normalized != id.String() || tag != 's' {
		t.Fatalf("UUID normalization = %#v/%q, want canonical text", normalized, tag)
	}

	blob := []byte{0x7a, 0x86, 0x9d, 0x3b}
	normalized, tag, _, err = canonicalColumnValue(blob, LegacyColumn{Name: "credential", LogicalType: "BLOB"})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := normalized.([]byte)
	if !ok || tag != 'x' || string(got) != string(blob) {
		t.Fatalf("BLOB normalization = %#v/%q, want unchanged bytes", normalized, tag)
	}

	normalized, tag, _, err = canonicalColumnValue([]byte{}, LegacyColumn{Name: "data", LogicalType: "BLOB"})
	if err != nil {
		t.Fatal(err)
	}
	empty, ok := normalized.([]byte)
	if !ok || empty == nil || len(empty) != 0 || tag != 'x' {
		t.Fatalf("empty BLOB normalization = %#v/%q, want a non-nil empty blob", normalized, tag)
	}
}

func (r *fakeLegacyRows) Next() bool { return r.index < len(r.rows) }

func (r *fakeLegacyRows) Scan(dest ...any) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.index]
	if len(row) != len(dest) {
		return errors.New("fake row width mismatch")
	}
	for i := range row {
		pointer, ok := dest[i].(*any)
		if !ok {
			return errors.New("fake destination is not *any")
		}
		*pointer = row[i]
	}
	r.index++
	return nil
}

func (r *fakeLegacyRows) Err() error   { return nil }
func (r *fakeLegacyRows) Close() error { return nil }

func TestImportLegacyCopiesAndVerifiesClosedRegistry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "template.db")
	template, err := Open(ctx, templatePath)
	if err != nil {
		t.Fatal(err)
	}
	columns := make(map[string][]string, len(legacyControlTables))
	tx, err := template.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range legacyControlTables {
		columns[table], err = sqliteTableColumns(ctx, tx, table)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := template.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(templatePath); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 27, 12, 34, 56, 123456789, time.FixedZone("fixture", 2*60*60))
	source := &fakeLegacySource{
		tables:  append(append(LegacyControlTables(), legacyEmptyAnalyticsTables...), legacyReplacedMetadataTables...),
		columns: columns,
		rows: map[string][][]any{
			"data_migrations": {
				{"default_tenant_split_v1", now},
				{"default_tenant_split_compacted_v1", now},
			},
			"users": {
				{"018f4f88-1c80-7000-8000-000000000001", "unicode+例@example.test", "hash", now, nil, "Lovelace", true},
			},
		},
		counts: map[string]int64{},
	}

	workPath := filepath.Join(dir, "hitkeep.db.sqlite-work")
	sourceDigest := strings.Repeat("0", 64)
	result, err := ImportLegacy(ctx, workPath, source, sourceDigest, strings.Repeat("1", 64))
	if err != nil {
		t.Fatal(err)
	}
	if result.Tables["users"].Rows != 1 || result.Tables["data_migrations"].Rows != 2 {
		t.Fatalf("unexpected import result: %+v", result.Tables)
	}

	target, err := Open(ctx, workPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })
	var email, importedSource string
	if err := target.db.QueryRowContext(ctx, "SELECT email FROM users").Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email != "unicode+例@example.test" {
		t.Fatalf("email=%q", email)
	}
	if err := target.db.QueryRowContext(ctx, "SELECT source_sha256 FROM control_imports WHERE name = ?", DuckDBControlImportV1).Scan(&importedSource); err != nil {
		t.Fatal(err)
	}
	if importedSource != sourceDigest {
		t.Fatalf("source digest=%q", importedSource)
	}
}

func TestCatalogFingerprintIsStableAndDetectsControlChanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	before, err := store.CatalogFingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := store.CatalogFingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before["users"] != repeated["users"] {
		t.Fatal("unchanged control fingerprint is unstable")
	}
	if _, err := store.CreateUser(ctx, "fingerprint@example.test", "hash"); err != nil {
		t.Fatal(err)
	}
	after, err := store.CatalogFingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before["users"] == after["users"] {
		t.Fatal("control mutation did not change users fingerprint")
	}
}

func TestImportLegacyRejectsNonEmptyCompatibilityTableBeforeCreatingWorkFile(t *testing.T) {
	t.Parallel()
	source := &fakeLegacySource{
		tables: append(append(LegacyControlTables(), legacyEmptyAnalyticsTables...), legacyReplacedMetadataTables...),
		counts: map[string]int64{"hits": 1},
	}
	workPath := filepath.Join(t.TempDir(), "hitkeep.db.sqlite-work")
	_, err := ImportLegacy(context.Background(), workPath, source, strings.Repeat("0", 64), strings.Repeat("1", 64))
	if err == nil {
		t.Fatal("ImportLegacy succeeded")
	}
	if _, statErr := os.Stat(workPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("work file exists after preflight: %v", statErr)
	}
}
