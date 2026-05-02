package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/worker"
)

// testMailable satisfies mailer.Mailable for sending a test email.
type testMailable struct {
	subject string
	link    string
}

func (m *testMailable) Subject() string  { return m.subject }
func (m *testMailable) Template() string { return "password_reset.mjml" }
func (m *testMailable) Data() any        { return map[string]string{"Link": m.link} }

func (h *handler) handleRefreshSpamFilter() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if h.ctx.SpamFilter == nil {
			h.appendAudit(r, "spam_filter.refresh", "system", "", "", "failure", "Spam filter not available")
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "error", "message": "Spam filter not available",
			})
			return
		}

		if err := h.ctx.SpamFilter.Update(ctx); err != nil {
			slog.Error("Failed to refresh spam filter", "error", err)
			h.appendAudit(r, "spam_filter.refresh", "system", "", "", "failure", err.Error())
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"status": "error", "message": "Failed to refresh spam filter: " + err.Error(),
			})
			return
		}

		h.appendAudit(r, "spam_filter.refresh", "system", "", "", "success", "Spam filter refreshed manually")
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok", "message": "Spam filter refreshed successfully",
		})
	}
}

func (h *handler) handleRunImportStageCleanup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			h.appendAudit(r, "import_stage_cleanup.run", "system", "", "", "failure", "Store not available")
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "error", "message": "Store not available",
			})
			return
		}
		if h.ctx.Config.ImportStageRetentionDays <= 0 {
			h.appendAudit(r, "import_stage_cleanup.run", "system", "", "", "failure", "Import stage cleanup is disabled")
			writeJSON(w, http.StatusConflict, map[string]string{
				"status": "error", "message": "Import stage cleanup is disabled",
			})
			return
		}

		result, err := worker.RunImportStageCleanup(
			r.Context(),
			h.ctx.Store,
			h.ctx.Config.DataPath,
			h.ctx.Config.ImportStageRetentionDays,
			h.ctx.ImportStageCleanupStatus,
		)
		if err != nil {
			slog.Error("Failed to clean import staging files", "error", err)
			h.appendAudit(r, "import_stage_cleanup.run", "system", "", "", "failure", err.Error())
			writeJSON(w, http.StatusInternalServerError, api.SystemImportStageCleanupRunResponse{
				Status:  "error",
				Message: "Import stage cleanup failed: " + err.Error(),
				Result:  result,
			})
			return
		}

		details := fmt.Sprintf(
			"Import stage cleanup removed %d staged file(s), cleared %d byte(s), and marked %d stale import(s) failed",
			result.FilesCleaned,
			result.BytesCleaned,
			result.ImportsMarkedFailed,
		)
		h.appendAudit(r, "import_stage_cleanup.run", "system", "", "", "success", details)
		writeJSON(w, http.StatusOK, api.SystemImportStageCleanupRunResponse{
			Status:  "ok",
			Message: "Import stage cleanup completed",
			Result:  result,
		})
	}
}

func (h *handler) handleTestMail() http.HandlerFunc {
	type request struct {
		Email string `json:"email"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Mailer == nil {
			h.appendAudit(r, "mail.test", "system", "", "", "failure", "Mailer not configured")
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "error", "message": "Mailer not configured",
			})
			return
		}

		actorID := shared.GetUserIDFromContext(r)
		actorEmail := "unknown"
		if actorID != uuid.Nil {
			user, err := h.ctx.Store.GetUserByID(r.Context(), actorID)
			if err == nil && user != nil {
				actorEmail = user.Email
			}
		}

		var req request
		if r.Body != nil && r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"status": "error", "message": "Invalid request body",
				})
				return
			}
		}

		recipient := strings.TrimSpace(req.Email)
		if recipient == "" {
			recipient = actorEmail
		}
		parsedRecipient, err := mail.ParseAddress(recipient)
		if err != nil || parsedRecipient == nil || strings.TrimSpace(parsedRecipient.Address) == "" {
			h.appendAudit(r, "mail.test", "mail", "", recipient, "failure", "Invalid test email recipient")
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"status": "error", "message": "Enter a valid email address",
			})
			return
		}
		recipient = strings.TrimSpace(parsedRecipient.Address)

		mailable := &testMailable{
			subject: "HitKeep System Test Email — " + time.Now().UTC().Format(time.RFC3339),
			link:    h.ctx.Config.PublicURL + "/admin/system",
		}
		err = h.ctx.Mailer.Send(recipient, mailable)

		if err != nil {
			slog.Error("Failed to send test email", "error", err)
			h.appendAudit(r, "mail.test", "mail", "", recipient, "failure", err.Error())
			if h.ctx.MailTestTracker != nil {
				h.ctx.MailTestTracker.SetResult(false)
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"status": "error", "message": "Failed to send test email: " + err.Error(),
			})
			return
		}

		h.appendAudit(r, "mail.test", "mail", "", recipient, "success", "Test email sent to "+recipient)
		if h.ctx.MailTestTracker != nil {
			h.ctx.MailTestTracker.SetResult(true)
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok", "message": "Test email sent successfully to " + recipient,
		})
	}
}
