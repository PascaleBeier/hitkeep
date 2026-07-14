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

func (s *Store) ListPendingTeamInvitesByEmail(ctx context.Context, email string) ([]api.TeamInvite, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, fmt.Errorf("invite email is required")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id,
			tenant_id,
			email,
			role,
			CAST(invited_user_id AS VARCHAR),
			status,
			requires_password_setup,
			CAST(created_by AS VARCHAR),
			created_at,
			expires_at,
			accepted_at,
			revoked_at
		FROM team_invites
		WHERE lower(email) = lower(?) AND status = ?
		ORDER BY created_at ASC
	`, email, TeamInviteStatusPending)
	if err != nil {
		return nil, fmt.Errorf("could not query pending invites: %w", err)
	}
	defer rows.Close()

	invites := make([]api.TeamInvite, 0)
	for rows.Next() {
		invite, err := scanTeamInvite(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read pending invites: %w", err)
	}

	return invites, nil
}

func (s *Store) ListSoleOwnerTeams(ctx context.Context, userID uuid.UUID) ([]api.Team, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.name, COALESCE(t.logo_url, ''), tm.role, t.created_at
		FROM tenants t
		JOIN tenant_members tm ON tm.tenant_id = t.id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
		WHERE tm.user_id = ?
			AND tm.role = ?
			AND ta.tenant_id IS NULL
			AND NOT EXISTS (
				SELECT 1
				FROM tenant_members other
				WHERE other.tenant_id = t.id
					AND other.role = ?
					AND other.user_id <> ?
			)
		ORDER BY t.created_at ASC
	`, userID, TenantRoleOwner, TenantRoleOwner, userID)
	if err != nil {
		return nil, fmt.Errorf("could not list sole-owner teams: %w", err)
	}
	defer rows.Close()

	teams := make([]api.Team, 0)
	for rows.Next() {
		var team api.Team
		if err := rows.Scan(&team.ID, &team.Name, &team.LogoURL, &team.Role, &team.CreatedAt); err != nil {
			return nil, fmt.Errorf("could not scan sole-owner team: %w", err)
		}
		teams = append(teams, team)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read sole-owner team rows: %w", err)
	}

	return teams, nil
}

func (s *Store) ListTeamMembers(ctx context.Context, tenantID uuid.UUID) ([]api.TeamMember, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT tm.id, tm.user_id, u.email, tm.role, tm.added_at
		FROM tenant_members tm
		JOIN users u ON u.id = tm.user_id
		WHERE tm.tenant_id = ?
		ORDER BY
			CASE tm.role
				WHEN 'owner' THEN 0
				WHEN 'admin' THEN 1
				ELSE 2
			END ASC,
			tm.added_at ASC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("could not list team members: %w", err)
	}
	defer rows.Close()

	members := make([]api.TeamMember, 0)
	for rows.Next() {
		var member api.TeamMember
		if err := rows.Scan(&member.ID, &member.UserID, &member.Email, &member.Role, &member.AddedAt); err != nil {
			return nil, fmt.Errorf("could not scan team member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read team member rows: %w", err)
	}

	return members, nil
}

