package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type GoogleSearchConsoleConnectionInput struct {
	TeamID             uuid.UUID
	ConnectedByUserID  uuid.UUID
	GoogleAccountEmail string
	GoogleAccountID    string
	AccessToken        string
	RefreshToken       string
	TokenType          string
	Scope              string
	TokenExpiry        time.Time
	ConnectedAt        time.Time
}

type GoogleSearchConsoleConnection struct {
	TeamID             uuid.UUID
	ConnectedByUserID  uuid.UUID
	GoogleAccountEmail string
	GoogleAccountID    string
	AccessToken        string
	RefreshToken       string
	TokenType          string
	Scope              string
	TokenExpiry        time.Time
	Connected          bool
	ConnectedAt        time.Time
	DisconnectedAt     *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type GoogleSearchConsolePropertyInput struct {
	TeamID          uuid.UUID
	URI             string
	PermissionLevel string
	SeenAt          time.Time
}

type GoogleSearchConsoleProperty struct {
	TeamID          uuid.UUID
	URI             string
	PermissionLevel string
	LastSeenAt      time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type GoogleSearchConsoleSiteMappingInput struct {
	SiteID      uuid.UUID
	TeamID      uuid.UUID
	PropertyURI string
	MappedBy    uuid.UUID
	MappedAt    time.Time
}

type GoogleSearchConsoleSiteMapping struct {
	SiteID      uuid.UUID
	TeamID      uuid.UUID
	PropertyURI string
	MappedBy    uuid.UUID
	MappedAt    time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type GoogleSearchConsoleSyncStateInput struct {
	SiteID            uuid.UUID
	TeamID            uuid.UUID
	State             string
	ImportedStartDate *time.Time
	ImportedEndDate   *time.Time
	LastSuccessAt     *time.Time
	LastAttemptAt     *time.Time
	LastErrorCategory string
	NextRetryAt       *time.Time
	Manual            bool
}

type GoogleSearchConsoleSyncState struct {
	SiteID            uuid.UUID
	TeamID            uuid.UUID
	State             string
	ImportedStartDate *time.Time
	ImportedEndDate   *time.Time
	LastSuccessAt     *time.Time
	LastAttemptAt     *time.Time
	LastErrorCategory string
	NextRetryAt       *time.Time
	Manual            bool
	UpdatedAt         time.Time
}

type GoogleSearchConsoleSyncCandidate struct {
	SiteID        uuid.UUID
	TeamID        uuid.UUID
	State         string
	LastSuccessAt *time.Time
	NextRetryAt   *time.Time
	Manual        bool
}

type GoogleSearchConsoleSystemStatus struct {
	ConnectedTeams      int
	MappedSites         int
	PendingSyncs        int
	RunningSyncs        int
	FailedSyncs         int
	NeedsAttentionSyncs int
	LastSuccessAt       *time.Time
	LastAttemptAt       *time.Time
	NextRetryAt         *time.Time
}

func (s *Store) UpsertGoogleSearchConsoleConnection(ctx context.Context, input GoogleSearchConsoleConnectionInput) error {
	return upsertGoogleSearchConsoleConnection(ctx, s.db, input)
}

func (s *Store) UpsertGoogleSearchConsoleConnectionWithAudit(ctx context.Context, input GoogleSearchConsoleConnectionInput, audit AuditEntryParams) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
		if err := upsertGoogleSearchConsoleConnection(ctx, tx, input); err != nil {
			return err
		}
		return appendAuditEntryTx(ctx, tx, audit)
	})
}

func upsertGoogleSearchConsoleConnection(ctx context.Context, exec sqlExecContext, input GoogleSearchConsoleConnectionInput) error {
	if input.TeamID == uuid.Nil {
		return fmt.Errorf("team id is required")
	}
	if input.ConnectedAt.IsZero() {
		input.ConnectedAt = time.Now().UTC()
	}

	_, err := exec.ExecContext(ctx, `
		INSERT INTO google_search_console_connections (
			team_id, connected_by_user_id, google_account_email, google_account_id,
			access_token, refresh_token, token_type, scope, token_expiry,
			connected, connected_at, disconnected_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, true, ?, NULL, now(), now())
		ON CONFLICT (team_id) DO UPDATE SET
			connected_by_user_id = excluded.connected_by_user_id,
			google_account_email = excluded.google_account_email,
			google_account_id = excluded.google_account_id,
			access_token = excluded.access_token,
			refresh_token = excluded.refresh_token,
			token_type = excluded.token_type,
			scope = excluded.scope,
			token_expiry = excluded.token_expiry,
			connected = true,
			connected_at = excluded.connected_at,
			disconnected_at = NULL,
			updated_at = now()
	`,
		input.TeamID,
		nullableUUID(input.ConnectedByUserID),
		strings.TrimSpace(input.GoogleAccountEmail),
		strings.TrimSpace(input.GoogleAccountID),
		strings.TrimSpace(input.AccessToken),
		strings.TrimSpace(input.RefreshToken),
		strings.TrimSpace(input.TokenType),
		strings.TrimSpace(input.Scope),
		nullableTime(input.TokenExpiry),
		input.ConnectedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("upsert Google Search Console connection: %w", err)
	}
	return nil
}

