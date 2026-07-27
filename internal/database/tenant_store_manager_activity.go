package database

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

type activationSiteMetadata struct {
	row       api.SystemActivationRow
	tenantID  uuid.UUID
	createdAt time.Time
}

// ListSystemActivation reads control-plane metadata and tenant-local activity
// separately. The legacy Store method remains available for single-database
// boots, while split boots must never query compatibility activity tables in
// hitkeep.db.
func (m *TenantStoreManager) ListSystemActivation(ctx context.Context, q ActivationQuery) (*api.SystemActivationResponse, error) {
	if m == nil || m.control == nil {
		return nil, fmt.Errorf("tenant store manager is not configured")
	}
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	if q.Now.IsZero() {
		q.Now = time.Now().UTC()
	}

	controlRows, err := m.control.ListActivationMetadata(ctx)
	if err != nil {
		return nil, err
	}
	metadataRows := make([]activationSiteMetadata, len(controlRows))
	for i, entry := range controlRows {
		metadataRows[i] = activationSiteMetadata{row: entry.Row, tenantID: entry.TenantID, createdAt: entry.CreatedAt}
	}
	dormantCutoff := q.Now.UTC().Add(-activityDormantAfter)
	statusFilter := strings.ToLower(strings.TrimSpace(q.Status))
	teamFilter := strings.ToLower(strings.TrimSpace(q.Team))
	domainFilter := strings.ToLower(strings.TrimSpace(q.Domain))
	filtered := make([]activationSiteMetadata, 0, len(metadataRows))
	activeByTenant := make(map[uuid.UUID]int)
	for i := range metadataRows {
		entry := &metadataRows[i]
		store, _, err := m.ResolveSiteStore(ctx, entry.row.SiteID)
		if err != nil {
			return nil, fmt.Errorf("resolve activation analytics store for site %s: %w", entry.row.SiteID, err)
		}
		status, err := store.GetSiteTrackingStatus(ctx, entry.row.SiteID, q.Now)
		if err != nil {
			return nil, fmt.Errorf("load activation status for site %s: %w", entry.row.SiteID, err)
		}
		if status == nil {
			continue
		}
		entry.row.Status = status.Status
		entry.row.FirstHitAt = status.FirstHitAt
		entry.row.LastHitAt = status.LastHitAt
		entry.row.LastEventAt = status.LastEventAt
		entry.row.LastEventName = status.LastEventName
		entry.row.TrackerSource = status.TrackerSource
		entry.row.TrackerVersion = status.TrackerVersion
		entry.row.HitsLast24h, entry.row.HitsLast7d, entry.row.EventsLast7d, err = store.SiteActivityCounts(ctx, entry.row.SiteID, q.Now)
		if err != nil {
			return nil, fmt.Errorf("load activation counts for site %s: %w", entry.row.SiteID, err)
		}
		if status.Status == api.TrackingStatusLive && status.LastHitAt != nil && !status.LastHitAt.Before(dormantCutoff) {
			activeByTenant[entry.tenantID]++
		}
		if statusFilter != "" && string(status.Status) != statusFilter {
			continue
		}
		if teamFilter != "" && !strings.Contains(strings.ToLower(entry.row.TeamName), teamFilter) {
			continue
		}
		if domainFilter != "" && !strings.Contains(strings.ToLower(entry.row.SiteDomain), domainFilter) {
			continue
		}
		if q.LastSeenFrom != nil && (status.LastHitAt == nil || status.LastHitAt.Before(q.LastSeenFrom.UTC())) {
			continue
		}
		if q.LastSeenTo != nil && (status.LastHitAt == nil || status.LastHitAt.After(q.LastSeenTo.UTC())) {
			continue
		}
		filtered = append(filtered, *entry)
	}
	for i := range filtered {
		filtered[i].row.SitesCount = 0
		filtered[i].row.ActiveSites = activeByTenant[filtered[i].tenantID]
	}
	siteCountByTenant := make(map[uuid.UUID]int)
	for _, entry := range metadataRows {
		siteCountByTenant[entry.tenantID]++
	}
	for i := range filtered {
		filtered[i].row.SitesCount = siteCountByTenant[filtered[i].tenantID]
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left, right := filtered[i], filtered[j]
		leftSort, rightSort := left.createdAt, right.createdAt
		if left.row.LastHitAt != nil {
			leftSort = *left.row.LastHitAt
		}
		if right.row.LastHitAt != nil {
			rightSort = *right.row.LastHitAt
		}
		if !leftSort.Equal(rightSort) {
			return leftSort.After(rightSort)
		}
		return left.row.SiteDomain < right.row.SiteDomain
	})
	resp := &api.SystemActivationResponse{Rows: make([]api.SystemActivationRow, 0), Total: len(filtered), Limit: q.Limit, Offset: q.Offset}
	if q.Offset < len(filtered) {
		end := min(q.Offset+q.Limit, len(filtered))
		for _, entry := range filtered[q.Offset:end] {
			resp.Rows = append(resp.Rows, entry.row)
		}
	}
	resp.HasMore = q.Offset+len(resp.Rows) < resp.Total
	return resp, nil
}
