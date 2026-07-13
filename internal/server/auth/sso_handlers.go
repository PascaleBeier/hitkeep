package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"hitkeep/internal/api"
	"hitkeep/internal/appurl"
	"hitkeep/internal/database"
	"hitkeep/internal/security"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/sso"
)

const (
	ssoFlowTTL         = 10 * time.Minute
	ssoProviderTimeout = 10 * time.Second
)

type ssoStartRequest struct {
	Email      string `json:"email"`
	ReturnURL  string `json:"return_url"`
	RememberMe bool   `json:"remember_me"`
}

func (h *handler) handleSSOStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil || h.ctx.AuthState == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		var req ssoStartRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeSSOStartError(w, http.StatusBadRequest, "invalid_email")
			return
		}
		email, domain, err := normalizeSSOEmail(req.Email)
		if err != nil {
			writeSSOStartError(w, http.StatusBadRequest, "invalid_email")
			return
		}
		config, err := h.ctx.Store.GetEnabledTeamSSOConfigByDomain(r.Context(), domain)
		if err != nil {
			slog.Error("Failed to resolve SSO provider", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if config == nil {
			h.appendAuthAuditSystem(r, "auth.sso_login_failed", "failure", email, "SSO login could not resolve an enabled provider")
			writeSSOStartError(w, http.StatusBadRequest, "sso_unavailable")
			return
		}
		if !h.ctx.Limits().AllowsSSO(r.Context(), uuid.Nil, config.TeamID) {
			h.appendAuthAuditSystem(r, "auth.sso_login_failed", "failure", email, "SSO login is not entitled for the configured team")
			writeSSOStartError(w, http.StatusBadRequest, "sso_unavailable")
			return
		}

		_, oauthConfig, err := h.teamSSOOAuthConfig(r.Context(), config)
		if err != nil {
			slog.Warn("Failed to prepare configured SSO provider", "team_id", config.TeamID)
			h.appendAuthAuditSystem(r, "auth.sso_login_failed", "failure", email, "SSO provider configuration was unavailable")
			writeSSOStartError(w, http.StatusServiceUnavailable, "sso_unavailable")
			return
		}
		nonce, err := security.GenerateRandomChallenge(32)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		codeVerifier, err := security.GenerateRandomChallenge(32)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		state := h.ctx.AuthState.CreateSSOOAuthState(shared.SSOOAuthState{
			TeamID:       config.TeamID,
			IssuerURL:    config.IssuerURL,
			ClientID:     config.ClientID,
			Email:        email,
			Nonce:        nonce,
			CodeVerifier: codeVerifier,
			ReturnPath:   sanitizeAuthReturnPath(req.ReturnURL),
			RememberMe:   req.RememberMe,
			ExpiresAt:    time.Now().UTC().Add(ssoFlowTTL),
		})
		authURL := oauthConfig.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(codeVerifier))

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(api.SSOStartResponse{AuthURL: authURL}); err != nil {
			slog.Error("Failed to encode SSO start response", "error", err)
		}
	}
}

