package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type rollupKind string

const (
	rollupHourly  rollupKind = "hourly"
	rollupDaily   rollupKind = "daily"
	rollupMonthly rollupKind = "monthly"
)

const (
	hitRollupsHourlyQuery = `
		SELECT bucket, SUM(pageviews) as pageviews, SUM(visitors) as visitors
		FROM hit_rollups_hourly
		WHERE site_id = ? AND bucket >= ? AND bucket <= ?
		GROUP BY bucket
		ORDER BY bucket
	`
	hitRollupsDailyQuery = `
		SELECT bucket, SUM(pageviews) as pageviews, SUM(visitors) as visitors
		FROM hit_rollups_daily
		WHERE site_id = ? AND bucket >= ? AND bucket <= ?
		GROUP BY bucket
		ORDER BY bucket
	`
	hitRollupsMonthlyQuery = `
		SELECT bucket, SUM(pageviews) as pageviews, SUM(visitors) as visitors
		FROM hit_rollups_monthly
		WHERE site_id = ? AND bucket >= ? AND bucket <= ?
		GROUP BY bucket
		ORDER BY bucket
	`
	goalRollupsHourlyQuery = `
		SELECT bucket, SUM(conversions) as conversions
		FROM goal_rollups_hourly
		WHERE site_id = ? AND bucket >= ? AND bucket <= ?
		GROUP BY bucket
		ORDER BY bucket
	`
	goalRollupsDailyQuery = `
		SELECT bucket, SUM(conversions) as conversions
		FROM goal_rollups_daily
		WHERE site_id = ? AND bucket >= ? AND bucket <= ?
		GROUP BY bucket
		ORDER BY bucket
	`
	goalRollupsMonthlyQuery = `
		SELECT bucket, SUM(conversions) as conversions
		FROM goal_rollups_monthly
		WHERE site_id = ? AND bucket >= ? AND bucket <= ?
		GROUP BY bucket
		ORDER BY bucket
	`
	funnelRollupsHourlyQuery = `
		SELECT bucket, SUM(entries) as entries, SUM(completions) as completions
		FROM funnel_rollups_hourly
		WHERE site_id = ? AND bucket >= ? AND bucket <= ?
		GROUP BY bucket
		ORDER BY bucket
	`
	funnelRollupsDailyQuery = `
		SELECT bucket, SUM(entries) as entries, SUM(completions) as completions
		FROM funnel_rollups_daily
		WHERE site_id = ? AND bucket >= ? AND bucket <= ?
		GROUP BY bucket
		ORDER BY bucket
	`
	funnelRollupsMonthlyQuery = `
		SELECT bucket, SUM(entries) as entries, SUM(completions) as completions
		FROM funnel_rollups_monthly
		WHERE site_id = ? AND bucket >= ? AND bucket <= ?
		GROUP BY bucket
		ORDER BY bucket
	`
	hitSeriesHourlyQuery = `
		SELECT date_trunc('hour', timestamp) AS bucket, COUNT(*) AS pageviews, COUNT(DISTINCT session_id) AS visitors
		FROM hits
		WHERE site_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY bucket
		ORDER BY bucket
	`
	hitSeriesDailyQuery = `
		SELECT date_trunc('day', timestamp) AS bucket, COUNT(*) AS pageviews, COUNT(DISTINCT session_id) AS visitors
		FROM hits
		WHERE site_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY bucket
		ORDER BY bucket
	`
	hitSeriesMonthlyQuery = `
		SELECT date_trunc('month', timestamp) AS bucket, COUNT(*) AS pageviews, COUNT(DISTINCT session_id) AS visitors
		FROM hits
		WHERE site_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY bucket
		ORDER BY bucket
	`
	funnelStepHitsPathHourlyQuery = `
		SELECT date_trunc('hour', timestamp) AS bucket, COUNT(DISTINCT session_id) AS conversions
		FROM hits
		WHERE site_id = ? AND timestamp >= ? AND timestamp < ? AND path = ?
		GROUP BY bucket
		ORDER BY bucket
	`
	funnelStepHitsPathDailyQuery = `
		SELECT date_trunc('day', timestamp) AS bucket, COUNT(DISTINCT session_id) AS conversions
		FROM hits
		WHERE site_id = ? AND timestamp >= ? AND timestamp < ? AND path = ?
		GROUP BY bucket
		ORDER BY bucket
	`
	funnelStepHitsPathMonthlyQuery = `
		SELECT date_trunc('month', timestamp) AS bucket, COUNT(DISTINCT session_id) AS conversions
		FROM hits
		WHERE site_id = ? AND timestamp >= ? AND timestamp < ? AND path = ?
		GROUP BY bucket
		ORDER BY bucket
	`
	funnelStepEventsNameHourlyQuery = `
		SELECT date_trunc('hour', timestamp) AS bucket, COUNT(DISTINCT session_id) AS conversions
		FROM events
		WHERE site_id = ? AND timestamp >= ? AND timestamp < ? AND name = ?
		GROUP BY bucket
		ORDER BY bucket
	`
	funnelStepEventsNameDailyQuery = `
		SELECT date_trunc('day', timestamp) AS bucket, COUNT(DISTINCT session_id) AS conversions
		FROM events
		WHERE site_id = ? AND timestamp >= ? AND timestamp < ? AND name = ?
		GROUP BY bucket
		ORDER BY bucket
	`
	funnelStepEventsNameMonthlyQuery = `
		SELECT date_trunc('month', timestamp) AS bucket, COUNT(DISTINCT session_id) AS conversions
		FROM events
		WHERE site_id = ? AND timestamp >= ? AND timestamp < ? AND name = ?
		GROUP BY bucket
		ORDER BY bucket
	`
)

