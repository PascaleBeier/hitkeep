package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"hitkeep/internal/api"
)

var importedEventAudienceDimensions = []string{"path", "referrer", "device", "country"}

var importedEventAudienceDimensionLabels = map[string]string{
	"path":     "page",
	"referrer": "referrer",
	"device":   "device",
	"country":  "country",
}

func (s *Store) GetEventNames(ctx context.Context, params api.EventNamesParams) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name FROM (
			SELECT DISTINCT name
			FROM events
			WHERE site_id = ? AND timestamp >= ? AND timestamp <= ?
			UNION
			SELECT DISTINCT event_name AS name
			FROM imported_event_daily
			WHERE site_id = ? AND date >= CAST(? AS DATE) AND date <= CAST(? AS DATE)
		)
		ORDER BY name
	`, params.SiteID, params.Start, params.End, params.SiteID, params.Start, params.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read event name rows: %w", err)
	}

	if names == nil {
		names = []string{}
	}
	return names, nil
}

func (s *Store) GetEventCounts(ctx context.Context, params api.EventNamesParams) ([]api.MetricStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name, SUM(count) AS count
		FROM (
			SELECT name, COUNT(*) AS count
			FROM events
			WHERE site_id = ? AND timestamp >= ? AND timestamp <= ?
			GROUP BY name
			UNION ALL
			SELECT event_name AS name, SUM(events) AS count
			FROM imported_event_daily
			WHERE site_id = ? AND date >= CAST(? AS DATE) AND date <= CAST(? AS DATE)
			GROUP BY event_name
		)
		WHERE name IS NOT NULL AND name <> ''
		GROUP BY name
		ORDER BY count DESC, name
	`, params.SiteID, params.Start, params.End, params.SiteID, params.Start, params.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []api.MetricStat{}
	for rows.Next() {
		var item api.MetricStat
		if err := rows.Scan(&item.Name, &item.Value); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read event count rows: %w", err)
	}
	return results, nil
}

// GetEventPropertyKeys returns the distinct JSON property keys for a given event name.
func (s *Store) GetEventPropertyKeys(ctx context.Context, params api.EventNamesParams, eventName string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT key FROM (
			SELECT DISTINCT unnest(json_keys(CAST(properties AS JSON))) AS key
			FROM events
			WHERE site_id = ? AND timestamp >= ? AND timestamp <= ? AND name = ? AND properties IS NOT NULL
			UNION
			SELECT DISTINCT property_key AS key
			FROM imported_event_properties_daily
			WHERE site_id = ? AND date >= CAST(? AS DATE) AND date <= CAST(? AS DATE) AND event_name = ?
		)
		ORDER BY key
	`, params.SiteID, params.Start, params.End, eventName, params.SiteID, params.Start, params.End, eventName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read event property key rows: %w", err)
	}

	if keys == nil {
		keys = []string{}
	}
	return keys, nil
}

// GetEventPropertyBreakdown returns a count breakdown of property values for a specific
// event name and property key, ordered by count descending.
func (s *Store) GetEventPropertyBreakdown(ctx context.Context, params api.EventBreakdownParams) ([]api.MetricStat, error) {
	jsonPath := "$." + params.PropertyKey
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			json_extract_string(properties, ?) AS prop_value,
			COUNT(DISTINCT session_id) AS cnt
		FROM events
		WHERE site_id = ? AND timestamp >= ? AND timestamp <= ? AND name = ?
			AND json_extract_string(properties, ?) IS NOT NULL
		GROUP BY prop_value
		ORDER BY cnt DESC
		LIMIT 20
	`, jsonPath, params.SiteID, params.Start, params.End, params.EventName, jsonPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []api.MetricStat
	for rows.Next() {
		var m api.MetricStat
		if err := rows.Scan(&m.Name, &m.Value); err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read event property breakdown rows: %w", err)
	}

	if results == nil {
		results = []api.MetricStat{}
	}
	imported, err := s.getImportedEventPropertyBreakdown(ctx, params)
	if err != nil {
		return nil, err
	}
	results = mergeMetricList(results, imported, 20)
	return results, nil
}

