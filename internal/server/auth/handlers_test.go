package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/mailer"
	"hitkeep/internal/security"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/testutil"
)

type authTestMailDriver struct {
	subject  string
	htmlBody string
	textBody string
}

func (d *authTestMailDriver) Send(_ []string, subject, htmlBody, textBody string) error {
	d.subject = subject
	d.htmlBody = htmlBody
	d.textBody = textBody
	return nil
}

func (d *authTestMailDriver) Close() error { return nil }

func extractMagicLinkToken(t *testing.T, body string) string {
	t.Helper()

	const prefix = "/api/auth/mfa/email-link/verify?token="
	idx := strings.Index(body, prefix)
	if idx == -1 {
		t.Fatalf("expected magic link in body, got:\n%s", body)
	}

	start := idx + len(prefix)
	token := body[start:]
	for i, r := range token {
		if r == '\n' || r == '\r' || r == ' ' {
			token = token[:i]
			break
		}
	}
	if _, err := uuid.Parse(token); err != nil {
		t.Fatalf("expected valid magic link token, got %q: %v", token, err)
	}
	return token
}

func setupAuthTestEnv(t *testing.T) (*handler, *database.Store) {
	t.Helper()

	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	conf := &config.Config{
		PublicURL: "http://localhost:8080",
		JWTSecret: "test-secret",
	}

	ctx := &shared.Context{
		Store:     store,
		Config:    conf,
		AuthState: shared.NewAuthStateStore(),
	}

	return &handler{ctx: ctx}, store
}

func TestHandleCreateInitialUser(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	body, err := json.Marshal(map[string]string{
		"email":      "admin@example.com",
		"password":   "password123",
		"given_name": "Ada",
		"last_name":  "Lovelace",
	})
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/initial-user", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handle := h.handleCreateInitialUser()
	handle.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["token"] == "" {
		t.Fatalf("expected token in response")
	}

	count, err := store.GetUserCount(context.Background())
	if err != nil {
		t.Fatalf("failed to count users: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user, got %d", count)
	}

	user, err := store.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("failed to fetch created user: %v", err)
	}
	if user == nil {
		t.Fatalf("expected created user to exist")
	}
	if user.GivenName != "Ada" || user.LastName != "Lovelace" {
		t.Fatalf("expected given/last name to be persisted, got %+v", user)
	}

	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("failed to get default tenant: %v", err)
	}
	defaultTenant, err := store.GetTenant(context.Background(), defaultTenantID)
	if err != nil {
		t.Fatalf("failed to fetch default tenant: %v", err)
	}
	if defaultTenant == nil {
		t.Fatalf("expected default tenant to exist")
	}
	if defaultTenant.Name != "Ada's Team" {
		t.Fatalf("expected default tenant name %q, got %q", "Ada's Team", defaultTenant.Name)
	}

	// Second call should be blocked once setup is complete.
	req2 := httptest.NewRequest(http.MethodPost, "/api/initial-user", bytes.NewReader(body))
	w2 := httptest.NewRecorder()
	handle.ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, w2.Code)
	}
}

func TestHandleCreateInitialUserRejectsManagedCloudBootstrap(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.Config.CloudHosted = true

	body, err := json.Marshal(map[string]string{
		"email":    "admin@example.com",
		"password": "password123",
	})
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/initial-user", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleCreateInitialUser().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleLogin(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "user@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if userID == uuid.Nil {
		t.Fatalf("expected valid user ID")
	}

	t.Run("invalid credentials", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"email":    "user@example.com",
			"password": "wrongpass",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
		w := httptest.NewRecorder()
		h.handleLogin().ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"email":    "user@example.com",
			"password": "password123",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
		w := httptest.NewRecorder()
		h.handleLogin().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		cookies := w.Header().Values("Set-Cookie")
		found := false
		for _, cookie := range cookies {
			if bytes.Contains([]byte(cookie), []byte(auth.CookieName+"=")) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected auth cookie to be set")
		}
	})
}

func TestHandleChangePassword(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "user@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	t.Run("wrong current password", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"current_password": "wrongpass",
			"new_password":     "newpassword123",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/user/password", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
		w := httptest.NewRecorder()
		h.handleChangePassword().ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected status %d, got %d", http.StatusForbidden, w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"current_password": "password123",
			"new_password":     "newpassword123",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/user/password", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
		w := httptest.NewRecorder()
		h.handleChangePassword().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		user, err := store.GetUserByID(context.Background(), userID)
		if err != nil {
			t.Fatalf("failed to load user: %v", err)
		}
		if user == nil {
			t.Fatalf("expected user")
		}
		match, err := verifyPassword("newpassword123", user.Password)
		if err != nil {
			t.Fatalf("failed to verify password: %v", err)
		}
		if !match {
			t.Fatalf("expected password to be updated")
		}
	})
}

