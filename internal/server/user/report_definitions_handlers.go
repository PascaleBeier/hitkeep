package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/appurl"
	"hitkeep/internal/database"
	"hitkeep/internal/mailables"
	"hitkeep/internal/mailer"
	"hitkeep/internal/reporting"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/worker"
)

func decodeReportJSON(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

func writeReportError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message}); err != nil {
		slog.Debug("Failed to encode report error response", "error_code", code)
	}
}

func (h *handler) handleReportGetAction() http.HandlerFunc {
	runs := h.ctx.Handler(shared.HandlerConfig{RequireAuth: true, RateLimiter: h.ctx.ApiLimiter}, h.handleListReportRuns())
	unsubscribe := h.ctx.Handler(shared.HandlerConfig{RateLimiter: h.ctx.ApiLimiter}, h.handleUnsubscribeReport())
	return func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("report_id") == "unsubscribe" {
			r.SetPathValue("opaque_token", r.PathValue("action"))
			unsubscribe.ServeHTTP(w, r)
			return
		}
		if r.PathValue("action") == "runs" {
			runs.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	}
}

func (h *handler) handleReportPostAction() http.HandlerFunc {
	testSend := h.ctx.Handler(shared.HandlerConfig{RequireAuth: true, HumanOnly: true, RateLimiter: h.ctx.ApiLimiter}, h.handleTestSendReport())
	resubscribe := h.ctx.Handler(shared.HandlerConfig{RequireAuth: true, HumanOnly: true, RateLimiter: h.ctx.ApiLimiter}, h.handleResubscribeReport())
	unsubscribe := h.ctx.Handler(shared.HandlerConfig{RateLimiter: h.ctx.ApiLimiter}, h.handleUnsubscribeReport())
	return func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("report_id") == "unsubscribe" {
			r.SetPathValue("opaque_token", r.PathValue("action"))
			unsubscribe.ServeHTTP(w, r)
			return
		}
		switch r.PathValue("action") {
		case "test-send":
			testSend.ServeHTTP(w, r)
		case "resubscribe":
			resubscribe.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func (h *handler) handleListReports() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		reports, err := h.ctx.Store.ListReportDefinitions(r.Context(), userID)
		if err != nil {
			slog.Error("Failed to list reports", "user_id", userID, "error_code", "report_list_failed")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		for index := range reports {
			manageable, manageErr := h.ctx.Store.CanManageReport(r.Context(), reports[index].ID, userID)
			if manageErr == nil && !manageable {
				reports[index].Recipients = currentReportRecipient(reports[index].Recipients, userID)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reports)
	}
}

func (h *handler) handleCreateReport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		var req api.CreateReportRequest
		if err := decodeReportJSON(w, r, &req); err != nil {
			writeReportError(w, http.StatusBadRequest, "invalid_request", "Invalid report definition")
			return
		}
		var err error
		req, err = h.canonicalizeReportRecipients(r, req)
		if err != nil {
			writeReportAuthorizationError(w, err)
			return
		}
		if req.Status == api.ReportStatusActive && h.ctx.Mailer == nil {
			writeReportError(w, http.StatusConflict, "mail_unavailable", "Mail delivery is unavailable; save this report as a draft")
			return
		}
		if err := h.authorizeReportRequest(r, userID, req); err != nil {
			writeReportAuthorizationError(w, err)
			return
		}
		report, err := h.ctx.Store.CreateReportDefinition(r.Context(), userID, req, h.ctx.Config.JWTSecret)
		if err != nil {
			writeReportError(w, http.StatusBadRequest, "invalid_report", err.Error())
			return
		}
		if report.TenantID != nil {
			h.appendReportAudit(r, *report.TenantID, userID, report.ID, "report.created", "Report created")
		}
		h.sendUnsentReportConfirmations(r, report.ID, h.reportRecipientLocale(r, userID))
		if refreshed, refreshErr := h.ctx.Store.GetReportDefinition(r.Context(), report.ID); refreshErr == nil {
			report = refreshed
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(report)
	}
}

func (h *handler) handleGetReport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reportID, ok := reportIDFromPath(w, r)
		if !ok {
			return
		}
		userID := shared.GetUserIDFromContext(r)
		allowed, err := h.ctx.Store.CanViewReport(r.Context(), reportID, userID)
		if err != nil || !allowed {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		report, err := h.ctx.Store.GetReportDefinition(r.Context(), reportID)
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		manageable, manageErr := h.ctx.Store.CanManageReport(r.Context(), reportID, userID)
		if manageErr == nil && !manageable {
			report.Recipients = currentReportRecipient(report.Recipients, userID)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(report)
	}
}

func (h *handler) handleUpdateReport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reportID, ok := reportIDFromPath(w, r)
		if !ok {
			return
		}
		userID := shared.GetUserIDFromContext(r)
		allowed, err := h.ctx.Store.CanManageReport(r.Context(), reportID, userID)
		if err != nil || !allowed {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		var patch api.UpdateReportRequest
		if err := decodeReportJSON(w, r, &patch); err != nil {
			writeReportError(w, http.StatusBadRequest, "invalid_request", "Invalid report update")
			return
		}
		current, err := h.ctx.Store.GetReportDefinition(r.Context(), reportID)
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if patch.Status != nil && *patch.Status == api.ReportStatusActive && h.ctx.Mailer == nil {
			writeReportError(w, http.StatusConflict, "mail_unavailable", "Mail delivery is unavailable; keep this report as a draft")
			return
		}
		merged := mergeReportPatch(current, patch)
		merged, err = h.canonicalizeReportRecipients(r, merged)
		if err != nil {
			writeReportAuthorizationError(w, err)
			return
		}
		externalRecipientsAllowed := true
		authorizationRequest := merged
		if len(merged.ExternalRecipientEmails) > 0 && merged.TenantID != nil && !h.ctx.Limits().AllowsExternalReportRecipients(r.Context(), userID, *merged.TenantID) {
			externalRecipientsAllowed = false
			if !reportUpdatePreservesExistingExternalScope(current, merged) {
				writeReportAuthorizationError(w, errReportPlanUpgradeRequired)
				return
			}
			// Existing external recipients may be retained while the manager
			// pauses, renames, reschedules, or removes recipients after a
			// downgrade. They are already normalized and are not deliverable.
			authorizationRequest.ExternalRecipientEmails = nil
		}
		if err := h.authorizeReportRequest(r, userID, authorizationRequest); err != nil {
			writeReportAuthorizationError(w, err)
			return
		}
		patch.RecipientUserIDs = &merged.RecipientUserIDs
		patch.ExternalRecipientEmails = &merged.ExternalRecipientEmails
		report, err := h.ctx.Store.UpdateReportDefinition(r.Context(), userID, reportID, patch, h.ctx.Config.JWTSecret)
		if err != nil {
			writeReportError(w, http.StatusBadRequest, "invalid_report", err.Error())
			return
		}
		if report.TenantID != nil {
			h.appendReportAudit(r, *report.TenantID, userID, report.ID, "report.updated", "Report updated")
		}
		if externalRecipientsAllowed {
			h.sendUnsentReportConfirmations(r, report.ID, h.reportRecipientLocale(r, userID))
		}
		if refreshed, refreshErr := h.ctx.Store.GetReportDefinition(r.Context(), report.ID); refreshErr == nil {
			report = refreshed
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(report)
	}
}

func (h *handler) handleDeleteReport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reportID, ok := reportIDFromPath(w, r)
		if !ok {
			return
		}
		userID := shared.GetUserIDFromContext(r)
		allowed, err := h.ctx.Store.CanManageReport(r.Context(), reportID, userID)
		if err != nil || !allowed {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		report, _ := h.ctx.Store.GetReportDefinition(r.Context(), reportID)
		if err := h.ctx.Store.DeleteReportDefinition(r.Context(), reportID); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if report != nil && report.TenantID != nil {
			h.appendReportAudit(r, *report.TenantID, userID, reportID, "report.deleted", "Report deleted")
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) handlePreviewReport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		var req api.ReportPreviewRequest
		if err := decodeReportJSON(w, r, &req); err != nil {
			writeReportError(w, http.StatusBadRequest, "invalid_request", "Invalid preview request")
			return
		}
		var err error
		req.Definition, err = h.canonicalizeReportRecipients(r, req.Definition)
		if err != nil {
			writeReportAuthorizationError(w, err)
			return
		}
		if err := h.authorizeReportRequest(r, userID, req.Definition); err != nil {
			writeReportAuthorizationError(w, err)
			return
		}
		next, err := reporting.NextOccurrence(req.Definition.Schedule, time.Now().UTC())
		if err != nil {
			writeReportError(w, http.StatusBadRequest, "invalid_schedule", "Invalid report schedule")
			return
		}
		start, end, _, _, err := reporting.PeriodBounds(req.Definition.Schedule, next)
		if err != nil {
			writeReportError(w, http.StatusBadRequest, "invalid_schedule", "Invalid report schedule")
			return
		}
		report := &api.ReportDefinition{
			Name: req.Definition.Name, Scope: req.Definition.Scope, Preset: req.Definition.Preset,
			SiteMode: req.Definition.SiteMode, Schedule: req.Definition.Schedule, Status: api.ReportStatusDraft,
		}
		for _, siteID := range req.Definition.SiteIDs {
			site, siteErr := h.ctx.Store.GetSiteByID(r.Context(), siteID)
			if siteErr != nil || site == nil {
				writeReportError(w, http.StatusBadRequest, "invalid_site", "Invalid report site")
				return
			}
			report.Sites = append(report.Sites, api.ReportSite{ID: site.ID, Domain: site.Domain})
		}
		locale := h.reportRecipientLocale(r, userID)
		email, shouldSend, err := h.reportContentBuilder().Build(r.Context(), worker.ReportContentRequest{
			Report: report, RecipientUserID: userID, RecipientLocale: locale,
			ScheduledFor: next, PeriodStart: start, PeriodEnd: end,
		})
		if err != nil {
			writeReportError(w, http.StatusUnprocessableEntity, "content_unavailable", "Report content is unavailable")
			return
		}
		subject := req.Definition.Name
		if shouldSend && email != nil {
			subject = email.Subject()
		}
		siteCount := len(req.Definition.SiteIDs)
		if req.Definition.SiteMode == api.ReportSiteModeAllAccessible {
			// The concrete personal portfolio is resolved again at send time.
			siteCount = 0
		}
		preview := api.ReportPreview{
			Subject: subject, Preset: req.Definition.Preset, Schedule: req.Definition.Schedule,
			SiteCount: siteCount, RecipientCount: len(req.Definition.RecipientUserIDs),
			PendingRecipientCount: len(req.Definition.ExternalRecipientEmails),
			PeriodStart:           start, PeriodEnd: end, Suppressed: !shouldSend,
		}
		if req.ReportID != nil {
			if manageable, manageErr := h.ctx.Store.CanManageReport(r.Context(), *req.ReportID, userID); manageErr == nil && manageable {
				if current, currentErr := h.ctx.Store.GetReportDefinition(r.Context(), *req.ReportID); currentErr == nil {
					preview.PendingRecipientCount = 0
					preview.RecipientCount = len(req.Definition.RecipientUserIDs)
					for _, email := range req.Definition.ExternalRecipientEmails {
						deliverable := false
						for _, recipient := range current.Recipients {
							if recipient.Kind == api.ReportRecipientKindExternal && strings.EqualFold(recipient.Email, email) &&
								recipient.Status == api.ReportRecipientStatusConfirmed && recipient.OptedOutAt == nil {
								deliverable = true
								break
							}
						}
						if deliverable {
							preview.RecipientCount++
						} else {
							preview.PendingRecipientCount++
						}
					}
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(preview)
	}
}

func (h *handler) handleTestSendReport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Mailer == nil {
			writeReportError(w, http.StatusConflict, "mail_unavailable", "Mail delivery is unavailable")
			return
		}
		reportID, ok := reportIDFromPath(w, r)
		if !ok {
			return
		}
		userID := shared.GetUserIDFromContext(r)
		allowed, err := h.ctx.Store.CanManageReport(r.Context(), reportID, userID)
		if err != nil || !allowed {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		report, err := h.ctx.Store.GetReportDefinition(r.Context(), reportID)
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		user, err := h.ctx.Store.GetUserByID(r.Context(), userID)
		if err != nil || user == nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		next, err := reporting.NextOccurrence(report.Schedule, time.Now().UTC())
		if err != nil {
			writeReportError(w, http.StatusUnprocessableEntity, "invalid_schedule", "Report schedule is invalid")
			return
		}
		start, end, _, _, err := reporting.PeriodBounds(report.Schedule, next)
		if err != nil {
			writeReportError(w, http.StatusUnprocessableEntity, "invalid_schedule", "Report schedule is invalid")
			return
		}
		email, shouldSend, err := h.reportContentBuilder().Build(r.Context(), worker.ReportContentRequest{
			Report: report, RecipientUserID: userID, RecipientLocale: h.reportRecipientLocale(r, userID),
			ScheduledFor: next, PeriodStart: start, PeriodEnd: end,
		})
		if err != nil {
			writeReportError(w, http.StatusUnprocessableEntity, "content_unavailable", "Report content is unavailable")
			return
		}
		if !shouldSend || email == nil {
			writeReportError(w, http.StatusUnprocessableEntity, "empty_report_suppressed", "This report has no content to send")
			return
		}
		messageID := fmt.Sprintf("<report-test.%s@hitkeep>", uuid.New())
		if err := h.ctx.Mailer.SendWithOptions(user.Email, email, mailer.SendOptions{MessageID: messageID, Headers: map[string]string{"Auto-Submitted": "auto-generated"}}); err != nil {
			writeReportError(w, http.StatusBadGateway, "smtp_send_failed", "Mail server did not accept the test message")
			return
		}
		if report.TenantID != nil {
			h.appendReportAudit(r, *report.TenantID, userID, report.ID, "report.test_sent", "Report test accepted by mail server")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.ReportTestSendResponse{Status: "accepted", MessageID: messageID, SentAt: time.Now().UTC()})
	}
}

func (h *handler) reportContentBuilder() *worker.ReportContentBuilder {
	return worker.NewReportContentBuilder(h.ctx.Store, h.ctx.AnalyticsStore, h.ctx.Config.PublicURL)
}

func (h *handler) reportRecipientLocale(r *http.Request, userID uuid.UUID) string {
	locale := "en"
	if prefs, err := h.ctx.Store.GetUserPreferences(r.Context(), userID); err == nil && prefs != nil && prefs.DefaultLocale != "" {
		locale = prefs.DefaultLocale
	}
	return locale
}

func (h *handler) handleListReportRuns() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reportID, ok := reportIDFromPath(w, r)
		if !ok {
			return
		}
		userID := shared.GetUserIDFromContext(r)
		allowed, err := h.ctx.Store.CanViewReport(r.Context(), reportID, userID)
		if err != nil || !allowed {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		runs, err := h.ctx.Store.ListReportRuns(r.Context(), reportID, 25)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		manageable, manageErr := h.ctx.Store.CanManageReport(r.Context(), reportID, userID)
		if manageErr == nil && !manageable {
			for index := range runs {
				runs[index].Deliveries = currentReportDeliveries(runs[index].Deliveries, userID)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(runs)
	}
}

func (h *handler) handleRetryReportRun() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := uuid.Parse(r.PathValue("run_id"))
		if err != nil {
			http.Error(w, "Invalid run ID", http.StatusBadRequest)
			return
		}
		reportID, err := h.ctx.Store.GetReportIDForRun(r.Context(), runID)
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		userID := shared.GetUserIDFromContext(r)
		allowed, err := h.ctx.Store.CanManageReport(r.Context(), reportID, userID)
		if err != nil || !allowed {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if h.ctx.Mailer == nil {
			writeReportError(w, http.StatusConflict, "mail_unavailable", "Mail delivery is unavailable")
			return
		}
		if err := h.ctx.Store.RetryReportRun(r.Context(), runID, time.Now().UTC()); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if report, _ := h.ctx.Store.GetReportDefinition(r.Context(), reportID); report != nil && report.TenantID != nil {
			h.appendReportAudit(r, *report.TenantID, userID, report.ID, "report.retry_requested", "Report retry requested")
		}
		w.WriteHeader(http.StatusAccepted)
	}
}

func (h *handler) handleUnsubscribeReport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("opaque_token")
		reportID, recipientID, ok := reporting.VerifyUnsubscribeToken(h.ctx.Config.JWTSecret, token)
		if !ok {
			http.Error(w, "Invalid unsubscribe link", http.StatusBadRequest)
			return
		}
		if err := h.ctx.Store.OptOutReportRecipient(r.Context(), reportID, recipientID, reporting.UnsubscribeTokenHash(token), time.Now().UTC()); err != nil {
			http.Error(w, "Invalid unsubscribe link", http.StatusBadRequest)
			return
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><html><body><main><h1>Report unsubscribed</h1><p>You will no longer receive this report. You can resubscribe from HitKeep Reporting.</p></main></body></html>")
	}
}

func (h *handler) handleGetReportRecipientConfirmation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		confirmation, err := h.ctx.Store.GetReportRecipientConfirmation(r.Context(), r.PathValue("opaque_token"), time.Now().UTC())
		if err != nil {
			if errors.Is(err, database.ErrReportConfirmationExpired) {
				writeReportError(w, http.StatusGone, "confirmation_expired", "This confirmation link has expired")
				return
			}
			writeReportError(w, http.StatusBadRequest, "confirmation_invalid", "This confirmation link is invalid")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(confirmation)
	}
}