func (s *Store) getImportedEventPropertyBreakdown(ctx context.Context, params api.EventBreakdownParams) ([]api.MetricStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT property_value, SUM(events) AS cnt
		FROM imported_event_properties_daily
		WHERE site_id = ? AND date >= CAST(? AS DATE) AND date <= CAST(? AS DATE)
			AND event_name = ? AND property_key = ?
		GROUP BY property_value
		ORDER BY cnt DESC
		LIMIT 20
	`, params.SiteID, params.Start, params.End, params.EventName, params.PropertyKey)
	if err != nil {
		return nil, fmt.Errorf("query imported event property breakdown: %w", err)
	}
	defer rows.Close()

	results := []api.MetricStat{}
	for rows.Next() {
		var m api.MetricStat
		if err := rows.Scan(&m.Name, &m.Value); err != nil {
			return nil, fmt.Errorf("scan imported event property breakdown: %w", err)
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read imported event property breakdown: %w", err)
	}
	return results, nil
}

func eventDimensionFilters(filters []api.Filter, legacyKey, legacyValue string) []api.Filter {
	if legacyKey == "" || legacyValue == "" {
		return filters
	}
	next := make([]api.Filter, 0, len(filters)+1)
	next = append(next, filters...)
	next = append(next, api.Filter{Type: legacyKey, Value: legacyValue})
	return next
}

// GetEventAudience returns the top pages, referrers, devices, and countries for sessions
// that contain a specific event name (with an optional property filter).
func (s *Store) GetEventAudience(ctx context.Context, params api.EventAudienceParams) (*api.EventAudience, error) {
	result := &api.EventAudience{
		TopPages:     []api.MetricStat{},
		TopReferrers: []api.MetricStat{},
		TopDevices:   []api.MetricStat{},
		TopCountries: []api.MetricStat{},
	}

	eventArgs := []any{params.SiteID, params.Start, params.End, params.EventName}
	propClause := ""
	if params.PropertyKey != "" && params.PropertyValue != "" {
		jsonPath := "$." + params.PropertyKey
		propClause = " AND json_extract_string(properties, ?) = ?"
		eventArgs = append(eventArgs, jsonPath, params.PropertyValue)
	}

	filterSQL, filterArgs := buildHitFilters(eventDimensionFilters(params.Filters, params.DimensionKey, params.DimensionValue), "h")

	//nolint:gosec // propClause and filterSQL are fixed literal SQL fragments with no user content interpolated
	query := fmt.Sprintf(`
		WITH event_sessions AS (
			SELECT DISTINCT session_id
			FROM events
			WHERE site_id = ? AND timestamp >= ? AND timestamp <= ? AND name = ?%s
		),
		base AS (
			SELECT
				h.path                      AS path,
				hk_referrer(h.referrer)     AS referrer,
				hk_device(h.viewport_width) AS device,
				hk_country(h.country_code)  AS country,
				h.session_id                AS session_id
			FROM hits h
			INNER JOIN event_sessions es ON h.session_id = es.session_id
			WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s
		),
		agg AS (
			SELECT
				CASE
					WHEN GROUPING(path)     = 0 THEN 'path'
					WHEN GROUPING(referrer) = 0 THEN 'referrer'
					WHEN GROUPING(device)   = 0 THEN 'device'
					WHEN GROUPING(country)  = 0 THEN 'country'
				END AS dim,
				COALESCE(path, referrer, device, country) AS name,
				COUNT(DISTINCT session_id) AS val
			FROM base
			GROUP BY GROUPING SETS ((path),(referrer),(device),(country))
		),
		ranked AS (
			SELECT dim, name, val,
				ROW_NUMBER() OVER (PARTITION BY dim ORDER BY val DESC) AS rn
			FROM agg
		)
		SELECT dim, name, val FROM ranked WHERE rn <= 10 ORDER BY dim, val DESC
	`, propClause, filterSQL)

	hitsArgs := append([]any{params.SiteID, params.Start, params.End}, filterArgs...)
	args := append(eventArgs, hitsArgs...)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var dim string
		var m api.MetricStat
		if err := rows.Scan(&dim, &m.Name, &m.Value); err != nil {
			return nil, err
		}
		switch dim {
		case "path":
			result.TopPages = append(result.TopPages, m)
		case "referrer":
			result.TopReferrers = append(result.TopReferrers, m)
		case "device":
			result.TopDevices = append(result.TopDevices, m)
		case "country":
			result.TopCountries = append(result.TopCountries, m)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(params.Filters) == 0 && params.DimensionKey == "" && params.DimensionValue == "" && params.PropertyKey == "" && params.PropertyValue == "" {
		importedDimensions, err := s.getImportedEventAudienceDimensions(ctx, params)
		if err != nil {
			return nil, err
		}
		result.TopPages = mergeMetricList(result.TopPages, importedDimensions["path"], 10)
		result.TopReferrers = mergeMetricList(result.TopReferrers, importedDimensions["referrer"], 10)
		result.TopDevices = mergeMetricList(result.TopDevices, importedDimensions["device"], 10)
		result.TopCountries = mergeMetricList(result.TopCountries, importedDimensions["country"], 10)
		if err := s.addImportedEventDimensionLimitation(ctx, params, result); err != nil {
			return nil, err
		}
	} else if err := s.addImportedEventAudienceExclusion(ctx, params, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *Store) addImportedEventAudienceExclusion(ctx context.Context, params api.EventAudienceParams, result *api.EventAudience) error {
	exists, err := s.hasImportedEventAudience(ctx, params)
	if err != nil {
		return err
	}
	if exists {
		result.ImportedExcluded = append(result.ImportedExcluded, api.ImportExclusionReason{
			Reason: "aggregate_filter",
			Detail: "The active event audience filters require raw session relationships that aggregate imports cannot prove.",
		})
	}
	return nil
}

func (s *Store) addImportedEventDimensionLimitation(ctx context.Context, params api.EventAudienceParams, result *api.EventAudience) error {
	exists, err := s.hasImportedEventAudience(ctx, params)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	available, err := s.importedEventAudienceAvailableDimensions(ctx, params)
	if err != nil {
		return err
	}
	missing := make([]string, 0, len(importedEventAudienceDimensions))
	for _, dimension := range importedEventAudienceDimensions {
		if _, ok := available[dimension]; ok {
			continue
		}
		missing = append(missing, importedEventAudienceDimensionLabels[dimension])
	}
	if len(missing) == 0 {
		return nil
	}
	result.ImportedExcluded = append(result.ImportedExcluded, api.ImportExclusionReason{
		Reason: "missing_event_dimension_relationships",
		Detail: "Imported aggregate event data does not include event-level " + strings.Join(missing, ", ") + " relationships for this report.",
	})
	return nil
}

func (s *Store) hasImportedEventAudience(ctx context.Context, params api.EventAudienceParams) (bool, error) {
	var exists bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM imported_event_daily
			WHERE site_id = ? AND date >= CAST(? AS DATE) AND date <= CAST(? AS DATE) AND event_name = ?
			LIMIT 1
		)
	`, params.SiteID, params.Start, params.End, params.EventName).Scan(&exists); err != nil {
		return false, fmt.Errorf("query imported event audience coverage: %w", err)
	}
	return exists, nil
}