func TestHandleLogout(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "user@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	token, err := store.CreateRememberMeToken(context.Background(), userID)
	if err != nil {
		t.Fatalf("failed to create remember me token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	req.AddCookie(&http.Cookie{Name: auth.RememberMeCookieName, Value: token})
	w := httptest.NewRecorder()
	h.handleLogout().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Token should be deleted after logout.
	validatedUser, err := store.ValidateRememberMeToken(context.Background(), token)
	if err != nil {
		t.Fatalf("failed to validate remember me token: %v", err)
	}
	if validatedUser != uuid.Nil {
		t.Fatalf("expected remember me token to be deleted")
	}
}

func TestHandleGetSession(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.Config.AuthSessionMinutes = 30
	h.ctx.Config.AuthSessionWarningSeconds = 90

	expiresAt := time.Now().UTC().Add(30 * time.Minute)
	issuedAt := time.Now().UTC()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	req = req.WithContext(context.WithValue(req.Context(), shared.AuthSessionKey, shared.AuthSessionContext{
		ExpiresAt: expiresAt,
		IssuedAt:  issuedAt,
	}))
	w := httptest.NewRecorder()

	h.handleGetSession().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp api.AuthSession
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.DurationSeconds != 1800 {
		t.Fatalf("expected 1800 duration seconds, got %d", resp.DurationSeconds)
	}
	if resp.WarningSeconds != 90 {
		t.Fatalf("expected 90 warning seconds, got %d", resp.WarningSeconds)
	}
	if !resp.Extendable || !resp.TimingAdjustable {
		t.Fatalf("expected extendable timing-adjustable session, got %+v", resp)
	}
}

func TestHandleGetSessionReflectsRememberMe(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.Config.AuthRememberMeDays = 14

	userID, err := store.CreateUser(context.Background(), "remembered-session@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	rememberToken, _, err := store.CreateRememberMeSessionWithDuration(context.Background(), userID, h.ctx.Config.AuthRememberMeDuration())
	if err != nil {
		t.Fatalf("create remember me token: %v", err)
	}

	expiresAt := time.Now().UTC().Add(30 * time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	req.AddCookie(&http.Cookie{Name: auth.RememberMeCookieName, Value: rememberToken})
	ctx := context.WithValue(req.Context(), shared.UserIDKey, userID)
	ctx = context.WithValue(ctx, shared.AuthSessionKey, shared.AuthSessionContext{
		ExpiresAt: expiresAt,
		IssuedAt:  time.Now().UTC(),
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.handleGetSession().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp api.AuthSession
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Remembered {
		t.Fatalf("expected remembered session, got %+v", resp)
	}
	if resp.RememberMeDurationDays != 14 {
		t.Fatalf("expected configured remember me duration in response, got %d", resp.RememberMeDurationDays)
	}
	if resp.RememberExpiresAt == nil || time.Until(*resp.RememberExpiresAt) < 13*24*time.Hour {
		t.Fatalf("expected remember me expiry around 14 days, got %+v", resp.RememberExpiresAt)
	}
}

func TestHandleExtendSessionIssuesConfiguredCookie(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.Config.AuthSessionMinutes = 30
	h.ctx.Config.AuthSessionWarningSeconds = 90

	userID, err := store.CreateUser(context.Background(), "session@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/session/extend", nil)
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
	w := httptest.NewRecorder()

	h.handleExtendSession().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp api.AuthSession
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.DurationSeconds != 1800 || resp.WarningSeconds != 90 {
		t.Fatalf("expected configured policy in response, got %+v", resp)
	}
	if time.Until(resp.ExpiresAt) < 29*time.Minute {
		t.Fatalf("expected renewed expiry around 30 minutes, got %s", resp.ExpiresAt)
	}

	cookies := w.Header().Values("Set-Cookie")
	if !slices.ContainsFunc(cookies, func(cookie string) bool {
		return strings.Contains(cookie, auth.CookieName+"=")
	}) {
		t.Fatalf("expected auth cookie to be set, got %v", cookies)
	}
}

func TestHandleExtendSessionRenewsRememberMeCookie(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.Config.AuthRememberMeDays = 14

	userID, err := store.CreateUser(context.Background(), "extend-remember@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	rememberToken, err := store.CreateRememberMeToken(context.Background(), userID)
	if err != nil {
		t.Fatalf("create remember me token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/session/extend", nil)
	req.AddCookie(&http.Cookie{Name: auth.RememberMeCookieName, Value: rememberToken})
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
	w := httptest.NewRecorder()

	h.handleExtendSession().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp api.AuthSession
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Remembered || resp.RememberExpiresAt == nil {
		t.Fatalf("expected renewed remember me session, got %+v", resp)
	}
	if resp.RememberMeDurationDays != 14 {
		t.Fatalf("expected configured remember me duration in response, got %d", resp.RememberMeDurationDays)
	}
	if time.Until(*resp.RememberExpiresAt) < 13*24*time.Hour {
		t.Fatalf("expected remember me expiry around 14 days, got %s", resp.RememberExpiresAt)
	}

	cookies := w.Header().Values("Set-Cookie")
	if !slices.ContainsFunc(cookies, func(cookie string) bool {
		return strings.Contains(cookie, auth.RememberMeCookieName+"=")
	}) {
		t.Fatalf("expected remember me cookie to be set, got %v", cookies)
	}

	resolvedUserID, err := store.ValidateRememberMeToken(context.Background(), rememberToken)
	if err != nil {
		t.Fatalf("validate old remember token: %v", err)
	}
	if resolvedUserID != uuid.Nil {
		t.Fatalf("expected old remember me token to be rotated, got %s", resolvedUserID)
	}
}

func TestHandleForgotPasswordFallsBackToAcceptLanguageLocale(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	if _, err := store.CreateUser(context.Background(), "reset@example.com", hashed); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	drv := &authTestMailDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(drv, h.ctx.Config)

	body, err := json.Marshal(map[string]string{"email": "reset@example.com"})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/forgot-password", bytes.NewReader(body))
	req.Header.Set("Accept-Language", "de-DE,de;q=0.9,en;q=0.8")
	w := httptest.NewRecorder()

	h.handleForgotPassword().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	if !strings.Contains(drv.subject, "Setze dein HitKeep-Passwort zurück") {
		t.Fatalf("expected localized German subject, got %q", drv.subject)
	}
	if !strings.Contains(drv.textBody, "Passwort zurücksetzen") {
		t.Fatalf("expected localized German email body, got:\n%s", drv.textBody)
	}
}

func TestHandleLoginIncludesEmailLinkFactorWhenMailerConfigured(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "mfa-email-link@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if err := store.EnableUserTOTP(context.Background(), userID, "JBSWY3DPEHPK3PXP"); err != nil {
		t.Fatalf("failed to enable user totp: %v", err)
	}

	h.ctx.Mailer = mailer.NewWithDriver(&authTestMailDriver{}, h.ctx.Config)

	body, _ := json.Marshal(map[string]any{
		"email":    "mfa-email-link@example.com",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleLogin().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp loginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if !containsFactor(resp.Factors, "email_link") {
		t.Fatalf("expected email_link factor, got %v", resp.Factors)
	}
}

func TestHandleMFAEmailLinkRequestAndVerify(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "email-link-verify@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if err := store.EnableUserTOTP(context.Background(), userID, "JBSWY3DPEHPK3PXP"); err != nil {
		t.Fatalf("failed to enable user totp: %v", err)
	}
	if err := store.UpsertUserPreferences(context.Background(), userID, api.UserPreferences{DefaultLocale: "de"}); err != nil {
		t.Fatalf("set user locale: %v", err)
	}

	drv := &authTestMailDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(drv, h.ctx.Config)

	loginBody, _ := json.Marshal(map[string]any{
		"email":       "email-link-verify@example.com",
		"password":    "password123",
		"remember_me": true,
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(loginBody))
	loginW := httptest.NewRecorder()
	h.handleLogin().ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, loginW.Code)
	}

	var loginResp loginResponse
	if err := json.NewDecoder(loginW.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if !containsFactor(loginResp.Factors, "email_link") || loginResp.ChallengeToken == "" {
		t.Fatalf("expected email_link factor and challenge token, got %+v", loginResp)
	}

	requestBody, _ := json.Marshal(map[string]string{
		"challenge_token": loginResp.ChallengeToken,
		"return_url":      "/events?range=7d",
	})
	requestReq := httptest.NewRequest(http.MethodPost, "/api/auth/mfa/email-link/request", bytes.NewReader(requestBody))
	requestReq.Header.Set("Accept-Language", "de-DE,de;q=0.9")
	requestW := httptest.NewRecorder()
	h.handleMFAEmailLinkRequest().ServeHTTP(requestW, requestReq)
	if requestW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, requestW.Code, requestW.Body.String())
	}
	if !strings.Contains(drv.subject, "Schließe deine HitKeep-Anmeldung ab") {
		t.Fatalf("expected localized German subject, got %q", drv.subject)
	}
	if !strings.Contains(drv.textBody, "Anmeldung abschließen") {
		t.Fatalf("expected localized German body, got:\n%s", drv.textBody)
	}

	token := extractMagicLinkToken(t, drv.textBody)
	verifyReq := httptest.NewRequest(http.MethodGet, "/api/auth/mfa/email-link/verify?token="+token, nil)
	verifyW := httptest.NewRecorder()
	h.handleMFAEmailLinkVerify().ServeHTTP(verifyW, verifyReq)
	if verifyW.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, verifyW.Code)
	}
	location, err := verifyW.Result().Location()
	if err != nil {
		t.Fatalf("expected redirect location: %v", err)
	}
	if got := location.String(); got != "http://localhost:8080/events?range=7d" {
		t.Fatalf("expected redirect to return url, got %q", got)
	}

	cookies := verifyW.Header().Values("Set-Cookie")
	foundAuth := false
	foundRemember := false
	for _, cookie := range cookies {
		if bytes.Contains([]byte(cookie), []byte(auth.CookieName+"=")) {
			foundAuth = true
		}
		if bytes.Contains([]byte(cookie), []byte(auth.RememberMeCookieName+"=")) {
			foundRemember = true
		}
	}
	if !foundAuth {
		t.Fatalf("expected auth cookie after email-link verification")
	}
	if !foundRemember {
		t.Fatalf("expected remember me cookie after email-link verification")
	}

	challengeID, err := uuid.Parse(loginResp.ChallengeToken)
	if err != nil {
		t.Fatalf("parse challenge token: %v", err)
	}
	if _, found := h.ctx.AuthState.GetPasskeyLoginChallenge(challengeID); found {
		t.Fatal("expected mfa challenge to be deleted after email-link verification")
	}
}

func TestHandleMFAEmailLinkVerifyRedirectsInvalidLink(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/mfa/email-link/verify?token="+uuid.NewString(), nil)
	w := httptest.NewRecorder()
	h.handleMFAEmailLinkVerify().ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, w.Code)
	}
	location, err := w.Result().Location()
	if err != nil {
		t.Fatalf("expected redirect location: %v", err)
	}
	if got, want := location.String(), "http://localhost:8080/login?error=mfa_link_invalid"; got != want {
		t.Fatalf("expected redirect %q, got %q", want, got)
	}
}

func TestHandlePasskeyLogin(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "user@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	fixture, err := testutil.NewPasskeyFixture()
	if err != nil {
		t.Fatalf("failed to create passkey fixture: %v", err)
	}

	_, err = store.CreateUserPasskeyCredential(context.Background(), userID, "Test Passkey", fixture.Credential())
	if err != nil {
		t.Fatalf("failed to create user passkey: %v", err)
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/auth/passkey/login/start", bytes.NewReader([]byte("{}")))
	startW := httptest.NewRecorder()
	h.handlePasskeyLoginStart().ServeHTTP(startW, startReq)
	if startW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, startW.Code)
	}

	var startResp passkeyLoginStartResponse
	if err := json.NewDecoder(startW.Body).Decode(&startResp); err != nil {
		t.Fatalf("failed to decode passkey start response: %v", err)
	}
	if startResp.ChallengeToken == "" || len(startResp.PublicKey.Challenge) == 0 {
		t.Fatalf("expected challenge token and challenge")
	}
	if startResp.PublicKey.UserVerification != "required" {
		t.Fatalf("expected required user verification, got %q", startResp.PublicKey.UserVerification)
	}

	credential, err := fixture.AssertionResponse(startResp.PublicKey.Challenge, "http://localhost:8080", "localhost", userID[:], 1, true)
	if err != nil {
		t.Fatalf("failed to create passkey assertion: %v", err)
	}

	finishBody, _ := json.Marshal(map[string]any{
		"challenge_token": startResp.ChallengeToken,
		"credential":      credential,
		"remember_me":     true,
	})
	finishReq := httptest.NewRequest(http.MethodPost, "/api/auth/passkey/login/finish", bytes.NewReader(finishBody))
	finishW := httptest.NewRecorder()
	h.handlePasskeyLoginFinish().ServeHTTP(finishW, finishReq)
	if finishW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, finishW.Code)
	}

	cookies := finishW.Header().Values("Set-Cookie")
	foundAuth := false
	foundRemember := false
	for _, cookie := range cookies {
		if bytes.Contains([]byte(cookie), []byte(auth.CookieName+"=")) {
			foundAuth = true
		}
		if bytes.Contains([]byte(cookie), []byte(auth.RememberMeCookieName+"=")) {
			foundRemember = true
		}
	}
	if !foundAuth {
		t.Fatalf("expected auth cookie to be set on passkey login")
	}
	if !foundRemember {
		t.Fatalf("expected remember me cookie to be set on passkey login")
	}
}

