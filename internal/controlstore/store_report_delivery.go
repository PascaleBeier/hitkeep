package controlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/reporting"
)

type PendingReportDelivery struct {
	DeliveryID      uuid.UUID
	RunID           uuid.UUID
	ReportID        uuid.UUID
	RecipientID     uuid.UUID
	RecipientKind   api.ReportRecipientKind
	RecipientUserID *uuid.UUID
	RecipientEmail  string
	RecipientLocale string
	ScheduledFor    time.Time
	PeriodStart     time.Time
	PeriodEnd       time.Time
	AttemptCount    int
	MessageID       string
	Report          *api.ReportDefinition
}

func (s *Store) ClaimDueReportRuns(ctx context.Context, now time.Time) ([]uuid.UUID, error) {
	s.reportClaimMu.Lock()
	defer s.reportClaimMu.Unlock()

	if err := s.EnsureReportNextRuns(ctx, now); err != nil {
		return nil, err
	}
	reportIDs, err := func() ([]uuid.UUID, error) {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id FROM report_definitions
			WHERE status = 'active' AND next_run_at <= ?
			ORDER BY next_run_at
		`, now.UTC())
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []uuid.UUID
		for rows.Next() {
			var reportID uuid.UUID
			if err := rows.Scan(&reportID); err != nil {
				return nil, err
			}
			ids = append(ids, reportID)
		}
		return ids, rows.Err()
	}()
	if err != nil {
		return nil, err
	}

	createdRuns := make([]uuid.UUID, 0, len(reportIDs))
	for _, reportID := range reportIDs {
		report, err := s.GetReportDefinition(ctx, reportID)
		if errors.Is(err, ErrReportNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if report.NextRunAt == nil || report.Status != api.ReportStatusActive || report.NextRunAt.After(now) {
			continue
		}
		claimedNextRun := report.NextRunAt.UTC()
		scheduledFor := claimedNextRun
		nextRun, err := reporting.NextOccurrence(report.Schedule, scheduledFor)
		if err != nil {
			return nil, err
		}
		for !nextRun.After(now) {
			scheduledFor = nextRun
			nextRun, err = reporting.NextOccurrence(report.Schedule, nextRun)
			if err != nil {
				return nil, err
			}
		}
		periodStart, periodEnd, _, _, err := reporting.PeriodBounds(report.Schedule, scheduledFor)
		if err != nil {
			return nil, err
		}
		runID := uuid.New()
		runStatus := "queued"
		safeErrorCode := any(nil)
		if now.Sub(scheduledFor) > reporting.CatchUpWindow(report.Schedule.Frequency) {
			runStatus = "skipped"
			safeErrorCode = "catch_up_expired"
		} else if activeRecipientCount(report.Recipients) == 0 {
			runStatus = "skipped"
			safeErrorCode = "no_recipients"
		}

		claimed := false
		err = s.transact(ctx, func(tx *sql.Tx) error {
			result, err := tx.ExecContext(ctx, `
				UPDATE report_definitions
				SET next_run_at = ?, updated_at = ?
				WHERE id = ? AND status = 'active' AND next_run_at = ?
			`, nextRun, now.UTC(), report.ID, claimedNextRun)
			if err != nil {
				return err
			}
			count, err := result.RowsAffected()
			if err != nil || count == 0 {
				return err
			}
			claimed = true
			var completedAt any
			if runStatus == "skipped" {
				completedAt = now.UTC()
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO report_runs (
					id, report_id, tenant_id, scheduled_for, period_start, period_end, status,
					safe_error_code, completed_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, runID, report.ID, report.TenantID, scheduledFor, periodStart, periodEnd, runStatus,
				safeErrorCode, completedAt, now.UTC(), now.UTC()); err != nil {
				return err
			}
			if runStatus == "skipped" {
				return nil
			}
			for _, recipient := range report.Recipients {
				if recipient.OptedOutAt != nil || recipient.Status != api.ReportRecipientStatusConfirmed {
					continue
				}
				messageID := fmt.Sprintf("<report.%s.%s@hitkeep>", runID, recipient.ID)
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO report_deliveries (
						id, report_id, run_id, tenant_id, recipient_id, recipient_kind, status,
						message_id, attempt_count, next_attempt_at, created_at, updated_at
					) VALUES (?, ?, ?, ?, ?, ?, 'queued', ?, 0, ?, ?, ?)
				`, uuid.New(), report.ID, runID, report.TenantID, recipient.ID, recipient.Kind,
					messageID, now.UTC(), now.UTC(), now.UTC()); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			alreadyHandled, checkErr := s.reportClaimAlreadyHandled(ctx, report.ID, claimedNextRun, scheduledFor)
			if checkErr == nil && alreadyHandled {
				continue
			}
			return nil, fmt.Errorf("claim report %s: %w", report.ID, err)
		}
		if claimed {
			createdRuns = append(createdRuns, runID)
		}
	}
	return createdRuns, nil
}