func (s *Store) getImportedEventAudienceDimensions(ctx context.Context, params api.EventAudienceParams) (map[string][]api.MetricStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH imported_dims AS (
			SELECT dimension, name, SUM(visitors) AS val
			FROM imported_event_dimensions_daily
			WHERE site_id = ? AND date >= CAST(? AS DATE) AND date <= CAST(? AS DATE)
				AND event_name = ? AND dimension IN ('path', 'referrer', 'device', 'country')
			GROUP BY dimension, name
			UNION ALL
			SELECT 'path' AS dimension, path AS name, SUM(visitors) AS val
			FROM imported_event_daily e
			WHERE site_id = ? AND date >= CAST(? AS DATE) AND date <= CAST(? AS DATE)
				AND event_name = ? AND path IS NOT NULL AND path <> ''
				AND NOT EXISTS (
					SELECT 1
					FROM imported_event_dimensions_daily d
					WHERE d.site_id = e.site_id
						AND d.import_id = e.import_id
						AND d.date = e.date
						AND d.event_name = e.event_name
						AND d.dimension = 'path'
				)
			GROUP BY path
		),
		aggregated AS (
			SELECT dimension, name, SUM(val) AS val
			FROM imported_dims
			GROUP BY dimension, name
		),
		ranked AS (
			SELECT dimension, name, val,
				ROW_NUMBER() OVER (PARTITION BY dimension ORDER BY val DESC, name ASC) AS rn
			FROM aggregated
		)
		SELECT dimension, name, val
		FROM ranked
		WHERE rn <= 10
		ORDER BY dimension, val DESC, name ASC
	`, params.SiteID, params.Start, params.End, params.EventName, params.SiteID, params.Start, params.End, params.EventName)
	if err != nil {
		return nil, fmt.Errorf("query imported event audience dimensions: %w", err)
	}
	defer rows.Close()

	results := map[string][]api.MetricStat{}
	for rows.Next() {
		var (
			dimension string
			m         api.MetricStat
		)
		if err := rows.Scan(&dimension, &m.Name, &m.Value); err != nil {
			return nil, fmt.Errorf("scan imported event audience dimensions: %w", err)
		}
		results[dimension] = append(results[dimension], m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read imported event audience dimensions: %w", err)
	}
	return results, nil
}

func (s *Store) importedEventAudienceAvailableDimensions(ctx context.Context, params api.EventAudienceParams) (map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT dimension
		FROM imported_event_dimensions_daily
		WHERE site_id = ? AND date >= CAST(? AS DATE) AND date <= CAST(? AS DATE)
			AND event_name = ? AND dimension IN ('path', 'referrer', 'device', 'country')
		UNION
		SELECT DISTINCT 'path' AS dimension
		FROM imported_event_daily
		WHERE site_id = ? AND date >= CAST(? AS DATE) AND date <= CAST(? AS DATE)
			AND event_name = ? AND path IS NOT NULL AND path <> ''
	`, params.SiteID, params.Start, params.End, params.EventName, params.SiteID, params.Start, params.End, params.EventName)
	if err != nil {
		return nil, fmt.Errorf("query imported event audience dimension coverage: %w", err)
	}
	defer rows.Close()

	available := map[string]struct{}{}
	for rows.Next() {
		var dimension string
		if err := rows.Scan(&dimension); err != nil {
			return nil, fmt.Errorf("scan imported event audience dimension coverage: %w", err)
		}
		available[dimension] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read imported event audience dimension coverage: %w", err)
	}
	return available, nil
}

