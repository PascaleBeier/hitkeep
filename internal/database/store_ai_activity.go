package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"hitkeep/internal/api"
)

// aiActivityCategoryDimPrefix namespaces the per-category agent top lists
// inside the shared dim column of the merged AI activity query, exactly like
// aiBotCategoryDimPrefix does for GetSiteStats.
const aiActivityCategoryDimPrefix = "agent_cat::"

// aiActivityTopListLimit caps every merged top list. Scalars that describe
// cardinality (paths_crawled, unique_agents) are counted before the cap so they
// never become an artifact of the limit.
const aiActivityTopListLimit = 10

// aiActivitySummaryDim marks the single summary row inside the merged result set.
const aiActivitySummaryDim = "__summary__"

// GetAIActivity returns the unified AI activity report for one site: a single
// merged view over tracked AI hits and server-log AI fetch records.
//
// Tracked hits are classified at query time — hk_ai_bot resolves the agent from
// the user agent, hk_ai_bot_category_from_name maps that agent name to its
// category, and hk_ai_source resolves AI referral surfaces from the referrer.
// Fetch records carry their own assistant_name/family/category, except that
// rows ingested before the category dimension existed have a NULL category and
// fall back to hk_ai_bot_category_from_name(assistant_name).
//
// Every count follows one rule: value = tracked hits + fetch records. Each row
// keeps the provenance split so callers can tell the two sides apart.
//
// Filter semantics, since the two tables do not share every dimension:
//   - ai_bot          → hits hk_ai_bot(user_agent), fetches assistant_name
//   - ai_bot_category → hits macro, fetches COALESCE(category, from_name(name))
//   - path            → both sides
//   - ai_source       → hits only; the fetch side is zeroed because ai_fetches
//     carries no referrer, so a referral filter cannot select fetch records
//   - every other hit dimension (country, device, browser, ...) → hits only.
//     ai_fetches has no such column, so the fetch side stays unrestricted
//     rather than being silently emptied.
//
// The pageviews denominator deliberately ignores AI dimensions: it is the total
// hit volume for the range under the non-AI filters only, so the AI share of a
// segment stays comparable when an AI dimension is drilled into.
//
// The work is split across three statements — merged summary plus top lists,
// the merged series, and the optional comparison scalars — mirroring how
// GetSiteStats splits its own query set.
func (s *Store) GetAIActivity(ctx context.Context, params api.AnalyticsParams) (*api.AIActivityReport, error) {
	report := &api.AIActivityReport{
		TopAgents:           []api.AIActivityStat{},
		TopCategories:       []api.AIActivityStat{},
		TopPaths:            []api.AIActivityStat{},
		TopSources:          []api.AIActivityStat{},
		TopFamilies:         []api.AIActivityStat{},
		TopResourceTypes:    []api.AIActivityStat{},
		TopErrorPaths:       []api.AIActivityStat{},
		TopAgentsByCategory: map[string][]api.AIActivityStat{},
		Series:              []api.AIActivitySeriesPoint{},
	}

	hitFilterSQL, hitFilterArgs := buildHitFilters(params.Filters, "h")
	fetchFilterSQL, fetchFilterArgs := buildAIActivityFetchFilters(params.Filters, "f")
	pageviewFilterSQL, pageviewFilterArgs := buildHitFilters(nonAIHitFilters(params.Filters), "h")

	rangeArgs := func(start, end time.Time) []any {
		args := make([]any, 0, 9+len(hitFilterArgs)+len(fetchFilterArgs)+len(pageviewFilterArgs))
		args = append(args, params.SiteID, start, end)
		args = append(args, hitFilterArgs...)
		args = append(args, params.SiteID, start, end)
		args = append(args, fetchFilterArgs...)
		args = append(args, params.SiteID, start, end)
		args = append(args, pageviewFilterArgs...)
		return args
	}

	ctes := aiActivityCTEs(hitFilterSQL, fetchFilterSQL, pageviewFilterSQL)

	//nolint:gosec // ctes is assembled from fixed SQL fragments; every filter clause is parameterized.
	mainQuery := `WITH ` + ctes + `,
		ranked AS (
			SELECT
				dim,
				name,
				tracked,
				fetched,
				ROW_NUMBER() OVER (PARTITION BY dim ORDER BY (tracked + fetched) DESC, name ASC) AS rn
			FROM merged
		),
		results AS (
			SELECT
				'` + aiActivitySummaryDim + `' AS dim,
				CAST(NULL AS VARCHAR) AS name,
				tracked_hits AS tracked,
				fetch_count AS fetched,
				referral_visits,
				paths_crawled,
				unique_agents,
				pageviews,
				error_rate_4xx,
				error_rate_5xx,
				median_response_ms,
				total_bytes
			FROM summary
			UNION ALL
			SELECT
				dim,
				name,
				tracked,
				fetched,
				CAST(NULL AS BIGINT),
				CAST(NULL AS BIGINT),
				CAST(NULL AS BIGINT),
				CAST(NULL AS BIGINT),
				CAST(NULL AS DOUBLE),
				CAST(NULL AS DOUBLE),
				CAST(NULL AS BIGINT),
				CAST(NULL AS BIGINT)
			FROM ranked
			WHERE rn <= ` + fmt.Sprint(aiActivityTopListLimit) + `
		)
		SELECT * FROM results
		ORDER BY CASE WHEN dim = '` + aiActivitySummaryDim + `' THEN 0 ELSE 1 END, dim, (tracked + fetched) DESC, name ASC`

	rows, err := s.db.QueryContext(ctx, mainQuery, rangeArgs(params.Start, params.End)...)
	if err != nil {
		return nil, fmt.Errorf("query ai activity report: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			dim              string
			name             sql.NullString
			tracked          int64
			fetched          int64
			referralVisits   sql.NullInt64
			pathsCrawled     sql.NullInt64
			uniqueAgents     sql.NullInt64
			pageviews        sql.NullInt64
			errorRate4xx     sql.NullFloat64
			errorRate5xx     sql.NullFloat64
			medianResponseMs sql.NullInt64
			totalBytes       sql.NullInt64
		)
		if err := rows.Scan(
			&dim, &name, &tracked, &fetched,
			&referralVisits, &pathsCrawled, &uniqueAgents, &pageviews,
			&errorRate4xx, &errorRate5xx, &medianResponseMs, &totalBytes,
		); err != nil {
			return nil, fmt.Errorf("scan ai activity row: %w", err)
		}

		if dim == aiActivitySummaryDim {
			report.TrackedHits = int(tracked)
			report.FetchCount = int(fetched)
			report.AIRequests = report.TrackedHits + report.FetchCount
			report.ReferralVisits = int(referralVisits.Int64)
			report.PathsCrawled = int(pathsCrawled.Int64)
			report.UniqueAgents = int(uniqueAgents.Int64)
			report.Pageviews = int(pageviews.Int64)
			report.ErrorRate4xx = errorRate4xx.Float64
			report.ErrorRate5xx = errorRate5xx.Float64
			report.MedianResponseMs = int(medianResponseMs.Int64)
			report.TotalBytes = totalBytes.Int64
			continue
		}

		stat := api.AIActivityStat{
			Name:        name.String,
			Value:       int(tracked + fetched),
			TrackedHits: int(tracked),
			FetchCount:  int(fetched),
		}
		if category, ok := strings.CutPrefix(dim, aiActivityCategoryDimPrefix); ok {
			report.TopAgentsByCategory[category] = append(report.TopAgentsByCategory[category], stat)
			continue
		}
		switch dim {
		case "agent":
			report.TopAgents = append(report.TopAgents, stat)
		case "category":
			report.TopCategories = append(report.TopCategories, stat)
		case "path":
			report.TopPaths = append(report.TopPaths, stat)
		case "source":
			report.TopSources = append(report.TopSources, stat)
		case "family":
			report.TopFamilies = append(report.TopFamilies, stat)
		case "resource_type":
			report.TopResourceTypes = append(report.TopResourceTypes, stat)
		case "error_path":
			report.TopErrorPaths = append(report.TopErrorPaths, stat)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read ai activity rows: %w", err)
	}

	series, err := s.aiActivitySeries(ctx, params, hitFilterSQL, hitFilterArgs, fetchFilterSQL, fetchFilterArgs)
	if err != nil {
		return nil, err
	}
	report.Series = series

	// Both ends must be present: the compare params are parsed leniently, so a
	// half-specified window would measure against [start, zero time] and report
	// an empty baseline as a 100% drop.
	if !params.CompareStart.IsZero() && !params.CompareEnd.IsZero() {
		//nolint:gosec // ctes is assembled from fixed SQL fragments; every filter clause is parameterized.
		comparisonQuery := `WITH ` + ctes + `
			SELECT tracked_hits, fetch_count, referral_visits, paths_crawled, unique_agents, pageviews
			FROM summary`

		var (
			trackedHits    int64
			fetchCount     int64
			referralVisits int64
			pathsCrawled   int64
			uniqueAgents   int64
			pageviews      int64
		)
		if err := s.db.QueryRowContext(ctx, comparisonQuery, rangeArgs(params.CompareStart, params.CompareEnd)...).Scan(
			&trackedHits, &fetchCount, &referralVisits, &pathsCrawled, &uniqueAgents, &pageviews,
		); err != nil {
			return nil, fmt.Errorf("query ai activity comparison: %w", err)
		}
		report.Comparison = &api.AIActivityScalars{
			AIRequests:     int(trackedHits + fetchCount),
			TrackedHits:    int(trackedHits),
			FetchCount:     int(fetchCount),
			ReferralVisits: int(referralVisits),
			PathsCrawled:   int(pathsCrawled),
			UniqueAgents:   int(uniqueAgents),
			Pageviews:      int(pageviews),
		}
	}

	return report, nil
}

// aiActivityCTEs builds the shared CTE prelude. Placeholders appear in CTE
// order: hit_rows, then fetch_rows, then pageview_rows — callers must bind
// their arguments in exactly that order.
//
// hit_rows classifies each hit once: the ~200-branch user-agent pattern walk
// runs for the agent name only, and the category is derived from that resolved
// name instead of walking the patterns a second time.
func aiActivityCTEs(hitFilterSQL, fetchFilterSQL, pageviewFilterSQL string) string {
	return fmt.Sprintf(`
		hit_rows AS (
			SELECT
				classified.*,
				hk_ai_bot_category_from_name(classified.agent) AS category
			FROM (
				SELECT
					hk_ai_bot(h.user_agent) AS agent,
					hk_ai_source(h.referrer) AS source,
					h.path AS path,
					h.session_id AS session_id
				FROM hits h
				WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s
			) classified
		),
		fetch_rows AS (
			SELECT
				f.assistant_name AS agent,
				COALESCE(NULLIF(TRIM(f.assistant_category), ''), hk_ai_bot_category_from_name(f.assistant_name)) AS category,
				f.assistant_family AS family,
				f.path AS path,
				f.resource_type AS resource_type,
				f.status_code AS status_code,
				f.response_ms AS response_ms,
				f.bytes_served AS bytes_served
			FROM ai_fetches f
			WHERE f.site_id = ? AND f.timestamp >= ? AND f.timestamp <= ?%s
		),
		pageview_rows AS (
			SELECT 1 AS counted
			FROM hits h
			WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s
		),
		dim_rows AS (
			SELECT 'agent' AS dim, agent AS name, CAST(1 AS BIGINT) AS tracked, CAST(0 AS BIGINT) AS fetched
			FROM hit_rows WHERE agent IS NOT NULL
			UNION ALL
			SELECT 'category', category, 1, 0 FROM hit_rows WHERE agent IS NOT NULL AND category IS NOT NULL
			UNION ALL
			SELECT 'path', path, 1, 0 FROM hit_rows WHERE agent IS NOT NULL AND path IS NOT NULL
			UNION ALL
			SELECT '%s' || category, agent, 1, 0 FROM hit_rows WHERE agent IS NOT NULL AND category IS NOT NULL
			UNION ALL
			-- Referral surfaces are measured in distinct sessions, so they are
			-- de-duplicated before entering the shared per-row merge.
			SELECT 'source', source, 1, 0
			FROM (SELECT DISTINCT source, session_id FROM hit_rows WHERE source IS NOT NULL)
			UNION ALL
			SELECT 'agent', agent, 0, 1 FROM fetch_rows WHERE agent IS NOT NULL
			UNION ALL
			SELECT 'category', category, 0, 1 FROM fetch_rows WHERE category IS NOT NULL
			UNION ALL
			SELECT 'path', path, 0, 1 FROM fetch_rows WHERE path IS NOT NULL
			UNION ALL
			SELECT '%s' || category, agent, 0, 1 FROM fetch_rows WHERE agent IS NOT NULL AND category IS NOT NULL
			UNION ALL
			SELECT 'family', family, 0, 1 FROM fetch_rows WHERE family IS NOT NULL
			UNION ALL
			SELECT 'resource_type', resource_type, 0, 1 FROM fetch_rows WHERE resource_type IS NOT NULL
			UNION ALL
			SELECT 'error_path', path, 0, 1 FROM fetch_rows WHERE status_code >= 400 AND path IS NOT NULL
		),
		merged AS (
			SELECT
				dim,
				name,
				CAST(SUM(tracked) AS BIGINT) AS tracked,
				CAST(SUM(fetched) AS BIGINT) AS fetched
			FROM dim_rows
			-- The single null guard for the dimension rows: it also keeps a NULL
			-- name out of merged_scalars, where it would inflate the cardinality
			-- counts. Scanning downstream therefore never sees a NULL name except
			-- on the summary row, which carries one by construction.
			WHERE name IS NOT NULL
			GROUP BY dim, name
		),
		-- Every scalar below is aggregated in ONE pass per source CTE. Splitting
		-- them into per-scalar subqueries let the optimizer clone the whole
		-- classification pipeline once per subquery — with a ~200-branch macro
		-- inside it, that grew the plan to hundreds of thousands of nodes and
		-- turned a 0.1s scan into seconds of planning.
		hit_scalars AS (
			SELECT
				CAST(COUNT(*) FILTER (WHERE agent IS NOT NULL) AS BIGINT) AS tracked_hits,
				CAST(COUNT(DISTINCT session_id) FILTER (WHERE source IS NOT NULL) AS BIGINT) AS referral_visits
			FROM hit_rows
		),
		fetch_scalars AS (
			SELECT
				CAST(COUNT(*) AS BIGINT) AS fetch_count,
				COALESCE(ROUND(COUNT(*) FILTER (WHERE status_code BETWEEN 400 AND 499) * 100.0 / NULLIF(COUNT(*), 0), 2), 0) AS error_rate_4xx,
				COALESCE(ROUND(COUNT(*) FILTER (WHERE status_code BETWEEN 500 AND 599) * 100.0 / NULLIF(COUNT(*), 0), 2), 0) AS error_rate_5xx,
				COALESCE(CAST(ROUND(MEDIAN(response_ms)) AS BIGINT), 0) AS median_response_ms,
				COALESCE(CAST(SUM(bytes_served) AS BIGINT), 0) AS total_bytes
			FROM fetch_rows
		),
		-- Counted over the merged dimension rows, before the top-list cap, so
		-- cardinality is never a top-10 artifact.
		merged_scalars AS (
			SELECT
				CAST(COUNT(*) FILTER (WHERE dim = 'path') AS BIGINT) AS paths_crawled,
				CAST(COUNT(*) FILTER (WHERE dim = 'agent') AS BIGINT) AS unique_agents
			FROM merged
		),
		pageview_scalars AS (
			SELECT CAST(COUNT(*) AS BIGINT) AS pageviews FROM pageview_rows
		),
		summary AS (
			SELECT
				hit_scalars.tracked_hits,
				fetch_scalars.fetch_count,
				hit_scalars.referral_visits,
				merged_scalars.paths_crawled,
				merged_scalars.unique_agents,
				pageview_scalars.pageviews,
				fetch_scalars.error_rate_4xx,
				fetch_scalars.error_rate_5xx,
				fetch_scalars.median_response_ms,
				fetch_scalars.total_bytes
			FROM hit_scalars, fetch_scalars, merged_scalars, pageview_scalars
		)`,
		hitFilterSQL,
		fetchFilterSQL,
		pageviewFilterSQL,
		aiActivityCategoryDimPrefix,
		aiActivityCategoryDimPrefix,
	)
}

// aiActivitySeries buckets both sides on the same grid and merges them, so the
// series always agrees with the merged scalars above it. referral_visits counts
// distinct sessions inside each bucket, which makes bucket values intentionally
// non-additive: a session spanning two buckets counts in both.
func (s *Store) aiActivitySeries(
	ctx context.Context,
	params api.AnalyticsParams,
	hitFilterSQL string,
	hitFilterArgs []any,
	fetchFilterSQL string,
	fetchFilterArgs []any,
) ([]api.AIActivitySeriesPoint, error) {
	truncUnit := truncUnitForRange(params.Start, params.End)

	//nolint:gosec // bucket expressions come from a fixed allowlist and every filter clause is parameterized.
	query := `
		WITH hit_buckets AS (
			SELECT
				` + bucketSQL("h.timestamp", truncUnit) + ` AS bucket,
				COUNT(*) FILTER (WHERE hk_ai_bot(h.user_agent) IS NOT NULL) AS tracked,
				CAST(0 AS BIGINT) AS fetched,
				COUNT(DISTINCT h.session_id) FILTER (WHERE hk_ai_source(h.referrer) IS NOT NULL) AS referral_visits
			FROM hits h
			WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?` + hitFilterSQL + `
			GROUP BY bucket
		),
		fetch_buckets AS (
			SELECT
				` + bucketSQL("f.timestamp", truncUnit) + ` AS bucket,
				CAST(0 AS BIGINT) AS tracked,
				COUNT(*) AS fetched,
				CAST(0 AS BIGINT) AS referral_visits
			FROM ai_fetches f
			WHERE f.site_id = ? AND f.timestamp >= ? AND f.timestamp <= ?` + fetchFilterSQL + `
			GROUP BY bucket
		)
		SELECT
			bucket,
			CAST(SUM(tracked) AS BIGINT) AS tracked,
			CAST(SUM(fetched) AS BIGINT) AS fetched,
			CAST(SUM(referral_visits) AS BIGINT) AS referral_visits
		FROM (SELECT * FROM hit_buckets UNION ALL SELECT * FROM fetch_buckets)
		GROUP BY bucket
		ORDER BY bucket`

	args := make([]any, 0, 6+len(hitFilterArgs)+len(fetchFilterArgs))
	args = append(args, params.SiteID, params.Start, params.End)
	args = append(args, hitFilterArgs...)
	args = append(args, params.SiteID, params.Start, params.End)
	args = append(args, fetchFilterArgs...)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query ai activity series: %w", err)
	}
	defer rows.Close()

	type bucketTotals struct {
		tracked        int
		fetched        int
		referralVisits int
	}
	totals := map[time.Time]bucketTotals{}
	for rows.Next() {
		var (
			bucket         time.Time
			tracked        int64
			fetched        int64
			referralVisits int64
		)
		if err := rows.Scan(&bucket, &tracked, &fetched, &referralVisits); err != nil {
			return nil, fmt.Errorf("scan ai activity series row: %w", err)
		}
		totals[truncToUnit(bucket, truncUnit)] = bucketTotals{
			tracked:        int(tracked),
			fetched:        int(fetched),
			referralVisits: int(referralVisits),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read ai activity series rows: %w", err)
	}

	buckets := buildSeriesBuckets(params.Start, params.End, truncUnit)
	points := make([]api.AIActivitySeriesPoint, 0, len(buckets))
	for _, bucket := range buckets {
		total := totals[bucket]
		points = append(points, api.AIActivitySeriesPoint{
			Time:           bucket,
			AIRequests:     total.tracked + total.fetched,
			TrackedHits:    total.tracked,
			FetchCount:     total.fetched,
			ReferralVisits: total.referralVisits,
		})
	}
	return points, nil
}

// buildAIActivityFetchFilters translates the shared hit filter set onto
// ai_fetches. Only the dimensions the fetch table actually carries are mapped;
// an ai_source filter zeroes the fetch side outright because a fetch record has
// no referrer, and every other hit-only dimension is skipped so the fetch side
// stays unrestricted instead of silently emptying.
func buildAIActivityFetchFilters(filters []api.Filter, alias string) (string, []any) {
	if len(filters) == 0 {
		return "", nil
	}

	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}

	var sqlFilter strings.Builder
	var args []any
	for _, filter := range filters {
		if filter.Type == "" || filter.Value == "" {
			continue
		}
		switch filter.Type {
		case "ai_bot":
			fmt.Fprintf(&sqlFilter, " AND %sassistant_name = ?", prefix)
			args = append(args, filter.Value)
		case "ai_bot_category":
			fmt.Fprintf(&sqlFilter, " AND COALESCE(NULLIF(TRIM(%sassistant_category), ''), hk_ai_bot_category_from_name(%sassistant_name)) = ?", prefix, prefix)
			args = append(args, filter.Value)
		case "path":
			fmt.Fprintf(&sqlFilter, " AND %spath = ?", prefix)
			args = append(args, filter.Value)
		case "ai_source":
			sqlFilter.WriteString(" AND 1=0")
		}
	}

	return sqlFilter.String(), args
}

// nonAIHitFilters drops the AI-specific dimensions so the pageviews denominator
// stays the total hit volume of the segment being inspected.
func nonAIHitFilters(filters []api.Filter) []api.Filter {
	if len(filters) == 0 {
		return nil
	}
	kept := make([]api.Filter, 0, len(filters))
	for _, filter := range filters {
		switch filter.Type {
		case "ai_bot", "ai_bot_category", "ai_source":
			continue
		default:
			kept = append(kept, filter)
		}
	}
	return kept
}