func TestHandlePasskeyLoginWithLegacyStoredCredential(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "legacy-passkey@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	fixture, err := testutil.NewPasskeyFixture()
	if err != nil {
		t.Fatalf("failed to create passkey fixture: %v", err)
	}

	legacyPublicKey, err := fixture.LegacyPublicKey()
	if err != nil {
		t.Fatalf("failed to encode legacy public key: %v", err)
	}

	if _, err := store.CreateUserPasskey(context.Background(), userID, "Legacy Passkey", fixture.CredentialID(), legacyPublicKey, nil); err != nil {
		t.Fatalf("failed to create legacy user passkey: %v", err)
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/auth/passkey/login/start", bytes.NewReader([]byte("{}")))
	startW := httptest.NewRecorder()
	h.handlePasskeyLoginStart().ServeHTTP(startW, startReq)
	if startW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, startW.Code)
	}

	var startResp passkeyLoginStartResponse
	if err := json.NewDecoder(startW.Body).Decode(&startResp); err != nil {
		t.Fatalf("failed to decode passkey start response: %v", err)
	}

	credential, err := fixture.AssertionResponse(startResp.PublicKey.Challenge, "http://localhost:8080", "localhost", userID[:], 1, true)
	if err != nil {
		t.Fatalf("failed to create passkey assertion: %v", err)
	}

	finishBody, _ := json.Marshal(map[string]any{
		"challenge_token": startResp.ChallengeToken,
		"credential":      credential,
		"remember_me":     false,
	})
	finishReq := httptest.NewRequest(http.MethodPost, "/api/auth/passkey/login/finish", bytes.NewReader(finishBody))
	finishW := httptest.NewRecorder()
	h.handlePasskeyLoginFinish().ServeHTTP(finishW, finishReq)
	if finishW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, finishW.Code, finishW.Body.String())
	}
}

