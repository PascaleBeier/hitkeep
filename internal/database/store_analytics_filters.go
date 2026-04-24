package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func (s *Store) buildSessionFilter(ctx context.Context, params api.AnalyticsParams, alias string) (string, []any, error) {
	if len(params.GoalIDs) == 0 && len(params.FunnelIDs) == 0 {
		return "", nil, nil
	}

	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}

	var clauses []string
	var args []any

	if len(params.GoalIDs) > 0 {
		goals, err := s.GetGoals(ctx, params.SiteID)
		if err != nil {
			return "", nil, fmt.Errorf("failed to load goals: %w", err)
		}
		allowed := make(map[uuid.UUID]struct{}, len(params.GoalIDs))
		for _, id := range params.GoalIDs {
			allowed[id] = struct{}{}
		}

		var pathValues []string
		var eventValues []string
		for _, goal := range goals {
			if _, ok := allowed[goal.ID]; !ok {
				continue
			}
			switch goal.Type {
			case "path":
				pathValues = append(pathValues, goal.Value)
			case "event":
				eventValues = append(eventValues, goal.Value)
			}
		}

		if len(pathValues) == 0 && len(eventValues) == 0 {
			return " AND 1=0", nil, nil
		}

		subquery, subArgs := buildSessionUnionSubquery(params.SiteID, params.Start, params.End, pathValues, eventValues)
		clauses = append(clauses, fmt.Sprintf("%ssession_id IN (%s)", prefix, subquery))
		args = append(args, subArgs...)
	}

	if len(params.FunnelIDs) > 0 {
		funnels, err := s.GetFunnels(ctx, params.SiteID)
		if err != nil {
			return "", nil, fmt.Errorf("failed to load funnels: %w", err)
		}
		allowed := make(map[uuid.UUID]struct{}, len(params.FunnelIDs))
		for _, id := range params.FunnelIDs {
			allowed[id] = struct{}{}
		}

		var entryPathValues []string
		var entryEventValues []string
		for _, funnel := range funnels {
			if _, ok := allowed[funnel.ID]; !ok {
				continue
			}
			if len(funnel.Steps) == 0 {
				continue
			}
			first := funnel.Steps[0]
			switch first.Type {
			case "path":
				entryPathValues = append(entryPathValues, first.Value)
			case "event":
				entryEventValues = append(entryEventValues, first.Value)
			}
		}

		if len(entryPathValues) == 0 && len(entryEventValues) == 0 {
			return " AND 1=0", nil, nil
		}

		subquery, subArgs := buildSessionUnionSubquery(params.SiteID, params.Start, params.End, entryPathValues, entryEventValues)
		clauses = append(clauses, fmt.Sprintf("%ssession_id IN (%s)", prefix, subquery))
		args = append(args, subArgs...)
	}

	if len(clauses) == 0 {
		return "", nil, nil
	}

	return " AND " + strings.Join(clauses, " AND "), args, nil
}

func (s *Store) buildFunnelPathFilter(ctx context.Context, params api.AnalyticsParams, alias string) (string, []any, error) {
	if len(params.FunnelIDs) == 0 {
		return "", nil, nil
	}

	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}

	funnels, err := s.GetFunnels(ctx, params.SiteID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to load funnels: %w", err)
	}
	allowed := make(map[uuid.UUID]struct{}, len(params.FunnelIDs))
	for _, id := range params.FunnelIDs {
		allowed[id] = struct{}{}
	}

	pathSet := make(map[string]struct{})
	for _, funnel := range funnels {
		if _, ok := allowed[funnel.ID]; !ok {
			continue
		}
		for _, step := range funnel.Steps {
			if step.Type == "path" && step.Value != "" {
				pathSet[step.Value] = struct{}{}
			}
		}
	}

	if len(pathSet) == 0 {
		return " AND 1=0", nil, nil
	}

	values := make([]string, 0, len(pathSet))
	for value := range pathSet {
		values = append(values, value)
	}

	placeholders := buildPlaceholders(len(values))
	args := make([]any, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}

	return fmt.Sprintf(" AND %spath IN (%s)", prefix, placeholders), args, nil
}

