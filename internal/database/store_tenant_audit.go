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

func (s *Store) AppendTeamAuditEntry(ctx context.Context, tenantID, actorID uuid.UUID, action, details string, targetUserID *uuid.UUID) error {
	action = strings.TrimSpace(action)
	if action == "" {
		return fmt.Errorf("team audit action is required")
	}

	details = strings.TrimSpace(details)
	var targetArg any
	if targetUserID != nil && *targetUserID != uuid.Nil {
		targetArg = *targetUserID
	}

	if err := s.Exec(ctx, `
		INSERT INTO team_audit_log (tenant_id, actor_id, target_user_id, action, details)
		VALUES (?, ?, ?, ?, ?)
	`, tenantID, nullableUUID(actorID), targetArg, action, details); err != nil {
		return fmt.Errorf("could not append team audit entry: %w", err)
	}
	return nil
}

func (s *Store) ListTeamAuditEntries(ctx context.Context, tenantID uuid.UUID, action string, limit int, offset int) ([]api.TeamAuditEntry, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	action = strings.TrimSpace(action)

	whereClause := "WHERE ta.tenant_id = ?"
	countArgs := []any{tenantID}
	queryArgs := []any{tenantID}
	if action != "" {
		whereClause += " AND ta.action = ?"
		countArgs = append(countArgs, action)
		queryArgs = append(queryArgs, action)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM team_audit_log ta
	`+whereClause, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("could not count team audit entries: %w", err)
	}

	queryArgs = append(queryArgs, limit, offset)

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			ta.id,
			ta.tenant_id,
			ta.action,
			ta.details,
			ta.created_at,
			CAST(ta.actor_id AS VARCHAR),
			COALESCE(actor.email, ''),
			CAST(ta.target_user_id AS VARCHAR),
			COALESCE(target.email, '')
		FROM team_audit_log ta
		LEFT JOIN users actor ON actor.id = ta.actor_id
		LEFT JOIN users target ON target.id = ta.target_user_id
		`+whereClause+`
		ORDER BY ta.created_at DESC
		LIMIT ?
		OFFSET ?
	`, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("could not list team audit entries: %w", err)
	}
	defer rows.Close()

	entries := make([]api.TeamAuditEntry, 0, limit)
	for rows.Next() {
		var entry api.TeamAuditEntry
		var actorIDRaw sql.NullString
		var targetIDRaw sql.NullString
		if err := rows.Scan(
			&entry.ID,
			&entry.TeamID,
			&entry.Action,
			&entry.Details,
			&entry.CreatedAt,
			&actorIDRaw,
			&entry.ActorEmail,
			&targetIDRaw,
			&entry.TargetEmail,
		); err != nil {
			return nil, 0, fmt.Errorf("could not scan team audit entry: %w", err)
		}

		if actorIDRaw.Valid && strings.TrimSpace(actorIDRaw.String) != "" {
			if actorID, err := uuid.Parse(actorIDRaw.String); err == nil {
				entry.ActorUserID = &actorID
			}
		}
		if targetIDRaw.Valid && strings.TrimSpace(targetIDRaw.String) != "" {
			if targetID, err := uuid.Parse(targetIDRaw.String); err == nil {
				entry.TargetUserID = &targetID
			}
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("could not read team audit entries: %w", err)
	}

	return entries, total, nil
}

func (s *Store) GetTenant(ctx context.Context, tenantID uuid.UUID) (*api.Team, error) {
	var team api.Team
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, COALESCE(logo_url, ''), created_at FROM tenants WHERE id = ? LIMIT 1",
		tenantID,
	).Scan(&team.ID, &team.Name, &team.LogoURL, &team.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not query tenant: %w", err)
	}
	return &team, nil
}

func (s *Store) UpdateTenant(ctx context.Context, tenantID uuid.UUID, name, logoURL string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("team name cannot be empty")
	}
	logoURL = strings.TrimSpace(logoURL)
	_, err := s.db.ExecContext(ctx,
		"UPDATE tenants SET name = ?, logo_url = ? WHERE id = ?",
		name, logoURL, tenantID,
	)
	if err != nil {
		return fmt.Errorf("could not update tenant: %w", err)
	}
	return nil
}

func (s *Store) CreateTenant(ctx context.Context, creatorID uuid.UUID, name, logoURL string) (*api.Team, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("team name cannot be empty")
	}
	logoURL = strings.TrimSpace(logoURL)

	var team api.Team
	err := s.Transact(ctx, func(tx *sql.Tx) error {
		tenantID := uuid.New()
		now := time.Now().UTC()
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO tenants (id, name, logo_url, created_at) VALUES (?, ?, ?, ?)",
			tenantID, name, logoURL, now,
		); err != nil {
			return fmt.Errorf("could not insert tenant: %w", err)
		}

		if _, err := tx.ExecContext(ctx,
			"INSERT INTO tenant_members (tenant_id, user_id, role, added_by) VALUES (?, ?, ?, ?)",
			tenantID, creatorID, TenantRoleOwner, creatorID,
		); err != nil {
			return fmt.Errorf("could not insert tenant owner: %w", err)
		}

		team = api.Team{
			ID:        tenantID,
			Name:      name,
			LogoURL:   logoURL,
			Role:      TenantRoleOwner,
			CreatedAt: now,
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("could not create tenant: %w", err)
	}
	return &team, nil
}

func (s *Store) ListNonDefaultTenantIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT CAST(t.id AS VARCHAR)
		FROM tenants t
		LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
		WHERE t.is_default = FALSE AND ta.tenant_id IS NULL
		ORDER BY t.created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("could not list non-default tenants: %w", err)
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("could not scan tenant id: %w", err)
		}
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid tenant id %q: %w", raw, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read tenant rows: %w", err)
	}
	return ids, nil
}
