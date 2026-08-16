package worker

import (
	"context"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/database"
	"hitkeep/internal/hklog"
)

type RollupBackfillWorker struct {
	tenantMgr *database.TenantStoreManager
}

func NewRollupBackfillWorker(tenantMgr *database.TenantStoreManager) *RollupBackfillWorker {
	return &RollupBackfillWorker{
		tenantMgr: tenantMgr,
	}
}

func (w *RollupBackfillWorker) Start(ctx context.Context) {
	go func() {
		if !waitForDelay(ctx, 10*time.Second) {
			return
		}
		if err := w.Run(ctx); err != nil {
			hklog.LoggerFromContext(ctx).Error("Initial rollup backfill failed", "error", err)
		}
	}()

	dirtyTicker := time.NewTicker(time.Minute)
	defer dirtyTicker.Stop()
	fullTicker := time.NewTicker(24 * time.Hour)
	defer fullTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-dirtyTicker.C:
			if err := w.RunDirty(ctx); err != nil {
				hklog.LoggerFromContext(ctx).Error("Rollup dirty refresh failed", "error", err)
			}
		case <-fullTicker.C:
			if err := w.Run(ctx); err != nil {
				hklog.LoggerFromContext(ctx).Error("Rollup backfill failed", "error", err)
			}
		}
	}
}

func (w *RollupBackfillWorker) Run(ctx context.Context) error {
	return w.runForEachSite(ctx,
		"Failed to scan site for rollup backfill",
		"Failed to resolve tenant store for rollup backfill",
		"Rollup backfill failed for site",
		func(ctx context.Context, store *database.Store, siteID uuid.UUID) error {
			return store.BackfillRollups(ctx, siteID)
		},
	)
}

func (w *RollupBackfillWorker) RunDirty(ctx context.Context) error {
	return w.runForEachSite(ctx,
		"Failed to scan site for dirty rollup refresh",
		"Failed to resolve tenant store for dirty rollup refresh",
		"Dirty rollup refresh failed for site",
		func(ctx context.Context, store *database.Store, siteID uuid.UUID) error {
			return store.ProcessDirtyRollups(ctx, siteID)
		},
	)
}

func (w *RollupBackfillWorker) runForEachSite(
	ctx context.Context,
	scanError, resolveError, operationError string,
	operation func(context.Context, *database.Store, uuid.UUID) error,
) error {
	shared := w.tenantMgr.Shared()

	rows, err := shared.DB().QueryContext(ctx, "SELECT id FROM sites")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var siteID uuid.UUID
		if err := rows.Scan(&siteID); err != nil {
			hklog.LoggerFromContext(ctx).Warn(scanError, "error", err)
			continue
		}

		tenantStore, _, err := w.tenantMgr.ResolveSiteStore(ctx, siteID)
		if err != nil {
			hklog.LoggerFromContext(ctx).Warn(resolveError, "error", err, "site_id", siteID)
			continue
		}

		if err := operation(ctx, tenantStore, siteID); err != nil {
			hklog.LoggerFromContext(ctx).Warn(operationError, "error", err, "site_id", siteID)
		}
	}

	return rows.Err()
}
