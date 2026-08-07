package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// RetentionArchiveImportSummary reports what an archive import inserted per
// table and how many source rows it left out because they were already
// present or referenced a missing parent row.
type RetentionArchiveImportSummary struct {
	Files    int
	Imported map[string]int64
	Skipped  map[string]int64
}

// DiscoverLocalRetentionArchives returns the retention parquet exports for
// one tenant under a local archive path. It covers both layouts the
// retention worker has written locally: the flat default-tenant layout at
// the archive root and the nested tenants/<id>/sites/<id> layout.
func DiscoverLocalRetentionArchives(archivePath string, tenantID uuid.UUID, includeFlat bool) ([]string, error) {
	nested, err := filepath.Glob(filepath.Join(archivePath, "tenants", tenantID.String(), "sites", "*", "site_*.parquet"))
	if err != nil {
		return nil, fmt.Errorf("discover nested retention archives: %w", err)
	}
	files := nested
	if includeFlat {
		flat, err := filepath.Glob(filepath.Join(archivePath, "site_*.parquet"))
		if err != nil {
			return nil, fmt.Errorf("discover flat retention archives: %w", err)
		}
		files = append(files, flat...)
	}
	sort.Strings(files)
	return files, nil
}

// ImportRetentionArchives inserts retention archive parquet exports back
// into an existing tenant database file. Imports are idempotent: rows that
// already exist are not duplicated, and rows whose parent row (for example a
// deleted site or goal) no longer exists are skipped instead of violating
// foreign keys. HitKeep must be stopped: DuckDB allows only one writer.
func ImportRetentionArchives(ctx context.Context, tenantPath string, files []string, opts ...StoreOption) (RetentionArchiveImportSummary, error) {
	summary := RetentionArchiveImportSummary{Imported: map[string]int64{}, Skipped: map[string]int64{}}
	if len(files) == 0 {
		return summary, errors.New("no retention archive files to import")
	}
	if _, err := os.Stat(tenantPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return summary, fmt.Errorf("tenant database %s does not exist; restore it or run 'hitkeep recover rebuild-default-tenant' first", tenantPath)
		}
		return summary, fmt.Errorf("stat tenant database %s: %w", tenantPath, err)
	}

	store := NewStore(tenantPath, opts...)
	if err := store.Connect(); err != nil {
		return summary, fmt.Errorf("open tenant database for archive import: %w", err)
	}
	defer store.Close()
	if err := store.migrateTenant(ctx, migrationRunOptions{guarded: true}); err != nil {
		return summary, fmt.Errorf("prepare tenant schema for archive import: %w", err)
	}
	db := store.DB()
	var catalog string
	if err := db.QueryRowContext(ctx, "SELECT current_database()").Scan(&catalog); err != nil {
		return summary, fmt.Errorf("resolve tenant catalog for archive import: %w", err)
	}
	tables, err := listCatalogTables(ctx, db, catalog)
	if err != nil {
		return summary, err
	}
	edges, err := listCatalogFKEdges(ctx, db, catalog)
	if err != nil {
		return summary, err
	}

	for _, file := range files {
		if err := importRetentionArchiveFile(ctx, db, catalog, file, tables, edges, &summary); err != nil {
			return summary, fmt.Errorf("import retention archive %s: %w", file, err)
		}
		summary.Files++
	}
	if _, err := db.ExecContext(ctx, "CHECKPOINT;"); err != nil {
		return summary, fmt.Errorf("checkpoint tenant database after archive import: %w", err)
	}
	return summary, nil
}

func importRetentionArchiveFile(ctx context.Context, db *sql.DB, catalog, file string, catalogTables []string, edges []fkEdge, summary *RetentionArchiveImportSummary) error {
	parquetColumns, err := describeParquetColumns(ctx, db, file)
	if err != nil {
		return err
	}
	if !slices.Contains(parquetColumns, "_source") {
		return errors.New("file has no _source column; not a retention archive export")
	}
	sources, err := listArchiveSources(ctx, db, file)
	if err != nil {
		return err
	}
	members := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		if slices.Contains(catalogTables, source) {
			members[source] = struct{}{}
		}
	}
	ordered, err := orderChildrenFirst(members, edges)
	if err != nil {
		return err
	}
	// orderChildrenFirst returns children before parents; import parents first.
	slices.Reverse(ordered)

	for _, table := range ordered {
		columns, err := listCatalogColumnTypes(ctx, db, catalog, table)
		if err != nil {
			return err
		}
		if err := importArchiveTable(ctx, db, file, table, columns, parquetColumns, edges, summary); err != nil {
			return fmt.Errorf("import table %s: %w", table, err)
		}
	}
	return nil
}

