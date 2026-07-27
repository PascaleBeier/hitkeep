package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"hitkeep/internal/controlstore"
)

const legacyControlCatalog = "legacy_control"

// LegacyControlSource is a read-only adapter between the frozen DuckDB
// control schema and the pure-Go SQLite importer.
type LegacyControlSource struct {
	db *sql.DB
}

type legacyControlRows struct {
	rows       *sql.Rows
	columns    []controlstore.LegacyColumn
	queryWidth int
}

var _ controlstore.LegacySource = (*LegacyControlSource)(nil)

// OpenLegacyControlSource attaches path read-only to an in-memory DuckDB
// worker. The source file is never opened as the worker's writable root.
func OpenLegacyControlSource(ctx context.Context, path string) (*LegacyControlSource, string, string, error) {
	fileSHA, err := sha256File(path)
	if err != nil {
		return nil, "", "", err
	}
	worker, err := openDuckDBFile(":memory:")
	if err != nil {
		return nil, "", "", fmt.Errorf("open legacy control import worker: %w", err)
	}
	attach := fmt.Sprintf("ATTACH '%s' AS %s (READ_ONLY)", escapeSQLString(path), quoteDuckDBIdentifier(legacyControlCatalog))
	if _, err := worker.ExecContext(ctx, attach); err != nil {
		_ = worker.Close()
		return nil, "", "", fmt.Errorf("attach legacy DuckDB control source read-only: %w", err)
	}
	worker.SetMaxOpenConns(1)
	worker.SetMaxIdleConns(1)

	source := &LegacyControlSource{db: worker}
	schemaSHA, err := source.schemaDigest(ctx)
	if err != nil {
		_ = source.Close()
		return nil, "", "", err
	}
	return source, fileSHA, schemaSHA, nil
}

func (s *LegacyControlSource) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	_, detachErr := s.db.ExecContext(context.Background(), "DETACH "+quoteDuckDBIdentifier(legacyControlCatalog))
	closeErr := s.db.Close()
	if detachErr != nil {
		return fmt.Errorf("detach legacy DuckDB control source: %w", detachErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close legacy DuckDB control worker: %w", closeErr)
	}
	return nil
}

func (s *LegacyControlSource) BaseTables(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT table_name
		FROM duckdb_tables()
		WHERE database_name = ? AND NOT internal
		ORDER BY table_name
	`, legacyControlCatalog)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

func (s *LegacyControlSource) Columns(ctx context.Context, table string) ([]controlstore.LegacyColumn, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_catalog = ? AND table_schema = 'main' AND table_name = ?
		ORDER BY ordinal_position
	`, legacyControlCatalog, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var columns []controlstore.LegacyColumn
	for rows.Next() {
		var column controlstore.LegacyColumn
		if err := rows.Scan(&column.Name, &column.LogicalType); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("legacy DuckDB table %s has no columns", table)
	}
	return columns, nil
}

func (s *LegacyControlSource) Rows(ctx context.Context, table string, columns []controlstore.LegacyColumn) (controlstore.LegacyRows, error) {
	selected := make([]string, 0, len(columns)+1)
	for _, column := range columns {
		quoted := quoteDuckDBIdentifier(column.Name)
		selected = append(selected, quoted)
		if strings.EqualFold(strings.TrimSpace(column.LogicalType), "BLOB") {
			// The DuckDB driver scans both NULL and empty BLOBs into an empty
			// byte slice when the destination is *any. Carry an explicit null
			// bit so the cross-engine fingerprint and copy remain lossless.
			selected = append(selected, quoted+" IS NULL")
		}
	}
	// Catalog, table, and columns are schema-discovered and passed through the
	// DuckDB identifier quoter before interpolation.
	query := "SELECT " + strings.Join(selected, ",") + " FROM " + quoteDuckDBIdentifier(legacyControlCatalog) + ".main." + quoteDuckDBIdentifier(table) //nolint:gosec
	// legacyControlRows exposes Rows.Err to the importer, which checks it after iteration.
	rows, err := s.db.QueryContext(ctx, query) //nolint:rowserrcheck
	if err != nil {
		return nil, err
	}
	return &legacyControlRows{rows: rows, columns: columns, queryWidth: len(selected)}, nil
}

func (r *legacyControlRows) Next() bool   { return r.rows.Next() }
func (r *legacyControlRows) Err() error   { return r.rows.Err() }
func (r *legacyControlRows) Close() error { return r.rows.Close() }

func (r *legacyControlRows) Scan(dest ...any) error {
	if len(dest) != len(r.columns) {
		return fmt.Errorf("legacy control row destination width %d does not match column width %d", len(dest), len(r.columns))
	}
	values := make([]any, r.queryWidth)
	queryDest := make([]any, r.queryWidth)
	for i := range values {
		queryDest[i] = &values[i]
	}
	if err := r.rows.Scan(queryDest...); err != nil {
		return err
	}
	queryIndex := 0
	for i, column := range r.columns {
		value := values[queryIndex]
		queryIndex++
		if strings.EqualFold(strings.TrimSpace(column.LogicalType), "BLOB") {
			isNull, ok := values[queryIndex].(bool)
			if !ok {
				return fmt.Errorf("legacy control BLOB null marker for %s is %T", column.Name, values[queryIndex])
			}
			queryIndex++
			if isNull {
				value = nil
			}
		}
		pointer, ok := dest[i].(*any)
		if !ok {
			return fmt.Errorf("legacy control destination %d is %T, expected *any", i, dest[i])
		}
		*pointer = value
	}
	return nil
}

func (s *LegacyControlSource) RowCount(ctx context.Context, table string) (int64, error) {
	query := "SELECT count(*) FROM " + quoteDuckDBIdentifier(legacyControlCatalog) + ".main." + quoteDuckDBIdentifier(table)
	var count int64
	if err := s.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *LegacyControlSource) schemaDigest(ctx context.Context) (string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT table_name, sql
		FROM duckdb_tables()
		WHERE database_name = ? AND NOT internal
		ORDER BY table_name
	`, legacyControlCatalog)
	if err != nil {
		return "", fmt.Errorf("read legacy DuckDB schema: %w", err)
	}
	defer rows.Close()
	h := sha256.New()
	for rows.Next() {
		var table, definition string
		if err := rows.Scan(&table, &definition); err != nil {
			return "", fmt.Errorf("scan legacy DuckDB schema: %w", err)
		}
		_, _ = io.WriteString(h, table)
		h.Write([]byte{0})
		_, _ = io.WriteString(h, definition)
		h.Write([]byte{0})
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate legacy DuckDB schema: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open legacy DuckDB control source for checksum: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("checksum legacy DuckDB control source: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
