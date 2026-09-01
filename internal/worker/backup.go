package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"hitkeep/hklog"
	"hitkeep/internal/database"
)

// BackupWorker periodically exports all DuckDB databases to Parquet snapshots.
type BackupWorker struct {
	tenantMgr      *database.TenantStoreManager
	dataPath       string
	backupPath     string
	intervalMin    int
	retentionCount int
	s3Config       *S3Config
	status         *database.BackupStatusTracker
}

// NewBackupWorker creates a BackupWorker. If backupPath is empty, Start is a no-op.
func NewBackupWorker(
	tenantMgr *database.TenantStoreManager,
	dataPath string,
	backupPath string,
	intervalMin int,
	retentionCount int,
	s3Config *S3Config,
	status *database.BackupStatusTracker,
) *BackupWorker {
	return &BackupWorker{
		tenantMgr:      tenantMgr,
		dataPath:       dataPath,
		backupPath:     strings.TrimSpace(backupPath),
		intervalMin:    intervalMin,
		retentionCount: retentionCount,
		s3Config:       s3Config,
		status:         status,
	}
}

// Start runs the backup worker on a ticker loop. It returns immediately if
// backupPath is empty (backups disabled).
func (w *BackupWorker) Start(ctx context.Context) {
	if w.backupPath == "" {
		return
	}

	if IsS3ArchivePath(w.backupPath) {
		hklog.LoggerFromContext(ctx).Info("S3 backup enabled", "path", w.backupPath, "interval_min", w.intervalMin, "retention", w.retentionCount)
	} else {
		hklog.LoggerFromContext(ctx).Info("Local backup enabled", "path", w.backupPath, "interval_min", w.intervalMin, "retention", w.retentionCount)
	}
	w.setNextBackup(time.Now().UTC().Add(30 * time.Second))

	// Initial run after a short delay to let DB settle.
	go func() {
		if !waitForDelay(ctx, 30*time.Second) {
			return
		}
		if err := w.Run(ctx); err != nil {
			logBackupFailure(ctx, "Initial backup run failed", err)
		}
	}()

	interval := time.Duration(w.intervalMin) * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.Run(ctx); err != nil {
				logBackupFailure(ctx, "Backup worker failed", err)
			}
		}
	}
}

// Run executes a single backup cycle: export shared DB + all tenant DBs,
// then prune old snapshots beyond the retention count.
func (w *BackupWorker) Run(ctx context.Context) (err error) {
	defer func() {
		finishedAt := time.Now().UTC()
		if err != nil {
			w.recordFailure(finishedAt, err)
			return
		}
		w.recordSuccess(finishedAt)
	}()

	timestamp := time.Now().UTC().Format("2006-01-02T150405Z")
	hklog.LoggerFromContext(ctx).Debug("Starting database backup", "timestamp", timestamp)

	isS3 := IsS3ArchivePath(w.backupPath)

	// Backup shared DB.
	sharedDest := joinArchivePath(w.backupPath, "shared", timestamp)
	if err := w.exportDatabase(ctx, w.tenantMgr.Shared(), sharedDest, isS3); err != nil {
		w.removeIncompleteLocalSnapshot(sharedDest, isS3, ctx)
		return fmt.Errorf("backup shared database: %w", err)
	}
	hklog.LoggerFromContext(ctx).Debug("Shared database backed up", "dest", sharedDest)

	// Discover every active tenant, including the default tenant. The shared
	// snapshot above remains control-plane-only; tenant snapshots live under
	// tenants/<tenantID>.
	tenantIDs, err := w.tenantMgr.Control().ListActiveTenantIDs(ctx)
	if err != nil {
		if isMissingRelationError(err, "tenants") || isBinderError(err) {
			hklog.LoggerFromContext(ctx).Debug("Tenants schema not ready, skipping tenant backups", "error_kind", backupErrorKind(err))
			tenantIDs = nil
		} else {
			hklog.LoggerFromContext(ctx).Error("Failed to list tenant IDs for backup", "error_kind", backupErrorKind(err))
			tenantIDs = nil
		}
	}

	// Backup each non-default tenant DB.
	var tenantErrors []error
	for _, tenantID := range tenantIDs {
		tenantStore, err := w.tenantMgr.ForTenant(ctx, tenantID)
		if err != nil {
			hklog.LoggerFromContext(ctx).Error("Failed to open tenant store for backup", "tenant_id", tenantID, "error_kind", backupErrorKind(err))
			tenantErrors = append(tenantErrors, fmt.Errorf("open tenant %s database for backup: %w", tenantID, err))
			continue
		}
		if tenantStore == w.tenantMgr.Control() {
			// Pre-split recovery tools and isolated compatibility tests may still
			// construct a manager over one shared file. Mandatory production
			// startup completes the split before the backup worker is created.
			continue
		}

		tenantDest := joinArchivePath(w.backupPath, "tenants", tenantID.String(), timestamp)
		if err := w.exportDatabase(ctx, tenantStore, tenantDest, isS3); err != nil {
			hklog.LoggerFromContext(ctx).Error("Failed to backup tenant database", "tenant_id", tenantID, "error_kind", backupErrorKind(err))
			w.removeIncompleteLocalSnapshot(tenantDest, isS3, ctx)
			tenantErrors = append(tenantErrors, fmt.Errorf("backup tenant %s database: %w", tenantID, err))
			continue
		}
		hklog.LoggerFromContext(ctx).Debug("Tenant database backed up", "tenant_id", tenantID, "dest", tenantDest)
	}

	// Prune old snapshots.
	if !isS3 {
		w.pruneLocalSnapshots(filepath.Join(w.backupPath, "shared"), ctx)
		for _, tenantID := range tenantIDs {
			w.pruneLocalSnapshots(filepath.Join(w.backupPath, "tenants", tenantID.String()), ctx)
		}
	} else {
		hklog.LoggerFromContext(ctx).Debug("S3 backup pruning: configure S3 lifecycle policies to manage snapshot retention")
	}
	if len(tenantErrors) > 0 {
		return fmt.Errorf("backup incomplete: %w", errors.Join(tenantErrors...))
	}

	hklog.LoggerFromContext(ctx).Info("Database backup completed", "timestamp", timestamp)
	return nil
}

