package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"hitkeep/internal/api"
	appauth "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	json "hitkeep/internal/jsonapi"
	"hitkeep/internal/sso"
)

func TestSSOErrorKindUsesStableCategories(t *testing.T) {
	for _, test := range []struct {
		name     string
		err      error
		fallback string
		want     string
	}{
		{name: "canceled", err: context.Canceled, fallback: "provider_lookup_failed", want: "canceled"},
		{name: "timeout", err: context.DeadlineExceeded, fallback: "provider_lookup_failed", want: "timeout"},
		{name: "internal provider detail", err: errors.New("OIDC response included issuer and transport details"), fallback: "provider_lookup_failed", want: "provider_lookup_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ssoErrorKind(test.err, test.fallback); got != test.want {
				t.Fatalf("ssoErrorKind() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSSOLoginUsesPKCENonceAndCreatesNormalSession(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProviderAtIssuerPath(t, "sso.user@example.com", true, "/realms/acme/")
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	teamID := configureTestSSO(t, h, store, provider.issuerURL())
	existingUserID := addTestSSOMember(t, store, teamID, "sso.user@example.com")

	authURL := startTestSSOLogin(t, h, "sso.user@example.com", "/events?range=30d", true)
	parsedAuthURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	query := parsedAuthURL.Query()
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		t.Fatalf("authorization request did not use PKCE S256: %s", authURL)
	}
	if query.Get("nonce") == "" || query.Get("state") == "" {
		t.Fatalf("authorization request omitted nonce or state: %s", authURL)
	}
	provider.setNonce(query.Get("nonce"))

	callback := httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?state="+url.QueryEscape(query.Get("state"))+"&code=test-code", nil)
	w := httptest.NewRecorder()
	h.handleSSOCallback().ServeHTTP(w, callback)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d: %s", http.StatusSeeOther, w.Code, w.Body.String())
	}
	if location := w.Header().Get("Location"); location != "http://localhost:8080/events?range=30d" {
		t.Fatalf("unexpected callback redirect %q", location)
	}
	if !hasCookie(w.Result().Cookies(), appauth.CookieName) || !hasCookie(w.Result().Cookies(), appauth.RememberMeCookieName) {
		t.Fatalf("expected normal and remembered HitKeep session cookies, got %+v", w.Result().Cookies())
	}
	verifier := provider.codeVerifier()
	if verifier == "" || oauth2.S256ChallengeFromVerifier(verifier) != query.Get("code_challenge") {
		t.Fatal("token exchange did not present the matching PKCE verifier")
	}

	user, err := store.GetUserByEmail(callback.Context(), "sso.user@example.com")
	if err != nil || user == nil || user.ID != existingUserID {
		t.Fatalf("expected existing SSO member %s, user=%+v err=%v", existingUserID, user, err)
	}
	role, err := store.GetTenantRole(callback.Context(), teamID, user.ID)
	if err != nil || role != database.TenantRoleMember {
		t.Fatalf("expected SSO team membership, role=%q err=%v", role, err)
	}

	replay := httptest.NewRecorder()
	h.handleSSOCallback().ServeHTTP(replay, callback)
	if replay.Code != http.StatusSeeOther || !strings.Contains(replay.Header().Get("Location"), "error=sso_failed") {
		t.Fatalf("expected consumed state to reject replay, status=%d location=%q", replay.Code, replay.Header().Get("Location"))
	}
}

func TestSSOLoginRejectsUnverifiedEmail(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProvider(t, "unverified@example.com", false)
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	teamID := configureTestSSO(t, h, store, provider.server.URL)
	addTestSSOMember(t, store, teamID, "unverified@example.com")

	authURL := startTestSSOLogin(t, h, "unverified@example.com", "/", false)
	parsedAuthURL, _ := url.Parse(authURL)
	provider.setNonce(parsedAuthURL.Query().Get("nonce"))
	callback := httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?state="+url.QueryEscape(parsedAuthURL.Query().Get("state"))+"&code=test-code", nil)
	w := httptest.NewRecorder()
	h.handleSSOCallback().ServeHTTP(w, callback)

	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "error=sso_email_unverified") {
		t.Fatalf("expected unverified email rejection, status=%d location=%q", w.Code, w.Header().Get("Location"))
	}
	var identityCount int
	if err := store.DB().QueryRowContext(callback.Context(), "SELECT COUNT(*) FROM sso_identities WHERE tenant_id = ?", teamID).Scan(&identityCount); err != nil || identityCount != 0 {
		t.Fatalf("unverified identity was linked: count=%d err=%v", identityCount, err)
	}
}

