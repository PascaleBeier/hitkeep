package database

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func (s *Store) CreateAIFetch(ctx context.Context, fetch *api.AIFetch) error {
	if fetch == nil {
		return fmt.Errorf("ai fetch is required")
	}
	return s.CreateAIFetchesBulk(ctx, []*api.AIFetch{fetch})
}

func (s *Store) CreateAIFetchesBulk(ctx context.Context, fetches []*api.AIFetch) error {
	if len(fetches) == 0 {
		return nil
	}

	return s.withAppender(ctx, "ai_fetches", func(appender rowAppender) error {
		for _, fetch := range fetches {
			if fetch == nil {
				continue
			}
			if fetch.ID == uuid.Nil {
				id, err := uuid.NewV7()
				if err != nil {
					return fmt.Errorf("generate ai fetch id: %w", err)
				}
				fetch.ID = id
			}
			if fetch.Timestamp.IsZero() {
				fetch.Timestamp = time.Now().UTC()
			}

			if err := appender.AppendRow(
				duckdb.UUID(fetch.ID),
				duckdb.UUID(fetch.SiteID),
				fetch.Timestamp,
				fetch.AssistantName,
				fetch.AssistantFamily,
				fetch.Path,
				nullableStringPtr(fetch.Hostname),
				fetch.StatusCode,
				nullableStringPtr(fetch.ContentType),
				fetch.ResourceType,
				nullableIntPtr(fetch.ResponseMs),
				nullableInt64Ptr(fetch.BytesServed),
				nullableStringPtr(fetch.UserAgent),
			); err != nil {
				return fmt.Errorf("append ai fetch row: %w", err)
			}
		}
		return nil
	})
}

