package controlstore

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

type siteStatsResetStep struct {
	table  string
	family string
	query  string
}

// siteDeleteSpec derives the full site delete plan from the live schema:
// every table with a site_id column is covered automatically, and foreign-key
// children are deleted before their parents. Only relationships the schema
// does not declare need to be registered here.
var siteExtraEdges = []fkEdge{
	{table: "site_import_files", column: "import_id", referencedTable: "site_imports", referencedColumn: "id"},
	{table: "webhook_event_subscriptions", column: "webhook_id", referencedTable: "webhooks", referencedColumn: "id"},
	// These QR ownership relationships intentionally have no physical foreign
	// key, so keep them in the explicit cleanup graph.
	{table: "qr_code_assets", column: "qr_code_id", referencedTable: "qr_codes", referencedColumn: "id"},
	{table: "qr_code_share_links", column: "qr_code_id", referencedTable: "qr_codes", referencedColumn: "id"},
}

var siteDeleteSpec = scopedDeleteSpec{
	scopeColumns: []string{"site_id"},
	rootTable:    "sites",
	extraEdges:   siteExtraEdges,
}

var siteStatsResetControlSteps = []siteStatsResetStep{
	{table: "opportunities", family: "ai", query: "DELETE FROM opportunities WHERE site_id = ?"},
	{table: "ai_runs", family: "ai", query: "DELETE FROM ai_runs WHERE site_id = ?"},
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func deleteSiteChildren(ctx context.Context, tx *sql.Tx, siteID uuid.UUID) error {
	steps, err := buildScopedDeletePlan(ctx, tx, siteDeleteSpec)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if _, err := tx.ExecContext(ctx, step.query, siteID); err != nil {
			return fmt.Errorf("could not delete from %s: %w", step.table, err)
		}
	}
	return nil
}

// ResetSiteControlMeasurements removes only control-plane opportunity/run and
// import status state. Tenant analytics are reset through TenantStoreManager.
func (s *Store) ResetSiteControlMeasurements(ctx context.Context, siteID uuid.UUID) (api.SiteStatsResetResponse, error) {
	return s.resetSiteStatsWithSteps(ctx, siteID, siteStatsResetControlSteps, true)
}

func (s *Store) resetSiteStatsWithSteps(ctx context.Context, siteID uuid.UUID, steps []siteStatsResetStep, markCompletedImports bool) (api.SiteStatsResetResponse, error) {
	result := api.SiteStatsResetResponse{Status: "reset"}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("could not begin site stats reset transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	existingTables, err := listTables(ctx, tx)
	if err != nil {
		return result, err
	}

	for _, step := range steps {
		if _, ok := existingTables[step.table]; !ok {
			continue
		}
		if !isSafeIdentifier(step.table) {
			return result, fmt.Errorf("unsafe table name %q", step.table)
		}
		sqlResult, err := tx.ExecContext(ctx, step.query, siteID)
		if err != nil {
			return result, fmt.Errorf("could not reset site stats from %s: %w", step.table, err)
		}
		affected, ok := rowsAffected(sqlResult)
		if !ok || affected <= 0 {
			continue
		}
		result.RowsCleared += affected
		addSiteStatsResetFamily(&result, step.family)
	}

	if markCompletedImports {
		importsDeleted, err := markCompletedImportsDeletedForSite(ctx, tx, siteID)
		if err != nil {
			return result, err
		}
		result.ImportsMarkedDeleted = importsDeleted
		if importsDeleted > 0 {
			addSiteStatsResetFamily(&result, "imports")
		}
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("could not commit site stats reset transaction: %w", err)
	}
	return result, nil
}

func rowsAffected(result sql.Result) (int64, bool) {
	if result == nil {
		return 0, false
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, false
	}
	return affected, true
}

func addSiteStatsResetFamily(result *api.SiteStatsResetResponse, family string) {
	family = strings.TrimSpace(family)
	if result == nil || family == "" {
		return
	}
	if slices.Contains(result.FamiliesCleared, family) {
		return
	}
	result.FamiliesCleared = append(result.FamiliesCleared, family)
}

func listTables(ctx context.Context, q queryer) (map[string]struct{}, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT name
		FROM pragma_table_list
		WHERE schema = 'main' AND type = 'table' AND name NOT LIKE 'sqlite_%'
	`)
	if err != nil {
		return nil, fmt.Errorf("could not list tables: %w", err)
	}
	defer rows.Close()

	tables := make(map[string]struct{})
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("could not scan table: %w", err)
		}
		tables[table] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}
	return tables, nil
}

func listSiteIDTables(ctx context.Context, q queryer) ([]string, error) {
	tableSet, err := listTables(ctx, q)
	if err != nil {
		return nil, err
	}
	var tables []string
	for table := range tableSet {
		hasSiteID, err := tableHasSiteID(ctx, q, table)
		if err != nil {
			return nil, err
		}
		if hasSiteID {
			tables = append(tables, table)
		}
	}
	slices.Sort(tables)
	return tables, nil
}

func tableHasSiteID(ctx context.Context, q queryer, table string) (bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT 1 FROM pragma_table_xinfo(?) WHERE name = 'site_id' AND hidden = 0`, table)
	if err != nil {
		return false, fmt.Errorf("inspect site scope for %s: %w", table, err)
	}
	defer rows.Close()

	hasSiteID := rows.Next()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("read site scope for %s: %w", table, err)
	}
	return hasSiteID, nil
}

func findSiteReferences(ctx context.Context, q queryer, siteID uuid.UUID) ([]string, error) {
	tables, err := listSiteIDTables(ctx, q)
	if err != nil {
		return nil, err
	}

	var refs []string
	for _, table := range tables {
		if table == "sites" {
			continue
		}
		if !isSafeIdentifier(table) {
			continue
		}
		// #nosec G201 -- table name is validated via isSafeIdentifier and discovered from information_schema.
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE site_id = ?", table)
		var count int
		if err := q.QueryRowContext(ctx, query, siteID).Scan(&count); err != nil {
			return nil, fmt.Errorf("could not count references in %s: %w", table, err)
		}
		if count > 0 {
			slog.Warn("Site references remain", "table", table, "site_id", siteID, "count", count)
			refs = append(refs, fmt.Sprintf("%s(%d)", table, count))
		}
	}
	return refs, nil
}

func isSafeIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}
