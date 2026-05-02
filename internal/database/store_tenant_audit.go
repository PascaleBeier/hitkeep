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
	targetUserIDValue := uuid.Nil
	if targetUserID != nil && *targetUserID != uuid.Nil {
		targetUserIDValue = *targetUserID
	}

	actorEmail := ""
	actorRole := ""
	if actorID != uuid.Nil {
		if actor, err := s.GetUserByID(ctx, actorID); err == nil && actor != nil {
			actorEmail = actor.Email
		}
		if role, err := s.GetTenantRole(ctx, tenantID, actorID); err == nil {
			actorRole = role
		}
	}

	targetLabel := ""
	if team, err := s.GetTenant(ctx, tenantID); err == nil && team != nil {
		targetLabel = team.Name
	}

	if err := s.AppendAuditEntry(ctx, AuditEntryParams{
		ActorID:      actorID,
		ActorEmail:   actorEmail,
		ActorRole:    actorRole,
		TeamID:       tenantID,
		TargetUserID: targetUserIDValue,
		Action:       action,
		TargetType:   "team",
		TargetID:     tenantID.String(),
		TargetLabel:  targetLabel,
		Outcome:      "success",
		Details:      details,
	}); err != nil {
		return fmt.Errorf("could not append team audit entry: %w", err)
	}
	return nil
}

type TeamAuditFilter struct {
	Action     string
	TargetType string
	Outcome    string
	Query      string
	From       time.Time
	To         time.Time
	Limit      int
	Offset     int
}

const (
	DefaultTeamAuditListLimit = 25
	MaxTeamAuditListLimit     = 200
)

func (s *Store) ListTeamAuditEntries(ctx context.Context, tenantID uuid.UUID, action string, limit int, offset int) ([]api.TeamAuditEntry, int, error) {
	return s.ListTeamAuditEntriesFiltered(ctx, tenantID, TeamAuditFilter{
		Action: action,
		Limit:  limit,
		Offset: offset,
	})
}

func (s *Store) ListTeamAuditEntriesFiltered(ctx context.Context, tenantID uuid.UUID, filter TeamAuditFilter) ([]api.TeamAuditEntry, int, error) {
	limit := filter.Limit
	if limit <= 0 || limit > MaxTeamAuditListLimit {
		limit = DefaultTeamAuditListLimit
	}
	offset := max(filter.Offset, 0)

	whereClauses := []string{"ta.tenant_id = ?"}
	args := []any{tenantID}
	if action := strings.TrimSpace(filter.Action); action != "" {
		whereClauses = append(whereClauses, "ta.action = ?")
		args = append(args, action)
	}
	if targetType := strings.TrimSpace(filter.TargetType); targetType != "" {
		whereClauses = append(whereClauses, "ta.target_type = ?")
		args = append(args, targetType)
	}
	if outcome := strings.TrimSpace(filter.Outcome); outcome != "" {
		whereClauses = append(whereClauses, "ta.outcome = ?")
		args = append(args, outcome)
	}
	if !filter.From.IsZero() {
		whereClauses = append(whereClauses, "ta.created_at >= ?")
		args = append(args, filter.From)
	}
	if !filter.To.IsZero() {
		whereClauses = append(whereClauses, "ta.created_at <= ?")
		args = append(args, filter.To)
	}
	if query := strings.TrimSpace(filter.Query); query != "" {
		q := "%" + query + "%"
		whereClauses = append(whereClauses, "(ta.actor_email_snapshot ILIKE ? OR ta.target_label ILIKE ? OR ta.details ILIKE ? OR ta.ip_address ILIKE ? OR ta.ip_country_code ILIKE ? OR ta.request_id ILIKE ?)")
		args = append(args, q, q, q, q, q, q)
	}
	whereClause := "WHERE " + strings.Join(whereClauses, " AND ")

	var total int
	// #nosec G202 -- whereClause is assembled only from fixed SQL fragments above; user values stay parameterized.
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM team_audit_log ta
	`+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("could not count team audit entries: %w", err)
	}

	queryArgs := make([]any, 0, len(args)+2)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, limit, offset)

	// #nosec G202 -- whereClause is assembled only from fixed SQL fragments above; user values stay parameterized.
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			ta.id,
			ta.tenant_id,
			ta.action,
			ta.details,
			ta.created_at,
			CAST(ta.actor_id AS VARCHAR),
			ta.actor_email_snapshot,
			ta.actor_role_snapshot,
			COALESCE(actor.email, ta.actor_email_snapshot, ''),
			CAST(ta.target_user_id AS VARCHAR),
			COALESCE(target.email, CASE WHEN ta.target_type = 'user' THEN ta.target_label ELSE '' END, ''),
			ta.target_type,
			ta.target_id,
			ta.target_label,
			ta.outcome,
			ta.ip_address,
			ta.ip_country_code,
			ta.user_agent,
			ta.request_id
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
			&entry.ActorEmailSnapshot,
			&entry.ActorRoleSnapshot,
			&entry.ActorEmail,
			&targetIDRaw,
			&entry.TargetEmail,
			&entry.TargetType,
			&entry.TargetID,
			&entry.TargetLabel,
			&entry.Outcome,
			&entry.IPAddress,
			&entry.IPCountryCode,
			&entry.UserAgent,
			&entry.RequestID,
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
