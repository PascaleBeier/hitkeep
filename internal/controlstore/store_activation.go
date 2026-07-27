package controlstore

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

type ActivationSiteMetadata struct {
	Row       api.SystemActivationRow
	TenantID  uuid.UUID
	CreatedAt time.Time
}

func (s *Store) ListActivationMetadata(ctx context.Context) ([]ActivationSiteMetadata, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.name,
			COALESCE((SELECT MIN(u.email) FROM tenant_members tm JOIN users u ON u.id = tm.user_id
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
	var result []ActivationSiteMetadata
	for rows.Next() {
		var entry ActivationSiteMetadata
		if err := rows.Scan(
			&entry.Row.TeamID, &entry.Row.TeamName, &entry.Row.OwnerEmail,
			&entry.Row.PlanCode, &entry.Row.PlanName,
			&entry.Row.SiteID, &entry.Row.SiteDomain, &entry.CreatedAt, &entry.TenantID,
		); err != nil {
			return nil, fmt.Errorf("scan activation metadata: %w", err)
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}
