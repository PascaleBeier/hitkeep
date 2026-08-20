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
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/database/migrations"
	"hitkeep/internal/hklog"
)

const (
	defaultTenantSplitMarker    = "default_tenant_split_v1"
	defaultTenantCompactionMark = "default_tenant_split_compacted_v1"
	defaultTenantSplitHeadroom  = int64(512 << 20)
	preCompactBackupSuffix      = ".pre-compact"
)

type splitTableFingerprint struct {
	Count   int64
	HashSum string
	HashXOR string
}

// RunDefaultTenantSplit moves the default tenant's site-scoped analytics out
// of the shared database. The shared database must be closed by the caller.
// The operation is restartable: the first marker makes the published tenant
// file authoritative, while the second marker records physical cleanup of the
// shared file.
func RunDefaultTenantSplit(ctx context.Context, sharedPath, dataPath string, opts ...StoreOption) error {
	return runDefaultTenantSplitWithOptions(ctx, sharedPath, dataPath, opts, opts)
}

// RunDefaultTenantSplitWithBudgets separates the temporary control-source
// budget from the tenant writer budget. It is primarily used by release
// acceptance harnesses that enforce both limits independently.
func RunDefaultTenantSplitWithBudgets(ctx context.Context, sharedPath, dataPath string, controlOpts, dataOpts []StoreOption) error {
	return runDefaultTenantSplitWithOptions(ctx, sharedPath, dataPath, controlOpts, dataOpts)
}

type defaultTenantSplitFaultHook func(point string) error

// runDefaultTenantSplitWithOptions keeps the startup call shape explicit:
// control schema migration may use a larger budget, but every operation in
// the split itself uses dataOpts. The first option set is retained for callers
// that configure both startup phases together.
func runDefaultTenantSplitWithOptions(ctx context.Context, sharedPath, dataPath string, controlOpts, dataOpts []StoreOption) error {
	return runDefaultTenantSplitWithFaults(ctx, sharedPath, dataPath, controlOpts, dataOpts, nil)
}

func runDefaultTenantSplitWithFaults(ctx context.Context, sharedPath, dataPath string, controlOpts, dataOpts []StoreOption, fault defaultTenantSplitFaultHook) error {
	ctx = hklog.WithLoggerIfAbsent(ctx, storeLoggerFromOptions(controlOpts, dataOpts))
	if strings.TrimSpace(sharedPath) == "" || strings.TrimSpace(dataPath) == "" {
		return errors.New("default tenant split requires shared and data paths")
	}
	if err := recoverInterruptedSplitControlSwap(sharedPath); err != nil {
		return err
	}

	shared, err := openSplitDatabase(sharedPath, dataOpts...)
	if err != nil {
		return fmt.Errorf("open shared database for split: %w", err)
	}
	defaultID, splitApplied, compacted, err := readDefaultTenantSplitState(ctx, shared)
	_ = shared.Close()
	if err != nil {
		return err
	}
	if defaultID == uuid.Nil {
		return errors.New("default tenant split requires a default tenant")
	}

	tenantDir := filepath.Join(dataPath, "tenants", defaultID.String())
	finalPath := filepath.Join(tenantDir, "hitkeep.db")
	workPath := finalPath + ".split-work"
	if splitApplied {
		if _, err := os.Stat(finalPath); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("stat default tenant file %s: %w", finalPath, err)
			}
			restored, restoreErr := restoreSplitControlBackupForMissingTenantFile(ctx, sharedPath, finalPath)
			if restoreErr != nil {
				return restoreErr
			}
			if !restored {
				return errMissingSplitTenantFile(finalPath, "restore the tenant file or a pre-split control database backup, or run 'hitkeep recover rebuild-default-tenant' to accept the loss and rebuild an empty one")
			}
			// The restored control predates the split, so run the whole split
			// again from the top. The restore consumed the backup, so a second
			// pass through this branch fails instead of looping.
			return runDefaultTenantSplitWithFaults(ctx, sharedPath, dataPath, controlOpts, dataOpts, fault)
		}
	} else {
		if err := prepareDefaultTenantFile(ctx, sharedPath, finalPath, workPath, defaultID, controlOpts, dataOpts, fault); err != nil {
			return err
		}
	}

	if !compacted {
		if err := triggerDefaultTenantSplitFault(fault, "before-control-rewrite"); err != nil {
			return err
		}
		if err := compactSplitSharedFile(ctx, sharedPath, finalPath, defaultID, fault, controlOpts...); err != nil {
			return err
		}
		if err := triggerDefaultTenantSplitFault(fault, "after-control-rewrite"); err != nil {
			return err
		}
		if err := verifyDefaultScopedRowsEmpty(ctx, sharedPath, finalPath, defaultID, dataOpts...); err != nil {
			return err
		}
		control, err := openSplitDatabase(sharedPath, dataOpts...)
		if err != nil {
			return fmt.Errorf("reopen shared database after split compaction: %w", err)
		}
		if err := triggerDefaultTenantSplitFault(fault, "before-cleanup-marker"); err != nil {
			_ = control.Close()
			return err
		}
		_, err = control.ExecContext(ctx, `
			INSERT INTO data_migrations (name, applied_at)
			VALUES (?, ?)
			ON CONFLICT (name) DO NOTHING
		`, defaultTenantCompactionMark, time.Now().UTC())
		if err == nil {
			err = triggerDefaultTenantSplitFault(fault, "after-cleanup-marker")
		}
		if err == nil {
			_, err = control.ExecContext(ctx, "CHECKPOINT;")
		}
		if err == nil {
			err = triggerDefaultTenantSplitFault(fault, "after-cleanup-marker-checkpoint")
		}
		closeErr := control.Close()
		if err != nil {
			return fmt.Errorf("record default tenant split compaction: %w", err)
		}
		if closeErr != nil {
			return fmt.Errorf("close shared database after split compaction: %w", closeErr)
		}
	}

	hklog.LoggerFromContext(ctx).Info("Default tenant split is complete", "tenant_id", defaultID, "tenant_path", finalPath)
	return nil
}

