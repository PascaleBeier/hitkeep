package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

const importStageCleanupInterval = 24 * time.Hour

type ImportStageCleanupWorker struct {
	store         *database.Store
	dataPath      string
	retentionDays int
	status        *database.ImportStageCleanupStatusTracker
}

type ImportStageCleaner struct {
	store         *database.Store
	dataPath      string
	retentionDays int
}

func NewImportStageCleanupWorker(store *database.Store, dataPath string, retentionDays int, status *database.ImportStageCleanupStatusTracker) *ImportStageCleanupWorker {
	return &ImportStageCleanupWorker{
		store:         store,
		dataPath:      dataPath,
		retentionDays: retentionDays,
		status:        status,
	}
}

func NewImportStageCleaner(store *database.Store, dataPath string, retentionDays int) *ImportStageCleaner {
	return &ImportStageCleaner{
		store:         store,
		dataPath:      normalizeImportStageDataPath(dataPath),
		retentionDays: retentionDays,
	}
}

func (w *ImportStageCleanupWorker) Start(ctx context.Context) {
	if w == nil || w.store == nil || w.retentionDays <= 0 {
		return
	}
	slog.Info("Import staging cleanup enabled", "retention_days", w.retentionDays)

	go func() {
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if _, err := RunImportStageCleanup(ctx, w.store, w.dataPath, w.retentionDays, w.status); err != nil {
				slog.Error("Initial import staging cleanup failed", "error", err)
			}
		}
	}()

	ticker := time.NewTicker(importStageCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := RunImportStageCleanup(ctx, w.store, w.dataPath, w.retentionDays, w.status); err != nil {
				slog.Error("Import staging cleanup failed", "error", err)
			}
		}
	}
}

func RunImportStageCleanup(ctx context.Context, store *database.Store, dataPath string, retentionDays int, status *database.ImportStageCleanupStatusTracker) (api.ImportStageCleanupRunResult, error) {
	cleaner := NewImportStageCleaner(store, dataPath, retentionDays)
	result, err := cleaner.Run(ctx)
	finishedAt := time.Now().UTC()
	if status != nil {
		if err != nil {
			status.SetFailed(finishedAt, err.Error(), result)
		} else {
			status.SetLastRun(finishedAt, result)
		}
	}
	return result, err
}

func (c *ImportStageCleaner) Estimate(ctx context.Context) (api.ImportStageCleanupEstimate, error) {
	if c == nil || c.store == nil {
		return api.ImportStageCleanupEstimate{}, fmt.Errorf("import stage cleanup store is not configured")
	}
	if c.retentionDays <= 0 {
		return api.ImportStageCleanupEstimate{}, nil
	}
	return c.store.EstimateStaleImportStageFiles(ctx, c.cutoff())
}

func (c *ImportStageCleaner) Run(ctx context.Context) (api.ImportStageCleanupRunResult, error) {
	result := api.ImportStageCleanupRunResult{}
	if c == nil || c.store == nil {
		return result, fmt.Errorf("import stage cleanup store is not configured")
	}
	if c.retentionDays <= 0 {
		return result, fmt.Errorf("import stage cleanup is disabled")
	}

	files, err := c.store.ListStaleImportStageFiles(ctx, c.cutoff())
	if err != nil {
		return result, err
	}

	now := time.Now().UTC()
	markedImports := map[uuid.UUID]struct{}{}
	skipImports := map[uuid.UUID]struct{}{}
	cleanedImports := map[uuid.UUID]struct{}{}
	pruneSiteDirs := map[uuid.UUID]struct{}{}
	var cleanupErrors []string

	for _, file := range files {
		if _, skip := skipImports[file.ImportID]; skip {
			continue
		}

		if isResumableImportStatus(file.ImportStatus) {
			if _, marked := markedImports[file.ImportID]; !marked {
				changed, err := c.store.MarkStaleImportExpired(ctx, file.ImportID, database.ImportStageExpiredMessage, now)
				if err != nil {
					cleanupErrors = append(cleanupErrors, err.Error())
					skipImports[file.ImportID] = struct{}{}
					continue
				}
				markedImports[file.ImportID] = struct{}{}
				if !changed {
					skipImports[file.ImportID] = struct{}{}
					continue
				}
				result.ImportsMarkedFailed++
			}
		}

		path, err := importStageFilePath(c.dataPath, file.RelativePath)
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Sprintf("%s: %v", file.ImportID, err))
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErrors = append(cleanupErrors, fmt.Sprintf("remove %s: %v", file.RelativePath, err))
			continue
		}
		if err := c.store.MarkImportFileCleaned(ctx, file.ImportID, file.FileID, now); err != nil {
			cleanupErrors = append(cleanupErrors, err.Error())
			continue
		}

		result.FilesCleaned++
		result.BytesCleaned += file.SizeBytes
		cleanedImports[file.ImportID] = struct{}{}
		pruneSiteDirs[file.SiteID] = struct{}{}
	}

	for siteID := range pruneSiteDirs {
		if err := os.Remove(filepath.Join(c.dataPath, "imports", siteID.String())); err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTEMPTY) {
			slog.Debug("Could not prune import staging directory", "site_id", siteID, "error", err)
		}
	}

	result.ImportsCleaned = len(cleanedImports)
	if len(cleanupErrors) > 0 {
		result.Errors = cleanupErrors
		return result, fmt.Errorf("import stage cleanup completed with %d error(s)", len(cleanupErrors))
	}
	return result, nil
}

func (c *ImportStageCleaner) cutoff() time.Time {
	return time.Now().UTC().AddDate(0, 0, -c.retentionDays)
}

func isResumableImportStatus(status string) bool {
	switch status {
	case database.ImportStatusUploading, database.ImportStatusValidated, database.ImportStatusValidationFailed:
		return true
	default:
		return false
	}
}

func importStageFilePath(dataPath, relativePath string) (string, error) {
	dataPath = normalizeImportStageDataPath(dataPath)
	cleanRelative := filepath.Clean(strings.TrimSpace(relativePath))
	if cleanRelative == "." || filepath.IsAbs(cleanRelative) {
		return "", fmt.Errorf("invalid staged import path")
	}
	parts := strings.Split(cleanRelative, string(filepath.Separator))
	if len(parts) < 3 || parts[0] != "imports" {
		return "", fmt.Errorf("staged import path must stay under imports")
	}
	target := filepath.Join(dataPath, cleanRelative)
	root, err := filepath.Abs(filepath.Join(dataPath, "imports"))
	if err != nil {
		return "", fmt.Errorf("resolve import staging root: %w", err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve staged import path: %w", err)
	}
	if absTarget != root && !strings.HasPrefix(absTarget, root+string(filepath.Separator)) {
		return "", fmt.Errorf("staged import path escapes import root")
	}
	return target, nil
}

func normalizeImportStageDataPath(dataPath string) string {
	dataPath = strings.TrimSpace(dataPath)
	if dataPath == "" {
		return "data"
	}
	return dataPath
}
