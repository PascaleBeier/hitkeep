package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type reportAccessibleSite struct {
	SiteID uuid.UUID
	Domain string
}

func (s *Store) listReportAccessibleSites(ctx context.Context, userID uuid.UUID) ([]reportAccessibleSite, error) {
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT s.id, s.domain
		FROM sites s
		LEFT JOIN site_tenants st ON st.site_id = s.id
		JOIN tenant_members tm ON tm.tenant_id = COALESCE(st.tenant_id, ?) AND tm.user_id = ?
		LEFT JOIN tenant_archives ta ON ta.tenant_id = tm.tenant_id
		LEFT JOIN site_members sm ON sm.site_id = s.id AND sm.user_id = ?
		WHERE ta.tenant_id IS NULL
		  AND (s.user_id = ? OR sm.user_id IS NOT NULL)
		ORDER BY s.domain ASC
	`, defaultTenantID, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sites := make([]reportAccessibleSite, 0)
	for rows.Next() {
		var site reportAccessibleSite
		if err := rows.Scan(&site.SiteID, &site.Domain); err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sites, nil
}

func (s *Store) CanAccessSiteForReports(ctx context.Context, userID, siteID uuid.UUID) (bool, error) {
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return false, err
	}

	var count int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sites s
		LEFT JOIN site_tenants st ON st.site_id = s.id
		JOIN tenant_members tm ON tm.tenant_id = COALESCE(st.tenant_id, ?) AND tm.user_id = ?
		LEFT JOIN tenant_archives ta ON ta.tenant_id = tm.tenant_id
		LEFT JOIN site_members sm ON sm.site_id = s.id AND sm.user_id = ?
		WHERE s.id = ?
		  AND ta.tenant_id IS NULL
		  AND (s.user_id = ? OR sm.user_id IS NOT NULL)
	`, defaultTenantID, userID, userID, siteID, userID).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetDailyPageviewsForPeriod returns daily pageview counts from hit_rollups_daily
// for the given site over [start, end), ordered oldest-first.
func (s *Store) GetDailyPageviewsForPeriod(ctx context.Context, siteID uuid.UUID, start, end time.Time) ([]int, error) {
	refreshEnd := end
	if end.After(start) {
		refreshEnd = end.Add(-time.Nanosecond)
	}
	if err := s.refreshDirtyRollupsInRange(ctx, siteID, dirtyRollupHit, rollupDaily, start, refreshEnd); err != nil {
		return nil, fmt.Errorf("refresh daily hit rollups: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT pageviews
		FROM hit_rollups_daily
		WHERE site_id = ?
		  AND bucket >= ? AND bucket < ?
		ORDER BY bucket ASC
	`, siteID, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("query daily pageviews: %w", err)
	}
	defer rows.Close()

	var result []int
	for rows.Next() {
		var pageviews int
		if err := rows.Scan(&pageviews); err != nil {
			return nil, fmt.Errorf("scan daily pageviews: %w", err)
		}
		result = append(result, pageviews)
	}
	return result, rows.Err()
}