func (s *Store) GetAIFetchOverview(ctx context.Context, params api.AIFetchQueryParams) (*api.AIFetchOverview, error) {
	overview := &api.AIFetchOverview{
		TopAssistants:     []api.MetricStat{},
		TopFamilies:       []api.MetricStat{},
		TopPaths:          []api.MetricStat{},
		TopErrorPaths:     []api.MetricStat{},
		ResourceTypeSplit: []api.MetricStat{},
	}

	filterSQL, args := buildAIFetchFilters(params)

	//nolint:gosec,dupword // filterSQL only contains parameterized clauses assembled from a fixed allowlist.
	query := `
		WITH base AS (
			SELECT assistant_name, assistant_family, path, resource_type, status_code
			FROM ai_fetches
			WHERE site_id = ? AND timestamp >= ? AND timestamp <= ?` + filterSQL + `
		),
		summary AS (
			SELECT
				COUNT(*) AS total_requests,
				COUNT(DISTINCT path) AS unique_paths,
				COUNT(DISTINCT assistant_name) AS unique_assistants,
				COALESCE(ROUND(COUNT(*) FILTER (WHERE status_code BETWEEN 400 AND 499) * 100.0 / NULLIF(COUNT(*), 0), 2), 0) AS error_rate_4xx,
				COALESCE(ROUND(COUNT(*) FILTER (WHERE status_code BETWEEN 500 AND 599) * 100.0 / NULLIF(COUNT(*), 0), 2), 0) AS error_rate_5xx,
				COALESCE(CAST(ROUND(MEDIAN(response_ms)) AS BIGINT), 0) AS median_response_ms,
				COALESCE(SUM(bytes_served), 0) AS total_bytes
			FROM ai_fetches
			WHERE site_id = ? AND timestamp >= ? AND timestamp <= ?` + filterSQL + `
		),
		combined AS (
			SELECT 'assistant' AS dim, assistant_name AS name, COUNT(*) AS val
			FROM base
			GROUP BY assistant_name
			UNION ALL
			SELECT 'family' AS dim, assistant_family AS name, COUNT(*) AS val
			FROM base
			GROUP BY assistant_family
			UNION ALL
			SELECT 'path' AS dim, path AS name, COUNT(*) AS val
			FROM base
			GROUP BY path
			UNION ALL
			SELECT 'error_path' AS dim, path AS name, COUNT(*) AS val
			FROM base
			WHERE status_code >= 400
			GROUP BY path
			UNION ALL
			SELECT 'resource_type' AS dim, resource_type AS name, COUNT(*) AS val
			FROM base
			GROUP BY resource_type
		),
		ranked AS (
			SELECT dim, name, val, ROW_NUMBER() OVER (PARTITION BY dim ORDER BY val DESC, name ASC) AS rn
			FROM combined
			WHERE name IS NOT NULL
		),
		results AS (
			SELECT
				'__summary__' AS dim,
				'' AS name,
				0 AS val,
				total_requests,
				unique_paths,
				unique_assistants,
				error_rate_4xx,
				error_rate_5xx,
				median_response_ms,
				total_bytes
			FROM summary
			UNION ALL
			SELECT
				dim,
				name,
				val,
				NULL,
				NULL,
				NULL,
				NULL,
				NULL,
				NULL,
				NULL
			FROM ranked
			WHERE rn <= 10
		)
		SELECT
			dim,
			name,
			val,
			total_requests,
			unique_paths,
			unique_assistants,
			error_rate_4xx,
			error_rate_5xx,
			median_response_ms,
			total_bytes
		FROM results
		ORDER BY dim, val DESC, name ASC`

	queryArgs := make([]any, 0, 3+len(args)+3+len(args))
	queryArgs = append(queryArgs, params.SiteID, params.Start, params.End)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, params.SiteID, params.Start, params.End)
	queryArgs = append(queryArgs, args...)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			dim              string
			item             api.MetricStat
			totalRequests    sql.NullInt64
			uniquePaths      sql.NullInt64
			uniqueAssistants sql.NullInt64
			errorRate4xx     sql.NullFloat64
			errorRate5xx     sql.NullFloat64
			medianResponseMs sql.NullInt64
			totalBytes       sql.NullInt64
		)
		if err := rows.Scan(
			&dim,
			&item.Name,
			&item.Value,
			&totalRequests,
			&uniquePaths,
			&uniqueAssistants,
			&errorRate4xx,
			&errorRate5xx,
			&medianResponseMs,
			&totalBytes,
		); err != nil {
			return nil, err
		}
		if dim == "__summary__" {
			if totalRequests.Valid {
				overview.TotalRequests = totalRequests.Int64
			}
			if uniquePaths.Valid {
				overview.UniquePaths = uniquePaths.Int64
			}
			if uniqueAssistants.Valid {
				overview.UniqueAssistants = uniqueAssistants.Int64
			}
			if errorRate4xx.Valid {
				overview.ErrorRate4xx = errorRate4xx.Float64
			}
			if errorRate5xx.Valid {
				overview.ErrorRate5xx = errorRate5xx.Float64
			}
			if medianResponseMs.Valid {
				overview.MedianResponseMs = int(medianResponseMs.Int64)
			}
			if totalBytes.Valid {
				overview.TotalBytes = totalBytes.Int64
			}
			continue
		}
		switch dim {
		case "assistant":
			overview.TopAssistants = append(overview.TopAssistants, item)
		case "family":
			overview.TopFamilies = append(overview.TopFamilies, item)
		case "path":
			overview.TopPaths = append(overview.TopPaths, item)
		case "error_path":
			overview.TopErrorPaths = append(overview.TopErrorPaths, item)
		case "resource_type":
			overview.ResourceTypeSplit = append(overview.ResourceTypeSplit, item)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return overview, nil
}