func (s *Store) AddTeamMember(ctx context.Context, tenantID, userID uuid.UUID, role string, addedBy uuid.UUID) error {
	role = strings.TrimSpace(strings.ToLower(role))
	if !IsValidTenantRole(role) {
		return fmt.Errorf("invalid tenant role")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tenant_members (tenant_id, user_id, role, added_by)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (tenant_id, user_id) DO UPDATE SET
			role = excluded.role,
			added_by = excluded.added_by,
			added_at = NOW()
	`, tenantID, userID, role, nullableUUID(addedBy))
	if err != nil {
		return fmt.Errorf("could not add tenant member: %w", err)
	}
	s.invalidateAllSiteRolesForUser(userID)

	return nil
}

func (s *Store) CreateTeamInvite(
	ctx context.Context,
	tenantID uuid.UUID,
	email string,
	role string,
	invitedUserID *uuid.UUID,
	createdBy uuid.UUID,
	requiresPasswordSetup bool,
) (*api.TeamInvite, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	role = strings.TrimSpace(strings.ToLower(role))
	if email == "" {
		return nil, fmt.Errorf("invite email is required")
	}
	if !IsValidTenantRole(role) {
		return nil, fmt.Errorf("invalid tenant role")
	}

	invite := &api.TeamInvite{
		ID:                    uuid.New(),
		TeamID:                tenantID,
		Email:                 email,
		Role:                  role,
		Status:                TeamInviteStatusPending,
		RequiresPasswordSetup: requiresPasswordSetup,
		CreatedAt:             time.Now().UTC(),
		ExpiresAt:             time.Now().UTC().Add(7 * 24 * time.Hour),
	}
	if invitedUserID != nil && *invitedUserID != uuid.Nil {
		userID := *invitedUserID
		invite.InvitedUserID = &userID
	}
	if createdBy != uuid.Nil {
		actorID := createdBy
		invite.CreatedBy = &actorID
	}

	err := s.Transact(ctx, func(tx *sql.Tx) error {
		var existingID uuid.UUID
		err := tx.QueryRowContext(ctx, `
			SELECT id
			FROM team_invites
			WHERE tenant_id = ? AND lower(email) = lower(?) AND status = ?
			ORDER BY created_at DESC
			LIMIT 1
		`, tenantID, email, TeamInviteStatusPending).Scan(&existingID)
		if err == nil {
			return ErrTeamInviteAlreadyPending
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("could not query pending invite: %w", err)
		}

		var invitedUserValue any
		if invite.InvitedUserID != nil {
			invitedUserValue = *invite.InvitedUserID
		}
		var createdByValue any
		if invite.CreatedBy != nil {
			createdByValue = *invite.CreatedBy
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO team_invites (
				id,
				tenant_id,
				email,
					role,
					invited_user_id,
					status,
					requires_password_setup,
					created_by,
					created_at,
					expires_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, invite.ID, invite.TeamID, invite.Email, invite.Role, invitedUserValue, invite.Status, invite.RequiresPasswordSetup, createdByValue, invite.CreatedAt, invite.ExpiresAt); err != nil {
			return fmt.Errorf("could not insert team invite: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return invite, nil
}

func (s *Store) ListTeamInvites(ctx context.Context, tenantID uuid.UUID) ([]api.TeamInvite, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id,
			tenant_id,
			email,
			role,
			CAST(invited_user_id AS VARCHAR),
			status,
			requires_password_setup,
			CAST(created_by AS VARCHAR),
			created_at,
			expires_at,
			accepted_at,
			revoked_at
		FROM team_invites
		WHERE tenant_id = ? AND status = ?
		ORDER BY created_at DESC
	`, tenantID, TeamInviteStatusPending)
	if err != nil {
		return nil, fmt.Errorf("could not list team invites: %w", err)
	}
	defer rows.Close()

	invites := make([]api.TeamInvite, 0)
	for rows.Next() {
		invite, err := scanTeamInvite(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read team invites: %w", err)
	}

	return invites, nil
}

func (s *Store) GetTeamInvite(ctx context.Context, tenantID, inviteID uuid.UUID) (*api.TeamInvite, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			id,
			tenant_id,
			email,
			role,
			CAST(invited_user_id AS VARCHAR),
			status,
			requires_password_setup,
			CAST(created_by AS VARCHAR),
			created_at,
			expires_at,
			accepted_at,
			revoked_at
		FROM team_invites
		WHERE tenant_id = ? AND id = ?
		LIMIT 1
	`, tenantID, inviteID)

	invite, err := scanTeamInviteRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTeamInviteNotFound
	}
	if err != nil {
		return nil, err
	}
	return invite, nil
}

func (s *Store) ResendTeamInvite(ctx context.Context, tenantID, inviteID uuid.UUID) (*api.TeamInvite, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(7 * 24 * time.Hour)

	err := s.Transact(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE team_invites
			SET created_at = ?, expires_at = ?
			WHERE tenant_id = ? AND id = ? AND status = ?
		`, now, expiresAt, tenantID, inviteID, TeamInviteStatusPending)
		if err != nil {
			return fmt.Errorf("could not update team invite: %w", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("could not update team invite: %w", err)
		}
		if rowsAffected == 0 {
			return ErrTeamInviteNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.GetTeamInvite(ctx, tenantID, inviteID)
}

func (s *Store) RevokeTeamInvite(ctx context.Context, tenantID, inviteID uuid.UUID) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE team_invites
		SET status = ?, revoked_at = ?
		WHERE tenant_id = ? AND id = ? AND status = ?
	`, TeamInviteStatusRevoked, now, tenantID, inviteID, TeamInviteStatusPending)
	if err != nil {
		return fmt.Errorf("could not revoke team invite: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("could not revoke team invite: %w", err)
	}
	if rowsAffected == 0 {
		return ErrTeamInviteNotFound
	}
	return nil
}

func (s *Store) AcceptTeamInvitesByEmail(ctx context.Context, email string, userID uuid.UUID) ([]api.TeamInvite, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, fmt.Errorf("invite email is required")
	}

	accepted := make([]api.TeamInvite, 0)
	err := s.Transact(ctx, func(tx *sql.Tx) error {
		var err error
		accepted, err = acceptTeamInvitesByEmailTx(ctx, tx, email, userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.invalidateAllSiteRolesForUser(userID)

	return accepted, nil
}

func (s *Store) AcceptInviteWithPassword(ctx context.Context, token string, newHashedPassword string) (string, uuid.UUID, []api.TeamInvite, error) {
	entry, found, err := s.lookupPasswordResetToken(token, true)
	if err != nil {
		return "", uuid.Nil, nil, err
	}
	if !found {
		return "", uuid.Nil, nil, ErrPasswordResetInvalid
	}

	email := strings.ToLower(strings.TrimSpace(entry.Email))
	var userID uuid.UUID
	accepted := make([]api.TeamInvite, 0)
	err = s.Transact(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT id FROM users WHERE lower(email) = lower(?)", email).Scan(&userID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrTeamInviteNotFound
			}
			return fmt.Errorf("could not load invited user: %w", err)
		}

		requiresPasswordSetup, err := hasPendingPasswordSetupInviteTx(ctx, tx, email)
		if err != nil {
			return err
		}
		if !requiresPasswordSetup {
			return ErrTeamInviteLoginRequired
		}

		if _, err := tx.ExecContext(ctx, "UPDATE users SET password = ? WHERE id = ?", newHashedPassword, userID); err != nil {
			return fmt.Errorf("could not update invited user password: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM remember_me_tokens WHERE user_id = ?", userID); err != nil {
			return fmt.Errorf("could not clear invited user remembered sessions: %w", err)
		}

		accepted, err = acceptTeamInvitesByEmailTx(ctx, tx, email, userID)
		return err
	})
	if err != nil {
		restorePasswordResetToken(s, token, entry)
		return "", uuid.Nil, nil, err
	}
	s.invalidateAllSiteRolesForUser(userID)
	return email, userID, accepted, nil
}

func (s *Store) AcceptInviteForAuthenticatedUser(ctx context.Context, token string, userID uuid.UUID) (string, []api.TeamInvite, error) {
	entry, found, err := s.lookupPasswordResetToken(token, true)
	if err != nil {
		return "", nil, err
	}
	if !found {
		return "", nil, ErrPasswordResetInvalid
	}

	inviteEmail := strings.ToLower(strings.TrimSpace(entry.Email))
	accepted := make([]api.TeamInvite, 0)
	err = s.Transact(ctx, func(tx *sql.Tx) error {
		var userEmail string
		if err := tx.QueryRowContext(ctx, "SELECT email FROM users WHERE id = ?", userID).Scan(&userEmail); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrUserNotFound
			}
			return fmt.Errorf("could not load authenticated user: %w", err)
		}
		if !strings.EqualFold(strings.TrimSpace(userEmail), inviteEmail) {
			return ErrTeamInviteEmailMismatch
		}

		accepted, err = acceptTeamInvitesByEmailTx(ctx, tx, inviteEmail, userID)
		return err
	})
	if err != nil {
		restorePasswordResetToken(s, token, entry)
		return "", nil, err
	}
	s.invalidateAllSiteRolesForUser(userID)
	return inviteEmail, accepted, nil
}

// AcceptTeamInviteForSSO accepts exactly one team's pending invitation after
// the caller has verified the user's email through OIDC. When token is set it
// is consumed and must belong to the same email; tokenless acceptance supports
// the regular login flow where the verified identity itself proves ownership.
func (s *Store) AcceptTeamInviteForSSO(ctx context.Context, token string, teamID, userID uuid.UUID) (api.TeamInvite, error) {
	if teamID == uuid.Nil || userID == uuid.Nil {
		return api.TeamInvite{}, ErrTeamInviteNotFound
	}

	token = strings.TrimSpace(token)
	var tokenEntry passwordResetEntry
	if token != "" {
		entry, found, err := s.lookupPasswordResetToken(token, true)
		if err != nil {
			return api.TeamInvite{}, err
		}
		if !found {
			return api.TeamInvite{}, ErrPasswordResetInvalid
		}
		tokenEntry = entry
	}

	accepted := api.TeamInvite{}
	err := s.Transact(ctx, func(tx *sql.Tx) error {
		var userEmail string
		if err := tx.QueryRowContext(ctx, "SELECT email FROM users WHERE id = ?", userID).Scan(&userEmail); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrUserNotFound
			}
			return fmt.Errorf("could not load SSO invite user: %w", err)
		}
		userEmail = strings.ToLower(strings.TrimSpace(userEmail))
		if token != "" && !strings.EqualFold(userEmail, strings.TrimSpace(tokenEntry.Email)) {
			return ErrTeamInviteEmailMismatch
		}

		invite, err := scanTeamInviteRow(tx.QueryRowContext(ctx, `
			SELECT
				id,
				tenant_id,
				email,
				role,
				CAST(invited_user_id AS VARCHAR),
				status,
				requires_password_setup,
				CAST(created_by AS VARCHAR),
				created_at,
				expires_at,
				accepted_at,
				revoked_at
			FROM team_invites
			WHERE tenant_id = ? AND lower(email) = lower(?) AND status = ?
			ORDER BY created_at ASC
			LIMIT 1
		`, teamID, userEmail, TeamInviteStatusPending))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTeamInviteNotFound
		}
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		if invite.ExpiresAt.Before(now) {
			if _, err := tx.ExecContext(ctx, `
				UPDATE team_invites
				SET status = ?, revoked_at = ?
				WHERE id = ?
			`, TeamInviteStatusRevoked, now, invite.ID); err != nil {
				return fmt.Errorf("could not expire SSO team invite: %w", err)
			}
			return ErrTeamInviteNotFound
		}

		createdBy := uuid.Nil
		if invite.CreatedBy != nil {
			createdBy = *invite.CreatedBy
		}
		if err := ensureTenantMemberTx(ctx, tx, teamID, userID, invite.Role, createdBy); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE team_invites
			SET invited_user_id = ?, status = ?, accepted_at = ?
			WHERE id = ? AND status = ?
		`, userID, TeamInviteStatusAccepted, now, invite.ID, TeamInviteStatusPending); err != nil {
			return fmt.Errorf("could not accept SSO team invite: %w", err)
		}
		if err := setActiveTenantTx(ctx, tx, userID, teamID); err != nil {
			return err
		}

		userIDCopy := userID
		invite.InvitedUserID = &userIDCopy
		invite.Status = TeamInviteStatusAccepted
		invite.AcceptedAt = &now
		accepted = *invite
		return nil
	})
	if err != nil {
		if token != "" {
			restorePasswordResetToken(s, token, tokenEntry)
		}
		return api.TeamInvite{}, err
	}
	if token == "" {
		s.deletePasswordResetTokenForEmail(accepted.Email)
	}
	s.invalidateAllSiteRolesForUser(userID)
	return accepted, nil
}

