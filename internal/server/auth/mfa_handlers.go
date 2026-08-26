package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/appurl"
	"hitkeep/internal/database"
	"hitkeep/internal/mailables"
	"hitkeep/internal/mailer"
	"hitkeep/internal/security"
	"hitkeep/internal/server/shared"
	json "hitkeep/jsonapi"
)

type mfaTotpVerifyRequest struct {
	ChallengeToken string `json:"challenge_token"`
	Code           string `json:"code"`
}

type mfaEmailLinkRequest struct {
	ChallengeToken string `json:"challenge_token"`
	ReturnURL      string `json:"return_url,omitempty"`
}

func mfaAuditDetails(challenge database.LoginChallenge, reason string) string {
	details := "reason=" + strings.TrimSpace(reason)
	if provider := strings.TrimSpace(challenge.AuthProvider); provider != "" {
		details = "provider=" + provider + ";" + details
	}
	return details
}

func (h *handler) loadMFAChallenge(w http.ResponseWriter, r *http.Request, challengeToken string) (database.LoginChallenge, bool) {
	challengeID, err := uuid.Parse(strings.TrimSpace(challengeToken))
	if err != nil {
		http.Error(w, "Invalid challenge token", http.StatusBadRequest)
		return database.LoginChallenge{}, false
	}

	if h.ctx.AuthState == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return database.LoginChallenge{}, false
	}
	challenge, found := h.ctx.AuthState.GetPasskeyLoginChallenge(challengeID)
	if !found {
		http.Error(w, "MFA challenge not found", http.StatusConflict)
		return database.LoginChallenge{}, false
	}
	if strings.TrimSpace(challenge.Flow) != "mfa" || !challenge.HasUserID {
		http.Error(w, "Invalid MFA challenge", http.StatusForbidden)
		return database.LoginChallenge{}, false
	}

	if time.Now().UTC().After(challenge.ExpiresAt.UTC()) {
		h.ctx.AuthState.DeletePasskeyLoginChallenge(challengeID)
		http.Error(w, "MFA challenge expired", http.StatusGone)
		return database.LoginChallenge{}, false
	}

	return challenge, true
}

func (h *handler) handleMFATOTPVerify() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req mfaTotpVerifyRequest
		if !decodeAuthJSON(w, r, &req, false) {
			return
		}

		challenge, ok := h.loadMFAChallenge(w, r, req.ChallengeToken)
		if !ok {
			return
		}

		secret, enabled, err := h.ctx.Store.GetUserTOTPSecret(r.Context(), challenge.UserID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load user totp secret for mfa verify", "error", err, "user_id", challenge.UserID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !enabled {
			http.Error(w, "TOTP is not enabled", http.StatusConflict)
			return
		}
		if !security.ValidateTOTPCodeStrict(secret, req.Code, time.Now().UTC()) {
			http.Error(w, "Invalid TOTP code", http.StatusUnauthorized)
			return
		}

		challengeID, err := uuid.Parse(strings.TrimSpace(req.ChallengeToken))
		if err != nil {
			http.Error(w, "Invalid challenge token", http.StatusBadRequest)
			return
		}
		h.ctx.AuthState.DeletePasskeyLoginChallenge(challengeID)

		if err := h.issueLoginSession(r.Context(), w, challenge.UserID, challenge.RememberMe); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to issue login session after mfa totp verification", "error", err, "user_id", challenge.UserID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.appendAuthAuditForUserTeams(r, challenge.UserID, "auth.mfa_succeeded", "success", mfaAuditDetails(challenge, "totp_succeeded"), true)
		h.appendAuthAuditForUserTeams(r, challenge.UserID, "auth.login_succeeded", "success", mfaAuditDetails(challenge, "mfa_succeeded"), true)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.MarshalWrite(w, loginResponse{Status: "ok"}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode mfa totp verification response", "error", err, "user_id", challenge.UserID)
		}
	}
}

func (h *handler) handleMFARecoveryCodeVerify() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req mfaTotpVerifyRequest
		if !decodeAuthJSON(w, r, &req, false) {
			return
		}

		challenge, ok := h.loadMFAChallenge(w, r, req.ChallengeToken)
		if !ok {
			return
		}

		_, consumed, err := h.ctx.Store.ConsumeRecoveryCode(r.Context(), challenge.UserID, req.Code)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to consume recovery code for mfa verify", "error", err, "user_id", challenge.UserID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !consumed {
			http.Error(w, "Invalid recovery code", http.StatusUnauthorized)
			return
		}

		challengeID, err := uuid.Parse(strings.TrimSpace(req.ChallengeToken))
		if err != nil {
			http.Error(w, "Invalid challenge token", http.StatusBadRequest)
			return
		}
		h.ctx.AuthState.DeletePasskeyLoginChallenge(challengeID)

		if err := h.issueLoginSession(r.Context(), w, challenge.UserID, challenge.RememberMe); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to issue login session after recovery code verification", "error", err, "user_id", challenge.UserID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.appendAuthAuditForUserTeams(r, challenge.UserID, "auth.mfa_succeeded", "success", mfaAuditDetails(challenge, "recovery_code_succeeded"), true)
		h.appendAuthAuditForUserTeams(r, challenge.UserID, "auth.login_succeeded", "success", mfaAuditDetails(challenge, "mfa_succeeded"), true)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.MarshalWrite(w, loginResponse{Status: "ok"}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode recovery code verification response", "error", err, "user_id", challenge.UserID)
		}
	}
}