func (s *Store) GetAIFetchTimeseries(ctx context.Context, params api.AIFetchQueryParams) ([]api.AIFetchSeriesPoint, error) {
	duration := params.End.Sub(params.Start)
	truncUnit := "day"
	if duration < 48*time.Hour {
		truncUnit = "hour"
	} else if duration >= 180*24*time.Hour {
		truncUnit = "month"
	}

	filterSQL, args := buildAIFetchFilters(params)
	//nolint:gosec // truncUnit is selected from a fixed allowlist and filterSQL is parameterized.
	query := `
		SELECT date_trunc('` + truncUnit + `', timestamp) AS bucket, COUNT(*) AS cnt
		FROM ai_fetches
		WHERE site_id = ? AND timestamp >= ? AND timestamp <= ?` + filterSQL + `
		GROUP BY bucket
		ORDER BY bucket`

	queryArgs := make([]any, 0, 3+len(args))
	queryArgs = append(queryArgs, params.SiteID, params.Start, params.End)
	queryArgs = append(queryArgs, args...)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []api.AIFetchSeriesPoint
	for rows.Next() {
		var point api.AIFetchSeriesPoint
		if err := rows.Scan(&point.Time, &point.Count); err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	if points == nil {
		points = []api.AIFetchSeriesPoint{}
	}
	return points, rows.Err()
}

func (s *Store) GetAIFetchCorrelation(ctx context.Context, params api.AIFetchCorrelationParams) (*api.AIFetchCorrelationReport, error) {
	report := &api.AIFetchCorrelationReport{
		CitationYield:    []api.AIFetchCitationYieldRow{},
		OpportunityPages: []api.AIFetchOpportunityRow{},
		FailureHotspots:  []api.AIFetchFailureHotspot{},
	}

	filterSQL, filterArgs := buildAIFetchFiltersWithAlias(params.AssistantName, params.AssistantFamily, params.ResourceType, "f.")
	windowDays := params.WindowDays
	if windowDays <= 0 {
		windowDays = 30
	}

	// !!! Assistant/family/resource filters scope the fetch side only. The correlated
	// visit side intentionally counts any later AI-referred human visits on the
	// same path within the window, regardless of which assistant referred them.
	//nolint:gosec,dupword // filterSQL is allowlisted and windowDays is range-validated by handlers/defaults.
	query := fmt.Sprintf(`
		WITH base_fetches AS (
			SELECT
				f.id,
				f.assistant_name,
				f.path,
				f.status_code,
				f.timestamp
			FROM ai_fetches f
			WHERE f.site_id = ? AND f.timestamp >= ? AND f.timestamp <= ?%s
		),
		ai_visits AS (
			SELECT
				h.path,
				h.session_id,
				MIN(h.timestamp) AS visit_at
			FROM hits h
			WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ? AND hk_ai_source(h.referrer) IS NOT NULL
			GROUP BY h.path, h.session_id
		),
		matched AS (
			SELECT
				f.id,
				f.assistant_name,
				f.path,
				f.status_code,
				v.session_id
			FROM base_fetches f
			LEFT JOIN ai_visits v
				ON v.path = f.path
				AND v.visit_at >= f.timestamp
				AND v.visit_at <= LEAST(f.timestamp + INTERVAL '%d days', ?)
		),
		fetch_summary AS (
			SELECT COUNT(*) AS total_fetches, COUNT(DISTINCT path) AS fetched_paths
			FROM base_fetches
		),
		correlation_summary AS (
			SELECT
				COUNT(DISTINCT CASE WHEN session_id IS NOT NULL THEN path END) AS correlated_paths,
				COUNT(DISTINCT session_id) AS ai_referred_visits,
				COUNT(*) FILTER (WHERE session_id IS NULL) AS uncorrelated_fetches
			FROM matched
		),
		citation_yield AS (
			SELECT
				path,
				assistant_name,
				COUNT(*) AS fetch_count,
				COUNT(DISTINCT session_id) AS ai_referred_visits,
				COALESCE(ROUND(COUNT(DISTINCT session_id) * 100.0 / NULLIF(COUNT(*), 0), 2), 0) AS citation_yield_pct
			FROM matched
			GROUP BY path, assistant_name
		),
		opportunity_pages AS (
			SELECT
				path,
				COUNT(*) AS fetch_count,
				COUNT(DISTINCT session_id) AS ai_referred_visits,
				COUNT(*) FILTER (WHERE status_code >= 400) AS error_requests,
				COALESCE(ROUND(COUNT(*) FILTER (WHERE status_code >= 400) * 100.0 / NULLIF(COUNT(*), 0), 2), 0) AS error_rate_pct
			FROM matched
			GROUP BY path
		),
		failure_hotspots AS (
			SELECT
				assistant_name,
				CASE
					WHEN regexp_extract(path, '^/[^/?#]+') = '' THEN '/'
					ELSE regexp_extract(path, '^/[^/?#]+')
				END AS path_prefix,
				COUNT(*) AS total_requests,
				COUNT(*) FILTER (WHERE status_code >= 400) AS error_requests,
				COALESCE(ROUND(COUNT(*) FILTER (WHERE status_code >= 400) * 100.0 / NULLIF(COUNT(*), 0), 2), 0) AS error_rate_pct
			FROM base_fetches
			GROUP BY assistant_name, path_prefix
		),
		ranked_citation_yield AS (
			SELECT *, ROW_NUMBER() OVER (ORDER BY citation_yield_pct DESC, ai_referred_visits DESC, fetch_count DESC, path ASC, assistant_name ASC) AS rn
			FROM citation_yield
		),
		ranked_opportunity_pages AS (
			SELECT *, ROW_NUMBER() OVER (ORDER BY fetch_count DESC, ai_referred_visits ASC, error_requests DESC, path ASC) AS rn
			FROM opportunity_pages
		),
		ranked_failure_hotspots AS (
			SELECT *, ROW_NUMBER() OVER (ORDER BY error_rate_pct DESC, error_requests DESC, total_requests DESC, assistant_name ASC, path_prefix ASC) AS rn
			FROM failure_hotspots
			WHERE error_requests > 0
		),
		results AS (
			SELECT
				'__summary__' AS section,
				'' AS name1,
				'' AS name2,
				COALESCE(fs.total_fetches, 0) AS v1,
				COALESCE(fs.fetched_paths, 0) AS v2,
				COALESCE(cs.correlated_paths, 0) AS v3,
				COALESCE(cs.ai_referred_visits, 0) AS v4,
				COALESCE(cs.uncorrelated_fetches, 0) AS v5
			FROM fetch_summary fs
			CROSS JOIN correlation_summary cs
			UNION ALL
			-- Percentages are pre-rounded to 2 decimals in SQL, then encoded as
			-- scaled integers so we can carry them through the UNION shape without
			-- widening every branch to DECIMAL/DOUBLE handling in Go scans.
			SELECT 'citation_yield', path, assistant_name, fetch_count, ai_referred_visits, CAST(citation_yield_pct * 100 AS BIGINT), 0, 0
			FROM ranked_citation_yield
			WHERE rn <= 10
			UNION ALL
			SELECT 'opportunity_pages', path, '', fetch_count, ai_referred_visits, error_requests, CAST(error_rate_pct * 100 AS BIGINT), 0
			FROM ranked_opportunity_pages
			WHERE rn <= 10
			UNION ALL
			SELECT 'failure_hotspots', assistant_name, path_prefix, total_requests, error_requests, CAST(error_rate_pct * 100 AS BIGINT), 0, 0
			FROM ranked_failure_hotspots
			WHERE rn <= 10
		)
		SELECT section, name1, name2, v1, v2, v3, v4, v5
		FROM results
		ORDER BY CASE section
			WHEN '__summary__' THEN 0
			WHEN 'citation_yield' THEN 1
			WHEN 'opportunity_pages' THEN 2
			ELSE 3
		END, v1 DESC, name1 ASC, name2 ASC
	`, filterSQL, windowDays)

	args := make([]any, 0, 3+len(filterArgs)+4)
	args = append(args, params.SiteID, params.Start, params.End)
	args = append(args, filterArgs...)
	args = append(args, params.SiteID, params.Start, params.End, params.End)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			section string
			name1   string
			name2   string
			v1      int64
			v2      int64
			v3      int64
			v4      int64
			v5      int64
		)
		if err := rows.Scan(&section, &name1, &name2, &v1, &v2, &v3, &v4, &v5); err != nil {
			return nil, err
		}

		switch section {
		case "__summary__":
			report.Summary = api.AIFetchCorrelationSummary{
				TotalFetches:        v1,
				FetchedPaths:        v2,
				CorrelatedPaths:     v3,
				AIReferredVisits:    v4,
				UncorrelatedFetches: v5,
			}
		case "citation_yield":
			report.CitationYield = append(report.CitationYield, api.AIFetchCitationYieldRow{
				Path:             name1,
				AssistantName:    name2,
				FetchCount:       v1,
				AIReferredVisits: v2,
				CitationYieldPct: float64(v3) / 100,
			})
		case "opportunity_pages":
			report.OpportunityPages = append(report.OpportunityPages, api.AIFetchOpportunityRow{
				Path:             name1,
				FetchCount:       v1,
				AIReferredVisits: v2,
				ErrorRequests:    v3,
				ErrorRatePct:     float64(v4) / 100,
			})
		case "failure_hotspots":
			report.FailureHotspots = append(report.FailureHotspots, api.AIFetchFailureHotspot{
				AssistantName: name1,
				PathPrefix:    name2,
				TotalRequests: v1,
				ErrorRequests: v2,
				ErrorRatePct:  float64(v3) / 100,
			})
		}
	}

	return report, rows.Err()
}