func TestSSOLoginRejectsIDTokenIssuerMismatch(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProviderAtIssuerPath(t, "issuer-mismatch@example.com", true, "/realms/acme/")
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	teamID := configureTestSSO(t, h, store, provider.issuerURL())
	addTestSSOMember(t, store, teamID, "issuer-mismatch@example.com")

	authURL := startTestSSOLogin(t, h, "issuer-mismatch@example.com", "/", false)
	parsedAuthURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	provider.setNonce(parsedAuthURL.Query().Get("nonce"))
	provider.setIDTokenIssuer(strings.TrimSuffix(provider.issuerURL(), "/"))

	callback := httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?state="+url.QueryEscape(parsedAuthURL.Query().Get("state"))+"&code=test-code", nil)
	w := httptest.NewRecorder()
	h.handleSSOCallback().ServeHTTP(w, callback)

	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "error=sso_failed") {
		t.Fatalf("expected issuer mismatch rejection, status=%d location=%q", w.Code, w.Header().Get("Location"))
	}
	var identityCount int
	if err := store.DB().QueryRowContext(callback.Context(), "SELECT COUNT(*) FROM sso_identities WHERE tenant_id = ?", teamID).Scan(&identityCount); err != nil || identityCount != 0 {
		t.Fatalf("issuer mismatch identity was linked: count=%d err=%v", identityCount, err)
	}
}

func TestSSOLoginEnforcesCloudEntitlementAtStartAndCallback(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProvider(t, "cloud-sso@example.com", true)
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	teamID := configureTestSSO(t, h, store, provider.server.URL)
	addTestSSOMember(t, store, teamID, "cloud-sso@example.com")
	h.ctx.Config.CloudHosted = true
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{}, entitlements.PlanInfo{Code: "pro", Name: "Pro"})

	body, _ := json.Marshal(map[string]any{"email": "cloud-sso@example.com", "return_url": "/", "remember_me": false})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sso/start", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleSSOStart().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"sso_unavailable"`) {
		t.Fatalf("expected non-Business cloud SSO start to be blocked, status=%d body=%s", w.Code, w.Body.String())
	}

	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{AllowSSO: true}, entitlements.PlanInfo{Code: "business", Name: "Business"})
	authURL := startTestSSOLogin(t, h, "cloud-sso@example.com", "/", false)
	parsedAuthURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	provider.setNonce(parsedAuthURL.Query().Get("nonce"))

	// A downgrade during an in-flight login invalidates the consumed state
	// before HitKeep exchanges the authorization code.
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{}, entitlements.PlanInfo{Code: "pro", Name: "Pro"})
	callback := httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?state="+url.QueryEscape(parsedAuthURL.Query().Get("state"))+"&code=test-code", nil)
	w = httptest.NewRecorder()
	h.handleSSOCallback().ServeHTTP(w, callback)
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "error=sso_unavailable") {
		t.Fatalf("expected cloud downgrade to block callback, status=%d location=%q", w.Code, w.Header().Get("Location"))
	}
	if provider.codeVerifier() != "" {
		t.Fatal("downgraded SSO callback exchanged an authorization code")
	}
}

