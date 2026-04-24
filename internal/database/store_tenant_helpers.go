package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func defaultTenantNameForSetup(givenName string) string {
	givenName = strings.TrimSpace(givenName)
	if givenName == "" {
		return defaultTenantName
	}
	return fmt.Sprintf("%s's Team", givenName)
}

func ensureDefaultTenantTx(ctx context.Context, tx *sql.Tx, tenantName string, renameExisting bool) error {
	tenantName = strings.TrimSpace(tenantName)
	if tenantName == "" {
		tenantName = defaultTenantName
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO tenants (name, is_default)
		SELECT ?, TRUE
		WHERE NOT EXISTS (
			SELECT 1
			FROM tenants
			WHERE is_default = TRUE
		)
	`, tenantName)
	if err != nil {
		return fmt.Errorf("could not ensure default tenant: %w", err)
	}
	if !renameExisting || tenantName == defaultTenantName {
		return nil
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE tenants
		SET name = ?
		WHERE is_default = TRUE
			AND name = ?
	`, tenantName, defaultTenantName)
	if err != nil {
		return fmt.Errorf("could not rename default tenant: %w", err)
	}
	return nil
}

func archivedTenantMemberIDs(ctx context.Context, tx *sql.Tx, tenantID uuid.UUID, operation string) ([]uuid.UUID, error) {
	memberRows, err := tx.QueryContext(ctx,
		"SELECT CAST(user_id AS VARCHAR) FROM tenant_members WHERE tenant_id = ?",
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("could not query team members for %s: %w", operation, err)
	}
	defer memberRows.Close()

	memberIDs := make([]uuid.UUID, 0)
	for memberRows.Next() {
		var raw string
		if err := memberRows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("could not scan archived team member: %w", err)
		}

		memberID, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid archived team member id %q: %w", raw, err)
		}

		memberIDs = append(memberIDs, memberID)
	}
	if err := memberRows.Err(); err != nil {
		return nil, fmt.Errorf("could not read archived team members: %w", err)
	}

	return memberIDs, nil
}

func ensureTenantMemberTx(ctx context.Context, tx *sql.Tx, tenantID, userID uuid.UUID, role string, addedBy uuid.UUID) error {
	role = strings.TrimSpace(role)
	if role == "" {
		role = TenantRoleMember
	}

	var addedByValue any
	if addedBy != uuid.Nil {
		addedByValue = addedBy
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO tenant_members (tenant_id, user_id, role, added_by)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (tenant_id, user_id) DO NOTHING
	`, tenantID, userID, role, addedByValue)
	if err != nil {
		return fmt.Errorf("could not ensure tenant membership: %w", err)
	}
	return nil
}

func IsValidTenantRole(role string) bool {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case TenantRoleOwner, TenantRoleAdmin, TenantRoleMember:
		return true
	default:
		return false
	}
}

func CanAssignTenantRole(actorRole, requestedRole string) bool {
	if !IsValidTenantRole(actorRole) || !IsValidTenantRole(requestedRole) {
		return false
	}
	return tenantRoleRank(requestedRole) >= tenantRoleRank(actorRole)
}

func tenantRoleRank(role string) int {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case TenantRoleOwner:
		return 0
	case TenantRoleAdmin:
		return 1
	case TenantRoleMember:
		return 2
	default:
		return 99
	}
}

func getPrimaryTenantID(ctx context.Context, q tenantRowQueryer, userID uuid.UUID) (uuid.UUID, error) {
	var tenantIDRaw sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT CAST(tm.tenant_id AS VARCHAR)
		FROM tenant_members tm
		JOIN tenants t ON t.id = tm.tenant_id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
		WHERE tm.user_id = ?
			AND ta.tenant_id IS NULL
		ORDER BY
			CASE tm.role
				WHEN 'owner' THEN 0
				WHEN 'admin' THEN 1
				ELSE 2
			END ASC,
			tm.added_at ASC
		LIMIT 1
	`, userID).Scan(&tenantIDRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return getDefaultTenantID(ctx, q)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("could not query primary tenant: %w", err)
	}

	if !tenantIDRaw.Valid || strings.TrimSpace(tenantIDRaw.String) == "" {
		return getDefaultTenantID(ctx, q)
	}

	tenantID, err := uuid.Parse(tenantIDRaw.String)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid tenant id %q: %w", tenantIDRaw.String, err)
	}
	return tenantID, nil
}

