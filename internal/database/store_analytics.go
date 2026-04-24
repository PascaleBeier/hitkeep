package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"hitkeep/internal/api"
)

// GetSiteStats returns aggregated KPIs and time-series data using the AnalyticsParams struct.
func (s *Store) GetSiteStats(ctx context.Context, params api.AnalyticsParams) (*api.SiteStats, error) {
	// Authorization is handled by the handler middleware (SitePerm/RequirePermission).
	// Tenant analytics stores do not contain the control-plane sites table.

	stats := &api.SiteStats{
		ChartData:       []api.ChartDataPoint{},
		TopPages:        []api.MetricStat{},
		TopLandingPages: []api.MetricStat{},
		TopExitPages:    []api.MetricStat{},
		TopReferrers:    []api.MetricStat{},
		TopDevices:      []api.MetricStat{},
		TopCountries:    []api.MetricStat{},
		TopBrowsers:     []api.MetricStat{},
		TopAIBots:       []api.MetricStat{},
		TopAISources:    []api.MetricStat{},
		TopLanguages:    []api.MetricStat{},
		TopUTMCampaigns: []api.MetricStat{},
		TopUTMContents:  []api.MetricStat{},
		TopUTMMediums:   []api.MetricStat{},
		TopUTMSources:   []api.MetricStat{},
		TopUTMTerms:     []api.MetricStat{},
		Goals:           []api.GoalStats{},
	}

	filterSQL, filterArgs := buildHitFilters(params.Filters, "h")
	funnelPathSQL, funnelPathArgs, err := s.buildFunnelPathFilter(ctx, params, "h")
	if err != nil {
		return nil, err
	}
	sessionSQL, sessionArgs, err := s.buildSessionFilter(ctx, params, "h")
	if err != nil {
		return nil, err
	}
	filterSQL += funnelPathSQL
	filterSQL += sessionSQL
	filterArgs = append(filterArgs, funnelPathArgs...)
	filterArgs = append(filterArgs, sessionArgs...)
	useRollups := len(params.Filters) == 0
	if sessionSQL != "" || funnelPathSQL != "" {
		useRollups = false
	}

	liveThreshold := time.Now().Add(-5 * time.Minute)
	liveQuery := "SELECT COUNT(DISTINCT h.session_id) FROM hits h WHERE h.site_id = ? AND h.timestamp >= ?" + filterSQL
	err = s.db.QueryRowContext(ctx, liveQuery, append([]any{params.SiteID, liveThreshold}, filterArgs...)...).Scan(&stats.LiveVisitors)
	if err != nil {
		return nil, fmt.Errorf("failed to calc live visitors: %w", err)
	}

	duration := params.End.Sub(params.Start)
	interval := "1 DAY"
	truncUnit := "day"
	rollupKind := rollupHourly

	var gridStart, gridEnd time.Time

	if duration < 48*time.Hour {
		interval = "1 HOUR"
		truncUnit = "hour"
		gridStart = params.Start.Truncate(time.Hour)
		gridEnd = params.End.Truncate(time.Hour)
		if !gridEnd.After(params.End) {
			gridEnd = gridEnd.Add(time.Hour)
		}
	} else {
		y, m, d := params.Start.Date()
		gridStart = time.Date(y, m, d, 0, 0, 0, 0, params.Start.Location())

		y, m, d = params.End.Date()
		gridEnd = time.Date(y, m, d, 0, 0, 0, 0, params.End.Location())
		if !gridEnd.After(params.End) {
			gridEnd = gridEnd.AddDate(0, 0, 1)
		}
		if duration >= 180*24*time.Hour {
			interval = "1 MONTH"
			truncUnit = "month"
			rollupKind = rollupMonthly
			gridStart = time.Date(gridStart.Year(), gridStart.Month(), 1, 0, 0, 0, 0, gridStart.Location())
			gridEnd = time.Date(gridEnd.Year(), gridEnd.Month(), 1, 0, 0, 0, 0, gridEnd.Location())
			if !gridEnd.After(params.End) {
				gridEnd = gridEnd.AddDate(0, 1, 0)
			}
		} else {
			rollupKind = rollupDaily
		}
	}

	if useRollups {
		if err := s.refreshDirtyRollupsInRange(ctx, params.SiteID, dirtyRollupSession, rollupKind, gridStart, gridEnd); err != nil {
			return nil, fmt.Errorf("failed to refresh session rollups: %w", err)
		}
	}

	err = s.queryKpis(ctx, params, filterSQL, filterArgs, useRollups, rollupKind, &stats.TotalPageviews, &stats.UniqueSessions, &stats.BounceRate, &stats.AvgSessionDuration, &stats.PagesPerSession)
	if err != nil {
		return nil, fmt.Errorf("failed to calc KPIs: %w", err)
	}
	err = s.queryUTMKpis(
		ctx,
		params,
		filterSQL,
		filterArgs,
		&stats.UTMCampaignHits,
		&stats.UTMContentHits,
		&stats.UTMMediumHits,
		&stats.UTMSourceHits,
		&stats.UTMTermHits,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to calc UTM KPIs: %w", err)
	}

	if useRollups {
		stats.ChartData, err = s.queryHybridChartData(ctx, params, truncUnit, rollupKind)
		if err != nil {
			return nil, fmt.Errorf("failed to query hybrid chart data: %w", err)
		}
	} else {
		rows, err := s.queryChartData(ctx, params, gridStart, gridEnd, interval, truncUnit, filterSQL, filterArgs, useRollups, rollupKind)
		if err != nil {
			return nil, fmt.Errorf("failed to query chart data: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var p api.ChartDataPoint
			if err := rows.Scan(&p.Time, &p.Pageviews, &p.Visitors); err != nil {
				return nil, err
			}
			stats.ChartData = append(stats.ChartData, p)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("failed to read chart data rows: %w", err)
		}
	}

	// Top lists via GROUPING SETS to keep a single scan.
	//nolint:gosec // filterSQL is derived from a fixed allowlist
	topQuery := fmt.Sprintf(`
		WITH base AS (
			SELECT
				h.path AS path,
				hk_referrer(h.referrer) AS referrer,
				hk_device(h.viewport_width) AS device,
				hk_country(h.country_code) AS country,
				hk_browser(h.user_agent) AS browser,
				hk_ai_bot(h.user_agent) AS ai_bot,
				hk_ai_source(h.referrer) AS ai_source,
				h.session_id AS session_id,
				CASE
					WHEN NULLIF(TRIM(h.language), '') IS NULL THEN '(Unspecified)'
					ELSE lower(split_part(TRIM(h.language), '-', 1))
				END AS language,
				COALESCE(NULLIF(TRIM(h.utm_campaign), ''), '(Unspecified)') AS utm_campaign,
				COALESCE(NULLIF(TRIM(h.utm_content), ''), '(Unspecified)') AS utm_content,
				COALESCE(NULLIF(TRIM(h.utm_medium), ''), '(Unspecified)') AS utm_medium,
				COALESCE(NULLIF(TRIM(h.utm_source), ''), '(Unspecified)') AS utm_source,
				COALESCE(NULLIF(TRIM(h.utm_term), ''), '(Unspecified)') AS utm_term
			FROM hits h
			WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s
		),
		agg AS (
			SELECT
				CASE
					WHEN GROUPING(path) = 0 THEN 'path'
					WHEN GROUPING(referrer) = 0 THEN 'referrer'
					WHEN GROUPING(device) = 0 THEN 'device'
					WHEN GROUPING(country) = 0 THEN 'country'
					WHEN GROUPING(browser) = 0 THEN 'browser'
					WHEN GROUPING(ai_bot) = 0 THEN 'ai_bot'
					WHEN GROUPING(language) = 0 THEN 'language'
					WHEN GROUPING(utm_campaign) = 0 THEN 'utm_campaign'
					WHEN GROUPING(utm_content) = 0 THEN 'utm_content'
					WHEN GROUPING(utm_medium) = 0 THEN 'utm_medium'
					WHEN GROUPING(utm_source) = 0 THEN 'utm_source'
					WHEN GROUPING(utm_term) = 0 THEN 'utm_term'
					ELSE '__summary__'
				END AS dim,
				COALESCE(path, referrer, device, country, browser, ai_bot, language, utm_campaign, utm_content, utm_medium, utm_source, utm_term) AS name,
				COUNT(*) AS val,
				COUNT(*) FILTER (WHERE ai_bot IS NOT NULL) AS ai_bot_hits,
				COUNT(DISTINCT session_id) FILTER (WHERE ai_source IS NOT NULL) AS ai_source_visits
			FROM base
			GROUP BY GROUPING SETS (
				(path),
				(referrer),
				(device),
				(country),
				(browser),
				(ai_bot),
				(language),
				(utm_campaign),
				(utm_content),
				(utm_medium),
				(utm_source),
				(utm_term),
				()
			)
		),
		ai_source_agg AS (
			SELECT
				'ai_source' AS dim,
				ai_source AS name,
				COUNT(DISTINCT session_id) AS val,
				NULL AS ai_bot_hits,
				NULL AS ai_source_visits
			FROM base
			WHERE ai_source IS NOT NULL
			GROUP BY ai_source
		),
		ranked AS (
			SELECT
				dim,
				name,
				val,
				ai_bot_hits,
				ai_source_visits,
				ROW_NUMBER() OVER (PARTITION BY dim ORDER BY val DESC) AS rn
			FROM (
				SELECT dim, name, val, ai_bot_hits, ai_source_visits FROM agg
				UNION ALL
				SELECT dim, name, val, ai_bot_hits, ai_source_visits FROM ai_source_agg
			)
			WHERE dim = '__summary__' OR name IS NOT NULL
		)
		SELECT dim, name, val, ai_bot_hits, ai_source_visits
		FROM ranked
		WHERE dim = '__summary__' OR rn <= 10
		ORDER BY CASE WHEN dim = '__summary__' THEN 0 ELSE 1 END, dim, val DESC;
	`, filterSQL)

	topRows, err := s.db.QueryContext(ctx, topQuery, append([]any{params.SiteID, params.Start, params.End}, filterArgs...)...)
	if err != nil {
		return nil, err
	}
	defer topRows.Close()

	for topRows.Next() {
		var dim string
		var name sql.NullString
		var value sql.NullInt64
		var aiBotHits sql.NullInt64
		var aiSourceVisits sql.NullInt64
		if err := topRows.Scan(&dim, &name, &value, &aiBotHits, &aiSourceVisits); err != nil {
			return nil, err
		}
		if dim == "__summary__" {
			if aiBotHits.Valid {
				stats.AIBotHits = int(aiBotHits.Int64)
			}
			if aiSourceVisits.Valid {
				stats.AISourceVisits = int(aiSourceVisits.Int64)
			}
			continue
		}
		if !name.Valid || !value.Valid {
			continue
		}
		m := api.MetricStat{
			Name:  name.String,
			Value: int(value.Int64),
		}
		switch dim {
		case "path":
			stats.TopPages = append(stats.TopPages, m)
		case "referrer":
			stats.TopReferrers = append(stats.TopReferrers, m)
		case "device":
			stats.TopDevices = append(stats.TopDevices, m)
		case "country":
			stats.TopCountries = append(stats.TopCountries, m)
		case "browser":
			stats.TopBrowsers = append(stats.TopBrowsers, m)
		case "ai_bot":
			stats.TopAIBots = append(stats.TopAIBots, m)
		case "ai_source":
			stats.TopAISources = append(stats.TopAISources, m)
		case "language":
			stats.TopLanguages = append(stats.TopLanguages, m)
		case "utm_campaign":
			stats.TopUTMCampaigns = append(stats.TopUTMCampaigns, m)
		case "utm_content":
			stats.TopUTMContents = append(stats.TopUTMContents, m)
		case "utm_medium":
			stats.TopUTMMediums = append(stats.TopUTMMediums, m)
		case "utm_source":
			stats.TopUTMSources = append(stats.TopUTMSources, m)
		case "utm_term":
			stats.TopUTMTerms = append(stats.TopUTMTerms, m)
		}
	}
	if err := topRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read top metric rows: %w", err)
	}

	//nolint:gosec // filterSQL is derived from a fixed allowlist
	landingExitQuery := fmt.Sprintf(`
		WITH matching_sessions AS (
			SELECT DISTINCT h.session_id
			FROM hits h
			WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s
		),
		session_hits AS (
			SELECT
				h.session_id,
				h.path,
				h.timestamp,
				h.page_id
			FROM hits h
			INNER JOIN matching_sessions ms ON ms.session_id = h.session_id
			WHERE h.site_id = ?
		),
		ranked_hits AS (
			SELECT
				session_id,
				path,
				ROW_NUMBER() OVER (
					PARTITION BY session_id
					ORDER BY timestamp ASC, path ASC, page_id ASC
				) AS landing_rn,
				ROW_NUMBER() OVER (
					PARTITION BY session_id
					ORDER BY timestamp DESC, path DESC, page_id DESC
				) AS exit_rn
			FROM session_hits
		),
		aggregated AS (
			SELECT 'landing' AS kind, path AS name, COUNT(*) AS val
			FROM ranked_hits
			WHERE landing_rn = 1
			GROUP BY path
			UNION ALL
			SELECT 'exit' AS kind, path AS name, COUNT(*) AS val
			FROM ranked_hits
			WHERE exit_rn = 1
			GROUP BY path
		),
		ranked AS (
			SELECT
				kind,
				name,
				val,
				ROW_NUMBER() OVER (PARTITION BY kind ORDER BY val DESC, name ASC) AS rn
			FROM aggregated
		)
		SELECT kind, name, val
		FROM ranked
		WHERE rn <= 10
		ORDER BY kind, val DESC, name ASC;
	`, filterSQL)

	landingExitArgs := append([]any{params.SiteID, params.Start, params.End}, filterArgs...)
	landingExitArgs = append(landingExitArgs, params.SiteID)
	landingExitRows, err := s.db.QueryContext(ctx, landingExitQuery, landingExitArgs...)
	if err != nil {
		return nil, err
	}
	defer landingExitRows.Close()

	for landingExitRows.Next() {
		var kind string
		var m api.MetricStat
		if err := landingExitRows.Scan(&kind, &m.Name, &m.Value); err != nil {
			return nil, err
		}
		switch kind {
		case "landing":
			stats.TopLandingPages = append(stats.TopLandingPages, m)
		case "exit":
			stats.TopExitPages = append(stats.TopExitPages, m)
		}
	}
	if err := landingExitRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read landing and exit rows: %w", err)
	}

	goals, err := s.GetGoals(ctx, params.SiteID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch goals: %w", err)
	}

	for _, goal := range goals {
		var conversions int
		var err error

		switch goal.Type {
		case "path":
			err = s.db.QueryRowContext(ctx, `
				SELECT COUNT(DISTINCT session_id)
				FROM hits
				WHERE site_id = ? AND timestamp >= ? AND timestamp <= ? AND path = ?
			`, params.SiteID, params.Start, params.End, goal.Value).Scan(&conversions)
		case "event":
			err = s.db.QueryRowContext(ctx, `
				SELECT COUNT(DISTINCT session_id)
				FROM events
				WHERE site_id = ? AND timestamp >= ? AND timestamp <= ? AND name = ?
			`, params.SiteID, params.Start, params.End, goal.Value).Scan(&conversions)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to calc goal conversions: %w", err)
		}

		rate := 0.0
		if stats.UniqueSessions > 0 {
			rate = (float64(conversions) / float64(stats.UniqueSessions)) * 100
		}

		stats.Goals = append(stats.Goals, api.GoalStats{
			GoalID:         goal.ID,
			Name:           goal.Name,
			Conversions:    conversions,
			ConversionRate: rate,
		})
	}

	if !params.CompareStart.IsZero() {
		comparison, err := s.GetComparisonStats(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("failed to calc comparison stats: %w", err)
		}
		stats.Comparison = comparison
	}

	return stats, nil
}
