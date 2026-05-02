package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

type siteDeleteStep struct {
	table string
	query string
}

var siteDeleteSteps = []siteDeleteStep{
	{table: "share_links", query: "DELETE FROM share_links WHERE site_id = ?"},
	{table: "site_activity_hourly_counts", query: "DELETE FROM site_activity_hourly_counts WHERE site_id = ?"},
	{table: "site_activity_summary", query: "DELETE FROM site_activity_summary WHERE site_id = ?"},
	{table: "site_exclusions", query: "DELETE FROM site_exclusions WHERE site_id = ?"},
	{table: "api_client_site_roles", query: "DELETE FROM api_client_site_roles WHERE site_id = ?"},
	{table: "site_members", query: "DELETE FROM site_members WHERE site_id = ?"},
	{table: "site_tenants", query: "DELETE FROM site_tenants WHERE site_id = ?"},
	{table: "site_import_files", query: "DELETE FROM site_import_files WHERE import_id IN (SELECT id FROM site_imports WHERE site_id = ?)"},
	{table: "site_imports", query: "DELETE FROM site_imports WHERE site_id = ?"},
	{table: "imported_event_properties_daily", query: "DELETE FROM imported_event_properties_daily WHERE site_id = ?"},
	{table: "imported_event_dimensions_daily", query: "DELETE FROM imported_event_dimensions_daily WHERE site_id = ?"},
	{table: "imported_event_daily", query: "DELETE FROM imported_event_daily WHERE site_id = ?"},
	{table: "imported_dimension_daily", query: "DELETE FROM imported_dimension_daily WHERE site_id = ?"},
	{table: "imported_traffic_daily", query: "DELETE FROM imported_traffic_daily WHERE site_id = ?"},
	{table: "goal_rollups_hourly", query: "DELETE FROM goal_rollups_hourly WHERE site_id = ?"},
	{table: "goal_rollups_daily", query: "DELETE FROM goal_rollups_daily WHERE site_id = ?"},
	{table: "goal_rollups_monthly", query: "DELETE FROM goal_rollups_monthly WHERE site_id = ?"},
	{table: "funnel_rollups_hourly", query: "DELETE FROM funnel_rollups_hourly WHERE site_id = ?"},
	{table: "funnel_rollups_daily", query: "DELETE FROM funnel_rollups_daily WHERE site_id = ?"},
	{table: "funnel_rollups_monthly", query: "DELETE FROM funnel_rollups_monthly WHERE site_id = ?"},
	{table: "session_rollups_hourly", query: "DELETE FROM session_rollups_hourly WHERE site_id = ?"},
	{table: "session_rollups_daily", query: "DELETE FROM session_rollups_daily WHERE site_id = ?"},
	{table: "session_rollups_monthly", query: "DELETE FROM session_rollups_monthly WHERE site_id = ?"},
	{table: "rollup_dirty_buckets", query: "DELETE FROM rollup_dirty_buckets WHERE site_id = ?"},
	{table: "hit_rollups_hourly", query: "DELETE FROM hit_rollups_hourly WHERE site_id = ?"},
	{table: "hit_rollups_daily", query: "DELETE FROM hit_rollups_daily WHERE site_id = ?"},
	{table: "hit_rollups_monthly", query: "DELETE FROM hit_rollups_monthly WHERE site_id = ?"},
	{table: "goals", query: "DELETE FROM goals WHERE site_id = ?"},
	{table: "funnels", query: "DELETE FROM funnels WHERE site_id = ?"},
	{table: "events", query: "DELETE FROM events WHERE site_id = ?"},
	{table: "hits", query: "DELETE FROM hits WHERE site_id = ?"},
	{table: "ai_fetches", query: "DELETE FROM ai_fetches WHERE site_id = ?"},
	{table: "site_report_subscriptions", query: "DELETE FROM site_report_subscriptions WHERE site_id = ?"},
}

var knownSiteDeleteTables = func() map[string]struct{} {
	known := make(map[string]struct{}, len(siteDeleteSteps))
	for _, step := range siteDeleteSteps {
		known[step.table] = struct{}{}
	}
	return known
}()

