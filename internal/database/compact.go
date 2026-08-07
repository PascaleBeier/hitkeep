package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

// CompactionOptions bound when MaybeCompactDatabase rewrites a database file.
// Both thresholds must be met.
type CompactionOptions struct {
	// MinReclaimableBytes is the least amount of free block space that makes
	// a rewrite worthwhile.
	MinReclaimableBytes int64
	// MinFreeRatio is the least share of free blocks (0..1) in the file.
	MinFreeRatio float64
	// MemoryLimit and Threads optionally constrain rewrite workers.
	MemoryLimit string
	Threads     int
}

// DefaultCompactionOptions skip small or barely fragmented files: rewriting
// costs a full copy of the database, so it must pay for itself.
func DefaultCompactionOptions() CompactionOptions {
	return CompactionOptions{
		MinReclaimableBytes: 64 << 20, // 64 MiB
		MinFreeRatio:        0.25,
	}
}

// CompactionResult reports what MaybeCompactDatabase measured and did.
type CompactionResult struct {
	Compacted        bool
	BytesBefore      int64
	BytesAfter       int64
	ReclaimableBytes int64
	FreeRatio        float64
}

// SchemaPreparer applies the embedded migrations for one schema flavor to a
// freshly created database.
type SchemaPreparer func(context.Context, *Store) error

// PrepareSharedSchema migrates a database with the shared control-plane schema.
func PrepareSharedSchema(ctx context.Context, s *Store) error { return s.Migrate(ctx) }

// PrepareTenantSchema migrates a database with the tenant analytics schema.
func PrepareTenantSchema(ctx context.Context, s *Store) error { return s.MigrateTenant(ctx) }

// MaybeCompactDatabase rewrites the fully migrated DuckDB file at path into a
// fresh file and swaps it in when the share of free blocks exceeds the
// thresholds. DELETE only marks rows and CHECKPOINT reuses blocks within the
// file, but DuckDB never shrinks the file itself — retention trims leave the
// space allocated until a rewrite. The file must not be open anywhere;
// DuckDB's file lock makes a concurrent open fail loudly rather than corrupt.
//
// The fresh file's schema comes from replaying the embedded migrations
// (prepare), then data is copied table by table in foreign-key order —
// DuckDB's own COPY FROM DATABASE copies in an order that violates foreign
// keys.
func MaybeCompactDatabase(ctx context.Context, path string, opts CompactionOptions, prepare SchemaPreparer) (CompactionResult, error) {
	var result CompactionResult
	if path == "" || path == ":memory:" {
		return result, nil
	}
	if err := recoverCompactionSwap(path); err != nil {
		return result, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("stat database %s: %w", path, err)
	}
	result.BytesBefore = info.Size()

	workPath := path + ".compacting"
	backupPath := path + preCompactBackupSuffix
	for _, stale := range []string{workPath, workPath + ".wal", backupPath} {
		if err := os.Remove(stale); err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, fmt.Errorf("remove stale compaction artifact %s: %w", stale, err)
		}
	}

	reclaimable, freeRatio, err := measureReclaimableSpace(ctx, path, opts)
	if err != nil {
		return result, err
	}
	result.ReclaimableBytes = reclaimable
	result.FreeRatio = freeRatio
	if reclaimable < opts.MinReclaimableBytes || freeRatio < opts.MinFreeRatio {
		return result, nil
	}

	if err := rewriteDatabaseFile(ctx, path, workPath, prepare, opts); err != nil {
		_ = os.Remove(workPath)
		_ = os.Remove(workPath + ".wal")
		return result, err
	}

	if err := os.Rename(path, backupPath); err != nil {
		_ = os.Remove(workPath)
		return result, fmt.Errorf("move database aside for compaction swap: %w", err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return result, fmt.Errorf("sync database directory after moving database aside: %w", err)
	}
	if err := os.Rename(workPath, path); err != nil {
		// Best-effort restore: the original file is still intact under backupPath.
		if restoreErr := os.Rename(backupPath, path); restoreErr != nil {
			return result, fmt.Errorf("swap compacted database: %w (RESTORE FAILED, database left at %s: %v)", err, backupPath, restoreErr)
		}
		_ = os.Remove(workPath)
		return result, fmt.Errorf("swap compacted database: %w", err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return result, fmt.Errorf("sync database directory after publishing compacted database: %w", err)
	}
	if err := os.Remove(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("Compaction succeeded but the pre-compaction backup could not be removed", "path", backupPath, "error", err)
	}
	// A stale WAL from the old file must not be replayed into the new one.
	if err := os.Remove(path + ".wal"); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("Could not remove stale WAL after compaction", "path", path+".wal", "error", err)
	}

	after, err := os.Stat(path)
	if err != nil {
		return result, fmt.Errorf("stat compacted database %s: %w", path, err)
	}
	result.BytesAfter = after.Size()
	result.Compacted = true
	return result, nil
}