func (s *Store) GetGoogleSearchConsoleConnection(ctx context.Context, teamID uuid.UUID) (*GoogleSearchConsoleConnection, error) {
	var conn GoogleSearchConsoleConnection
	var connectedByRaw sql.NullString
	var tokenExpiry sql.NullTime
	var disconnectedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT
			team_id,
			CAST(connected_by_user_id AS VARCHAR),
			google_account_email,
			google_account_id,
			access_token,
			refresh_token,
			token_type,
			scope,
			token_expiry,
			connected,
			connected_at,
			disconnected_at,
			created_at,
			updated_at
		FROM google_search_console_connections
		WHERE team_id = ?
		LIMIT 1
	`, teamID).Scan(
		&conn.TeamID,
		&connectedByRaw,
		&conn.GoogleAccountEmail,
		&conn.GoogleAccountID,
		&conn.AccessToken,
		&conn.RefreshToken,
		&conn.TokenType,
		&conn.Scope,
		&tokenExpiry,
		&conn.Connected,
		&conn.ConnectedAt,
		&disconnectedAt,
		&conn.CreatedAt,
		&conn.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get Google Search Console connection: %w", err)
	}

	if connectedByRaw.Valid && strings.TrimSpace(connectedByRaw.String) != "" {
		if id, err := uuid.Parse(connectedByRaw.String); err == nil {
			conn.ConnectedByUserID = id
		}
	}
	if tokenExpiry.Valid {
		conn.TokenExpiry = tokenExpiry.Time.UTC()
	}
	if disconnectedAt.Valid {
		value := disconnectedAt.Time.UTC()
		conn.DisconnectedAt = &value
	}
	return &conn, nil
}

func (s *Store) DisconnectGoogleSearchConsoleConnection(ctx context.Context, teamID uuid.UUID, disconnectedAt time.Time) error {
	return disconnectGoogleSearchConsoleConnection(ctx, s.db, teamID, disconnectedAt)
}

func (s *Store) DisconnectGoogleSearchConsoleConnectionWithAudit(ctx context.Context, teamID uuid.UUID, disconnectedAt time.Time, audit AuditEntryParams) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
		if err := disconnectGoogleSearchConsoleConnection(ctx, tx, teamID, disconnectedAt); err != nil {
			return err
		}
		return appendAuditEntryTx(ctx, tx, audit)
	})
}

func disconnectGoogleSearchConsoleConnection(ctx context.Context, exec sqlExecContext, teamID uuid.UUID, disconnectedAt time.Time) error {
	if disconnectedAt.IsZero() {
		disconnectedAt = time.Now().UTC()
	}

	_, err := exec.ExecContext(ctx, `
		UPDATE google_search_console_connections
		SET
			connected = false,
			access_token = '',
			refresh_token = '',
			token_type = '',
			token_expiry = NULL,
			disconnected_at = ?,
			updated_at = now()
		WHERE team_id = ?
	`, disconnectedAt.UTC(), teamID)
	if err != nil {
		return fmt.Errorf("disconnect Google Search Console connection: %w", err)
	}
	return nil
}

func (s *Store) UpsertGoogleSearchConsoleProperty(ctx context.Context, input GoogleSearchConsolePropertyInput) error {
	return upsertGoogleSearchConsoleProperty(ctx, s.db, input)
}

func (s *Store) UpsertGoogleSearchConsolePropertiesWithAudit(ctx context.Context, inputs []GoogleSearchConsolePropertyInput, audit AuditEntryParams) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
		for _, input := range inputs {
			if err := upsertGoogleSearchConsoleProperty(ctx, tx, input); err != nil {
				return err
			}
		}
		return appendAuditEntryTx(ctx, tx, audit)
	})
}

func upsertGoogleSearchConsoleProperty(ctx context.Context, exec sqlExecContext, input GoogleSearchConsolePropertyInput) error {
	if input.TeamID == uuid.Nil {
		return fmt.Errorf("team id is required")
	}
	propertyURI := strings.TrimSpace(input.URI)
	if propertyURI == "" {
		return fmt.Errorf("property uri is required")
	}
	if input.SeenAt.IsZero() {
		input.SeenAt = time.Now().UTC()
	}

	_, err := exec.ExecContext(ctx, `
		INSERT INTO google_search_console_properties (
			team_id, property_uri, permission_level, last_seen_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, now(), now())
		ON CONFLICT (team_id, property_uri) DO UPDATE SET
			permission_level = excluded.permission_level,
			last_seen_at = excluded.last_seen_at,
			updated_at = now()
	`, input.TeamID, propertyURI, strings.TrimSpace(input.PermissionLevel), input.SeenAt.UTC())
	if err != nil {
		return fmt.Errorf("upsert Google Search Console property: %w", err)
	}
	return nil
}

func (s *Store) GetGoogleSearchConsoleProperty(ctx context.Context, teamID uuid.UUID, propertyURI string) (*GoogleSearchConsoleProperty, error) {
	var property GoogleSearchConsoleProperty
	err := s.db.QueryRowContext(ctx, `
		SELECT team_id, property_uri, permission_level, last_seen_at, created_at, updated_at
		FROM google_search_console_properties
		WHERE team_id = ? AND property_uri = ?
		LIMIT 1
	`, teamID, strings.TrimSpace(propertyURI)).Scan(
		&property.TeamID,
		&property.URI,
		&property.PermissionLevel,
		&property.LastSeenAt,
		&property.CreatedAt,
		&property.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get Google Search Console property: %w", err)
	}
	return &property, nil
}

func (s *Store) UpsertGoogleSearchConsoleSiteMapping(ctx context.Context, input GoogleSearchConsoleSiteMappingInput) error {
	return upsertGoogleSearchConsoleSiteMapping(ctx, s.db, input)
}

func (s *Store) UpsertGoogleSearchConsoleSiteMappingWithAudit(ctx context.Context, input GoogleSearchConsoleSiteMappingInput, audit AuditEntryParams) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
		if err := upsertGoogleSearchConsoleSiteMapping(ctx, tx, input); err != nil {
			return err
		}
		return appendAuditEntryTx(ctx, tx, audit)
	})
}

func upsertGoogleSearchConsoleSiteMapping(ctx context.Context, exec sqlExecContext, input GoogleSearchConsoleSiteMappingInput) error {
	if input.SiteID == uuid.Nil {
		return fmt.Errorf("site id is required")
	}
	if input.TeamID == uuid.Nil {
		return fmt.Errorf("team id is required")
	}
	propertyURI := strings.TrimSpace(input.PropertyURI)
	if propertyURI == "" {
		return fmt.Errorf("property uri is required")
	}
	if input.MappedAt.IsZero() {
		input.MappedAt = time.Now().UTC()
	}

	_, err := exec.ExecContext(ctx, `
		INSERT INTO google_search_console_site_mappings (
			site_id, team_id, property_uri, mapped_by_user_id, mapped_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, now(), now())
		ON CONFLICT (site_id) DO UPDATE SET
			team_id = excluded.team_id,
			property_uri = excluded.property_uri,
			mapped_by_user_id = excluded.mapped_by_user_id,
			mapped_at = excluded.mapped_at,
			updated_at = now()
	`, input.SiteID, input.TeamID, propertyURI, nullableUUID(input.MappedBy), input.MappedAt.UTC())
	if err != nil {
		return fmt.Errorf("upsert Google Search Console site mapping: %w", err)
	}
	return nil
}

func (s *Store) GetGoogleSearchConsoleSiteMapping(ctx context.Context, siteID uuid.UUID) (*GoogleSearchConsoleSiteMapping, error) {
	var mapping GoogleSearchConsoleSiteMapping
	var mappedByRaw sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT site_id, team_id, property_uri, CAST(mapped_by_user_id AS VARCHAR), mapped_at, created_at, updated_at
		FROM google_search_console_site_mappings
		WHERE site_id = ?
		LIMIT 1
	`, siteID).Scan(
		&mapping.SiteID,
		&mapping.TeamID,
		&mapping.PropertyURI,
		&mappedByRaw,
		&mapping.MappedAt,
		&mapping.CreatedAt,
		&mapping.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get Google Search Console site mapping: %w", err)
	}
	if mappedByRaw.Valid && strings.TrimSpace(mappedByRaw.String) != "" {
		if id, err := uuid.Parse(mappedByRaw.String); err == nil {
			mapping.MappedBy = id
		}
	}
	return &mapping, nil
}

