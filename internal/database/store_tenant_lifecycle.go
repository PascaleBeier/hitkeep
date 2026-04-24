package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func (s *Store) CountTeamSites(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM site_tenants WHERE tenant_id = ?",
		tenantID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("could not count tenant sites: %w", err)
	}
	return count, nil
}

func (s *Store) ArchiveTenant(ctx context.Context, tenantID, actorID uuid.UUID) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
		var isDefault bool
		err := tx.QueryRowContext(ctx, `
			SELECT t.is_default
			FROM tenants t
			LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
			WHERE t.id = ? AND ta.tenant_id IS NULL`,
			tenantID,
		).Scan(&isDefault)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTenantMembershipRequired
		}
		if err != nil {
			return fmt.Errorf("could not resolve team: %w", err)
		}
		if isDefault {
			return ErrTeamArchiveDefaultTenant
		}

		var actorRole string
		if err := tx.QueryRowContext(ctx,
			`SELECT tm.role
			FROM tenant_members tm
			JOIN tenants t ON t.id = tm.tenant_id
			LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
			WHERE tm.tenant_id = ? AND tm.user_id = ? AND ta.tenant_id IS NULL`,
			tenantID, actorID,
		).Scan(&actorRole); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrTenantMembershipRequired
			}
			return fmt.Errorf("could not resolve actor team role: %w", err)
		}
		if actorRole != TenantRoleOwner {
			return ErrTeamArchiveRequiresOwner
		}

		var siteCount int
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM site_tenants WHERE tenant_id = ?",
			tenantID,
		).Scan(&siteCount); err != nil {
			return fmt.Errorf("could not count tenant sites: %w", err)
		}
		if siteCount > 0 {
			return ErrTeamArchiveHasSites
		}

		defaultTenantID, err := getDefaultTenantID(ctx, tx)
		if err != nil {
			return err
		}

		memberIDs, err := archivedTenantMemberIDs(ctx, tx, tenantID, "archive")
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		for _, memberID := range memberIDs {
			if err := ensureTenantMemberTx(ctx, tx, defaultTenantID, memberID, TenantRoleMember, actorID); err != nil {
				return err
			}
		}

		if _, err := tx.ExecContext(ctx,
			"INSERT INTO tenant_archives (tenant_id, archived_at, archived_by) VALUES (?, ?, ?)",
			tenantID, now, nullableUUID(actorID),
		); err != nil {
			return fmt.Errorf("could not archive tenant: %w", err)
		}

		for _, memberID := range memberIDs {
			currentActiveTenantID, err := getActiveTenantID(ctx, tx, memberID)
			if err != nil {
				currentActiveTenantID = uuid.Nil
			}
			if currentActiveTenantID != tenantID {
				continue
			}

			replacementTenantID, err := getPrimaryTenantID(ctx, tx, memberID)
			if err != nil {
				return fmt.Errorf("could not resolve replacement team after archive: %w", err)
			}
			locale, err := getUserLocaleTx(ctx, tx, memberID)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO user_preferences (user_id, default_locale, updated_at, active_tenant_id)
				VALUES (?, ?, ?, ?)
				ON CONFLICT (user_id) DO UPDATE SET
					active_tenant_id = excluded.active_tenant_id,
					updated_at = excluded.updated_at
			`, memberID, locale, now, replacementTenantID); err != nil {
				return fmt.Errorf("could not update active team after archive: %w", err)
			}
		}

		return nil
	})
}

// ListAllTeams returns every tenant with member/site counts and archive status.
// Intended for instance-admin views.
func (s *Store) ListAllTeams(ctx context.Context) ([]api.AdminTeam, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.is_default, t.created_at,
		       (SELECT COUNT(*) FROM tenant_members tm WHERE tm.tenant_id = t.id) AS member_count,
		       (SELECT COUNT(*) FROM site_tenants st WHERE st.tenant_id = t.id) AS site_count,
		       CASE WHEN ta.tenant_id IS NOT NULL THEN 1 ELSE 0 END AS is_archived
		FROM tenants t
		LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
		ORDER BY t.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list all teams: %w", err)
	}
	defer rows.Close()

	teams := []api.AdminTeam{}
	for rows.Next() {
		var team api.AdminTeam
		var isArchived int
		if err := rows.Scan(&team.ID, &team.Name, &team.IsDefault, &team.CreatedAt,
			&team.MemberCount, &team.SiteCount, &isArchived); err != nil {
			return nil, fmt.Errorf("scan admin team: %w", err)
		}
		team.IsArchived = isArchived == 1
		teams = append(teams, team)
	}
	return teams, rows.Err()
}

// AdminArchiveTenant archives a tenant without checking actor ownership.
// Sites must be deleted before calling this. Intended for instance-admin
// force-delete flows where the admin is not a member of the target team.
func (s *Store) AdminArchiveTenant(ctx context.Context, tenantID, actorID uuid.UUID) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
		var isDefault bool
		err := tx.QueryRowContext(ctx, `
			SELECT t.is_default
			FROM tenants t
			LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
			WHERE t.id = ? AND ta.tenant_id IS NULL`,
			tenantID,
		).Scan(&isDefault)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTenantMembershipRequired
		}
		if err != nil {
			return fmt.Errorf("could not resolve team: %w", err)
		}
		if isDefault {
			return ErrTeamArchiveDefaultTenant
		}

		defaultTenantID, err := getDefaultTenantID(ctx, tx)
		if err != nil {
			return err
		}

		memberIDs, err := archivedTenantMemberIDs(ctx, tx, tenantID, "admin archive")
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		for _, memberID := range memberIDs {
			if err := ensureTenantMemberTx(ctx, tx, defaultTenantID, memberID, TenantRoleMember, actorID); err != nil {
				return err
			}
		}

		if _, err := tx.ExecContext(ctx,
			"INSERT INTO tenant_archives (tenant_id, archived_at, archived_by) VALUES (?, ?, ?)",
			tenantID, now, nullableUUID(actorID),
		); err != nil {
			return fmt.Errorf("could not archive tenant: %w", err)
		}

		for _, memberID := range memberIDs {
			currentActiveTenantID, err := getActiveTenantID(ctx, tx, memberID)
			if err != nil {
				currentActiveTenantID = uuid.Nil
			}
			if currentActiveTenantID != tenantID {
				continue
			}

			replacementTenantID, err := getPrimaryTenantID(ctx, tx, memberID)
			if err != nil {
				return fmt.Errorf("could not resolve replacement team after archive: %w", err)
			}
			locale, err := getUserLocaleTx(ctx, tx, memberID)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO user_preferences (user_id, default_locale, updated_at, active_tenant_id)
				VALUES (?, ?, ?, ?)
				ON CONFLICT (user_id) DO UPDATE SET
					active_tenant_id = excluded.active_tenant_id,
					updated_at = excluded.updated_at
			`, memberID, locale, now, replacementTenantID); err != nil {
				return fmt.Errorf("could not update active team after archive: %w", err)
			}
		}

		return nil
	})
}