func TestHandlePasskeyLoginWithLegacyStoredCredentialAndBackupFlags(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "legacy-backup-passkey@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	fixture, err := testutil.NewPasskeyFixture()
	if err != nil {
		t.Fatalf("failed to create passkey fixture: %v", err)
	}

	legacyPublicKey, err := fixture.LegacyPublicKey()
	if err != nil {
		t.Fatalf("failed to encode legacy public key: %v", err)
	}

	if _, err := store.CreateUserPasskey(context.Background(), userID, "Legacy Backup Passkey", fixture.CredentialID(), legacyPublicKey, nil); err != nil {
		t.Fatalf("failed to create legacy user passkey: %v", err)
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/auth/passkey/login/start", bytes.NewReader([]byte("{}")))
	startW := httptest.NewRecorder()
	h.handlePasskeyLoginStart().ServeHTTP(startW, startReq)
	if startW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, startW.Code)
	}

	var startResp passkeyLoginStartResponse
	if err := json.NewDecoder(startW.Body).Decode(&startResp); err != nil {
		t.Fatalf("failed to decode passkey start response: %v", err)
	}

	credential, err := fixture.AssertionResponseWithFlags(
		startResp.PublicKey.Challenge,
		"http://localhost:8080",
		"localhost",
		userID[:],
		1,
		true,
		true,
		true,
	)
	if err != nil {
		t.Fatalf("failed to create passkey assertion: %v", err)
	}

	finishBody, _ := json.Marshal(map[string]any{
		"challenge_token": startResp.ChallengeToken,
		"credential":      credential,
		"remember_me":     false,
	})
	finishReq := httptest.NewRequest(http.MethodPost, "/api/auth/passkey/login/finish", bytes.NewReader(finishBody))
	finishW := httptest.NewRecorder()
	h.handlePasskeyLoginFinish().ServeHTTP(finishW, finishReq)
	if finishW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, finishW.Code, finishW.Body.String())
	}
}