// recoverInterruptedSplitControlSwap closes the only non-atomic window in the
// two-rename control-file publication sequence. If the process stopped after
// moving the authoritative source aside, restore it and rebuild the rewrite on
// this boot. A published shared file always wins; its split marker determines
// whether another harmless rewrite is required.
func recoverInterruptedSplitControlSwap(sharedPath string) error {
	_, sharedErr := os.Stat(sharedPath)
	if sharedErr == nil {
		return nil
	}
	if !errors.Is(sharedErr, os.ErrNotExist) {
		return fmt.Errorf("inspect shared database during split swap recovery: %w", sharedErr)
	}
	_, backupErr := os.Stat(sharedPath + preCompactBackupSuffix)
	if errors.Is(backupErr, os.ErrNotExist) {
		return nil
	}
	if backupErr != nil {
		return fmt.Errorf("inspect pre-compaction control database during split swap recovery: %w", backupErr)
	}
	return restorePreCompactControlBackup(sharedPath)
}

// restorePreCompactControlBackup moves the pre-compaction control backup back
// over the shared database file and makes the rename durable. Callers decide
// when restoring is the right resolution.
func restorePreCompactControlBackup(sharedPath string) error {
	if err := os.Rename(sharedPath+preCompactBackupSuffix, sharedPath); err != nil {
		return fmt.Errorf("restore pre-compaction control database: %w", err)
	}
	if err := syncDirectory(filepath.Dir(sharedPath)); err != nil {
		return fmt.Errorf("sync restored control database: %w", err)
	}
	return nil
}

// restoreSplitControlBackupForMissingTenantFile covers the crash window where
// the rewritten control file (already carrying the split marker) was published
// but the pre-compaction backup was not yet removed. If the split tenant file
// has since disappeared, for example because the tenant data directory was
// not on persistent storage, the backup still holds every pre-split row, so
// restoring it and re-running the whole split loses nothing. The published
// control never served application writes in this state: startup finishes the
// split before the application starts.
func restoreSplitControlBackupForMissingTenantFile(ctx context.Context, sharedPath, tenantPath string) (bool, error) {
	if _, err := os.Stat(sharedPath + preCompactBackupSuffix); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect pre-compaction control database for missing tenant file recovery: %w", err)
	}
	if err := restorePreCompactControlBackup(sharedPath); err != nil {
		return false, err
	}
	hklog.LoggerFromContext(ctx).Warn("Restored pre-split control database because the split tenant file is missing; the default tenant split will run again",
		"tenant_path", tenantPath, "control_path", sharedPath)
	return true, nil
}

// errMissingSplitTenantFile explains the unrecoverable split state to the
// operator: the split marker is durable but the tenant database is gone,
// which almost always means the tenant data directory was not persistent.
func errMissingSplitTenantFile(path, remedy string) error {
	return fmt.Errorf("default tenant split marker exists but tenant file is missing: %s; the tenant data directory (HITKEEP_DATA_PATH) no longer holds the split tenant database, most likely because it was not on persistent storage when the split ran (in Docker, keep HITKEEP_DATA_PATH on the mounted data volume); %s, then restart", path, remedy)
}

// HasDefaultTenantSplit reports whether the data-plane file has become
// authoritative. Missing data_migrations is expected on pre-split installs.
func (s *Store) HasDefaultTenantSplit(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("database is not connected")
	}
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM data_migrations WHERE name = ?", defaultTenantSplitMarker,
	).Scan(&count)
	if err != nil {
		if isMissingDataMigrationsTable(err) {
			return false, nil
		}
		return false, fmt.Errorf("read default tenant split marker: %w", err)
	}
	return count > 0, nil
}

// DefaultTenantSplitComplete reports whether both durable split markers are
// present. It lets startup avoid closing/reopening an already-clean database.
func (s *Store) DefaultTenantSplitComplete(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("database is not connected")
	}
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM data_migrations
		WHERE name IN (?, ?)
	`, defaultTenantSplitMarker, defaultTenantCompactionMark).Scan(&count)
	if err != nil {
		if isMissingDataMigrationsTable(err) {
			return false, nil
		}
		return false, fmt.Errorf("read default tenant split completion: %w", err)
	}
	return count == 2, nil
}

func isMissingDataMigrationsTable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "data_migrations") && strings.Contains(message, "does not exist")
}

func readDefaultTenantSplitState(ctx context.Context, db *sql.DB) (uuid.UUID, bool, bool, error) {
	var raw string
	if err := db.QueryRowContext(ctx, `
		SELECT CAST(id AS VARCHAR) FROM tenants WHERE is_default = TRUE LIMIT 1
	`).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, false, false, nil
		}
		return uuid.Nil, false, false, fmt.Errorf("resolve default tenant for split: %w", err)
	}
	defaultID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false, false, fmt.Errorf("parse default tenant id %q: %w", raw, err)
	}
	var splitCount, compactCount int
	if err := db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE name = ?),
			COUNT(*) FILTER (WHERE name = ?)
		FROM data_migrations
	`, defaultTenantSplitMarker, defaultTenantCompactionMark).Scan(&splitCount, &compactCount); err != nil {
		if isMissingDataMigrationsTable(err) {
			return defaultID, false, false, nil
		}
		return uuid.Nil, false, false, fmt.Errorf("read default tenant split state: %w", err)
	}
	return defaultID, splitCount > 0, compactCount > 0, nil
}

