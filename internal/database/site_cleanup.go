package database

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
	// DuckDB rewrites a referenced row for any UPDATE, even when its primary
	// key is unchanged. Keep these QR ownership relationships in the shared
	// graph instead of physical constraints so qr_codes.site_id can move during
	// a site-domain rename.
	{table: "qr_code_assets", column: "qr_code_id", referencedTable: "qr_codes", referencedColumn: "id"},
	{table: "qr_code_share_links", column: "qr_code_id", referencedTable: "qr_codes", referencedColumn: "id"},
}

var siteDeleteSpec = scopedDeleteSpec{
	scopeColumns: []string{"site_id"},
	rootTable:    "sites",
	extraEdges:   siteExtraEdges,
}

var siteStatsResetAnalyticsSteps = []siteStatsResetStep{
	{table: "imported_event_properties_daily", family: "imports", query: "DELETE FROM imported_event_properties_daily WHERE site_id = ?"},
	{table: "imported_event_dimensions_daily", family: "imports", query: "DELETE FROM imported_event_dimensions_daily WHERE site_id = ?"},
	{table: "imported_event_daily", family: "imports", query: "DELETE FROM imported_event_daily WHERE site_id = ?"},
	{table: "imported_dimension_daily", family: "imports", query: "DELETE FROM imported_dimension_daily WHERE site_id = ?"},
	{table: "imported_traffic_daily", family: "imports", query: "DELETE FROM imported_traffic_daily WHERE site_id = ?"},
	{table: "search_console_facts", family: "search_console", query: "DELETE FROM search_console_facts WHERE site_id = ?"},
	{table: "goal_rollups_hourly", family: "rollups", query: "DELETE FROM goal_rollups_hourly WHERE site_id = ?"},
	{table: "goal_rollups_daily", family: "rollups", query: "DELETE FROM goal_rollups_daily WHERE site_id = ?"},
	{table: "goal_rollups_monthly", family: "rollups", query: "DELETE FROM goal_rollups_monthly WHERE site_id = ?"},
	{table: "funnel_rollups_hourly", family: "rollups", query: "DELETE FROM funnel_rollups_hourly WHERE site_id = ?"},
	{table: "funnel_rollups_daily", family: "rollups", query: "DELETE FROM funnel_rollups_daily WHERE site_id = ?"},
	{table: "funnel_rollups_monthly", family: "rollups", query: "DELETE FROM funnel_rollups_monthly WHERE site_id = ?"},
	{table: "session_rollups_hourly", family: "rollups", query: "DELETE FROM session_rollups_hourly WHERE site_id = ?"},
	{table: "session_rollups_daily", family: "rollups", query: "DELETE FROM session_rollups_daily WHERE site_id = ?"},
	{table: "session_rollups_monthly", family: "rollups", query: "DELETE FROM session_rollups_monthly WHERE site_id = ?"},
	{table: "rollup_dirty_buckets", family: "rollups", query: "DELETE FROM rollup_dirty_buckets WHERE site_id = ?"},
	{table: "hit_rollups_hourly", family: "rollups", query: "DELETE FROM hit_rollups_hourly WHERE site_id = ?"},
	{table: "hit_rollups_daily", family: "rollups", query: "DELETE FROM hit_rollups_daily WHERE site_id = ?"},
	{table: "hit_rollups_monthly", family: "rollups", query: "DELETE FROM hit_rollups_monthly WHERE site_id = ?"},
	{table: "events", family: "native", query: "DELETE FROM events WHERE site_id = ?"},
	{table: "hits", family: "native", query: "DELETE FROM hits WHERE site_id = ?"},
	{table: "web_vitals", family: "web_vitals", query: "DELETE FROM web_vitals WHERE site_id = ?"},
	{table: "ai_fetches", family: "ai", query: "DELETE FROM ai_fetches WHERE site_id = ?"},
	{table: "qr_code_opens", family: "qr", query: "DELETE FROM qr_code_opens WHERE site_id = ?"},
	{table: "site_activity_hourly_counts", family: "activity", query: "DELETE FROM site_activity_hourly_counts WHERE site_id = ?"},
	{table: "site_activity_summary", family: "activity", query: "DELETE FROM site_activity_summary WHERE site_id = ?"},
}