func (s *Store) reportClaimAlreadyHandled(
	ctx context.Context,
	reportID uuid.UUID,
	claimedNextRun time.Time,
	scheduledFor time.Time,
) (bool, error) {
	var nextRun sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT next_run_at
		FROM report_definitions
		WHERE id = ?
	`, reportID).Scan(&nextRun)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if nextRun.Valid && !nextRun.Time.UTC().Equal(claimedNextRun.UTC()) {
		return true, nil
	}

	var runCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM report_runs
		WHERE report_id = ? AND scheduled_for = ?
	`, reportID, scheduledFor.UTC()).Scan(&runCount); err != nil {
		return false, err
	}
	return runCount > 0, nil
}

func activeRecipientCount(recipients []api.ReportRecipient) int {
	count := 0
	for _, recipient := range recipients {
		if recipient.OptedOutAt == nil && recipient.Status == api.ReportRecipientStatusConfirmed {
			count++
		}
	}
	return count
}

func (s *Store) ListDueReportDeliveries(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id
		FROM report_deliveries
		WHERE status IN ('queued', 'failed')
		  AND attempt_count < 4
		  AND COALESCE(next_attempt_at, created_at) <= ?
		ORDER BY COALESCE(next_attempt_at, created_at), created_at
		LIMIT ?
	`, now.UTC(), limit)
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

func (s *Store) ClaimReportDelivery(ctx context.Context, deliveryID uuid.UUID, now time.Time) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE report_deliveries
		SET status = 'sending', attempt_count = attempt_count + 1, updated_at = ?
		WHERE id = ?
		  AND status IN ('queued', 'failed')
		  AND attempt_count < 4
		  AND COALESCE(next_attempt_at, created_at) <= ?
	`, now.UTC(), deliveryID, now.UTC())
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

func (s *Store) RecoverStaleReportDeliveries(ctx context.Context, now time.Time) error {
	staleBefore := now.UTC().Add(-10 * time.Minute)
	return s.exec(ctx, `
		UPDATE report_deliveries
		SET status = 'failed', next_attempt_at = ?, safe_error_code = 'worker_interrupted', updated_at = ?
		WHERE status = 'sending' AND updated_at <= ?
	`, now.UTC(), now.UTC(), staleBefore)
}

