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

	return nil
}

func (s *Store) CreateTeamInvite(
	ctx context.Context,
	tenantID uuid.UUID,
	email string,
	role string,
	invitedUserID *uuid.UUID,
	createdBy uuid.UUID,
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
		ID:        uuid.New(),
		TeamID:    tenantID,
		Email:     email,
		Role:      role,
		Status:    TeamInviteStatusPending,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(7 * 24 * time.Hour),
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
				created_by,
				created_at,
				expires_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, invite.ID, invite.TeamID, invite.Email, invite.Role, invitedUserValue, invite.Status, createdByValue, invite.CreatedAt, invite.ExpiresAt); err != nil {
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
		rows, err := tx.QueryContext(ctx, `
			SELECT
				id,
				tenant_id,
				email,
				role,
				CAST(invited_user_id AS VARCHAR),
				status,
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
			return fmt.Errorf("could not query pending invites: %w", err)
		}
		defer rows.Close()

		now := time.Now().UTC()
		for rows.Next() {
			invite, err := scanTeamInvite(rows)
			if err != nil {
				return err
			}
			if invite.ExpiresAt.Before(now) {
				if _, err := tx.ExecContext(ctx, `
					UPDATE team_invites
					SET status = ?, revoked_at = ?
					WHERE id = ?
				`, TeamInviteStatusRevoked, now, invite.ID); err != nil {
					return fmt.Errorf("could not expire team invite: %w", err)
				}
				continue
			}

			createdBy := uuid.Nil
			if invite.CreatedBy != nil {
				createdBy = *invite.CreatedBy
			}
			if err := ensureTenantMemberTx(ctx, tx, invite.TeamID, userID, invite.Role, createdBy); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE team_invites
				SET invited_user_id = ?, status = ?, accepted_at = ?
				WHERE id = ?
			`, userID, TeamInviteStatusAccepted, now, invite.ID); err != nil {
				return fmt.Errorf("could not accept team invite: %w", err)
			}
			userIDCopy := userID
			invite.InvitedUserID = &userIDCopy
			invite.Status = TeamInviteStatusAccepted
			invite.AcceptedAt = &now
			accepted = append(accepted, invite)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("could not read pending invites: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return accepted, nil
}

func (s *Store) RemoveTeamMember(ctx context.Context, tenantID, userID uuid.UUID) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
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

	return nextActiveTenantID, nil
}

func (s *Store) TransferTeamOwnership(ctx context.Context, tenantID, actorID, targetUserID uuid.UUID) error {
	if actorID == targetUserID {
		return ErrTeamTransferSelf
	}

	return s.Transact(ctx, func(tx *sql.Tx) error {
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
