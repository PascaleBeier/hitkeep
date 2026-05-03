package user

import (
	"context"
	"crypto/md5" //nolint:gosec // Gravatar requires MD5 hashes for avatar lookups.
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

type handler struct {
	ctx *shared.Context
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("GET /api/user/bootstrap", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetUserBootstrap()))
	mux.HandleFunc("GET /api/user/profile", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetUserProfile()))
	mux.HandleFunc("PUT /api/user/profile", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleUpdateUserProfile()))
	mux.HandleFunc("GET /api/user/avatar", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetUserAvatar()))
	mux.HandleFunc("GET /api/user/current-ip", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetCurrentIP()))
	mux.HandleFunc("GET /api/user/preferences", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetUserPreferences()))
	mux.HandleFunc("PUT /api/user/preferences", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleUpdateUserPreferences()))
	mux.HandleFunc("GET /api/user/onboarding", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetUserOnboarding()))
	mux.HandleFunc("POST /api/user/onboarding/dismiss", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleDismissUserOnboarding()))
	mux.HandleFunc("GET /api/user/security", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetUserSecurityStatus()))
	mux.HandleFunc("POST /api/user/security/totp/setup/start", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.AuthLimiter,
	}, h.handleStartTOTPSetup()))
	mux.HandleFunc("POST /api/user/security/totp/setup/verify", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.AuthLimiter,
	}, h.handleVerifyTOTPSetup()))
	mux.HandleFunc("POST /api/user/security/totp/disable", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.AuthLimiter,
	}, h.handleDisableTOTP()))
	mux.HandleFunc("POST /api/user/security/passkeys/register/start", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.AuthLimiter,
	}, h.handleStartPasskeyRegistration()))
	mux.HandleFunc("POST /api/user/security/passkeys/register/finish", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.AuthLimiter,
	}, h.handleFinishPasskeyRegistration()))
	mux.HandleFunc("DELETE /api/user/security/passkeys/{id}", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.AuthLimiter,
	}, h.handleDeleteUserPasskey()))
	mux.HandleFunc("POST /api/user/security/recovery-codes/regenerate", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.AuthLimiter,
	}, h.handleRegenerateRecoveryCodes()))
	mux.HandleFunc("GET /api/user/api-clients", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleListAPIClients()))
	mux.HandleFunc("POST /api/user/api-clients", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreateAPIClient()))
	mux.HandleFunc("PUT /api/user/api-clients/{id}", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleUpdateAPIClient()))
	mux.HandleFunc("DELETE /api/user/api-clients/{id}", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleDeleteAPIClient()))
	mux.HandleFunc("GET /api/user/teams/{id}/api-clients", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleListTeamAPIClients()))
	mux.HandleFunc("POST /api/user/teams/{id}/api-clients", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreateTeamAPIClient()))
	mux.HandleFunc("PUT /api/user/teams/{id}/api-clients/{clientId}", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleUpdateTeamAPIClient()))
	mux.HandleFunc("DELETE /api/user/teams/{id}/api-clients/{clientId}", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleDeleteTeamAPIClient()))
	mux.HandleFunc("GET /api/user/report-subscriptions", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetReportSubscriptions()))
	mux.HandleFunc("PUT /api/user/report-subscriptions/sites/{site_id}", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleUpdateSiteReportSubscription()))
	mux.HandleFunc("PUT /api/user/report-subscriptions/digest", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleUpdateDigestSubscription()))
	mux.HandleFunc("POST /api/user/teams", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreateTeam()))
	mux.HandleFunc("GET /api/user/teams", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetTeams()))
	mux.HandleFunc("PUT /api/user/teams/active", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleSetActiveTeam()))
	mux.HandleFunc("PATCH /api/user/teams/{id}", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleUpdateTeam()))
	mux.HandleFunc("PUT /api/user/teams/{id}", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleUpdateTeam()))
	mux.HandleFunc("POST /api/user/teams/{id}/transfer-ownership", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleTransferTeamOwnership()))
	mux.HandleFunc("POST /api/user/teams/{id}/archive", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleArchiveTeam()))
	mux.HandleFunc("GET /api/user/teams/{id}/members", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetTeamMembers()))
	mux.HandleFunc("GET /api/user/teams/{id}/invites", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetTeamInvites()))
	mux.HandleFunc("GET /api/user/teams/{id}/audit", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetTeamAudit()))
	mux.HandleFunc("POST /api/user/teams/{id}/members", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleAddTeamMember()))
	mux.HandleFunc("POST /api/user/teams/{id}/invites/{inviteId}/resend", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleResendTeamInvite()))
	mux.HandleFunc("DELETE /api/user/teams/{id}/invites/{inviteId}", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleRevokeTeamInvite()))
	mux.HandleFunc("DELETE /api/user/teams/{id}/members/{userId}", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleRemoveTeamMember()))
	mux.HandleFunc("DELETE /api/user/teams/{id}/leave", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleLeaveTeam()))
}

const gravatarBaseURL = "https://www.gravatar.com"

