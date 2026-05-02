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

	if IsS3ArchivePath(w.path) {
		if err := w.ensureS3Support(ctx); err != nil {
			return fmt.Errorf("failed to enable duckdb s3 support: %w", err)
		}
	} else {
		// Ensure the local archive directory exists.
		if err := os.MkdirAll(w.path, 0755); err != nil {
			return fmt.Errorf("failed to create archive directory: %w", err)
		}
	}

	defaultTenantID, err := w.tenantMgr.Shared().GetDefaultTenantID(ctx)
	if err != nil {
		if isBinderError(err) {
			slog.Debug("Tenant schema not ready, using nil default tenant ID", "error", err)
		} else {
			slog.Warn("Failed to resolve default tenant for retention archive layout", "error", err)
		}
		defaultTenantID = uuid.Nil
	}

	policies, err := w.loadRetentionPolicies(ctx, defaultTenantID)
	if err != nil {
		return err
	}

	for _, p := range policies {
		cutoff := time.Now().AddDate(0, 0, -p.Days)

		tenantStore, err := w.tenantMgr.ForTenant(ctx, p.TenantID)
		if err != nil {
			slog.Error("Failed to resolve tenant store for retention", "error", err, "site_id", p.ID, "tenant_id", p.TenantID)
			continue
		}
		db := tenantStore.DB()

		var hitCount, eventCount, fetchCount, importedTrafficCount, importedDimensionCount, importedEventCount, importedEventPropertyCount int64
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ? AND timestamp < ?", p.ID, cutoff).Scan(&hitCount)
		if err != nil {
			slog.Error("Failed to count hits for retention", "error", err, "site_id", p.ID)
			continue
		}
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE site_id = ? AND timestamp < ?", p.ID, cutoff).Scan(&eventCount)
		if err != nil {
			slog.Error("Failed to count events for retention", "error", err, "site_id", p.ID)
			continue
		}
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_fetches WHERE site_id = ? AND timestamp < ?", p.ID, cutoff).Scan(&fetchCount)
		if err != nil {
			slog.Error("Failed to count ai fetches for retention", "error", err, "site_id", p.ID)
			continue
		}
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM imported_traffic_daily WHERE site_id = ? AND date < ?", p.ID, cutoff).Scan(&importedTrafficCount)
		if err != nil {
			slog.Error("Failed to count imported traffic for retention", "error", err, "site_id", p.ID)
			continue
		}
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM imported_dimension_daily WHERE site_id = ? AND date < ?", p.ID, cutoff).Scan(&importedDimensionCount)
		if err != nil {
			slog.Error("Failed to count imported dimensions for retention", "error", err, "site_id", p.ID)
			continue
		}
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM imported_event_daily WHERE site_id = ? AND date < ?", p.ID, cutoff).Scan(&importedEventCount)
		if err != nil {
			slog.Error("Failed to count imported events for retention", "error", err, "site_id", p.ID)
			continue
		}
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM imported_event_properties_daily WHERE site_id = ? AND date < ?", p.ID, cutoff).Scan(&importedEventPropertyCount)
		if err != nil {
			slog.Error("Failed to count imported event properties for retention", "error", err, "site_id", p.ID)
			continue
		}

		if hitCount == 0 && eventCount == 0 && fetchCount == 0 && importedTrafficCount == 0 && importedDimensionCount == 0 && importedEventCount == 0 && importedEventPropertyCount == 0 {
			continue
		}

		slog.Info("Archiving old data", "site_id", p.ID, "hits", hitCount, "events", eventCount, "ai_fetches", fetchCount, "imported_traffic", importedTrafficCount, "imported_dimensions", importedDimensionCount, "imported_events", importedEventCount, "imported_event_properties", importedEventPropertyCount, "cutoff", cutoff.Format(time.DateOnly))

		filename := w.archiveFilename(p.ID, p.TenantID, defaultTenantID)
		if !IsS3ArchivePath(filename) {
			if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
				slog.Error("Failed to create archive destination", "error", err, "site_id", p.ID, "tenant_id", p.TenantID, "path", filename)
				continue
			}
		}

		safeFilename := strings.ReplaceAll(filename, "'", "''")

		//nolint:gosec // DuckDB COPY doesn't support parameterized queries; values are internally generated (UUID, time, escaped filepath)
		exportQuery := fmt.Sprintf(`
				COPY (
					SELECT 'hits' AS _source, * FROM hits WHERE site_id = '%s' AND timestamp < '%s'
					UNION BY NAME
					SELECT 'events' AS _source, * FROM events WHERE site_id = '%s' AND timestamp < '%s'
					UNION BY NAME
					SELECT 'ai_fetches' AS _source, * FROM ai_fetches WHERE site_id = '%s' AND timestamp < '%s'
					UNION BY NAME
					SELECT 'imported_traffic_daily' AS _source, * FROM imported_traffic_daily WHERE site_id = '%s' AND date < '%s'
					UNION BY NAME
					SELECT 'imported_dimension_daily' AS _source, * FROM imported_dimension_daily WHERE site_id = '%s' AND date < '%s'
					UNION BY NAME
					SELECT 'imported_event_daily' AS _source, * FROM imported_event_daily WHERE site_id = '%s' AND date < '%s'
					UNION BY NAME
					SELECT 'imported_event_properties_daily' AS _source, * FROM imported_event_properties_daily WHERE site_id = '%s' AND date < '%s'
				) TO '%s' (FORMAT PARQUET, COMPRESSION 'SNAPPY');
			`, p.ID, cutoff.Format(time.RFC3339), p.ID, cutoff.Format(time.RFC3339), p.ID, cutoff.Format(time.RFC3339), p.ID, cutoff.Format(time.DateOnly), p.ID, cutoff.Format(time.DateOnly), p.ID, cutoff.Format(time.DateOnly), p.ID, cutoff.Format(time.DateOnly), safeFilename)

		err = database.WithDuckDBSession(ctx, db, database.DuckDBSessionOptions{
			S3: s3ConfigForSession(IsS3ArchivePath(w.path), w.s3Config),
		}, func(conn *sql.Conn) error {
			if _, err := conn.ExecContext(ctx, exportQuery); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			slog.Error("Failed to export data to parquet", "error", err, "site_id", p.ID)
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			slog.Error("Failed to start transaction for deletion", "error", err)
			continue
		}

		if hitCount > 0 {
			if _, err := tx.ExecContext(ctx, "DELETE FROM hits WHERE site_id = ? AND timestamp < ?", p.ID, cutoff); err != nil {
				slog.Error("Failed to prune hits", "error", err, "site_id", p.ID)
				_ = tx.Rollback()
				continue
			}
		}

		if eventCount > 0 {
			if _, err := tx.ExecContext(ctx, "DELETE FROM events WHERE site_id = ? AND timestamp < ?", p.ID, cutoff); err != nil {
				slog.Error("Failed to prune events", "error", err, "site_id", p.ID)
				_ = tx.Rollback()
				continue
			}
		}

		if fetchCount > 0 {
			if _, err := tx.ExecContext(ctx, "DELETE FROM ai_fetches WHERE site_id = ? AND timestamp < ?", p.ID, cutoff); err != nil {
				slog.Error("Failed to prune ai fetches", "error", err, "site_id", p.ID)
				_ = tx.Rollback()
				continue
			}
		}

		if importedTrafficCount > 0 {
			if _, err := tx.ExecContext(ctx, "DELETE FROM imported_traffic_daily WHERE site_id = ? AND date < ?", p.ID, cutoff); err != nil {
				slog.Error("Failed to prune imported traffic", "error", err, "site_id", p.ID)
				_ = tx.Rollback()
				continue
			}
		}
		if importedDimensionCount > 0 {
			if _, err := tx.ExecContext(ctx, "DELETE FROM imported_dimension_daily WHERE site_id = ? AND date < ?", p.ID, cutoff); err != nil {
				slog.Error("Failed to prune imported dimensions", "error", err, "site_id", p.ID)
				_ = tx.Rollback()
				continue
			}
		}
		if importedEventCount > 0 {
			if _, err := tx.ExecContext(ctx, "DELETE FROM imported_event_daily WHERE site_id = ? AND date < ?", p.ID, cutoff); err != nil {
				slog.Error("Failed to prune imported events", "error", err, "site_id", p.ID)
				_ = tx.Rollback()
				continue
			}
		}
		if importedEventPropertyCount > 0 {
			if _, err := tx.ExecContext(ctx, "DELETE FROM imported_event_properties_daily WHERE site_id = ? AND date < ?", p.ID, cutoff); err != nil {
				slog.Error("Failed to prune imported event properties", "error", err, "site_id", p.ID)
				_ = tx.Rollback()
				continue
			}
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM hit_rollups_hourly WHERE site_id = ? AND bucket < ?", p.ID, cutoff); err != nil {
			slog.Error("Failed to prune hourly rollups", "error", err, "site_id", p.ID)
			_ = tx.Rollback()
			continue
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM hit_rollups_daily WHERE site_id = ? AND bucket < ?", p.ID, cutoff); err != nil {
			slog.Error("Failed to prune daily rollups", "error", err, "site_id", p.ID)
			_ = tx.Rollback()
			continue
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM hit_rollups_monthly WHERE site_id = ? AND bucket < ?", p.ID, cutoff); err != nil {
			slog.Error("Failed to prune monthly rollups", "error", err, "site_id", p.ID)
			_ = tx.Rollback()
			continue
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM goal_rollups_hourly WHERE site_id = ? AND bucket < ?", p.ID, cutoff); err != nil {
			slog.Error("Failed to prune hourly goal rollups", "error", err, "site_id", p.ID)
			_ = tx.Rollback()
			continue
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM goal_rollups_daily WHERE site_id = ? AND bucket < ?", p.ID, cutoff); err != nil {
			slog.Error("Failed to prune daily goal rollups", "error", err, "site_id", p.ID)
			_ = tx.Rollback()
			continue
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM goal_rollups_monthly WHERE site_id = ? AND bucket < ?", p.ID, cutoff); err != nil {
			slog.Error("Failed to prune monthly goal rollups", "error", err, "site_id", p.ID)
			_ = tx.Rollback()
			continue
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM funnel_rollups_hourly WHERE site_id = ? AND bucket < ?", p.ID, cutoff); err != nil {
			slog.Error("Failed to prune hourly funnel rollups", "error", err, "site_id", p.ID)
			_ = tx.Rollback()
			continue
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM funnel_rollups_daily WHERE site_id = ? AND bucket < ?", p.ID, cutoff); err != nil {
			slog.Error("Failed to prune daily funnel rollups", "error", err, "site_id", p.ID)
			_ = tx.Rollback()
			continue
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM funnel_rollups_monthly WHERE site_id = ? AND bucket < ?", p.ID, cutoff); err != nil {
			slog.Error("Failed to prune monthly funnel rollups", "error", err, "site_id", p.ID)
			_ = tx.Rollback()
			continue
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM session_rollups_hourly WHERE site_id = ? AND bucket < ?", p.ID, cutoff); err != nil {
			slog.Error("Failed to prune hourly session rollups", "error", err, "site_id", p.ID)
			_ = tx.Rollback()
			continue
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM session_rollups_daily WHERE site_id = ? AND bucket < ?", p.ID, cutoff); err != nil {
			slog.Error("Failed to prune daily session rollups", "error", err, "site_id", p.ID)
			_ = tx.Rollback()
			continue
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM session_rollups_monthly WHERE site_id = ? AND bucket < ?", p.ID, cutoff); err != nil {
			slog.Error("Failed to prune monthly session rollups", "error", err, "site_id", p.ID)
			_ = tx.Rollback()
			continue
		}

		if err := tx.Commit(); err != nil {
			slog.Error("Failed to commit deletion", "error", err, "site_id", p.ID)
		} else {
			slog.Info("Retention process completed", "site_id", p.ID, "tenant_id", p.TenantID, "archive", filename)
		}
	}

	return nil
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