func buildAIFetchFilters(params api.AIFetchQueryParams) (string, []any) {
	return buildAIFetchFiltersWithAlias(params.AssistantName, params.AssistantFamily, params.ResourceType, "")
}

func buildAIFetchFiltersWithAlias(assistantName, assistantFamily, resourceType, prefix string) (string, []any) {
	sqlFilter := ""
	args := []any{}

	if assistantName != "" {
		sqlFilter += " AND " + prefix + "assistant_name = ?"
		args = append(args, assistantName)
	}
	if assistantFamily != "" {
		sqlFilter += " AND " + prefix + "assistant_family = ?"
		args = append(args, assistantFamily)
	}
	if resourceType != "" {
		sqlFilter += " AND " + prefix + "resource_type = ?"
		args = append(args, resourceType)
	}

	return sqlFilter, args
}

func (s *Store) ExportAIFetchCSV(ctx context.Context, params api.AIFetchQueryParams, w io.Writer) error {
	selectQuery, args := buildAIFetchExportQuery(params)

	rows, err := s.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return fmt.Errorf("failed to query ai fetches for export: %w", err)
	}
	defer rows.Close()

	writer := csv.NewWriter(w)
	if err := writer.Write([]string{
		"id",
		"site_id",
		"timestamp",
		"assistant_name",
		"assistant_family",
		"path",
		"hostname",
		"status_code",
		"content_type",
		"resource_type",
		"response_ms",
		"bytes_served",
		"user_agent",
	}); err != nil {
		return fmt.Errorf("failed to write csv header: %w", err)
	}

	for rows.Next() {
		var (
			id, siteID                           uuid.UUID
			timestamp                            time.Time
			assistantName, assistantFamily, path string
			hostname, contentType, userAgent     sql.NullString
			statusCode                           int
			resourceType                         string
			responseMs                           sql.NullInt32
			bytesServed                          sql.NullInt64
		)
		if err := rows.Scan(
			&id, &siteID, &timestamp, &assistantName, &assistantFamily, &path,
			&hostname, &statusCode, &contentType, &resourceType, &responseMs,
			&bytesServed, &userAgent,
		); err != nil {
			return fmt.Errorf("failed to scan ai fetch export row: %w", err)
		}

		record := []string{
			id.String(),
			siteID.String(),
			timestamp.Format(time.RFC3339),
			assistantName,
			assistantFamily,
			path,
			nullString(hostname),
			fmt.Sprintf("%d", statusCode),
			nullString(contentType),
			resourceType,
			nullInt32(responseMs),
			nullInt64(bytesServed),
			nullString(userAgent),
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write csv record: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read ai fetch export rows: %w", err)
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush csv: %w", err)
	}
	return nil
}