func (s *Store) GetGoogleSearchConsoleSiteMappingForTeam(ctx context.Context, siteID, teamID uuid.UUID) (*GoogleSearchConsoleSiteMapping, error) {
	mapping, err := s.GetGoogleSearchConsoleSiteMapping(ctx, siteID)
	if err != nil || mapping == nil || mapping.TeamID != teamID {
		return nil, err
	}
	return mapping, nil
}

func (s *Store) DeleteGoogleSearchConsoleSiteMapping(ctx context.Context, siteID uuid.UUID) error {
	return deleteGoogleSearchConsoleSiteMapping(ctx, s.db, siteID)
}

func (s *Store) DeleteGoogleSearchConsoleSiteMappingWithAudit(ctx context.Context, siteID uuid.UUID, audit AuditEntryParams) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
		if err := deleteGoogleSearchConsoleSiteMapping(ctx, tx, siteID); err != nil {
			return err
		}
		return appendAuditEntryTx(ctx, tx, audit)
	})
}

func (s *Store) TransferSiteTeamWithAudit(ctx context.Context, siteID, destinationTeamID uuid.UUID, clearSearchConsoleMapping bool, audits []AuditEntryParams) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
		if err := updateSiteTenant(ctx, tx, siteID, destinationTeamID); err != nil {
			return err
		}
		if clearSearchConsoleMapping {
			if err := deleteGoogleSearchConsoleSiteMapping(ctx, tx, siteID); err != nil {
				return err
			}
		}
		for _, audit := range audits {
			if err := appendAuditEntryTx(ctx, tx, audit); err != nil {
				return err
			}
		}
		return nil
	})
}

