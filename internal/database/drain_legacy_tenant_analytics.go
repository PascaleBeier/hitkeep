package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/uuid"
)

const legacyTenantAnalyticsDrainMarker = "non_default_tenant_analytics_drain_v1"

// DrainLegacyTenantAnalytics makes the post-split DuckDB control source safe
// to discard during the SQLite conversion. Older bridge releases retained
// non-default analytics rows in the shared file. Copying only rows that are
// missing from each tenant catalog makes this operation restartable without
// duplicating data that was already dual-written to the tenant file.
//
// The caller must close every connection to sharedPath before calling this.
func DrainLegacyTenantAnalytics(ctx context.Context, sharedPath, dataPath string, opts ...StoreOption) error {
	if strings.TrimSpace(sharedPath) == "" || strings.TrimSpace(dataPath) == "" {
		return errors.New("legacy tenant analytics drain requires shared and data paths")
	}

	tenantIDs, err := legacyAnalyticsTenantIDs(ctx, sharedPath, opts...)
	if err != nil {
		return err
	}
	for _, tenantID := range tenantIDs {
		if err := drainLegacyTenantCatalog(ctx, sharedPath, dataPath, tenantID, opts...); err != nil {
			return err
		}
	}
	if err := finishLegacyTenantAnalyticsDrain(ctx, sharedPath, opts...); err != nil {
		return err
	}

	// The default-tenant split rewrites the control file before this
	// release-skipping drain removes any remaining non-default analytics. Force
	// one final rewrite so the retained pre-SQLite evidence is truly compact and
	// contains only control data, regardless of normal startup compaction flags.
	template := NewStore(sharedPath, opts...)
	compaction := CompactionOptions{
		MinReclaimableBytes: 0,
		MinFreeRatio:        0,
		MemoryLimit:         template.memoryLimit,
		Threads:             template.threads,
	}
	if _, err := MaybeCompactDatabase(ctx, sharedPath, compaction, PrepareSharedSchema); err != nil {
		return fmt.Errorf("rewrite drained legacy control database: %w", err)
	}
	return nil
}

