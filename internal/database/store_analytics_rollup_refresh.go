package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (s *Store) ensureHourlyRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	return s.ensureHourlyRollup(ctx, "hit_rollups_hourly", siteID, start, end, s.insertHourlyRollups)
}

func (s *Store) insertHourlyRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	if end.Before(start) {
		return nil
	}
	endExclusive := end.Add(time.Hour)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO hit_rollups_hourly (site_id, bucket, pageviews, visitors)
		SELECT
			site_id,
			date_trunc('hour', timestamp)::TIMESTAMPTZ as bucket,
			COUNT(*) as pageviews,
			COUNT(DISTINCT session_id) as visitors
		FROM hits
		WHERE site_id = ? AND timestamp >= ? AND timestamp < ?
		GROUP BY site_id, bucket
		ON CONFLICT (site_id, bucket) DO UPDATE SET
			pageviews = EXCLUDED.pageviews,
			visitors = EXCLUDED.visitors
	`, siteID, start, endExclusive)
	if err != nil {
		return fmt.Errorf("failed to insert rollups: %w", err)
	}
	return nil
}

func (s *Store) ensureDailyRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	return s.ensureDailyRollup(ctx, "hit_rollups_daily", siteID, start, end, s.insertDailyRollups)
}

func (s *Store) insertDailyRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	if end.Before(start) {
		return nil
	}
	endExclusive := end.AddDate(0, 0, 1)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO hit_rollups_daily (site_id, bucket, pageviews, visitors)
		SELECT
			site_id,
			date_trunc('day', timestamp)::DATE as bucket,
			COUNT(*) as pageviews,
			COUNT(DISTINCT session_id) as visitors
		FROM hits
		WHERE site_id = ? AND timestamp >= ? AND timestamp < ?
		GROUP BY site_id, bucket
		ON CONFLICT (site_id, bucket) DO UPDATE SET
			pageviews = EXCLUDED.pageviews,
			visitors = EXCLUDED.visitors
	`, siteID, start, endExclusive)
	if err != nil {
		return fmt.Errorf("failed to insert daily rollups: %w", err)
	}
	return nil
}

func (s *Store) ensureMonthlyRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	return s.ensureMonthlyRollup(ctx, "hit_rollups_monthly", siteID, start, end, s.insertMonthlyRollups)
}

func (s *Store) ensureGoalRollups(ctx context.Context, kind rollupKind, siteID uuid.UUID, start time.Time, end time.Time) error {
	switch kind {
	case rollupHourly:
		return s.ensureHourlyRollup(ctx, "goal_rollups_hourly", siteID, start, end, s.insertHourlyGoalRollups)
	case rollupDaily:
		return s.ensureDailyRollup(ctx, "goal_rollups_daily", siteID, start, end, s.insertDailyGoalRollups)
	case rollupMonthly:
		return s.ensureMonthlyRollup(ctx, "goal_rollups_monthly", siteID, start, end, s.insertMonthlyGoalRollups)
	default:
		return s.ensureDailyRollup(ctx, "goal_rollups_daily", siteID, start, end, s.insertDailyGoalRollups)
	}
}

func (s *Store) ensureFunnelRollups(ctx context.Context, kind rollupKind, siteID uuid.UUID, start time.Time, end time.Time) error {
	switch kind {
	case rollupHourly:
		return s.ensureHourlyRollup(ctx, "funnel_rollups_hourly", siteID, start, end, s.insertHourlyFunnelRollups)
	case rollupDaily:
		return s.ensureDailyRollup(ctx, "funnel_rollups_daily", siteID, start, end, s.insertDailyFunnelRollups)
	case rollupMonthly:
		return s.ensureMonthlyRollup(ctx, "funnel_rollups_monthly", siteID, start, end, s.insertMonthlyFunnelRollups)
	default:
		return s.ensureDailyRollup(ctx, "funnel_rollups_daily", siteID, start, end, s.insertDailyFunnelRollups)
	}
}

func (s *Store) ensureHourlySessionRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	return s.ensureHourlyRollup(ctx, "session_rollups_hourly", siteID, start, end, s.insertHourlySessionRollups)
}