type analyticsStatements struct {
	hitRollupsHourly            *sql.Stmt
	hitRollupsDaily             *sql.Stmt
	hitRollupsMonthly           *sql.Stmt
	goalRollupsHourly           *sql.Stmt
	goalRollupsDaily            *sql.Stmt
	goalRollupsMonthly          *sql.Stmt
	funnelRollupsHourly         *sql.Stmt
	funnelRollupsDaily          *sql.Stmt
	funnelRollupsMonthly        *sql.Stmt
	hitSeriesHourly             *sql.Stmt
	hitSeriesDaily              *sql.Stmt
	hitSeriesMonthly            *sql.Stmt
	funnelStepHitsPathHourly    *sql.Stmt
	funnelStepHitsPathDaily     *sql.Stmt
	funnelStepHitsPathMonthly   *sql.Stmt
	funnelStepEventsNameHourly  *sql.Stmt
	funnelStepEventsNameDaily   *sql.Stmt
	funnelStepEventsNameMonthly *sql.Stmt
}

func (a *analyticsStatements) close() error {
	var firstErr error
	for _, stmt := range []*sql.Stmt{
		a.hitRollupsHourly,
		a.hitRollupsDaily,
		a.hitRollupsMonthly,
		a.goalRollupsHourly,
		a.goalRollupsDaily,
		a.goalRollupsMonthly,
		a.funnelRollupsHourly,
		a.funnelRollupsDaily,
		a.funnelRollupsMonthly,
		a.hitSeriesHourly,
		a.hitSeriesDaily,
		a.hitSeriesMonthly,
		a.funnelStepHitsPathHourly,
		a.funnelStepHitsPathDaily,
		a.funnelStepHitsPathMonthly,
		a.funnelStepEventsNameHourly,
		a.funnelStepEventsNameDaily,
		a.funnelStepEventsNameMonthly,
	} {
		if stmt == nil {
			continue
		}
		if err := stmt.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (a *analyticsStatements) hitRollupStmt(kind rollupKind) *sql.Stmt {
	switch kind {
	case rollupHourly:
		return a.hitRollupsHourly
	case rollupDaily:
		return a.hitRollupsDaily
	case rollupMonthly:
		return a.hitRollupsMonthly
	}
	return a.hitRollupsHourly
}

func (a *analyticsStatements) goalRollupStmt(kind rollupKind) *sql.Stmt {
	switch kind {
	case rollupHourly:
		return a.goalRollupsHourly
	case rollupDaily:
		return a.goalRollupsDaily
	case rollupMonthly:
		return a.goalRollupsMonthly
	}
	return a.goalRollupsHourly
}

func (a *analyticsStatements) funnelRollupStmt(kind rollupKind) *sql.Stmt {
	switch kind {
	case rollupHourly:
		return a.funnelRollupsHourly
	case rollupDaily:
		return a.funnelRollupsDaily
	case rollupMonthly:
		return a.funnelRollupsMonthly
	}
	return a.funnelRollupsHourly
}

func (a *analyticsStatements) hitSeriesStmt(truncUnit string) *sql.Stmt {
	switch truncUnit {
	case "hour":
		return a.hitSeriesHourly
	case "month":
		return a.hitSeriesMonthly
	default:
		return a.hitSeriesDaily
	}
}

func (a *analyticsStatements) funnelStepStmt(stepType string, truncUnit string) *sql.Stmt {
	useHits := stepType == "path"
	switch truncUnit {
	case "hour":
		if useHits {
			return a.funnelStepHitsPathHourly
		}
		return a.funnelStepEventsNameHourly
	case "month":
		if useHits {
			return a.funnelStepHitsPathMonthly
		}
		return a.funnelStepEventsNameMonthly
	default:
		if useHits {
			return a.funnelStepHitsPathDaily
		}
		return a.funnelStepEventsNameDaily
	}
}

func prepareAnalyticsStatements(ctx context.Context, db *sql.DB) (*analyticsStatements, error) {
	stmts := &analyticsStatements{}
	var err error

	stmts.hitRollupsHourly, err = db.PrepareContext(ctx, hitRollupsHourlyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}
	stmts.hitRollupsDaily, err = db.PrepareContext(ctx, hitRollupsDailyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}
	stmts.hitRollupsMonthly, err = db.PrepareContext(ctx, hitRollupsMonthlyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}

	stmts.goalRollupsHourly, err = db.PrepareContext(ctx, goalRollupsHourlyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}
	stmts.goalRollupsDaily, err = db.PrepareContext(ctx, goalRollupsDailyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}
	stmts.goalRollupsMonthly, err = db.PrepareContext(ctx, goalRollupsMonthlyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}

	stmts.funnelRollupsHourly, err = db.PrepareContext(ctx, funnelRollupsHourlyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}
	stmts.funnelRollupsDaily, err = db.PrepareContext(ctx, funnelRollupsDailyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}
	stmts.funnelRollupsMonthly, err = db.PrepareContext(ctx, funnelRollupsMonthlyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}

	stmts.hitSeriesHourly, err = db.PrepareContext(ctx, hitSeriesHourlyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}
	stmts.hitSeriesDaily, err = db.PrepareContext(ctx, hitSeriesDailyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}
	stmts.hitSeriesMonthly, err = db.PrepareContext(ctx, hitSeriesMonthlyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}

	stmts.funnelStepHitsPathHourly, err = db.PrepareContext(ctx, funnelStepHitsPathHourlyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}
	stmts.funnelStepHitsPathDaily, err = db.PrepareContext(ctx, funnelStepHitsPathDailyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}
	stmts.funnelStepHitsPathMonthly, err = db.PrepareContext(ctx, funnelStepHitsPathMonthlyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}

	stmts.funnelStepEventsNameHourly, err = db.PrepareContext(ctx, funnelStepEventsNameHourlyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}
	stmts.funnelStepEventsNameDaily, err = db.PrepareContext(ctx, funnelStepEventsNameDailyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}
	stmts.funnelStepEventsNameMonthly, err = db.PrepareContext(ctx, funnelStepEventsNameMonthlyQuery)
	if err != nil {
		_ = stmts.close()
		return nil, err
	}

	return stmts, nil
}

func (s *Store) ensureAnalyticsStatements(ctx context.Context) (*analyticsStatements, error) {
	s.analyticsMu.Lock()
	defer s.analyticsMu.Unlock()
	if s.analyticsStatements != nil {
		return s.analyticsStatements, nil
	}

	stmts, err := prepareAnalyticsStatements(ctx, s.db)
	if err != nil {
		return nil, err
	}
	s.analyticsStatements = stmts
	return stmts, nil
}

func rollupKindFromTruncUnit(truncUnit string) rollupKind {
	switch truncUnit {
	case "hour":
		return rollupHourly
	case "month":
		return rollupMonthly
	default:
		return rollupDaily
	}
}

func (s *Store) queryRollupChart(
	ctx context.Context,
	siteID uuid.UUID,
	gridStart time.Time,
	gridEnd time.Time,
	interval string,
	truncUnit string,
	kind rollupKind,
) (*sql.Rows, error) {
	var table string
	switch kind {
	case rollupHourly:
		table = "hit_rollups_hourly"
	case rollupDaily:
		table = "hit_rollups_daily"
	case rollupMonthly:
		table = "hit_rollups_monthly"
	default:
		table = "hit_rollups_hourly"
	}

	//nolint:gosec // interval/truncUnit are derived from fixed allowlists
	chartQuery := fmt.Sprintf(`
	WITH time_range AS (
		SELECT unnest(generate_series(?::TIMESTAMP, ?::TIMESTAMP, INTERVAL %s)) as bucket
	),
	rollup_hits AS (
		SELECT 
			date_trunc('%s', bucket)::TIMESTAMP as bucket,
			SUM(pageviews) as pageviews,
			SUM(visitors) as visitors
		FROM %s
		WHERE site_id = ? AND bucket >= ? AND bucket <= ?
		GROUP BY bucket
	)
	SELECT 
		tr.bucket,
		COALESCE(rh.pageviews, 0),
		COALESCE(rh.visitors, 0)
	FROM time_range tr
	LEFT JOIN rollup_hits rh ON tr.bucket = rh.bucket
	ORDER BY tr.bucket ASC;
	`, interval, truncUnit, table)

	return s.db.QueryContext(ctx, chartQuery, gridStart, gridEnd, siteID, gridStart, gridEnd)
}
