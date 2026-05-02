package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/mailables"
	"hitkeep/internal/server/shared"
)

func (h *handler) handleForgotPassword() http.HandlerFunc {
	type request struct {
		Email string `json:"email"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		user, err := h.ctx.Store.GetUserByEmail(r.Context(), req.Email)
		if err != nil {
			slog.Error("Database error checking user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if user == nil {
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(map[string]string{"message": "If an account exists, a reset link has been sent."}); err != nil {
				slog.Error("Failed to encode response", "error", err)
			}
			return
		}

		token, err := h.ctx.Store.CreatePasswordResetToken(r.Context(), user.Email)
		if err != nil {
			slog.Error("Failed to create reset token", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		resetLink := fmt.Sprintf("%s/reset-password?token=%s", h.ctx.Config.PublicURL, token)
		locale := h.preferredMailLocale(r, user.ID)

		err = h.ctx.Mailer.Send(user.Email, mailables.NewPasswordReset(resetLink, locale))
		if err != nil {
			slog.Error("Failed to send password reset email", "error", err, "email", user.Email)
			http.Error(w, "Failed to send email. Check server logs.", http.StatusBadGateway)
			return
		}

		slog.Info("Password reset requested", "email", user.Email)

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"message": "If an account exists, a reset link has been sent."}); err != nil {
			slog.Error("Failed to encode response", "error", err)
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Token == "" || len(req.Password) < 8 {
			http.Error(w, "Invalid token or password too short", http.StatusBadRequest)
			return
		}

		hashedPassword, err := HashPassword(req.Password)
		if err != nil {
			slog.Error("Failed to hash password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		err = h.ctx.Store.CompletePasswordReset(r.Context(), req.Token, hashedPassword)
		if err != nil {
			if errors.Is(err, database.ErrPasswordResetInvalid) || errors.Is(err, database.ErrPasswordResetExpired) {
				http.Error(w, "Invalid or expired link", http.StatusBadRequest)
				return
			}

			slog.Error("Failed to complete password reset", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		slog.Info("Password reset successful", "token_mask", req.Token[:4]+"...")

		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Password updated successfully"})
		if err != nil {
			slog.Error("Failed to complete password reset", "error", err)
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Token == "" || len(req.Password) < 8 {
			http.Error(w, "Invalid token or password too short", http.StatusBadRequest)
			return
		}

		email, err := h.ctx.Store.ResolvePasswordResetEmail(r.Context(), req.Token)
		if err != nil {
			if errors.Is(err, database.ErrPasswordResetInvalid) || errors.Is(err, database.ErrPasswordResetExpired) {
				http.Error(w, "Invalid or expired link", http.StatusBadRequest)
				return
			}
			slog.Error("Failed to resolve invite token", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		hashedPassword, err := HashPassword(req.Password)
		if err != nil {
			slog.Error("Failed to hash password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		err = h.ctx.Store.CompletePasswordReset(r.Context(), req.Token, hashedPassword)
		if err != nil {
			if errors.Is(err, database.ErrPasswordResetInvalid) || errors.Is(err, database.ErrPasswordResetExpired) {
				http.Error(w, "Invalid or expired link", http.StatusBadRequest)
				return
			}

			slog.Error("Failed to complete invite acceptance", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		user, err := h.ctx.Store.GetUserByEmail(r.Context(), email)
		if err != nil {
			slog.Error("Failed to load invited user after password reset", "error", err, "email", email)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, "Invalid or expired link", http.StatusBadRequest)
			return
		}

		if h.ctx.Config.CloudHosted {
			pendingInvites, err := h.ctx.Store.ListPendingTeamInvitesByEmail(r.Context(), email)
			if err != nil {
				slog.Error("Failed to list pending cloud invites", "error", err, "email", email, "user_id", user.ID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			targetTeams := make(map[uuid.UUID]struct{})
			for _, invite := range pendingInvites {
				isMember, err := h.ctx.Store.IsTenantMember(r.Context(), invite.TeamID, user.ID)
				if err != nil {
					slog.Error("Failed to check cloud invite membership", "error", err, "email", email, "user_id", user.ID, "team_id", invite.TeamID)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
				if !isMember {
					targetTeams[invite.TeamID] = struct{}{}
				}
			}

			teamCount, err := h.ctx.Store.CountUserNonDefaultTeams(r.Context(), user.ID)
			if err != nil {
				slog.Error("Failed to count cloud invite teams", "error", err, "email", email, "user_id", user.ID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if len(targetTeams) > 1 || (teamCount > 0 && len(targetTeams) > 0) {
				http.Error(w, "Managed cloud accounts are limited to one team", http.StatusForbidden)
				return
			}
		}

		acceptedInvites, err := h.ctx.Store.AcceptTeamInvitesByEmail(r.Context(), email, user.ID)
		if err != nil {
			slog.Error("Failed to accept team invites", "error", err, "email", email, "user_id", user.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		for _, invite := range acceptedInvites {
			h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
				ActorID:      user.ID,
				TeamID:       invite.TeamID,
				TargetUserID: user.ID,
				Action:       "member.invite_accepted",
				TargetType:   "user",
				TargetID:     user.ID.String(),
				TargetLabel:  email,
				Outcome:      "success",
				Details:      fmt.Sprintf("Invitation accepted by %s", email),
			})
			h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
				ActorID:      user.ID,
				TeamID:       invite.TeamID,
				TargetUserID: user.ID,
				Action:       "member.added",
				TargetType:   "user",
				TargetID:     user.ID.String(),
				TargetLabel:  email,
				Outcome:      "success",
				Details:      fmt.Sprintf("Member %s added after accepting an invitation", email),
			})
		}

		slog.Info("Invite accepted", "token_mask", req.Token[:4]+"...")

		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Account set up successfully. Please log in."})
		if err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		if len(req.NewPassword) < 8 {
			http.Error(w, "New password must be at least 8 characters", http.StatusBadRequest)
			return
		}

		user, err := h.ctx.Store.GetUserByID(r.Context(), userID)
		if err != nil {
			slog.Error("Failed to fetch user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		match, err := verifyPassword(req.CurrentPassword, user.Password)
		if err != nil {
			slog.Error("Error verifying password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !match {
			http.Error(w, "Current password is incorrect", http.StatusForbidden)
			return
		}

		newHash, err := HashPassword(req.NewPassword)
		if err != nil {
			slog.Error("Failed to hash new password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := h.ctx.Store.UpdatePasswordByID(r.Context(), userID.String(), newHash); err != nil {
			slog.Error("Failed to update password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		slog.Info("User changed password", "user_id", userID)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
				slog.Error("Failed to delete remember me token", "error", err)
			}
		}

		authcore.ClearCookies(w, isSecure)
		if userID != uuid.Nil {
			h.appendAuthAuditForUserTeams(r, userID, "auth.logout", "success", "User logged out", true)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