// recoverCompactionSwap repairs the only non-atomic window in the file swap.
// If a process dies after moving the live file aside but before publishing the
// replacement, restore the original before any caller can create a new empty
// database at the live path. A published replacement wins over a leftover
// backup; the backup is then safe to remove.
func recoverCompactionSwap(path string) error {
	backupPath := path + preCompactBackupSuffix
	workPath := path + ".compacting"
	_, pathErr := os.Stat(path)
	_, backupErr := os.Stat(backupPath)
	pathExists := pathErr == nil
	backupExists := backupErr == nil
	if pathErr != nil && !errors.Is(pathErr, os.ErrNotExist) {
		return fmt.Errorf("stat database during compaction recovery %s: %w", path, pathErr)
	}
	if backupErr != nil && !errors.Is(backupErr, os.ErrNotExist) {
		return fmt.Errorf("stat compaction backup %s: %w", backupPath, backupErr)
	}
	if !pathExists && backupExists {
		if err := os.Rename(backupPath, path); err != nil {
			return fmt.Errorf("restore database after interrupted compaction: %w", err)
		}
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return fmt.Errorf("sync database directory after compaction recovery: %w", err)
		}
		if err := os.Remove(workPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove incomplete compaction work file: %w", err)
		}
		_ = os.Remove(workPath + ".wal")
		return nil
	}
	if pathExists && backupExists {
		if err := os.Remove(backupPath); err != nil {
			return fmt.Errorf("remove completed compaction backup: %w", err)
		}
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return fmt.Errorf("sync database directory after compaction cleanup: %w", err)
		}
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// measureReclaimableSpace opens the database briefly (replaying any WAL) and
// reads the free/total block counts.
func measureReclaimableSpace(ctx context.Context, path string, opts CompactionOptions) (int64, float64, error) {
	db, err := openDuckDBFile(path)
	if err != nil {
		return 0, 0, fmt.Errorf("open database for compaction measurement: %w", err)
	}
	defer db.Close()
	if err := configureCompactionDB(ctx, db, opts); err != nil {
		return 0, 0, err
	}

	var blockSize, totalBlocks, freeBlocks int64
	if err := db.QueryRowContext(ctx, `
		SELECT block_size, total_blocks, free_blocks
		FROM pragma_database_size()
		LIMIT 1
	`).Scan(&blockSize, &totalBlocks, &freeBlocks); err != nil {
		return 0, 0, fmt.Errorf("measure database size: %w", err)
	}
	if totalBlocks <= 0 {
		return 0, 0, nil
	}
	return freeBlocks * blockSize, float64(freeBlocks) / float64(totalBlocks), nil
}

