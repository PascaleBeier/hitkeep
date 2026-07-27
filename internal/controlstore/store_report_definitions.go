package controlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/reporting"
)

var (
	ErrReportNotFound       = errors.New("report not found")
	ErrReportAccessRequired = errors.New("report access required")
)

const MaxExternalReportRecipients = 25

type reportScanner interface {
	Scan(dest ...any) error
}

func scanReportDefinition(scanner reportScanner) (*api.ReportDefinition, error) {
	var report api.ReportDefinition
	var tenantID, ownerUserID, createdBy sql.NullString
	var scope, preset, siteMode, frequency, status string
	var weeklyDay, monthlyDay sql.NullInt64
	var nextRun sql.NullTime
	if err := scanner.Scan(
		&report.ID, &tenantID, &ownerUserID, &createdBy, &report.Name,
		&scope, &preset, &siteMode, &frequency, &report.Schedule.Timezone,
		&report.Schedule.LocalTime, &weeklyDay, &monthlyDay, &status,
		&report.ConsentVersion, &nextRun, &report.CreatedAt, &report.UpdatedAt,
	); err != nil {
		return nil, err
	}
	report.Scope = api.ReportScope(scope)
	report.Preset = api.ReportPreset(preset)
	report.SiteMode = api.ReportSiteMode(siteMode)
	report.Schedule.Frequency = api.ReportFrequency(frequency)
	report.Status = api.ReportStatus(status)
	if tenantID.Valid {
		value, err := uuid.Parse(tenantID.String)
		if err != nil {
			return nil, err
		}
		report.TenantID = &value
	}
	if ownerUserID.Valid {
		value, err := uuid.Parse(ownerUserID.String)
		if err != nil {
			return nil, err
		}
		report.OwnerUserID = &value
	}
	if createdBy.Valid {
		value, err := uuid.Parse(createdBy.String)
		if err != nil {
			return nil, err
		}
		report.CreatedBy = &value
	}
	if weeklyDay.Valid {
		value := int(weeklyDay.Int64)
		report.Schedule.WeeklyDay = &value
	}
	if monthlyDay.Valid {
		value := int(monthlyDay.Int64)
		report.Schedule.MonthlyDay = &value
	}
	if nextRun.Valid {
		value := nextRun.Time.UTC()
		report.NextRunAt = &value
	}
	return &report, nil
}

const reportDefinitionSelect = `
	SELECT rd.id, CAST(rd.tenant_id AS VARCHAR), CAST(rd.owner_user_id AS VARCHAR),
	       CAST(rd.created_by AS VARCHAR), rd.name, rd.scope, rd.preset, rd.site_mode,
	       rd.frequency, rd.timezone, rd.local_time, rd.weekly_day, rd.monthly_day,
	       rd.status, rd.consent_version, rd.next_run_at, rd.created_at, rd.updated_at
	FROM report_definitions rd
`