func TestHandlePasskeyLoginRejectsMissingUserVerification(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "user-uv@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	fixture, err := testutil.NewPasskeyFixture()
	if err != nil {
		t.Fatalf("failed to create passkey fixture: %v", err)
	}

	_, err = store.CreateUserPasskeyCredential(context.Background(), userID, "Test Passkey", fixture.Credential())
	if err != nil {
		t.Fatalf("failed to create user passkey: %v", err)
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/auth/passkey/login/start", bytes.NewReader([]byte("{}")))
	startW := httptest.NewRecorder()
	h.handlePasskeyLoginStart().ServeHTTP(startW, startReq)
	if startW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, startW.Code)
	}

	var startResp passkeyLoginStartResponse
	if err := json.NewDecoder(startW.Body).Decode(&startResp); err != nil {
		t.Fatalf("failed to decode passkey start response: %v", err)
	}

	credential, err := fixture.AssertionResponse(startResp.PublicKey.Challenge, "http://localhost:8080", "localhost", userID[:], 1, false)
	if err != nil {
		t.Fatalf("failed to create passkey assertion: %v", err)
	}

	finishBody, _ := json.Marshal(map[string]any{
		"challenge_token": startResp.ChallengeToken,
		"credential":      credential,
		"remember_me":     false,
	})
	finishReq := httptest.NewRequest(http.MethodPost, "/api/auth/passkey/login/finish", bytes.NewReader(finishBody))
	finishW := httptest.NewRecorder()
	h.handlePasskeyLoginFinish().ServeHTTP(finishW, finishReq)
	if finishW.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, finishW.Code)
	}
}