func getActiveTenantID(ctx context.Context, q tenantRowQueryer, userID uuid.UUID) (uuid.UUID, error) {
	var tenantIDRaw sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT CAST(up.active_tenant_id AS VARCHAR)
		FROM user_preferences up
		JOIN tenant_members tm
			ON tm.tenant_id = up.active_tenant_id
			AND tm.user_id = ?
		JOIN tenants t
			ON t.id = up.active_tenant_id
		LEFT JOIN tenant_archives ta
			ON ta.tenant_id = t.id
		WHERE up.user_id = ?
			AND ta.tenant_id IS NULL
		LIMIT 1
	`, userID, userID).Scan(&tenantIDRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return getPrimaryTenantID(ctx, q, userID)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("could not query active tenant: %w", err)
	}
	if !tenantIDRaw.Valid || strings.TrimSpace(tenantIDRaw.String) == "" {
		return getPrimaryTenantID(ctx, q, userID)
	}

	tenantID, err := uuid.Parse(tenantIDRaw.String)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid active tenant id %q: %w", tenantIDRaw.String, err)
	}
	return tenantID, nil
}

func getDefaultTenantID(ctx context.Context, q tenantRowQueryer) (uuid.UUID, error) {
	var tenantIDRaw sql.NullString
	err := q.QueryRowContext(ctx, "SELECT CAST(id AS VARCHAR) FROM tenants WHERE is_default = TRUE LIMIT 1").Scan(&tenantIDRaw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		// The is_default column may not exist yet (pre-migration). Fall back
		// to the oldest tenant which is the implicit default in that state.
		if isBinderError(err) {
			return getFirstTenantID(ctx, q)
		}
		return uuid.Nil, fmt.Errorf("could not query default tenant: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) || !tenantIDRaw.Valid || strings.TrimSpace(tenantIDRaw.String) == "" {
		return uuid.Nil, fmt.Errorf("default tenant not found")
	}

	tenantID, err := uuid.Parse(tenantIDRaw.String)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid default tenant id %q: %w", tenantIDRaw.String, err)
	}
	return tenantID, nil
}

// getFirstTenantID returns the oldest tenant by created_at. Used as a fallback
// when the is_default column doesn't exist yet (pre-migration schema).
func getFirstTenantID(ctx context.Context, q tenantRowQueryer) (uuid.UUID, error) {
	var tenantIDRaw sql.NullString
	err := q.QueryRowContext(ctx, "SELECT CAST(id AS VARCHAR) FROM tenants ORDER BY created_at ASC LIMIT 1").Scan(&tenantIDRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("default tenant not found")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("could not query first tenant: %w", err)
	}
	if !tenantIDRaw.Valid || strings.TrimSpace(tenantIDRaw.String) == "" {
		return uuid.Nil, fmt.Errorf("first tenant id is empty")
	}

	tenantID, err := uuid.Parse(tenantIDRaw.String)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid first tenant id %q: %w", tenantIDRaw.String, err)
	}
	return tenantID, nil
}

// isBinderError returns true when DuckDB reports a Binder Error, typically
// because a referenced column or table doesn't exist in the current schema.
func isBinderError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "binder error")
}

func countTeamOwnersTx(ctx context.Context, tx *sql.Tx, tenantID uuid.UUID) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tenant_members WHERE tenant_id = ? AND role = ?",
		tenantID, TenantRoleOwner,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("could not count tenant owners: %w", err)
	}
	return count, nil
}

func scanTeamInvite(scanner interface {
	Scan(dest ...any) error
}) (api.TeamInvite, error) {
	var invite api.TeamInvite
	var invitedUserRaw sql.NullString
	var createdByRaw sql.NullString
	var acceptedAt sql.NullTime
	var revokedAt sql.NullTime
	if err := scanner.Scan(
		&invite.ID,
		&invite.TeamID,
		&invite.Email,
		&invite.Role,
		&invitedUserRaw,
		&invite.Status,
		&createdByRaw,
		&invite.CreatedAt,
		&invite.ExpiresAt,
		&acceptedAt,
		&revokedAt,
	); err != nil {
		return api.TeamInvite{}, fmt.Errorf("could not scan team invite: %w", err)
	}

	if invitedUserRaw.Valid && strings.TrimSpace(invitedUserRaw.String) != "" {
		invitedUserID, err := uuid.Parse(invitedUserRaw.String)
		if err != nil {
			return api.TeamInvite{}, fmt.Errorf("invalid invited user id %q: %w", invitedUserRaw.String, err)
		}
		invite.InvitedUserID = &invitedUserID
	}
	if createdByRaw.Valid && strings.TrimSpace(createdByRaw.String) != "" {
		createdBy, err := uuid.Parse(createdByRaw.String)
		if err != nil {
			return api.TeamInvite{}, fmt.Errorf("invalid created by id %q: %w", createdByRaw.String, err)
		}
		invite.CreatedBy = &createdBy
	}
	if acceptedAt.Valid {
		accepted := acceptedAt.Time
		invite.AcceptedAt = &accepted
	}
	if revokedAt.Valid {
		revoked := revokedAt.Time
		invite.RevokedAt = &revoked
	}

	return invite, nil
}

func scanTeamInviteRow(row *sql.Row) (*api.TeamInvite, error) {
	invite, err := scanTeamInvite(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, err
	}
	return &invite, nil
}

func getUserLocaleTx(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (string, error) {
	var locale sql.NullString
	err := tx.QueryRowContext(ctx, "SELECT default_locale FROM user_preferences WHERE user_id = ?", userID).Scan(&locale)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("could not query user locale: %w", err)
	}

	out := strings.TrimSpace(locale.String)
	if out == "" {
		out = defaultLocaleCode
	}
	return out, nil
}
