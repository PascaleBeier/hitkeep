package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	json "hitkeep/internal/jsonapi"
)

func (s *Store) GetGoalTimeseries(ctx context.Context, params api.AnalyticsParams, goalIDs []uuid.UUID) ([]api.GoalSeriesPoint, error) {
	goals, err := s.GetGoals(ctx, params.SiteID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch goals: %w", err)
	}
	if len(goals) == 0 {
		return []api.GoalSeriesPoint{}, nil
	}

	if len(goalIDs) > 0 {
		allowed := make(map[uuid.UUID]bool, len(goalIDs))
		for _, id := range goalIDs {
			allowed[id] = true
		}
		filtered := goals[:0]
		for _, goal := range goals {
			if allowed[goal.ID] {
				filtered = append(filtered, goal)
			}
		}
		goals = filtered
		if len(goals) == 0 {
			return []api.GoalSeriesPoint{}, nil
		}
	}

	var pathValues []string
	var eventValues []string
	for _, goal := range goals {
		if goal.Type == "path" {
			pathValues = append(pathValues, goal.Value)
		}
		if goal.Type == "event" {
			eventValues = append(eventValues, goal.Value)
		}
	}

	truncUnit := truncUnitForRange(params.Start, params.End)
	rollupKind := rollupKindFromTruncUnit(truncUnit)
	window := buildRollupWindow(params.Start, params.End, truncUnit)

	counts := make(map[time.Time]int)
	if window.UseRollup {
		if err := s.ensureGoalRollups(ctx, rollupKind, params.SiteID, window.FullStart, window.FullEnd); err != nil {
			return nil, err
		}
		rollupCounts, err := s.queryGoalRollupCounts(ctx, rollupKind, params.SiteID, window.FullStart, window.FullEnd, goalIDs)
		if err != nil {
			return nil, err
		}
		for bucket, count := range rollupCounts {
			counts[bucket] += count
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
		if len(pathValues) > 0 {
			pathCounts, err := s.querySeriesCounts(ctx, "hits", "path", pathValues, edgeParams, truncUnit)
			if err != nil {
				return nil, err
			}
			for bucket, count := range pathCounts {
				counts[bucket] += count
			}
		}
		if len(eventValues) > 0 {
			eventCounts, err := s.querySeriesCounts(ctx, "events", "name", eventValues, edgeParams, truncUnit)
			if err != nil {
				return nil, err
			}
			for bucket, count := range eventCounts {
				counts[bucket] += count
			}
		}
	}

	if window.Trailing != nil {
		edgeStart := *window.Trailing
		edgeParams := params
		edgeParams.Start = edgeStart
		edgeParams.End = params.End
		if len(pathValues) > 0 {
			pathCounts, err := s.querySeriesCounts(ctx, "hits", "path", pathValues, edgeParams, truncUnit)
			if err != nil {
				return nil, err
			}
			for bucket, count := range pathCounts {
				counts[bucket] += count
			}
		}
		if len(eventValues) > 0 {
			eventCounts, err := s.querySeriesCounts(ctx, "events", "name", eventValues, edgeParams, truncUnit)
			if err != nil {
				return nil, err
			}
			for bucket, count := range eventCounts {
				counts[bucket] += count
			}
		}
	}

	if canIncludeImportedSiteAggregates(params, truncUnit) && len(eventValues) > 0 {
		importedCounts, err := s.queryImportedEventGoalSeries(ctx, params, truncUnit, eventValues)
		if err != nil {
			return nil, err
		}
		for bucket, count := range importedCounts {
			counts[bucket] += count
		}
	}

	buckets := buildSeriesBuckets(params.Start, params.End, truncUnit)
	series := make([]api.GoalSeriesPoint, 0, len(buckets))
	for _, bucket := range buckets {
		series = append(series, api.GoalSeriesPoint{
			Time:        bucket,
			Conversions: counts[bucket],
		})
	}
	return series, nil
}

func (s *Store) GetFunnelTimeseries(ctx context.Context, params api.AnalyticsParams, funnelIDs []uuid.UUID) ([]api.FunnelSeriesPoint, error) {
	funnels, err := s.GetFunnels(ctx, params.SiteID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch funnels: %w", err)
	}
	if len(funnels) == 0 {
		return []api.FunnelSeriesPoint{}, nil
	}

	if len(funnelIDs) > 0 {
		allowed := make(map[uuid.UUID]bool, len(funnelIDs))
		for _, id := range funnelIDs {
			allowed[id] = true
		}
		filtered := funnels[:0]
		for _, funnel := range funnels {
			if allowed[funnel.ID] {
				filtered = append(filtered, funnel)
			}
		}
		funnels = filtered
		if len(funnels) == 0 {
			return []api.FunnelSeriesPoint{}, nil
		}
	}

	entryPaths := make(map[string]bool)
	entryEvents := make(map[string]bool)
	completionPaths := make(map[string]bool)
	completionEvents := make(map[string]bool)

	for _, funnel := range funnels {
		if len(funnel.Steps) == 0 {
			continue
		}
		first := funnel.Steps[0]
		last := funnel.Steps[len(funnel.Steps)-1]

		if first.Type == "path" {
			entryPaths[first.Value] = true
		} else {
			entryEvents[first.Value] = true
		}

		if last.Type == "path" {
			completionPaths[last.Value] = true
		} else {
			completionEvents[last.Value] = true
		}
	}

	if len(entryPaths) == 0 && len(entryEvents) == 0 && len(completionPaths) == 0 && len(completionEvents) == 0 {
		return []api.FunnelSeriesPoint{}, nil
	}

	truncUnit := truncUnitForRange(params.Start, params.End)
	rollupKind := rollupKindFromTruncUnit(truncUnit)
	window := buildRollupWindow(params.Start, params.End, truncUnit)
	entryCounts := make(map[time.Time]int)
	completionCounts := make(map[time.Time]int)

	entryPathValues := make([]string, 0, len(entryPaths))
	for v := range entryPaths {
		entryPathValues = append(entryPathValues, v)
	}
	entryEventValues := make([]string, 0, len(entryEvents))
	for v := range entryEvents {
		entryEventValues = append(entryEventValues, v)
	}
	completionPathValues := make([]string, 0, len(completionPaths))
	for v := range completionPaths {
		completionPathValues = append(completionPathValues, v)
	}
	completionEventValues := make([]string, 0, len(completionEvents))
	for v := range completionEvents {
		completionEventValues = append(completionEventValues, v)
	}

	if window.UseRollup {
		if err := s.ensureFunnelRollups(ctx, rollupKind, params.SiteID, window.FullStart, window.FullEnd); err != nil {
			return nil, err
		}
		rollupEntries, rollupCompletions, err := s.queryFunnelRollupCounts(ctx, rollupKind, params.SiteID, window.FullStart, window.FullEnd, funnelIDs)
		if err != nil {
			return nil, err
		}
		for bucket, count := range rollupEntries {
			entryCounts[bucket] += count
		}
		for bucket, count := range rollupCompletions {
			completionCounts[bucket] += count
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
		if len(entryPathValues) > 0 {
			pathCounts, err := s.querySeriesCounts(ctx, "hits", "path", entryPathValues, edgeParams, truncUnit)
			if err != nil {
				return nil, err
			}
			for bucket, count := range pathCounts {
				entryCounts[bucket] += count
			}
		}
		if len(entryEventValues) > 0 {
			eventCounts, err := s.querySeriesCounts(ctx, "events", "name", entryEventValues, edgeParams, truncUnit)
			if err != nil {
				return nil, err
			}
			for bucket, count := range eventCounts {
				entryCounts[bucket] += count
			}
		}
		if len(completionPathValues) > 0 {
			pathCounts, err := s.querySeriesCounts(ctx, "hits", "path", completionPathValues, edgeParams, truncUnit)
			if err != nil {
				return nil, err
			}
			for bucket, count := range pathCounts {
				completionCounts[bucket] += count
			}
		}
		if len(completionEventValues) > 0 {
			eventCounts, err := s.querySeriesCounts(ctx, "events", "name", completionEventValues, edgeParams, truncUnit)
			if err != nil {
				return nil, err
			}
			for bucket, count := range eventCounts {
				completionCounts[bucket] += count
			}
		}
	}

	if window.Trailing != nil {
		edgeStart := *window.Trailing
		edgeParams := params
		edgeParams.Start = edgeStart
		edgeParams.End = params.End
		if len(entryPathValues) > 0 {
			pathCounts, err := s.querySeriesCounts(ctx, "hits", "path", entryPathValues, edgeParams, truncUnit)
			if err != nil {
				return nil, err
			}
			for bucket, count := range pathCounts {
				entryCounts[bucket] += count
			}
		}
		if len(entryEventValues) > 0 {
			eventCounts, err := s.querySeriesCounts(ctx, "events", "name", entryEventValues, edgeParams, truncUnit)
			if err != nil {
				return nil, err
			}
			for bucket, count := range eventCounts {
				entryCounts[bucket] += count
			}
		}
		if len(completionPathValues) > 0 {
			pathCounts, err := s.querySeriesCounts(ctx, "hits", "path", completionPathValues, edgeParams, truncUnit)
			if err != nil {
				return nil, err
			}
			for bucket, count := range pathCounts {
				completionCounts[bucket] += count
			}
		}
		if len(completionEventValues) > 0 {
			eventCounts, err := s.querySeriesCounts(ctx, "events", "name", completionEventValues, edgeParams, truncUnit)
			if err != nil {
				return nil, err
			}
			for bucket, count := range eventCounts {
				completionCounts[bucket] += count
			}
		}
	}

	buckets := buildSeriesBuckets(params.Start, params.End, truncUnit)
	series := make([]api.FunnelSeriesPoint, 0, len(buckets))
	for _, bucket := range buckets {
		series = append(series, api.FunnelSeriesPoint{
			Time:        bucket,
			Entries:     entryCounts[bucket],
			Completions: completionCounts[bucket],
		})
	}
	return series, nil
}

func (s *Store) GetFunnelStats(ctx context.Context, funnelID uuid.UUID, params api.AnalyticsParams) (*api.FunnelStats, error) {
	var funnel api.Funnel
	var stepsJSON []byte
	err := s.db.QueryRowContext(ctx, "SELECT id, name, CAST(steps AS VARCHAR) FROM funnels WHERE id = ? AND site_id = ?", funnelID, params.SiteID).Scan(&funnel.ID, &funnel.Name, &stepsJSON)
	if err != nil {
		return nil, fmt.Errorf("funnel not found: %w", err)
	}
	if err := json.Unmarshal(stepsJSON, &funnel.Steps); err != nil {
		return nil, err
	}

	stats := &api.FunnelStats{
		FunnelID: funnel.ID,
		Name:     funnel.Name,
		Steps:    make([]api.FunnelStepStats, len(funnel.Steps)),
	}

	var previousStepSessions []uuid.UUID
	var firstStepCount int

	for i, step := range funnel.Steps {
		var currentSessions []uuid.UUID

		stepSessionMap, err := s.queryFunnelStepSessions(ctx, params, step)
		if err != nil {
			return nil, err
		}

		if i == 0 {
			for sid := range stepSessionMap {
				currentSessions = append(currentSessions, sid)
			}
			firstStepCount = len(currentSessions)
		} else {
			for _, prevSid := range previousStepSessions {
				if stepSessionMap[prevSid] {
					currentSessions = append(currentSessions, prevSid)
				}
			}
		}

		count := len(currentSessions)
		dropoff := 0
		conversionRate := 0.0

		if i > 0 {
			prevCount := stats.Steps[i-1].Visitors
			dropoff = prevCount - count
			if prevCount > 0 {
				conversionRate = (float64(count) / float64(prevCount)) * 100
			}
		} else {
			conversionRate = 100.0
		}

		stats.Steps[i] = api.FunnelStepStats{
			StepIndex:      i,
			Name:           fmt.Sprintf("%s: %s", step.Type, step.Value),
			Visitors:       count,
			Dropoff:        dropoff,
			ConversionRate: conversionRate,
		}

		previousStepSessions = currentSessions
	}

	stats.TotalEntries = firstStepCount
	if len(stats.Steps) > 0 {
		stats.TotalCompletions = stats.Steps[len(stats.Steps)-1].Visitors
	}
	if stats.TotalEntries > 0 {
		stats.OverallConversionRate = (float64(stats.TotalCompletions) / float64(stats.TotalEntries)) * 100
	}

	return stats, nil
}

// GetComparisonStats runs the same KPI and chart queries as GetSiteStats but
// over the CompareStart/CompareEnd window. Rollups are intentionally skipped
// so any arbitrary window is supported.
func (s *Store) GetComparisonStats(ctx context.Context, params api.AnalyticsParams) (*api.ComparisonStats, error) {
	cmp := api.AnalyticsParams{
		SiteID:    params.SiteID,
		UserID:    params.UserID,
		Start:     params.CompareStart,
		End:       params.CompareEnd,
		Filters:   params.Filters,
		GoalIDs:   params.GoalIDs,
		FunnelIDs: params.FunnelIDs,
	}

	stats := &api.ComparisonStats{
		ChartData: []api.ChartDataPoint{},
	}

	truncUnit := truncUnitForRange(cmp.Start, cmp.End)
	gridStart := truncToUnit(cmp.Start, truncUnit)
	gridEnd := truncToUnit(cmp.End, truncUnit)
	filterSQL, filterArgs := buildHitFilters(cmp.Filters, "h")
	sessionSQL, sessionArgs, err := s.buildSessionFilter(ctx, cmp, "h")
	if err != nil {
		return nil, err
	}
	filterSQL += sessionSQL
	filterArgs = append(filterArgs, sessionArgs...)

	if err := s.queryKpis(ctx, cmp, filterSQL, filterArgs, false, rollupHourly,
		&stats.TotalPageviews, &stats.UniqueSessions, &stats.BounceRate, &stats.AvgSessionDuration, &stats.PagesPerSession,
	); err != nil {
		return nil, fmt.Errorf("comparison KPI query failed: %w", err)
	}

	if err := s.queryUTMKpis(ctx, cmp, filterSQL, filterArgs,
		&stats.UTMCampaignHits, &stats.UTMContentHits, &stats.UTMMediumHits, &stats.UTMSourceHits, &stats.UTMTermHits,
	); err != nil {
		return nil, fmt.Errorf("comparison UTM KPI query failed: %w", err)
	}

	if err := s.queryAIKpis(ctx, cmp, filterSQL, filterArgs, &stats.AIBotHits, &stats.AISourceVisits); err != nil {
		return nil, fmt.Errorf("comparison AI KPI query failed: %w", err)
	}

	rows, err := s.queryChartData(ctx, cmp, gridStart, gridEnd, truncUnit, filterSQL, filterArgs, false, rollupHourly)
	if err != nil {
		return nil, fmt.Errorf("comparison chart query failed: %w", err)
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
		return nil, fmt.Errorf("failed to read comparison chart rows: %w", err)
	}

	goals, err := s.GetGoals(ctx, params.SiteID)
	if err != nil {
		return nil, fmt.Errorf("comparison goals fetch failed: %w", err)
	}

	for _, goal := range goals {
		conversions, err := s.queryGoalConversions(ctx, cmp, goal, filterSQL, filterArgs)
		if err != nil {
			return nil, fmt.Errorf("comparison goal conversions query failed: %w", err)
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
		stats.TotalConversions += conversions
	}

	return stats, nil
}

func (s *Store) queryGoalConversions(
	ctx context.Context,
	params api.AnalyticsParams,
	goal api.Goal,
	filterSQL string,
	filterArgs []any,
) (int, error) {
	var goalTable string
	var goalColumn string
	switch goal.Type {
	case "path":
		goalTable = "hits"
		goalColumn = "path"
	case "event":
		goalTable = "events"
		goalColumn = "name"
	default:
		return 0, nil
	}

	// Goal conversions belong to the report's session cohort. Applying a page
	// filter directly to the goal row would incorrectly require the goal page to
	// be the filtered page, so the filtered hits are selected as a cohort first.
	//nolint:gosec // table/column are fixed above and filterSQL comes from allowlisted filters
	query := fmt.Sprintf(`
		SELECT COUNT(DISTINCT g.session_id)
		FROM %s g
		WHERE g.site_id = ?
			AND g.timestamp >= ?
			AND g.timestamp <= ?
			AND g.%s = ?
	`, goalTable, goalColumn)
	args := []any{params.SiteID, params.Start, params.End, goal.Value}
	if filterSQL != "" {
		//nolint:gosec // filterSQL comes from allowlisted analytics filters
		query += fmt.Sprintf(`
			AND g.session_id IN (
				SELECT DISTINCT h.session_id
				FROM hits h
				WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s
			)
		`, filterSQL)
		args = append(args, params.SiteID, params.Start, params.End)
		args = append(args, filterArgs...)
	}

	var conversions int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&conversions); err != nil {
		return 0, err
	}
	return conversions, nil
}

func (s *Store) queryFunnelStepSessions(ctx context.Context, params api.AnalyticsParams, step api.FunnelStep) (map[uuid.UUID]bool, error) {
	var query string
	var args []any

	if step.Type == "path" {
		query = "SELECT DISTINCT session_id FROM hits WHERE site_id = ? AND timestamp >= ? AND timestamp <= ? AND path = ?"
		args = []any{params.SiteID, params.Start, params.End, step.Value}
	} else {
		query = "SELECT DISTINCT session_id FROM events WHERE site_id = ? AND timestamp >= ? AND timestamp <= ? AND name = ?"
		args = []any{params.SiteID, params.Start, params.End, step.Value}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query funnel step sessions: %w", err)
	}
	defer rows.Close()

	stepSessionMap := make(map[uuid.UUID]bool)
	for rows.Next() {
		var sid uuid.UUID
		if err := rows.Scan(&sid); err != nil {
			return nil, fmt.Errorf("failed to scan funnel session: %w", err)
		}
		stepSessionMap[sid] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read funnel session rows: %w", err)
	}

	return stepSessionMap, nil
}
