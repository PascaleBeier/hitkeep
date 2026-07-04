package database

import (
	"context"
	"fmt"

	"hitkeep/internal/api"
)

// GetSiteOverviewStats returns the lightweight analytics slice used by the multi-site overview.
func (s *Store) GetSiteOverviewStats(ctx context.Context, params api.AnalyticsParams) (*api.SiteOverviewStats, error) {
	stats := &api.SiteOverviewStats{
		SiteID:    params.SiteID,
		Status:    api.SiteOverviewStatsReady,
		ChartData: []api.ChartDataPoint{},
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

	truncUnit := truncUnitForRange(params.Start, params.End)
	rollupKind := rollupKindFromTruncUnit(truncUnit)
	gridStart := truncToUnit(params.Start, truncUnit)
	gridEnd := truncToUnit(params.End, truncUnit)
	useRollups := len(params.Filters) == 0 && canUseRollupsForTruncUnit(truncUnit)
	if sessionSQL != "" || funnelPathSQL != "" {
		useRollups = false
	}

	if useRollups {
		if err := s.refreshDirtyRollupsInRange(ctx, params.SiteID, dirtyRollupSession, rollupKind, gridStart, gridEnd); err != nil {
			return nil, fmt.Errorf("failed to refresh session rollups: %w", err)
		}
	}

	var avgSessionDuration float64
	var pagesPerSession float64
	if err := s.queryKpis(ctx, params, filterSQL, filterArgs, useRollups, rollupKind, &stats.TotalPageviews, &stats.UniqueSessions, &stats.BounceRate, &avgSessionDuration, &pagesPerSession); err != nil {
		return nil, fmt.Errorf("failed to calc overview KPIs: %w", err)
	}

	if useRollups {
		stats.ChartData, err = s.queryHybridChartData(ctx, params, truncUnit, rollupKind)
		if err != nil {
			return nil, fmt.Errorf("failed to query overview chart data: %w", err)
		}
	} else {
		rows, err := s.queryChartData(ctx, params, gridStart, gridEnd, truncUnit, filterSQL, filterArgs, useRollups, rollupKind)
		if err != nil {
			return nil, fmt.Errorf("failed to query overview chart data: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var point api.ChartDataPoint
			if err := rows.Scan(&point.Time, &point.Pageviews, &point.Visitors); err != nil {
				return nil, err
			}
			stats.ChartData = append(stats.ChartData, point)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("failed to read overview chart data: %w", err)
		}
	}

	if err := s.augmentImportedSiteOverviewStats(ctx, params, truncUnit, stats); err != nil {
		return nil, err
	}

	return stats, nil
}
