package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
)

// GetInstanceRole returns the user's instance-level role
func (s *Store) GetInstanceRole(ctx context.Context, userID uuid.UUID) (auth.InstanceRole, error) {
	if cached, ok := s.getCachedInstanceRole(userID); ok {
		return cached, nil
	}

	var role string
	err := s.db.QueryRowContext(ctx,
		"SELECT role FROM instance_roles WHERE user_id = ?",
		userID,
	).Scan(&role)

	if err == sql.ErrNoRows {
		s.cacheInstanceRole(userID, auth.InstanceUser)
		return auth.InstanceUser, nil // Default role
	}
	if err != nil {
		return "", fmt.Errorf("failed to get instance role: %w", err)
	}

	resolved := auth.InstanceRole(role)
	s.cacheInstanceRole(userID, resolved)
	return resolved, nil
}

// UpdateInstanceRole updates a user's instance-level role
func (s *Store) UpdateInstanceRole(ctx context.Context, targetUserID uuid.UUID, role auth.InstanceRole, actorID uuid.UUID) error {
	var oldRole string
	err := s.db.QueryRowContext(ctx, "SELECT role FROM instance_roles WHERE user_id = ?", targetUserID).Scan(&oldRole)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO instance_roles (user_id, role, granted_by)
		 VALUES (?, ?, ?)
		 ON CONFLICT (user_id) DO UPDATE SET role = excluded.role, granted_by = excluded.granted_by, granted_at = NOW()`,
		targetUserID, role, actorID,
	)
	if err != nil {
		return fmt.Errorf("failed to update instance role: %w", err)
	}
	s.invalidateInstanceRole(targetUserID)

	// Audit log
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO permission_audit (actor_id, action, resource_type, target_user_id, old_role, new_role)
		 VALUES (?, 'update', 'instance', ?, ?, ?)`,
		actorID, targetUserID, oldRole, role,
	)

	return nil
}

// GetSiteRole returns the user's role for a specific site
func (s *Store) GetSiteRole(ctx context.Context, userID uuid.UUID, siteID uuid.UUID) (auth.SiteRole, error) {
	if cached, ok := s.getCachedSiteRole(userID, siteID); ok {
		return cached, nil
	}

	activeTenantID, err := s.GetActiveTenantID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve active tenant: %w", err)
	}
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to resolve default tenant: %w", err)
	}

	var role string
	err = s.db.QueryRowContext(ctx, `
		SELECT sm.role
		FROM site_members sm
		JOIN sites s ON s.id = sm.site_id
		LEFT JOIN site_tenants st ON st.site_id = s.id
		WHERE sm.user_id = ?
			AND sm.site_id = ?
			AND COALESCE(st.tenant_id, ?) = ?
	`,
		userID, siteID, defaultTenantID, activeTenantID,
	).Scan(&role)

	if err == sql.ErrNoRows {
		// Check if user is site owner (backward compatibility)
		var ownerID uuid.UUID
		err2 := s.db.QueryRowContext(ctx, `
			SELECT user_id
			FROM sites s
			LEFT JOIN site_tenants st ON st.site_id = s.id
			WHERE s.id = ?
				AND COALESCE(st.tenant_id, ?) = ?
		`,
			siteID, defaultTenantID, activeTenantID,
		).Scan(&ownerID)

		if err2 == nil && ownerID == userID {
			s.cacheSiteRole(userID, siteID, auth.SiteOwner)
			return auth.SiteOwner, nil
		}

		return "", fmt.Errorf("no access to site")
	}
	if err != nil {
		return "", fmt.Errorf("failed to get site role: %w", err)
	}

	resolved := auth.SiteRole(role)
	s.cacheSiteRole(userID, siteID, resolved)
	return resolved, nil
}

// AddSiteMember grants a user access to a site
func (s *Store) AddSiteMember(ctx context.Context, siteID uuid.UUID, userID uuid.UUID, role auth.SiteRole, addedBy uuid.UUID) error {
	siteTenantID, err := s.GetSiteTenantID(ctx, siteID)
	if err != nil {
		return fmt.Errorf("failed to resolve site tenant: %w", err)
	}

	isMember, err := s.IsTenantMember(ctx, siteTenantID, userID)
	if err != nil {
		return fmt.Errorf("failed to check tenant membership: %w", err)
	}
	if !isMember {
		return fmt.Errorf("failed to add site member: user is not part of tenant")
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO site_members (site_id, user_id, role, added_by)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (site_id, user_id) DO UPDATE SET role = excluded.role`,
		siteID, userID, role, addedBy,
	)

	if err != nil {
		return fmt.Errorf("failed to add site member: %w", err)
	}
	s.invalidateSiteRole(userID, siteID)

	// Audit log
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO permission_audit (actor_id, action, resource_type, resource_id, target_user_id, new_role)
		 VALUES (?, 'grant', 'site', ?, ?, ?)`,
		addedBy, siteID, userID, role,
	)

	return nil
}

// RemoveSiteMember revokes a user's access to a site
func (s *Store) RemoveSiteMember(ctx context.Context, siteID uuid.UUID, userID uuid.UUID, removedBy uuid.UUID) error {
	var oldRole string
	err := s.db.QueryRowContext(ctx,
		"SELECT role FROM site_members WHERE site_id = ? AND user_id = ?",
		siteID, userID,
	).Scan(&oldRole)

	if err != nil {
		return fmt.Errorf("member not found")
	}

	_, err = s.db.ExecContext(ctx,
		"DELETE FROM site_members WHERE site_id = ? AND user_id = ?",
		siteID, userID,
	)

	if err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}
	s.invalidateSiteRole(userID, siteID)

	// Audit log
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO permission_audit (actor_id, action, resource_type, resource_id, target_user_id, old_role)
		 VALUES (?, 'revoke', 'site', ?, ?, ?)`,
		removedBy, siteID, userID, oldRole,
	)

	return nil
}

// GetSiteMembers lists all members of a site
func (s *Store) GetSiteMembers(ctx context.Context, siteID uuid.UUID) ([]api.SiteMember, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT sm.id, sm.user_id, u.email, sm.role, sm.added_at
		 FROM site_members sm
		 JOIN users u ON sm.user_id = u.id
		 WHERE sm.site_id = ?
		 ORDER BY sm.added_at ASC`,
		siteID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := []api.SiteMember{}
	for rows.Next() {
		var m api.SiteMember
		if err := rows.Scan(&m.ID, &m.UserID, &m.Email, &m.Role, &m.AddedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read site member rows: %w", err)
	}

	return members, nil
}

// CountSiteOwners counts how many owners a site has
func (s *Store) CountSiteOwners(ctx context.Context, siteID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM site_members WHERE site_id = ? AND role = 'owner'",
		siteID,
	).Scan(&count)
	if err != nil {
		return 0, err
	}

	// Also check the legacy owner column in sites table
	var legacyOwnerID uuid.UUID
	err = s.db.QueryRowContext(ctx, "SELECT user_id FROM sites WHERE id = ?", siteID).Scan(&legacyOwnerID)
	if err == nil {
		// Check if this legacy owner is already counted in site_members
		var exists int
		err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM site_members WHERE site_id = ? AND user_id = ? AND role = 'owner'", siteID, legacyOwnerID).Scan(&exists)
		if err == nil && exists == 0 {
			count++
		}
	}

	return count, nil
}
