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
)

const (
	defaultTenantName = "Default Tenant"
	defaultLocaleCode = "en"

	TenantRoleOwner  = "owner"
	TenantRoleAdmin  = "admin"
	TenantRoleMember = "member"
)

var ErrTenantMembershipRequired = errors.New("tenant membership required")
var ErrTeamLastOwner = errors.New("team must retain at least one owner")
var ErrUserOnlyTeam = errors.New("user cannot leave their only team")
var ErrTeamInviteAlreadyPending = errors.New("team invite already pending")
var ErrTeamInviteNotFound = errors.New("team invite not found")
var ErrManagedCloudSingleTeamLimit = errors.New("managed cloud accounts are limited to one team")
var ErrTeamTransferRequiresOwner = errors.New("team ownership transfer requires owner")
var ErrTeamTransferSelf = errors.New("cannot transfer ownership to self")
var ErrTeamTransferTargetNotMember = errors.New("ownership transfer target must be a team member")
var ErrTeamTransferTargetAlreadyOwner = errors.New("ownership transfer target is already owner")
var ErrTeamArchiveRequiresOwner = errors.New("team archive requires owner")
var ErrTeamArchiveDefaultTenant = errors.New("default team cannot be archived")
var ErrTeamArchiveHasSites = errors.New("team archive requires empty site list")
var ErrTeamPurgeDefaultTenant = errors.New("default team cannot be purged")
var ErrTeamPurgeNotArchived = errors.New("team must be archived before purge")
var ErrTeamPurgeHasSites = errors.New("team purge requires empty site list")

const (
	TeamInviteStatusPending  = "pending"
	TeamInviteStatusAccepted = "accepted"
	TeamInviteStatusRevoked  = "revoked"
)

type tenantRowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func removeUserTenantScopedSiteAccessTx(ctx context.Context, tx *sql.Tx, tenantID, userID uuid.UUID) error {
	defaultTenantID, err := getDefaultTenantID(ctx, tx)
	if err != nil {
		return fmt.Errorf("could not resolve default tenant: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM site_members
		WHERE user_id = ?
		  AND site_id IN (
			SELECT s.id
			FROM sites s
			LEFT JOIN site_tenants st ON st.site_id = s.id
			WHERE COALESCE(st.tenant_id, ?) = ?
		  )
	`, userID, defaultTenantID, tenantID); err != nil {
		return fmt.Errorf("could not remove tenant-scoped site memberships: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM site_report_subscriptions
		WHERE user_id = ?
		  AND site_id IN (
			SELECT s.id
			FROM sites s
			LEFT JOIN site_tenants st ON st.site_id = s.id
			WHERE COALESCE(st.tenant_id, ?) = ?
		  )
	`, userID, defaultTenantID, tenantID); err != nil {
		return fmt.Errorf("could not remove tenant-scoped report subscriptions: %w", err)
	}

	return nil
}

func (s *Store) GetPrimaryTenantID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return getPrimaryTenantID(ctx, s.db, userID)
}

func (s *Store) GetActiveTenantID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return getActiveTenantID(ctx, s.db, userID)
}

func (s *Store) GetDefaultTenantID(ctx context.Context) (uuid.UUID, error) {
	return getDefaultTenantID(ctx, s.db)
}

func (s *Store) GetSiteTenantID(ctx context.Context, siteID uuid.UUID) (uuid.UUID, error) {
	var tenantIDRaw sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT CAST(st.tenant_id AS VARCHAR)
		FROM sites s
		LEFT JOIN site_tenants st ON st.site_id = s.id
		WHERE s.id = ?
	`, siteID).Scan(&tenantIDRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("site not found")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("could not query site tenant: %w", err)
	}

	if !tenantIDRaw.Valid || strings.TrimSpace(tenantIDRaw.String) == "" {
		return getDefaultTenantID(ctx, s.db)
	}

	tenantID, err := uuid.Parse(tenantIDRaw.String)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid tenant id %q: %w", tenantIDRaw.String, err)
	}
	return tenantID, nil
}

func (s *Store) IsTenantMember(ctx context.Context, tenantID, userID uuid.UUID) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*)
		FROM tenant_members tm
		JOIN tenants t ON t.id = tm.tenant_id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
		WHERE tm.tenant_id = ? AND tm.user_id = ? AND ta.tenant_id IS NULL`,
		tenantID, userID,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("could not query tenant membership: %w", err)
	}
	return count > 0, nil
}

