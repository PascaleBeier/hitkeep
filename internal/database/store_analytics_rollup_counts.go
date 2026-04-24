package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func (s *Store) queryFunnelStepCounts(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time, truncUnit string, step api.FunnelStep) (map[time.Time]int, error) {
	if end.Before(start) {
		return map[time.Time]int{}, nil
	}

	stmts, err := s.ensureAnalyticsStatements(ctx)
	if err != nil {
		return nil, err
	}

	stmt := stmts.funnelStepStmt(step.Type, truncUnit) //nolint:sqlclosecheck // cached statement is closed by analyticsStatements.close.
	rows, err := stmt.QueryContext(ctx, siteID, start, end, step.Value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[time.Time]int)
	for rows.Next() {
		var bucket time.Time
		var count int
		if err := rows.Scan(&bucket, &count); err != nil {
			return nil, err
		}
		result[truncToUnit(bucket, truncUnit)] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read funnel step count rows: %w", err)
	}

	return result, nil
}

func (s *Store) queryGoalRollupCounts(ctx context.Context, kind rollupKind, siteID uuid.UUID, start time.Time, end time.Time, goalIDs []uuid.UUID) (map[time.Time]int, error) {
	var (
		rows  *sql.Rows
		err   error
		stmts *analyticsStatements
	)

	if len(goalIDs) == 0 {
		stmts, err = s.ensureAnalyticsStatements(ctx)
		if err != nil {
			return nil, err
		}
		stmt := stmts.goalRollupStmt(kind) //nolint:sqlclosecheck // cached statement is closed by analyticsStatements.close.
		rows, err = stmt.QueryContext(ctx, siteID, start, end)
	} else {
		var table string
		switch kind {
		case rollupHourly:
			table = "goal_rollups_hourly"
		case rollupDaily:
			table = "goal_rollups_daily"
		case rollupMonthly:
			table = "goal_rollups_monthly"
		default:
			table = "goal_rollups_hourly"
		}

		args := []any{siteID, start, end}
		filterSQL := fmt.Sprintf(" AND goal_id IN (%s)", buildPlaceholders(len(goalIDs)))
		for _, id := range goalIDs {
			args = append(args, id)
		}

		//nolint:gosec // table is selected from fixed allowlist
		query := fmt.Sprintf(`
			SELECT bucket, SUM(conversions) as conversions
			FROM %s
			WHERE site_id = ? AND bucket >= ? AND bucket <= ?%s
			GROUP BY bucket
			ORDER BY bucket
		`, table, filterSQL)

		rows, err = s.db.QueryContext(ctx, query, args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[time.Time]int)
	for rows.Next() {
		var bucket time.Time
		var count int
		if err := rows.Scan(&bucket, &count); err != nil {
			return nil, err
		}
		result[truncToUnit(bucket, kindToTruncUnit(kind))] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read goal rollup count rows: %w", err)
	}

	return result, nil
}

func (s *Store) queryFunnelRollupCounts(ctx context.Context, kind rollupKind, siteID uuid.UUID, start time.Time, end time.Time, funnelIDs []uuid.UUID) (map[time.Time]int, map[time.Time]int, error) {
	var (
		rows  *sql.Rows
		err   error
		stmts *analyticsStatements
	)

	if len(funnelIDs) == 0 {
		stmts, err = s.ensureAnalyticsStatements(ctx)
		if err != nil {
			return nil, nil, err
		}
		stmt := stmts.funnelRollupStmt(kind) //nolint:sqlclosecheck // cached statement is closed by analyticsStatements.close.
		rows, err = stmt.QueryContext(ctx, siteID, start, end)
	} else {
		var table string
		switch kind {
		case rollupHourly:
			table = "funnel_rollups_hourly"
		case rollupDaily:
			table = "funnel_rollups_daily"
		case rollupMonthly:
			table = "funnel_rollups_monthly"
		default:
			table = "funnel_rollups_hourly"
		}

		args := []any{siteID, start, end}
		filterSQL := fmt.Sprintf(" AND funnel_id IN (%s)", buildPlaceholders(len(funnelIDs)))
		for _, id := range funnelIDs {
			args = append(args, id)
		}

		//nolint:gosec // table is selected from fixed allowlist
		query := fmt.Sprintf(`
			SELECT bucket, SUM(entries) as entries, SUM(completions) as completions
			FROM %s
			WHERE site_id = ? AND bucket >= ? AND bucket <= ?%s
			GROUP BY bucket
			ORDER BY bucket
		`, table, filterSQL)

		rows, err = s.db.QueryContext(ctx, query, args...)
	}
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	entryCounts := make(map[time.Time]int)
	completionCounts := make(map[time.Time]int)
	for rows.Next() {
		var bucket time.Time
		var entries int
		var completions int
		if err := rows.Scan(&bucket, &entries, &completions); err != nil {
			return nil, nil, err
		}
		normalized := truncToUnit(bucket, kindToTruncUnit(kind))
		entryCounts[normalized] = entries
		completionCounts[normalized] = completions
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to read funnel rollup count rows: %w", err)
	}

	return entryCounts, completionCounts, nil
}