func TestSSOLoginRejectsTrustedDomainWithoutTeamAuthorization(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProvider(t, "outsider@example.com", true)
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	configureTestSSO(t, h, store, provider.server.URL)

	body, _ := json.Marshal(map[string]any{"email": "outsider@example.com", "return_url": "/"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sso/start", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleSSOStart().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), `"code":"sso_access_denied"`) {
		t.Fatalf("expected trusted domain without authorization to fail, status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSSOLoginAutoProvisionsTrustedDomainUserAsMember(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProvider(t, "new-member@example.com", true)
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	teamID := configureTestSSO(t, h, store, provider.server.URL)
	enableTestSSOAutoProvision(t, store, teamID)

	authURL := startTestSSOLogin(t, h, "new-member@example.com", "/dashboard", false)
	parsedAuthURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	provider.setNonce(parsedAuthURL.Query().Get("nonce"))
	callback := httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?state="+url.QueryEscape(parsedAuthURL.Query().Get("state"))+"&code=test-code", nil)
	callback.Header.Set("X-Request-Id", "sso-auto-provision-request")
	response := httptest.NewRecorder()
	h.handleSSOCallback().ServeHTTP(response, callback)

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "http://localhost:8080/dashboard" {
		t.Fatalf("complete auto-provisioned SSO login: status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
	user, err := store.GetUserByEmail(t.Context(), "new-member@example.com")
	if err != nil || user == nil {
		t.Fatalf("load auto-provisioned user: user=%+v err=%v", user, err)
	}
	role, err := store.GetTenantRole(t.Context(), teamID, user.ID)
	if err != nil || role != database.TenantRoleMember {
		t.Fatalf("expected auto-provisioned Member role, role=%q err=%v", role, err)
	}

	var metadataJSON, requestID, targetLabel string
	if err := store.DB().QueryRowContext(t.Context(), `
		SELECT metadata_json, request_id, target_label
		FROM instance_audit_log
		WHERE action = 'auth.sso_login_succeeded'
		ORDER BY created_at DESC
		LIMIT 1
	`).Scan(&metadataJSON, &requestID, &targetLabel); err != nil {
		t.Fatalf("load SSO success audit metadata: %v", err)
	}
	var metadata map[string]string
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		t.Fatalf("decode SSO success audit metadata: %v", err)
	}
	if metadata["flow"] != "callback" || metadata["reason"] != "login_succeeded" || metadata["access_mode"] != "auto_provision" || metadata["email_domain"] != "example.com" || metadata["configured_team_id"] != teamID.String() {
		t.Fatalf("unexpected SSO success metadata: %+v", metadata)
	}
	if requestID != "sso-auto-provision-request" {
		t.Fatalf("expected request ID correlation, got %q", requestID)
	}
	if targetLabel != "new-member@example.com" || strings.Contains(metadataJSON, "test-client-secret") || strings.Contains(metadataJSON, "test-code") {
		t.Fatalf("SSO audit metadata exposed sensitive material: target=%q metadata=%q", targetLabel, metadataJSON)
	}
}

func TestSSOLoginAutoProvisioningEnforcesCloudSeatLimit(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProvider(t, "over-limit@example.com", true)
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	teamID := configureTestSSO(t, h, store, provider.server.URL)
	enableTestSSOAutoProvision(t, store, teamID)
	h.ctx.Config.CloudHosted = true
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{
		AllowSSO:       true,
		MaxTeamMembers: 1,
		MaxTeams:       1,
	}, entitlements.PlanInfo{Code: "business", Name: "Business"})

	body, _ := json.Marshal(map[string]any{"email": "over-limit@example.com", "return_url": "/"})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/sso/start", bytes.NewReader(body))
	response := httptest.NewRecorder()
	h.handleSSOStart().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"code":"sso_access_denied"`) {
		t.Fatalf("expected cloud seat limit to block JIT provisioning, status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSSOLoginAutoProvisioningEnforcesCloudTeamMembershipLimit(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProvider(t, "at-team-limit@example.com", true)
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	targetTeamID := configureTestSSO(t, h, store, provider.server.URL)
	enableTestSSOAutoProvision(t, store, targetTeamID)
	userID, err := store.CreateUserWithoutDefaultTenant(t.Context(), "at-team-limit@example.com", "existing-password-hash")
	if err != nil {
		t.Fatalf("create existing user: %v", err)
	}
	existingTeam, err := store.CreateTenant(t.Context(), userID, "Existing team", "")
	if err != nil {
		t.Fatalf("create existing team: %v", err)
	}
	if err := store.SetActiveTenantID(t.Context(), userID, existingTeam.ID); err != nil {
		t.Fatalf("set existing active team: %v", err)
	}
	h.ctx.Config.CloudHosted = true
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{
		AllowSSO:       true,
		MaxTeamMembers: 10,
		MaxTeams:       1,
	}, entitlements.PlanInfo{Code: "business", Name: "Business"})

	body, _ := json.Marshal(map[string]any{"email": "at-team-limit@example.com", "return_url": "/"})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/sso/start", bytes.NewReader(body))
	response := httptest.NewRecorder()
	h.handleSSOStart().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"code":"sso_access_denied"`) {
		t.Fatalf("expected cloud team limit to block JIT provisioning, status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSSOInvitationFlowPreservesTokenAndRequestedRole(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProvider(t, "invited-admin@example.com", true)
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	teamID := configureTestSSO(t, h, store, provider.server.URL)
	var ownerID uuid.UUID
	if err := store.DB().QueryRowContext(context.Background(), "SELECT user_id FROM tenant_members WHERE tenant_id = ? AND role = ? LIMIT 1", teamID, database.TenantRoleOwner).Scan(&ownerID); err != nil {
		t.Fatalf("get team owner: %v", err)
	}
	inviteeID, err := store.CreateUserWithoutDefaultTenant(context.Background(), "invited-admin@example.com", "temporary-password-hash")
	if err != nil {
		t.Fatalf("create invitee: %v", err)
	}
	if _, err := store.CreateTeamInvite(context.Background(), teamID, "invited-admin@example.com", database.TenantRoleAdmin, &inviteeID, ownerID, true); err != nil {
		t.Fatalf("create team invite: %v", err)
	}
	inviteToken, err := store.CreatePasswordResetToken(context.Background(), "invited-admin@example.com")
	if err != nil {
		t.Fatalf("create invite token: %v", err)
	}

	availabilityBody, _ := json.Marshal(map[string]string{"invite_token": inviteToken})
	availabilityRequest := httptest.NewRequest(http.MethodPost, "/api/auth/sso/invite", bytes.NewReader(availabilityBody))
	availabilityResponse := httptest.NewRecorder()
	h.handleSSOInviteAvailability().ServeHTTP(availabilityResponse, availabilityRequest)
	if availabilityResponse.Code != http.StatusOK || !strings.Contains(availabilityResponse.Body.String(), `"enabled":true`) {
		t.Fatalf("expected invitation SSO availability, status=%d body=%s", availabilityResponse.Code, availabilityResponse.Body.String())
	}

	startBody, _ := json.Marshal(map[string]any{"invite_token": inviteToken, "return_url": "/dashboard"})
	startRequest := httptest.NewRequest(http.MethodPost, "/api/auth/sso/start", bytes.NewReader(startBody))
	startResponse := httptest.NewRecorder()
	h.handleSSOStart().ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusOK {
		t.Fatalf("start invited SSO login: status=%d body=%s", startResponse.Code, startResponse.Body.String())
	}
	var started api.SSOStartResponse
	if err := json.UnmarshalRead(startResponse.Body, &started); err != nil {
		t.Fatalf("decode invited SSO response: %v", err)
	}
	authURL, _ := url.Parse(started.AuthURL)
	provider.setNonce(authURL.Query().Get("nonce"))

	callback := httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?state="+url.QueryEscape(authURL.Query().Get("state"))+"&code=test-code", nil)
	callbackResponse := httptest.NewRecorder()
	h.handleSSOCallback().ServeHTTP(callbackResponse, callback)
	if callbackResponse.Code != http.StatusSeeOther || callbackResponse.Header().Get("Location") != "http://localhost:8080/dashboard" {
		t.Fatalf("complete invited SSO login: status=%d location=%q", callbackResponse.Code, callbackResponse.Header().Get("Location"))
	}
	role, err := store.GetTenantRole(context.Background(), teamID, inviteeID)
	if err != nil || role != database.TenantRoleAdmin {
		t.Fatalf("expected invited admin role, role=%q err=%v", role, err)
	}
	if _, err := store.ResolvePasswordResetEmail(context.Background(), inviteToken); err == nil {
		t.Fatal("expected invite token to be consumed after verified identity")
	}
}

func TestSSOInvitationProviderErrorReturnsToInvitationWithoutConsumingToken(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProvider(t, "invited-error@example.com", true)
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	teamID := configureTestSSO(t, h, store, provider.server.URL)
	var ownerID uuid.UUID
	if err := store.DB().QueryRowContext(t.Context(), "SELECT user_id FROM tenant_members WHERE tenant_id = ? AND role = ? LIMIT 1", teamID, database.TenantRoleOwner).Scan(&ownerID); err != nil {
		t.Fatalf("get team owner: %v", err)
	}
	inviteeID, err := store.CreateUserWithoutDefaultTenant(t.Context(), "invited-error@example.com", "temporary-password-hash")
	if err != nil {
		t.Fatalf("create invitee: %v", err)
	}
	if _, err := store.CreateTeamInvite(t.Context(), teamID, "invited-error@example.com", database.TenantRoleMember, &inviteeID, ownerID, true); err != nil {
		t.Fatalf("create team invite: %v", err)
	}
	inviteToken, err := store.CreatePasswordResetToken(t.Context(), "invited-error@example.com")
	if err != nil {
		t.Fatalf("create invite token: %v", err)
	}

	startBody, _ := json.Marshal(map[string]any{"invite_token": inviteToken, "return_url": "/dashboard"})
	startRequest := httptest.NewRequest(http.MethodPost, "/api/auth/sso/start", bytes.NewReader(startBody))
	startResponse := httptest.NewRecorder()
	h.handleSSOStart().ServeHTTP(startResponse, startRequest)
	var started api.SSOStartResponse
	if err := json.UnmarshalRead(startResponse.Body, &started); err != nil {
		t.Fatalf("decode SSO start: %v", err)
	}
	authURL, _ := url.Parse(started.AuthURL)

	callback := httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?state="+url.QueryEscape(authURL.Query().Get("state"))+"&error=access_denied", nil)
	response := httptest.NewRecorder()
	h.handleSSOCallback().ServeHTTP(response, callback)

	expected := "http://localhost:8080/accept-invite?error=sso_provider_error&token=" + url.QueryEscape(inviteToken)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != expected {
		t.Fatalf("expected invitation error redirect %q, status=%d location=%q", expected, response.Code, response.Header().Get("Location"))
	}
	if email, err := store.ResolvePasswordResetEmail(t.Context(), inviteToken); err != nil || email != "invited-error@example.com" {
		t.Fatalf("expected failed provider flow to preserve invite token, email=%q err=%v", email, err)
	}
}

func configureTestSSO(t *testing.T, h *handler, store *database.Store, issuerURL string) uuid.UUID {
	t.Helper()
	if _, err := store.CreateUser(t.Context(), "sso-instance-owner@example.net", "instance-owner-password-hash"); err != nil {
		t.Fatalf("create instance owner: %v", err)
	}
	ownerID, err := store.CreateUserWithoutDefaultTenant(t.Context(), "sso-owner@example.net", "owner-password-hash")
	if err != nil {
		t.Fatalf("create SSO owner: %v", err)
	}
	team, err := store.CreateTenant(t.Context(), ownerID, "SSO Team", "")
	if err != nil {
		t.Fatalf("get SSO team: %v", err)
	}
	teamID := team.ID
	box, _ := sso.NewSecretBox(h.ctx.Config.JWTSecret)
	secret, _ := box.Seal("test-client-secret")
	if err := store.UpsertTeamSSOConfig(t.Context(), database.TeamSSOConfig{
		TeamID:                teamID,
		ProviderType:          "oidc",
		IssuerURL:             issuerURL,
		ClientID:              "hitkeep-dashboard",
		ClientSecretEncrypted: secret,
		AllowedDomains:        []string{"example.com"},
		EmailClaim:            "email",
		DisplayNameClaim:      "name",
		Enabled:               true,
	}); err != nil {
		t.Fatalf("configure SSO: %v", err)
	}
	return teamID
}

func enableTestSSOAutoProvision(t *testing.T, store *database.Store, teamID uuid.UUID) {
	t.Helper()
	config, err := store.GetTeamSSOConfig(t.Context(), teamID)
	if err != nil || config == nil {
		t.Fatalf("load SSO config: config=%+v err=%v", config, err)
	}
	config.AutoProvision = true
	if err := store.UpsertTeamSSOConfig(t.Context(), *config); err != nil {
		t.Fatalf("enable SSO auto provisioning: %v", err)
	}
}

func addTestSSOMember(t *testing.T, store *database.Store, teamID uuid.UUID, email string) uuid.UUID {
	t.Helper()
	userID, err := store.CreateUserWithoutDefaultTenant(t.Context(), email, "existing-password-hash")
	if err != nil {
		t.Fatalf("create SSO member: %v", err)
	}
	if err := store.AddTeamMember(t.Context(), teamID, userID, database.TenantRoleMember, uuid.Nil); err != nil {
		t.Fatalf("add SSO member: %v", err)
	}
	return userID
}

func startTestSSOLogin(t *testing.T, h *handler, email, returnURL string, rememberMe bool) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"email": email, "return_url": returnURL, "remember_me": rememberMe})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sso/start", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleSSOStart().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("start SSO login: status=%d body=%s", w.Code, w.Body.String())
	}
	var response api.SSOStartResponse
	if err := json.UnmarshalRead(w.Body, &response); err != nil || response.AuthURL == "" {
		t.Fatalf("decode SSO start response: response=%+v err=%v", response, err)
	}
	return response.AuthURL
}

