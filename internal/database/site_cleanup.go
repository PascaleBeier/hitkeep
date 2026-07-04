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

type siteDeleteStep struct {
	table string
	query string
}

type siteStatsResetStep struct {
	table  string
	family string
	query  string
}

var siteDeleteSteps = []siteDeleteStep{
	{table: "share_links", query: "DELETE FROM share_links WHERE site_id = ?"},
	{table: "opportunities", query: "DELETE FROM opportunities WHERE site_id = ?"},
	{table: "ai_runs", query: "DELETE FROM ai_runs WHERE site_id = ?"},
	{table: "site_activity_hourly_counts", query: "DELETE FROM site_activity_hourly_counts WHERE site_id = ?"},
	{table: "site_activity_summary", query: "DELETE FROM site_activity_summary WHERE site_id = ?"},
	{table: "site_exclusions", query: "DELETE FROM site_exclusions WHERE site_id = ?"},
	{table: "site_country_exclusions", query: "DELETE FROM site_country_exclusions WHERE site_id = ?"},
	{table: "api_client_site_roles", query: "DELETE FROM api_client_site_roles WHERE site_id = ?"},
	{table: "google_search_console_sync_state", query: "DELETE FROM google_search_console_sync_state WHERE site_id = ?"},
	{table: "google_search_console_site_mappings", query: "DELETE FROM google_search_console_site_mappings WHERE site_id = ?"},
	{table: "site_members", query: "DELETE FROM site_members WHERE site_id = ?"},
	{table: "site_tenants", query: "DELETE FROM site_tenants WHERE site_id = ?"},
	{table: "site_import_files", query: "DELETE FROM site_import_files WHERE import_id IN (SELECT id FROM site_imports WHERE site_id = ?)"},
	{table: "site_imports", query: "DELETE FROM site_imports WHERE site_id = ?"},
	{table: "imported_event_properties_daily", query: "DELETE FROM imported_event_properties_daily WHERE site_id = ?"},
	{table: "imported_event_dimensions_daily", query: "DELETE FROM imported_event_dimensions_daily WHERE site_id = ?"},
	{table: "imported_event_daily", query: "DELETE FROM imported_event_daily WHERE site_id = ?"},
	{table: "imported_dimension_daily", query: "DELETE FROM imported_dimension_daily WHERE site_id = ?"},
	{table: "imported_traffic_daily", query: "DELETE FROM imported_traffic_daily WHERE site_id = ?"},
	{table: "search_console_facts", query: "DELETE FROM search_console_facts WHERE site_id = ?"},
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
	{table: "web_vitals", query: "DELETE FROM web_vitals WHERE site_id = ?"},
	{table: "ai_fetches", query: "DELETE FROM ai_fetches WHERE site_id = ?"},
	{table: "qr_code_opens", query: "DELETE FROM qr_code_opens WHERE site_id = ?"},
	{table: "qr_code_assets", query: "DELETE FROM qr_code_assets WHERE site_id = ?"},
	{table: "qr_code_share_links", query: "DELETE FROM qr_code_share_links WHERE site_id = ?"},
	{table: "qr_codes", query: "DELETE FROM qr_codes WHERE site_id = ?"},
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
	{table: "search_console_facts", query: "DELETE FROM search_console_facts WHERE site_id = ?"},
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
	{table: "web_vitals", query: "DELETE FROM web_vitals WHERE site_id = ?"},
	{table: "ai_fetches", query: "DELETE FROM ai_fetches WHERE site_id = ?"},
	{table: "qr_code_opens", query: "DELETE FROM qr_code_opens WHERE site_id = ?"},
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