func (s *Store) insertHourlySessionRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	if end.Before(start) {
		return nil
	}
	endExclusive := end.Add(time.Hour)

	_, err := s.db.ExecContext(ctx, `
		WITH session_metrics AS (
			SELECT
				site_id,
				session_id,
				date_trunc('hour', timestamp)::TIMESTAMPTZ as bucket,
				COUNT(*) as pvs,
				EXTRACT('epoch' FROM (MAX(timestamp) - MIN(timestamp))) as duration_seconds
			FROM hits
			WHERE site_id = ? AND timestamp >= ? AND timestamp < ?
			GROUP BY site_id, session_id, bucket
		)
		INSERT INTO session_rollups_hourly (site_id, bucket, sessions, bounced_sessions, duration_sum_seconds, pageviews)
		SELECT
			site_id,
			bucket,
			COUNT(*) as sessions,
			SUM(CASE WHEN pvs = 1 THEN 1 ELSE 0 END) as bounced_sessions,
			COALESCE(SUM(duration_seconds), 0) as duration_sum_seconds,
			COALESCE(SUM(pvs), 0) as pageviews
		FROM session_metrics
		GROUP BY site_id, bucket
		ON CONFLICT (site_id, bucket) DO UPDATE SET
			sessions = EXCLUDED.sessions,
			bounced_sessions = EXCLUDED.bounced_sessions,
			duration_sum_seconds = EXCLUDED.duration_sum_seconds,
			pageviews = EXCLUDED.pageviews
	`, siteID, start, endExclusive)
	if err != nil {
		return fmt.Errorf("failed to insert hourly session rollups: %w", err)
	}
	return nil
}

func (s *Store) ensureDailySessionRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	return s.ensureDailyRollup(ctx, "session_rollups_daily", siteID, start, end, s.insertDailySessionRollups)
}

func (s *Store) insertDailySessionRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	if end.Before(start) {
		return nil
	}
	endExclusive := end.AddDate(0, 0, 1)

	_, err := s.db.ExecContext(ctx, `
		WITH session_metrics AS (
			SELECT
				site_id,
				session_id,
				date_trunc('day', timestamp)::DATE as bucket,
				COUNT(*) as pvs,
				EXTRACT('epoch' FROM (MAX(timestamp) - MIN(timestamp))) as duration_seconds
			FROM hits
			WHERE site_id = ? AND timestamp >= ? AND timestamp < ?
			GROUP BY site_id, session_id, bucket
		)
		INSERT INTO session_rollups_daily (site_id, bucket, sessions, bounced_sessions, duration_sum_seconds, pageviews)
		SELECT
			site_id,
			bucket,
			COUNT(*) as sessions,
			SUM(CASE WHEN pvs = 1 THEN 1 ELSE 0 END) as bounced_sessions,
			COALESCE(SUM(duration_seconds), 0) as duration_sum_seconds,
			COALESCE(SUM(pvs), 0) as pageviews
		FROM session_metrics
		GROUP BY site_id, bucket
		ON CONFLICT (site_id, bucket) DO UPDATE SET
			sessions = EXCLUDED.sessions,
			bounced_sessions = EXCLUDED.bounced_sessions,
			duration_sum_seconds = EXCLUDED.duration_sum_seconds,
			pageviews = EXCLUDED.pageviews
	`, siteID, start, endExclusive)
	if err != nil {
		return fmt.Errorf("failed to insert daily session rollups: %w", err)
	}
	return nil
}

func (s *Store) ensureMonthlySessionRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	return s.ensureMonthlyRollup(ctx, "session_rollups_monthly", siteID, start, end, s.insertMonthlySessionRollups)
}

