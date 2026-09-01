package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"hitkeep/appurl"
	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/mailables"
	"hitkeep/internal/mailer"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/webhooks"
	json "hitkeep/jsonapi"
)

func (h *handler) handleForgotPassword() http.HandlerFunc {
	type request struct {
		Email string `json:"email"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.UnmarshalRead(r.Body, &req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		user, err := h.ctx.Store.GetUserByEmail(r.Context(), req.Email)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Database error checking user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if user == nil {
			w.WriteHeader(http.StatusOK)
			if err := json.MarshalWrite(w, map[string]string{"message": "If an account exists, a reset link has been sent."}); err != nil {
				shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
			}
			return
		}

		token, err := h.ctx.Store.CreatePasswordResetToken(r.Context(), user.Email)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to create reset token", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		resetLink := appurl.Path(h.ctx.Config.PublicURL, "/reset-password?token="+token)
		locale := h.preferredMailLocale(r, user.ID)

		if h.ctx.Mailer == nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to send password reset email", "error_code", "mailer_unavailable")
		} else {
			err = h.ctx.Mailer.Send(user.Email, mailables.NewPasswordReset(resetLink, locale))
			if err != nil {
				details := mailer.DescribeError(err)
				shared.LoggerFromContext(r.Context()).Error("Failed to send password reset email", "error_code", "smtp_send_failed", "error_stage", details.Stage, "error_kind", details.Kind, "error_message", details.Message, "smtp_code", details.SMTPCode)
			} else {
				shared.LoggerFromContext(r.Context()).Info("Password reset requested", "user_id", user.ID)
			}
		}

		w.WriteHeader(http.StatusOK)
		if err := json.MarshalWrite(w, map[string]string{"message": "If an account exists, a reset link has been sent."}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleResetPassword() http.HandlerFunc {
	type request struct {
		Token string `json:"token"`
		//nolint:gosec // request payload intentionally accepts plaintext password input.
		Password string `json:"password"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.UnmarshalRead(r.Body, &req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Token == "" || len(req.Password) < 8 {
			http.Error(w, "Invalid token or password too short", http.StatusBadRequest)
			return
		}

		hashedPassword, err := HashPassword(req.Password)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to hash password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		err = h.ctx.Store.CompletePasswordReset(r.Context(), req.Token, hashedPassword)
		if err != nil {
			if errors.Is(err, database.ErrPasswordResetInvalid) || errors.Is(err, database.ErrPasswordResetExpired) {
				http.Error(w, "Invalid or expired link", http.StatusBadRequest)
				return
			}

			shared.LoggerFromContext(r.Context()).Error("Failed to complete password reset", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		shared.LoggerFromContext(r.Context()).Info("Password reset successful")

		w.WriteHeader(http.StatusOK)
		err = json.MarshalWrite(w, map[string]string{"status": "ok", "message": "Password updated successfully"})
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to complete password reset", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
}
func (h *handler) handleAcceptInvite() http.HandlerFunc {
	type request struct {
		Token string `json:"token"`
		//nolint:gosec // request payload intentionally accepts plaintext password input.
		Password string `json:"password"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.UnmarshalRead(r.Body, &req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if strings.TrimSpace(req.Token) == "" {
			http.Error(w, "Invalid token", http.StatusBadRequest)
			return
		}

		email, err := h.ctx.Store.ResolvePasswordResetEmail(r.Context(), req.Token)
		if err != nil {
			if errors.Is(err, database.ErrPasswordResetInvalid) || errors.Is(err, database.ErrPasswordResetExpired) {
				http.Error(w, "Invalid or expired link", http.StatusBadRequest)
				return
			}
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve invite token", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		user, err := h.ctx.Store.GetUserByEmail(r.Context(), email)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load invited user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, "Invalid or expired link", http.StatusBadRequest)
			return
		}

		pendingInvites, err := h.ctx.Store.ListPendingTeamInvitesByEmail(r.Context(), email)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to list pending invites", "error", err, "user_id", user.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if len(pendingInvites) == 0 {
			http.Error(w, "Invalid or expired link", http.StatusBadRequest)
			return
		}

		authenticatedUserID := h.userIDFromLogoutRequest(r)
		if authenticatedUserID != uuid.Nil {
			authenticatedUser, err := h.ctx.Store.GetUserByID(r.Context(), authenticatedUserID)
			if err != nil {
				shared.LoggerFromContext(r.Context()).Error("Failed to load authenticated invite user", "error", err, "user_id", authenticatedUserID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if authenticatedUser == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			if !strings.EqualFold(authenticatedUser.Email, email) {
				http.Error(w, "Invite does not match signed-in user", http.StatusForbidden)
				return
			}

			if err := h.validateCloudInviteAcceptance(r.Context(), email, authenticatedUser.ID, pendingInvites); err != nil {
				h.writeInviteAcceptanceError(r.Context(), w, err, authenticatedUser.ID)
				return
			}

			acceptedEmail, acceptedInvites, err := h.ctx.Store.AcceptInviteForAuthenticatedUser(r.Context(), req.Token, authenticatedUser.ID)
			if err != nil {
				h.writeInviteAcceptanceError(r.Context(), w, err, authenticatedUser.ID)
				return
			}
			h.appendInviteAcceptedAuditEvents(r, authenticatedUser.ID, acceptedEmail, acceptedInvites)

			shared.LoggerFromContext(r.Context()).Info("Invite accepted by existing user", "user_id", authenticatedUser.ID)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.MarshalWrite(w, loginResponse{Status: "ok"}); err != nil {
				shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
			}
			return
		}

		requiresPasswordSetup := false
		for _, invite := range pendingInvites {
			if invite.RequiresPasswordSetup {
				requiresPasswordSetup = true
				break
			}
		}
		if !requiresPasswordSetup {
			http.Error(w, "Sign in to accept this invitation", http.StatusUnauthorized)
			return
		}

		if len(req.Password) < 8 {
			http.Error(w, "Invalid token or password too short", http.StatusBadRequest)
			return
		}

		if err := h.validateCloudInviteAcceptance(r.Context(), email, user.ID, pendingInvites); err != nil {
			h.writeInviteAcceptanceError(r.Context(), w, err, user.ID)
			return
		}

		hashedPassword, err := HashPassword(req.Password)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to hash password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		acceptedEmail, userID, acceptedInvites, err := h.ctx.Store.AcceptInviteWithPassword(r.Context(), req.Token, hashedPassword)
		if err != nil {
			h.writeInviteAcceptanceError(r.Context(), w, err, user.ID)
			return
		}
		h.appendInviteAcceptedAuditEvents(r, userID, acceptedEmail, acceptedInvites)

		if err := h.issueLoginSession(r.Context(), w, userID, false); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to issue invite acceptance session", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.appendAuthAuditForUserTeams(r, userID, "auth.login_succeeded", "success", "Login succeeded after accepting an invitation", true)

		shared.LoggerFromContext(r.Context()).Info("Invite accepted", "user_id", userID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.MarshalWrite(w, loginResponse{Status: "ok"}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) validateCloudInviteAcceptance(ctx context.Context, email string, userID uuid.UUID, pendingInvites []api.TeamInvite) error {
	if !h.ctx.Config.CloudHosted {
		return nil
	}

	targetTeams := make(map[uuid.UUID]struct{})
	for _, invite := range pendingInvites {
		isMember, err := h.ctx.Store.IsTenantMember(ctx, invite.TeamID, userID)
		if err != nil {
			return fmt.Errorf("check cloud invite membership for team %s: %w", invite.TeamID, err)
		}
		if !isMember {
			targetTeams[invite.TeamID] = struct{}{}
		}
	}

	if len(targetTeams) == 0 {
		return nil
	}
	return h.ctx.Limits().RequireTeamMembershipCapacity(ctx, userID, len(targetTeams))
}

func (h *handler) writeInviteAcceptanceError(ctx context.Context, w http.ResponseWriter, err error, userID uuid.UUID) {
	switch {
	case errors.Is(err, database.ErrPasswordResetInvalid), errors.Is(err, database.ErrPasswordResetExpired), errors.Is(err, database.ErrTeamInviteNotFound):
		http.Error(w, "Invalid or expired link", http.StatusBadRequest)
	case errors.Is(err, database.ErrTeamInviteLoginRequired):
		http.Error(w, "Sign in to accept this invitation", http.StatusUnauthorized)
	case errors.Is(err, database.ErrTeamInviteEmailMismatch):
		http.Error(w, "Invite does not match signed-in user", http.StatusForbidden)
	case errors.Is(err, entitlements.ErrTeamMembershipLimitReached):
		http.Error(w, "Managed cloud accounts are limited to one team", http.StatusForbidden)
	default:
		shared.LoggerFromContext(ctx).Error("Failed to accept invite", "error", err, "user_id", userID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *handler) appendInviteAcceptedAuditEvents(r *http.Request, userID uuid.UUID, email string, acceptedInvites []api.TeamInvite) {
	for _, invite := range acceptedInvites {
		h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
			ActorID:      userID,
			TeamID:       invite.TeamID,
			TargetUserID: userID,
			Action:       "member.invite_accepted",
			TargetType:   "user",
			TargetID:     userID.String(),
			TargetLabel:  email,
			Outcome:      "success",
			Details:      fmt.Sprintf("Invitation accepted by %s", email),
		})
		h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
			ActorID:      userID,
			TeamID:       invite.TeamID,
			TargetUserID: userID,
			Action:       "member.added",
			TargetType:   "user",
			TargetID:     userID.String(),
			TargetLabel:  email,
			Outcome:      "success",
			Details:      fmt.Sprintf("Member %s added after accepting an invitation", email),
		})
		h.ctx.EmitWebhookEvent(r.Context(), webhooks.Event{
			Type: webhooks.EventTeamMemberAdded,
			Data: map[string]any{
				"team_id": invite.TeamID.String(),
				"user_id": userID.String(),
			},
		})
	}
}

func (h *handler) handleChangePassword() http.HandlerFunc {
	type request struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if h.ctx.Store == nil {
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}

		var req request
		if err := json.UnmarshalRead(r.Body, &req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		if len(req.NewPassword) < 8 {
			http.Error(w, "New password must be at least 8 characters", http.StatusBadRequest)
			return
		}

		user, err := h.ctx.Store.GetUserByID(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to fetch user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		match, err := verifyPassword(req.CurrentPassword, user.Password)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Error verifying password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !match {
			http.Error(w, "Current password is incorrect", http.StatusForbidden)
			return
		}

		newHash, err := HashPassword(req.NewPassword)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to hash new password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := h.ctx.Store.UpdatePasswordByID(r.Context(), userID.String(), newHash); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to update password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		shared.LoggerFromContext(r.Context()).Info("User changed password", "user_id", userID)
		w.WriteHeader(http.StatusOK)
		_ = json.MarshalWrite(w, map[string]string{"status": "ok"})
	}
}

func (h *handler) userIDFromLogoutRequest(r *http.Request) uuid.UUID {
	userID := shared.GetUserIDFromContext(r)
	if userID != uuid.Nil {
		return userID
	}
	if h.ctx == nil || h.ctx.Store == nil || h.ctx.Config == nil {
		return uuid.Nil
	}

	if cookie, err := r.Cookie(authcore.CookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		if claims, err := authcore.ValidateTokenClaims(cookie.Value, h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL); err == nil && claims != nil {
			return claims.UserID
		}
	}
	if cookie, err := r.Cookie(authcore.RememberMeCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		if rememberedUserID, err := h.ctx.Store.ValidateRememberMeToken(r.Context(), cookie.Value); err == nil {
			return rememberedUserID
		}
	}
	return uuid.Nil
}

func (h *handler) handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isSecure := strings.HasPrefix(h.ctx.Config.PublicURL, "https://")
		userID := h.userIDFromLogoutRequest(r)

		if cookie, err := r.Cookie(authcore.RememberMeCookieName); err == nil {
			if err := h.ctx.Store.DeleteRememberMeToken(r.Context(), cookie.Value); err != nil {
				shared.LoggerFromContext(r.Context()).Error("Failed to delete remember me token", "error", err)
			}
		}

		authcore.ClearCookies(w, isSecure)
		if userID != uuid.Nil {
			h.appendAuthAuditForUserTeams(r, userID, "auth.logout", "success", "User logged out", true)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.MarshalWrite(w, map[string]string{"status": "ok"})
	}
}
