package worker

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/database"
)

type S3Config = database.S3SecretConfig

type RetentionWorker struct {
	tenantMgr   *database.TenantStoreManager
	path        string
	defaultDays int
	s3Config    *S3Config
}

type retentionSitePolicy struct {
	ID       uuid.UUID
	TenantID uuid.UUID
	Days     int
}

type retentionCounts struct {
	Hits                    int64
	Events                  int64
	WebVitals               int64
	AIFetches               int64
	ImportedTraffic         int64
	ImportedDimensions      int64
	ImportedEvents          int64
	ImportedEventDimensions int64
	ImportedEventProperties int64
}

type retentionCountQuery struct {
	name   string
	query  string
	assign func(int64)
}

type retentionDeleteQuery struct {
	count int64
	label string
	query string
}

func NewRetentionWorker(tenantMgr *database.TenantStoreManager, archivePath string, defaultDays int, s3Config *S3Config) *RetentionWorker {
	path := strings.TrimSpace(archivePath)
	if path == "" {
		path = "archive"
	}
	return &RetentionWorker{
		tenantMgr:   tenantMgr,
		path:        path,
		defaultDays: defaultDays,
		s3Config:    s3Config,
	}
}

// archiveFilename returns the full destination path for a site's Parquet archive.
//
// For local paths, the default tenant keeps the legacy flat layout to preserve
// existing deployments. Non-default tenants are isolated under tenants/<id>/...
// For s3:// paths, all tenants are always isolated under tenants/<id>/...
func (w *RetentionWorker) archiveFilename(siteID, tenantID, defaultTenantID uuid.UUID) string {
	name := fmt.Sprintf("site_%s_%d.parquet", siteID, time.Now().Unix())

	if IsS3ArchivePath(w.path) {
		return joinArchivePath(w.path, "tenants", tenantID.String(), "sites", siteID.String(), name)
	}

	if tenantID == uuid.Nil || (defaultTenantID != uuid.Nil && tenantID == defaultTenantID) {
		return joinArchivePath(w.path, name)
	}

	return joinArchivePath(w.path, "tenants", tenantID.String(), "sites", siteID.String(), name)
}

func (w *RetentionWorker) Start(ctx context.Context) {
	// Run once on startup after a short delay to let DB settle
	go func() {
		time.Sleep(10 * time.Second)
		if err := w.Run(ctx); err != nil {
			slog.Error("Initial retention run failed", "error", err)
		}
	}()

	// Run daily
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.Run(ctx); err != nil {
				slog.Error("Retention worker failed", "error", err)
			}
		}
	}
}

func (w *RetentionWorker) Run(ctx context.Context) error {
	slog.Debug("Checking for data retention cleanup...")

	if err := w.prepareArchiveDestination(ctx); err != nil {
		return err
	}

	defaultTenantID := w.resolveDefaultTenantID(ctx)
	policies, err := w.loadRetentionPolicies(ctx, defaultTenantID)
	if err != nil {
		return err
	}

	for _, p := range policies {
		w.processSitePolicy(ctx, p, defaultTenantID)
	}

	return nil
}