func (s *Store) insertMonthlySessionRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	if end.Before(start) {
		return nil
	}
	endExclusive := end.AddDate(0, 1, 0)

	_, err := s.db.ExecContext(ctx, `
		WITH session_metrics AS (
			SELECT
				site_id,
				session_id,
				date_trunc('month', timestamp)::DATE as bucket,
				COUNT(*) as pvs,
				EXTRACT('epoch' FROM (MAX(timestamp) - MIN(timestamp))) as duration_seconds
			FROM hits
			WHERE site_id = ? AND timestamp >= ? AND timestamp < ?
			GROUP BY site_id, session_id, bucket
		)
		INSERT INTO session_rollups_monthly (site_id, bucket, sessions, bounced_sessions, duration_sum_seconds, pageviews)
		SELECT
			site_id,
			bucket,
			COUNT(*) as sessions,
			SUM(CASE WHEN pvs = 1 THEN 1 ELSE 0 END) as bounced_sessions,
			COALESCE(SUM(duration_seconds), 0) as duration_sum_seconds,
			COALESCE(SUM(pvs), 0) as pageviews
		FROM session_metrics
		GROUP BY site_id, bucket
		ON CONFLICT (site_id, bucket) DO UPDATE SET
			sessions = EXCLUDED.sessions,
			bounced_sessions = EXCLUDED.bounced_sessions,
			duration_sum_seconds = EXCLUDED.duration_sum_seconds,
			pageviews = EXCLUDED.pageviews
	`, siteID, start, endExclusive)
	if err != nil {
		return fmt.Errorf("failed to insert monthly session rollups: %w", err)
	}
	return nil
}

func (s *Store) ensureMonthlyRollup(
	ctx context.Context,
	table string,
	siteID uuid.UUID,
	start time.Time,
	end time.Time,
	insertFn func(context.Context, uuid.UUID, time.Time, time.Time) error,
) error {
	startBucket := monthOnly(start)
	endBucket := monthOnly(end)
	if endBucket.Before(startBucket) {
		return nil
	}

	return insertFn(ctx, siteID, startBucket, endBucket)
}

func (s *Store) ensureDailyRollup(
	ctx context.Context,
	table string,
	siteID uuid.UUID,
	start time.Time,
	end time.Time,
	insertFn func(context.Context, uuid.UUID, time.Time, time.Time) error,
) error {
	startBucket := dateOnly(start)
	endBucket := dateOnly(end)
	if endBucket.Before(startBucket) {
		return nil
	}

	return insertFn(ctx, siteID, startBucket, endBucket)
}

func (s *Store) ensureHourlyRollup(
	ctx context.Context,
	table string,
	siteID uuid.UUID,
	start time.Time,
	end time.Time,
	insertFn func(context.Context, uuid.UUID, time.Time, time.Time) error,
) error {
	startBucket := start.Truncate(time.Hour)
	endBucket := end.Truncate(time.Hour)
	if endBucket.Before(startBucket) {
		return nil
	}

	return insertFn(ctx, siteID, startBucket, endBucket)
}

func (s *Store) BackfillRollups(ctx context.Context, siteID uuid.UUID) error {
	var minTS sql.NullTime
	var maxTS sql.NullTime
	if err := s.db.QueryRowContext(ctx,
		"SELECT MIN(timestamp), MAX(timestamp) FROM hits WHERE site_id = ?",
		siteID,
	).Scan(&minTS, &maxTS); err != nil {
		return err
	}
	if !minTS.Valid || !maxTS.Valid {
		return nil
	}

	start := minTS.Time
	end := maxTS.Time

	if err := s.ensureHourlyRollups(ctx, siteID, start, end); err != nil {
		return err
	}
	if err := s.ensureDailyRollups(ctx, siteID, start, end); err != nil {
		return err
	}
	if err := s.ensureMonthlyRollups(ctx, siteID, start, end); err != nil {
		return err
	}
	if err := s.ensureHourlySessionRollups(ctx, siteID, start, end); err != nil {
		return err
	}
	if err := s.ensureDailySessionRollups(ctx, siteID, start, end); err != nil {
		return err
	}
	if err := s.ensureMonthlySessionRollups(ctx, siteID, start, end); err != nil {
		return err
	}
	if err := s.ensureGoalRollups(ctx, rollupHourly, siteID, start, end); err != nil {
		return err
	}
	if err := s.ensureGoalRollups(ctx, rollupDaily, siteID, start, end); err != nil {
		return err
	}
	if err := s.ensureGoalRollups(ctx, rollupMonthly, siteID, start, end); err != nil {
		return err
	}
	if err := s.ensureFunnelRollups(ctx, rollupHourly, siteID, start, end); err != nil {
		return err
	}
	if err := s.ensureFunnelRollups(ctx, rollupDaily, siteID, start, end); err != nil {
		return err
	}
	if err := s.ensureFunnelRollups(ctx, rollupMonthly, siteID, start, end); err != nil {
		return err
	}

	return nil
}