func (s *Store) GetTenantRole(ctx context.Context, tenantID, userID uuid.UUID) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT tm.role
		FROM tenant_members tm
		JOIN tenants t ON t.id = tm.tenant_id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
		WHERE tm.tenant_id = ? AND tm.user_id = ? AND ta.tenant_id IS NULL`,
		tenantID, userID,
	).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("no access to tenant")
	}
	if err != nil {
		return "", fmt.Errorf("could not query tenant role: %w", err)
	}
	return strings.TrimSpace(role), nil
}

func (s *Store) SetActiveTenantID(ctx context.Context, userID, tenantID uuid.UUID) error {
	isMember, err := s.IsTenantMember(ctx, tenantID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return fmt.Errorf("could not set active tenant: %w", ErrTenantMembershipRequired)
	}

	var currentLocale sql.NullString
	if err := s.QueryRowOrNil(ctx, "SELECT default_locale FROM user_preferences WHERE user_id = ?", []any{&currentLocale}, userID); err != nil {
		return fmt.Errorf("could not query user preferences: %w", err)
	}

	locale := strings.TrimSpace(currentLocale.String)
	if locale == "" {
		locale = defaultLocaleCode
	}
	now := time.Now().UTC()
	err = s.Exec(ctx, `
		INSERT INTO user_preferences (user_id, default_locale, updated_at, active_tenant_id)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			active_tenant_id = excluded.active_tenant_id,
			updated_at = excluded.updated_at
	`, userID, locale, now, tenantID)
	if err != nil {
		return fmt.Errorf("could not set active tenant: %w", err)
	}
	s.invalidateAllSiteRolesForUser(userID)
	return nil
}

func (s *Store) ListUserTeams(ctx context.Context, userID uuid.UUID) ([]api.Team, uuid.UUID, error) {
	activeTenantID, err := s.GetActiveTenantID(ctx, userID)
	if err != nil {
		return nil, uuid.Nil, fmt.Errorf("could not resolve active team: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.name, COALESCE(t.logo_url, ''), tm.role, t.created_at
		FROM tenants t
		JOIN tenant_members tm ON tm.tenant_id = t.id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
		WHERE tm.user_id = ?
			AND ta.tenant_id IS NULL
		ORDER BY
			CASE tm.role
				WHEN 'owner' THEN 0
				WHEN 'admin' THEN 1
				ELSE 2
			END ASC,
			t.created_at ASC
	`, userID)
	if err != nil {
		return nil, uuid.Nil, fmt.Errorf("could not list teams: %w", err)
	}
	defer rows.Close()

	teams := make([]api.Team, 0)
	for rows.Next() {
		var team api.Team
		if err := rows.Scan(&team.ID, &team.Name, &team.LogoURL, &team.Role, &team.CreatedAt); err != nil {
			return nil, uuid.Nil, fmt.Errorf("could not scan team: %w", err)
		}
		teams = append(teams, team)
	}
	if err := rows.Err(); err != nil {
		return nil, uuid.Nil, fmt.Errorf("could not read team rows: %w", err)
	}

	return teams, activeTenantID, nil
}

func (s *Store) CountUserNonDefaultTeams(ctx context.Context, userID uuid.UUID) (int, error) {
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return 0, err
	}

	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM tenant_members tm
		JOIN tenants t ON t.id = tm.tenant_id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
		WHERE tm.user_id = ?
		  AND tm.tenant_id <> ?
		  AND ta.tenant_id IS NULL
	`, userID, defaultTenantID).Scan(&count); err != nil {
		return 0, fmt.Errorf("could not count user teams: %w", err)
	}

	return count, nil
}