func (w *RetentionWorker) prepareArchiveDestination(ctx context.Context) error {
	if IsS3ArchivePath(w.path) {
		if err := w.ensureS3Support(ctx); err != nil {
			return fmt.Errorf("failed to enable duckdb s3 support: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(w.path, 0755); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}
	return nil
}

func (w *RetentionWorker) resolveDefaultTenantID(ctx context.Context) uuid.UUID {
	defaultTenantID, err := w.tenantMgr.Shared().GetDefaultTenantID(ctx)
	if err == nil {
		return defaultTenantID
	}
	if isBinderError(err) {
		slog.Debug("Tenant schema not ready, using nil default tenant ID", "error", err)
	} else {
		slog.Warn("Failed to resolve default tenant for retention archive layout", "error", err)
	}
	return uuid.Nil
}

func (w *RetentionWorker) processSitePolicy(ctx context.Context, policy retentionSitePolicy, defaultTenantID uuid.UUID) {
	cutoff := time.Now().AddDate(0, 0, -policy.Days)

	tenantStore, err := w.tenantMgr.ForTenant(ctx, policy.TenantID)
	if err != nil {
		slog.Error("Failed to resolve tenant store for retention", "error", err, "site_id", policy.ID, "tenant_id", policy.TenantID)
		return
	}

	db := tenantStore.DB()
	counts, err := countRetainedRows(ctx, db, policy.ID, cutoff)
	if err != nil {
		slog.Error("Failed to count rows for retention", "error", err, "site_id", policy.ID)
		return
	}
	if !counts.hasColdData() {
		return
	}

	slog.Info("Archiving old data", counts.logAttrs(policy.ID, cutoff)...)

	filename := w.archiveFilename(policy.ID, policy.TenantID, defaultTenantID)
	if err := ensureArchiveParent(filename); err != nil {
		slog.Error("Failed to create archive destination", "error", err, "site_id", policy.ID, "tenant_id", policy.TenantID, "path", filename)
		return
	}

	if err := w.exportSiteArchive(ctx, db, policy.ID, cutoff, filename); err != nil {
		slog.Error("Failed to export data to parquet", "error", err, "site_id", policy.ID)
		return
	}

	if err := pruneRetainedRows(ctx, db, policy.ID, cutoff, counts); err != nil {
		slog.Error("Failed to prune retained rows", "error", err, "site_id", policy.ID)
		return
	}

	slog.Info("Retention process completed", "site_id", policy.ID, "tenant_id", policy.TenantID, "archive", filename)
}

func countRetainedRows(ctx context.Context, db *sql.DB, siteID uuid.UUID, cutoff time.Time) (retentionCounts, error) {
	var counts retentionCounts
	queries := []retentionCountQuery{
		{name: "hits", query: "SELECT COUNT(*) FROM hits WHERE site_id = ? AND timestamp < ?", assign: func(v int64) { counts.Hits = v }},
		{name: "events", query: "SELECT COUNT(*) FROM events WHERE site_id = ? AND timestamp < ?", assign: func(v int64) { counts.Events = v }},
		{name: "web vitals", query: "SELECT COUNT(*) FROM web_vitals WHERE site_id = ? AND timestamp < ?", assign: func(v int64) { counts.WebVitals = v }},
		{name: "ai fetches", query: "SELECT COUNT(*) FROM ai_fetches WHERE site_id = ? AND timestamp < ?", assign: func(v int64) { counts.AIFetches = v }},
		{name: "imported traffic", query: "SELECT COUNT(*) FROM imported_traffic_daily WHERE site_id = ? AND date < ?", assign: func(v int64) { counts.ImportedTraffic = v }},
		{name: "imported dimensions", query: "SELECT COUNT(*) FROM imported_dimension_daily WHERE site_id = ? AND date < ?", assign: func(v int64) { counts.ImportedDimensions = v }},
		{name: "imported events", query: "SELECT COUNT(*) FROM imported_event_daily WHERE site_id = ? AND date < ?", assign: func(v int64) { counts.ImportedEvents = v }},
		{name: "imported event dimensions", query: "SELECT COUNT(*) FROM imported_event_dimensions_daily WHERE site_id = ? AND date < ?", assign: func(v int64) { counts.ImportedEventDimensions = v }},
		{name: "imported event properties", query: "SELECT COUNT(*) FROM imported_event_properties_daily WHERE site_id = ? AND date < ?", assign: func(v int64) { counts.ImportedEventProperties = v }},
	}
	for _, q := range queries {
		var count int64
		if err := db.QueryRowContext(ctx, q.query, siteID, cutoff).Scan(&count); err != nil {
			return counts, fmt.Errorf("count %s: %w", q.name, err)
		}
		q.assign(count)
	}
	return counts, nil
}

func (c retentionCounts) hasColdData() bool {
	return c.Hits > 0 ||
		c.Events > 0 ||
		c.WebVitals > 0 ||
		c.AIFetches > 0 ||
		c.ImportedTraffic > 0 ||
		c.ImportedDimensions > 0 ||
		c.ImportedEvents > 0 ||
		c.ImportedEventDimensions > 0 ||
		c.ImportedEventProperties > 0
}

func (c retentionCounts) logAttrs(siteID uuid.UUID, cutoff time.Time) []any {
	return []any{
		"site_id", siteID,
		"hits", c.Hits,
		"events", c.Events,
		"web_vitals", c.WebVitals,
		"ai_fetches", c.AIFetches,
		"imported_traffic", c.ImportedTraffic,
		"imported_dimensions", c.ImportedDimensions,
		"imported_events", c.ImportedEvents,
		"imported_event_dimensions", c.ImportedEventDimensions,
		"imported_event_properties", c.ImportedEventProperties,
		"cutoff", cutoff.Format(time.DateOnly),
	}
}

func ensureArchiveParent(filename string) error {
	if IsS3ArchivePath(filename) {
		return nil
	}
	return os.MkdirAll(filepath.Dir(filename), 0755)
}

func (w *RetentionWorker) exportSiteArchive(ctx context.Context, db *sql.DB, siteID uuid.UUID, cutoff time.Time, filename string) error {
	query := buildRetentionExportQuery(siteID, cutoff, filename)
	return database.WithDuckDBSession(ctx, db, database.DuckDBSessionOptions{
		S3: s3ConfigForSession(IsS3ArchivePath(w.path), w.s3Config),
	}, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(ctx, query)
		return err
	})
}

func buildRetentionExportQuery(siteID uuid.UUID, cutoff time.Time, filename string) string {
	timestampCutoff := cutoff.Format(time.RFC3339)
	dateCutoff := cutoff.Format(time.DateOnly)
	safeFilename := strings.ReplaceAll(filename, "'", "''")

	//nolint:gosec // DuckDB COPY doesn't support parameters; values are internally generated and escaped above.
	return fmt.Sprintf(`
				COPY (
					SELECT 'hits' AS _source, * FROM hits WHERE site_id = '%s' AND timestamp < '%s'
					UNION BY NAME
					SELECT 'events' AS _source, * FROM events WHERE site_id = '%s' AND timestamp < '%s'
					UNION BY NAME
					SELECT 'web_vitals' AS _source, * FROM web_vitals WHERE site_id = '%s' AND timestamp < '%s'
					UNION BY NAME
					SELECT 'ai_fetches' AS _source, * FROM ai_fetches WHERE site_id = '%s' AND timestamp < '%s'
					UNION BY NAME
					SELECT 'imported_traffic_daily' AS _source, * FROM imported_traffic_daily WHERE site_id = '%s' AND date < '%s'
					UNION BY NAME
					SELECT 'imported_dimension_daily' AS _source, * FROM imported_dimension_daily WHERE site_id = '%s' AND date < '%s'
					UNION BY NAME
					SELECT 'imported_event_daily' AS _source, * FROM imported_event_daily WHERE site_id = '%s' AND date < '%s'
					UNION BY NAME
					SELECT 'imported_event_dimensions_daily' AS _source, * FROM imported_event_dimensions_daily WHERE site_id = '%s' AND date < '%s'
					UNION BY NAME
					SELECT 'imported_event_properties_daily' AS _source, * FROM imported_event_properties_daily WHERE site_id = '%s' AND date < '%s'
				) TO '%s' (FORMAT PARQUET, COMPRESSION 'SNAPPY');
			`, siteID, timestampCutoff, siteID, timestampCutoff, siteID, timestampCutoff, siteID, timestampCutoff, siteID, dateCutoff, siteID, dateCutoff, siteID, dateCutoff, siteID, dateCutoff, siteID, dateCutoff, safeFilename)
}

func pruneRetainedRows(ctx context.Context, db *sql.DB, siteID uuid.UUID, cutoff time.Time, counts retentionCounts) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start deletion transaction: %w", err)
	}
	if err := execRetentionDeletes(ctx, tx, siteID, cutoff, retentionDeleteQueries(counts)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit deletion: %w", err)
	}
	return nil
}