func restorePasswordResetToken(s *Store, token string, entry passwordResetEntry) {
	if time.Now().UTC().Before(entry.ExpiresAt.UTC()) {
		s.storePasswordResetToken(entry.Email, strings.TrimSpace(token), entry.ExpiresAt)
	}
}

func hasPendingPasswordSetupInviteTx(ctx context.Context, tx *sql.Tx, email string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM team_invites
		WHERE lower(email) = lower(?)
			AND status = ?
			AND requires_password_setup = TRUE
	`, email, TeamInviteStatusPending).Scan(&count); err != nil {
		return false, fmt.Errorf("could not query password setup invites: %w", err)
	}
	return count > 0, nil
}

func acceptTeamInvitesByEmailTx(ctx context.Context, tx *sql.Tx, email string, userID uuid.UUID) ([]api.TeamInvite, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT
			id,
			tenant_id,
			email,
			role,
			CAST(invited_user_id AS VARCHAR),
			status,
			requires_password_setup,
			CAST(created_by AS VARCHAR),
			created_at,
			expires_at,
			accepted_at,
			revoked_at
		FROM team_invites
		WHERE lower(email) = lower(?) AND status = ?
		ORDER BY created_at ASC
	`, email, TeamInviteStatusPending)
	if err != nil {
		return nil, fmt.Errorf("could not query pending invites: %w", err)
	}
	defer rows.Close()

	accepted := make([]api.TeamInvite, 0)
	now := time.Now().UTC()
	for rows.Next() {
		invite, err := scanTeamInvite(rows)
		if err != nil {
			return nil, err
		}
		if invite.ExpiresAt.Before(now) {
			if _, err := tx.ExecContext(ctx, `
				UPDATE team_invites
				SET status = ?, revoked_at = ?
				WHERE id = ?
			`, TeamInviteStatusRevoked, now, invite.ID); err != nil {
				return nil, fmt.Errorf("could not expire team invite: %w", err)
			}
			continue
		}

		createdBy := uuid.Nil
		if invite.CreatedBy != nil {
			createdBy = *invite.CreatedBy
		}
		if err := ensureTenantMemberTx(ctx, tx, invite.TeamID, userID, invite.Role, createdBy); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE team_invites
			SET invited_user_id = ?, status = ?, accepted_at = ?
			WHERE id = ?
		`, userID, TeamInviteStatusAccepted, now, invite.ID); err != nil {
			return nil, fmt.Errorf("could not accept team invite: %w", err)
		}
		userIDCopy := userID
		invite.InvitedUserID = &userIDCopy
		invite.Status = TeamInviteStatusAccepted
		invite.AcceptedAt = &now
		accepted = append(accepted, invite)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read pending invites: %w", err)
	}
	if len(accepted) == 0 {
		return nil, ErrTeamInviteNotFound
	}
	activeTenantID := accepted[len(accepted)-1].TeamID
	if err := setActiveTenantTx(ctx, tx, userID, activeTenantID); err != nil {
		return nil, err
	}

	return accepted, nil
}

func setActiveTenantTx(ctx context.Context, tx *sql.Tx, userID, tenantID uuid.UUID) error {
	locale, err := getUserLocaleTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_preferences (user_id, default_locale, updated_at, active_tenant_id)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			active_tenant_id = excluded.active_tenant_id,
			updated_at = excluded.updated_at
	`, userID, locale, now, tenantID); err != nil {
		return fmt.Errorf("could not set accepted invite active team: %w", err)
	}
	return nil
}