func (s *Store) ExportAIFetchFile(ctx context.Context, params api.AIFetchQueryParams, format string) (string, error) {
	selectQuery, args := buildAIFetchExportQuery(params)
	filename, err := s.exportQueryToTempFile(ctx, "hitkeep_aifetch_", "hitkeep_aifetch_", selectQuery, args, format)
	if err != nil {
		return "", fmt.Errorf("failed to export ai fetches: %w", err)
	}

	return filename, nil
}

func buildAIFetchExportQuery(params api.AIFetchQueryParams) (string, []any) {
	baseQuery := `
		FROM ai_fetches
		WHERE site_id = ?
		  AND timestamp >= ?
		  AND timestamp <= ?
	`
	filterSQL, filterArgs := buildAIFetchFilters(params)
	args := make([]any, 0, 3+len(filterArgs))
	args = append(args, params.SiteID, params.Start, params.End)
	baseQuery += filterSQL
	args = append(args, filterArgs...)

	baseQuery += " ORDER BY timestamp DESC"

	//nolint:gosec // baseQuery is built from fixed allowlists and parameter placeholders
	selectQuery := `
		SELECT
			id, site_id, timestamp, assistant_name, assistant_family, path,
			hostname, status_code, content_type, resource_type, response_ms,
			bytes_served, user_agent
	` + baseQuery

	return selectQuery, args
}

func nullInt64(value sql.NullInt64) string {
	if value.Valid {
		return fmt.Sprintf("%d", value.Int64)
	}
	return ""
}

func nullableInt64Ptr(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