// GetEventTimeseries returns event occurrence counts per time bucket for a given event name.
// If both PropertyKey and PropertyValue are set, only events where that JSON property equals
// the given value are counted.
func (s *Store) GetEventTimeseries(ctx context.Context, params api.EventTimeseriesParams) ([]api.EventSeriesPoint, error) {
	duration := params.End.Sub(params.Start)
	truncUnit := "day"
	if duration < 48*time.Hour {
		truncUnit = "hour"
	} else if duration >= 180*24*time.Hour {
		truncUnit = "month"
	}

	args := []any{params.SiteID, params.Start, params.End, params.EventName}
	propClause := ""
	if params.PropertyKey != "" && params.PropertyValue != "" {
		jsonPath := "$." + params.PropertyKey
		propClause = " AND json_extract_string(properties, ?) = ?"
		args = append(args, jsonPath, params.PropertyValue)
	}

	dimClause := ""
	filterSQL, filterArgs := buildHitFilters(eventDimensionFilters(params.Filters, params.DimensionKey, params.DimensionValue), "")
	if filterSQL != "" {
		//nolint:gosec // filterSQL is built from fixed allowlisted filter fragments with parameterized values
		dimClause = fmt.Sprintf(" AND session_id IN (SELECT DISTINCT session_id FROM hits WHERE site_id = ? AND timestamp >= ? AND timestamp <= ?%s)", filterSQL)
		args = append(args, params.SiteID, params.Start, params.End)
		args = append(args, filterArgs...)
	}

	//nolint:gosec // truncUnit is from a fixed allowlist; propClause/dimClause are literal SQL fragments with no user content
	query := fmt.Sprintf(`
		SELECT date_trunc('%s', timestamp) AS bucket, COUNT(*) AS count
		FROM events
		WHERE site_id = ? AND timestamp >= ? AND timestamp <= ? AND name = ?%s%s
		GROUP BY bucket
		ORDER BY bucket
	`, truncUnit, propClause, dimClause)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[time.Time]int)
	for rows.Next() {
		var bucket time.Time
		var count int
		if err := rows.Scan(&bucket, &count); err != nil {
			return nil, err
		}
		counts[truncToUnit(bucket, truncUnit)] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if truncUnit != "hour" && len(params.Filters) == 0 && params.DimensionKey == "" && params.DimensionValue == "" {
		if err := s.mergeImportedEventTimeseries(ctx, params, truncUnit, counts); err != nil {
			return nil, err
		}
	}

	buckets := buildSeriesBuckets(params.Start, params.End, truncUnit)
	series := make([]api.EventSeriesPoint, 0, len(buckets))
	for _, bucket := range buckets {
		series = append(series, api.EventSeriesPoint{
			Time:  bucket,
			Count: counts[bucket],
		})
	}
	return series, nil
}

