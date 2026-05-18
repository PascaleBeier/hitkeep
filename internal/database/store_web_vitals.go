package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"

	"hitkeep/internal/api"
)

var webVitalAppenderColumns = []string{
	"id",
	"site_id",
	"session_id",
	"page_id",
	"metric",
	"value",
	"rating",
	"path",
	"navigation_type",
	"timestamp",
	"tracker_source",
	"tracker_version",
}

func WebVitalRatingForValue(metric api.WebVitalMetric, value float64) (api.WebVitalRating, error) {
	if value < 0 {
		return "", fmt.Errorf("web vital value must be non-negative")
	}

	switch metric {
	case api.WebVitalLCP:
		return thresholdRating(value, 2500, 4000), nil
	case api.WebVitalINP:
		return thresholdRating(value, 200, 500), nil
	case api.WebVitalCLS:
		return thresholdRating(value, 0.1, 0.25), nil
	case api.WebVitalFCP:
		return thresholdRating(value, 1800, 3000), nil
	case api.WebVitalTTFB:
		return thresholdRating(value, 800, 1800), nil
	default:
		return "", fmt.Errorf("unsupported web vital metric %q", metric)
	}
}

func thresholdRating(value, goodMax, needsImprovementMax float64) api.WebVitalRating {
	if value <= goodMax {
		return api.WebVitalRatingGood
	}
	if value <= needsImprovementMax {
		return api.WebVitalRatingNeedsImprovement
	}
	return api.WebVitalRatingPoor
}

func (s *Store) CreateWebVitalsBulk(ctx context.Context, vitals []*api.WebVital) error {
	if len(vitals) == 0 {
		return nil
	}

	return s.withAppenderColumns(ctx, "web_vitals", webVitalAppenderColumns, func(appender rowAppender) error {
		for _, vital := range vitals {
			if vital == nil {
				continue
			}
			if vital.ID == uuid.Nil {
				vital.ID = uuid.New()
			}
			if vital.Timestamp.IsZero() {
				vital.Timestamp = time.Now()
			}
			if vital.Path == "" {
				vital.Path = "/"
			}
			rating, err := WebVitalRatingForValue(vital.Metric, vital.Value)
			if err != nil {
				return err
			}
			vital.Rating = rating

			if err := appender.AppendRow(
				duckdb.UUID(vital.ID),
				duckdb.UUID(vital.SiteID),
				duckdb.UUID(vital.SessionID),
				duckdb.UUID(vital.PageID),
				string(vital.Metric),
				vital.Value,
				string(vital.Rating),
				vital.Path,
				nullableStringPtr(vital.NavigationType),
				vital.Timestamp,
				nullableNonEmptyString(vital.TrackerSource),
				nullableNonEmptyString(vital.TrackerVersion),
			); err != nil {
				return fmt.Errorf("append web vital row: %w", err)
			}
		}
		return nil
	})
}

func (s *Store) CreateWebVital(ctx context.Context, vital *api.WebVital) error {
	if vital == nil {
		return fmt.Errorf("web vital is required")
	}
	return s.CreateWebVitalsBulk(ctx, []*api.WebVital{vital})
}