func (s *Store) insertMonthlyRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	if end.Before(start) {
		return nil
	}
	endExclusive := end.AddDate(0, 1, 0)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO hit_rollups_monthly (site_id, bucket, pageviews, visitors)
		SELECT
			site_id,
			date_trunc('month', timestamp)::DATE as bucket,
			COUNT(*) as pageviews,
			COUNT(DISTINCT session_id) as visitors
		FROM hits
		WHERE site_id = ? AND timestamp >= ? AND timestamp < ?
		GROUP BY site_id, bucket
		ON CONFLICT (site_id, bucket) DO UPDATE SET
			pageviews = EXCLUDED.pageviews,
			visitors = EXCLUDED.visitors
	`, siteID, start, endExclusive)
	if err != nil {
		return fmt.Errorf("failed to insert monthly rollups: %w", err)
	}
	return nil
}

func (s *Store) insertHourlyGoalRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	if end.Before(start) {
		return nil
	}
	endExclusive := end.Add(time.Hour)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO goal_rollups_hourly (site_id, goal_id, bucket, conversions)
		SELECT
			g.site_id,
			g.id,
			date_trunc('hour', h.timestamp)::TIMESTAMPTZ as bucket,
			COUNT(DISTINCT h.session_id) as conversions
		FROM goals g
		JOIN hits h ON h.site_id = g.site_id AND g.type = 'path' AND h.path = g.value
		WHERE g.site_id = ? AND h.timestamp >= ? AND h.timestamp < ?
		GROUP BY g.site_id, g.id, bucket
		UNION ALL
		SELECT
			g.site_id,
			g.id,
			date_trunc('hour', e.timestamp)::TIMESTAMPTZ as bucket,
			COUNT(DISTINCT e.session_id) as conversions
		FROM goals g
		JOIN events e ON e.site_id = g.site_id AND g.type = 'event' AND e.name = g.value
		WHERE g.site_id = ? AND e.timestamp >= ? AND e.timestamp < ?
		GROUP BY g.site_id, g.id, bucket
		ON CONFLICT (site_id, goal_id, bucket) DO UPDATE SET
			conversions = EXCLUDED.conversions
	`, siteID, start, endExclusive, siteID, start, endExclusive)
	if err != nil {
		return fmt.Errorf("failed to insert hourly goal rollups: %w", err)
	}
	return nil
}

