package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
)

func (s *Store) FindSiteByDomain(ctx context.Context, domain string) (*api.Site, error) {
	var site api.Site
	err := s.db.QueryRowContext(ctx, "SELECT id, user_id, domain, created_at FROM sites WHERE domain = ?", domain).Scan(&site.ID, &site.UserID, &site.Domain, &site.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("could not query for site: %w", err)
	}
	return &site, nil
}

func (s *Store) GetSiteByID(ctx context.Context, siteID uuid.UUID) (*api.Site, error) {
	var site api.Site
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, domain, data_retention_days, created_at
		FROM sites
		WHERE id = ?`,
		siteID,
	).Scan(&site.ID, &site.UserID, &site.Domain, &site.DataRetentionDays, &site.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not query site by id: %w", err)
	}
	return &site, nil
}

func (s *Store) GetSite(ctx context.Context, siteID uuid.UUID, userID uuid.UUID) (*api.Site, error) {
	activeTenantID, err := s.GetActiveTenantID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("could not resolve active tenant: %w", err)
	}
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not resolve default tenant: %w", err)
	}

	var site api.Site
	err = s.db.QueryRowContext(ctx, `
		SELECT id, user_id, domain, created_at
		FROM sites s
		LEFT JOIN site_tenants st ON st.site_id = s.id
		WHERE s.id = ?
			AND s.user_id = ?
			AND COALESCE(st.tenant_id, ?) = ?
	`, siteID, userID, defaultTenantID, activeTenantID).Scan(&site.ID, &site.UserID, &site.Domain, &site.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("could not query for site: %w", err)
	}
	return &site, nil
}

func (s *Store) CreateSite(ctx context.Context, userID uuid.UUID, domain string) (*api.Site, error) {
	id := uuid.New()
	now := time.Now()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := ensureDefaultTenantTx(ctx, tx, defaultTenantName, false); err != nil {
		return nil, err
	}

	tenantID, err := getActiveTenantID(ctx, tx, userID)
	if err != nil {
		return nil, fmt.Errorf("could not resolve site tenant: %w", err)
	}

	if err := ensureTenantMemberTx(ctx, tx, tenantID, userID, TenantRoleOwner, userID); err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx,
		"INSERT INTO sites (id, user_id, domain, created_at) VALUES (?, ?, ?, ?)",
		id, userID, domain, now,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create site: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		"INSERT INTO site_tenants (site_id, tenant_id, created_at) VALUES (?, ?, ?)",
		id, tenantID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create site tenant mapping: %w", err)
	}

	// Add creator as site owner
	_, err = tx.ExecContext(ctx,
		"INSERT INTO site_members (site_id, user_id, role, added_by) VALUES (?, ?, 'owner', ?)",
		id, userID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("could not add site owner: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return &api.Site{
		ID:        id,
		UserID:    userID,
		Domain:    domain,
		CreatedAt: now,
	}, nil
}

func (s *Store) UpdateSiteTenant(ctx context.Context, siteID, tenantID uuid.UUID) error {
	return updateSiteTenant(ctx, s.db, siteID, tenantID)
}

func updateSiteTenant(ctx context.Context, exec sqlExecContext, siteID, tenantID uuid.UUID) error {
	now := time.Now().UTC()
	_, err := exec.ExecContext(ctx, `
		INSERT INTO site_tenants (site_id, tenant_id, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT (site_id) DO UPDATE SET
			tenant_id = excluded.tenant_id
	`, siteID, tenantID, now)
	if err != nil {
		return fmt.Errorf("could not update site tenant mapping: %w", err)
	}
	return nil
}

func (s *Store) GetSites(ctx context.Context, userID uuid.UUID) ([]api.Site, error) {
	instanceRole, err := s.GetInstanceRole(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("could not get instance role: %w", err)
	}
	activeTenantID, err := s.GetActiveTenantID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("could not resolve active tenant: %w", err)
	}
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not resolve default tenant: %w", err)
	}

	canManageActiveTenantSites := false
	if tenantRole, err := s.GetTenantRole(ctx, activeTenantID, userID); err == nil {
		_, canManageActiveTenantSites = tenantRoleSiteRole(tenantRole)
	}

	var rows *sql.Rows
	if instanceRole.HasPermission(auth.PermInstanceViewAllSites) || canManageActiveTenantSites {
		rows, err = s.db.QueryContext(ctx, `
			SELECT s.id, s.user_id, s.domain, s.data_retention_days, s.created_at
			FROM sites s
			LEFT JOIN site_tenants st ON st.site_id = s.id
			WHERE COALESCE(st.tenant_id, ?) = ?
			ORDER BY s.created_at DESC
		`, defaultTenantID, activeTenantID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT DISTINCT s.id, s.user_id, s.domain, s.data_retention_days, s.created_at
			FROM sites s
			LEFT JOIN site_tenants st ON st.site_id = s.id
			LEFT JOIN site_members sm ON sm.site_id = s.id AND sm.user_id = ?
			WHERE COALESCE(st.tenant_id, ?) = ?
				AND (s.user_id = ? OR sm.user_id IS NOT NULL)
			ORDER BY s.created_at DESC
		`,
			userID, defaultTenantID, activeTenantID, userID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sites := []api.Site{}
	for rows.Next() {
		var site api.Site
		if err := rows.Scan(&site.ID, &site.UserID, &site.Domain, &site.DataRetentionDays, &site.CreatedAt); err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read site rows: %w", err)
	}

	return sites, nil
}

func (s *Store) ListSitesForTenant(ctx context.Context, tenantID uuid.UUID) ([]api.Site, error) {
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not resolve default tenant: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.user_id, s.domain, s.data_retention_days, s.created_at
		FROM sites s
		LEFT JOIN site_tenants st ON st.site_id = s.id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = COALESCE(st.tenant_id, ?)
		WHERE COALESCE(st.tenant_id, ?) = ?
			AND ta.tenant_id IS NULL
		ORDER BY s.created_at DESC
	`, defaultTenantID, defaultTenantID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("could not list tenant sites: %w", err)
	}
	defer rows.Close()

	sites := make([]api.Site, 0)
	for rows.Next() {
		var site api.Site
		if err := rows.Scan(&site.ID, &site.UserID, &site.Domain, &site.DataRetentionDays, &site.CreatedAt); err != nil {
			return nil, fmt.Errorf("could not scan tenant site: %w", err)
		}
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read tenant sites: %w", err)
	}

	return sites, nil
}

func (s *Store) UpdateSiteRetention(ctx context.Context, siteID uuid.UUID, userID uuid.UUID, days int) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE sites SET data_retention_days = ? WHERE id = ? AND user_id = ?",
		days, siteID, userID,
	)
	if err != nil {
		return fmt.Errorf("could not update site retention: %w", err)
	}
	return nil
}

func (s *Store) UpsertSiteMirror(ctx context.Context, site *api.Site) error {
	if site == nil {
		return fmt.Errorf("site is required")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sites (id, domain, data_retention_days)
		VALUES (?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			domain = excluded.domain,
			data_retention_days = excluded.data_retention_days`,
		site.ID, site.Domain, site.DataRetentionDays,
	)
	if err != nil {
		return fmt.Errorf("could not upsert site mirror: %w", err)
	}
	return nil
}