func (s *Store) GetWebVitalsSummary(ctx context.Context, params api.WebVitalsParams) ([]api.WebVitalSummaryMetric, error) {
	where, args, err := buildWebVitalsWhere(params, false)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT
			metric,
			COALESCE(quantile_cont(value, 0.75), 0) AS p75,
			COUNT(*) AS samples,
			COUNT(*) FILTER (WHERE rating = 'good') AS good,
			COUNT(*) FILTER (WHERE rating = 'needs_improvement') AS needs_improvement,
			COUNT(*) FILTER (WHERE rating = 'poor') AS poor
		FROM web_vitals
		%s
		GROUP BY metric
		ORDER BY metric
	`, where), args...)
	if err != nil {
		return nil, fmt.Errorf("query web vitals summary: %w", err)
	}
	defer rows.Close()

	results := []api.WebVitalSummaryMetric{}
	for rows.Next() {
		var item api.WebVitalSummaryMetric
		if err := rows.Scan(&item.Metric, &item.P75, &item.Samples, &item.Good, &item.NeedsImprove, &item.Poor); err != nil {
			return nil, fmt.Errorf("scan web vitals summary: %w", err)
		}
		item.Rating, _ = WebVitalRatingForValue(item.Metric, item.P75)
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read web vitals summary: %w", err)
	}
	return results, nil
}

func (s *Store) GetWebVitalsTimeseries(ctx context.Context, params api.WebVitalsParams) ([]api.WebVitalSeriesPoint, error) {
	where, args, err := buildWebVitalsWhere(params, true)
	if err != nil {
		return nil, err
	}
	unit := webVitalsBucketUnit(params.Start, params.End)

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT
			date_trunc('%s', timestamp)::TIMESTAMPTZ AS bucket,
			COALESCE(quantile_cont(value, 0.75), 0) AS p75,
			COUNT(*) AS samples,
			COUNT(*) FILTER (WHERE rating = 'good') AS good,
			COUNT(*) FILTER (WHERE rating = 'needs_improvement') AS needs_improvement,
			COUNT(*) FILTER (WHERE rating = 'poor') AS poor
		FROM web_vitals
		%s
		GROUP BY bucket
		ORDER BY bucket
	`, unit, where), args...)
	if err != nil {
		return nil, fmt.Errorf("query web vitals timeseries: %w", err)
	}
	defer rows.Close()

	results := []api.WebVitalSeriesPoint{}
	for rows.Next() {
		var item api.WebVitalSeriesPoint
		if err := rows.Scan(&item.Time, &item.P75, &item.Samples, &item.Good, &item.NeedsImprove, &item.Poor); err != nil {
			return nil, fmt.Errorf("scan web vitals timeseries: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read web vitals timeseries: %w", err)
	}
	return results, nil
}