func execRetentionDeletes(ctx context.Context, tx *sql.Tx, siteID uuid.UUID, cutoff time.Time, queries []retentionDeleteQuery) error {
	for _, q := range queries {
		if q.count == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, q.query, siteID, cutoff); err != nil {
			return fmt.Errorf("prune %s: %w", q.label, err)
		}
	}
	return nil
}

func retentionDeleteQueries(counts retentionCounts) []retentionDeleteQuery {
	return []retentionDeleteQuery{
		{count: counts.Hits, label: "hits", query: "DELETE FROM hits WHERE site_id = ? AND timestamp < ?"},
		{count: counts.Events, label: "events", query: "DELETE FROM events WHERE site_id = ? AND timestamp < ?"},
		{count: counts.WebVitals, label: "web vitals", query: "DELETE FROM web_vitals WHERE site_id = ? AND timestamp < ?"},
		{count: counts.AIFetches, label: "ai fetches", query: "DELETE FROM ai_fetches WHERE site_id = ? AND timestamp < ?"},
		{count: counts.ImportedTraffic, label: "imported traffic", query: "DELETE FROM imported_traffic_daily WHERE site_id = ? AND date < ?"},
		{count: counts.ImportedDimensions, label: "imported dimensions", query: "DELETE FROM imported_dimension_daily WHERE site_id = ? AND date < ?"},
		{count: counts.ImportedEvents, label: "imported events", query: "DELETE FROM imported_event_daily WHERE site_id = ? AND date < ?"},
		{count: counts.ImportedEventDimensions, label: "imported event dimensions", query: "DELETE FROM imported_event_dimensions_daily WHERE site_id = ? AND date < ?"},
		{count: counts.ImportedEventProperties, label: "imported event properties", query: "DELETE FROM imported_event_properties_daily WHERE site_id = ? AND date < ?"},
		{count: 1, label: "hourly rollups", query: "DELETE FROM hit_rollups_hourly WHERE site_id = ? AND bucket < ?"},
		{count: 1, label: "daily rollups", query: "DELETE FROM hit_rollups_daily WHERE site_id = ? AND bucket < ?"},
		{count: 1, label: "monthly rollups", query: "DELETE FROM hit_rollups_monthly WHERE site_id = ? AND bucket < ?"},
		{count: 1, label: "hourly goal rollups", query: "DELETE FROM goal_rollups_hourly WHERE site_id = ? AND bucket < ?"},
		{count: 1, label: "daily goal rollups", query: "DELETE FROM goal_rollups_daily WHERE site_id = ? AND bucket < ?"},
		{count: 1, label: "monthly goal rollups", query: "DELETE FROM goal_rollups_monthly WHERE site_id = ? AND bucket < ?"},
		{count: 1, label: "hourly funnel rollups", query: "DELETE FROM funnel_rollups_hourly WHERE site_id = ? AND bucket < ?"},
		{count: 1, label: "daily funnel rollups", query: "DELETE FROM funnel_rollups_daily WHERE site_id = ? AND bucket < ?"},
		{count: 1, label: "monthly funnel rollups", query: "DELETE FROM funnel_rollups_monthly WHERE site_id = ? AND bucket < ?"},
		{count: 1, label: "hourly session rollups", query: "DELETE FROM session_rollups_hourly WHERE site_id = ? AND bucket < ?"},
		{count: 1, label: "daily session rollups", query: "DELETE FROM session_rollups_daily WHERE site_id = ? AND bucket < ?"},
		{count: 1, label: "monthly session rollups", query: "DELETE FROM session_rollups_monthly WHERE site_id = ? AND bucket < ?"},
	}
}