func (h *handler) handleConfirmReportRecipient() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("opaque_token")
		auditTarget, _ := h.ctx.Store.GetReportConfirmationAuditTarget(r.Context(), token)
		var req api.ReportRecipientConfirmationRequest
		if r.ContentLength > 0 {
			if err := decodeReportJSON(w, r, &req); err != nil {
				writeReportError(w, http.StatusBadRequest, "invalid_request", "Invalid confirmation action")
				return
			}
		}
		var err error
		switch req.Action {
		case "decline":
			err = h.ctx.Store.DeclineReportRecipient(r.Context(), token, time.Now().UTC())
		case "", "confirm":
			err = h.ctx.Store.ConfirmReportRecipient(r.Context(), token, time.Now().UTC())
		default:
			writeReportError(w, http.StatusBadRequest, "invalid_request", "Invalid confirmation action")
			return
		}
		if err != nil {
			if errors.Is(err, database.ErrReportConfirmationExpired) {
				writeReportError(w, http.StatusGone, "confirmation_expired", "This confirmation link has expired")
				return
			}
			writeReportError(w, http.StatusBadRequest, "confirmation_invalid", "This confirmation link is invalid")
			return
		}
		if auditTarget != nil {
			state := "confirmed"
			if req.Action == "decline" {
				state = "declined"
			}
			h.appendReportRecipientAudit(r, auditTarget.TenantID, uuid.Nil, auditTarget.ReportID, auditTarget.RecipientID, "report.recipient_consent_"+state, state)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) handleResendReportRecipientConfirmation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Mailer == nil {
			writeReportError(w, http.StatusConflict, "mail_unavailable", "Mail delivery is unavailable")
			return
		}
		reportID, ok := reportIDFromPath(w, r)
		if !ok {
			return
		}
		recipientID, err := uuid.Parse(r.PathValue("recipient_id"))
		if err != nil {
			writeReportError(w, http.StatusBadRequest, "invalid_recipient", "Invalid report recipient")
			return
		}
		actorID := shared.GetUserIDFromContext(r)
		allowed, err := h.ctx.Store.CanManageReport(r.Context(), reportID, actorID)
		if err != nil || !allowed {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		report, err := h.ctx.Store.GetReportDefinition(r.Context(), reportID)
		if err != nil || report.TenantID == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if !h.ctx.Limits().AllowsExternalReportRecipients(r.Context(), actorID, *report.TenantID) {
			writeReportError(w, http.StatusForbidden, "plan_upgrade_required", "External report recipients require the Pro plan or higher")
			return
		}
		prepared, err := h.ctx.Store.PrepareReportRecipientConfirmation(
			r.Context(), reportID, recipientID, h.reportRecipientLocale(r, actorID), time.Now().UTC(), true,
		)
		if err != nil {
			switch {
			case errors.Is(err, database.ErrReportConfirmationCooldown):
				writeReportError(w, http.StatusTooManyRequests, "confirmation_recently_sent", "A confirmation email was sent recently")
			case errors.Is(err, database.ErrReportConfirmationInvalid):
				http.Error(w, "Not found", http.StatusNotFound)
			default:
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
		if err := h.sendPreparedReportConfirmation(r, prepared); err != nil {
			writeReportError(w, http.StatusBadGateway, "smtp_send_failed", "Mail server did not accept the confirmation email")
			return
		}
		h.appendReportRecipientAudit(r, *report.TenantID, actorID, reportID, recipientID, "report.recipient_confirmation_resent", "pending")
		w.WriteHeader(http.StatusAccepted)
	}
}

func (h *handler) sendUnsentReportConfirmations(r *http.Request, reportID uuid.UUID, locale string) {
	if err := h.ctx.Store.CaptureExternalReportRecipientLocale(r.Context(), reportID, locale, time.Now().UTC()); err != nil {
		slog.Warn("Failed to capture report recipient locale", "report_id", reportID, "error_code", "confirmation_locale_failed")
	}
	if h.ctx.Mailer == nil {
		return
	}
	recipientIDs, err := h.ctx.Store.ListUnsentExternalReportRecipients(r.Context(), reportID)
	if err != nil {
		slog.Warn("Failed to list pending report confirmations", "report_id", reportID, "error_code", "confirmation_list_failed")
		return
	}
	for _, recipientID := range recipientIDs {
		prepared, err := h.ctx.Store.PrepareReportRecipientConfirmation(r.Context(), reportID, recipientID, locale, time.Now().UTC(), false)
		if err != nil {
			slog.Warn("Failed to prepare report confirmation", "report_id", reportID, "recipient_id", recipientID, "error_code", "confirmation_prepare_failed")
			continue
		}
		_ = h.sendPreparedReportConfirmation(r, prepared)
	}
}

func (h *handler) sendPreparedReportConfirmation(r *http.Request, prepared *database.PreparedReportConfirmation) error {
	if prepared == nil || h.ctx.Mailer == nil {
		return mailer.ErrMailerDisabled
	}
	link := appurl.Path(h.ctx.Config.PublicURL, "/report-confirmation") + "?token=" + url.QueryEscape(prepared.Token)
	email := mailables.NewReportRecipientConfirmation(link, prepared.Locale, prepared.Metadata)
	if err := h.ctx.Mailer.Send(prepared.Email, email); err != nil {
		_ = h.ctx.Store.RecordReportConfirmationSendResult(r.Context(), prepared.RecipientID, "smtp_send_failed", time.Now().UTC())
		slog.Warn("Mail server did not accept report confirmation", "recipient_id", prepared.RecipientID, "error_code", "smtp_send_failed")
		return err
	}
	return h.ctx.Store.RecordReportConfirmationSendResult(r.Context(), prepared.RecipientID, "", time.Now().UTC())
}

func (h *handler) handleResubscribeReport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reportID, ok := reportIDFromPath(w, r)
		if !ok {
			return
		}
		userID := shared.GetUserIDFromContext(r)
		allowed, err := h.ctx.Store.CanViewReport(r.Context(), reportID, userID)
		if err != nil || !allowed {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		report, err := h.ctx.Store.GetReportDefinition(r.Context(), reportID)
		if err != nil || len(currentReportRecipient(report.Recipients, userID)) == 0 {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if err := h.ctx.Store.ResubscribeReportRecipient(r.Context(), reportID, userID, time.Now().UTC()); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func currentReportRecipient(recipients []api.ReportRecipient, userID uuid.UUID) []api.ReportRecipient {
	for _, recipient := range recipients {
		if recipient.UserID != nil && *recipient.UserID == userID {
			return []api.ReportRecipient{recipient}
		}
	}
	return []api.ReportRecipient{}
}

func currentReportDeliveries(deliveries []api.ReportDelivery, userID uuid.UUID) []api.ReportDelivery {
	for _, delivery := range deliveries {
		if delivery.RecipientUserID != nil && *delivery.RecipientUserID == userID {
			return []api.ReportDelivery{delivery}
		}
	}
	return []api.ReportDelivery{}
}

func reportIDFromPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	reportID, err := uuid.Parse(r.PathValue("report_id"))
	if err != nil {
		http.Error(w, "Invalid report ID", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return reportID, true
}

func mergeReportPatch(current *api.ReportDefinition, patch api.UpdateReportRequest) api.CreateReportRequest {
	req := api.CreateReportRequest{
		Name: current.Name, Scope: current.Scope, TenantID: current.TenantID,
		Preset: current.Preset, SiteMode: current.SiteMode, Schedule: current.Schedule, Status: current.Status,
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
	return req
}

func reportUpdatePreservesExistingExternalScope(current *api.ReportDefinition, next api.CreateReportRequest) bool {
	if current == nil || current.Preset != next.Preset || current.Schedule.Frequency != next.Schedule.Frequency || len(current.Sites) != len(next.SiteIDs) {
		return false
	}
	for _, site := range current.Sites {
		if !slices.Contains(next.SiteIDs, site.ID) {
			return false
		}
	}
	currentEmails := make([]string, 0, len(current.Recipients))
	for _, recipient := range current.Recipients {
		if recipient.Kind == api.ReportRecipientKindExternal {
			currentEmails = append(currentEmails, strings.ToLower(strings.TrimSpace(recipient.Email)))
		}
	}
	for _, email := range next.ExternalRecipientEmails {
		if !slices.Contains(currentEmails, strings.ToLower(strings.TrimSpace(email))) {
			return false
		}
	}
	return true
}

var errReportForbidden = errors.New("report forbidden")
var errReportInvalidRecipient = errors.New("invalid report recipient")
var errReportPlanUpgradeRequired = errors.New("report plan upgrade required")

func (h *handler) canonicalizeReportRecipients(r *http.Request, req api.CreateReportRequest) (api.CreateReportRequest, error) {
	if req.Scope != api.ReportScopeTeam || req.TenantID == nil {
		return req, nil
	}
	members, err := h.ctx.Store.ListTeamMembers(r.Context(), *req.TenantID)
	if err != nil {
		return req, err
	}
	memberByEmail := make(map[string]uuid.UUID, len(members))
	for _, member := range members {
		memberByEmail[strings.ToLower(strings.TrimSpace(member.Email))] = member.UserID
	}
	external := make([]string, 0, len(req.ExternalRecipientEmails))
	for _, raw := range req.ExternalRecipientEmails {
		email := strings.ToLower(strings.TrimSpace(raw))
		if memberID, ok := memberByEmail[email]; ok {
			req.RecipientUserIDs = append(req.RecipientUserIDs, memberID)
			continue
		}
		if email != "" && !slices.Contains(external, email) {
			external = append(external, email)
		}
	}
	req.RecipientUserIDs = uniqueReportUserIDs(req.RecipientUserIDs)
	req.ExternalRecipientEmails = external
	return req, nil
}

func uniqueReportUserIDs(values []uuid.UUID) []uuid.UUID {
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if value != uuid.Nil && !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func (h *handler) authorizeReportRequest(r *http.Request, actorID uuid.UUID, req api.CreateReportRequest) error {
	if req.Scope == api.ReportScopePersonal {
		if len(req.ExternalRecipientEmails) > 0 {
			return errReportInvalidRecipient
		}
		for _, siteID := range req.SiteIDs {
			allowed, err := h.ctx.Store.CanAccessSiteForReports(r.Context(), actorID, siteID)
			if err != nil || !allowed {
				return errReportForbidden
			}
		}
		return nil
	}
	if req.Scope != api.ReportScopeTeam || req.TenantID == nil {
		return errors.New("invalid report scope")
	}
	if len(req.ExternalRecipientEmails) > database.MaxExternalReportRecipients {
		return errReportInvalidRecipient
	}
	for _, raw := range req.ExternalRecipientEmails {
		email := strings.ToLower(strings.TrimSpace(raw))
		parsed, err := mail.ParseAddress(email)
		if err != nil || !strings.EqualFold(parsed.Address, email) {
			return errReportInvalidRecipient
		}
	}
	members, err := h.ctx.Store.ListTeamMembers(r.Context(), *req.TenantID)
	if err != nil {
		return err
	}
	memberIDs := make(map[uuid.UUID]api.TeamMember, len(members))
	for _, member := range members {
		memberIDs[member.UserID] = member
	}
	actor, found := memberIDs[actorID]
	if !found || !canManageTeam(actor.Role) {
		return errReportForbidden
	}
	if len(req.ExternalRecipientEmails) > 0 && !h.ctx.Limits().AllowsExternalReportRecipients(r.Context(), actorID, *req.TenantID) {
		return errReportPlanUpgradeRequired
	}
	for _, siteID := range req.SiteIDs {
		tenantID, err := h.ctx.Store.GetSiteTenantID(r.Context(), siteID)
		if err != nil || tenantID != *req.TenantID {
			return errReportForbidden
		}
		allowed, err := h.ctx.Store.CanAccessSiteForReports(r.Context(), actorID, siteID)
		if err != nil || !allowed {
			return errReportForbidden
		}
	}
	for _, recipientID := range req.RecipientUserIDs {
		if _, ok := memberIDs[recipientID]; !ok {
			return errReportInvalidRecipient
		}
		for _, siteID := range req.SiteIDs {
			allowed, err := h.ctx.Store.CanAccessSiteForReports(r.Context(), recipientID, siteID)
			if err != nil || !allowed {
				return errReportInvalidRecipient
			}
		}
	}
	return nil
}

func writeReportAuthorizationError(w http.ResponseWriter, err error) {
	if errors.Is(err, errReportPlanUpgradeRequired) {
		writeReportError(w, http.StatusForbidden, "plan_upgrade_required", "External report recipients require the Pro plan or higher")
		return
	}
	if errors.Is(err, errReportForbidden) {
		writeReportError(w, http.StatusForbidden, "forbidden", "You cannot manage this report")
		return
	}
	if errors.Is(err, errReportInvalidRecipient) {
		writeReportError(w, http.StatusBadRequest, "invalid_recipient", "Recipients must be eligible team members or valid external addresses on a team report")
		return
	}
	writeReportError(w, http.StatusBadRequest, "invalid_report", "Invalid report definition")
}

func (h *handler) appendReportAudit(r *http.Request, teamID, actorID, reportID uuid.UUID, action, details string) {
	h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
		ActorID: actorID, TeamID: teamID, Action: action, TargetType: "report",
		TargetID: reportID.String(), Outcome: "success", Details: strings.TrimSpace(details),
	})
}

func (h *handler) appendReportRecipientAudit(r *http.Request, teamID, actorID, reportID, recipientID uuid.UUID, action, state string) {
	h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
		ActorID: actorID, TeamID: teamID, Action: action, TargetType: "report_recipient",
		TargetID: recipientID.String(), Outcome: "success",
		Details: fmt.Sprintf("report_id=%s state=%s", reportID, strings.TrimSpace(state)),
	})
}
