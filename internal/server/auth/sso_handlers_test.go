package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"hitkeep/internal/api"
	appauth "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/sso"
)

func TestSSOLoginUsesPKCENonceAndCreatesNormalSession(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProvider(t, "sso.user@example.com", true)
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	teamID := configureTestSSO(t, h, store, provider.server.URL)

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
	if err != nil || user == nil {
		t.Fatalf("expected JIT-provisioned user, user=%+v err=%v", user, err)
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
	configureTestSSO(t, h, store, provider.server.URL)

	authURL := startTestSSOLogin(t, h, "unverified@example.com", "/", false)
	parsedAuthURL, _ := url.Parse(authURL)
	provider.setNonce(parsedAuthURL.Query().Get("nonce"))
	callback := httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?state="+url.QueryEscape(parsedAuthURL.Query().Get("state"))+"&code=test-code", nil)
	w := httptest.NewRecorder()
	h.handleSSOCallback().ServeHTTP(w, callback)

	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "error=sso_email_unverified") {
		t.Fatalf("expected unverified email rejection, status=%d location=%q", w.Code, w.Header().Get("Location"))
	}
	user, err := store.GetUserByEmail(callback.Context(), "unverified@example.com")
	if err != nil || user != nil {
		t.Fatalf("unverified identity created a user: user=%+v err=%v", user, err)
	}
}

func TestSSOLoginEnforcesCloudEntitlementAtStartAndCallback(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	provider := newFakeOIDCProvider(t, "cloud-sso@example.com", true)
	h.ctx.SSO = sso.NewClient(provider.server.Client())
	configureTestSSO(t, h, store, provider.server.URL)
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

func configureTestSSO(t *testing.T, h *handler, store *database.Store, issuerURL string) uuid.UUID {
	t.Helper()
	ownerID, err := store.CreateUser(t.Context(), "sso-owner@example.net", "owner-password-hash")
	if err != nil {
		t.Fatalf("create SSO owner: %v", err)
	}
	teamID, err := store.GetActiveTenantID(t.Context(), ownerID)
	if err != nil {
		t.Fatalf("get SSO team: %v", err)
	}
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
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil || response.AuthURL == "" {
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
	privateKey    *rsa.PrivateKey
	email         string
	emailVerified bool

	mu               sync.Mutex
	expectedNonce    string
	receivedVerifier string
}

func newFakeOIDCProvider(t *testing.T, email string, emailVerified bool) *fakeOIDCProvider {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	provider := &fakeOIDCProvider{privateKey: privateKey, email: email, emailVerified: emailVerified}
	provider.server = httptest.NewTLSServer(http.HandlerFunc(provider.serveHTTP))
	t.Cleanup(provider.server.Close)
	return provider
}

func (p *fakeOIDCProvider) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                p.server.URL,
			"authorization_endpoint":                p.server.URL + "/authorize",
			"token_endpoint":                        p.server.URL + "/token",
			"jwks_uri":                              p.server.URL + "/keys",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	case "/keys":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []jose.JSONWebKey{{Key: &p.privateKey.PublicKey, KeyID: "test-key", Algorithm: string(jose.RS256), Use: "sig"}}})
	case "/token":
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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ephemeral-access-token",
			"token_type":   "Bearer",
			"expires_in":   300,
			"id_token":     rawIDToken,
		})
	default:
		http.NotFound(w, r)
	}
}

func (p *fakeOIDCProvider) signIDToken(nonce string) (string, error) {
	options := (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key")
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: p.privateKey}, options)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	return jwt.Signed(signer).
		Claims(jwt.Claims{
			Issuer:   p.server.URL,
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

func (p *fakeOIDCProvider) setNonce(nonce string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expectedNonce = nonce
}

func (p *fakeOIDCProvider) codeVerifier() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.receivedVerifier
}