func (s *Store) RemoveTeamMember(ctx context.Context, tenantID, userID uuid.UUID) error {
	err := s.Transact(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "DELETE FROM tenant_members WHERE tenant_id = ? AND user_id = ?", tenantID, userID)
		if err != nil {
			return fmt.Errorf("could not remove tenant member: %w", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("could not remove tenant member: %w", err)
		}
		if rowsAffected == 0 {
			return fmt.Errorf("tenant member not found")
		}

		if err := removeUserTenantScopedSiteAccessTx(ctx, tx, tenantID, userID); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}
	s.invalidateAllSiteRolesForUser(userID)
	return nil
}

func (s *Store) LeaveTeam(ctx context.Context, tenantID, userID uuid.UUID) (uuid.UUID, error) {
	nextActiveTenantID := uuid.Nil

	err := s.Transact(ctx, func(tx *sql.Tx) error {
		var role string
		if err := tx.QueryRowContext(ctx,
			"SELECT role FROM tenant_members WHERE tenant_id = ? AND user_id = ?",
			tenantID, userID,
		).Scan(&role); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrTenantMembershipRequired
			}
			return fmt.Errorf("could not resolve tenant membership: %w", err)
		}

		var userTeamCount int
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM tenant_members WHERE user_id = ?",
			userID,
		).Scan(&userTeamCount); err != nil {
			return fmt.Errorf("could not count user teams: %w", err)
		}
		if userTeamCount <= 1 {
			return ErrUserOnlyTeam
		}

		if strings.EqualFold(strings.TrimSpace(role), TenantRoleOwner) {
			ownerCount, err := countTeamOwnersTx(ctx, tx, tenantID)
			if err != nil {
				return err
			}
			if ownerCount <= 1 {
				return ErrTeamLastOwner
			}
		}

		if _, err := tx.ExecContext(ctx,
			"DELETE FROM tenant_members WHERE tenant_id = ? AND user_id = ?",
			tenantID, userID,
		); err != nil {
			return fmt.Errorf("could not remove tenant member: %w", err)
		}

		if err := removeUserTenantScopedSiteAccessTx(ctx, tx, tenantID, userID); err != nil {
			return err
		}

		currentActiveTenantID, err := getActiveTenantID(ctx, tx, userID)
		if err != nil {
			currentActiveTenantID = uuid.Nil
		}
		nextActiveTenantID = currentActiveTenantID

		if currentActiveTenantID == uuid.Nil || currentActiveTenantID == tenantID {
			replacementTenantID, err := getPrimaryTenantID(ctx, tx, userID)
			if err != nil {
				return fmt.Errorf("could not resolve replacement team: %w", err)
			}
			nextActiveTenantID = replacementTenantID
		}

		locale, err := getUserLocaleTx(ctx, tx, userID)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_preferences (user_id, default_locale, updated_at, active_tenant_id)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (user_id) DO UPDATE SET
				active_tenant_id = excluded.active_tenant_id,
				updated_at = excluded.updated_at
		`, userID, locale, now, nextActiveTenantID); err != nil {
			return fmt.Errorf("could not update active team after leave: %w", err)
		}

		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	s.invalidateAllSiteRolesForUser(userID)

	return nextActiveTenantID, nil
}

func (s *Store) TransferTeamOwnership(ctx context.Context, tenantID, actorID, targetUserID uuid.UUID) error {
	if actorID == targetUserID {
		return ErrTeamTransferSelf
	}

	err := s.Transact(ctx, func(tx *sql.Tx) error {
		var actorRole string
		if err := tx.QueryRowContext(ctx,
			"SELECT role FROM tenant_members WHERE tenant_id = ? AND user_id = ?",
			tenantID, actorID,
		).Scan(&actorRole); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrTenantMembershipRequired
			}
			return fmt.Errorf("could not resolve actor role: %w", err)
		}
		if actorRole != TenantRoleOwner {
			return ErrTeamTransferRequiresOwner
		}

		var targetRole string
		if err := tx.QueryRowContext(ctx,
			"SELECT role FROM tenant_members WHERE tenant_id = ? AND user_id = ?",
			tenantID, targetUserID,
		).Scan(&targetRole); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrTeamTransferTargetNotMember
			}
			return fmt.Errorf("could not resolve target role: %w", err)
		}
		if targetRole == TenantRoleOwner {
			return ErrTeamTransferTargetAlreadyOwner
		}

		if _, err := tx.ExecContext(ctx,
			"UPDATE tenant_members SET role = ? WHERE tenant_id = ? AND user_id = ?",
			TenantRoleOwner, tenantID, targetUserID,
		); err != nil {
			return fmt.Errorf("could not promote target owner: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE tenant_members SET role = ? WHERE tenant_id = ? AND user_id = ?",
			TenantRoleAdmin, tenantID, actorID,
		); err != nil {
			return fmt.Errorf("could not demote current owner: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}
	s.invalidateAllSiteRolesForUser(actorID)
	s.invalidateAllSiteRolesForUser(targetUserID)
	return nil
}

func (s *Store) CountTeamOwners(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tenant_members WHERE tenant_id = ? AND role = ?",
		tenantID, TenantRoleOwner,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("could not count tenant owners: %w", err)
	}
	return count, nil
}
