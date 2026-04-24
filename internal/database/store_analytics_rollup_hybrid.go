package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func (s *Store) queryHybridChartData(ctx context.Context, params api.AnalyticsParams, truncUnit string, rollupKind rollupKind) ([]api.ChartDataPoint, error) {
	window := buildRollupWindow(params.Start, params.End, truncUnit)
	counts := make(map[time.Time]api.ChartDataPoint)

	if window.UseRollup {
		if err := s.refreshDirtyRollupsInRange(ctx, params.SiteID, dirtyRollupHit, rollupKind, window.FullStart, window.FullEnd); err != nil {
			return nil, err
		}
		rollupCounts, err := s.queryHitRollupCounts(ctx, rollupKind, params.SiteID, window.FullStart, window.FullEnd)
		if err != nil {
			return nil, err
		}
		for bucket, point := range rollupCounts {
			counts[bucket] = mergeChartPoint(counts[bucket], point)
		}
	}

	if window.Leading != nil {
		edgeEnd := *window.Leading
		if edgeEnd.After(params.Start) {
			edgeEnd = edgeEnd.Add(-time.Nanosecond)
		}
		edgeParams := params
		edgeParams.Start = params.Start
		edgeParams.End = edgeEnd
		rawCounts, err := s.queryHitSeriesCounts(ctx, edgeParams, truncUnit)
		if err != nil {
			return nil, err
		}
		for bucket, point := range rawCounts {
			counts[bucket] = mergeChartPoint(counts[bucket], point)
		}
	}

	if window.Trailing != nil {
		edgeStart := *window.Trailing
		edgeParams := params
		edgeParams.Start = edgeStart
		edgeParams.End = params.End
		rawCounts, err := s.queryHitSeriesCounts(ctx, edgeParams, truncUnit)
		if err != nil {
			return nil, err
		}
		for bucket, point := range rawCounts {
			counts[bucket] = mergeChartPoint(counts[bucket], point)
		}
	}

	buckets := buildSeriesBuckets(params.Start, params.End, truncUnit)
	series := make([]api.ChartDataPoint, 0, len(buckets))
	for _, bucket := range buckets {
		point := counts[bucket]
		point.Time = bucket
		series = append(series, point)
	}

	return series, nil
}

func mergeChartPoint(base api.ChartDataPoint, next api.ChartDataPoint) api.ChartDataPoint {
	return api.ChartDataPoint{
		Time:      base.Time,
		Pageviews: base.Pageviews + next.Pageviews,
		Visitors:  base.Visitors + next.Visitors,
	}
}

func (s *Store) queryHitRollupCounts(ctx context.Context, kind rollupKind, siteID uuid.UUID, start time.Time, end time.Time) (map[time.Time]api.ChartDataPoint, error) {
	stmts, err := s.ensureAnalyticsStatements(ctx)
	if err != nil {
		return nil, err
	}
	stmt := stmts.hitRollupStmt(kind) //nolint:sqlclosecheck // cached statement is closed by analyticsStatements.close.
	rows, err := stmt.QueryContext(ctx, siteID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[time.Time]api.ChartDataPoint)
	for rows.Next() {
		var bucket time.Time
		var pageviews int
		var visitors int
		if err := rows.Scan(&bucket, &pageviews, &visitors); err != nil {
			return nil, err
		}
		normalized := truncToUnit(bucket, kindToTruncUnit(kind))
		result[normalized] = api.ChartDataPoint{
			Time:      normalized,
			Pageviews: pageviews,
			Visitors:  visitors,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read hit rollup count rows: %w", err)
	}

	return result, nil
}

func (s *Store) queryHitSeriesCounts(ctx context.Context, params api.AnalyticsParams, truncUnit string) (map[time.Time]api.ChartDataPoint, error) {
	stmts, err := s.ensureAnalyticsStatements(ctx)
	if err != nil {
		return nil, err
	}
	stmt := stmts.hitSeriesStmt(truncUnit) //nolint:sqlclosecheck // cached statement is closed by analyticsStatements.close.
	rows, err := stmt.QueryContext(ctx, params.SiteID, params.Start, params.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[time.Time]api.ChartDataPoint)
	for rows.Next() {
		var bucket time.Time
		var pageviews int
		var visitors int
		if err := rows.Scan(&bucket, &pageviews, &visitors); err != nil {
			return nil, err
		}
		normalized := truncToUnit(bucket, truncUnit)
		result[normalized] = api.ChartDataPoint{
			Time:      normalized,
			Pageviews: pageviews,
			Visitors:  visitors,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read hit series count rows: %w", err)
	}

	return result, nil
}