func (s *Store) ListAllSites(ctx context.Context) ([]api.Site, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT s.id, s.user_id, s.domain, s.created_at, COALESCE(u.email, '') AS owner_email
		 FROM sites s
		 LEFT JOIN users u ON u.id = s.user_id
		 ORDER BY s.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sites := []api.Site{}
	for rows.Next() {
		var site api.Site
		if err := rows.Scan(&site.ID, &site.UserID, &site.Domain, &site.CreatedAt, &site.OwnerEmail); err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read all site rows: %w", err)
	}

	return sites, nil
}

func (s *Store) DeleteSite(ctx context.Context, siteID uuid.UUID) error {
	if err := s.deleteSiteData(ctx, siteID); err != nil {
		return err
	}
	return s.deleteSiteRow(ctx, siteID)
}

func (s *Store) deleteSiteData(ctx context.Context, siteID uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := deleteSiteChildren(ctx, tx, siteID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}
	return nil
}

func (s *Store) deleteSiteRow(ctx context.Context, siteID uuid.UUID) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sites WHERE id = ?", siteID); err != nil {
		refs, refErr := findSiteReferences(ctx, s.db, siteID)
		if refErr != nil {
			return fmt.Errorf("could not delete site: %w (failed to resolve references: %v)", err, refErr)
		}
		if len(refs) > 0 {
			return fmt.Errorf("could not delete site: %w (still referenced by: %s)", err, strings.Join(refs, ", "))
		}
		return fmt.Errorf("could not delete site: %w", err)
	}
	return nil
}