func (s *Store) queryChartData(
	ctx context.Context,
	params api.AnalyticsParams,
	gridStart time.Time,
	gridEnd time.Time,
	interval string,
	truncUnit string,
	filterSQL string,
	filterArgs []any,
	useRollups bool,
	rollupKind rollupKind,
) (*sql.Rows, error) {
	if useRollups {
		return s.queryRollupChart(ctx, params.SiteID, gridStart, gridEnd, interval, truncUnit, rollupKind)
	}

	//nolint:gosec // interval/truncUnit/filterSQL are derived from fixed allowlists
	chartQuery := fmt.Sprintf(`
	WITH time_range AS (
		SELECT unnest(generate_series(?::TIMESTAMP, ?::TIMESTAMP, INTERVAL %s)) as bucket
	),
	daily_hits AS (
		SELECT 
			date_trunc('%s', timestamp)::TIMESTAMP as bucket,
			COUNT(*) as pageviews,
			COUNT(DISTINCT session_id) as visitors
		FROM hits h
		WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s
		GROUP BY bucket
	)
	SELECT 
		tr.bucket,
		COALESCE(dh.pageviews, 0),
		COALESCE(dh.visitors, 0)
	FROM time_range tr
	LEFT JOIN daily_hits dh ON tr.bucket = dh.bucket
	ORDER BY tr.bucket ASC;
	`, interval, truncUnit, filterSQL)

	return s.db.QueryContext(ctx, chartQuery, append([]any{gridStart, gridEnd, params.SiteID, params.Start, params.End}, filterArgs...)...)
}

func (s *Store) queryKpis(
	ctx context.Context,
	params api.AnalyticsParams,
	filterSQL string,
	filterArgs []any,
	useRollups bool,
	kind rollupKind,
	totalPageviews *int,
	uniqueSessions *int,
	bounceRate *float64,
	avgDuration *float64,
	pagesPerSession *float64,
) error {
	_ = useRollups
	_ = kind

	totals, err := s.queryRawSessionKpis(ctx, params, filterSQL, filterArgs)
	if err != nil {
		return err
	}

	*totalPageviews = totals.TotalPageviews
	*uniqueSessions = totals.UniqueSessions
	if totals.UniqueSessions == 0 {
		*bounceRate = 0
		*avgDuration = 0
		*pagesPerSession = 0
		return nil
	}

	*bounceRate = (float64(totals.BouncedSessions) / float64(totals.UniqueSessions)) * 100
	*avgDuration = totals.DurationSumSeconds / float64(totals.UniqueSessions)
	*pagesPerSession = float64(totals.TotalPageviews) / float64(totals.UniqueSessions)
	return nil
}

type sessionKpiTotals struct {
	TotalPageviews     int
	UniqueSessions     int
	BouncedSessions    int
	DurationSumSeconds float64
}

func (s *Store) queryRawSessionKpis(
	ctx context.Context,
	params api.AnalyticsParams,
	filterSQL string,
	filterArgs []any,
) (sessionKpiTotals, error) {
	//nolint:gosec // filterSQL is derived from a fixed allowlist
	kpiQuery := fmt.Sprintf(`
	WITH session_metrics AS (
		SELECT 
			session_id,
			count(*) as pvs,
			(MAX(timestamp) - MIN(timestamp)) as duration
		FROM hits h
		WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s
		GROUP BY session_id
	)
	SELECT 
		COALESCE(SUM(pvs), 0) as total_pageviews,
		COUNT(session_id) as unique_sessions,
		COALESCE(SUM(CASE WHEN pvs = 1 THEN 1 ELSE 0 END), 0) as bounced_sessions,
		COALESCE(SUM(EXTRACT('epoch' FROM duration)), 0) as duration_sum_seconds
	FROM session_metrics;
	`, filterSQL)

	var totals sessionKpiTotals
	err := s.db.QueryRowContext(ctx, kpiQuery, append([]any{params.SiteID, params.Start, params.End}, filterArgs...)...).Scan(
		&totals.TotalPageviews,
		&totals.UniqueSessions,
		&totals.BouncedSessions,
		&totals.DurationSumSeconds,
	)
	return totals, err
}

