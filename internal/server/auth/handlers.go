package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	webauthnlib "github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
	"golang.org/x/text/language"

	"hitkeep/internal/api"
	"hitkeep/internal/appurl"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/mailer"
	"hitkeep/internal/security"
	"hitkeep/internal/server/shared"
)

type handler struct {
	ctx *shared.Context
}

func fallbackMailLocale(acceptLanguage string) string {
	tags, _, err := language.ParseAcceptLanguage(strings.TrimSpace(acceptLanguage))
	if err != nil {
		return "en"
	}

	for _, tag := range tags {
		base, _ := tag.Base()
		if base.String() == language.Und.String() || base.String() == "" {
			continue
		}
		return mailer.NormalizeLocale(base.String())
	}

	return "en"
}

func (h *handler) preferredMailLocale(r *http.Request, userID uuid.UUID) string {
	locale := fallbackMailLocale(r.Header.Get("Accept-Language"))
	prefs, err := h.ctx.Store.GetUserPreferences(r.Context(), userID)
	if err != nil {
		slog.Warn("Failed to load user preferences for auth mail", "error", err, "user_id", userID)
		return locale
	}
	if prefs != nil && strings.TrimSpace(prefs.DefaultLocale) != "" {
		return mailer.NormalizeLocale(prefs.DefaultLocale)
	}
	return locale
}

func sanitizeAuthReturnPath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "/dashboard"
	}
	if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "/dashboard"
	}
	if strings.HasPrefix(value, "/login") || strings.HasPrefix(value, "/setup") {
		return "/dashboard"
	}
	return value
}

func (h *handler) publicRedirectURL(relativePath string) string {
	return appurl.Path(h.ctx.Config.PublicURL, sanitizeAuthReturnPath(relativePath))
}

func (h *handler) loginErrorRedirectURL(code string) string {
	return appurl.Path(h.ctx.Config.PublicURL, "/login?error="+code)
}

func (h *handler) appendAuthAuditForUserTeams(r *http.Request, userID uuid.UUID, action, outcome, details string, actorVerified bool) {
	if h.ctx.Store == nil || userID == uuid.Nil {
		return
	}
	targetLabel := userID.String()
	if user, err := h.ctx.Store.GetUserByID(r.Context(), userID); err == nil && user != nil {
		targetLabel = user.Email
	}
	actorID := uuid.Nil
	if actorVerified {
		actorID = userID
	}
	h.ctx.AppendAuditEventForUserTeams(r.Context(), r, userID, shared.AuditEvent{
		ActorID:      actorID,
		TargetUserID: userID,
		Action:       action,
		TargetType:   "user",
		TargetID:     userID.String(),
		TargetLabel:  targetLabel,
		Outcome:      outcome,
		Details:      details,
	})
}

func (h *handler) appendAuthAuditSystem(r *http.Request, action, outcome, targetLabel, details string) {
	h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
		Action:      action,
		TargetType:  "user",
		TargetLabel: strings.ToLower(strings.TrimSpace(targetLabel)),
		Outcome:     outcome,
		Details:     details,
	})
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("POST /api/initial-user", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handleCreateInitialUser()))
	mux.HandleFunc("POST /api/login", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handleLogin()))
	mux.HandleFunc("POST /api/logout", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handleLogout()))
	mux.HandleFunc("GET /api/auth/session", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetSession()))
	mux.HandleFunc("POST /api/auth/session/extend", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.AuthLimiter,
	}, h.handleExtendSession()))
	mux.HandleFunc("POST /api/auth/forgot-password", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handleForgotPassword()))
	mux.HandleFunc("POST /api/auth/reset-password", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handleResetPassword()))
	mux.HandleFunc("POST /api/auth/accept-invite", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handleAcceptInvite()))
	mux.HandleFunc("POST /api/auth/passkey/login/start", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handlePasskeyLoginStart()))
	mux.HandleFunc("POST /api/auth/passkey/login/finish", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handlePasskeyLoginFinish()))
	mux.HandleFunc("POST /api/auth/mfa/totp/verify", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handleMFATOTPVerify()))
	mux.HandleFunc("POST /api/auth/mfa/email-link/request", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handleMFAEmailLinkRequest()))
	mux.HandleFunc("GET /api/auth/mfa/email-link/verify", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handleMFAEmailLinkVerify()))
	mux.HandleFunc("POST /api/auth/mfa/recovery-code/verify", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handleMFARecoveryCodeVerify()))
	mux.HandleFunc("POST /api/user/password", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.AuthLimiter,
	}, h.handleChangePassword()))
}