func (w *RetentionWorker) ensureS3Support(ctx context.Context) error {
	return database.WithDuckDBSession(ctx, w.tenantMgr.Shared().DB(), database.DuckDBSessionOptions{
		S3: s3ConfigForSession(true, w.s3Config),
	}, func(conn *sql.Conn) error {
		return nil
	})
}

func s3ConfigForSession(enabled bool, cfg *S3Config) *database.S3SecretConfig {
	if !enabled {
		return nil
	}
	return cfg
}

func (w *RetentionWorker) loadRetentionPolicies(ctx context.Context, defaultTenantID uuid.UUID) ([]retentionSitePolicy, error) {
	const tenantAwareQuery = `
		SELECT s.id, s.data_retention_days, CAST(st.tenant_id AS VARCHAR)
		FROM sites s
		LEFT JOIN site_tenants st ON st.site_id = s.id
		WHERE s.data_retention_days IS NOT NULL AND s.data_retention_days > 0
	`

	rows, err := w.tenantMgr.Shared().DB().QueryContext(ctx, tenantAwareQuery)
	if err != nil {
		if !isMissingRelationError(err, "site_tenants") {
			return nil, fmt.Errorf("failed to query retention policies: %w", err)
		}

		slog.Warn("Tenant mapping table not available; falling back to legacy retention layout", "error", err)
		return w.loadLegacyRetentionPolicies(ctx, defaultTenantID)
	}
	defer rows.Close()

	policies := make([]retentionSitePolicy, 0)
	for rows.Next() {
		var (
			policy      retentionSitePolicy
			tenantIDRaw sql.NullString
		)
		if err := rows.Scan(&policy.ID, &policy.Days, &tenantIDRaw); err != nil {
			slog.Error("Failed to scan tenant-aware site policy", "error", err)
			continue
		}

		policy.TenantID = defaultTenantID
		if tenantIDRaw.Valid && strings.TrimSpace(tenantIDRaw.String) != "" {
			tenantID, parseErr := uuid.Parse(strings.TrimSpace(tenantIDRaw.String))
			if parseErr != nil {
				slog.Error("Invalid tenant ID in site_tenants mapping", "error", parseErr, "site_id", policy.ID, "raw_tenant_id", tenantIDRaw.String)
				continue
			}
			policy.TenantID = tenantID
		}

		policies = append(policies, policy)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read retention policies: %w", err)
	}

	return policies, nil
}