func prepareDefaultTenantFile(ctx context.Context, sharedPath, finalPath, workPath string, defaultID uuid.UUID, controlOpts, dataOpts []StoreOption, fault defaultTenantSplitFaultHook) error {
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return fmt.Errorf("create default tenant directory: %w", err)
	}
	for _, stale := range []string{workPath, workPath + ".wal"} {
		if err := os.Remove(stale); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale default tenant split artifact %s: %w", stale, err)
		}
	}
	if _, err := os.Stat(finalPath); err == nil {
		stray := fmt.Sprintf("%s.stray-%d", finalPath, time.Now().UTC().UnixNano())
		if err := os.Rename(finalPath, stray); err != nil {
			return fmt.Errorf("preserve unsentinelled default tenant file: %w", err)
		}
		if err := syncParentDirectory(stray); err != nil {
			return fmt.Errorf("sync preserved default tenant file: %w", err)
		}
		hklog.LoggerFromContext(ctx).Warn("Preserved unsentinelled default tenant file and will rebuild it", "path", stray)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat default tenant file: %w", err)
	}

	dataRequired, controlRequired, err := defaultTenantSplitSpaceRequirements(sharedPath)
	if err != nil {
		return err
	}
	if err := ensureAvailableSpace(filepath.Dir(workPath), dataRequired); err != nil {
		return fmt.Errorf("preflight default tenant split data space: %w", err)
	}
	if err := ensureAvailableSpace(filepath.Dir(sharedPath), controlRequired); err != nil {
		return fmt.Errorf("preflight default tenant split control space: %w", err)
	}
	target := NewStore(workPath, dataOpts...)
	if err := target.Connect(); err != nil {
		return fmt.Errorf("create default tenant split target: %w", err)
	}
	if err := target.migrateTenant(ctx, migrationRunOptions{guarded: true}); err != nil {
		_ = target.Close()
		return fmt.Errorf("prepare default tenant split target schema: %w", err)
	}
	if err := target.Close(); err != nil {
		return fmt.Errorf("close default tenant split target schema: %w", err)
	}
	if err := triggerDefaultTenantSplitFault(fault, "after-target-preparation"); err != nil {
		return err
	}

	worker, err := openSplitDatabase(":memory:", dataOpts...)
	if err != nil {
		return fmt.Errorf("open default tenant split worker: %w", err)
	}
	defer func() {
		if worker != nil {
			_ = worker.Close()
		}
	}()
	if err := disableSplitIndexScans(ctx, worker); err != nil {
		return err
	}
	if _, err := worker.ExecContext(ctx, fmt.Sprintf("ATTACH '%s' AS split_source (READ_ONLY);", escapeSQLString(sharedPath))); err != nil {
		return fmt.Errorf("attach default tenant split source: %w", err)
	}
	if _, err := worker.ExecContext(ctx, fmt.Sprintf("ATTACH '%s' AS split_target;", escapeSQLString(workPath))); err != nil {
		return fmt.Errorf("attach default tenant split target: %w", err)
	}

	if _, err := worker.ExecContext(ctx, `
		CREATE TEMP TABLE split_scope (site_id UUID PRIMARY KEY);
		INSERT INTO split_scope
		SELECT s.id
		FROM split_source.sites s
		LEFT JOIN split_source.site_tenants st ON st.site_id = s.id
		WHERE st.tenant_id = ? OR st.site_id IS NULL
	`, defaultID); err != nil {
		return fmt.Errorf("build default tenant split scope: %w", err)
	}
	if _, err := worker.ExecContext(ctx, `
		INSERT INTO split_target.sites (id, domain, data_retention_days)
		SELECT s.id, s.domain, s.data_retention_days
		FROM split_source.sites s
		JOIN split_scope scope ON scope.site_id = s.id
	`); err != nil {
		return fmt.Errorf("copy default tenant site mirrors: %w", err)
	}
	if _, err := worker.ExecContext(ctx, "CHECKPOINT split_target;"); err != nil {
		return fmt.Errorf("checkpoint default tenant site mirrors: %w", err)
	}
	scopePath := workPath + ".split-scope.parquet"
	if err := os.Remove(scopePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale default tenant split scope: %w", err)
	}
	defer os.Remove(scopePath)
	if _, err := worker.ExecContext(ctx, fmt.Sprintf("COPY split_scope TO '%s' (FORMAT PARQUET, COMPRESSION ZSTD)", escapeSQLString(scopePath))); err != nil {
		return fmt.Errorf("persist default tenant split scope: %w", err)
	}
	tables, err := splitTables(ctx, worker)
	if err != nil {
		return err
	}
	type splitCopyPlan struct {
		table   string
		columns []string
	}
	plans := make([]splitCopyPlan, 0, len(tables))
	// orderChildrenFirst returns children before their parents. Copy in the
	// opposite direction so tenant foreign keys are satisfied.
	for _, table := range slices.Backward(tables) {
		targetColumns, err := listCatalogColumns(ctx, worker, "split_target", table)
		if err != nil {
			return err
		}
		sourceColumns, err := listCatalogColumns(ctx, worker, "split_source", table)
		if err != nil {
			return err
		}
		sourceColumnSet := make(map[string]struct{}, len(sourceColumns))
		for _, column := range sourceColumns {
			sourceColumnSet[column] = struct{}{}
		}
		copyColumns := make([]string, 0, len(targetColumns))
		for _, column := range targetColumns {
			if _, ok := sourceColumnSet[column]; ok {
				copyColumns = append(copyColumns, column)
			}
		}
		if len(copyColumns) == 0 || !slices.Contains(copyColumns, "site_id") {
			return fmt.Errorf("table %s has no site-scoped source/target column intersection", table)
		}
		plans = append(plans, splitCopyPlan{table: table, columns: copyColumns})
	}
	// The scope and mirror queries can touch legacy control indexes. Closing the
	// whole in-memory worker resets DuckDB's buffer manager before analytics.
	for _, alias := range []string{"split_source", "split_target"} {
		if _, err := worker.ExecContext(ctx, "DETACH "+alias+";"); err != nil {
			return fmt.Errorf("detach default tenant split %s after site mirrors: %w", alias, err)
		}
	}
	if err := worker.Close(); err != nil {
		return fmt.Errorf("close default tenant split scope worker: %w", err)
	}
	worker = nil
	sourceExportOpts := controlOpts
	for _, plan := range plans {
		table := plan.table
		copyColumns := plan.columns
		columnList := strings.Join(copyColumns, ", ")
		sourceColumnList := make([]string, len(copyColumns))
		for i, column := range copyColumns {
			sourceColumnList[i] = "src." + column
		}
		exportPath := workPath + ".split-export." + table + ".parquet"
		if err := os.Remove(exportPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale default tenant table export %s: %w", table, err)
		}
		defer os.Remove(exportPath)
		if err := func() error {
			sourceDB, err := openSplitDatabase(sharedPath, sourceExportOpts...)
			if err != nil {
				return err
			}
			defer sourceDB.Close()
			if !splitUsesLiveSiteProbe(table) {
				if err := disableSplitIndexScans(ctx, sourceDB); err != nil {
					return err
				}
			}
			var sourceCatalog string
			if err := sourceDB.QueryRowContext(ctx, "SELECT current_database()").Scan(&sourceCatalog); err != nil {
				return err
			}
			// #nosec G201 -- identifiers come from validated catalog metadata and the path is escaped.
			exportQuery := fmt.Sprintf("COPY (SELECT %s FROM %s) TO '%s' (FORMAT PARQUET, COMPRESSION ZSTD)", splitQualifiedColumns(copyColumns, "rows"), splitSourceExportRelation(sourceCatalog, table), escapeSQLString(exportPath))
			if _, err := sourceDB.ExecContext(ctx, exportQuery); err != nil {
				return fmt.Errorf("export default tenant source table %s: %w", table, err)
			}
			return nil
		}(); err != nil {
			return err
		}
		if err := func() error {
			worker, err := openSplitDatabase(":memory:", dataOpts...)
			if err != nil {
				return err
			}
			defer worker.Close()
			if _, err := worker.ExecContext(ctx, fmt.Sprintf("ATTACH '%s' AS split_target;", escapeSQLString(workPath))); err != nil {
				return err
			}
			defer func() { _, _ = worker.ExecContext(context.Background(), "DETACH split_target;") }()
			if _, err := worker.ExecContext(ctx, fmt.Sprintf("CREATE TEMP TABLE split_scope AS SELECT site_id FROM read_parquet('%s')", escapeSQLString(scopePath))); err != nil {
				return err
			}
			// #nosec G201 -- identifiers come from validated catalog metadata and the path is escaped.
			query := fmt.Sprintf("INSERT INTO split_target.%s (%s) SELECT %s FROM read_parquet('%s') src JOIN split_scope scope ON CAST(scope.site_id AS VARCHAR) = CAST(src.site_id AS VARCHAR)", table, columnList, strings.Join(sourceColumnList, ", "), escapeSQLString(exportPath))
			if _, err := worker.ExecContext(ctx, query); err != nil {
				return fmt.Errorf("copy default tenant table %s: %w", table, err)
			}
			if err := triggerDefaultTenantSplitFault(fault, "after-copy:"+table); err != nil {
				return err
			}
			sourceFingerprint, err := fingerprintSplitParquet(ctx, worker, exportPath, table, copyColumns, "JOIN split_scope scope ON CAST(scope.site_id AS VARCHAR) = CAST(rows.site_id AS VARCHAR)")
			if err != nil {
				return fmt.Errorf("fingerprint shared default tenant table %s: %w", table, err)
			}
			if err := triggerDefaultTenantSplitFault(fault, "after-source-fingerprint:"+table); err != nil {
				return err
			}
			targetFingerprint, err := fingerprintSplitTable(ctx, worker, "split_target", table, copyColumns, "")
			if err != nil {
				return fmt.Errorf("fingerprint tenant default table %s: %w", table, err)
			}
			if err := triggerDefaultTenantSplitFault(fault, "after-target-fingerprint:"+table); err != nil {
				return err
			}
			if sourceFingerprint != targetFingerprint {
				return fmt.Errorf("default tenant table %s integrity mismatch: source_count=%d target_count=%d", table, sourceFingerprint.Count, targetFingerprint.Count)
			}
			if _, err := worker.ExecContext(ctx, "CHECKPOINT split_target;"); err != nil {
				return fmt.Errorf("checkpoint copied default tenant table %s: %w", table, err)
			}
			return triggerDefaultTenantSplitFault(fault, "after-copy-checkpoint:"+table)
		}(); err != nil {
			return err
		}
		if err := os.Remove(exportPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove default tenant table export %s: %w", table, err)
		}
	}
	if err := os.Remove(scopePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove default tenant split scope: %w", err)
	}
	finalDB, err := openSplitDatabase(workPath, dataOpts...)
	if err != nil {
		return err
	}
	if _, err := finalDB.ExecContext(ctx, "CHECKPOINT;"); err != nil {
		_ = finalDB.Close()
		return fmt.Errorf("checkpoint default tenant split target: %w", err)
	}
	if err := triggerDefaultTenantSplitFault(fault, "after-target-checkpoint"); err != nil {
		_ = finalDB.Close()
		return err
	}
	if err := finalDB.Close(); err != nil {
		return fmt.Errorf("close default tenant split target: %w", err)
	}
	if err := os.Rename(workPath, finalPath); err != nil {
		return fmt.Errorf("publish default tenant split file: %w", err)
	}
	if err := triggerDefaultTenantSplitFault(fault, "after-target-rename"); err != nil {
		return err
	}
	if err := syncParentDirectory(finalPath); err != nil {
		return fmt.Errorf("sync default tenant split publication: %w", err)
	}
	return nil
}