func (h *handler) handleCreateInitialUser() http.HandlerFunc {
	type request struct {
		Email string `json:"email"`
		//nolint:gosec // request payload intentionally accepts plaintext password input.
		Password  string `json:"password"`
		GivenName string `json:"given_name"`
		LastName  string `json:"last_name"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		if h.ctx.Config.CloudHosted {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		userCount, err := h.ctx.Store.GetUserCount(r.Context())
		if err != nil {
			slog.Error("Failed to check user count during setup", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if userCount > 0 {
			http.Error(w, "Setup has already been completed.", http.StatusForbidden)
			return
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Email == "" || len(req.Password) < 8 {
			http.Error(w, "Email required; Password must be at least 8 characters", http.StatusBadRequest)
			return
		}

		hashedPassword, err := HashPassword(req.Password)
		if err != nil {
			slog.Error("Failed to hash password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		locale := fallbackMailLocale(r.Header.Get("Accept-Language"))
		ctx := context.WithValue(r.Context(), database.LocaleContextKey, locale)
		userID, err := h.ctx.Store.CreateUserWithNames(ctx, req.Email, hashedPassword, req.GivenName, req.LastName)
		if err != nil {
			slog.Error("Failed to create initial user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		duration := h.ctx.Config.AuthSessionDuration()
		token, _, err := authcore.GenerateTokenWithDuration(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, userID, duration)
		if err != nil {
			slog.Error("Failed to generate auth token", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		isSecure := strings.HasPrefix(h.ctx.Config.PublicURL, "https://")
		authcore.SetTokenCookieWithDuration(w, token, isSecure, duration)

		//nolint:gosec // email/user_id are expected audit fields after successful account setup.
		slog.Info("Initial admin user created", "email", req.Email, "user_id", userID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]string{
			"token": token,
		}); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleLogin() http.HandlerFunc {
	type request struct {
		Email string `json:"email"`
		//nolint:gosec // request payload intentionally accepts plaintext password input.
		Password   string `json:"password"`
		RememberMe bool   `json:"remember_me"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		user, err := h.ctx.Store.GetUserByEmail(r.Context(), req.Email)
		if err != nil {
			slog.Error("Database error during login", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			burnLoginKDF(req.Password)
			h.appendAuthAuditSystem(r, "auth.login_failed", "failure", req.Email, "Login failed because no matching user was resolved")
			http.Error(w, "Invalid email or password", http.StatusUnauthorized)
			return
		}

		match, err := verifyPassword(req.Password, user.Password)
		if err != nil {
			slog.Error("Password verification error", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !match {
			h.appendAuthAuditForUserTeams(r, user.ID, "auth.login_failed", "failure", "Login failed because the password was invalid", false)
			http.Error(w, "Invalid email or password", http.StatusUnauthorized)
			return
		}

		totpEnabled, err := h.ctx.Store.HasEnabledTOTP(r.Context(), user.ID)
		if err != nil {
			//nolint:gosec // user_id comes from authenticated lookup and is logged for auditability.
			slog.Error("Failed to check user totp status during login", "error", err, "user_id", user.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		passkeys, err := h.ctx.Store.ListUserPasskeys(r.Context(), user.ID)
		if err != nil {
			//nolint:gosec // user_id comes from authenticated lookup and is logged for auditability.
			slog.Error("Failed to list user passkeys during login", "error", err, "user_id", user.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		hasPasskey := len(passkeys) > 0
		recoveryCodesRemaining, err := h.ctx.Store.CountActiveRecoveryCodes(r.Context(), user.ID)
		if err != nil {
			slog.Error("Failed to count user recovery codes during login", "error", err, "user_id", user.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		hasRecoveryCode := recoveryCodesRemaining > 0

		if totpEnabled || hasPasskey {
			userID := user.ID
			var (
				challenge      string
				session        *webauthnlib.SessionData
				passkeyOptions *protocol.PublicKeyCredentialRequestOptions
			)

			if hasPasskey {
				passkeyUser, err := h.loadPasskeyUser(r.Context(), user.ID)
				if err != nil {
					slog.Error("Failed to load passkey user during login", "error", err, "user_id", user.ID)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}

				webAuthn, err := security.NewWebAuthn(h.ctx.Config.PublicURL, r)
				if err != nil {
					slog.Error("Failed to configure MFA passkey login", "error", err, "user_id", user.ID)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}

				assertion, beginSession, err := webAuthn.BeginLogin(passkeyUser)
				if err != nil {
					slog.Error("Failed to begin MFA passkey login", "error", err, "user_id", user.ID)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}

				challenge = beginSession.Challenge
				session = beginSession
				passkeyOptions = &assertion.Response
			} else {
				var err error
				challenge, err = security.GenerateRandomChallenge(32)
				if err != nil {
					slog.Error("Failed to generate mfa challenge for login", "error", err, "user_id", user.ID)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
			}

			expiresAt := time.Now().UTC().Add(passkeyLoginChallengeTTL)
			if session != nil && !session.Expires.IsZero() {
				expiresAt = session.Expires
			}

			if h.ctx.AuthState == nil {
				http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
				return
			}
			challengeID := h.ctx.AuthState.CreatePasskeyLoginChallenge(challenge, database.CreateLoginChallengeInput{
				UserID:     &userID,
				RememberMe: req.RememberMe,
				Flow:       "mfa",
			}, expiresAt, session)

			factors := make([]string, 0, 2)
			if totpEnabled {
				factors = append(factors, "totp")
			}
			if hasRecoveryCode {
				factors = append(factors, "recovery_code")
			}
			if h.ctx.Mailer != nil {
				factors = append(factors, "email_link")
			}
			resp := loginResponse{
				Status:         "mfa_required",
				ChallengeToken: challengeID.String(),
				Factors:        factors,
			}

			if hasPasskey {
				resp.Factors = append(resp.Factors, "passkey")
				resp.Passkey = passkeyOptions
			}

			h.appendAuthAuditForUserTeams(r, user.ID, "auth.mfa_required", "success", "Login requires multi-factor authentication", true)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				slog.Error("Failed to encode mfa-required login response", "error", err, "user_id", user.ID)
			}
			return
		}

		if err := h.issueLoginSession(r.Context(), w, user.ID, req.RememberMe); err != nil {
			slog.Error("Failed to issue login session", "error", err, "user_id", user.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.appendAuthAuditForUserTeams(r, user.ID, "auth.login_succeeded", "success", "Login succeeded", true)
		slog.Info("User logged in", "user_id", user.ID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(loginResponse{Status: "ok"}); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

type loginResponse struct {
	Status         string                                      `json:"status"`
	ChallengeToken string                                      `json:"challenge_token,omitempty"`
	Factors        []string                                    `json:"factors,omitempty"`
	Passkey        *protocol.PublicKeyCredentialRequestOptions `json:"passkey,omitempty"`
}

func (h *handler) issueLoginSession(ctx context.Context, w http.ResponseWriter, userID uuid.UUID, rememberMe bool) error {
	duration := h.ctx.Config.AuthSessionDuration()
	token, _, err := authcore.GenerateTokenWithDuration(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, userID, duration)
	if err != nil {
		return fmt.Errorf("could not generate auth token: %w", err)
	}

	isSecure := strings.HasPrefix(h.ctx.Config.PublicURL, "https://")
	authcore.SetTokenCookieWithDuration(w, token, isSecure, duration)

	if rememberMe {
		rememberDuration := h.ctx.Config.AuthRememberMeDuration()
		rememberToken, err := h.ctx.Store.CreateRememberMeTokenWithDuration(ctx, userID, rememberDuration)
		if err != nil {
			slog.Error("Failed to create remember me token", "error", err, "user_id", userID)
			return nil
		}
		authcore.SetRememberMeCookieWithDuration(w, rememberToken, isSecure, rememberDuration)
	}
	return nil
}

func (h *handler) handleGetSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := r.Context().Value(shared.AuthSessionKey).(shared.AuthSessionContext)
		if !ok || session.ExpiresAt.IsZero() {
			http.Error(w, "Session unavailable", http.StatusUnauthorized)
			return
		}

		writeSessionResponse(w, h.ctx.AuthSessionResponseForRequest(r, shared.GetUserIDFromContext(r), session))
	}
}

func (h *handler) handleExtendSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		duration := h.ctx.Config.AuthSessionDuration()
		token, expiresAt, err := authcore.GenerateTokenWithDuration(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, userID, duration)
		if err != nil {
			slog.Error("Failed to extend auth session", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		isSecure := strings.HasPrefix(h.ctx.Config.PublicURL, "https://")
		authcore.SetTokenCookieWithDuration(w, token, isSecure, duration)
		resp := h.ctx.AuthSessionResponse(shared.AuthSessionContext{
			ExpiresAt: expiresAt.UTC(),
			IssuedAt:  time.Now().UTC(),
		})
		if rememberExpiresAt := h.renewRememberedSession(r, w, userID, isSecure); rememberExpiresAt != nil {
			resp.Remembered = true
			resp.RememberExpiresAt = rememberExpiresAt
		}
		h.appendAuthAuditForUserTeams(r, userID, "auth.session_extended", "success", "Session extended", true)
		writeSessionResponse(w, resp)
	}
}

func (h *handler) renewRememberedSession(r *http.Request, w http.ResponseWriter, userID uuid.UUID, isSecure bool) *time.Time {
	if h.ctx.Store == nil || userID == uuid.Nil {
		return nil
	}
	cookie, err := r.Cookie(authcore.RememberMeCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return nil
	}
	rememberedUserID, currentExpiresAt, err := h.ctx.Store.ValidateRememberMeSession(r.Context(), cookie.Value)
	if err != nil || rememberedUserID != userID || currentExpiresAt.IsZero() {
		return nil
	}
	if err := h.ctx.Store.DeleteRememberMeToken(r.Context(), cookie.Value); err != nil {
		slog.Error("Failed to rotate remember me token during session extension", "error", err, "user_id", userID)
		return &currentExpiresAt
	}
	rememberDuration := h.ctx.Config.AuthRememberMeDuration()
	rememberToken, rememberExpiresAt, err := h.ctx.Store.CreateRememberMeSessionWithDuration(r.Context(), userID, rememberDuration)
	if err != nil {
		slog.Error("Failed to renew remember me token during session extension", "error", err, "user_id", userID)
		return &currentExpiresAt
	}
	authcore.SetRememberMeCookieWithDuration(w, rememberToken, isSecure, rememberDuration)
	return &rememberExpiresAt
}

func writeSessionResponse(w http.ResponseWriter, resp api.AuthSession) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("Failed to encode auth session response", "error", err)
	}
}

func HashPassword(password string) (string, error) {
	const (
		time    = 1
		memory  = 64 * 1024
		threads = 4
		keyLen  = 32
		saltLen = 16
	)
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, time, memory, threads, keyLen)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, memory, time, threads, b64Salt, b64Hash), nil
}

func verifyPassword(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, errors.New("invalid hash format")
	}
	if parts[1] != "argon2id" {
		return false, errors.New("incompatible variant")
	}
	var version int
	_, err := fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil {
		return false, err
	}
	if version != argon2.Version {
		return false, errors.New("incompatible version")
	}
	var memory uint32
	var time uint32
	var threads uint8
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads)
	if err != nil {
		return false, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	decodedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	if uint64(len(decodedHash)) > math.MaxUint32 {
		return false, errors.New("decoded hash too long")
	}
	//nolint:gosec // decodedHash length is bounded before conversion.
	keyLen := uint32(len(decodedHash))
	comparisonHash := argon2.IDKey([]byte(password), salt, time, memory, threads, keyLen)
	if subtle.ConstantTimeCompare(decodedHash, comparisonHash) == 1 {
		return true, nil
	}
	return false, nil
}

func burnLoginKDF(secret string) {
	const (
		timeCost   uint32 = 1
		memoryCost uint32 = 64 * 1024
		threads    uint8  = 4
		keyLen     uint32 = 32
	)

	// Deterministic salt keeps the operation stable and avoids external dependencies.
	salt := []byte("0123456789abcdef")
	hash := argon2.IDKey([]byte(secret), salt, timeCost, memoryCost, threads, keyLen)
	if subtle.ConstantTimeCompare(hash, hash) != 1 {
		slog.Warn("Unreachable login kdf comparison failure")
	}
}