func (h *handler) handleGetUserProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		resp, err := h.userProfileResponse(r.Context(), userID)
		if err != nil {
			if errors.Is(err, errBootstrapUserNotFound) {
				http.Error(w, "User not found", http.StatusNotFound)
				return
			}
			slog.Error("Failed to load user profile", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to encode user profile", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleUpdateUserProfile() http.HandlerFunc {
	type request struct {
		Email     string `json:"email"`
		GivenName string `json:"given_name"`
		LastName  string `json:"last_name"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req request
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		email := strings.ToLower(strings.TrimSpace(req.Email))
		givenName := strings.TrimSpace(req.GivenName)
		lastName := strings.TrimSpace(req.LastName)

		if email == "" {
			http.Error(w, "Email is required", http.StatusBadRequest)
			return
		}
		if len(email) > 320 {
			http.Error(w, "Email must be 320 characters or fewer", http.StatusBadRequest)
			return
		}
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed.Address != email {
			http.Error(w, "Invalid email address", http.StatusBadRequest)
			return
		}
		if len(givenName) > 120 || len(lastName) > 120 {
			http.Error(w, "Given and last name must be 120 characters or fewer", http.StatusBadRequest)
			return
		}

		if err := h.ctx.Store.UpdateUserProfile(r.Context(), userID, email, givenName, lastName); err != nil {
			switch {
			case errors.Is(err, database.ErrUserEmailAlreadyExists):
				http.Error(w, "Email already exists", http.StatusConflict)
			case errors.Is(err, database.ErrUserNotFound):
				http.Error(w, "User not found", http.StatusNotFound)
			default:
				slog.Error("Failed to update user profile", "error", err, "user_id", userID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		user, err := h.ctx.Store.GetUserByID(r.Context(), userID)
		if err != nil {
			slog.Error("Failed to load updated user profile", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		resp := api.UserProfile{
			ID:          user.ID,
			Email:       user.Email,
			GivenName:   user.GivenName,
			LastName:    user.LastName,
			DisplayName: displayNameForUser(user),
			AvatarURL:   "/api/user/avatar?s=96",
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to encode updated user profile", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleGetUserAvatar() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := h.ctx.Store.GetUserByID(r.Context(), userID)
		if err != nil {
			slog.Error("Failed to load user for avatar", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		size := clampAvatarSize(r.URL.Query().Get("s"))
		req, err := newGravatarRequest(r.Context(), user.Email, size)
		if err != nil {
			http.Error(w, "Failed to request avatar", http.StatusInternalServerError)
			return
		}

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req) //nolint:gosec // Request target is pinned to the Gravatar origin by newGravatarRequest; see avatar_test.go.
		if err != nil {
			slog.Warn("Failed to fetch gravatar", "error", err, "user_id", userID)
			http.Error(w, "Avatar unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= http.StatusBadRequest {
			http.Error(w, "Avatar unavailable", resp.StatusCode)
			return
		}

		if contentType := resp.Header.Get("Content-Type"); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			slog.Warn("Failed to proxy gravatar response", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleGetUserPreferences() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		prefs, err := h.userPreferencesResponse(r, userID)
		if err != nil {
			slog.Error("Failed to load user preferences", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(prefs); err != nil {
			slog.Error("Failed to encode user preferences", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleGetCurrentIP() http.HandlerFunc {
	type response struct {
		IP   string `json:"ip"`
		CIDR string `json:"cidr"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ipValue := strings.TrimSpace(shared.GetRealIP(r, h.ctx.Config.GetTrustedProxyNetworks()))
		if _, err := netip.ParseAddr(ipValue); err != nil {
			ipValue = strings.TrimSpace(shared.RemoteIPFromAddr(ipValue))
		}

		parsedIP, err := netip.ParseAddr(ipValue)
		if err != nil {
			http.Error(w, "Unable to resolve client IP", http.StatusBadRequest)
			return
		}
		parsedIP = parsedIP.Unmap()

		cidrSuffix := "/128"
		if parsedIP.Is4() {
			cidrSuffix = "/32"
		}

		resp := response{
			IP:   parsedIP.String(),
			CIDR: parsedIP.String() + cidrSuffix,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to encode current IP response", "error", err)
		}
	}
}

func (h *handler) handleUpdateUserPreferences() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var payload api.UserPreferences
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		prefs, err := validateUserPreferences(payload.DefaultLocale)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := h.ctx.Store.UpsertUserPreferences(r.Context(), userID, prefs); err != nil {
			slog.Error("Failed to update user preferences", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(prefs); err != nil {
			slog.Error("Failed to encode user preferences", "error", err, "user_id", userID)
		}
	}
}

func newGravatarRequest(ctx context.Context, email string, size int) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gravatarBaseURL, nil)
	if err != nil {
		return nil, err
	}

	hash := gravatarHash(email)
	params := url.Values{
		"s": {strconv.Itoa(size)},
		"d": {"mp"},
	}

	req.URL.Path = "/avatar/" + hash
	req.URL.RawQuery = params.Encode()
	return req, nil
}

func gravatarHash(email string) string {
	normalized := strings.TrimSpace(strings.ToLower(email))
	sum := md5.Sum([]byte(normalized)) //nolint:gosec // Gravatar requires MD5 hashes for avatar lookups.
	return hex.EncodeToString(sum[:])
}

func clampAvatarSize(raw string) int {
	if raw == "" {
		return 96
	}
	size, err := strconv.Atoi(raw)
	if err != nil {
		return 96
	}
	if size < 32 {
		return 32
	}
	if size > 256 {
		return 256
	}
	return size
}

func displayNameFromEmail(email string) string {
	local := strings.SplitN(strings.TrimSpace(email), "@", 2)[0]
	if local == "" {
		return "User"
	}
	parts := strings.FieldsFunc(local, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	name := strings.TrimSpace(strings.Join(parts, " "))
	if name == "" {
		return "User"
	}
	return name
}

func displayNameForUser(user *api.User) string {
	if user == nil {
		return "User"
	}
	parts := make([]string, 0, 2)
	if given := strings.TrimSpace(user.GivenName); given != "" {
		parts = append(parts, given)
	}
	if last := strings.TrimSpace(user.LastName); last != "" {
		parts = append(parts, last)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return displayNameFromEmail(user.Email)
}