func TestHandleLoginMFARequiredWithTOTPOnly(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "mfa-user@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	totpSecret := "JBSWY3DPEHPK3PXP"
	if err := store.EnableUserTOTP(context.Background(), userID, totpSecret); err != nil {
		t.Fatalf("failed to enable user totp: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"email":    "mfa-user@example.com",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleLogin().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp loginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}
	if resp.Status != "mfa_required" {
		t.Fatalf("expected mfa_required status, got %q", resp.Status)
	}
	if resp.ChallengeToken == "" {
		t.Fatalf("expected challenge token when mfa is required")
	}
	if len(resp.Factors) != 1 || resp.Factors[0] != "totp" {
		t.Fatalf("expected only totp factor, got %v", resp.Factors)
	}
	if resp.Passkey != nil {
		t.Fatalf("expected no passkey options for totp-only user")
	}

	for _, cookie := range w.Header().Values("Set-Cookie") {
		if bytes.Contains([]byte(cookie), []byte(auth.CookieName+"=")) {
			t.Fatalf("did not expect auth cookie before mfa completion")
		}
	}
}

func TestHandleLoginMFARequiredWithPasskeyOnly(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "mfa-passkey-only@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	fixture, err := testutil.NewPasskeyFixture()
	if err != nil {
		t.Fatalf("failed to create passkey fixture: %v", err)
	}
	_, err = store.CreateUserPasskeyCredential(context.Background(), userID, "MFA Passkey", fixture.Credential())
	if err != nil {
		t.Fatalf("failed to create user passkey: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"email":    "mfa-passkey-only@example.com",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleLogin().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp loginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}
	if resp.Status != "mfa_required" {
		t.Fatalf("expected mfa_required status, got %q", resp.Status)
	}
	if resp.ChallengeToken == "" {
		t.Fatalf("expected challenge token when mfa is required")
	}
	if len(resp.Factors) != 1 || resp.Factors[0] != "passkey" {
		t.Fatalf("expected only passkey factor, got %v", resp.Factors)
	}
	if resp.Passkey == nil || len(resp.Passkey.Challenge) == 0 {
		t.Fatalf("expected passkey request options in mfa response")
	}

	for _, cookie := range w.Header().Values("Set-Cookie") {
		if bytes.Contains([]byte(cookie), []byte(auth.CookieName+"=")) {
			t.Fatalf("did not expect auth cookie before mfa completion")
		}
	}
}

func TestHandleMFATOTPVerify(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "mfa-verify@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	totpSecret := "JBSWY3DPEHPK3PXP"
	if err := store.EnableUserTOTP(context.Background(), userID, totpSecret); err != nil {
		t.Fatalf("failed to enable user totp: %v", err)
	}

	loginBody, _ := json.Marshal(map[string]any{
		"email":       "mfa-verify@example.com",
		"password":    "password123",
		"remember_me": true,
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(loginBody))
	loginW := httptest.NewRecorder()
	h.handleLogin().ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, loginW.Code)
	}

	var loginResp loginResponse
	if err := json.NewDecoder(loginW.Body).Decode(&loginResp); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}
	if loginResp.Status != "mfa_required" || loginResp.ChallengeToken == "" {
		t.Fatalf("expected mfa_required response with challenge token, got %+v", loginResp)
	}

	code, err := security.GenerateCurrentTOTPCode(totpSecret, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to generate totp code: %v", err)
	}

	verifyBody, _ := json.Marshal(map[string]any{
		"challenge_token": loginResp.ChallengeToken,
		"code":            code,
	})
	verifyReq := httptest.NewRequest(http.MethodPost, "/api/auth/mfa/totp/verify", bytes.NewReader(verifyBody))
	verifyW := httptest.NewRecorder()
	h.handleMFATOTPVerify().ServeHTTP(verifyW, verifyReq)

	if verifyW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, verifyW.Code)
	}

	var verifyResp loginResponse
	if err := json.NewDecoder(verifyW.Body).Decode(&verifyResp); err != nil {
		t.Fatalf("failed to decode mfa verify response: %v", err)
	}
	if verifyResp.Status != "ok" {
		t.Fatalf("expected ok status after mfa verification, got %q", verifyResp.Status)
	}

	cookies := verifyW.Header().Values("Set-Cookie")
	foundAuth := false
	foundRemember := false
	for _, cookie := range cookies {
		if bytes.Contains([]byte(cookie), []byte(auth.CookieName+"=")) {
			foundAuth = true
		}
		if bytes.Contains([]byte(cookie), []byte(auth.RememberMeCookieName+"=")) {
			foundRemember = true
		}
	}
	if !foundAuth {
		t.Fatalf("expected auth cookie after mfa totp verification")
	}
	if !foundRemember {
		t.Fatalf("expected remember me cookie after mfa totp verification")
	}
}

func TestHandleMFATOTPVerifyKeepsChallengeAfterInvalidCode(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "mfa-retry@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	totpSecret := "JBSWY3DPEHPK3PXP"
	if err := store.EnableUserTOTP(context.Background(), userID, totpSecret); err != nil {
		t.Fatalf("failed to enable user totp: %v", err)
	}

	loginBody, _ := json.Marshal(map[string]any{
		"email":       "mfa-retry@example.com",
		"password":    "password123",
		"remember_me": false,
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(loginBody))
	loginW := httptest.NewRecorder()
	h.handleLogin().ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, loginW.Code)
	}

	var loginResp loginResponse
	if err := json.NewDecoder(loginW.Body).Decode(&loginResp); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}
	if loginResp.Status != "mfa_required" || loginResp.ChallengeToken == "" {
		t.Fatalf("expected mfa_required response with challenge token, got %+v", loginResp)
	}

	currentCode, err := security.GenerateCurrentTOTPCode(totpSecret, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to generate baseline totp code: %v", err)
	}
	invalidCode := "0" + currentCode[1:]
	if currentCode[0] == '0' {
		invalidCode = "1" + currentCode[1:]
	}

	firstVerifyBody, _ := json.Marshal(map[string]any{
		"challenge_token": loginResp.ChallengeToken,
		"code":            invalidCode,
	})
	firstVerifyReq := httptest.NewRequest(http.MethodPost, "/api/auth/mfa/totp/verify", bytes.NewReader(firstVerifyBody))
	firstVerifyW := httptest.NewRecorder()
	h.handleMFATOTPVerify().ServeHTTP(firstVerifyW, firstVerifyReq)

	if firstVerifyW.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d for invalid totp, got %d", http.StatusUnauthorized, firstVerifyW.Code)
	}

	validCode, err := security.GenerateCurrentTOTPCode(totpSecret, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to generate totp code: %v", err)
	}
	secondVerifyBody, _ := json.Marshal(map[string]any{
		"challenge_token": loginResp.ChallengeToken,
		"code":            validCode,
	})
	secondVerifyReq := httptest.NewRequest(http.MethodPost, "/api/auth/mfa/totp/verify", bytes.NewReader(secondVerifyBody))
	secondVerifyW := httptest.NewRecorder()
	h.handleMFATOTPVerify().ServeHTTP(secondVerifyW, secondVerifyReq)

	if secondVerifyW.Code != http.StatusOK {
		t.Fatalf("expected status %d after retry with valid code, got %d", http.StatusOK, secondVerifyW.Code)
	}
}

