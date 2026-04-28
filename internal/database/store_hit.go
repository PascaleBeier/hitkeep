package database

import (
	"context"
	"database/sql"
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
	"utm_source",
	"utm_medium",
	"utm_campaign",
	"utm_term",
	"utm_content",
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

	if err := s.withAppenderColumns(ctx, "hits", hitAppenderColumns, func(appender rowAppender) error {
		for _, hit := range hits {
			if hit == nil {
				continue
			}

			if err := appender.AppendRow(
				duckdb.UUID(hit.ID),
				duckdb.UUID(hit.SiteID),
				duckdb.UUID(hit.SessionID),
				duckdb.UUID(hit.PageID),
				hit.Timestamp,
				hit.Path,
				nullableStringPtr(hit.Hostname),
				nullableStringPtr(hit.Referrer),
				nullableStringPtr(hit.UserAgent),
				nullableIntPtr(hit.ViewportWidth),
				nullableIntPtr(hit.ViewportHeight),
				nullableIntPtr(hit.ScreenWidth),
				nullableIntPtr(hit.ScreenHeight),
				nullableStringPtr(hit.Language),
				nullableBoolPtr(hit.IsUnique),
				nullableStringPtr(hit.CountryCode),
				nullableStringPtr(hit.UTMSource),
				nullableStringPtr(hit.UTMMedium),
				nullableStringPtr(hit.UTMCampaign),
				nullableStringPtr(hit.UTMTerm),
				nullableStringPtr(hit.UTMContent),
			); err != nil {
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
		)`
		wildcard := "%" + params.Query + "%"
		args = append(args, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard)
	}

	var total int
	countQuery := "SELECT COUNT(*) " + baseQuery
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count hits: %w", err)
	}

	// Whitelist
	orderBy := "h.timestamp" // default
	switch params.SortField {
	case "path":
		orderBy = "h.path"
	case "referrer":
		orderBy = "h.referrer"
	case "timestamp":
		orderBy = "h.timestamp"
	}

	orderDir := "DESC"
	if strings.ToLower(params.SortOrder) == "asc" {
		orderDir = "ASC"
	}

	baseQuery += fmt.Sprintf(" ORDER BY %s %s", orderBy, orderDir)

	baseQuery += " LIMIT ? OFFSET ?"
	args = append(args, params.Limit, params.Offset)

	//nolint:gosec
	selectQuery := `
		SELECT
            h.id, h.site_id, h.session_id, h.page_id, h.timestamp, h.path, h.hostname, h.referrer, h.user_agent,
            h.viewport_width, h.viewport_height, h.screen_width, h.screen_height, h.language, h.country_code,
            h.utm_source, h.utm_medium, h.utm_campaign, h.utm_term, h.utm_content, h.is_unique
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
			&hit.ScreenHeight, &hit.Language, &hit.CountryCode, &hit.UTMSource, &hit.UTMMedium, &hit.UTMCampaign, &hit.UTMTerm, &hit.UTMContent, &hit.IsUnique,
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
		"utm_source",
		"utm_medium",
		"utm_campaign",
		"utm_term",
		"utm_content",
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
			utmSource, utmMedium, utmCampaign                    sql.NullString
			utmTerm, utmContent                                  sql.NullString
			viewportWidth, viewportHeight                        sql.NullInt32
			screenWidth, screenHeight                            sql.NullInt32
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
			&utmSource,
			&utmMedium,
			&utmCampaign,
			&utmTerm,
			&utmContent,
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
			nullString(utmSource),
			nullString(utmMedium),
			nullString(utmCampaign),
			nullString(utmTerm),
			nullString(utmContent),
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
		)`
		wildcard := "%" + params.Query + "%"
		args = append(args, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard, wildcard)
	}

	baseQuery += " ORDER BY h.timestamp DESC"

	//nolint:gosec // baseQuery is built from fixed allowlists and parameter placeholders
	selectQuery := `
		SELECT
            h.id, h.site_id, h.session_id, h.page_id, h.timestamp, h.path, h.hostname, h.referrer, h.user_agent,
            h.viewport_width, h.viewport_height, h.screen_width, h.screen_height, h.language, h.country_code,
            h.utm_source, h.utm_medium, h.utm_campaign, h.utm_term, h.utm_content, h.is_unique
	` + baseQuery

	return selectQuery, args
}
