package controlstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func (s *Store) GetSiteCount(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sites").Scan(&count); err != nil {
		return 0, fmt.Errorf("could not count sites: %w", err)
	}
	return count, nil
}

func (s *Store) GetTenantList(ctx context.Context) ([]api.TenantDBInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.is_default FROM tenants t
		LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
		WHERE ta.tenant_id IS NULL ORDER BY t.name
	`)
	if err != nil {
		return nil, fmt.Errorf("could not list tenants: %w", err)
	}
	defer rows.Close()
	var tenants []api.TenantDBInfo
	for rows.Next() {
		var rawID, name string
		var isDefault bool
		if err := rows.Scan(&rawID, &name, &isDefault); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(rawID)
		if err != nil {
			return nil, fmt.Errorf("invalid tenant id: %w", err)
		}
		tenants = append(tenants, api.TenantDBInfo{TenantID: id, Name: name, IsDefault: isDefault})
	}
	return tenants, rows.Err()
}
