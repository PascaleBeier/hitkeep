package database

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"

	"hitkeep/api"
)

// breakdownDimensions maps each breakdown dimension to its hit expression,
// using the same classifications as the site overview's top lists.
var breakdownDimensions = map[string]string{
	"page":         "h.path",
	"referrer":     "hk_referrer(h.referrer)",
	"device":       "hk_device(h.viewport_width)",
	"country":      "hk_country(h.country_code)",
	"city":         "COALESCE(NULLIF(TRIM(h.city), ''), '(Unknown)')",
	"provider":     "COALESCE(NULLIF(TRIM(h.provider), ''), '(Unknown)')",
	"asn":          "hk_asn(h.asn, h.asn_org)",
	"language":     "CASE WHEN NULLIF(TRIM(h.language), '') IS NULL THEN '(Unspecified)' ELSE lower(split_part(TRIM(h.language), '-', 1)) END",
	"ai_source":    "hk_ai_source(h.referrer)",
	"utm_source":   "COALESCE(NULLIF(TRIM(h.utm_source), ''), '(Unspecified)')",
	"utm_medium":   "COALESCE(NULLIF(TRIM(h.utm_medium), ''), '(Unspecified)')",
	"utm_campaign": "COALESCE(NULLIF(TRIM(h.utm_campaign), ''), '(Unspecified)')",
	"utm_content":  "COALESCE(NULLIF(TRIM(h.utm_content), ''), '(Unspecified)')",
	"utm_term":     "COALESCE(NULLIF(TRIM(h.utm_term), ''), '(Unspecified)')",
}

// agentDimensions classify user agents. They are grouped by raw agent first
// and classified per distinct agent: the pattern walk is too slow per hit.
var agentDimensions = map[string]string{
	"browser": "hk_browser",
	"ai_bot":  "hk_ai_bot",
}

// BreakdownDimensions lists the dimensions GetDimensionBreakdown accepts.
func BreakdownDimensions() []string {
	names := slices.AppendSeq(slices.Collect(maps.Keys(breakdownDimensions)), maps.Keys(agentDimensions))
	slices.Sort(names)
	return names
}

// GetDimensionBreakdown counts pageviews and visitors per value of one
// dimension, largest first. It reads tracked hits only; imported history is
// not broken down.
func (s *Store) GetDimensionBreakdown(ctx context.Context, params api.AnalyticsParams, dimension string, limit int) ([]api.DimensionStat, error) {
	expr, isHit := breakdownDimensions[dimension]
	classify, isAgent := agentDimensions[dimension]
	if !isHit && !isAgent {
		return nil, fmt.Errorf("unknown breakdown dimension %q", dimension)
	}
	filterSQL, filterArgs := buildHitFilters(params.Filters, "h")
	sessionSQL, sessionArgs, err := s.buildSessionFilter(ctx, params, "h")
	if err != nil {
		return nil, err
	}
	//nolint:gosec // expr comes from breakdownDimensions and the filters from a fixed allowlist
	query := fmt.Sprintf(`
		SELECT %s AS name, COUNT(*) AS pageviews, COUNT(DISTINCT h.session_id) AS visitors
		FROM hits h
		WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s%s
		GROUP BY name
		HAVING name IS NOT NULL
		ORDER BY pageviews DESC, name
		LIMIT ?`, expr, filterSQL, sessionSQL)
	if isAgent {
		// A cookieless session belongs to one agent, so visitors sum exactly.
		//nolint:gosec // classify comes from agentDimensions and the filters from a fixed allowlist
		query = fmt.Sprintf(`
			WITH per_agent AS (
				SELECT h.user_agent, COUNT(*) AS pageviews, COUNT(DISTINCT h.session_id) AS visitors
				FROM hits h
				WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s%s
				GROUP BY h.user_agent
			)
			SELECT %s(user_agent) AS name, CAST(SUM(pageviews) AS BIGINT) AS pageviews, CAST(SUM(visitors) AS BIGINT) AS visitors
			FROM per_agent
			GROUP BY name
			ORDER BY pageviews DESC, name
			LIMIT ?`, filterSQL, sessionSQL, classify)
		// DuckDB pushes a filter on the classified name down to the hit scan,
		// which classifies every hit again, so the NULL group is dropped in Go.
		limit++
	}
	args := append([]any{params.SiteID, params.Start, params.End}, filterArgs...)
	args = append(append(args, sessionArgs...), limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query %s breakdown: %w", dimension, err)
	}
	defer rows.Close()
	stats := []api.DimensionStat{}
	for rows.Next() {
		var name sql.NullString
		var stat api.DimensionStat
		if err := rows.Scan(&name, &stat.Pageviews, &stat.Visitors); err != nil {
			return nil, err
		}
		if name.Valid {
			stat.Name = name.String
			stats = append(stats, stat)
		}
	}
	if isAgent {
		stats = stats[:min(len(stats), limit-1)]
	}
	return stats, rows.Err()
}