func TestHandleMFAPasskeyLoginFinish(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "mfa-passkey@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	totpSecret := "JBSWY3DPEHPK3PXP"
	if err := store.EnableUserTOTP(context.Background(), userID, totpSecret); err != nil {
		t.Fatalf("failed to enable user totp: %v", err)
	}

	fixture, err := testutil.NewPasskeyFixture()
	if err != nil {
		t.Fatalf("failed to create passkey fixture: %v", err)
	}

	_, err = store.CreateUserPasskeyCredential(context.Background(), userID, "MFA Passkey", fixture.Credential())
	if err != nil {
		t.Fatalf("failed to create user passkey: %v", err)
	}

	loginBody, _ := json.Marshal(map[string]any{
		"email":       "mfa-passkey@example.com",
		"password":    "password123",
		"remember_me": true,
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(loginBody))
	loginW := httptest.NewRecorder()
	h.handleLogin().ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, loginW.Code)
	}

	var loginResp loginResponse
	if err := json.NewDecoder(loginW.Body).Decode(&loginResp); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}
	if loginResp.Status != "mfa_required" || loginResp.ChallengeToken == "" {
		t.Fatalf("expected mfa_required response with challenge token, got %+v", loginResp)
	}
	if !containsFactor(loginResp.Factors, "totp") || !containsFactor(loginResp.Factors, "passkey") {
		t.Fatalf("expected both totp and passkey factors, got %v", loginResp.Factors)
	}
	if loginResp.Passkey == nil || len(loginResp.Passkey.Challenge) == 0 {
		t.Fatalf("expected passkey request options in mfa response")
	}
	if loginResp.Passkey.UserVerification != "required" {
		t.Fatalf("expected required user verification for mfa passkey flow, got %q", loginResp.Passkey.UserVerification)
	}

	credential, err := fixture.AssertionResponse(loginResp.Passkey.Challenge, "http://localhost:8080", "localhost", userID[:], 1, true)
	if err != nil {
		t.Fatalf("failed to create passkey assertion: %v", err)
	}

	finishBody, _ := json.Marshal(map[string]any{
		"challenge_token": loginResp.ChallengeToken,
		"credential":      credential,
		// This should be ignored for MFA flow; remember-me comes from the challenge created on /api/login.
		"remember_me": false,
	})
	finishReq := httptest.NewRequest(http.MethodPost, "/api/auth/passkey/login/finish", bytes.NewReader(finishBody))
	finishW := httptest.NewRecorder()
	h.handlePasskeyLoginFinish().ServeHTTP(finishW, finishReq)
	if finishW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, finishW.Code)
	}

	cookies := finishW.Header().Values("Set-Cookie")
	foundAuth := false
	foundRemember := false
	for _, cookie := range cookies {
		if bytes.Contains([]byte(cookie), []byte(auth.CookieName+"=")) {
			foundAuth = true
		}
		if bytes.Contains([]byte(cookie), []byte(auth.RememberMeCookieName+"=")) {
			foundRemember = true
		}
	}
	if !foundAuth {
		t.Fatalf("expected auth cookie after mfa passkey verification")
	}
	if !foundRemember {
		t.Fatalf("expected remember me cookie to follow login challenge in mfa flow")
	}
}

func TestHandleAcceptInviteActivatesPendingTeamInvite(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	ownerHash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash owner password: %v", err)
	}
	ownerID, err := store.CreateUser(context.Background(), "owner-invite@example.com", ownerHash)
	if err != nil {
		t.Fatalf("failed to create owner user: %v", err)
	}

	inviteeHash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash invitee password: %v", err)
	}
	inviteeID, err := store.CreateUser(context.Background(), "accept-invite@example.com", inviteeHash)
	if err != nil {
		t.Fatalf("failed to create invitee user: %v", err)
	}

	teamID := uuid.New()
	if _, err := store.DB().ExecContext(context.Background(),
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		teamID, "Accept Invite", time.Now().UTC(),
	); err != nil {
		t.Fatalf("failed to insert team: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), teamID, ownerID, database.TenantRoleOwner, ownerID); err != nil {
		t.Fatalf("failed to add owner to team: %v", err)
	}
	if _, err := store.CreateTeamInvite(context.Background(), teamID, "accept-invite@example.com", database.TenantRoleAdmin, &inviteeID, ownerID); err != nil {
		t.Fatalf("failed to create team invite: %v", err)
	}

	token, err := store.CreatePasswordResetToken(context.Background(), "accept-invite@example.com")
	if err != nil {
		t.Fatalf("failed to create invite token: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"token":    token,
		"password": "new-password-123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/accept-invite", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleAcceptInvite().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	isMember, err := store.IsTenantMember(context.Background(), teamID, inviteeID)
	if err != nil {
		t.Fatalf("check invitee membership: %v", err)
	}
	if !isMember {
		t.Fatalf("expected invitee to become a team member")
	}

	role, err := store.GetTenantRole(context.Background(), teamID, inviteeID)
	if err != nil {
		t.Fatalf("get invitee role: %v", err)
	}
	if role != database.TenantRoleAdmin {
		t.Fatalf("expected invitee role %q, got %q", database.TenantRoleAdmin, role)
	}

	invites, err := store.ListTeamInvites(context.Background(), teamID)
	if err != nil {
		t.Fatalf("list team invites: %v", err)
	}
	if len(invites) != 0 {
		t.Fatalf("expected no pending invites after acceptance, got %d", len(invites))
	}
}

func TestHandleLoginIncludesRecoveryCodeFactor(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "recovery-factor@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate totp secret: %v", err)
	}
	if err := store.EnableUserTOTP(context.Background(), userID, secret); err != nil {
		t.Fatalf("enable totp: %v", err)
	}

	codes, err := security.GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("generate recovery codes: %v", err)
	}
	hashes := make([]string, 0, len(codes))
	for _, code := range codes {
		hash, err := security.HashRecoveryCode(code)
		if err != nil {
			t.Fatalf("hash recovery code: %v", err)
		}
		hashes = append(hashes, hash)
	}
	if err := store.ReplaceUserRecoveryCodes(context.Background(), userID, hashes); err != nil {
		t.Fatalf("replace recovery codes: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"email":    "recovery-factor@example.com",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleLogin().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp loginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if resp.Status != "mfa_required" {
		t.Fatalf("expected mfa_required status, got %q", resp.Status)
	}
	if !containsFactor(resp.Factors, "totp") {
		t.Fatalf("expected totp factor, got %v", resp.Factors)
	}
	if !containsFactor(resp.Factors, "recovery_code") {
		t.Fatalf("expected recovery code factor, got %v", resp.Factors)
	}
}

