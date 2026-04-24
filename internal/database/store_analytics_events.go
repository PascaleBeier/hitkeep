package database

import (
	"context"
	"fmt"
	"time"

	"hitkeep/internal/api"
)

func (s *Store) GetEventNames(ctx context.Context, params api.EventNamesParams) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT name FROM events
		WHERE site_id = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY name
	`, params.SiteID, params.Start, params.End)
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

// GetEventPropertyKeys returns the distinct JSON property keys for a given event name.
func (s *Store) GetEventPropertyKeys(ctx context.Context, params api.EventNamesParams, eventName string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT unnest(json_keys(CAST(properties AS JSON))) AS key
		FROM events
		WHERE site_id = ? AND timestamp >= ? AND timestamp <= ? AND name = ? AND properties IS NOT NULL
		ORDER BY key
	`, params.SiteID, params.Start, params.End, eventName)
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
	return results, nil
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

	// Build an optional WHERE filter on the pre-processed dimension value.
	// DimensionKey is validated against a fixed set so it is safe to interpolate.
	dimClause := ""
	var dimArgs []any
	if params.DimensionKey != "" && params.DimensionValue != "" {
		switch params.DimensionKey {
		case "path":
			dimClause = " AND h.path = ?"
			dimArgs = []any{params.DimensionValue}
		case "referrer":
			dimClause = " AND hk_referrer(h.referrer) = ?"
			dimArgs = []any{params.DimensionValue}
		case "device":
			dimClause = " AND hk_device(h.viewport_width) = ?"
			dimArgs = []any{params.DimensionValue}
		case "country":
			dimClause = " AND hk_country(h.country_code) = ?"
			dimArgs = []any{params.DimensionValue}
		}
	}

	//nolint:gosec // propClause and dimClause are fixed literal SQL fragments with no user content interpolated
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
	`, propClause, dimClause)

	hitsArgs := append([]any{params.SiteID, params.Start, params.End}, dimArgs...)
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

	return result, nil
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

	// DimensionKey is validated against a fixed set so it is safe to interpolate.
	dimClause := ""
	if params.DimensionKey != "" && params.DimensionValue != "" {
		var dimCol string
		switch params.DimensionKey {
		case "path":
			dimCol = "path = ?"
		case "referrer":
			dimCol = "hk_referrer(referrer) = ?"
		case "device":
			dimCol = "hk_device(viewport_width) = ?"
		case "country":
			dimCol = "hk_country(country_code) = ?"
		}
		if dimCol != "" {
			//nolint:gosec // dimCol is selected from a fixed allowlist above
			dimClause = fmt.Sprintf(
				" AND session_id IN (SELECT DISTINCT session_id FROM hits WHERE site_id = ? AND timestamp >= ? AND timestamp <= ? AND %s)",
				dimCol,
			)
			args = append(args, params.SiteID, params.Start, params.End, params.DimensionValue)
		}
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