var siteStatsResetSharedSteps = []siteStatsResetStep{
	{table: "opportunities", family: "ai", query: "DELETE FROM opportunities WHERE site_id = ?"},
	{table: "ai_runs", family: "ai", query: "DELETE FROM ai_runs WHERE site_id = ?"},
	{table: "site_activity_hourly_counts", family: "activity", query: "DELETE FROM site_activity_hourly_counts WHERE site_id = ?"},
	{table: "site_activity_summary", family: "activity", query: "DELETE FROM site_activity_summary WHERE site_id = ?"},
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

// deleteSiteAnalyticsOnly removes the site's rows from the given tables
// (children-first: tables arrives in the parents-first copy order and is
// walked in reverse) and optionally the mirrored site row afterwards.
func deleteSiteAnalyticsOnly(ctx context.Context, store *Store, siteID uuid.UUID, tables []string, deleteSiteRow bool) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin analytics cleanup transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for i := len(tables) - 1; i >= 0; i-- {
		table := tables[i]
		if !isSafeIdentifier(table) {
			return fmt.Errorf("unsafe table name %q", table)
		}
		// #nosec G201 -- table names are discovered from the live schema and validated via isSafeIdentifier.
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE site_id = ?", table), siteID); err != nil {
			return fmt.Errorf("could not delete analytics from %s: %w", table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit analytics cleanup transaction: %w", err)
	}

	if deleteSiteRow {
		// The mirrored site row is removed outside the cleanup transaction:
		// DuckDB's foreign key check still sees child rows deleted in the
		// same transaction and would reject the parent-row delete.
		if _, err := store.db.ExecContext(ctx, "DELETE FROM sites WHERE id = ?", siteID); err != nil {
			return fmt.Errorf("could not delete mirrored site row: %w", err)
		}
	}
	return nil
}

func (s *Store) ResetSiteStats(ctx context.Context, siteID uuid.UUID) (api.SiteStatsResetResponse, error) {
	result, err := s.resetSiteAnalyticsMeasurements(ctx, siteID)
	if err != nil {
		return result, err
	}
	sharedResult, err := s.resetSiteSharedMeasurements(ctx, siteID)
	if err != nil {
		return result, err
	}
	mergeSiteStatsResetResult(&result, sharedResult)
	return result, nil
}

func (s *Store) resetSiteAnalyticsMeasurements(ctx context.Context, siteID uuid.UUID) (api.SiteStatsResetResponse, error) {
	return s.resetSiteStatsWithSteps(ctx, siteID, siteStatsResetAnalyticsSteps, false)
}

func (s *Store) resetSiteSharedMeasurements(ctx context.Context, siteID uuid.UUID) (api.SiteStatsResetResponse, error) {
	return s.resetSiteStatsWithSteps(ctx, siteID, siteStatsResetSharedSteps, true)
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

func mergeSiteStatsResetResult(dst *api.SiteStatsResetResponse, src api.SiteStatsResetResponse) {
	if dst == nil {
		return
	}
	if strings.TrimSpace(dst.Status) == "" {
		dst.Status = "reset"
	}
	dst.RowsCleared += src.RowsCleared
	dst.ImportsMarkedDeleted += src.ImportsMarkedDeleted
	for _, family := range src.FamiliesCleared {
		addSiteStatsResetFamily(dst, family)
	}
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
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema NOT IN ('information_schema', 'pg_catalog')
			AND table_type = 'BASE TABLE'
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
	rows, err := q.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.columns
		WHERE column_name = 'site_id'
			AND table_schema NOT IN ('information_schema', 'pg_catalog')
	`)
	if err != nil {
		return nil, fmt.Errorf("could not list site tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("could not scan site table: %w", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list site tables: %w", err)
	}
	return tables, nil
}

func findSiteReferences(ctx context.Context, q queryer, siteID uuid.UUID, logger *slog.Logger) ([]string, error) {
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
			logger.Warn("Site references remain", "table", table, "site_id", siteID, "count", count)
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