func (h *handler) handleMFAEmailLinkRequest() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Mailer == nil {
			http.Error(w, "Email sign-in is not available", http.StatusServiceUnavailable)
			return
		}

		var req mfaEmailLinkRequest
		if !decodeAuthJSON(w, r, &req, false) {
			return
		}

		challenge, ok := h.loadMFAChallenge(w, r, req.ChallengeToken)
		if !ok {
			return
		}
		challengeID, err := uuid.Parse(strings.TrimSpace(req.ChallengeToken))
		if err != nil {
			http.Error(w, "Invalid challenge token", http.StatusBadRequest)
			return
		}

		user, err := h.ctx.Store.GetUserByID(r.Context(), challenge.UserID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load user for mfa email link", "error", err, "user_id", challenge.UserID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, "MFA challenge not found", http.StatusConflict)
			return
		}

		tokenID := h.ctx.AuthState.CreateMFAEmailLink(challengeID, sanitizeAuthReturnPath(req.ReturnURL), challenge.ExpiresAt)
		expiresInMinutes := max(int(time.Until(challenge.ExpiresAt).Minutes()), 1)
		verifyURL := appurl.Path(h.ctx.Config.PublicURL, "/api/auth/mfa/email-link/verify?token="+tokenID.String())
		locale := h.preferredMailLocale(r, user.ID)

		if err := h.ctx.Mailer.Send(user.Email, mailables.NewMFAMagicLink(verifyURL, locale, expiresInMinutes)); err != nil {
			details := mailer.DescribeError(err)
			shared.LoggerFromContext(r.Context()).Error("Failed to send mfa email link", "error_code", "smtp_send_failed", "error_stage", details.Stage, "error_kind", details.Kind, "error_message", details.Message, "smtp_code", details.SMTPCode, "user_id", user.ID)
			http.Error(w, "Failed to send email sign-in link", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.MarshalWrite(w, map[string]string{"status": "sent"}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode mfa email link response", "error", err, "user_id", user.ID)
		}
	}
}

func (h *handler) handleMFAEmailLinkVerify() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenID, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("token")))
		if err != nil {
			http.Redirect(w, r, h.loginErrorRedirectURL("mfa_link_invalid"), http.StatusSeeOther)
			return
		}
		if h.ctx.AuthState == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		link, ok := h.ctx.AuthState.ConsumeMFAEmailLink(tokenID)
		if !ok {
			http.Redirect(w, r, h.loginErrorRedirectURL("mfa_link_invalid"), http.StatusSeeOther)
			return
		}

		challenge, ok := h.ctx.AuthState.GetPasskeyLoginChallenge(link.ChallengeID)
		if !ok || strings.TrimSpace(challenge.Flow) != "mfa" || !challenge.HasUserID {
			http.Redirect(w, r, h.loginErrorRedirectURL("mfa_link_invalid"), http.StatusSeeOther)
			return
		}
		if time.Now().UTC().After(challenge.ExpiresAt.UTC()) {
			h.ctx.AuthState.DeletePasskeyLoginChallenge(link.ChallengeID)
			http.Redirect(w, r, h.loginErrorRedirectURL("mfa_link_invalid"), http.StatusSeeOther)
			return
		}

		if err := h.issueLoginSession(r.Context(), w, challenge.UserID, challenge.RememberMe); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to issue login session after mfa email link verification", "error", err, "user_id", challenge.UserID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.appendAuthAuditForUserTeams(r, challenge.UserID, "auth.mfa_succeeded", "success", mfaAuditDetails(challenge, "email_link_succeeded"), true)
		h.appendAuthAuditForUserTeams(r, challenge.UserID, "auth.login_succeeded", "success", mfaAuditDetails(challenge, "mfa_succeeded"), true)

		h.ctx.AuthState.DeletePasskeyLoginChallenge(link.ChallengeID)
		http.Redirect(w, r, h.publicRedirectURL(link.ReturnPath), http.StatusSeeOther)
	}
}