func (h *handler) handleSSOCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil || h.ctx.AuthState == nil {
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_failed"), http.StatusSeeOther)
			return
		}
		state, ok := h.ctx.AuthState.ConsumeSSOOAuthState(r.URL.Query().Get("state"))
		if !ok {
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_failed"), http.StatusSeeOther)
			return
		}
		if !h.ctx.Limits().AllowsSSO(r.Context(), uuid.Nil, state.TeamID) {
			h.appendAuthAuditSystem(r, "auth.sso_login_failed", "failure", state.Email, "SSO entitlement changed during login")
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_unavailable"), http.StatusSeeOther)
			return
		}
		if providerError := strings.TrimSpace(r.URL.Query().Get("error")); providerError != "" {
			h.appendAuthAuditSystem(r, "auth.sso_login_failed", "failure", state.Email, "SSO provider denied or cancelled login")
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_provider_error"), http.StatusSeeOther)
			return
		}
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if code == "" {
			h.appendAuthAuditSystem(r, "auth.sso_login_failed", "failure", state.Email, "SSO callback omitted authorization code")
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_failed"), http.StatusSeeOther)
			return
		}

		config, err := h.ctx.Store.GetTeamSSOConfig(r.Context(), state.TeamID)
		if err != nil || !ssoStateMatchesConfig(state, config) {
			h.appendAuthAuditSystem(r, "auth.sso_login_failed", "failure", state.Email, "SSO configuration changed during login")
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_unavailable"), http.StatusSeeOther)
			return
		}
		provider, oauthConfig, err := h.teamSSOOAuthConfig(r.Context(), config)
		if err != nil {
			slog.Warn("Failed to prepare SSO provider during callback", "team_id", state.TeamID)
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_unavailable"), http.StatusSeeOther)
			return
		}
		token, err := oauthConfig.Exchange(h.ssoClient().Context(r.Context()), code, oauth2.VerifierOption(state.CodeVerifier))
		if err != nil {
			slog.Warn("SSO token exchange failed", "team_id", state.TeamID)
			h.appendAuthAuditSystem(r, "auth.sso_login_failed", "failure", state.Email, "SSO token exchange failed")
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_failed"), http.StatusSeeOther)
			return
		}
		rawIDToken, ok := token.Extra("id_token").(string)
		if !ok || strings.TrimSpace(rawIDToken) == "" {
			h.appendAuthAuditSystem(r, "auth.sso_login_failed", "failure", state.Email, "SSO provider omitted ID token")
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_failed"), http.StatusSeeOther)
			return
		}
		verifier := provider.VerifierContext(h.ssoClient().Context(r.Context()), &oidc.Config{ClientID: config.ClientID})
		idToken, err := verifier.Verify(r.Context(), rawIDToken)
		if err != nil || subtle.ConstantTimeCompare([]byte(idTokenNonce(idToken)), []byte(state.Nonce)) != 1 {
			h.appendAuthAuditSystem(r, "auth.sso_login_failed", "failure", state.Email, "SSO ID token validation failed")
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_failed"), http.StatusSeeOther)
			return
		}

		identity, err := extractSSOIdentity(idToken, config)
		if err != nil {
			h.appendAuthAuditSystem(r, "auth.sso_login_failed", "failure", state.Email, "SSO ID token did not contain an acceptable verified email")
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_email_unverified"), http.StatusSeeOther)
			return
		}
		if !strings.EqualFold(identity.Email, state.Email) || !slices.Contains(config.AllowedDomains, identity.Domain) {
			h.appendAuthAuditSystem(r, "auth.sso_login_failed", "failure", identity.Email, "SSO email did not match the requested account or allowed domain")
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_email_not_allowed"), http.StatusSeeOther)
			return
		}

		randomPassword, err := security.GenerateRandomChallenge(48)
		if err != nil {
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_failed"), http.StatusSeeOther)
			return
		}
		passwordHash, err := HashPassword(randomPassword)
		if err != nil {
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_failed"), http.StatusSeeOther)
			return
		}
		givenName, lastName := splitSSODisplayName(identity.DisplayName)
		resolved, err := h.ctx.Store.ResolveSSOUser(r.Context(), database.ResolveSSOUserInput{
			TeamID:       config.TeamID,
			IssuerURL:    config.IssuerURL,
			Subject:      idToken.Subject,
			Email:        identity.Email,
			GivenName:    givenName,
			LastName:     lastName,
			PasswordHash: passwordHash,
		})
		if err != nil {
			slog.Error("Failed to resolve SSO user", "error", err, "team_id", config.TeamID)
			h.appendAuthAuditSystem(r, "auth.sso_login_failed", "failure", identity.Email, "SSO identity could not be linked")
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_failed"), http.StatusSeeOther)
			return
		}
		if err := h.issueLoginSession(r.Context(), w, resolved.UserID, state.RememberMe); err != nil {
			slog.Error("Failed to issue SSO login session", "error", err, "user_id", resolved.UserID)
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_failed"), http.StatusSeeOther)
			return
		}
		details := "SSO login succeeded"
		if resolved.Created {
			details = "SSO login succeeded and created a user"
		}
		h.appendAuthAuditForUserTeams(r, resolved.UserID, "auth.sso_login_succeeded", "success", details, true)
		http.Redirect(w, r, h.publicRedirectURL(state.ReturnPath), http.StatusSeeOther)
	}
}