// rewriteDatabaseFile creates target with the current embedded schema and
// copies every table's rows from source in foreign-key order.
func rewriteDatabaseFile(ctx context.Context, source, target string, prepare SchemaPreparer, opts CompactionOptions) error {
	targetStore := NewStore(target, compactionStoreOptions(opts)...)
	if err := targetStore.Connect(); err != nil {
		return fmt.Errorf("create compaction target: %w", err)
	}
	if err := prepare(ctx, targetStore); err != nil {
		_ = targetStore.Close()
		return fmt.Errorf("prepare compaction target schema: %w", err)
	}
	if err := targetStore.Close(); err != nil {
		return fmt.Errorf("close compaction target after schema setup: %w", err)
	}

	worker, err := openDuckDBFile(":memory:")
	if err != nil {
		return fmt.Errorf("open compaction worker: %w", err)
	}
	defer worker.Close()
	if err := configureCompactionDB(ctx, worker, opts); err != nil {
		return err
	}

	if _, err := worker.ExecContext(ctx, fmt.Sprintf("ATTACH '%s' AS compact_source (READ_ONLY);", escapeSQLString(source))); err != nil {
		return fmt.Errorf("attach compaction source: %w", err)
	}
	if _, err := worker.ExecContext(ctx, fmt.Sprintf("ATTACH '%s' AS compact_target;", escapeSQLString(target))); err != nil {
		return fmt.Errorf("attach compaction target: %w", err)
	}

	// The source must already be on the current schema, or column sets would
	// diverge from the freshly migrated target.
	var missingMigrations int
	if err := worker.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM compact_target.migrations t
		WHERE t.migration NOT IN (SELECT migration FROM compact_source.migrations)
	`).Scan(&missingMigrations); err != nil {
		return fmt.Errorf("compare migration state for compaction: %w", err)
	}
	if missingMigrations > 0 {
		return fmt.Errorf("database %s is not fully migrated (%d pending); run migrations before compacting", source, missingMigrations)
	}

	tables, err := listCatalogTables(ctx, worker, "compact_source")
	if err != nil {
		return err
	}
	targetTables, err := listCatalogTables(ctx, worker, "compact_target")
	if err != nil {
		return err
	}

	members := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		if table == "migrations" {
			continue // recreated by the schema preparer
		}
		if !slices.Contains(targetTables, table) {
			return fmt.Errorf("compaction target is missing table %s present in the source", table)
		}
		members[table] = struct{}{}
	}

	edges, err := listCatalogFKEdges(ctx, worker, "compact_target")
	if err != nil {
		return err
	}
	childrenFirst, err := orderChildrenFirst(members, edges)
	if err != nil {
		return err
	}
	// Freshly migrated schemas may contain bootstrap rows (the default tenant
	// is one example). Clear every source-backed target table child-first so
	// the rewrite is an exact physical copy rather than an append over seeds.
	for _, table := range childrenFirst {
		if _, err := worker.ExecContext(ctx, fmt.Sprintf("DELETE FROM compact_target.%s", table)); err != nil {
			return fmt.Errorf("clear compaction target table %s: %w", table, err)
		}
	}
	slices.Reverse(childrenFirst) // parents before their foreign-key children

	for _, table := range childrenFirst {
		columns, err := listCatalogColumns(ctx, worker, "compact_target", table)
		if err != nil {
			return err
		}
		sourceColumns, err := listCatalogColumns(ctx, worker, "compact_source", table)
		if err != nil {
			return err
		}
		slicesEqual := len(columns) == len(sourceColumns)
		if slicesEqual {
			sorted := slices.Clone(columns)
			sortedSource := slices.Clone(sourceColumns)
			slices.Sort(sorted)
			slices.Sort(sortedSource)
			slicesEqual = slices.Equal(sorted, sortedSource)
		}
		if !slicesEqual {
			return fmt.Errorf("table %s has diverging columns between source and compaction target", table)
		}
		columnList := strings.Join(columns, ", ")
		// #nosec G201 -- identifiers come from the live catalogs and pass isSafeIdentifier in listCatalog helpers.
		query := fmt.Sprintf("INSERT INTO compact_target.%s (%s) SELECT %s FROM compact_source.%s", table, columnList, columnList, table)
		if _, err := worker.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("copy table %s during compaction: %w", table, err)
		}
	}

	for _, detach := range []string{"DETACH compact_source;", "DETACH compact_target;"} {
		if _, err := worker.ExecContext(ctx, detach); err != nil {
			return fmt.Errorf("detach compaction database: %w", err)
		}
	}
	return nil
}

func compactionStoreOptions(opts CompactionOptions) []StoreOption {
	var storeOpts []StoreOption
	if strings.TrimSpace(opts.MemoryLimit) != "" {
		storeOpts = append(storeOpts, WithMemoryLimit(opts.MemoryLimit))
	}
	if opts.Threads > 0 {
		storeOpts = append(storeOpts, WithThreads(opts.Threads))
	}
	return storeOpts
}

func configureCompactionDB(ctx context.Context, db *sql.DB, opts CompactionOptions) error {
	// Compaction workers are opened directly rather than through Store, so
	// apply the same bounded-result and allocator-flushing settings here. A
	// rewrite otherwise retains freed native blocks until the worker exits,
	// which can push a large control-file cleanup well above its configured
	// memory budget.
	if _, err := db.ExecContext(ctx, "SET preserve_insertion_order=false; SET allocator_flush_threshold='64MB'; SET allocator_bulk_deallocation_flush_threshold='128MB';"); err != nil {
		return fmt.Errorf("configure compaction allocator: %w", err)
	}
	if strings.TrimSpace(opts.MemoryLimit) != "" {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("SET memory_limit='%s';", escapeSQLString(opts.MemoryLimit))); err != nil {
			return fmt.Errorf("set compaction memory limit: %w", err)
		}
	}
	if opts.Threads > 0 {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("SET threads=%d;", opts.Threads)); err != nil {
			return fmt.Errorf("set compaction threads: %w", err)
		}
	}
	return nil
}

func listCatalogTables(ctx context.Context, db *sql.DB, catalog string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name FROM duckdb_tables()
		WHERE database_name = ? AND NOT internal
	`, catalog)
	if err != nil {
		return nil, fmt.Errorf("list tables of %s: %w", catalog, err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("scan table of %s: %w", catalog, err)
		}
		if !isSafeIdentifier(table) {
			return nil, fmt.Errorf("unsafe table name %q in %s", table, catalog)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tables of %s: %w", catalog, err)
	}
	return tables, nil
}

