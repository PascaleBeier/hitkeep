package worker

import (
	"context"
	"log/slog"
	"time"

	"hitkeep/internal/database"
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
		time.Sleep(10 * time.Second)
		if err := w.Run(ctx); err != nil {
			slog.Error("Initial rollup backfill failed", "error", err)
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
				slog.Error("Rollup dirty refresh failed", "error", err)
			}
		case <-fullTicker.C:
			if err := w.Run(ctx); err != nil {
				slog.Error("Rollup backfill failed", "error", err)
			}
		}
	}
}

func (w *RollupBackfillWorker) Run(ctx context.Context) error {
	sites, err := w.tenantMgr.Control().ListAllSites(ctx)
	if err != nil {
		return err
	}
	for _, site := range sites {
		siteID := site.ID
		tenantStore, _, err := w.tenantMgr.ResolveSiteStore(ctx, siteID)
		if err != nil {
			slog.Warn("Failed to resolve tenant store for rollup backfill", "error", err, "site_id", siteID)
			continue
		}

		if err := tenantStore.BackfillRollups(ctx, siteID); err != nil {
			slog.Warn("Rollup backfill failed for site", "error", err, "site_id", siteID)
		}
	}

	return nil
}

func (w *RollupBackfillWorker) RunDirty(ctx context.Context) error {
	sites, err := w.tenantMgr.Control().ListAllSites(ctx)
	if err != nil {
		return err
	}
	for _, site := range sites {
		siteID := site.ID
		tenantStore, _, err := w.tenantMgr.ResolveSiteStore(ctx, siteID)
		if err != nil {
			slog.Warn("Failed to resolve tenant store for dirty rollup refresh", "error", err, "site_id", siteID)
			continue
		}

		if err := tenantStore.ProcessDirtyRollups(ctx, siteID); err != nil {
			slog.Warn("Dirty rollup refresh failed for site", "error", err, "site_id", siteID)
		}
	}

	return nil
}