func deleteGoogleSearchConsoleSiteMapping(ctx context.Context, exec sqlExecContext, siteID uuid.UUID) error {
	if _, err := exec.ExecContext(ctx, "DELETE FROM google_search_console_site_mappings WHERE site_id = ?", siteID); err != nil {
		return fmt.Errorf("delete Google Search Console site mapping: %w", err)
	}
	return nil
}

func (s *Store) UpsertGoogleSearchConsoleSyncState(ctx context.Context, input GoogleSearchConsoleSyncStateInput) error {
	return upsertGoogleSearchConsoleSyncState(ctx, s.db, input)
}

func (s *Store) UpsertGoogleSearchConsoleSyncStateWithAudit(ctx context.Context, input GoogleSearchConsoleSyncStateInput, audit AuditEntryParams) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
		if err := upsertGoogleSearchConsoleSyncState(ctx, tx, input); err != nil {
			return err
		}
		return appendAuditEntryTx(ctx, tx, audit)
	})
}

func upsertGoogleSearchConsoleSyncState(ctx context.Context, exec sqlExecContext, input GoogleSearchConsoleSyncStateInput) error {
	if input.SiteID == uuid.Nil {
		return fmt.Errorf("site id is required")
	}
	if input.TeamID == uuid.Nil {
		return fmt.Errorf("team id is required")
	}
	state := strings.TrimSpace(input.State)
	if state == "" {
		state = "idle"
	}
	_, err := exec.ExecContext(ctx, `
		INSERT INTO google_search_console_sync_state (
			site_id, team_id, state, imported_start_date, imported_end_date,
			last_success_at, last_attempt_at, last_error_category, next_retry_at,
			manual, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now())
		ON CONFLICT (site_id) DO UPDATE SET
			team_id = excluded.team_id,
			state = excluded.state,
			imported_start_date = excluded.imported_start_date,
			imported_end_date = excluded.imported_end_date,
			last_success_at = excluded.last_success_at,
			last_attempt_at = excluded.last_attempt_at,
			last_error_category = excluded.last_error_category,
			next_retry_at = excluded.next_retry_at,
			manual = excluded.manual,
			updated_at = now()
	`,
		input.SiteID,
		input.TeamID,
		state,
		nullableTimePtr(input.ImportedStartDate),
		nullableTimePtr(input.ImportedEndDate),
		nullableTimePtr(input.LastSuccessAt),
		nullableTimePtr(input.LastAttemptAt),
		strings.TrimSpace(input.LastErrorCategory),
		nullableTimePtr(input.NextRetryAt),
		input.Manual,
	)
	if err != nil {
		return fmt.Errorf("upsert Google Search Console sync state: %w", err)
	}
	return nil
}