func triggerDefaultTenantSplitFault(fault defaultTenantSplitFaultHook, point string) error {
	if fault == nil {
		return nil
	}
	if err := fault(point); err != nil {
		return fmt.Errorf("default tenant split fault at %s: %w", point, err)
	}
	return nil
}

func defaultTenantSplitSpaceRequirements(sharedPath string) (dataRequired, controlRequired int64, err error) {
	sharedInfo, err := os.Stat(sharedPath)
	if err != nil {
		return 0, 0, fmt.Errorf("inspect shared database size for default tenant split: %w", err)
	}
	return sharedInfo.Size() + defaultTenantSplitHeadroom, defaultTenantSplitHeadroom, nil
}

func splitTables(ctx context.Context, worker *sql.DB) ([]string, error) {
	source, err := listCatalogTables(ctx, worker, "split_source")
	if err != nil {
		return nil, err
	}
	target, err := listCatalogTables(ctx, worker, "split_target")
	if err != nil {
		return nil, err
	}
	targetSet := make(map[string]struct{}, len(target))
	for _, table := range target {
		if table != "migrations" && table != "sites" {
			targetSet[table] = struct{}{}
		}
	}
	members := make(map[string]struct{})
	for _, table := range source {
		if _, ok := targetSet[table]; !ok {
			continue
		}
		columns, err := listCatalogColumns(ctx, worker, "split_target", table)
		if err != nil {
			return nil, err
		}
		if slices.Contains(columns, "site_id") {
			members[table] = struct{}{}
		}
	}
	edges, err := listCatalogFKEdges(ctx, worker, "split_target")
	if err != nil {
		return nil, err
	}
	ordered, err := orderChildrenFirst(members, edges)
	if err != nil {
		return nil, err
	}
	return ordered, nil
}

