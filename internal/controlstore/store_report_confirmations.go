package controlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/reporting"
)

const (
	ReportConfirmationLifetime = 7 * 24 * time.Hour
	ReportConfirmationCooldown = 15 * time.Minute
)

var (
	ErrReportConfirmationInvalid  = errors.New("report recipient confirmation is invalid")
	ErrReportConfirmationExpired  = errors.New("report recipient confirmation has expired")
	ErrReportConfirmationCooldown = errors.New("report recipient confirmation was sent recently")
)

type PreparedReportConfirmation struct {
	RecipientID uuid.UUID
	Email       string
	Locale      string
	Token       string
	Metadata    api.ReportRecipientConfirmation
}

type ReportConfirmationAuditTarget struct {
	ReportID    uuid.UUID
	RecipientID uuid.UUID
	TenantID    uuid.UUID
}

func (s *Store) CaptureExternalReportRecipientLocale(ctx context.Context, reportID uuid.UUID, locale string, now time.Time) error {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		locale = "en"
	}
	return s.exec(ctx, `
		UPDATE report_recipients
		SET external_locale = ?, updated_at = ?
		WHERE report_id = ?
		  AND external_email IS NOT NULL
		  AND confirmation_sent_at IS NULL
		  AND confirmed_at IS NULL
	`, locale, now.UTC(), reportID)
}

func (s *Store) GetReportConfirmationAuditTarget(ctx context.Context, token string) (*ReportConfirmationAuditTarget, error) {
	var target ReportConfirmationAuditTarget
	err := s.db.QueryRowContext(ctx, `
		SELECT rr.report_id, rr.id, rd.tenant_id
		FROM report_recipients rr
		JOIN report_definitions rd ON rd.id = rr.report_id
		WHERE rr.confirmation_token_hash = ?
		  AND rr.external_email IS NOT NULL
		  AND rd.scope = 'team'
	`, reporting.ConfirmationTokenHash(token)).Scan(&target.ReportID, &target.RecipientID, &target.TenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReportConfirmationInvalid
	}
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func (s *Store) ListUnsentExternalReportRecipients(ctx context.Context, reportID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rr.id
		FROM report_recipients rr
		JOIN report_definitions rd ON rd.id = rr.report_id
		WHERE rr.report_id = ?
		  AND rr.external_email IS NOT NULL
		  AND rr.opted_out_at IS NULL
		  AND rr.confirmed_at IS NULL
		  AND rr.confirmation_sent_at IS NULL
		  AND rr.consent_version = rd.consent_version
		ORDER BY rr.created_at
	`, reportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) PrepareReportRecipientConfirmation(
	ctx context.Context,
	reportID, recipientID uuid.UUID,
	locale string,
	now time.Time,
	force bool,
) (*PreparedReportConfirmation, error) {
	report, err := s.GetReportDefinition(ctx, reportID)
	if err != nil || report.Scope != api.ReportScopeTeam || report.TenantID == nil {
		return nil, ErrReportConfirmationInvalid
	}

	var email, storedLocale string
	var recipientConsentVersion int
	var sentAt, confirmedAt, optedOutAt sql.NullTime
	err = s.db.QueryRowContext(ctx, `
		SELECT external_email, COALESCE(external_locale, 'en'), consent_version,
		       confirmation_sent_at, confirmed_at, opted_out_at
		FROM report_recipients
		WHERE id = ? AND report_id = ? AND external_email IS NOT NULL
	`, recipientID, reportID).Scan(&email, &storedLocale, &recipientConsentVersion, &sentAt, &confirmedAt, &optedOutAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReportConfirmationInvalid
	}
	if err != nil {
		return nil, err
	}
	if confirmedAt.Valid && !optedOutAt.Valid && recipientConsentVersion == report.ConsentVersion {
		return nil, ErrReportConfirmationInvalid
	}
	if !force && optedOutAt.Valid {
		return nil, ErrReportConfirmationInvalid
	}
	if sentAt.Valid && now.UTC().Sub(sentAt.Time.UTC()) < ReportConfirmationCooldown {
		return nil, ErrReportConfirmationCooldown
	}

	token, err := reporting.NewConfirmationToken()
	if err != nil {
		return nil, fmt.Errorf("generate report confirmation token: %w", err)
	}
	expiresAt := now.UTC().Add(ReportConfirmationLifetime)
	locale = strings.TrimSpace(locale)
	if locale == "" {
		locale = storedLocale
	}
	if locale == "" {
		locale = "en"
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE report_recipients
		SET external_locale = ?, consent_version = ?, confirmation_token_hash = ?,
		    confirmation_expires_at = ?, confirmation_sent_at = ?,
		    confirmation_error_code = NULL, confirmed_at = NULL, updated_at = ?
		WHERE id = ? AND report_id = ? AND external_email IS NOT NULL
	`, locale, report.ConsentVersion, reporting.ConfirmationTokenHash(token), expiresAt,
		now.UTC(), now.UTC(), recipientID, reportID)
	if err != nil {
		return nil, err
	}
	if count, err := result.RowsAffected(); err != nil || count == 0 {
		return nil, ErrReportConfirmationInvalid
	}

	team, err := s.GetTenant(ctx, *report.TenantID)
	if err != nil || team == nil {
		return nil, ErrReportConfirmationInvalid
	}
	return &PreparedReportConfirmation{
		RecipientID: recipientID,
		Email:       email,
		Locale:      locale,
		Token:       token,
		Metadata: api.ReportRecipientConfirmation{
			ReportName: report.Name,
			TeamName:   team.Name,
			Preset:     report.Preset,
			Schedule:   report.Schedule,
			Sites:      report.Sites,
			ExpiresAt:  expiresAt,
		},
	}, nil
}