func (s *Store) GetGoogleSearchConsoleSyncState(ctx context.Context, siteID uuid.UUID) (*GoogleSearchConsoleSyncState, error) {
	var state GoogleSearchConsoleSyncState
	var importedStart, importedEnd, lastSuccess, lastAttempt, nextRetry sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT site_id, team_id, state, imported_start_date, imported_end_date,
			last_success_at, last_attempt_at, last_error_category, next_retry_at,
			manual, updated_at
		FROM google_search_console_sync_state
		WHERE site_id = ?
		LIMIT 1
	`, siteID).Scan(
		&state.SiteID,
		&state.TeamID,
		&state.State,
		&importedStart,
		&importedEnd,
		&lastSuccess,
		&lastAttempt,
		&state.LastErrorCategory,
		&nextRetry,
		&state.Manual,
		&state.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get Google Search Console sync state: %w", err)
	}
	state.ImportedStartDate = nullableTimeValue(importedStart)
	state.ImportedEndDate = nullableTimeValue(importedEnd)
	state.LastSuccessAt = nullableTimeValue(lastSuccess)
	state.LastAttemptAt = nullableTimeValue(lastAttempt)
	state.NextRetryAt = nullableTimeValue(nextRetry)
	return &state, nil
}

func (s *Store) ListGoogleSearchConsoleSyncCandidates(ctx context.Context, now time.Time, limit int) ([]GoogleSearchConsoleSyncCandidate, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if limit <= 0 {
		limit = 25
	}
	recurringBefore := now.UTC().Add(-24 * time.Hour)
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			m.site_id,
			m.team_id,
			COALESCE(ss.state, 'idle') AS state,
			ss.last_success_at,
			ss.next_retry_at,
			COALESCE(ss.manual, false) AS manual
		FROM google_search_console_site_mappings m
		JOIN google_search_console_connections c
			ON c.team_id = m.team_id
			AND c.connected = true
		LEFT JOIN google_search_console_sync_state ss
			ON ss.site_id = m.site_id
		WHERE
			ss.site_id IS NULL
			OR ss.state = 'pending'
			OR (ss.state IN ('idle', 'failed') AND (ss.next_retry_at IS NULL OR ss.next_retry_at <= ?))
			OR (ss.state = 'succeeded' AND (ss.last_success_at IS NULL OR ss.last_success_at <= ?))
		ORDER BY
			COALESCE(ss.manual, false) DESC,
			ss.next_retry_at ASC NULLS FIRST,
			ss.last_success_at ASC NULLS FIRST,
			m.site_id ASC
		LIMIT ?
	`, now.UTC(), recurringBefore, limit)
	if err != nil {
		return nil, fmt.Errorf("list Google Search Console sync candidates: %w", err)
	}
	defer rows.Close()

	var candidates []GoogleSearchConsoleSyncCandidate
	for rows.Next() {
		var candidate GoogleSearchConsoleSyncCandidate
		var lastSuccess, nextRetry sql.NullTime
		if err := rows.Scan(
			&candidate.SiteID,
			&candidate.TeamID,
			&candidate.State,
			&lastSuccess,
			&nextRetry,
			&candidate.Manual,
		); err != nil {
			return nil, fmt.Errorf("scan Google Search Console sync candidate: %w", err)
		}
		candidate.LastSuccessAt = nullableTimeValue(lastSuccess)
		candidate.NextRetryAt = nullableTimeValue(nextRetry)
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read Google Search Console sync candidates: %w", err)
	}
	return candidates, nil
}

func (s *Store) GetGoogleSearchConsoleSystemStatus(ctx context.Context) (GoogleSearchConsoleSystemStatus, error) {
	var status GoogleSearchConsoleSystemStatus
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM google_search_console_connections WHERE connected = true`).Scan(&status.ConnectedTeams); err != nil {
		return status, fmt.Errorf("count Google Search Console connected teams: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM google_search_console_site_mappings`).Scan(&status.MappedSites); err != nil {
		return status, fmt.Errorf("count Google Search Console mapped sites: %w", err)
	}

	var lastSuccess, lastAttempt, nextRetry sql.NullTime
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN state = 'pending' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN state = 'running' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN state = 'failed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN state = 'needs_attention' THEN 1 ELSE 0 END), 0),
			MAX(last_success_at),
			MAX(last_attempt_at),
			MIN(next_retry_at)
		FROM google_search_console_sync_state
	`).Scan(
		&status.PendingSyncs,
		&status.RunningSyncs,
		&status.FailedSyncs,
		&status.NeedsAttentionSyncs,
		&lastSuccess,
		&lastAttempt,
		&nextRetry,
	); err != nil {
		return status, fmt.Errorf("summarize Google Search Console sync state: %w", err)
	}
	status.LastSuccessAt = nullableTimeValue(lastSuccess)
	status.LastAttemptAt = nullableTimeValue(lastAttempt)
	status.NextRetryAt = nullableTimeValue(nextRetry)
	return status, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func nullableTimePtr(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC()
}

func nullableTimeValue(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	out := value.Time.UTC()
	return &out
}