func (s *Store) GetPurgeableTenant(ctx context.Context, tenantID uuid.UUID) (*api.Team, error) {
	team, err := s.getPurgeableTenant(ctx, s.db, tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return team, nil
}

func (s *Store) DeleteArchivedTenantMetadata(ctx context.Context, tenantID uuid.UUID) (*api.Team, error) {
	deleted, err := s.GetPurgeableTenant(ctx, tenantID)
	if err != nil || deleted == nil {
		return deleted, err
	}

	if err := s.Transact(ctx, func(tx *sql.Tx) error {
		tables, err := listTables(ctx, tx)
		if err != nil {
			return err
		}
		execIfTableExists := func(table string, query string, args ...any) error {
			if _, ok := tables[table]; !ok {
				return nil
			}
			if _, err := tx.ExecContext(ctx, query, args...); err != nil {
				return err
			}
			return nil
		}

		statements := []struct {
			table string
			query string
			args  []any
		}{
			{table: "api_client_site_roles", query: "DELETE FROM api_client_site_roles WHERE api_client_id IN (SELECT id FROM api_clients WHERE tenant_id = ?)", args: []any{tenantID}},
			{table: "api_clients", query: "DELETE FROM api_clients WHERE tenant_id = ?", args: []any{tenantID}},
			{table: "cloud_billing_events", query: "DELETE FROM cloud_billing_events WHERE tenant_id = ?", args: []any{tenantID}},
			{table: "cloud_billing_accounts", query: "DELETE FROM cloud_billing_accounts WHERE tenant_id = ?", args: []any{tenantID}},
			{table: "team_audit_log", query: "DELETE FROM team_audit_log WHERE tenant_id = ?", args: []any{tenantID}},
			{table: "team_invites", query: "DELETE FROM team_invites WHERE tenant_id = ?", args: []any{tenantID}},
			{table: "tenant_members", query: "DELETE FROM tenant_members WHERE tenant_id = ?", args: []any{tenantID}},
			{table: "tenant_archives", query: "DELETE FROM tenant_archives WHERE tenant_id = ?", args: []any{tenantID}},
		}

		for _, stmt := range statements {
			if err := execIfTableExists(stmt.table, stmt.query, stmt.args...); err != nil {
				return fmt.Errorf("could not purge archived tenant metadata: %w", err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if _, err := s.db.ExecContext(ctx, "DELETE FROM tenants WHERE id = ?", tenantID); err != nil {
		return nil, fmt.Errorf("could not delete archived tenant row: %w", err)
	}

	return deleted, nil
}

func (s *Store) getPurgeableTenant(ctx context.Context, queryer tenantRowQueryer, tenantID uuid.UUID) (*api.Team, error) {
	var (
		team       api.Team
		isDefault  bool
		archivedAt sql.NullTime
	)

	err := queryer.QueryRowContext(ctx, `
		SELECT
			t.id,
			t.name,
			COALESCE(t.logo_url, ''),
			t.created_at,
			t.is_default,
			ta.archived_at
		FROM tenants t
		LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
		WHERE t.id = ?
		LIMIT 1
	`, tenantID).Scan(&team.ID, &team.Name, &team.LogoURL, &team.CreatedAt, &isDefault, &archivedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("could not query tenant purge state: %w", err)
	}

	if isDefault {
		return nil, ErrTeamPurgeDefaultTenant
	}
	if !archivedAt.Valid {
		return nil, ErrTeamPurgeNotArchived
	}

	var siteCount int
	if err := queryer.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM site_tenants WHERE tenant_id = ?",
		tenantID,
	).Scan(&siteCount); err != nil {
		return nil, fmt.Errorf("could not count tenant sites for purge: %w", err)
	}
	if siteCount > 0 {
		return nil, ErrTeamPurgeHasSites
	}

	return &team, nil
}