func (s *Store) GetPendingReportDelivery(ctx context.Context, deliveryID uuid.UUID) (*PendingReportDelivery, error) {
	var delivery PendingReportDelivery
	var kind string
	var userID sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT d.id, d.run_id, d.report_id, rr.id, d.recipient_kind,
		       CAST(rr.user_id AS VARCHAR), COALESCE(u.email, rr.external_email),
		       COALESCE(up.default_locale, rr.external_locale, 'en'), r.scheduled_for, r.period_start,
		       r.period_end, d.attempt_count, d.message_id
		FROM report_deliveries d
		JOIN report_runs r ON r.id = d.run_id
		JOIN report_recipients rr ON rr.id = d.recipient_id AND rr.report_id = d.report_id
		LEFT JOIN users u ON u.id = rr.user_id
		LEFT JOIN user_preferences up ON up.user_id = rr.user_id
		WHERE d.id = ?
	`, deliveryID).Scan(&delivery.DeliveryID, &delivery.RunID, &delivery.ReportID, &delivery.RecipientID,
		&kind, &userID, &delivery.RecipientEmail, &delivery.RecipientLocale,
		&delivery.ScheduledFor, &delivery.PeriodStart, &delivery.PeriodEnd,
		&delivery.AttemptCount, &delivery.MessageID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReportNotFound
	}
	if err != nil {
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
	delivery.Report, err = s.GetReportDefinition(ctx, delivery.ReportID)
	return &delivery, err
}

func (s *Store) MarkReportDeliveryAccepted(ctx context.Context, deliveryID uuid.UUID, now time.Time) error {
	return s.exec(ctx, `
		UPDATE report_deliveries
		SET status = 'accepted', next_attempt_at = NULL, safe_error_code = NULL,
		    smtp_accepted_at = ?, updated_at = ?
		WHERE id = ?
	`, now.UTC(), now.UTC(), deliveryID)
}

func (s *Store) MarkReportDeliverySkipped(ctx context.Context, deliveryID uuid.UUID, code string, now time.Time) error {
	return s.exec(ctx, `
		UPDATE report_deliveries
		SET status = 'skipped', next_attempt_at = NULL, safe_error_code = ?, updated_at = ?
		WHERE id = ?
	`, code, now.UTC(), deliveryID)
}

func (s *Store) MarkReportDeliveryFailed(ctx context.Context, deliveryID uuid.UUID, attempt int, code string, now time.Time) error {
	delays := []time.Duration{5 * time.Minute, 30 * time.Minute, 2 * time.Hour}
	status := "failed"
	var nextAttempt any
	if attempt >= 1 && attempt <= len(delays) {
		nextAttempt = now.Add(delays[attempt-1]).UTC()
	}
	return s.exec(ctx, `
		UPDATE report_deliveries
		SET status = ?, next_attempt_at = ?, safe_error_code = ?, updated_at = ?
		WHERE id = ?
	`, status, nextAttempt, code, now.UTC(), deliveryID)
}

func (s *Store) FinalizeReportRun(ctx context.Context, runID uuid.UUID, now time.Time) error {
	var queued, sending, accepted, failed, skipped int
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			count(*) FILTER (WHERE status = 'queued'),
			count(*) FILTER (WHERE status = 'sending'),
			count(*) FILTER (WHERE status = 'accepted'),
			count(*) FILTER (WHERE status = 'failed'),
			count(*) FILTER (WHERE status = 'skipped')
		FROM report_deliveries WHERE run_id = ?
	`, runID).Scan(&queued, &sending, &accepted, &failed, &skipped); err != nil {
		return err
	}
	if queued > 0 || sending > 0 || (failed > 0 && s.reportRunHasRetryableDelivery(ctx, runID)) {
		return s.exec(ctx, `UPDATE report_runs SET status = 'running', started_at = COALESCE(started_at, ?), updated_at = ? WHERE id = ?`, now.UTC(), now.UTC(), runID)
	}
	status := "completed"
	var code any
	switch {
	case accepted > 0 && failed > 0:
		status = "partial"
		code = "delivery_failed"
	case failed > 0:
		status = "failed"
		code = "delivery_failed"
	case accepted == 0 && skipped > 0:
		status = "skipped"
		code = "no_eligible_recipients"
	}
	return s.exec(ctx, `
		UPDATE report_runs
		SET status = ?, safe_error_code = ?, started_at = COALESCE(started_at, ?),
		    completed_at = ?, updated_at = ?
		WHERE id = ?
	`, status, code, now.UTC(), now.UTC(), now.UTC(), runID)
}

func (s *Store) reportRunHasRetryableDelivery(ctx context.Context, runID uuid.UUID) bool {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM report_deliveries
		WHERE run_id = ? AND status = 'failed' AND attempt_count < 4 AND next_attempt_at IS NOT NULL
	`, runID).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

func (s *Store) ResolveReportSites(ctx context.Context, report *api.ReportDefinition, userID uuid.UUID) ([]api.ReportSite, error) {
	if report.SiteMode == api.ReportSiteModeSelected {
		return report.Sites, nil
	}
	sites, err := s.listReportAccessibleSites(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]api.ReportSite, 0, len(sites))
	for _, site := range sites {
		result = append(result, api.ReportSite{ID: site.SiteID, Domain: site.Domain})
	}
	return result, nil
}

func (s *Store) RecipientCanAccessReportSites(ctx context.Context, report *api.ReportDefinition, userID uuid.UUID) (bool, error) {
	sites, err := s.ResolveReportSites(ctx, report, userID)
	if err != nil {
		return false, err
	}
	if len(sites) == 0 {
		return false, nil
	}
	for _, site := range sites {
		allowed, err := s.CanAccessSiteForReports(ctx, userID, site.ID)
		if err != nil {
			return false, err
		}
		if !allowed {
			return false, nil
		}
	}
	return true, nil
}

func (s *Store) ExternalRecipientCanAccessReportSites(ctx context.Context, report *api.ReportDefinition) (bool, error) {
	if report == nil || report.Scope != api.ReportScopeTeam || report.TenantID == nil || report.SiteMode != api.ReportSiteModeSelected || len(report.Sites) == 0 {
		return false, nil
	}
	var activeTenantCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM tenants t
		LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
		WHERE t.id = ? AND ta.tenant_id IS NULL
	`, *report.TenantID).Scan(&activeTenantCount); err != nil {
		return false, err
	}
	if activeTenantCount == 0 {
		return false, nil
	}
	for _, site := range report.Sites {
		tenantID, err := s.GetSiteTenantID(ctx, site.ID)
		if err != nil || tenantID != *report.TenantID {
			return false, err
		}
	}
	return true, nil
}