func listCatalogColumns(ctx context.Context, db *sql.DB, catalog, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT column_name FROM duckdb_columns()
		WHERE database_name = ? AND table_name = ?
		ORDER BY column_index
	`, catalog, table)
	if err != nil {
		return nil, fmt.Errorf("list columns of %s.%s: %w", catalog, table, err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, fmt.Errorf("scan column of %s.%s: %w", catalog, table, err)
		}
		if !isSafeIdentifier(column) {
			return nil, fmt.Errorf("unsafe column name %q in %s.%s", column, catalog, table)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read columns of %s.%s: %w", catalog, table, err)
	}
	return columns, nil
}

// listCatalogFKEdges is listFKEdges scoped to one attached catalog; the
// compaction worker has several databases attached at once.
func listCatalogFKEdges(ctx context.Context, db *sql.DB, catalog string) ([]fkEdge, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name, constraint_column_names, referenced_table, referenced_column_names
		FROM duckdb_constraints()
		WHERE constraint_type = 'FOREIGN KEY' AND database_name = ?
	`, catalog)
	if err != nil {
		return nil, fmt.Errorf("list foreign keys of %s: %w", catalog, err)
	}
	defer rows.Close()

	var edges []fkEdge
	for rows.Next() {
		var table, referencedTable string
		var columns, referencedColumns any
		if err := rows.Scan(&table, &columns, &referencedTable, &referencedColumns); err != nil {
			return nil, fmt.Errorf("scan foreign key of %s: %w", catalog, err)
		}
		column, okColumn := singleListElement(columns)
		referencedColumn, okReferenced := singleListElement(referencedColumns)
		if !okColumn || !okReferenced {
			return nil, fmt.Errorf("unsupported multi-column foreign key on table %s", table)
		}
		edges = append(edges, fkEdge{table: table, column: column, referencedTable: referencedTable, referencedColumn: referencedColumn})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read foreign keys of %s: %w", catalog, err)
	}
	return edges, nil
}

func openDuckDBFile(path string) (*sql.DB, error) {
	connector, err := duckdb.NewConnector(path, nil)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(connector), nil
}
