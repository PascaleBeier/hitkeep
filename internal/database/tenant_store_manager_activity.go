package database

import (
	"cmp"
	"context"
	"fmt"
	"slices"
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
	if m == nil || m.shared == nil || !m.dataPlaneEnabled {
		if m == nil || m.shared == nil {
			return nil, fmt.Errorf("tenant store manager is not configured")
		}
		return m.shared.ListSystemActivation(ctx, q)
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

	metadataRows, err := m.loadActivationMetadata(ctx)
	if err != nil {
		return nil, err
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
	slices.SortStableFunc(filtered, func(left, right activationSiteMetadata) int {
		leftSort, rightSort := left.createdAt, right.createdAt
		if left.row.LastHitAt != nil {
			leftSort = *left.row.LastHitAt
		}
		if right.row.LastHitAt != nil {
			rightSort = *right.row.LastHitAt
		}
		if !leftSort.Equal(rightSort) {
			return rightSort.Compare(leftSort)
		}
		return cmp.Compare(left.row.SiteDomain, right.row.SiteDomain)
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

func (m *TenantStoreManager) loadActivationMetadata(ctx context.Context) ([]activationSiteMetadata, error) {
	rows, err := m.shared.DB().QueryContext(ctx, `
		SELECT
			t.id, t.name,
			COALESCE((SELECT MIN(u.email)
				FROM tenant_members tm
				JOIN users u ON u.id = tm.user_id
				WHERE tm.tenant_id = t.id AND tm.role = 'owner'), ''),
			COALESCE(cba.plan_code, ''), COALESCE(cba.plan_name, ''),
			s.id, s.domain, s.created_at, st.tenant_id
		FROM sites s
		JOIN site_tenants st ON st.site_id = s.id
		JOIN tenants t ON t.id = st.tenant_id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
		LEFT JOIN cloud_billing_accounts cba ON cba.tenant_id = t.id
		WHERE ta.tenant_id IS NULL
		ORDER BY s.created_at DESC, s.domain ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query activation metadata: %w", err)
	}
	defer rows.Close()
	result := make([]activationSiteMetadata, 0)
	for rows.Next() {
		var entry activationSiteMetadata
		if err := rows.Scan(
			&entry.row.TeamID, &entry.row.TeamName, &entry.row.OwnerEmail,
			&entry.row.PlanCode, &entry.row.PlanName,
			&entry.row.SiteID, &entry.row.SiteDomain, &entry.createdAt, &entry.tenantID,
		); err != nil {
			return nil, fmt.Errorf("scan activation metadata: %w", err)
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read activation metadata: %w", err)
	}
	return result, nil
}