func (s *Store) GetWebVitalsPages(ctx context.Context, params api.WebVitalsParams) ([]api.WebVitalPageRow, error) {
	selectedWhere, selectedArgs, err := buildWebVitalsWhereWithAlias(params, true, "w")
	if err != nil {
		return nil, err
	}
	allMetricParams := params
	allMetricParams.Metric = ""
	allMetricParams.Rating = ""
	allMetricWhere, allMetricArgs, err := buildWebVitalsWhereWithAlias(allMetricParams, false, "w")
	if err != nil {
		return nil, err
	}
	limit := params.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	args := append([]any{}, selectedArgs...)
	args = append(args, limit)
	args = append(args, allMetricArgs...)

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		WITH selected_paths AS (
			SELECT
				w.path,
				COALESCE(quantile_cont(w.value, 0.75), 0) AS p75,
				COUNT(*) AS samples,
				COUNT(*) FILTER (WHERE w.rating = 'good') AS good,
				COUNT(*) FILTER (WHERE w.rating = 'needs_improvement') AS needs_improvement,
				COUNT(*) FILTER (WHERE w.rating = 'poor') AS poor
			FROM web_vitals w
			%s
			GROUP BY w.path
			ORDER BY p75 DESC, samples DESC, w.path
			LIMIT ?
		),
		metric_rows AS (
			SELECT
				w.path,
				w.metric,
				COALESCE(quantile_cont(w.value, 0.75), 0) AS p75,
				COUNT(*) AS samples,
				COUNT(*) FILTER (WHERE w.rating = 'good') AS good,
				COUNT(*) FILTER (WHERE w.rating = 'needs_improvement') AS needs_improvement,
				COUNT(*) FILTER (WHERE w.rating = 'poor') AS poor
			FROM web_vitals w
			JOIN selected_paths sp ON sp.path = w.path
			%s
			GROUP BY w.path, w.metric
		)
		SELECT
			sp.path,
			sp.p75,
			sp.samples,
			sp.good,
			sp.needs_improvement,
			sp.poor,
			mr.metric,
			mr.p75,
			mr.samples,
			mr.good,
			mr.needs_improvement,
			mr.poor
		FROM selected_paths sp
		JOIN metric_rows mr ON mr.path = sp.path
		ORDER BY sp.p75 DESC, sp.samples DESC, sp.path, mr.metric
	`, selectedWhere, allMetricWhere), args...)
	if err != nil {
		return nil, fmt.Errorf("query web vitals pages: %w", err)
	}
	defer rows.Close()

	results := []api.WebVitalPageRow{}
	byPath := map[string]int{}
	for rows.Next() {
		row, err := scanWebVitalPageMetricRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan web vitals page row: %w", err)
		}
		results = appendWebVitalPageMetricRow(results, byPath, params.Metric, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read web vitals page rows: %w", err)
	}
	return results, nil
}

type webVitalPageMetricRow struct {
	path                 string
	selectedP75          float64
	selectedSamples      int64
	selectedGood         int64
	selectedNeedsImprove int64
	selectedPoor         int64
	metric               api.WebVitalMetric
	metricP75            float64
	metricSamples        int64
	metricGood           int64
	metricNeedsImprove   int64
	metricPoor           int64
}

func scanWebVitalPageMetricRow(rows *sql.Rows) (webVitalPageMetricRow, error) {
	var row webVitalPageMetricRow
	err := rows.Scan(
		&row.path,
		&row.selectedP75,
		&row.selectedSamples,
		&row.selectedGood,
		&row.selectedNeedsImprove,
		&row.selectedPoor,
		&row.metric,
		&row.metricP75,
		&row.metricSamples,
		&row.metricGood,
		&row.metricNeedsImprove,
		&row.metricPoor,
	)
	return row, err
}

func appendWebVitalPageMetricRow(results []api.WebVitalPageRow, byPath map[string]int, selectedMetric api.WebVitalMetric, row webVitalPageMetricRow) []api.WebVitalPageRow {
	index, ok := byPath[row.path]
	if !ok {
		item := api.WebVitalPageRow{
			Path:         row.path,
			P75:          row.selectedP75,
			Samples:      row.selectedSamples,
			Good:         row.selectedGood,
			NeedsImprove: row.selectedNeedsImprove,
			Poor:         row.selectedPoor,
			Metrics:      map[api.WebVitalMetric]api.WebVitalMetricBreakdown{},
		}
		item.Rating, _ = WebVitalRatingForValue(selectedMetric, item.P75)
		results = append(results, item)
		index = len(results) - 1
		byPath[row.path] = index
	}

	breakdown := api.WebVitalMetricBreakdown{
		P75:          row.metricP75,
		Samples:      row.metricSamples,
		Good:         row.metricGood,
		NeedsImprove: row.metricNeedsImprove,
		Poor:         row.metricPoor,
	}
	breakdown.Rating, _ = WebVitalRatingForValue(row.metric, breakdown.P75)
	results[index].Metrics[row.metric] = breakdown
	return results
}

func buildWebVitalsWhere(params api.WebVitalsParams, requireMetric bool) (string, []any, error) {
	return buildWebVitalsWhereWithAlias(params, requireMetric, "")
}

func buildWebVitalsWhereWithAlias(params api.WebVitalsParams, requireMetric bool, alias string) (string, []any, error) {
	if params.SiteID == uuid.Nil {
		return "", nil, fmt.Errorf("site_id is required")
	}
	if params.Start.IsZero() || params.End.IsZero() {
		return "", nil, fmt.Errorf("from and to are required")
	}
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	clauses := []string{prefix + "site_id = ?", prefix + "timestamp >= ?", prefix + "timestamp <= ?"}
	args := []any{params.SiteID, params.Start, params.End}
	if params.Metric != "" {
		if _, err := WebVitalRatingForValue(params.Metric, 0); err != nil {
			return "", nil, err
		}
		clauses = append(clauses, prefix+"metric = ?")
		args = append(args, string(params.Metric))
	} else if requireMetric {
		return "", nil, fmt.Errorf("metric is required")
	}
	if params.Path != "" {
		clauses = append(clauses, prefix+"path = ?")
		args = append(args, params.Path)
	}
	if params.Rating != "" {
		switch params.Rating {
		case api.WebVitalRatingGood, api.WebVitalRatingNeedsImprovement, api.WebVitalRatingPoor:
		default:
			return "", nil, fmt.Errorf("unsupported web vital rating %q", params.Rating)
		}
		clauses = append(clauses, prefix+"rating = ?")
		args = append(args, string(params.Rating))
	}
	return "WHERE " + strings.Join(clauses, " AND "), args, nil
}

func (s *Store) GetWebVitalsBreakdown(ctx context.Context, params api.WebVitalsParams, dimension api.WebVitalDimension) ([]api.WebVitalDimensionRow, error) {
	where, args, err := buildWebVitalsWhereWithAlias(params, true, "w")
	if err != nil {
		return nil, err
	}
	expr, err := webVitalsDimensionExpr(dimension)
	if err != nil {
		return nil, err
	}
	limit := params.Limit
	if limit <= 0 || limit > 50 {
		limit = 25
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		WITH hit_context AS (
			SELECT
				site_id,
				page_id,
				arg_min(user_agent, timestamp) AS user_agent,
				arg_min(viewport_width, timestamp) AS viewport_width,
				arg_min(country_code, timestamp) AS country_code,
				arg_min(city, timestamp) AS city,
				arg_min(provider, timestamp) AS provider,
				arg_min(asn, timestamp) AS asn,
				arg_min(asn_org, timestamp) AS asn_org,
				arg_min(language, timestamp) AS language
			FROM hits
			GROUP BY site_id, page_id
		)
		SELECT
			%s AS name,
			COALESCE(quantile_cont(w.value, 0.75), 0) AS p75,
			COUNT(*) AS samples,
			COUNT(*) FILTER (WHERE w.rating = 'good') AS good,
			COUNT(*) FILTER (WHERE w.rating = 'needs_improvement') AS needs_improvement,
			COUNT(*) FILTER (WHERE w.rating = 'poor') AS poor
		FROM web_vitals w
		LEFT JOIN hit_context ctx ON ctx.site_id = w.site_id AND ctx.page_id = w.page_id
		%s
		GROUP BY name
		ORDER BY p75 DESC, samples DESC, name
		LIMIT ?
	`, expr, where), args...)
	if err != nil {
		return nil, fmt.Errorf("query web vitals %s breakdown: %w", dimension, err)
	}
	defer rows.Close()

	results := []api.WebVitalDimensionRow{}
	for rows.Next() {
		var item api.WebVitalDimensionRow
		var name sql.NullString
		if err := rows.Scan(&name, &item.P75, &item.Samples, &item.Good, &item.NeedsImprove, &item.Poor); err != nil {
			return nil, fmt.Errorf("scan web vitals %s breakdown row: %w", dimension, err)
		}
		if name.Valid && strings.TrimSpace(name.String) != "" {
			item.Name = name.String
		} else {
			item.Name = "(Unknown)"
		}
		item.Rating, _ = WebVitalRatingForValue(params.Metric, item.P75)
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read web vitals %s breakdown rows: %w", dimension, err)
	}
	return results, nil
}

