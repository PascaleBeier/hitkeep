package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"

	"hitkeep/internal/api"
)

var hitAppenderColumns = []string{
	"id",
	"site_id",
	"session_id",
	"page_id",
	"timestamp",
	"path",
	"hostname",
	"referrer",
	"user_agent",
	"viewport_width",
	"viewport_height",
	"screen_width",
	"screen_height",
	"language",
	"is_unique",
	"country_code",
	"region",
	"city",
	"provider",
	"asn",
	"asn_org",
	"utm_source",
	"utm_medium",
	"utm_campaign",
	"utm_term",
	"utm_content",
	"qr_code_id",
}

type hitAppenderValueFunc func(*api.Hit) driver.Value

var hitAppenderValueFns = map[string]hitAppenderValueFunc{
	"id":              func(hit *api.Hit) driver.Value { return duckdb.UUID(hit.ID) },
	"site_id":         func(hit *api.Hit) driver.Value { return duckdb.UUID(hit.SiteID) },
	"session_id":      func(hit *api.Hit) driver.Value { return duckdb.UUID(hit.SessionID) },
	"page_id":         func(hit *api.Hit) driver.Value { return duckdb.UUID(hit.PageID) },
	"timestamp":       func(hit *api.Hit) driver.Value { return hit.Timestamp },
	"path":            func(hit *api.Hit) driver.Value { return hit.Path },
	"hostname":        func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.Hostname) },
	"referrer":        func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.Referrer) },
	"user_agent":      func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.UserAgent) },
	"viewport_width":  func(hit *api.Hit) driver.Value { return nullableIntPtr(hit.ViewportWidth) },
	"viewport_height": func(hit *api.Hit) driver.Value { return nullableIntPtr(hit.ViewportHeight) },
	"screen_width":    func(hit *api.Hit) driver.Value { return nullableIntPtr(hit.ScreenWidth) },
	"screen_height":   func(hit *api.Hit) driver.Value { return nullableIntPtr(hit.ScreenHeight) },
	"language":        func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.Language) },
	"is_unique":       func(hit *api.Hit) driver.Value { return nullableBoolPtr(hit.IsUnique) },
	"country_code":    func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.CountryCode) },
	"region":          func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.Region) },
	"city":            func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.City) },
	"provider":        func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.Provider) },
	"asn":             func(hit *api.Hit) driver.Value { return nullableIntPtr(hit.ASN) },
	"asn_org":         func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.ASNOrg) },
	"utm_source":      func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.UTMSource) },
	"utm_medium":      func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.UTMMedium) },
	"utm_campaign":    func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.UTMCampaign) },
	"utm_term":        func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.UTMTerm) },
	"utm_content":     func(hit *api.Hit) driver.Value { return nullableStringPtr(hit.UTMContent) },
	"qr_code_id":      func(hit *api.Hit) driver.Value { return nullableDuckDBUUIDPtr(hit.QRCodeID) },
}

// availableHitAppenderColumns reports the intersection of hitAppenderColumns
// and the live hits schema. The schema only changes through migrations at
// startup, so the first successful result is cached for the store's lifetime.
func (s *Store) availableHitAppenderColumns(ctx context.Context) ([]string, error) {
	s.hitColumnsMu.Lock()
	cached := s.hitColumns
	s.hitColumnsMu.Unlock()
	if cached != nil {
		return cached, nil
	}

	columns, err := s.queryAvailableHitAppenderColumns(ctx)
	if err != nil {
		return nil, err
	}

	s.hitColumnsMu.Lock()
	s.hitColumns = columns
	s.hitColumnsMu.Unlock()
	return columns, nil
}