func (s *Store) RecordReportConfirmationSendResult(ctx context.Context, recipientID uuid.UUID, safeErrorCode string, now time.Time) error {
	var value any
	if strings.TrimSpace(safeErrorCode) != "" {
		value = strings.TrimSpace(safeErrorCode)
	}
	return s.exec(ctx, `
		UPDATE report_recipients
		SET confirmation_error_code = ?, updated_at = ?
		WHERE id = ? AND external_email IS NOT NULL
	`, value, now.UTC(), recipientID)
}

func (s *Store) GetReportRecipientConfirmation(ctx context.Context, token string, now time.Time) (*api.ReportRecipientConfirmation, error) {
	var reportID uuid.UUID
	var expiresAt time.Time
	var recipientConsentVersion, reportConsentVersion int
	err := s.db.QueryRowContext(ctx, `
		SELECT rr.report_id, rr.confirmation_expires_at, rr.consent_version, rd.consent_version
		FROM report_recipients rr
		JOIN report_definitions rd ON rd.id = rr.report_id
		WHERE rr.confirmation_token_hash = ? AND rr.external_email IS NOT NULL
	`, reporting.ConfirmationTokenHash(token)).Scan(&reportID, &expiresAt, &recipientConsentVersion, &reportConsentVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReportConfirmationInvalid
	}
	if err != nil {
		return nil, err
	}
	if !expiresAt.After(now.UTC()) {
		return nil, ErrReportConfirmationExpired
	}
	if recipientConsentVersion != reportConsentVersion {
		return nil, ErrReportConfirmationInvalid
	}
	report, err := s.GetReportDefinition(ctx, reportID)
	if err != nil || report.Scope != api.ReportScopeTeam || report.TenantID == nil {
		return nil, ErrReportConfirmationInvalid
	}
	team, err := s.GetTenant(ctx, *report.TenantID)
	if err != nil || team == nil {
		return nil, ErrReportConfirmationInvalid
	}
	return &api.ReportRecipientConfirmation{
		ReportName: report.Name,
		TeamName:   team.Name,
		Preset:     report.Preset,
		Schedule:   report.Schedule,
		Sites:      report.Sites,
		ExpiresAt:  expiresAt.UTC(),
	}, nil
}

func (s *Store) ConfirmReportRecipient(ctx context.Context, token string, now time.Time) error {
	if _, err := s.GetReportRecipientConfirmation(ctx, token, now); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE report_recipients
		SET confirmed_at = ?, opted_out_at = NULL, confirmation_token_hash = NULL,
		    confirmation_expires_at = NULL, confirmation_error_code = NULL, updated_at = ?
		WHERE confirmation_token_hash = ?
		  AND confirmation_expires_at > ?
		  AND external_email IS NOT NULL
		  AND consent_version = (
		      SELECT rd.consent_version FROM report_definitions rd
		      WHERE rd.id = report_recipients.report_id
		  )
	`, now.UTC(), now.UTC(), reporting.ConfirmationTokenHash(token), now.UTC())
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil || count == 0 {
		return ErrReportConfirmationInvalid
	}
	return nil
}

func (s *Store) DeclineReportRecipient(ctx context.Context, token string, now time.Time) error {
	if _, err := s.GetReportRecipientConfirmation(ctx, token, now); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE report_recipients
		SET opted_out_at = ?, confirmed_at = NULL, confirmation_token_hash = NULL,
		    confirmation_expires_at = NULL, confirmation_error_code = NULL, updated_at = ?
		WHERE confirmation_token_hash = ?
		  AND confirmation_expires_at > ?
		  AND external_email IS NOT NULL
	`, now.UTC(), now.UTC(), reporting.ConfirmationTokenHash(token), now.UTC())
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil || count == 0 {
		return ErrReportConfirmationInvalid
	}
	return nil
}