func (w *RetentionWorker) loadLegacyRetentionPolicies(ctx context.Context, defaultTenantID uuid.UUID) ([]retentionSitePolicy, error) {
	rows, err := w.tenantMgr.Shared().DB().QueryContext(ctx, "SELECT id, data_retention_days FROM sites WHERE data_retention_days IS NOT NULL AND data_retention_days > 0")
	if err != nil {
		return nil, fmt.Errorf("failed to query legacy retention policies: %w", err)
	}
	defer rows.Close()

	policies := make([]retentionSitePolicy, 0)
	for rows.Next() {
		var policy retentionSitePolicy
		if err := rows.Scan(&policy.ID, &policy.Days); err != nil {
			slog.Error("Failed to scan legacy site policy", "error", err)
			continue
		}
		policy.TenantID = defaultTenantID
		policies = append(policies, policy)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read legacy retention policies: %w", err)
	}
	return policies, nil
}

func IsS3ArchivePath(path string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(path)), "s3://")
}

func joinArchivePath(base string, elems ...string) string {
	if IsS3ArchivePath(base) {
		normalized := strings.TrimRight(base, "/")
		parts := make([]string, 0, len(elems))
		for _, elem := range elems {
			clean := strings.Trim(elem, "/")
			if clean == "" {
				continue
			}
			parts = append(parts, clean)
		}
		if len(parts) == 0 {
			return normalized
		}
		return normalized + "/" + strings.Join(parts, "/")
	}

	all := make([]string, 0, len(elems)+1)
	all = append(all, base)
	for _, elem := range elems {
		clean := strings.Trim(elem, "/")
		if clean == "" {
			continue
		}
		all = append(all, clean)
	}
	return filepath.Join(all...)
}

func isMissingRelationError(err error, relation string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	rel := strings.ToLower(strings.TrimSpace(relation))
	return strings.Contains(msg, "does not exist") && strings.Contains(msg, rel)
}

// isBinderError returns true when DuckDB reports a Binder Error, typically
// because a referenced column or table doesn't exist in the current schema.
// This happens when migrations haven't been applied yet.
func isBinderError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "binder error")
}
