package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

type InstanceAuditParams struct {
	ActorID       uuid.UUID
	ActorEmail    string
	ActorRole     string
	Action        string
	TargetType    string
	TargetID      string
	TargetLabel   string
	Outcome       string
	IPAddress     string
	IPCountryCode string
	UserAgent     string
	RequestID     string
	Details       string
	MetadataJSON  string
}

type AuditEntryParams struct {
	ActorID       uuid.UUID
	ActorEmail    string
	ActorRole     string
	TeamID        uuid.UUID
	TargetUserID  uuid.UUID
	Action        string
	TargetType    string
	TargetID      string
	TargetLabel   string
	Outcome       string
	IPAddress     string
	IPCountryCode string
	UserAgent     string
	RequestID     string
	Details       string
	MetadataJSON  string
}

func (s *Store) AppendAuditEntry(ctx context.Context, params AuditEntryParams) error {
	return appendAuditEntry(ctx, s.db, params)
}

func appendAuditEntryTx(ctx context.Context, tx *sql.Tx, params AuditEntryParams) error {
	return appendAuditEntry(ctx, tx, params)
}

func appendAuditEntry(ctx context.Context, exec sqlExecContext, params AuditEntryParams) error {
	action := strings.TrimSpace(params.Action)
	if action == "" {
		return fmt.Errorf("audit action is required")
	}

	actorID := nullableUUID(params.ActorID)
	teamID := nullableUUID(params.TeamID)
	targetUserID := nullableUUID(params.TargetUserID)
	outcome := strings.TrimSpace(params.Outcome)
	if outcome == "" {
		outcome = "success"
	}
	metadataJSON := strings.TrimSpace(params.MetadataJSON)
	if metadataJSON == "" {
		metadataJSON = "{}"
	}

	_, err := exec.ExecContext(ctx, `
		INSERT INTO instance_audit_log (
			actor_id, actor_email_snapshot, actor_role_snapshot, team_id, target_user_id,
			action, target_type, target_id, target_label,
			outcome, ip_address, ip_country_code, user_agent, request_id,
			details, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		actorID,
		strings.TrimSpace(params.ActorEmail),
		strings.TrimSpace(params.ActorRole),
		teamID,
		targetUserID,
		action,
		strings.TrimSpace(params.TargetType),
		strings.TrimSpace(params.TargetID),
		strings.TrimSpace(params.TargetLabel),
		outcome,
		strings.TrimSpace(params.IPAddress),
		strings.ToUpper(strings.TrimSpace(params.IPCountryCode)),
		strings.TrimSpace(params.UserAgent),
		strings.TrimSpace(params.RequestID),
		strings.TrimSpace(params.Details),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("could not append audit entry: %w", err)
	}
	return nil
}

func (s *Store) AppendInstanceAuditEntry(ctx context.Context, params InstanceAuditParams) error {
	err := s.AppendAuditEntry(ctx, AuditEntryParams{
		ActorID:       params.ActorID,
		ActorEmail:    params.ActorEmail,
		ActorRole:     params.ActorRole,
		Action:        params.Action,
		TargetType:    params.TargetType,
		TargetID:      params.TargetID,
		TargetLabel:   params.TargetLabel,
		Outcome:       params.Outcome,
		IPAddress:     params.IPAddress,
		IPCountryCode: params.IPCountryCode,
		UserAgent:     params.UserAgent,
		RequestID:     params.RequestID,
		Details:       params.Details,
		MetadataJSON:  params.MetadataJSON,
	})
	if err != nil {
		return fmt.Errorf("could not append instance audit entry: %w", err)
	}
	return nil
}

type InstanceAuditFilter struct {
	Action     string
	ActorID    uuid.UUID
	TargetType string
	Outcome    string
	From       time.Time
	To         time.Time
	Query      string
	Limit      int
	Offset     int
}

const (
	DefaultInstanceAuditListLimit   = 100
	MaxInstanceAuditListLimit       = 200
	DefaultInstanceAuditExportLimit = 10000
	MaxInstanceAuditExportLimit     = 50000
)

func (s *Store) ListInstanceAuditEntries(ctx context.Context, filter InstanceAuditFilter) ([]api.InstanceAuditEntry, int, error) {
	limit := filter.Limit
	if limit <= 0 || limit > MaxInstanceAuditListLimit {
		limit = DefaultInstanceAuditListLimit
	}
	offset := max(filter.Offset, 0)

	whereClauses := make([]string, 0)
	args := make([]any, 0)

	if filter.Action != "" {
		whereClauses = append(whereClauses, "ial.action = ?")
		args = append(args, filter.Action)
	}
	if filter.ActorID != uuid.Nil {
		whereClauses = append(whereClauses, "ial.actor_id = ?")
		args = append(args, filter.ActorID)
	}
	if filter.TargetType != "" {
		whereClauses = append(whereClauses, "ial.target_type = ?")
		args = append(args, filter.TargetType)
	}
	if filter.Outcome != "" {
		whereClauses = append(whereClauses, "ial.outcome = ?")
		args = append(args, filter.Outcome)
	}
	if !filter.From.IsZero() {
		whereClauses = append(whereClauses, "ial.created_at >= ?")
		args = append(args, filter.From)
	}
	if !filter.To.IsZero() {
		whereClauses = append(whereClauses, "ial.created_at <= ?")
		args = append(args, filter.To)
	}
	if filter.Query != "" {
		q := "%" + filter.Query + "%"
		whereClauses = append(whereClauses, "(ial.actor_email_snapshot ILIKE ? OR ial.target_label ILIKE ? OR ial.details ILIKE ? OR ial.ip_address ILIKE ? OR ial.ip_country_code ILIKE ? OR ial.request_id ILIKE ?)")
		args = append(args, q, q, q, q, q, q)
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM instance_audit_log ial " + whereSQL
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("could not count instance audit entries: %w", err)
	}

	queryArgs := make([]any, 0, len(args)+2)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, limit, offset)

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			ial.id,
			ial.created_at,
			CAST(ial.actor_id AS VARCHAR),
			CAST(ial.team_id AS VARCHAR),
			CAST(ial.target_user_id AS VARCHAR),
			ial.actor_email_snapshot,
			ial.actor_role_snapshot,
			ial.action,
			ial.target_type,
			ial.target_id,
			ial.target_label,
			ial.outcome,
			ial.ip_address,
			ial.ip_country_code,
			ial.user_agent,
			ial.request_id,
			ial.details
		FROM instance_audit_log ial
		`+whereSQL+`
		ORDER BY ial.created_at DESC
		LIMIT ?
		OFFSET ?
	`, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("could not list instance audit entries: %w", err)
	}
	defer rows.Close()

	entries := make([]api.InstanceAuditEntry, 0, limit)
	for rows.Next() {
		var entry api.InstanceAuditEntry
		var actorIDRaw sql.NullString
		var teamIDRaw sql.NullString
		var targetUserIDRaw sql.NullString
		if err := rows.Scan(
			&entry.ID,
			&entry.CreatedAt,
			&actorIDRaw,
			&teamIDRaw,
			&targetUserIDRaw,
			&entry.ActorEmailSnapshot,
			&entry.ActorRoleSnapshot,
			&entry.Action,
			&entry.TargetType,
			&entry.TargetID,
			&entry.TargetLabel,
			&entry.Outcome,
			&entry.IPAddress,
			&entry.IPCountryCode,
			&entry.UserAgent,
			&entry.RequestID,
			&entry.Details,
		); err != nil {
			return nil, 0, fmt.Errorf("could not scan instance audit entry: %w", err)
		}

		if actorIDRaw.Valid && strings.TrimSpace(actorIDRaw.String) != "" {
			if actorID, err := uuid.Parse(actorIDRaw.String); err == nil {
				entry.ActorID = &actorID
			}
		}
		entry.TeamID = parseNullUUIDPointer(teamIDRaw)
		entry.TargetUserID = parseNullUUIDPointer(targetUserIDRaw)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("could not read instance audit entries: %w", err)
	}

	return entries, total, nil
}