func (s *Store) queryUTMKpis(
	ctx context.Context,
	params api.AnalyticsParams,
	filterSQL string,
	filterArgs []any,
	utmCampaignHits *int,
	utmContentHits *int,
	utmMediumHits *int,
	utmSourceHits *int,
	utmTermHits *int,
) error {
	//nolint:gosec // filterSQL is derived from a fixed allowlist
	query := fmt.Sprintf(`
		SELECT
			COALESCE(SUM(CASE WHEN NULLIF(TRIM(h.utm_campaign), '') IS NOT NULL THEN 1 ELSE 0 END), 0) AS utm_campaign_hits,
			COALESCE(SUM(CASE WHEN NULLIF(TRIM(h.utm_content), '') IS NOT NULL THEN 1 ELSE 0 END), 0) AS utm_content_hits,
			COALESCE(SUM(CASE WHEN NULLIF(TRIM(h.utm_medium), '') IS NOT NULL THEN 1 ELSE 0 END), 0) AS utm_medium_hits,
			COALESCE(SUM(CASE WHEN NULLIF(TRIM(h.utm_source), '') IS NOT NULL THEN 1 ELSE 0 END), 0) AS utm_source_hits,
			COALESCE(SUM(CASE WHEN NULLIF(TRIM(h.utm_term), '') IS NOT NULL THEN 1 ELSE 0 END), 0) AS utm_term_hits
		FROM hits h
		WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s
	`, filterSQL)

	return s.db.QueryRowContext(ctx, query, append([]any{params.SiteID, params.Start, params.End}, filterArgs...)...).Scan(
		utmCampaignHits,
		utmContentHits,
		utmMediumHits,
		utmSourceHits,
		utmTermHits,
	)
}

func buildPlaceholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func buildSessionUnionSubquery(siteID uuid.UUID, start time.Time, end time.Time, pathValues []string, eventValues []string) (string, []any) {
	var parts []string
	var args []any

	if len(pathValues) > 0 {
		parts = append(parts, buildSessionValueSubquery("hits", "path", len(pathValues)))
		args = append(args, siteID, start, end)
		for _, value := range pathValues {
			args = append(args, value)
		}
	}

	if len(eventValues) > 0 {
		parts = append(parts, buildSessionValueSubquery("events", "name", len(eventValues)))
		args = append(args, siteID, start, end)
		for _, value := range eventValues {
			args = append(args, value)
		}
	}

	return strings.Join(parts, " UNION "), args
}

func buildSessionValueSubquery(table string, field string, valueCount int) string {
	placeholders := buildPlaceholders(valueCount)
	//nolint:gosec // table/field are fixed allowlists in call sites
	return fmt.Sprintf(
		"SELECT DISTINCT session_id FROM %s WHERE site_id = ? AND timestamp >= ? AND timestamp <= ? AND %s IN (%s)",
		table,
		field,
		placeholders,
	)
}

func (s *Store) querySeriesCounts(ctx context.Context, table string, valueField string, values []string, params api.AnalyticsParams, truncUnit string) (map[time.Time]int, error) {
	result := make(map[time.Time]int)
	if len(values) == 0 {
		return result, nil
	}

	placeholders := buildPlaceholders(len(values))
	args := make([]any, 0, 3+len(values))
	args = append(args, params.SiteID, params.Start, params.End)
	for _, v := range values {
		args = append(args, v)
	}

	//nolint:gosec // table/valueField/truncUnit are from fixed allowlists
	query := fmt.Sprintf(`
		SELECT date_trunc('%s', timestamp) AS bucket, COUNT(DISTINCT session_id) AS conversions
		FROM %s
		WHERE site_id = ? AND timestamp >= ? AND timestamp <= ? AND %s IN (%s)
		GROUP BY bucket
		ORDER BY bucket
	`, truncUnit, table, valueField, placeholders)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var bucket time.Time
		var count int
		if err := rows.Scan(&bucket, &count); err != nil {
			return nil, err
		}
		result[truncToUnit(bucket, truncUnit)] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read conversion count rows: %w", err)
	}

	return result, nil
}
