package sso

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"golang.org/x/oauth2"

	json "hitkeep/internal/jsonapi"
)

func TestRelyingPartyBeginUsesPKCES256(t *testing.T) {
	provider := &oidctest.Server{}
	server := httptest.NewTLSServer(provider)
	t.Cleanup(server.Close)
	provider.SetIssuer(server.URL)

	relyingParty := NewRelyingParty(NewClient(server.Client()))
	request, err := relyingParty.Begin(t.Context(), TeamConfig{
		IssuerURL:    server.URL,
		ClientID:     "hitkeep-dashboard",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://hitkeep.example/api/auth/sso/callback",
	})
	if err != nil {
		t.Fatalf("begin SSO authorization: %v", err)
	}

	authorizationURL, err := url.Parse(request.URL("opaque-state"))
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	query := authorizationURL.Query()
	if query.Get("state") != "opaque-state" {
		t.Fatalf("unexpected state %q", query.Get("state"))
	}
	if query.Get("nonce") == "" || query.Get("nonce") != request.FlowState.Nonce {
		t.Fatalf("authorization request nonce does not match flow state: %+v", request.FlowState)
	}
	if query.Get("code_challenge_method") != "S256" {
		t.Fatalf("unexpected PKCE challenge method %q", query.Get("code_challenge_method"))
	}
	if len(request.FlowState.CodeVerifier) != 43 {
		t.Fatalf("expected the RFC 7636 43-character verifier, got %d characters", len(request.FlowState.CodeVerifier))
	}
	if challenge := oauth2.S256ChallengeFromVerifier(request.FlowState.CodeVerifier); query.Get("code_challenge") != challenge {
		t.Fatalf("authorization request challenge %q does not match verifier", query.Get("code_challenge"))
	}
}

func TestRelyingPartyCompleteExchangesAndVerifiesIdentity(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	providerHandler := &oidctest.Server{PublicKeys: []oidctest.PublicKey{{
		PublicKey: &privateKey.PublicKey,
		KeyID:     "test-key",
		Algorithm: oidc.RS256,
	}}}
	var server *httptest.Server
	var expectedNonce string
	var receivedVerifier string
	discoveryRequests := 0
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			discoveryRequests++
			providerHandler.ServeHTTP(w, r)
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid token request", http.StatusBadRequest)
				return
			}
			receivedVerifier = r.Form.Get("code_verifier")
			claims, _ := json.Marshal(map[string]any{
				"iss":            server.URL,
				"sub":            "subject-123",
				"aud":            "hitkeep-dashboard",
				"exp":            time.Now().Add(5 * time.Minute).Unix(),
				"iat":            time.Now().Unix(),
				"nonce":          expectedNonce,
				"email":          "ada@example.com",
				"email_verified": true,
				"profile":        map[string]any{"display_name": "Ada Lovelace"},
			})
			rawIDToken := oidctest.SignIDToken(privateKey, "test-key", oidc.RS256, string(claims))
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, map[string]any{
				"access_token": "ephemeral-access-token",
				"token_type":   "Bearer",
				"expires_in":   300,
				"id_token":     rawIDToken,
			})

		default:
			providerHandler.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	providerHandler.SetIssuer(server.URL)

	config := TeamConfig{
		IssuerURL:        server.URL,
		ClientID:         "hitkeep-dashboard",
		ClientSecret:     "test-client-secret",
		RedirectURL:      "https://hitkeep.example/api/auth/sso/callback",
		EmailClaim:       "email",
		DisplayNameClaim: "profile.display_name",
	}
	relyingParty := NewRelyingParty(NewClient(server.Client()))
	authorization, err := relyingParty.Begin(t.Context(), config)
	if err != nil {
		t.Fatalf("begin SSO authorization: %v", err)
	}
	expectedNonce = authorization.FlowState.Nonce

	identity, err := relyingParty.Complete(t.Context(), config, authorization.FlowState, "test-code")
	if err != nil {
		t.Fatalf("complete SSO authorization: %v", err)
	}
	if identity.Subject != "subject-123" || identity.Email != "ada@example.com" || identity.Domain != "example.com" || identity.DisplayName != "Ada Lovelace" {
		t.Fatalf("unexpected identity %+v", identity)
	}
	if receivedVerifier != authorization.FlowState.CodeVerifier {
		t.Fatalf("token exchange verifier %q does not match flow state", receivedVerifier)
	}
	if discoveryRequests != 1 {
		t.Fatalf("expected Begin and Complete to share one provider discovery, got %d", discoveryRequests)
	}
}