func (s *Store) queryAvailableHitAppenderColumns(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_name = 'hits'
			AND table_schema NOT IN ('information_schema', 'pg_catalog')
	`)
	if err != nil {
		return nil, fmt.Errorf("list hit columns: %w", err)
	}
	defer rows.Close()

	existing := map[string]struct{}{}
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, fmt.Errorf("scan hit column: %w", err)
		}
		existing[column] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read hit columns: %w", err)
	}

	columns := make([]string, 0, len(hitAppenderColumns))
	for _, column := range hitAppenderColumns {
		if _, ok := existing[column]; ok {
			columns = append(columns, column)
		}
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("hits table has no compatible columns")
	}
	return columns, nil
}

func hitAppenderValue(hit *api.Hit, column string) (driver.Value, error) {
	valueFn, ok := hitAppenderValueFns[column]
	if !ok {
		return nil, fmt.Errorf("unsupported hit appender column %q", column)
	}
	return valueFn(hit), nil
}

func nullableDuckDBUUIDPtr(value *uuid.UUID) any {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	return duckdb.UUID(*value)
}

func (s *Store) CreateHit(ctx context.Context, hit *api.Hit) error {
	if hit == nil {
		return fmt.Errorf("hit is required")
	}
	return s.CreateHitsBulk(ctx, []*api.Hit{hit})
}

func (s *Store) CreateHitsBulk(ctx context.Context, hits []*api.Hit) error {
	return s.createHitsBulk(ctx, hits, false)
}

// CreateHitsBulkUnsafe is like CreateHitsBulk but skips dirty rollup bucket tracking.
// Use when BackfillRollups will follow shortly after (e.g. during seeding).
func (s *Store) CreateHitsBulkUnsafe(ctx context.Context, hits []*api.Hit) error {
	return s.createHitsBulk(ctx, hits, true)
}

func (s *Store) createHitsBulk(ctx context.Context, hits []*api.Hit, skipDirtyBuckets bool) error {
	if len(hits) == 0 {
		return nil
	}

	if !skipDirtyBuckets {
		dirtyBuckets := make([]dirtyRollupBucket, 0, len(hits)*6)
		for _, hit := range hits {
			if hit == nil {
				continue
			}
			if hit.ID == uuid.Nil {
				hit.ID = uuid.New()
			}
			if hit.Timestamp.IsZero() {
				hit.Timestamp = time.Now()
			}
			dirtyBuckets = append(dirtyBuckets, dirtyBucketsForHit(hit)...)
		}
		if err := s.markDirtyRollupBuckets(ctx, dirtyBuckets); err != nil {
			return fmt.Errorf("mark dirty rollups before hit insert: %w", err)
		}
	} else {
		for _, hit := range hits {
			if hit == nil {
				continue
			}
			if hit.ID == uuid.Nil {
				hit.ID = uuid.New()
			}
			if hit.Timestamp.IsZero() {
				hit.Timestamp = time.Now()
			}
		}
	}

	appenderColumns, err := s.availableHitAppenderColumns(ctx)
	if err != nil {
		return err
	}

	if err := s.withAppenderColumns(ctx, "hits", appenderColumns, func(appender rowAppender) error {
		for _, hit := range hits {
			if hit == nil {
				continue
			}

			values := make([]driver.Value, 0, len(appenderColumns))
			for _, column := range appenderColumns {
				value, err := hitAppenderValue(hit, column)
				if err != nil {
					return err
				}
				values = append(values, value)
			}

			if err := appender.AppendRow(values...); err != nil {
				return fmt.Errorf("append hit row: %w", err)
			}
		}

		return nil
	}); err != nil {
		return err
	}

	return nil
}

// GetHits returns paginated, sorted, and filtered hits.
func (s *Store) GetHits(ctx context.Context, params api.HitQueryParams) (*api.PaginatedHits, error) {
	// Authorization is handled by the handler middleware (SitePerm/RequirePermission).
	// This query runs against the tenant-specific analytics DB which has no sites table.
	baseQuery := `
		FROM hits h
		WHERE h.site_id = ?
		  AND h.timestamp >= ?
		  AND h.timestamp <= ?
	`
	args := []any{params.SiteID, params.Start, params.End}

	filterSQL, filterArgs := buildHitFilters(params.Filters, "h")
	baseQuery += filterSQL
	args = append(args, filterArgs...)

	if params.Query != "" {
		baseQuery += ` AND (
			h.path ILIKE ?
			OR h.hostname ILIKE ?
			OR h.referrer ILIKE ?
			OR h.user_agent ILIKE ?
			OR h.utm_source ILIKE ?
			OR h.utm_medium ILIKE ?
			OR h.utm_campaign ILIKE ?
			OR h.utm_term ILIKE ?
			OR h.utm_content ILIKE ?
			OR CAST(h.qr_code_id AS VARCHAR) ILIKE ?
		)`
		wildcard := "%" + params.Query + "%"
		args = append(args, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard)
	}

	var total int
	countQuery := "SELECT COUNT(*) " + baseQuery
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count hits: %w", err)
	}

	// Whitelist
	baseQuery += fmt.Sprintf(" ORDER BY %s %s", hitSortColumn(params.SortField), hitSortDirection(params.SortOrder))

	baseQuery += " LIMIT ? OFFSET ?"
	args = append(args, params.Limit, params.Offset)

	//nolint:gosec
	selectQuery := `
		SELECT
            h.id, h.site_id, h.session_id, h.page_id, h.timestamp, h.path, h.hostname, h.referrer, h.user_agent,
            h.viewport_width, h.viewport_height, h.screen_width, h.screen_height, h.language, h.country_code,
            h.region, h.city, h.provider, h.asn, h.asn_org,
            h.utm_source, h.utm_medium, h.utm_campaign, h.utm_term, h.utm_content, h.qr_code_id, h.is_unique
	` + baseQuery // whitelisted

	rows, err := s.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query hits: %w", err)
	}
	defer rows.Close()

	hits := []api.Hit{}
	for rows.Next() {
		var hit api.Hit
		if err := rows.Scan(
			&hit.ID, &hit.SiteID, &hit.SessionID, &hit.PageID, &hit.Timestamp, &hit.Path, &hit.Hostname, &hit.Referrer,
			&hit.UserAgent, &hit.ViewportWidth, &hit.ViewportHeight, &hit.ScreenWidth,
			&hit.ScreenHeight, &hit.Language, &hit.CountryCode, &hit.Region, &hit.City, &hit.Provider, &hit.ASN, &hit.ASNOrg, &hit.UTMSource, &hit.UTMMedium, &hit.UTMCampaign, &hit.UTMTerm, &hit.UTMContent, &hit.QRCodeID, &hit.IsUnique,
		); err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read hit rows: %w", err)
	}

	return &api.PaginatedHits{
		Data:  hits,
		Total: total,
	}, nil
}

func hitSortColumn(field string) string {
	switch field {
	case "path":
		return "h.path"
	case "referrer":
		return "h.referrer"
	default:
		return "h.timestamp"
	}
}

func hitSortDirection(order string) string {
	if strings.ToLower(order) == "asc" {
		return "ASC"
	}
	return "DESC"
}

func (s *Store) ExportHitsCSV(ctx context.Context, params api.HitQueryParams, w io.Writer) error {
	selectQuery, args := buildHitExportQuery(params)

	rows, err := s.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return fmt.Errorf("failed to query hits for export: %w", err)
	}
	defer rows.Close()

	writer := csv.NewWriter(w)
	if err := writer.Write([]string{
		"id",
		"site_id",
		"session_id",
		"page_id",
		"timestamp",
		"path",
		"hostname",
		"referrer",
		"user_agent",
		"viewport_width",
		"viewport_height",
		"screen_width",
		"screen_height",
		"language",
		"country_code",
		"region",
		"city",
		"provider",
		"asn",
		"asn_org",
		"utm_source",
		"utm_medium",
		"utm_campaign",
		"utm_term",
		"utm_content",
		"qr_code_id",
		"is_unique",
	}); err != nil {
		return fmt.Errorf("failed to write csv header: %w", err)
	}

	for rows.Next() {
		var (
			id, siteID, sessionID, pageID                        uuid.UUID
			timestamp                                            time.Time
			path                                                 string
			hostname, referrer, userAgent, language, countryCode sql.NullString
			region, city, provider, asnOrg                       sql.NullString
			utmSource, utmMedium, utmCampaign                    sql.NullString
			utmTerm, utmContent                                  sql.NullString
			qrCodeID                                             uuid.NullUUID
			viewportWidth, viewportHeight                        sql.NullInt32
			screenWidth, screenHeight                            sql.NullInt32
			asn                                                  sql.NullInt32
			isUnique                                             sql.NullBool
		)
		if err := rows.Scan(
			&id,
			&siteID,
			&sessionID,
			&pageID,
			&timestamp,
			&path,
			&hostname,
			&referrer,
			&userAgent,
			&viewportWidth,
			&viewportHeight,
			&screenWidth,
			&screenHeight,
			&language,
			&countryCode,
			&region,
			&city,
			&provider,
			&asn,
			&asnOrg,
			&utmSource,
			&utmMedium,
			&utmCampaign,
			&utmTerm,
			&utmContent,
			&qrCodeID,
			&isUnique,
		); err != nil {
			return fmt.Errorf("failed to scan export row: %w", err)
		}

		record := []string{
			id.String(),
			siteID.String(),
			sessionID.String(),
			pageID.String(),
			timestamp.Format(time.RFC3339),
			path,
			nullString(hostname),
			nullString(referrer),
			nullString(userAgent),
			nullInt32(viewportWidth),
			nullInt32(viewportHeight),
			nullInt32(screenWidth),
			nullInt32(screenHeight),
			nullString(language),
			nullString(countryCode),
			nullString(region),
			nullString(city),
			nullString(provider),
			nullInt32(asn),
			nullString(asnOrg),
			nullString(utmSource),
			nullString(utmMedium),
			nullString(utmCampaign),
			nullString(utmTerm),
			nullString(utmContent),
			nullUUID(qrCodeID),
			nullBool(isUnique),
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write csv record: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read hit export rows: %w", err)
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush csv: %w", err)
	}
	return nil
}

func (s *Store) ExportHitsFile(ctx context.Context, params api.HitQueryParams, format string) (string, error) {
	selectQuery, args := buildHitExportQuery(params)
	filename, err := s.exportQueryToTempFile(ctx, "hitkeep_hits_", "hitkeep_hits_", selectQuery, args, format)
	if err != nil {
		return "", fmt.Errorf("failed to export hits: %w", err)
	}

	return filename, nil
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func nullInt32(value sql.NullInt32) string {
	if value.Valid {
		return strconv.FormatInt(int64(value.Int32), 10)
	}
	return ""
}

func nullBool(value sql.NullBool) string {
	if value.Valid {
		if value.Bool {
			return "true"
		}
		return "false"
	}
	return ""
}

func nullUUID(value uuid.NullUUID) string {
	if value.Valid {
		return value.UUID.String()
	}
	return ""
}

func buildHitExportQuery(params api.HitQueryParams) (string, []any) {
	// Authorization is handled by the handler middleware (SitePerm/RequirePermission).
	// This query runs against the tenant-specific analytics DB which has no sites table.
	baseQuery := `
		FROM hits h
		WHERE h.site_id = ?
		  AND h.timestamp >= ?
		  AND h.timestamp <= ?
	`
	args := []any{params.SiteID, params.Start, params.End}

	filterSQL, filterArgs := buildHitFilters(params.Filters, "h")
	baseQuery += filterSQL
	args = append(args, filterArgs...)

	if params.Query != "" {
		baseQuery += ` AND (
			h.path ILIKE ?
			OR h.hostname ILIKE ?
			OR h.referrer ILIKE ?
			OR h.user_agent ILIKE ?
			OR h.utm_source ILIKE ?
			OR h.utm_medium ILIKE ?
			OR h.utm_campaign ILIKE ?
			OR h.utm_term ILIKE ?
			OR h.utm_content ILIKE ?
			OR CAST(h.qr_code_id AS VARCHAR) ILIKE ?
		)`
		wildcard := "%" + params.Query + "%"
		args = append(args, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard)
	}

	baseQuery += " ORDER BY h.timestamp DESC"

	//nolint:gosec // baseQuery is built from fixed allowlists and parameter placeholders
	selectQuery := `
		SELECT
            h.id, h.site_id, h.session_id, h.page_id, h.timestamp, h.path, h.hostname, h.referrer, h.user_agent,
            h.viewport_width, h.viewport_height, h.screen_width, h.screen_height, h.language, h.country_code,
            h.region, h.city, h.provider, h.asn, h.asn_org,
            h.utm_source, h.utm_medium, h.utm_campaign, h.utm_term, h.utm_content, h.qr_code_id, h.is_unique
	` + baseQuery

	return selectQuery, args
}