func importArchiveTable(ctx context.Context, db *sql.DB, file, table string, columns []catalogColumnType, parquetColumns []string, edges []fkEdge, summary *RetentionArchiveImportSummary) error {
	types := make(map[string]string, len(columns))
	copyColumns := make([]catalogColumnType, 0, len(columns))
	for _, column := range columns {
		types[column.Name] = column.DataType
		if slices.Contains(parquetColumns, column.Name) {
			copyColumns = append(copyColumns, column)
		}
	}
	if len(copyColumns) == 0 || types["site_id"] == "" || !slices.Contains(parquetColumns, "site_id") {
		return errors.New("archive and tenant schema share no site-scoped columns")
	}

	names := make([]string, len(copyColumns))
	selects := make([]string, len(copyColumns))
	for i, column := range copyColumns {
		quoted := quoteDuckDBIdentifier(column.Name)
		names[i] = quoted
		selects[i] = fmt.Sprintf("CAST(src.%s AS %s) AS %s", quoted, column.DataType, quoted)
	}

	// Guard every foreign key whose column travels with the archive so rows
	// pointing at deleted parents are skipped instead of failing the import.
	guards := []string{fmt.Sprintf("src._source = '%s'", table)}
	for _, edge := range edges {
		if edge.table != table || edge.referencedTable == table {
			continue
		}
		columnType, ok := types[edge.column]
		if !ok || !slices.Contains(parquetColumns, edge.column) {
			continue
		}
		// #nosec G201 -- identifiers come from validated catalog metadata.
		guards = append(guards, fmt.Sprintf(
			"(src.%s IS NULL OR EXISTS (SELECT 1 FROM %s parent WHERE parent.%s = CAST(src.%s AS %s)))",
			quoteDuckDBIdentifier(edge.column),
			quoteDuckDBIdentifier(edge.referencedTable),
			quoteDuckDBIdentifier(edge.referencedColumn),
			quoteDuckDBIdentifier(edge.column),
			columnType,
		))
	}
	// Rows for sites this tenant no longer owns are always out of scope,
	// with or without a declared foreign key.
	guards = append(guards, fmt.Sprintf(
		"EXISTS (SELECT 1 FROM sites scope WHERE scope.id = CAST(src.site_id AS %s))", types["site_id"],
	))

	var total int64
	// #nosec G201 -- the table literal comes from the tenant catalog and the path is escaped.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM read_parquet('%s') src WHERE src._source = '%s'", escapeSQLString(file), table)
	if err := db.QueryRowContext(ctx, countQuery).Scan(&total); err != nil {
		return err
	}
	if total == 0 {
		return nil
	}

	columnList := strings.Join(names, ", ")
	// EXCEPT ALL keeps the import idempotent without assuming primary keys.
	// #nosec G201 -- identifiers come from validated catalog metadata and the path is escaped.
	insertQuery := fmt.Sprintf(`
		INSERT INTO %s (%s)
		SELECT * FROM (
			SELECT %s FROM read_parquet('%s') src WHERE %s
			EXCEPT ALL
			SELECT %s FROM %s
		)`,
		quoteDuckDBIdentifier(table), columnList,
		strings.Join(selects, ", "), escapeSQLString(file), strings.Join(guards, " AND "),
		columnList, quoteDuckDBIdentifier(table),
	)
	result, err := db.ExecContext(ctx, insertQuery)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	summary.Imported[table] += inserted
	summary.Skipped[table] += total - inserted
	return nil
}

type catalogColumnType struct {
	Name     string
	DataType string
}

func listCatalogColumnTypes(ctx context.Context, db *sql.DB, catalog, table string) ([]catalogColumnType, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type FROM duckdb_columns()
		WHERE database_name = ? AND table_name = ?
		ORDER BY column_index
	`, catalog, table)
	if err != nil {
		return nil, fmt.Errorf("list column types of %s.%s: %w", catalog, table, err)
	}
	defer rows.Close()
	var columns []catalogColumnType
	for rows.Next() {
		var column catalogColumnType
		if err := rows.Scan(&column.Name, &column.DataType); err != nil {
			return nil, fmt.Errorf("scan column type of %s.%s: %w", catalog, table, err)
		}
		if !isSafeIdentifier(column.Name) {
			return nil, fmt.Errorf("unsafe column name %q in %s.%s", column.Name, catalog, table)
		}
		columns = append(columns, column)
	}
	return columns, rows.Err()
}

func describeParquetColumns(ctx context.Context, db *sql.DB, file string) ([]string, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT column_name FROM (DESCRIBE SELECT * FROM read_parquet('%s'))", escapeSQLString(file)))
	if err != nil {
		return nil, fmt.Errorf("describe archive columns: %w", err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

func listArchiveSources(ctx context.Context, db *sql.DB, file string) ([]string, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT DISTINCT _source FROM read_parquet('%s') ORDER BY _source", escapeSQLString(file)))
	if err != nil {
		return nil, fmt.Errorf("list archive source tables: %w", err)
	}
	defer rows.Close()
	var sources []string
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}