func (s *Store) mergeImportedEventTimeseries(ctx context.Context, params api.EventTimeseriesParams, truncUnit string, counts map[time.Time]int) error {
	var (
		rows *sql.Rows
		err  error
	)
	if params.PropertyKey != "" && params.PropertyValue != "" {
		//nolint:gosec // truncUnit is chosen from the event bucket allowlist before this helper is called.
		query := fmt.Sprintf(`
			SELECT date_trunc('%s', date)::TIMESTAMPTZ AS bucket, SUM(events)
			FROM imported_event_properties_daily
			WHERE site_id = ? AND date >= CAST(? AS DATE) AND date <= CAST(? AS DATE)
				AND event_name = ? AND property_key = ? AND property_value = ?
			GROUP BY bucket
		`, truncUnit)
		rows, err = s.db.QueryContext(ctx, query, params.SiteID, params.Start, params.End, params.EventName, params.PropertyKey, params.PropertyValue)
	} else {
		//nolint:gosec // truncUnit is chosen from the event bucket allowlist before this helper is called.
		query := fmt.Sprintf(`
			SELECT date_trunc('%s', date)::TIMESTAMPTZ AS bucket, SUM(events)
			FROM imported_event_daily
			WHERE site_id = ? AND date >= CAST(? AS DATE) AND date <= CAST(? AS DATE)
				AND event_name = ?
			GROUP BY bucket
		`, truncUnit)
		rows, err = s.db.QueryContext(ctx, query, params.SiteID, params.Start, params.End, params.EventName)
	}
	if err != nil {
		return fmt.Errorf("query imported event timeseries: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var bucket time.Time
		var count int
		if err := rows.Scan(&bucket, &count); err != nil {
			return fmt.Errorf("scan imported event timeseries: %w", err)
		}
		counts[truncToUnit(bucket, truncUnit)] += count
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read imported event timeseries: %w", err)
	}
	return nil
}