func (s *Store) insertDailyGoalRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	if end.Before(start) {
		return nil
	}
	endExclusive := end.AddDate(0, 0, 1)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO goal_rollups_daily (site_id, goal_id, bucket, conversions)
		SELECT
			g.site_id,
			g.id,
			date_trunc('day', h.timestamp)::DATE as bucket,
			COUNT(DISTINCT h.session_id) as conversions
		FROM goals g
		JOIN hits h ON h.site_id = g.site_id AND g.type = 'path' AND h.path = g.value
		WHERE g.site_id = ? AND h.timestamp >= ? AND h.timestamp < ?
		GROUP BY g.site_id, g.id, bucket
		UNION ALL
		SELECT
			g.site_id,
			g.id,
			date_trunc('day', e.timestamp)::DATE as bucket,
			COUNT(DISTINCT e.session_id) as conversions
		FROM goals g
		JOIN events e ON e.site_id = g.site_id AND g.type = 'event' AND e.name = g.value
		WHERE g.site_id = ? AND e.timestamp >= ? AND e.timestamp < ?
		GROUP BY g.site_id, g.id, bucket
		ON CONFLICT (site_id, goal_id, bucket) DO UPDATE SET
			conversions = EXCLUDED.conversions
	`, siteID, start, endExclusive, siteID, start, endExclusive)
	if err != nil {
		return fmt.Errorf("failed to insert daily goal rollups: %w", err)
	}
	return nil
}

func (s *Store) insertMonthlyGoalRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	if end.Before(start) {
		return nil
	}
	endExclusive := end.AddDate(0, 1, 0)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO goal_rollups_monthly (site_id, goal_id, bucket, conversions)
		SELECT
			g.site_id,
			g.id,
			date_trunc('month', h.timestamp)::DATE as bucket,
			COUNT(DISTINCT h.session_id) as conversions
		FROM goals g
		JOIN hits h ON h.site_id = g.site_id AND g.type = 'path' AND h.path = g.value
		WHERE g.site_id = ? AND h.timestamp >= ? AND h.timestamp < ?
		GROUP BY g.site_id, g.id, bucket
		UNION ALL
		SELECT
			g.site_id,
			g.id,
			date_trunc('month', e.timestamp)::DATE as bucket,
			COUNT(DISTINCT e.session_id) as conversions
		FROM goals g
		JOIN events e ON e.site_id = g.site_id AND g.type = 'event' AND e.name = g.value
		WHERE g.site_id = ? AND e.timestamp >= ? AND e.timestamp < ?
		GROUP BY g.site_id, g.id, bucket
		ON CONFLICT (site_id, goal_id, bucket) DO UPDATE SET
			conversions = EXCLUDED.conversions
	`, siteID, start, endExclusive, siteID, start, endExclusive)
	if err != nil {
		return fmt.Errorf("failed to insert monthly goal rollups: %w", err)
	}
	return nil
}

func (s *Store) insertHourlyFunnelRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	return s.insertFunnelRollups(ctx, "funnel_rollups_hourly", "hour", siteID, start, end.Add(time.Hour))
}

func (s *Store) insertDailyFunnelRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	return s.insertFunnelRollups(ctx, "funnel_rollups_daily", "day", siteID, start, end.AddDate(0, 0, 1))
}

func (s *Store) insertMonthlyFunnelRollups(ctx context.Context, siteID uuid.UUID, start time.Time, end time.Time) error {
	return s.insertFunnelRollups(ctx, "funnel_rollups_monthly", "month", siteID, start, end.AddDate(0, 1, 0))
}

func (s *Store) insertFunnelRollups(ctx context.Context, table string, truncUnit string, siteID uuid.UUID, start time.Time, endExclusive time.Time) error {
	if endExclusive.Before(start) {
		return nil
	}

	funnels, err := s.GetFunnels(ctx, siteID)
	if err != nil {
		return fmt.Errorf("failed to fetch funnels: %w", err)
	}
	if len(funnels) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	//nolint:gosec // table is selected from fixed allowlist
	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (site_id, funnel_id, bucket, entries, completions)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (site_id, funnel_id, bucket) DO UPDATE SET
			entries = EXCLUDED.entries,
			completions = EXCLUDED.completions
	`, table))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, funnel := range funnels {
		if len(funnel.Steps) == 0 {
			continue
		}
		first := funnel.Steps[0]
		last := funnel.Steps[len(funnel.Steps)-1]

		entryCounts, err := s.queryFunnelStepCounts(ctx, siteID, start, endExclusive, truncUnit, first)
		if err != nil {
			return err
		}
		completionCounts, err := s.queryFunnelStepCounts(ctx, siteID, start, endExclusive, truncUnit, last)
		if err != nil {
			return err
		}

		buckets := make(map[time.Time]bool)
		for bucket := range entryCounts {
			buckets[bucket] = true
		}
		for bucket := range completionCounts {
			buckets[bucket] = true
		}

		for bucket := range buckets {
			entries := entryCounts[bucket]
			completions := completionCounts[bucket]
			if entries == 0 && completions == 0 {
				continue
			}
			if _, err := stmt.ExecContext(ctx, siteID, funnel.ID, bucket, entries, completions); err != nil {
				return fmt.Errorf("failed to insert funnel rollup: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}