func legacyAnalyticsTenantIDs(ctx context.Context, sharedPath string, opts ...StoreOption) ([]uuid.UUID, error) {
	db, err := openSplitDatabase(sharedPath, opts...)
	if err != nil {
		return nil, fmt.Errorf("open legacy control database for tenant analytics drain: %w", err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT CAST(st.tenant_id AS VARCHAR)
		FROM site_tenants st
		JOIN tenants t ON t.id = st.tenant_id
		WHERE t.is_default = FALSE
		ORDER BY 1`)
	if err != nil {
		return nil, fmt.Errorf("list legacy analytics tenants: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan legacy analytics tenant: %w", err)
		}
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("parse legacy analytics tenant identifier: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read legacy analytics tenants: %w", err)
	}
	return ids, nil
}

func drainLegacyTenantCatalog(ctx context.Context, sharedPath, dataPath string, tenantID uuid.UUID, opts ...StoreOption) error {
	tenantDir := filepath.Join(dataPath, "tenants", tenantID.String())
	if err := os.MkdirAll(tenantDir, 0o755); err != nil {
		return fmt.Errorf("create tenant analytics directory: %w", err)
	}
	tenantPath := filepath.Join(tenantDir, "hitkeep.db")
	tenant, err := OpenMigratedTenantStore(ctx, tenantPath, opts...)
	if err != nil {
		return fmt.Errorf("prepare tenant catalog for legacy analytics drain: %w", err)
	}
	if err := tenant.Close(); err != nil {
		return fmt.Errorf("close prepared tenant catalog: %w", err)
	}

	worker, err := openSplitDatabase(":memory:", opts...)
	if err != nil {
		return fmt.Errorf("open legacy tenant analytics drain worker: %w", err)
	}
	defer worker.Close()
	if err := disableSplitIndexScans(ctx, worker); err != nil {
		return err
	}
	if _, err := worker.ExecContext(ctx, fmt.Sprintf("ATTACH '%s' AS split_source;", escapeSQLString(sharedPath))); err != nil {
		return fmt.Errorf("attach legacy control source for tenant analytics drain: %w", err)
	}
	if _, err := worker.ExecContext(ctx, fmt.Sprintf("ATTACH '%s' AS split_target;", escapeSQLString(tenantPath))); err != nil {
		return fmt.Errorf("attach tenant target for legacy analytics drain: %w", err)
	}
	if _, err := worker.ExecContext(ctx, `
		CREATE TEMP TABLE legacy_tenant_scope (site_id UUID PRIMARY KEY);
		INSERT INTO legacy_tenant_scope
		SELECT site_id FROM split_source.site_tenants WHERE tenant_id = ?`, tenantID); err != nil {
		return fmt.Errorf("build legacy tenant analytics scope: %w", err)
	}
	if _, err := worker.ExecContext(ctx, `
		INSERT INTO split_target.sites (id, domain, data_retention_days)
		SELECT s.id, s.domain, s.data_retention_days
		FROM split_source.sites s
		JOIN legacy_tenant_scope scope ON scope.site_id = s.id
		ON CONFLICT (id) DO UPDATE SET
			domain = excluded.domain,
			data_retention_days = excluded.data_retention_days`); err != nil {
		return fmt.Errorf("sync tenant site mirrors before legacy analytics drain: %w", err)
	}

	childrenFirst, err := splitTables(ctx, worker)
	if err != nil {
		return fmt.Errorf("plan legacy tenant analytics drain: %w", err)
	}
	parentsFirst := slices.Clone(childrenFirst)
	slices.Reverse(parentsFirst)
	for _, table := range parentsFirst {
		columns, err := matchingLegacyDrainColumns(ctx, worker, table)
		if err != nil {
			return err
		}
		columnList := joinQuotedDuckDBIdentifiers(columns)
		// Table and column identifiers are discovered from both catalogs, validated,
		// and quoted before interpolation; values remain relationally scoped.
		//nolint:gosec
		query := fmt.Sprintf(`
			INSERT INTO split_target.%s (%s)
			SELECT * FROM (
				SELECT %s FROM split_source.%s src
				JOIN legacy_tenant_scope scope ON scope.site_id = src.site_id
				EXCEPT ALL
				SELECT %s FROM split_target.%s dst
				JOIN legacy_tenant_scope scope ON scope.site_id = dst.site_id
			) missing_rows`,
			quoteDuckDBIdentifier(table), columnList,
			splitQualifiedColumns(columns, "src"), quoteDuckDBIdentifier(table),
			splitQualifiedColumns(columns, "dst"), quoteDuckDBIdentifier(table),
		)
		if _, err := worker.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("copy missing legacy analytics rows for table %s: %w", table, err)
		}
		missing, err := countMissingLegacyRows(ctx, worker, table, columns)
		if err != nil {
			return err
		}
		if missing != 0 {
			return fmt.Errorf("tenant catalog verification failed for legacy analytics table %s: %d source rows are missing", table, missing)
		}
	}
	if _, err := worker.ExecContext(ctx, "CHECKPOINT split_target;"); err != nil {
		return fmt.Errorf("checkpoint drained tenant analytics catalog: %w", err)
	}

	tx, err := worker.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin legacy analytics source cleanup: %w", err)
	}
	for _, table := range childrenFirst {
		// table is catalog-discovered and quoteDuckDBIdentifier rejects unsafe identifiers.
		query := fmt.Sprintf("DELETE FROM split_source.%s WHERE site_id IN (SELECT site_id FROM legacy_tenant_scope)", quoteDuckDBIdentifier(table)) //nolint:gosec
		if _, err := tx.ExecContext(ctx, query); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete copied legacy analytics rows from table %s: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit legacy analytics source cleanup: %w", err)
	}
	if _, err := worker.ExecContext(ctx, "CHECKPOINT split_source;"); err != nil {
		return fmt.Errorf("checkpoint legacy analytics source cleanup: %w", err)
	}
	return nil
}

func matchingLegacyDrainColumns(ctx context.Context, worker *sql.DB, table string) ([]string, error) {
	source, err := listCatalogColumns(ctx, worker, "split_source", table)
	if err != nil {
		return nil, err
	}
	target, err := listCatalogColumns(ctx, worker, "split_target", table)
	if err != nil {
		return nil, err
	}
	if !sameColumns(source, target) {
		return nil, fmt.Errorf("legacy analytics table %s has diverging source and tenant columns", table)
	}
	if !slices.Contains(source, "site_id") {
		return nil, fmt.Errorf("legacy analytics table %s is not site scoped", table)
	}
	return source, nil
}

func countMissingLegacyRows(ctx context.Context, worker *sql.DB, table string, columns []string) (int64, error) {
	// Table and column identifiers are discovered from both catalogs, validated,
	// and quoted before interpolation; values remain relationally scoped.
	//nolint:gosec
	query := fmt.Sprintf(`
		SELECT count(*) FROM (
			SELECT %s FROM split_source.%s src
			JOIN legacy_tenant_scope scope ON scope.site_id = src.site_id
			EXCEPT ALL
			SELECT %s FROM split_target.%s dst
			JOIN legacy_tenant_scope scope ON scope.site_id = dst.site_id
		) missing_rows`,
		splitQualifiedColumns(columns, "src"), quoteDuckDBIdentifier(table),
		splitQualifiedColumns(columns, "dst"), quoteDuckDBIdentifier(table),
	)
	var count int64
	if err := worker.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("verify copied legacy analytics table %s: %w", table, err)
	}
	return count, nil
}

func joinQuotedDuckDBIdentifiers(columns []string) string {
	quoted := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = quoteDuckDBIdentifier(column)
	}
	return strings.Join(quoted, ", ")
}

func finishLegacyTenantAnalyticsDrain(ctx context.Context, sharedPath string, opts ...StoreOption) error {
	tenantSchema := NewStore(":memory:", opts...)
	if err := tenantSchema.Connect(); err != nil {
		return fmt.Errorf("open tenant schema for final analytics drain verification: %w", err)
	}
	if err := tenantSchema.MigrateTenant(ctx); err != nil {
		_ = tenantSchema.Close()
		return fmt.Errorf("migrate tenant schema for final analytics drain verification: %w", err)
	}
	var tenantCatalog string
	if err := tenantSchema.DB().QueryRowContext(ctx, "SELECT current_database()").Scan(&tenantCatalog); err != nil {
		_ = tenantSchema.Close()
		return fmt.Errorf("resolve tenant schema catalog: %w", err)
	}
	tenantTables, err := listCatalogTables(ctx, tenantSchema.DB(), tenantCatalog)
	if err != nil {
		_ = tenantSchema.Close()
		return err
	}
	if err := tenantSchema.Close(); err != nil {
		return fmt.Errorf("close tenant schema after analytics drain verification: %w", err)
	}
	tenantTableSet := make(map[string]struct{}, len(tenantTables))
	for _, table := range tenantTables {
		tenantTableSet[table] = struct{}{}
	}

	db, err := openSplitDatabase(sharedPath, opts...)
	if err != nil {
		return fmt.Errorf("open legacy control database for final analytics drain verification: %w", err)
	}
	defer db.Close()
	var catalog string
	if err := db.QueryRowContext(ctx, "SELECT current_database()").Scan(&catalog); err != nil {
		return fmt.Errorf("resolve legacy control catalog: %w", err)
	}
	tables, err := listCatalogTables(ctx, db, catalog)
	if err != nil {
		return err
	}
	for _, table := range tables {
		if _, ok := tenantTableSet[table]; !ok {
			continue
		}
		columns, err := listCatalogColumns(ctx, db, catalog, table)
		if err != nil {
			return err
		}
		if !slices.Contains(columns, "site_id") || table == "sites" || table == "site_tenants" {
			continue
		}
		var count int64
		// table is catalog-discovered and quoteDuckDBIdentifier rejects unsafe identifiers.
		query := fmt.Sprintf("SELECT count(*) FROM %s", quoteDuckDBIdentifier(table)) //nolint:gosec
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return fmt.Errorf("verify drained legacy analytics table %s: %w", table, err)
		}
		if count != 0 {
			return fmt.Errorf("legacy analytics compatibility table %s still contains %d rows after tenant drain", table, count)
		}
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO data_migrations (name, applied_at)
		VALUES (?, current_timestamp)
		ON CONFLICT (name) DO NOTHING`, legacyTenantAnalyticsDrainMarker); err != nil {
		return fmt.Errorf("record legacy tenant analytics drain: %w", err)
	}
	if _, err := db.ExecContext(ctx, "CHECKPOINT;"); err != nil {
		return fmt.Errorf("checkpoint completed legacy tenant analytics drain: %w", err)
	}
	return nil
}