var siteAnalyticsDeleteSteps = []siteDeleteStep{
	{table: "imported_event_properties_daily", query: "DELETE FROM imported_event_properties_daily WHERE site_id = ?"},
	{table: "imported_event_dimensions_daily", query: "DELETE FROM imported_event_dimensions_daily WHERE site_id = ?"},
	{table: "imported_event_daily", query: "DELETE FROM imported_event_daily WHERE site_id = ?"},
	{table: "imported_dimension_daily", query: "DELETE FROM imported_dimension_daily WHERE site_id = ?"},
	{table: "imported_traffic_daily", query: "DELETE FROM imported_traffic_daily WHERE site_id = ?"},
	{table: "goal_rollups_hourly", query: "DELETE FROM goal_rollups_hourly WHERE site_id = ?"},
	{table: "goal_rollups_daily", query: "DELETE FROM goal_rollups_daily WHERE site_id = ?"},
	{table: "goal_rollups_monthly", query: "DELETE FROM goal_rollups_monthly WHERE site_id = ?"},
	{table: "funnel_rollups_hourly", query: "DELETE FROM funnel_rollups_hourly WHERE site_id = ?"},
	{table: "funnel_rollups_daily", query: "DELETE FROM funnel_rollups_daily WHERE site_id = ?"},
	{table: "funnel_rollups_monthly", query: "DELETE FROM funnel_rollups_monthly WHERE site_id = ?"},
	{table: "session_rollups_hourly", query: "DELETE FROM session_rollups_hourly WHERE site_id = ?"},
	{table: "session_rollups_daily", query: "DELETE FROM session_rollups_daily WHERE site_id = ?"},
	{table: "session_rollups_monthly", query: "DELETE FROM session_rollups_monthly WHERE site_id = ?"},
	{table: "rollup_dirty_buckets", query: "DELETE FROM rollup_dirty_buckets WHERE site_id = ?"},
	{table: "hit_rollups_hourly", query: "DELETE FROM hit_rollups_hourly WHERE site_id = ?"},
	{table: "hit_rollups_daily", query: "DELETE FROM hit_rollups_daily WHERE site_id = ?"},
	{table: "hit_rollups_monthly", query: "DELETE FROM hit_rollups_monthly WHERE site_id = ?"},
	{table: "goals", query: "DELETE FROM goals WHERE site_id = ?"},
	{table: "funnels", query: "DELETE FROM funnels WHERE site_id = ?"},
	{table: "events", query: "DELETE FROM events WHERE site_id = ?"},
	{table: "hits", query: "DELETE FROM hits WHERE site_id = ?"},
	{table: "ai_fetches", query: "DELETE FROM ai_fetches WHERE site_id = ?"},
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func deleteSiteChildren(ctx context.Context, tx *sql.Tx, siteID uuid.UUID) error {
	existingTables, err := listTables(ctx, tx)
	if err != nil {
		return err
	}

	for _, step := range siteDeleteSteps {
		if _, ok := existingTables[step.table]; !ok {
			continue
		}
		if !isSafeIdentifier(step.table) {
			return fmt.Errorf("unsafe table name %q", step.table)
		}
		if _, err := tx.ExecContext(ctx, step.query, siteID); err != nil {
			return fmt.Errorf("could not delete from %s: %w", step.table, err)
		}
	}

	siteTables, err := listSiteIDTables(ctx, tx)
	if err != nil {
		return err
	}

	for _, table := range siteTables {
		if table == "sites" {
			continue
		}
		if _, ok := knownSiteDeleteTables[table]; ok {
			continue
		}
		if !isSafeIdentifier(table) {
			return fmt.Errorf("unsafe table name %q", table)
		}
		slog.Info("Deleting site data from unexpected table", "table", table, "site_id", siteID)
		// #nosec G201 -- table name is validated via isSafeIdentifier and discovered from information_schema.
		query := fmt.Sprintf("DELETE FROM %s WHERE site_id = ?", table)
		if _, err := tx.ExecContext(ctx, query, siteID); err != nil {
			return fmt.Errorf("could not delete from %s: %w", table, err)
		}
	}

	return nil
}

func deleteSiteAnalyticsOnly(ctx context.Context, store *Store, siteID uuid.UUID, deleteSiteRow bool) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin analytics cleanup transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	existingTables, err := listTables(ctx, tx)
	if err != nil {
		return err
	}

	for _, step := range siteAnalyticsDeleteSteps {
		if _, ok := existingTables[step.table]; !ok {
			continue
		}
		if !isSafeIdentifier(step.table) {
			return fmt.Errorf("unsafe table name %q", step.table)
		}
		if _, err := tx.ExecContext(ctx, step.query, siteID); err != nil {
			return fmt.Errorf("could not delete analytics from %s: %w", step.table, err)
		}
	}

	if deleteSiteRow {
		if _, ok := existingTables["sites"]; ok {
			if _, err := tx.ExecContext(ctx, "DELETE FROM sites WHERE id = ?", siteID); err != nil {
				return fmt.Errorf("could not delete mirrored site row: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit analytics cleanup transaction: %w", err)
	}
	return nil
}

func listTables(ctx context.Context, q queryer) (map[string]struct{}, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema NOT IN ('information_schema', 'pg_catalog')
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