func webVitalsDimensionExpr(dimension api.WebVitalDimension) (string, error) {
	switch dimension {
	case api.WebVitalDimensionCountry:
		return "hk_country(ctx.country_code)", nil
	case api.WebVitalDimensionCity:
		return "COALESCE(NULLIF(TRIM(ctx.city), ''), '(Unknown)')", nil
	case api.WebVitalDimensionProvider:
		return "COALESCE(NULLIF(TRIM(ctx.provider), ''), '(Unknown)')", nil
	case api.WebVitalDimensionASN:
		return "hk_asn(ctx.asn, ctx.asn_org)", nil
	case api.WebVitalDimensionLanguage:
		return "CASE WHEN NULLIF(TRIM(ctx.language), '') IS NULL THEN '(Unspecified)' ELSE lower(split_part(TRIM(ctx.language), '-', 1)) END", nil
	case api.WebVitalDimensionBrowser:
		return "hk_browser(ctx.user_agent)", nil
	case api.WebVitalDimensionDevice:
		return "CASE WHEN ctx.viewport_width IS NULL THEN '(Unknown)' ELSE hk_device(ctx.viewport_width) END", nil
	default:
		return "", fmt.Errorf("unsupported web vital dimension %q", dimension)
	}
}

func webVitalsBucketUnit(start, end time.Time) string {
	if end.Sub(start) <= 48*time.Hour {
		return "hour"
	}
	return "day"
}

func nullableNonEmptyString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
