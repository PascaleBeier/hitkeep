package controlstore

import (
	"context"

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
	var sites []reportAccessibleSite
	for rows.Next() {
		var site reportAccessibleSite
		if err := rows.Scan(&site.SiteID, &site.Domain); err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	return sites, rows.Err()
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
		WHERE s.id = ? AND ta.tenant_id IS NULL
		  AND (s.user_id = ? OR sm.user_id IS NOT NULL)
	`, defaultTenantID, userID, userID, siteID, userID).Scan(&count)
	return count > 0, err
}
