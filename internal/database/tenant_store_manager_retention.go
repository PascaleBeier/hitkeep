package database

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

// SyncTeamRetention brings every plan-managed site owned by teamID in line
// with the team's plan-resolved retention cap (0 = unlimited, no-op).
//
// Plan-managed sites (RetentionSyncedFromPlan = true) are set to exactly
// maxRetentionDays, in both directions: an upgrade raises the cap, a
// downgrade lowers it. This is what makes retention a real, working signal
// for a customer to upgrade.
//
// Manually-customized sites (RetentionSyncedFromPlan = false, set the
// moment a user edits retention via PUT /api/sites/{id}/retention) are only
// ever clamped downward when their current value exceeds the new cap, and
// are never raised automatically — preserving HitKeep's documented
// "operator-controlled retention windows" compliance behavior.
//
// Returns the number of sites updated.
func (m *TenantStoreManager) SyncTeamRetention(ctx context.Context, teamID uuid.UUID, maxRetentionDays int) (int, error) {
	if teamID == uuid.Nil {
		return 0, nil
	}
	if maxRetentionDays < 0 {
		maxRetentionDays = 0
	}

	sites, err := m.control.ListSitesForTenant(ctx, teamID)
	if err != nil {
		return 0, err
	}

	updated := 0
	var firstErr error
	for _, site := range sites {
		newDays, needsUpdate := resolveSyncedRetentionDays(site, maxRetentionDays)
		if !needsUpdate {
			continue
		}

		if err := m.control.SetSiteRetentionDaysSystem(ctx, site.ID, newDays, site.RetentionSyncedFromPlan); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Error("Failed to sync site retention to plan cap", "site_id", site.ID, "team_id", teamID, "error", err)
			continue
		}
		if err := m.SyncSite(ctx, site.ID); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Error("Failed to sync tenant mirror after retention sync", "site_id", site.ID, "team_id", teamID, "error", err)
			continue
		}

		updated++
		slog.Info("Synced site retention to plan cap", "site_id", site.ID, "team_id", teamID, "days", newDays)
	}

	if updated > 0 {
		slog.Info("Synced team retention to plan cap", "team_id", teamID, "sites_updated", updated, "max_retention_days", maxRetentionDays)
	}

	return updated, firstErr
}

// resolveSyncedRetentionDays returns the retention value a site should have
// given the team's plan cap, and whether it differs from the site's current
// value (i.e. whether a write is needed).
func resolveSyncedRetentionDays(site api.Site, maxRetentionDays int) (int, bool) {
	if site.RetentionSyncedFromPlan {
		if site.DataRetentionDays == maxRetentionDays {
			return maxRetentionDays, false
		}
		return maxRetentionDays, true
	}

	// Manually customized: only clamp downward, never raise automatically.
	if maxRetentionDays == 0 {
		return site.DataRetentionDays, false
	}
	if site.DataRetentionDays != 0 && site.DataRetentionDays <= maxRetentionDays {
		return site.DataRetentionDays, false
	}
	return maxRetentionDays, true
}