func hasCookie(cookies []*http.Cookie, name string) bool {
	for _, cookie := range cookies {
		if cookie.Name == name && cookie.Value != "" {
			return true
		}
	}
	return false
}

type fakeOIDCProvider struct {
	server        *httptest.Server
	standard      *oidctest.Server
	privateKey    *rsa.PrivateKey
	email         string
	emailVerified bool
	issuerPath    string

	mu               sync.Mutex
	expectedNonce    string
	receivedVerifier string
	idTokenIssuer    string
}

func newFakeOIDCProvider(t *testing.T, email string, emailVerified bool) *fakeOIDCProvider {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	provider := &fakeOIDCProvider{
		privateKey:    privateKey,
		email:         email,
		emailVerified: emailVerified,
		standard: &oidctest.Server{PublicKeys: []oidctest.PublicKey{{
			PublicKey: &privateKey.PublicKey,
			KeyID:     "test-key",
			Algorithm: oidc.RS256,
		}}},
	}
	provider.server = httptest.NewTLSServer(http.HandlerFunc(provider.serveHTTP))
	provider.standard.SetIssuer(provider.server.URL)
	t.Cleanup(provider.server.Close)
	return provider
}

func newFakeOIDCProviderAtIssuerPath(t *testing.T, email string, emailVerified bool, issuerPath string) *fakeOIDCProvider {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	provider := &fakeOIDCProvider{
		privateKey:    privateKey,
		email:         email,
		emailVerified: emailVerified,
		issuerPath:    issuerPath,
	}
	provider.server = httptest.NewTLSServer(http.HandlerFunc(provider.serveHTTP))
	t.Cleanup(provider.server.Close)
	return provider
}