func sameColumns(left, right []string) bool {
	a := slices.Clone(left)
	b := slices.Clone(right)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// fingerprintSplitTable computes two independent, order-independent hashes
// over every column. Combined with the exact count this catches lost,
// duplicated, or mutated rows without materializing client data in Go or logs.
func fingerprintSplitTable(ctx context.Context, db *sql.DB, catalog, table string, columns []string, suffix string) (splitTableFingerprint, error) {
	relation := quoteDuckDBIdentifier(catalog) + "." + quoteDuckDBIdentifier(table)
	return fingerprintSplitRelation(ctx, db, relation, table, columns, suffix)
}

func fingerprintSplitParquet(ctx context.Context, db *sql.DB, path, table string, columns []string, suffix string) (splitTableFingerprint, error) {
	relation := "read_parquet('" + escapeSQLString(path) + "')"
	return fingerprintSplitRelation(ctx, db, relation, table, columns, suffix)
}

func fingerprintSplitRelation(ctx context.Context, db *sql.DB, relation, label string, columns []string, suffix string) (splitTableFingerprint, error) {
	if len(columns) == 0 {
		return splitTableFingerprint{}, fmt.Errorf("table %s has no columns", label)
	}
	qualifiedColumns := make([]string, len(columns))
	for i, column := range columns {
		qualifiedColumns[i] = "rows." + quoteDuckDBIdentifier(column)
	}
	hashExpression := "hash(" + strings.Join(qualifiedColumns, ", ") + ")"
	// #nosec G201 -- relation, suffix, and identifiers are assembled only from validated catalog metadata.
	query := fmt.Sprintf(`
		SELECT
			COUNT(*),
			COALESCE(CAST(SUM(CAST(%s AS DECIMAL(38, 0))) AS VARCHAR), '0'),
			COALESCE(CAST(BIT_XOR(%s) AS VARCHAR), '0')
		FROM %s rows %s`,
		hashExpression,
		hashExpression,
		relation,
		suffix,
	)
	var fingerprint splitTableFingerprint
	if err := db.QueryRowContext(ctx, query).Scan(&fingerprint.Count, &fingerprint.HashSum, &fingerprint.HashXOR); err != nil {
		return splitTableFingerprint{}, err
	}
	return fingerprint, nil
}

func compactSplitSharedFile(ctx context.Context, sharedPath, tenantPath string, defaultID uuid.UUID, fault defaultTenantSplitFaultHook, opts ...StoreOption) error {
	compactionOpts := CompactionOptions{MinReclaimableBytes: 0, MinFreeRatio: 0}
	if len(opts) > 0 {
		template := NewStore(":memory:", opts...)
		compactionOpts.MemoryLimit = template.memoryLimit
		compactionOpts.Threads = template.threads
	}
	if err := rewriteSplitControlDatabase(ctx, sharedPath, tenantPath, defaultID, fault, compactionOpts, opts...); err != nil {
		return fmt.Errorf("compact shared database after default tenant split: %w", err)
	}
	return nil
}

func rewriteSplitControlDatabase(ctx context.Context, sharedPath, tenantPath string, defaultID uuid.UUID, fault defaultTenantSplitFaultHook, compactionOpts CompactionOptions, storeOpts ...StoreOption) error {
	applied, err := readSplitSourceMigrations(ctx, sharedPath, storeOpts...)
	if err != nil {
		return err
	}
	workPath := sharedPath + ".compacting"
	backupPath := sharedPath + preCompactBackupSuffix
	for _, stale := range []string{workPath, workPath + ".wal", backupPath} {
		if err := os.Remove(stale); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale split compaction artifact %s: %w", stale, err)
		}
	}
	prepare := func(ctx context.Context, target *Store) error {
		if err := prepareSharedSchemaAtMigrations(ctx, target, applied); err != nil {
			return err
		}
		if err := target.ensureMigrationCheckpointTable(ctx); err != nil {
			return err
		}
		_, err := target.DB().ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS data_migrations (
				name VARCHAR PRIMARY KEY,
				applied_at TIMESTAMP NOT NULL
			)`)
		return err
	}
	if err := rewriteSplitControlWorkFile(ctx, sharedPath, tenantPath, workPath, defaultID, prepare, compactionOpts, fault); err != nil {
		_ = os.Remove(workPath)
		_ = os.Remove(workPath + ".wal")
		return err
	}
	migrated, err := OpenMigratedStore(ctx, workPath, storeOpts...)
	if err != nil {
		_ = os.Remove(workPath)
		_ = os.Remove(workPath + ".wal")
		return fmt.Errorf("migrate rewritten split control database: %w", err)
	}
	if err := migrated.Close(); err != nil {
		return fmt.Errorf("close migrated split control database: %w", err)
	}
	if err := os.Rename(sharedPath, backupPath); err != nil {
		return fmt.Errorf("move pre-split control database aside: %w", err)
	}
	if err := syncDirectory(filepath.Dir(sharedPath)); err != nil {
		if restoreErr := os.Rename(backupPath, sharedPath); restoreErr != nil {
			return fmt.Errorf("sync control directory before split rewrite publication: %w (restore failed: %v)", err, restoreErr)
		}
		return fmt.Errorf("sync control directory before split rewrite publication: %w", err)
	}
	if err := triggerDefaultTenantSplitFault(fault, "after-control-source-rename"); err != nil {
		return err
	}
	if err := os.Rename(workPath, sharedPath); err != nil {
		if restoreErr := os.Rename(backupPath, sharedPath); restoreErr != nil {
			return fmt.Errorf("publish split control database: %w (restore failed: %v)", err, restoreErr)
		}
		return fmt.Errorf("publish split control database: %w", err)
	}
	if err := syncDirectory(filepath.Dir(sharedPath)); err != nil {
		return fmt.Errorf("sync control directory after split rewrite publication: %w", err)
	}
	if err := triggerDefaultTenantSplitFault(fault, "after-control-target-rename"); err != nil {
		return err
	}
	if err := os.Remove(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove pre-split control database: %w", err)
	}
	if err := os.Remove(sharedPath + ".wal"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove obsolete split control WAL: %w", err)
	}
	return nil
}

func rewriteSplitControlWorkFile(
	ctx context.Context,
	sourcePath, tenantPath, targetPath string,
	defaultID uuid.UUID,
	prepare SchemaPreparer,
	opts CompactionOptions,
	fault defaultTenantSplitFaultHook,
) error {
	target := NewStore(targetPath, compactionStoreOptions(opts)...)
	if err := target.Connect(); err != nil {
		return fmt.Errorf("create split control rewrite target: %w", err)
	}
	if err := prepare(ctx, target); err != nil {
		_ = target.Close()
		return fmt.Errorf("prepare split control source schema: %w", err)
	}
	if err := target.Close(); err != nil {
		return fmt.Errorf("close prepared split control target: %w", err)
	}

	tenantDB, err := openDuckDBFile(tenantPath)
	if err != nil {
		return err
	}
	var tenantCatalog string
	if err := tenantDB.QueryRowContext(ctx, "SELECT current_database()").Scan(&tenantCatalog); err != nil {
		_ = tenantDB.Close()
		return err
	}
	tenantTables, err := listCatalogTables(ctx, tenantDB, tenantCatalog)
	if closeErr := tenantDB.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	analytics := make(map[string]struct{}, len(tenantTables))
	for _, table := range tenantTables {
		analytics[table] = struct{}{}
	}

	sourceDB, err := openSplitDatabase(sourcePath, compactionStoreOptions(opts)...)
	if err != nil {
		return err
	}
	var sourceCatalog string
	if err := sourceDB.QueryRowContext(ctx, "SELECT current_database()").Scan(&sourceCatalog); err != nil {
		_ = sourceDB.Close()
		return fmt.Errorf("resolve split control export source catalog: %w", err)
	}
	sourceTables, err := listCatalogTables(ctx, sourceDB, sourceCatalog)
	if closeErr := sourceDB.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}

	targetDB, err := openDuckDBFile(targetPath)
	if err != nil {
		return err
	}
	if err := configureCompactionDB(ctx, targetDB, opts); err != nil {
		_ = targetDB.Close()
		return err
	}
	var targetCatalog string
	if err := targetDB.QueryRowContext(ctx, "SELECT current_database()").Scan(&targetCatalog); err != nil {
		_ = targetDB.Close()
		return err
	}
	targetTables, err := listCatalogTables(ctx, targetDB, targetCatalog)
	if err != nil {
		_ = targetDB.Close()
		return err
	}
	members := make(map[string]struct{}, len(sourceTables))
	for _, table := range sourceTables {
		if table == "migrations" || table == "data_migrations" || table == migrationCheckpointTable {
			continue
		}
		if !slices.Contains(targetTables, table) {
			return fmt.Errorf("split control target is missing source table %s", table)
		}
		members[table] = struct{}{}
	}
	edges, err := listCatalogFKEdges(ctx, targetDB, targetCatalog)
	if err != nil {
		_ = targetDB.Close()
		return err
	}
	childrenFirst, err := orderChildrenFirst(members, edges)
	if err != nil {
		_ = targetDB.Close()
		return err
	}
	for _, table := range childrenFirst {
		// #nosec G202 -- table is a validated catalog identifier.
		if _, err := targetDB.ExecContext(ctx, "DELETE FROM "+quoteDuckDBIdentifier(table)); err != nil {
			_ = targetDB.Close()
			return fmt.Errorf("clear split control target table %s: %w", table, err)
		}
	}
	if _, err := targetDB.ExecContext(ctx, "CHECKPOINT;"); err != nil {
		_ = targetDB.Close()
		return fmt.Errorf("checkpoint cleared split control target: %w", err)
	}
	if err := targetDB.Close(); err != nil {
		return err
	}
	slices.Reverse(childrenFirst)
	type splitControlExport struct {
		table       string
		columns     []string
		filter      string
		path        string
		fingerprint splitTableFingerprint
	}
	exports := make([]splitControlExport, 0, len(childrenFirst))
	for _, table := range childrenFirst {
		sourceDB, err = openSplitDatabase(sourcePath, compactionStoreOptions(opts)...)
		if err != nil {
			return err
		}
		if !splitUsesLiveSiteProbe(table) {
			if err := disableSplitIndexScans(ctx, sourceDB); err != nil {
				_ = sourceDB.Close()
				return err
			}
		}
		if err := sourceDB.QueryRowContext(ctx, "SELECT current_database()").Scan(&sourceCatalog); err != nil {
			_ = sourceDB.Close()
			return fmt.Errorf("resolve split control export source catalog: %w", err)
		}
		if _, err := sourceDB.ExecContext(ctx, `
			CREATE TEMP TABLE split_scope (site_id UUID PRIMARY KEY);
			INSERT INTO split_scope
			SELECT s.id
			FROM sites s
			LEFT JOIN site_tenants st ON st.site_id = s.id
			WHERE st.tenant_id = ? OR st.site_id IS NULL
		`, defaultID); err != nil {
			_ = sourceDB.Close()
			return err
		}
		columns, err := listCatalogColumns(ctx, sourceDB, sourceCatalog, table)
		if err != nil {
			_ = sourceDB.Close()
			return err
		}
		columnList := strings.Join(columns, ", ")
		filter := ""
		if _, ok := analytics[table]; ok && table != "sites" && slices.Contains(columns, "site_id") {
			filter = " WHERE NOT EXISTS (SELECT 1 FROM split_scope scope WHERE CAST(scope.site_id AS VARCHAR) = CAST(rows.site_id AS VARCHAR))"
		}
		exportPath := targetPath + ".split-export." + table + ".parquet"
		if err := os.Remove(exportPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = sourceDB.Close()
			return err
		}
		// #nosec G201 -- identifiers come from validated catalog metadata and the path is escaped.
		exportQuery := fmt.Sprintf("COPY (SELECT %s FROM %s) TO '%s' (FORMAT PARQUET, COMPRESSION ZSTD)", splitQualifiedColumns(columns, "rows"), splitSourceExportRelation(sourceCatalog, table), escapeSQLString(exportPath))
		if _, err := sourceDB.ExecContext(ctx, exportQuery); err != nil {
			_ = sourceDB.Close()
			return fmt.Errorf("export split control table %s: %w", table, err)
		}
		if filter != "" {
			rawPath := exportPath + ".raw"
			if err := os.Rename(exportPath, rawPath); err != nil {
				_ = sourceDB.Close()
				return fmt.Errorf("stage unfiltered split control table %s: %w", table, err)
			}
			// #nosec G201 -- columns/filter come from validated metadata and paths are escaped.
			filteredQuery := fmt.Sprintf("COPY (SELECT %s FROM read_parquet('%s') rows%s) TO '%s' (FORMAT PARQUET, COMPRESSION ZSTD)", columnList, escapeSQLString(rawPath), filter, escapeSQLString(exportPath))
			if _, err := sourceDB.ExecContext(ctx, filteredQuery); err != nil {
				_ = os.Remove(rawPath)
				_ = sourceDB.Close()
				return fmt.Errorf("filter split control table %s export: %w", table, err)
			}
			if err := os.Remove(rawPath); err != nil {
				_ = sourceDB.Close()
				return err
			}
		}
		sourceFingerprint, err := fingerprintSplitParquet(ctx, sourceDB, exportPath, table, columns, "")
		if err != nil {
			_ = sourceDB.Close()
			return fmt.Errorf("fingerprint split control source table %s export: %w", table, err)
		}
		exports = append(exports, splitControlExport{table: table, columns: columns, filter: filter, path: exportPath, fingerprint: sourceFingerprint})
		if err := sourceDB.Close(); err != nil {
			return err
		}
	}
	defer func() {
		for _, export := range exports {
			_ = os.Remove(export.path)
		}
	}()

	targetDB, err = openDuckDBFile(targetPath)
	if err != nil {
		return err
	}
	defer targetDB.Close()
	if err := configureCompactionDB(ctx, targetDB, opts); err != nil {
		_ = targetDB.Close()
		return err
	}
	for _, export := range exports {
		table := export.table
		columns := export.columns
		columnList := strings.Join(columns, ", ")
		targetColumns, err := listCatalogColumns(ctx, targetDB, targetCatalog, table)
		if err != nil {
			_ = targetDB.Close()
			return err
		}
		if !sameColumns(columns, targetColumns) {
			_ = targetDB.Close()
			return fmt.Errorf("split control table %s diverges from reconstructed source schema", table)
		}
		// #nosec G201 -- table/columns come from validated catalog metadata and the path is escaped.
		importQuery := fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM read_parquet('%s')", quoteDuckDBIdentifier(table), columnList, columnList, escapeSQLString(export.path))
		if _, err := targetDB.ExecContext(ctx, importQuery); err != nil {
			_ = targetDB.Close()
			return fmt.Errorf("import split control table %s: %w", table, err)
		}
		if export.filter != "" {
			if err := triggerDefaultTenantSplitFault(fault, "after-delete:"+table); err != nil {
				_ = targetDB.Close()
				return err
			}
		}
		targetFingerprint, err := fingerprintSplitTable(ctx, targetDB, targetCatalog, table, columns, "")
		if err != nil {
			_ = targetDB.Close()
			return err
		}
		if export.fingerprint != targetFingerprint {
			_ = targetDB.Close()
			return fmt.Errorf("split control table %s changed during staged rewrite", table)
		}
		if export.filter != "" {
			if err := triggerDefaultTenantSplitFault(fault, "after-cleanup-fingerprint:"+table); err != nil {
				_ = targetDB.Close()
				return err
			}
		}
		if _, err := targetDB.ExecContext(ctx, "CHECKPOINT;"); err != nil {
			_ = targetDB.Close()
			return fmt.Errorf("checkpoint imported split control table %s: %w", table, err)
		}
		if err := os.Remove(export.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := triggerDefaultTenantSplitFault(fault, "before-split-marker"); err != nil {
		return err
	}
	if _, err := targetDB.ExecContext(ctx, `
		INSERT INTO data_migrations (name, applied_at)
		VALUES (?, ?)
		ON CONFLICT (name) DO NOTHING
	`, defaultTenantSplitMarker, time.Now().UTC()); err != nil {
		_ = targetDB.Close()
		return fmt.Errorf("record split marker in rewritten control target: %w", err)
	}
	if err := triggerDefaultTenantSplitFault(fault, "after-split-marker-commit"); err != nil {
		_ = targetDB.Close()
		return err
	}
	if _, err := targetDB.ExecContext(ctx, "CHECKPOINT;"); err != nil {
		_ = targetDB.Close()
		return fmt.Errorf("checkpoint rewritten split control target: %w", err)
	}
	if err := triggerDefaultTenantSplitFault(fault, "after-split-marker-checkpoint"); err != nil {
		_ = targetDB.Close()
		return err
	}
	return targetDB.Close()
}

// disableSplitIndexScans prevents selective site filters from materializing
// legacy ART indexes. DuckDB does not evict loaded ART buffers, so a scan of a
// large pre-split file can otherwise consume the entire migration budget even
// though the copy itself is streaming and single-threaded.
func disableSplitIndexScans(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		SET index_scan_percentage = 0;
		SET index_scan_max_count = 0;
		SET allocator_flush_threshold = '1MiB';
		SET allocator_bulk_deallocation_flush_threshold = '1MiB';
		SET allocator_background_threads = true;
		PRAGMA disable_optimizer;
	`); err != nil {
		return fmt.Errorf("disable index scans for default tenant split: %w", err)
	}
	return nil
}