func (s *Store) ListReportDefinitions(ctx context.Context, userID uuid.UUID) ([]api.ReportDefinition, error) {
	rows, err := s.db.QueryContext(ctx, reportDefinitionSelect+`
		WHERE rd.owner_user_id = ?
		   OR (rd.tenant_id IS NOT NULL AND (
		       EXISTS (
		           SELECT 1 FROM tenant_members tm
		           WHERE tm.tenant_id = rd.tenant_id AND tm.user_id = ?
		             AND lower(tm.role) IN ('owner', 'admin')
		       )
		       OR EXISTS (
		           SELECT 1 FROM report_recipients rr
		           WHERE rr.report_id = rd.id AND rr.user_id = ?
		       )
		   ))
		ORDER BY rd.created_at DESC
	`, userID, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	defer rows.Close()

	reports := make([]api.ReportDefinition, 0)
	for rows.Next() {
		report, err := scanReportDefinition(rows)
		if err != nil {
			return nil, fmt.Errorf("scan report: %w", err)
		}
		if err := s.loadReportRelations(ctx, report); err != nil {
			return nil, err
		}
		reports = append(reports, *report)
	}
	return reports, rows.Err()
}

func (s *Store) GetReportDefinition(ctx context.Context, reportID uuid.UUID) (*api.ReportDefinition, error) {
	report, err := scanReportDefinition(s.db.QueryRowContext(ctx, reportDefinitionSelect+" WHERE rd.id = ?", reportID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReportNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get report: %w", err)
	}
	if err := s.loadReportRelations(ctx, report); err != nil {
		return nil, err
	}
	return report, nil
}

func (s *Store) loadReportRelations(ctx context.Context, report *api.ReportDefinition) error {
	report.Sites = make([]api.ReportSite, 0)
	if err := func() error {
		siteRows, err := s.db.QueryContext(ctx, `
			SELECT s.id, s.domain
			FROM report_definition_sites rds
			JOIN sites s ON s.id = rds.site_id
			WHERE rds.report_id = ?
			ORDER BY s.domain
		`, report.ID)
		if err != nil {
			return fmt.Errorf("list report sites: %w", err)
		}
		defer siteRows.Close()
		for siteRows.Next() {
			var site api.ReportSite
			if err := siteRows.Scan(&site.ID, &site.Domain); err != nil {
				return fmt.Errorf("scan report site: %w", err)
			}
			report.Sites = append(report.Sites, site)
		}
		return siteRows.Err()
	}(); err != nil {
		return err
	}

	report.Recipients = make([]api.ReportRecipient, 0)
	if err := func() error {
		recipientRows, err := s.db.QueryContext(ctx, `
			SELECT rr.id, CAST(rr.user_id AS VARCHAR),
			       COALESCE(u.email, rr.external_email), rr.consent_version,
			       rr.confirmation_expires_at, rr.confirmation_sent_at,
			       COALESCE(rr.confirmation_error_code, ''), rr.confirmed_at, rr.opted_out_at
			FROM report_recipients rr
			LEFT JOIN users u ON u.id = rr.user_id
			WHERE rr.report_id = ?
			ORDER BY COALESCE(u.email, rr.external_email)
		`, report.ID)
		if err != nil {
			return fmt.Errorf("list report recipients: %w", err)
		}
		defer recipientRows.Close()
		for recipientRows.Next() {
			var recipient api.ReportRecipient
			var userID sql.NullString
			var recipientConsentVersion int
			var expiresAt, sentAt, confirmedAt, optedOut sql.NullTime
			var confirmationError string
			if err := recipientRows.Scan(&recipient.ID, &userID, &recipient.Email, &recipientConsentVersion,
				&expiresAt, &sentAt, &confirmationError, &confirmedAt, &optedOut); err != nil {
				return fmt.Errorf("scan report recipient: %w", err)
			}
			if userID.Valid {
				value, err := uuid.Parse(userID.String)
				if err != nil {
					return fmt.Errorf("parse report recipient user: %w", err)
				}
				recipient.UserID = &value
				recipient.Kind = api.ReportRecipientKindMember
				recipient.Status = api.ReportRecipientStatusConfirmed
			} else {
				recipient.Kind = api.ReportRecipientKindExternal
				recipient.Status = api.ReportRecipientStatusPending
				recipient.InvitationState = "pending"
				if sentAt.Valid {
					recipient.InvitationState = "sent"
				}
				if confirmationError != "" {
					recipient.InvitationState = "failed"
				}
				if expiresAt.Valid {
					value := expiresAt.Time.UTC()
					recipient.ConfirmationExpiresAt = &value
				}
				if confirmedAt.Valid {
					value := confirmedAt.Time.UTC()
					recipient.ConfirmedAt = &value
					if recipientConsentVersion == report.ConsentVersion {
						recipient.Status = api.ReportRecipientStatusConfirmed
					}
				}
			}
			if optedOut.Valid {
				value := optedOut.Time.UTC()
				recipient.OptedOutAt = &value
				recipient.Status = api.ReportRecipientStatusOptedOut
			}
			report.Recipients = append(report.Recipients, recipient)
		}
		return recipientRows.Err()
	}(); err != nil {
		return err
	}

	var last api.ReportLastOutcome
	var completed sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, status, scheduled_for, completed_at
		FROM report_runs
		WHERE report_id = ?
		ORDER BY scheduled_for DESC
		LIMIT 1
	`, report.ID).Scan(&last.RunID, &last.Status, &last.ScheduledAt, &completed)
	if err == nil {
		if completed.Valid {
			value := completed.Time.UTC()
			last.CompletedAt = &value
		}
		report.LastOutcome = &last
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("get report outcome: %w", err)
	}
	return nil
}

func (s *Store) CanManageReport(ctx context.Context, reportID, userID uuid.UUID) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM report_definitions rd
		LEFT JOIN tenant_members tm ON tm.tenant_id = rd.tenant_id AND tm.user_id = ?
		WHERE rd.id = ?
		  AND (rd.owner_user_id = ? OR lower(COALESCE(tm.role, '')) IN ('owner', 'admin'))
	`, userID, reportID, userID).Scan(&count)
	return count > 0, err
}

func (s *Store) CanViewReport(ctx context.Context, reportID, userID uuid.UUID) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM report_definitions rd
		WHERE rd.id = ? AND (
			rd.owner_user_id = ?
			OR EXISTS (
				SELECT 1 FROM tenant_members tm
				WHERE tm.tenant_id = rd.tenant_id AND tm.user_id = ?
				  AND lower(tm.role) IN ('owner', 'admin')
			)
			OR EXISTS (SELECT 1 FROM report_recipients rr WHERE rr.report_id = rd.id AND rr.user_id = ?)
		)
	`, reportID, userID, userID, userID).Scan(&count)
	return count > 0, err
}

func normalizeReportRequest(req *api.CreateReportRequest, actorID uuid.UUID) {
	req.Name = strings.TrimSpace(req.Name)
	if req.SiteMode == "" {
		req.SiteMode = api.ReportSiteModeSelected
	}
	if req.Status == "" {
		req.Status = api.ReportStatusDraft
	}
	if req.Scope == api.ReportScopePersonal {
		req.TenantID = nil
		req.RecipientUserIDs = []uuid.UUID{actorID}
	}
	req.SiteIDs = uniqueUUIDs(req.SiteIDs)
	req.RecipientUserIDs = uniqueUUIDs(req.RecipientUserIDs)
	req.ExternalRecipientEmails = uniqueNormalizedEmails(req.ExternalRecipientEmails)
}

func validateReportRequest(req api.CreateReportRequest) error {
	if req.Name == "" || len(req.Name) > 120 {
		return fmt.Errorf("invalid report name")
	}
	if err := reporting.ValidateSchedule(req.Schedule); err != nil {
		return err
	}
	if req.Scope != api.ReportScopePersonal && req.Scope != api.ReportScopeTeam {
		return fmt.Errorf("invalid report scope")
	}
	if req.Scope == api.ReportScopeTeam && (req.TenantID == nil || *req.TenantID == uuid.Nil) {
		return fmt.Errorf("team report requires tenant")
	}
	if req.Preset != api.ReportPresetSiteSummary && req.Preset != api.ReportPresetPortfolioDigest && req.Preset != api.ReportPresetOpportunityBrief {
		return fmt.Errorf("invalid report preset")
	}
	if req.Status != api.ReportStatusDraft && req.Status != api.ReportStatusActive && req.Status != api.ReportStatusPaused {
		return fmt.Errorf("invalid report status")
	}
	if req.SiteMode != api.ReportSiteModeSelected && req.SiteMode != api.ReportSiteModeAllAccessible {
		return fmt.Errorf("invalid site mode")
	}
	if req.Preset != api.ReportPresetPortfolioDigest && len(req.SiteIDs) != 1 {
		return fmt.Errorf("preset requires exactly one site")
	}
	if req.Preset == api.ReportPresetOpportunityBrief && req.Schedule.Frequency == api.ReportFrequencyMonthly {
		return fmt.Errorf("opportunity brief does not support monthly cadence")
	}
	if req.Scope == api.ReportScopeTeam && req.SiteMode == api.ReportSiteModeAllAccessible {
		return fmt.Errorf("team reports require explicit sites")
	}
	if req.SiteMode == api.ReportSiteModeSelected && len(req.SiteIDs) == 0 {
		return fmt.Errorf("selected reports require sites")
	}
	if req.Scope != api.ReportScopeTeam && len(req.ExternalRecipientEmails) > 0 {
		return fmt.Errorf("external recipients require a team report")
	}
	if len(req.ExternalRecipientEmails) > MaxExternalReportRecipients {
		return fmt.Errorf("report supports at most %d external recipients", MaxExternalReportRecipients)
	}
	for _, email := range req.ExternalRecipientEmails {
		parsed, err := mail.ParseAddress(email)
		if err != nil || !strings.EqualFold(parsed.Address, email) {
			return fmt.Errorf("invalid external recipient email")
		}
	}
	if len(req.RecipientUserIDs) == 0 && len(req.ExternalRecipientEmails) == 0 {
		return fmt.Errorf("report requires recipients")
	}
	return nil
}

func uniqueNormalizedEmails(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized != "" && !slices.Contains(result, normalized) {
			result = append(result, normalized)
		}
	}
	return result
}

func uniqueUUIDs(values []uuid.UUID) []uuid.UUID {
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if value != uuid.Nil && !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func (s *Store) CreateReportDefinition(ctx context.Context, actorID uuid.UUID, req api.CreateReportRequest, tokenSecret string) (*api.ReportDefinition, error) {
	normalizeReportRequest(&req, actorID)
	if err := validateReportRequest(req); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var nextRun *time.Time
	if req.Status == api.ReportStatusActive {
		value, err := reporting.NextOccurrence(req.Schedule, now)
		if err != nil {
			return nil, err
		}
		nextRun = &value
	}

	reportID := uuid.New()
	var ownerUserID any
	var tenantID any
	if req.Scope == api.ReportScopePersonal {
		ownerUserID = actorID
	} else {
		tenantID = *req.TenantID
	}
	siteTenants := make(map[uuid.UUID]uuid.UUID, len(req.SiteIDs))
	for _, siteID := range req.SiteIDs {
		value, err := s.GetSiteTenantID(ctx, siteID)
		if err != nil {
			return nil, err
		}
		siteTenants[siteID] = value
	}

	err := s.transact(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO report_definitions (
				id, tenant_id, owner_user_id, created_by, name, scope, preset, site_mode,
				frequency, timezone, local_time, weekly_day, monthly_day, status,
				consent_version, next_run_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
		`, reportID, tenantID, ownerUserID, actorID, req.Name, req.Scope, req.Preset, req.SiteMode,
			req.Schedule.Frequency, req.Schedule.Timezone, req.Schedule.LocalTime,
			nullableInt(req.Schedule.WeeklyDay), nullableInt(req.Schedule.MonthlyDay), req.Status,
			nextRun, now, now)
		if err != nil {
			return err
		}
		for _, siteID := range req.SiteIDs {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO report_definition_sites (id, report_id, site_id, tenant_id, created_at)
				VALUES (?, ?, ?, ?, ?)
			`, uuid.New(), reportID, siteID, siteTenants[siteID], now); err != nil {
				return err
			}
		}
		for _, userID := range req.RecipientUserIDs {
			recipientID := uuid.New()
			tokenHash := reporting.UnsubscribeTokenHash(reporting.UnsubscribeToken(tokenSecret, reportID, recipientID))
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO report_recipients (
					id, report_id, tenant_id, user_id, unsubscribe_token_hash, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?)
			`, recipientID, reportID, tenantID, userID, tokenHash, now, now); err != nil {
				return err
			}
		}
		for _, email := range req.ExternalRecipientEmails {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO report_recipients (
					id, report_id, tenant_id, external_email, external_locale,
					consent_version, created_at, updated_at
				) VALUES (?, ?, ?, ?, 'en', 1, ?, ?)
			`, uuid.New(), reportID, tenantID, email, now, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create report: %w", err)
	}
	return s.GetReportDefinition(ctx, reportID)
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func (s *Store) UpdateReportDefinition(ctx context.Context, actorID, reportID uuid.UUID, patch api.UpdateReportRequest, tokenSecret string) (*api.ReportDefinition, error) {
	current, err := s.GetReportDefinition(ctx, reportID)
	if err != nil {
		return nil, err
	}
	req := api.CreateReportRequest{
		Name: current.Name, Scope: current.Scope, TenantID: current.TenantID,
		Preset: current.Preset, SiteMode: current.SiteMode, Schedule: current.Schedule,
		Status: current.Status,
	}
	for _, site := range current.Sites {
		req.SiteIDs = append(req.SiteIDs, site.ID)
	}
	for _, recipient := range current.Recipients {
		if recipient.UserID != nil && recipient.OptedOutAt == nil {
			req.RecipientUserIDs = append(req.RecipientUserIDs, *recipient.UserID)
		}
		if recipient.Kind == api.ReportRecipientKindExternal {
			req.ExternalRecipientEmails = append(req.ExternalRecipientEmails, recipient.Email)
		}
	}
	if patch.Name != nil {
		req.Name = *patch.Name
	}
	if patch.Preset != nil {
		req.Preset = *patch.Preset
	}
	if patch.SiteMode != nil {
		req.SiteMode = *patch.SiteMode
	}
	if patch.SiteIDs != nil {
		req.SiteIDs = *patch.SiteIDs
	}
	if patch.RecipientUserIDs != nil {
		req.RecipientUserIDs = *patch.RecipientUserIDs
	}
	if patch.ExternalRecipientEmails != nil {
		req.ExternalRecipientEmails = *patch.ExternalRecipientEmails
	}
	if patch.Schedule != nil {
		req.Schedule = *patch.Schedule
	}
	if patch.Status != nil {
		req.Status = *patch.Status
	}
	normalizeReportRequest(&req, actorID)
	if err := validateReportRequest(req); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	consentVersion := current.ConsentVersion
	if consentVersion <= 0 {
		consentVersion = 1
	}
	if reportConsentScopeChanged(current, req) {
		consentVersion++
	}
	var nextRun *time.Time
	if req.Status == api.ReportStatusActive {
		value, err := reporting.NextOccurrence(req.Schedule, now)
		if err != nil {
			return nil, err
		}
		nextRun = &value
	}
	siteTenants := make(map[uuid.UUID]uuid.UUID, len(req.SiteIDs))
	for _, siteID := range req.SiteIDs {
		value, err := s.GetSiteTenantID(ctx, siteID)
		if err != nil {
			return nil, err
		}
		siteTenants[siteID] = value
	}

	err = s.transact(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			UPDATE report_definitions
			SET name = ?, preset = ?, site_mode = ?, frequency = ?, timezone = ?, local_time = ?,
			    weekly_day = ?, monthly_day = ?, status = ?, consent_version = ?, next_run_at = ?, updated_at = ?
			WHERE id = ?
		`, req.Name, req.Preset, req.SiteMode, req.Schedule.Frequency, req.Schedule.Timezone,
			req.Schedule.LocalTime, nullableInt(req.Schedule.WeeklyDay), nullableInt(req.Schedule.MonthlyDay),
			req.Status, consentVersion, nextRun, now, reportID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM report_definition_sites WHERE report_id = ?", reportID); err != nil {
			return err
		}
		for _, siteID := range req.SiteIDs {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO report_definition_sites (id, report_id, site_id, tenant_id, created_at)
				VALUES (?, ?, ?, ?, ?)
			`, uuid.New(), reportID, siteID, siteTenants[siteID], now); err != nil {
				return err
			}
		}
		for _, recipient := range current.Recipients {
			removeMember := recipient.UserID != nil && recipient.OptedOutAt == nil && !slices.Contains(req.RecipientUserIDs, *recipient.UserID)
			removeExternal := recipient.Kind == api.ReportRecipientKindExternal && !slices.Contains(req.ExternalRecipientEmails, strings.ToLower(recipient.Email))
			if removeMember || removeExternal {
				if _, err := tx.ExecContext(ctx, "DELETE FROM report_recipients WHERE id = ?", recipient.ID); err != nil {
					return err
				}
			}
		}
		if consentVersion != current.ConsentVersion {
			if _, err := tx.ExecContext(ctx, `
				UPDATE report_recipients
				SET consent_version = ?, confirmation_token_hash = NULL,
				    confirmation_expires_at = NULL, confirmation_sent_at = NULL,
				    confirmation_error_code = NULL, confirmed_at = NULL, updated_at = ?
				WHERE report_id = ? AND external_email IS NOT NULL
			`, consentVersion, now, reportID); err != nil {
				return err
			}
		}
		for _, userID := range req.RecipientUserIDs {
			recipientID := uuid.New()
			tokenHash := reporting.UnsubscribeTokenHash(reporting.UnsubscribeToken(tokenSecret, reportID, recipientID))
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO report_recipients (
					id, report_id, tenant_id, user_id, unsubscribe_token_hash, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (report_id, user_id) DO UPDATE SET
					unsubscribe_token_hash = COALESCE(report_recipients.unsubscribe_token_hash, excluded.unsubscribe_token_hash),
					updated_at = excluded.updated_at
			`, recipientID, reportID, current.TenantID, userID, tokenHash, now, now); err != nil {
				return err
			}
		}
		for _, email := range req.ExternalRecipientEmails {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO report_recipients (
					id, report_id, tenant_id, external_email, external_locale,
					consent_version, created_at, updated_at
				) VALUES (?, ?, ?, ?, 'en', ?, ?, ?)
				ON CONFLICT (report_id, external_email) DO UPDATE SET updated_at = excluded.updated_at
			`, uuid.New(), reportID, current.TenantID, email, consentVersion, now, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("update report: %w", err)
	}
	return s.GetReportDefinition(ctx, reportID)
}

func reportConsentScopeChanged(current *api.ReportDefinition, next api.CreateReportRequest) bool {
	if current.Preset != next.Preset || current.Schedule.Frequency != next.Schedule.Frequency || len(current.Sites) != len(next.SiteIDs) {
		return true
	}
	for _, site := range current.Sites {
		if !slices.Contains(next.SiteIDs, site.ID) {
			return true
		}
	}
	return false
}

func (s *Store) DeleteReportDefinition(ctx context.Context, reportID uuid.UUID) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		for _, query := range []string{
			"DELETE FROM report_deliveries WHERE report_id = ?",
			"DELETE FROM report_runs WHERE report_id = ?",
			"DELETE FROM report_recipients WHERE report_id = ?",
			"DELETE FROM report_definition_sites WHERE report_id = ?",
			"DELETE FROM report_definitions WHERE id = ?",
		} {
			if _, err := tx.ExecContext(ctx, query, reportID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ListReportRuns(ctx context.Context, reportID uuid.UUID, limit int) ([]api.ReportRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, report_id, scheduled_for, period_start, period_end, status,
		       COALESCE(safe_error_code, ''), started_at, completed_at
		FROM report_runs
		WHERE report_id = ?
		ORDER BY scheduled_for DESC
		LIMIT ?
	`, reportID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]api.ReportRun, 0)
	for rows.Next() {
		var run api.ReportRun
		var started, completed sql.NullTime
		if err := rows.Scan(&run.ID, &run.ReportID, &run.ScheduledFor, &run.PeriodStart, &run.PeriodEnd,
			&run.Status, &run.SafeErrorCode, &started, &completed); err != nil {
			return nil, err
		}
		if started.Valid {
			value := started.Time.UTC()
			run.StartedAt = &value
		}
		if completed.Valid {
			value := completed.Time.UTC()
			run.CompletedAt = &value
		}
		run.Deliveries, err = s.listReportDeliveries(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) listReportDeliveries(ctx context.Context, runID uuid.UUID) ([]api.ReportDelivery, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.recipient_id, d.recipient_kind, CAST(rr.user_id AS VARCHAR),
		       COALESCE(u.email, rr.external_email, ''), d.status, d.attempt_count,
		       d.next_attempt_at, COALESCE(d.safe_error_code, ''), d.smtp_accepted_at
		FROM report_deliveries d
		LEFT JOIN report_recipients rr ON rr.id = d.recipient_id
		LEFT JOIN users u ON u.id = rr.user_id
		WHERE d.run_id = ?
		ORDER BY d.created_at
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deliveries := make([]api.ReportDelivery, 0)
	for rows.Next() {
		var delivery api.ReportDelivery
		var kind string
		var userID sql.NullString
		var nextAttempt, accepted sql.NullTime
		if err := rows.Scan(&delivery.ID, &delivery.RecipientID, &kind, &userID, &delivery.RecipientEmail, &delivery.Status,
			&delivery.AttemptCount, &nextAttempt, &delivery.SafeErrorCode, &accepted); err != nil {
			return nil, err
		}
		delivery.RecipientKind = api.ReportRecipientKind(kind)
		if userID.Valid {
			value, err := uuid.Parse(userID.String)
			if err != nil {
				return nil, err
			}
			delivery.RecipientUserID = &value
		}
		if nextAttempt.Valid {
			value := nextAttempt.Time.UTC()
			delivery.NextAttemptAt = &value
		}
		if accepted.Valid {
			value := accepted.Time.UTC()
			delivery.SMTPAcceptedAt = &value
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func (s *Store) RetryReportRun(ctx context.Context, runID uuid.UUID, now time.Time) error {
	return s.exec(ctx, `
		UPDATE report_deliveries
		SET status = 'queued', attempt_count = 0, next_attempt_at = ?, safe_error_code = NULL, updated_at = ?
		WHERE run_id = ? AND status = 'failed'
	`, now.UTC(), now.UTC(), runID)
}

func (s *Store) GetReportIDForRun(ctx context.Context, runID uuid.UUID) (uuid.UUID, error) {
	var reportID uuid.UUID
	if err := s.db.QueryRowContext(ctx, "SELECT report_id FROM report_runs WHERE id = ?", runID).Scan(&reportID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, ErrReportNotFound
		}
		return uuid.Nil, err
	}
	return reportID, nil
}

func (s *Store) OptOutReportRecipient(ctx context.Context, reportID, recipientID uuid.UUID, tokenHash string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE report_recipients
		SET opted_out_at = COALESCE(opted_out_at, ?), updated_at = ?
		WHERE id = ? AND report_id = ? AND unsubscribe_token_hash = ?
	`, now.UTC(), now.UTC(), recipientID, reportID, tokenHash)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrReportNotFound
	}
	return nil
}

func (s *Store) ResubscribeReportRecipient(ctx context.Context, reportID, userID uuid.UUID, now time.Time) error {
	return s.exec(ctx, `
		UPDATE report_recipients
		SET opted_out_at = NULL, updated_at = ?
		WHERE report_id = ? AND user_id = ?
	`, now.UTC(), reportID, userID)
}

func (s *Store) SetReportRecipientTokenHash(ctx context.Context, recipientID uuid.UUID, tokenHash string) error {
	return s.exec(ctx, `
		UPDATE report_recipients
		SET unsubscribe_token_hash = ?, updated_at = ?
		WHERE id = ? AND (unsubscribe_token_hash IS NULL OR unsubscribe_token_hash <> ?)
	`, tokenHash, time.Now().UTC(), recipientID, tokenHash)
}

func (s *Store) EnsureReportNextRuns(ctx context.Context, now time.Time) error {
	rows, err := s.db.QueryContext(ctx, reportDefinitionSelect+`
		WHERE rd.status = 'active' AND rd.next_run_at IS NULL
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type update struct {
		id   uuid.UUID
		next time.Time
	}
	updates := make([]update, 0)
	for rows.Next() {
		report, err := scanReportDefinition(rows)
		if err != nil {
			return err
		}
		next, err := reporting.NextOccurrence(report.Schedule, now)
		if err != nil {
			return err
		}
		updates = append(updates, update{id: report.ID, next: next})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	for _, value := range updates {
		if err := s.exec(ctx, `UPDATE report_definitions SET next_run_at = ?, updated_at = ? WHERE id = ? AND next_run_at IS NULL`, value.next, now.UTC(), value.id); err != nil {
			return err
		}
	}
	return nil
}
