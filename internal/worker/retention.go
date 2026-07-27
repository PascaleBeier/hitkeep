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

	"hitkeep/internal/assetstore"
	"hitkeep/internal/database"
)

type S3Config = database.S3SecretConfig

type RetentionWorker struct {
	tenantMgr   *database.TenantStoreManager
	path        string
	defaultDays int
	s3Config    *S3Config
	assets      *assetstore.Store
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
	QROpens                 int64
	ImportedTraffic         int64
	ImportedDimensions      int64
	ImportedEvents          int64
	ImportedEventDimensions int64
	ImportedEventProperties int64
	SearchConsoleFacts      int64
	DirtyRollupBuckets      int64
	Rollups                 int64
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

func NewRetentionWorker(tenantMgr *database.TenantStoreManager, archivePath string, defaultDays int, s3Config *S3Config, dataPath ...string) *RetentionWorker {
	path := strings.TrimSpace(archivePath)
	if path == "" {
		path = "archive"
	}
	assetDataPath := ""
	if len(dataPath) > 0 {
		assetDataPath = dataPath[0]
	}
	return &RetentionWorker{
		tenantMgr:   tenantMgr,
		path:        path,
		defaultDays: defaultDays,
		s3Config:    s3Config,
		assets:      assetstore.New(assetDataPath),
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
	defaultTenantID, err := w.tenantMgr.Control().GetDefaultTenantID(ctx)
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
	if err := w.pruneArchivedQRAssets(ctx, policy.ID, cutoff); err != nil {
		slog.Error("Failed to prune archived QR assets", "error", err, "site_id", policy.ID)
		return
	}
	if !counts.hasColdData() {
		if counts.DirtyRollupBuckets > 0 {
			if _, err := db.ExecContext(ctx, "DELETE FROM rollup_dirty_buckets WHERE site_id = ? AND bucket < ?", policy.ID, cutoff); err != nil {
				slog.Error("Failed to prune expired dirty rollup buckets", "error", err, "site_id", policy.ID)
			}
		}
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

func (w *RetentionWorker) pruneArchivedQRAssets(ctx context.Context, siteID uuid.UUID, cutoff time.Time) error {
	if w.tenantMgr == nil || w.tenantMgr.Control() == nil {
		return nil
	}
	assets, err := w.tenantMgr.Control().ListArchivedQRCodeAssetsForRetention(ctx, siteID, cutoff)
	if err != nil {
		if isMissingRelationError(err, "qr_code_assets") || isMissingRelationError(err, "qr_codes") || isBinderError(err) {
			return nil
		}
		return err
	}
	for _, asset := range assets {
		if asset.StorageKey != "" && w.assets != nil {
			if err := w.assets.Delete(asset.StorageKey); err != nil {
				slog.Warn("Failed to delete retained QR asset file", "error", err, "site_id", asset.SiteID, "qr_code_id", asset.QRCodeID, "storage_key", asset.StorageKey)
			}
		}
		if w.assets != nil {
			if err := w.assets.DeleteQRCodeAssetDir(asset.SiteID, asset.QRCodeID); err != nil {
				slog.Warn("Failed to delete retained QR asset directory", "error", err, "site_id", asset.SiteID, "qr_code_id", asset.QRCodeID)
			}
		}
		if _, err := w.tenantMgr.Control().DeleteQRCodeAsset(ctx, asset.SiteID, asset.QRCodeID); err != nil {
			return err
		}
	}
	return nil
}

func countRetainedRows(ctx context.Context, db *sql.DB, siteID uuid.UUID, cutoff time.Time) (retentionCounts, error) {
	var counts retentionCounts
	queries := []retentionCountQuery{
		{name: "hits", query: "SELECT COUNT(*) FROM hits WHERE site_id = ? AND timestamp < ?", assign: func(v int64) { counts.Hits = v }},
		{name: "events", query: "SELECT COUNT(*) FROM events WHERE site_id = ? AND timestamp < ?", assign: func(v int64) { counts.Events = v }},
		{name: "web vitals", query: "SELECT COUNT(*) FROM web_vitals WHERE site_id = ? AND timestamp < ?", assign: func(v int64) { counts.WebVitals = v }},
		{name: "ai fetches", query: "SELECT COUNT(*) FROM ai_fetches WHERE site_id = ? AND timestamp < ?", assign: func(v int64) { counts.AIFetches = v }},
		{name: "qr code opens", query: "SELECT COUNT(*) FROM qr_code_opens WHERE site_id = ? AND timestamp < ?", assign: func(v int64) { counts.QROpens = v }},
		{name: "imported traffic", query: "SELECT COUNT(*) FROM imported_traffic_daily WHERE site_id = ? AND date < ?", assign: func(v int64) { counts.ImportedTraffic = v }},
		{name: "imported dimensions", query: "SELECT COUNT(*) FROM imported_dimension_daily WHERE site_id = ? AND date < ?", assign: func(v int64) { counts.ImportedDimensions = v }},
		{name: "imported events", query: "SELECT COUNT(*) FROM imported_event_daily WHERE site_id = ? AND date < ?", assign: func(v int64) { counts.ImportedEvents = v }},
		{name: "imported event dimensions", query: "SELECT COUNT(*) FROM imported_event_dimensions_daily WHERE site_id = ? AND date < ?", assign: func(v int64) { counts.ImportedEventDimensions = v }},
		{name: "imported event properties", query: "SELECT COUNT(*) FROM imported_event_properties_daily WHERE site_id = ? AND date < ?", assign: func(v int64) { counts.ImportedEventProperties = v }},
		{name: "Search Console facts", query: "SELECT COUNT(*) FROM search_console_facts WHERE site_id = ? AND date < ?", assign: func(v int64) { counts.SearchConsoleFacts = v }},
		{name: "dirty rollup buckets", query: "SELECT COUNT(*) FROM rollup_dirty_buckets WHERE site_id = ? AND bucket < ?", assign: func(v int64) { counts.DirtyRollupBuckets = v }},
	}
	for _, q := range queries {
		var count int64
		if err := db.QueryRowContext(ctx, q.query, siteID, cutoff).Scan(&count); err != nil {
			return counts, fmt.Errorf("count %s: %w", q.name, err)
		}
		q.assign(count)
	}
	for _, table := range retentionRollupTables {
		var count int64
		query := "SELECT COUNT(*) FROM " + table + " WHERE site_id = ? AND bucket < ?"
		if err := db.QueryRowContext(ctx, query, siteID, cutoff).Scan(&count); err != nil {
			return counts, fmt.Errorf("count %s: %w", table, err)
		}
		counts.Rollups += count
	}
	return counts, nil
}

func (c retentionCounts) hasColdData() bool {
	return c.Hits > 0 ||
		c.Events > 0 ||
		c.WebVitals > 0 ||
		c.AIFetches > 0 ||
		c.QROpens > 0 ||
		c.ImportedTraffic > 0 ||
		c.ImportedDimensions > 0 ||
		c.ImportedEvents > 0 ||
		c.ImportedEventDimensions > 0 ||
		c.ImportedEventProperties > 0 ||
		c.SearchConsoleFacts > 0 ||
		c.Rollups > 0
}

func (c retentionCounts) logAttrs(siteID uuid.UUID, cutoff time.Time) []any {
	return []any{
		"site_id", siteID,
		"hits", c.Hits,
		"events", c.Events,
		"web_vitals", c.WebVitals,
		"ai_fetches", c.AIFetches,
		"qr_code_opens", c.QROpens,
		"imported_traffic", c.ImportedTraffic,
		"imported_dimensions", c.ImportedDimensions,
		"imported_events", c.ImportedEvents,
		"imported_event_dimensions", c.ImportedEventDimensions,
		"imported_event_properties", c.ImportedEventProperties,
		"search_console_facts", c.SearchConsoleFacts,
		"dirty_rollup_buckets", c.DirtyRollupBuckets,
		"rollups", c.Rollups,
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
	parts := make([]string, 0, len(retentionArchiveSources))
	for _, source := range retentionArchiveSources {
		cutoffValue := timestampCutoff
		if source.cutoffColumn == "date" {
			cutoffValue = dateCutoff
		}
		//nolint:gosec // source names are a closed internal registry; IDs and dates are generated values.
		parts = append(parts, fmt.Sprintf(
			"SELECT '%s' AS _source, * FROM %s WHERE site_id = '%s' AND %s < '%s'",
			source.table, source.table, siteID, source.cutoffColumn, cutoffValue,
		))
	}
	return fmt.Sprintf("COPY (%s) TO '%s' (FORMAT PARQUET, COMPRESSION 'SNAPPY');",
		strings.Join(parts, " UNION BY NAME "), safeFilename)
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
		{count: counts.QROpens, label: "qr code opens", query: "DELETE FROM qr_code_opens WHERE site_id = ? AND timestamp < ?"},
		{count: counts.ImportedTraffic, label: "imported traffic", query: "DELETE FROM imported_traffic_daily WHERE site_id = ? AND date < ?"},
		{count: counts.ImportedDimensions, label: "imported dimensions", query: "DELETE FROM imported_dimension_daily WHERE site_id = ? AND date < ?"},
		{count: counts.ImportedEvents, label: "imported events", query: "DELETE FROM imported_event_daily WHERE site_id = ? AND date < ?"},
		{count: counts.ImportedEventDimensions, label: "imported event dimensions", query: "DELETE FROM imported_event_dimensions_daily WHERE site_id = ? AND date < ?"},
		{count: counts.ImportedEventProperties, label: "imported event properties", query: "DELETE FROM imported_event_properties_daily WHERE site_id = ? AND date < ?"},
		{count: counts.SearchConsoleFacts, label: "Search Console facts", query: "DELETE FROM search_console_facts WHERE site_id = ? AND date < ?"},
		{count: counts.DirtyRollupBuckets, label: "dirty rollup buckets", query: "DELETE FROM rollup_dirty_buckets WHERE site_id = ? AND bucket < ?"},
		{count: counts.Rollups, label: "hourly rollups", query: "DELETE FROM hit_rollups_hourly WHERE site_id = ? AND bucket < ?"},
		{count: counts.Rollups, label: "daily rollups", query: "DELETE FROM hit_rollups_daily WHERE site_id = ? AND bucket < ?"},
		{count: counts.Rollups, label: "monthly rollups", query: "DELETE FROM hit_rollups_monthly WHERE site_id = ? AND bucket < ?"},
		{count: counts.Rollups, label: "hourly goal rollups", query: "DELETE FROM goal_rollups_hourly WHERE site_id = ? AND bucket < ?"},
		{count: counts.Rollups, label: "daily goal rollups", query: "DELETE FROM goal_rollups_daily WHERE site_id = ? AND bucket < ?"},
		{count: counts.Rollups, label: "monthly goal rollups", query: "DELETE FROM goal_rollups_monthly WHERE site_id = ? AND bucket < ?"},
		{count: counts.Rollups, label: "hourly funnel rollups", query: "DELETE FROM funnel_rollups_hourly WHERE site_id = ? AND bucket < ?"},
		{count: counts.Rollups, label: "daily funnel rollups", query: "DELETE FROM funnel_rollups_daily WHERE site_id = ? AND bucket < ?"},
		{count: counts.Rollups, label: "monthly funnel rollups", query: "DELETE FROM funnel_rollups_monthly WHERE site_id = ? AND bucket < ?"},
		{count: counts.Rollups, label: "hourly session rollups", query: "DELETE FROM session_rollups_hourly WHERE site_id = ? AND bucket < ?"},
		{count: counts.Rollups, label: "daily session rollups", query: "DELETE FROM session_rollups_daily WHERE site_id = ? AND bucket < ?"},
		{count: counts.Rollups, label: "monthly session rollups", query: "DELETE FROM session_rollups_monthly WHERE site_id = ? AND bucket < ?"},
	}
}

type retentionArchiveSource struct {
	table        string
	cutoffColumn string
}

var retentionRollupTables = []string{
	"hit_rollups_hourly", "hit_rollups_daily", "hit_rollups_monthly",
	"session_rollups_hourly", "session_rollups_daily", "session_rollups_monthly",
	"goal_rollups_hourly", "goal_rollups_daily", "goal_rollups_monthly",
	"funnel_rollups_hourly", "funnel_rollups_daily", "funnel_rollups_monthly",
}

var retentionArchiveSources = []retentionArchiveSource{
	{table: "hits", cutoffColumn: "timestamp"},
	{table: "events", cutoffColumn: "timestamp"},
	{table: "web_vitals", cutoffColumn: "timestamp"},
	{table: "ai_fetches", cutoffColumn: "timestamp"},
	{table: "qr_code_opens", cutoffColumn: "timestamp"},
	{table: "imported_traffic_daily", cutoffColumn: "date"},
	{table: "imported_dimension_daily", cutoffColumn: "date"},
	{table: "imported_event_daily", cutoffColumn: "date"},
	{table: "imported_event_dimensions_daily", cutoffColumn: "date"},
	{table: "imported_event_properties_daily", cutoffColumn: "date"},
	{table: "search_console_facts", cutoffColumn: "date"},
	{table: "hit_rollups_hourly", cutoffColumn: "bucket"},
	{table: "hit_rollups_daily", cutoffColumn: "bucket"},
	{table: "hit_rollups_monthly", cutoffColumn: "bucket"},
	{table: "session_rollups_hourly", cutoffColumn: "bucket"},
	{table: "session_rollups_daily", cutoffColumn: "bucket"},
	{table: "session_rollups_monthly", cutoffColumn: "bucket"},
	{table: "goal_rollups_hourly", cutoffColumn: "bucket"},
	{table: "goal_rollups_daily", cutoffColumn: "bucket"},
	{table: "goal_rollups_monthly", cutoffColumn: "bucket"},
	{table: "funnel_rollups_hourly", cutoffColumn: "bucket"},
	{table: "funnel_rollups_daily", cutoffColumn: "bucket"},
	{table: "funnel_rollups_monthly", cutoffColumn: "bucket"},
}

func (w *RetentionWorker) ensureS3Support(ctx context.Context) error {
	root, err := w.tenantMgr.ForTenant(ctx, uuid.Nil)
	if err != nil {
		return fmt.Errorf("open default tenant data-plane root: %w", err)
	}
	return database.WithDuckDBSession(ctx, root.DB(), database.DuckDBSessionOptions{
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
	sites, err := w.tenantMgr.Control().ListAllSites(ctx)
	if err != nil {
		return nil, fmt.Errorf("list retention sites: %w", err)
	}
	policies := make([]retentionSitePolicy, 0, len(sites))
	for _, site := range sites {
		if site.DataRetentionDays <= 0 {
			continue
		}
		tenantID, resolveErr := w.tenantMgr.Control().GetSiteTenantID(ctx, site.ID)
		if resolveErr != nil {
			if defaultTenantID == uuid.Nil {
				return nil, fmt.Errorf("resolve tenant for retention site: %w", resolveErr)
			}
			tenantID = defaultTenantID
		}
		policies = append(policies, retentionSitePolicy{ID: site.ID, Days: site.DataRetentionDays, TenantID: tenantID})
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