func splitUsesLiveSiteProbe(table string) bool {
	return table == "site_activity_summary" || table == "site_activity_hourly_counts"
}

func splitQualifiedColumns(columns []string, alias string) string {
	qualified := make([]string, len(columns))
	for i, column := range columns {
		qualified[i] = quoteDuckDBIdentifier(alias) + "." + quoteDuckDBIdentifier(column)
	}
	return strings.Join(qualified, ", ")
}

func splitSourceExportRelation(catalog, table string) string {
	relation := quoteDuckDBIdentifier(catalog) + "." + quoteDuckDBIdentifier(table) + " AS rows"
	if splitUsesLiveSiteProbe(table) {
		relation += " JOIN " + quoteDuckDBIdentifier(catalog) + ".sites AS split_live_sites ON rows.site_id = split_live_sites.id"
	}
	return relation
}

func readSplitSourceMigrations(ctx context.Context, path string, opts ...StoreOption) ([]string, error) {
	db, err := openSplitDatabase(path, opts...)
	if err != nil {
		return nil, fmt.Errorf("open split source migration catalog: %w", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT migration FROM migrations ORDER BY migration")
	if err != nil {
		return nil, fmt.Errorf("read split source migrations: %w", err)
	}
	defer rows.Close()
	var applied []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied = append(applied, name)
	}
	return applied, rows.Err()
}

func prepareSharedSchemaAtMigrations(ctx context.Context, target *Store, applied []string) error {
	migrationTableSQL, err := migrations.Fs.ReadFile("0000_00_00_000000_create_migrations_table.sql")
	if err != nil {
		return err
	}
	if _, err := target.DB().ExecContext(ctx, string(migrationTableSQL)); err != nil {
		return err
	}
	sort.Strings(applied)
	for _, name := range applied {
		contents, err := migrations.Fs.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read applied source migration %s: %w", name, err)
		}
		tx, err := target.DB().BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(contents)); err != nil {
			return rollbackMigrationStep(tx, fmt.Errorf("replay source schema migration %s: %w", name, err))
		}
		if err := target.addAppliedMigration(ctx, tx, name); err != nil {
			return rollbackMigrationStep(tx, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func verifyDefaultScopedRowsEmpty(ctx context.Context, sharedPath, tenantPath string, defaultID uuid.UUID, opts ...StoreOption) error {
	db, err := openSplitDatabase(sharedPath, opts...)
	if err != nil {
		return fmt.Errorf("open shared database for default tenant cleanup verification: %w", err)
	}
	defer db.Close()
	tenantDB, err := openSplitDatabase(tenantPath, opts...)
	if err != nil {
		return fmt.Errorf("open tenant database for default tenant cleanup verification: %w", err)
	}
	var tenantCatalog string
	if err := tenantDB.QueryRowContext(ctx, "SELECT current_database()").Scan(&tenantCatalog); err != nil {
		_ = tenantDB.Close()
		return fmt.Errorf("resolve tenant catalog for default tenant cleanup verification: %w", err)
	}
	tenantTables, err := listCatalogTables(ctx, tenantDB, tenantCatalog)
	closeTenantErr := tenantDB.Close()
	if err != nil {
		return fmt.Errorf("list tenant tables for default tenant cleanup verification: %w", err)
	}
	if closeTenantErr != nil {
		return fmt.Errorf("close tenant database for default tenant cleanup verification: %w", closeTenantErr)
	}
	analyticsTables := make(map[string]struct{}, len(tenantTables))
	for _, table := range tenantTables {
		analyticsTables[table] = struct{}{}
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TEMP TABLE split_verify_scope (site_id UUID PRIMARY KEY);
		INSERT INTO split_verify_scope
		SELECT s.id
		FROM sites s
		LEFT JOIN site_tenants st ON st.site_id = s.id
		WHERE st.tenant_id = ? OR st.site_id IS NULL
	`, defaultID); err != nil {
		return fmt.Errorf("build default tenant cleanup verification scope: %w", err)
	}
	var catalog string
	if err := db.QueryRowContext(ctx, "SELECT current_database()").Scan(&catalog); err != nil {
		return fmt.Errorf("resolve shared catalog for default tenant cleanup verification: %w", err)
	}
	tables, err := listCatalogTables(ctx, db, catalog)
	if err != nil {
		return fmt.Errorf("list shared tables for default tenant cleanup verification: %w", err)
	}
	for _, table := range tables {
		if table == "sites" || table == "migrations" || table == "data_migrations" {
			continue
		}
		if _, ok := analyticsTables[table]; !ok {
			continue
		}
		columns, err := listCatalogColumns(ctx, db, catalog, table)
		if err != nil {
			return err
		}
		if !slices.Contains(columns, "site_id") {
			continue
		}
		var count int64
		// #nosec G201 -- table is a validated catalog identifier.
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s src_rows JOIN split_verify_scope scope ON scope.site_id = src_rows.site_id", quoteDuckDBIdentifier(table))
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return fmt.Errorf("verify shared default tenant table %s: %w", table, err)
		}
		if count != 0 {
			return fmt.Errorf("shared default tenant table %s still contains %d scoped rows", table, count)
		}
	}
	return nil
}

func openSplitDatabase(path string, opts ...StoreOption) (*sql.DB, error) {
	if len(opts) == 0 {
		return openDuckDBFile(path)
	}
	store := NewStore(path, opts...)
	if err := store.Connect(); err != nil {
		return nil, err
	}
	db := store.DB()
	// Transfer ownership of the connection to the caller. The Store wrapper
	// is intentionally not closed here; closing it would close this *sql.DB.
	store.db = nil
	return db, nil
}
