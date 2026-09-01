//go:build billing

package worker

import (
	"context"
	"time"

	"hitkeep/config"
	"hitkeep/hklog"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
)

// CloudRetentionSyncWorker keeps every cloud team's site retention in line
// with their current plan's MaxRetentionDays entitlement. It is the daily
// reconciliation safety net for internal/server/cloud's webhook-triggered
// sync (which applies changes immediately on plan change); this worker
// additionally covers teams that existed before this feature shipped and
// any sync missed due to a transient failure.
type CloudRetentionSyncWorker struct {
	tenantMgr *database.TenantStoreManager
	ent       *entitlements.Service
	conf      *config.Config
}

func NewCloudRetentionSyncWorker(tenantMgr *database.TenantStoreManager, ent *entitlements.Service, conf *config.Config) *CloudRetentionSyncWorker {
	return &CloudRetentionSyncWorker{
		tenantMgr: tenantMgr,
		ent:       ent,
		conf:      conf,
	}
}

// Start waits until 09:00 UTC, then ticks every 24 hours.
func (w *CloudRetentionSyncWorker) Start(ctx context.Context) {
	runDailyAtUTC(ctx, "CloudRetentionSyncWorker", 9, w.Run)
}

func (w *CloudRetentionSyncWorker) Run(ctx context.Context) {
	w.RunAt(ctx, time.Now().UTC())
}

func (w *CloudRetentionSyncWorker) RunAt(ctx context.Context, now time.Time) {
	if w == nil || w.tenantMgr == nil || w.tenantMgr.Shared() == nil || w.ent == nil || w.conf == nil || !w.conf.CloudHosted {
		return
	}

	teamIDs, err := w.tenantMgr.Shared().ListNonDefaultTenantIDs(ctx)
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("CloudRetentionSyncWorker: failed to list teams", "error", err)
		return
	}

	for _, teamID := range teamIDs {
		if ctx.Err() != nil {
			hklog.LoggerFromContext(ctx).Warn("CloudRetentionSyncWorker: context cancelled, halting sync")
			return
		}

		ent := w.ent.TeamEntitlements(ctx, teamID)
		if ent == nil {
			continue
		}

		if _, err := w.tenantMgr.SyncTeamRetention(ctx, teamID, ent.MaxRetentionDays); err != nil {
			hklog.LoggerFromContext(ctx).Error("CloudRetentionSyncWorker: failed to sync team retention", "team_id", teamID, "error", err)
		}
	}
}