func (s *Store) ExportInstanceAuditEntries(ctx context.Context, filter InstanceAuditFilter) ([]api.InstanceAuditEntry, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultInstanceAuditExportLimit
	}
	if limit > MaxInstanceAuditExportLimit {
		limit = MaxInstanceAuditExportLimit
	}

	whereClauses := make([]string, 0)
	args := make([]any, 0)

	if filter.Action != "" {
		whereClauses = append(whereClauses, "ial.action = ?")
		args = append(args, filter.Action)
	}
	if filter.ActorID != uuid.Nil {
		whereClauses = append(whereClauses, "ial.actor_id = ?")
		args = append(args, filter.ActorID)
	}
	if filter.TargetType != "" {
		whereClauses = append(whereClauses, "ial.target_type = ?")
		args = append(args, filter.TargetType)
	}
	if filter.Outcome != "" {
		whereClauses = append(whereClauses, "ial.outcome = ?")
		args = append(args, filter.Outcome)
	}
	if !filter.From.IsZero() {
		whereClauses = append(whereClauses, "ial.created_at >= ?")
		args = append(args, filter.From)
	}
	if !filter.To.IsZero() {
		whereClauses = append(whereClauses, "ial.created_at <= ?")
		args = append(args, filter.To)
	}
	if filter.Query != "" {
		q := "%" + filter.Query + "%"
		whereClauses = append(whereClauses, "(ial.actor_email_snapshot ILIKE ? OR ial.target_label ILIKE ? OR ial.details ILIKE ? OR ial.ip_address ILIKE ? OR ial.ip_country_code ILIKE ? OR ial.request_id ILIKE ?)")
		args = append(args, q, q, q, q, q, q)
	}
	args = append(args, limit)

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			ial.id,
			ial.created_at,
			CAST(ial.actor_id AS VARCHAR),
			CAST(ial.team_id AS VARCHAR),
			CAST(ial.target_user_id AS VARCHAR),
			ial.actor_email_snapshot,
			ial.actor_role_snapshot,
			ial.action,
			ial.target_type,
			ial.target_id,
			ial.target_label,
			ial.outcome,
			ial.ip_address,
			ial.ip_country_code,
			ial.user_agent,
			ial.request_id,
			ial.details
		FROM instance_audit_log ial
		`+whereSQL+`
		ORDER BY ial.created_at DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("could not export instance audit entries: %w", err)
	}
	defer rows.Close()

	var entries []api.InstanceAuditEntry
	for rows.Next() {
		var entry api.InstanceAuditEntry
		var actorIDRaw sql.NullString
		var teamIDRaw sql.NullString
		var targetUserIDRaw sql.NullString
		if err := rows.Scan(
			&entry.ID,
			&entry.CreatedAt,
			&actorIDRaw,
			&teamIDRaw,
			&targetUserIDRaw,
			&entry.ActorEmailSnapshot,
			&entry.ActorRoleSnapshot,
			&entry.Action,
			&entry.TargetType,
			&entry.TargetID,
			&entry.TargetLabel,
			&entry.Outcome,
			&entry.IPAddress,
			&entry.IPCountryCode,
			&entry.UserAgent,
			&entry.RequestID,
			&entry.Details,
		); err != nil {
			return nil, fmt.Errorf("could not scan instance audit entry: %w", err)
		}

		if actorIDRaw.Valid && strings.TrimSpace(actorIDRaw.String) != "" {
			if actorID, err := uuid.Parse(actorIDRaw.String); err == nil {
				entry.ActorID = &actorID
			}
		}
		entry.TeamID = parseNullUUIDPointer(teamIDRaw)
		entry.TargetUserID = parseNullUUIDPointer(targetUserIDRaw)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read instance audit entries: %w", err)
	}

	return entries, nil
}