func (w *BackupWorker) removeIncompleteLocalSnapshot(path string, isS3 bool, ctxArgs ...context.Context) {
	if isS3 || strings.TrimSpace(path) == "" {
		return
	}
	if err := os.RemoveAll(path); err != nil {
		ctx := context.Background()
		if len(ctxArgs) > 0 && ctxArgs[0] != nil {
			ctx = ctxArgs[0]
		}
		hklog.LoggerFromContext(ctx).Warn("Could not remove incomplete backup snapshot", "path", path, "error_kind", backupErrorKind(err))
	}
}

func (w *BackupWorker) recordSuccess(at time.Time) {
	if w.status == nil {
		return
	}
	w.status.SetLastBackup(at)
	if next, ok := w.nextBackupTime(at); ok {
		w.status.SetNextBackup(next)
	}
}

func (w *BackupWorker) recordFailure(at time.Time, err error) {
	if w.status == nil {
		return
	}
	w.status.SetFailed(at, err.Error())
	if next, ok := w.nextBackupTime(at); ok {
		w.status.SetNextBackup(next)
	}
}

func (w *BackupWorker) setNextBackup(at time.Time) {
	if w.status == nil {
		return
	}
	w.status.SetNextBackup(at)
}

func (w *BackupWorker) nextBackupTime(after time.Time) (time.Time, bool) {
	if w.intervalMin <= 0 {
		return time.Time{}, false
	}
	return after.Add(time.Duration(w.intervalMin) * time.Minute), true
}

// exportDatabase checkpoints and exports a DuckDB database to the given destination.
func (w *BackupWorker) exportDatabase(ctx context.Context, store *database.Store, dest string, isS3 bool) error {
	if store == nil {
		return errors.New("database store is not available")
	}
	if err := store.Checkpoint(ctx, "backup"); err != nil {
		return fmt.Errorf("checkpoint before export: %w", err)
	}
	// Ensure local directory exists.
	if !isS3 {
		if err := os.MkdirAll(dest, 0755); err != nil {
			return fmt.Errorf("create backup directory %s: %w", dest, err)
		}
	}

	safeDest := strings.ReplaceAll(dest, "'", "''")
	query := fmt.Sprintf("EXPORT DATABASE '%s' (FORMAT PARQUET);", safeDest)
	return database.WithDuckDBSession(ctx, store.DB(), database.DuckDBSessionOptions{
		S3: s3ConfigForSession(isS3, w.s3Config),
	}, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("export database to %s: %w", dest, err)
		}
		return nil
	})
}

// pruneLocalSnapshots removes the oldest snapshot directories in dir,
// keeping at most retentionCount. Snapshot dirs are ISO timestamp names
// that sort lexicographically.
func (w *BackupWorker) pruneLocalSnapshots(dir string, ctxArgs ...context.Context) {
	ctx := context.Background()
	if len(ctxArgs) > 0 && ctxArgs[0] != nil {
		ctx = ctxArgs[0]
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			hklog.LoggerFromContext(ctx).Warn("Could not read backup directory for pruning", "dir", dir, "error_kind", backupErrorKind(err))
		}
		return
	}

	// Collect only directories (snapshot timestamps).
	dirs := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}

	if len(dirs) <= w.retentionCount {
		return
	}

	sort.Strings(dirs)

	toRemove := dirs[:len(dirs)-w.retentionCount]
	for _, name := range toRemove {
		path := filepath.Join(dir, name)
		if err := os.RemoveAll(path); err != nil {
			hklog.LoggerFromContext(ctx).Error("Failed to prune old backup snapshot", "path", path, "error_kind", backupErrorKind(err))
		} else {
			hklog.LoggerFromContext(ctx).Debug("Pruned old backup snapshot", "path", path)
		}
	}
}

func backupErrorKind(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, os.ErrNotExist):
		return "not_found"
	case errors.Is(err, os.ErrPermission):
		return "permission_denied"
	default:
		return "backup_failed"
	}
}

func logBackupFailure(ctx context.Context, message string, err error) {
	hklog.LoggerFromContext(ctx).Error(message, "error_kind", backupErrorKind(err))
}