func (p *fakeOIDCProvider) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if p.standard != nil {
		if r.URL.Path == "/token" {
			p.serveToken(w, r)
			return
		}
		p.standard.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == strings.TrimSuffix(p.issuerPath, "/")+"/.well-known/openid-configuration" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.MarshalWrite(w, map[string]any{
			"issuer":                                p.issuerURL(),
			"authorization_endpoint":                p.server.URL + "/authorize",
			"token_endpoint":                        p.server.URL + "/token",
			"jwks_uri":                              p.server.URL + "/keys",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})

		return
	}

	switch r.URL.Path {
	case "/keys":
		w.Header().Set("Content-Type", "application/json")
		_ = json.MarshalWrite(w, map[string]any{"keys": []jose.JSONWebKey{{Key: &p.privateKey.PublicKey, KeyID: "test-key", Algorithm: string(jose.RS256), Use: "sig"}}})
	case "/token":
		p.serveToken(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (p *fakeOIDCProvider) serveToken(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	p.mu.Lock()
	p.receivedVerifier = r.Form.Get("code_verifier")
	nonce := p.expectedNonce
	p.mu.Unlock()
	rawIDToken, err := p.signIDToken(nonce)
	if err != nil {
		http.Error(w, "could not sign token", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.MarshalWrite(w, map[string]any{
		"access_token": "ephemeral-access-token",
		"token_type":   "Bearer",
		"expires_in":   300,
		"id_token":     rawIDToken,
	})
}

func (p *fakeOIDCProvider) signIDToken(nonce string) (string, error) {
	options := (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key")
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: p.privateKey}, options)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	issuer := p.issuerURL()
	p.mu.Lock()
	if p.idTokenIssuer != "" {
		issuer = p.idTokenIssuer
	}
	p.mu.Unlock()
	return jwt.Signed(signer).
		Claims(jwt.Claims{
			Issuer:   issuer,
			Subject:  "subject-123",
			Audience: jwt.Audience{"hitkeep-dashboard"},
			Expiry:   jwt.NewNumericDate(now.Add(5 * time.Minute)),
			IssuedAt: jwt.NewNumericDate(now),
		}).
		Claims(map[string]any{
			"nonce":          nonce,
			"email":          p.email,
			"email_verified": p.emailVerified,
			"name":           "Ada Lovelace",
		}).Serialize()
}

func (p *fakeOIDCProvider) issuerURL() string {
	return p.server.URL + p.issuerPath
}

func (p *fakeOIDCProvider) setNonce(nonce string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expectedNonce = nonce
}

func (p *fakeOIDCProvider) setIDTokenIssuer(issuer string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.idTokenIssuer = issuer
}

func (p *fakeOIDCProvider) codeVerifier() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.receivedVerifier
}