func TestHandleMFARecoveryCodeVerify(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	hashed, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "recovery-verify@example.com", hashed)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate totp secret: %v", err)
	}
	if err := store.EnableUserTOTP(context.Background(), userID, secret); err != nil {
		t.Fatalf("enable totp: %v", err)
	}

	codes, err := security.GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("generate recovery codes: %v", err)
	}
	hashes := make([]string, 0, len(codes))
	for _, code := range codes {
		hash, err := security.HashRecoveryCode(code)
		if err != nil {
			t.Fatalf("hash recovery code: %v", err)
		}
		hashes = append(hashes, hash)
	}
	if err := store.ReplaceUserRecoveryCodes(context.Background(), userID, hashes); err != nil {
		t.Fatalf("replace recovery codes: %v", err)
	}

	loginBody, _ := json.Marshal(map[string]any{
		"email":       "recovery-verify@example.com",
		"password":    "password123",
		"remember_me": true,
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(loginBody))
	loginW := httptest.NewRecorder()
	h.handleLogin().ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, loginW.Code)
	}

	var loginResp loginResponse
	if err := json.NewDecoder(loginW.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResp.ChallengeToken == "" {
		t.Fatal("expected challenge token")
	}

	t.Run("invalid code", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"challenge_token": loginResp.ChallengeToken,
			"code":            "BAD-CODE",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/mfa/recovery-code/verify", bytes.NewReader(body))
		w := httptest.NewRecorder()
		h.handleMFARecoveryCodeVerify().ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
		}

		challengeID, err := uuid.Parse(loginResp.ChallengeToken)
		if err != nil {
			t.Fatalf("parse challenge token: %v", err)
		}
		if _, found := h.ctx.AuthState.GetPasskeyLoginChallenge(challengeID); !found {
			t.Fatal("expected mfa challenge to remain after invalid recovery code")
		}
	})

	t.Run("success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"challenge_token": loginResp.ChallengeToken,
			"code":            codes[0],
		})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/mfa/recovery-code/verify", bytes.NewReader(body))
		w := httptest.NewRecorder()
		h.handleMFARecoveryCodeVerify().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		remaining, err := store.CountActiveRecoveryCodes(context.Background(), userID)
		if err != nil {
			t.Fatalf("count active recovery codes: %v", err)
		}
		if remaining != len(codes)-1 {
			t.Fatalf("expected %d remaining recovery codes, got %d", len(codes)-1, remaining)
		}

		challengeID, err := uuid.Parse(loginResp.ChallengeToken)
		if err != nil {
			t.Fatalf("parse challenge token: %v", err)
		}
		if _, found := h.ctx.AuthState.GetPasskeyLoginChallenge(challengeID); found {
			t.Fatal("expected mfa challenge to be deleted after recovery code verification")
		}

		cookies := w.Header().Values("Set-Cookie")
		foundAuth := false
		foundRemember := false
		for _, cookie := range cookies {
			if bytes.Contains([]byte(cookie), []byte(auth.CookieName+"=")) {
				foundAuth = true
			}
			if bytes.Contains([]byte(cookie), []byte(auth.RememberMeCookieName+"=")) {
				foundRemember = true
			}
		}
		if !foundAuth {
			t.Fatalf("expected auth cookie after recovery code verification")
		}
		if !foundRemember {
			t.Fatalf("expected remember me cookie after recovery code verification")
		}
	})
}

func containsFactor(factors []string, factor string) bool {
	return slices.Contains(factors, factor)
}