func (h *handler) teamSSOOAuthConfig(ctx context.Context, config *database.TeamSSOConfig) (*oidc.Provider, *oauth2.Config, error) {
	if config == nil || h.ctx.Config == nil {
		return nil, nil, errors.New("SSO configuration is unavailable")
	}
	box, err := sso.NewSecretBox(h.ctx.Config.JWTSecret)
	if err != nil {
		return nil, nil, err
	}
	clientSecret, err := box.Open(config.ClientSecretEncrypted)
	if err != nil {
		return nil, nil, err
	}
	discoveryCtx, cancel := context.WithTimeout(ctx, ssoProviderTimeout)
	defer cancel()
	provider, err := h.ssoClient().Discover(discoveryCtx, config.IssuerURL)
	if err != nil {
		return nil, nil, err
	}
	return provider, &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  appurl.Path(h.ctx.Config.PublicURL, "/api/auth/sso/callback"),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}, nil
}

func ssoStateMatchesConfig(state shared.SSOOAuthState, config *database.TeamSSOConfig) bool {
	return config != nil && config.Enabled && state.TeamID == config.TeamID && subtle.ConstantTimeCompare([]byte(state.IssuerURL), []byte(config.IssuerURL)) == 1 && subtle.ConstantTimeCompare([]byte(state.ClientID), []byte(config.ClientID)) == 1
}

type ssoIdentity struct {
	Email       string
	Domain      string
	DisplayName string
}

func extractSSOIdentity(idToken *oidc.IDToken, config *database.TeamSSOConfig) (ssoIdentity, error) {
	if idToken == nil || strings.TrimSpace(idToken.Subject) == "" {
		return ssoIdentity{}, errors.New("OIDC subject is required")
	}
	claims := make(map[string]any)
	if err := idToken.Claims(&claims); err != nil {
		return ssoIdentity{}, errors.New("could not decode OIDC claims")
	}
	verified, ok := claims["email_verified"].(bool)
	if !ok || !verified {
		return ssoIdentity{}, errors.New("OIDC email is not verified")
	}
	emailClaim, ok := ssoClaimString(claims, config.EmailClaim)
	if !ok {
		return ssoIdentity{}, errors.New("OIDC email claim is missing")
	}
	email, domain, err := normalizeSSOEmail(emailClaim)
	if err != nil {
		return ssoIdentity{}, err
	}
	displayName, _ := ssoClaimString(claims, config.DisplayNameClaim)
	return ssoIdentity{Email: email, Domain: domain, DisplayName: strings.TrimSpace(displayName)}, nil
}

func ssoClaimString(claims map[string]any, path string) (string, bool) {
	var current any = claims
	for _, segment := range strings.Split(strings.TrimSpace(path), ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		current, ok = object[segment]
		if !ok {
			return "", false
		}
	}
	value, ok := current.(string)
	return strings.TrimSpace(value), ok && strings.TrimSpace(value) != ""
}

func normalizeSSOEmail(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	address, err := mail.ParseAddress(raw)
	if err != nil || !strings.EqualFold(address.Address, raw) || strings.Count(address.Address, "@") != 1 {
		return "", "", errors.New("valid email is required")
	}
	email := strings.ToLower(address.Address)
	at := strings.LastIndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return "", "", errors.New("valid email is required")
	}
	domain := strings.TrimSuffix(email[at+1:], ".")
	if domain == "" {
		return "", "", errors.New("valid email is required")
	}
	return email[:at+1] + domain, domain, nil
}

func splitSSODisplayName(displayName string) (string, string) {
	parts := strings.Fields(displayName)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func idTokenNonce(idToken *oidc.IDToken) string {
	if idToken == nil {
		return ""
	}
	return idToken.Nonce
}

func writeSSOStartError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "error",
		"code":    code,
		"message": "SSO login could not be started for this email address",
	})
}

func (h *handler) ssoClient() *sso.Client {
	if h.ctx.SSO != nil {
		return h.ctx.SSO
	}
	return sso.NewClient(nil)
}
