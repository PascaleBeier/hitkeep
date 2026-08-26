package socialauth

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	jose "github.com/go-jose/go-jose/v4"
	"golang.org/x/oauth2"

	"hitkeep/config"
	"hitkeep/internal/sso"
	json "hitkeep/jsonapi"
)

func TestProviderStatusesRequireCompleteConfiguration(t *testing.T) {
	statuses := ProviderStatuses(&config.Config{
		SocialGoogleClientID: "google-id",
		SocialGitHubClientID: "github-id", SocialGitHubClientSecret: "github-secret",
	})
	if len(statuses) != 3 || !statuses[0].Partial || statuses[0].Configured {
		t.Fatalf("expected partial Google configuration, got %+v", statuses)
	}
	if !statuses[1].Configured || statuses[1].Partial {
		t.Fatalf("expected configured GitHub provider, got %+v", statuses)
	}
	if statuses[2].Configured || statuses[2].Partial {
		t.Fatalf("expected disabled Microsoft provider, got %+v", statuses)
	}
}

func TestMicrosoftConfigurationRejectsInvalidTenantSelector(t *testing.T) {
	conf := &config.Config{
		PublicURL:                   "https://hitkeep.test",
		SocialMicrosoftClientID:     "microsoft-id",
		SocialMicrosoftClientSecret: "microsoft-secret",
		SocialMicrosoftTenant:       "not-a-tenant",
	}
	statuses := ProviderStatuses(conf)
	if statuses[2].Configured || !statuses[2].Partial {
		t.Fatalf("invalid Microsoft tenant should be degraded, got %+v", statuses[2])
	}
	if _, err := ConfigForProvider(conf, ProviderMicrosoft); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("invalid Microsoft tenant should not fall back to common: %v", err)
	}
	if err := validateProviderConfig(ProviderConfig{
		Provider: ProviderMicrosoft, ClientID: "id", ClientSecret: "secret",
		RedirectURL: "https://hitkeep.test/callback", IssuerURL: microsoftIssuerBase + "/common/v2.0",
		MicrosoftTenant: "not-a-tenant",
	}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("manual invalid Microsoft tenant configuration was accepted: %v", err)
	}
}

func TestGoogleOIDCUsesPKCEAndValidatesNonceAudienceIssuerAndVerifiedEmail(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	providerHandler := &oidctest.Server{PublicKeys: []oidctest.PublicKey{{
		PublicKey: &privateKey.PublicKey, KeyID: "social-key", Algorithm: oidc.RS256,
	}}}
	var server *httptest.Server
	var expectedNonce string
	var tokenNonce string
	var tokenIssuer string
	tokenAudience := "hitkeep-social"
	emailVerified := true
	receivedVerifier := ""
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid token request", http.StatusBadRequest)
				return
			}
			receivedVerifier = r.Form.Get("code_verifier")
			claims, _ := json.Marshal(map[string]any{
				"iss": tokenIssuer, "sub": "google-subject", "aud": tokenAudience,
				"exp": time.Now().Add(5 * time.Minute).Unix(), "iat": time.Now().Unix(),
				"nonce": tokenNonce, "email": "Ada@Example.com", "email_verified": emailVerified, "name": "Ada",
			})
			rawIDToken := oidctest.SignIDToken(privateKey, "social-key", oidc.RS256, string(claims))
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, map[string]any{"access_token": "discard-me", "token_type": "Bearer", "id_token": rawIDToken})
		default:
			providerHandler.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	providerHandler.SetIssuer(server.URL)
	tokenIssuer = server.URL

	client := NewClient(sso.NewClient(server.Client()))
	cfg := ProviderConfig{Provider: ProviderGoogle, ClientID: "hitkeep-social", ClientSecret: "secret", RedirectURL: "https://hitkeep.test/api/auth/social/google/callback", IssuerURL: server.URL}
	authorization, err := client.Begin(t.Context(), cfg)
	if err != nil {
		t.Fatalf("begin Google authorization: %v", err)
	}
	parsed, err := url.Parse(authorization.URL("opaque-state"))
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	query := parsed.Query()
	if query.Get("nonce") != authorization.FlowState.Nonce || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("missing nonce or PKCE S256: %s", parsed.String())
	}
	if scope := query.Get("scope"); scope != "openid email" {
		t.Fatalf("unexpected Google OIDC scopes %q", scope)
	}
	if query.Get("code_challenge") != oauth2.S256ChallengeFromVerifier(authorization.FlowState.CodeVerifier) {
		t.Fatal("PKCE challenge does not match verifier")
	}
	expectedNonce = authorization.FlowState.Nonce
	tokenNonce = expectedNonce
	identity, err := client.Complete(t.Context(), cfg, authorization.FlowState, "code")
	if err != nil {
		t.Fatalf("complete Google authorization: %v", err)
	}
	if identity.Subject != "google-subject" || identity.Email != "ada@example.com" || !identity.EmailVerified {
		t.Fatalf("unexpected Google identity: %+v", identity)
	}
	if receivedVerifier != authorization.FlowState.CodeVerifier {
		t.Fatalf("token exchange did not send the original verifier")
	}

	emailVerified = false
	rejected, err := client.Begin(t.Context(), cfg)
	if err != nil {
		t.Fatalf("begin unverified-email authorization: %v", err)
	}
	tokenNonce = rejected.FlowState.Nonce
	if _, err := client.Complete(t.Context(), cfg, rejected.FlowState, "code"); !errors.Is(err, ErrEmailNotVerified) {
		t.Fatalf("expected unverified Google email rejection, got %v", err)
	}

	emailVerified = true
	rejected, err = client.Begin(t.Context(), cfg)
	if err != nil {
		t.Fatalf("begin nonce-mismatch authorization: %v", err)
	}
	tokenNonce = "wrong-nonce"
	if _, err := client.Complete(t.Context(), cfg, rejected.FlowState, "code"); !errors.Is(err, ErrTokenValidation) {
		t.Fatalf("expected nonce rejection, got %v", err)
	}

	rejected, err = client.Begin(t.Context(), cfg)
	if err != nil {
		t.Fatalf("begin audience-mismatch authorization: %v", err)
	}
	tokenNonce = rejected.FlowState.Nonce
	tokenAudience = "another-client"
	if _, err := client.Complete(t.Context(), cfg, rejected.FlowState, "code"); !errors.Is(err, ErrTokenValidation) {
		t.Fatalf("expected audience rejection, got %v", err)
	}
	tokenAudience = "hitkeep-social"

	rejected, err = client.Begin(t.Context(), cfg)
	if err != nil {
		t.Fatalf("begin issuer-mismatch authorization: %v", err)
	}
	tokenNonce = rejected.FlowState.Nonce
	tokenIssuer = "https://issuer.example.invalid"
	if _, err := client.Complete(t.Context(), cfg, rejected.FlowState, "code"); !errors.Is(err, ErrTokenValidation) {
		t.Fatalf("expected issuer rejection, got %v", err)
	}
}

func TestGoogleIssuerAllowsOnlyOfficialDocumentedVariants(t *testing.T) {
	for _, issuer := range []string{"https://accounts.google.com", "accounts.google.com"} {
		if !googleIssuerMatches(issuer, googleIssuer) {
			t.Fatalf("official Google issuer %q was rejected", issuer)
		}
	}
	for _, issuer := range []string{"https://accounts.google.com.evil.test", "http://accounts.google.com", ""} {
		if googleIssuerMatches(issuer, googleIssuer) {
			t.Fatalf("untrusted Google issuer %q was accepted", issuer)
		}
	}
	if googleIssuerMatches("accounts.google.com", "https://provider-double.test") {
		t.Fatal("non-production provider override accepted Google's alternate issuer")
	}
}

func TestGitHubUsesMinimalScopesPKCEAndPrimaryVerifiedEmail(t *testing.T) {
	receivedVerifier := ""
	emails := []map[string]any{
		{"email": "secondary@example.com", "primary": false, "verified": true},
		{"email": "Octo@Example.com", "primary": true, "verified": true},
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid", http.StatusBadRequest)
				return
			}
			receivedVerifier = r.Form.Get("code_verifier")
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, map[string]any{"access_token": "ephemeral-token", "token_type": "Bearer"})
		case "/user":
			if r.Header.Get("Authorization") != "Bearer ephemeral-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_ = json.MarshalWrite(w, map[string]any{"id": 42, "login": "octo", "name": "Octo Cat"})
		case "/user/emails":
			_ = json.MarshalWrite(w, emails)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	cfg := ProviderConfig{
		Provider: ProviderGitHub, ClientID: "github-client", ClientSecret: "github-secret",
		RedirectURL: "https://hitkeep.test/api/auth/social/github/callback",
		AuthURL:     server.URL + "/authorize", TokenURL: server.URL + "/token", APIBaseURL: server.URL,
	}
	client := &Client{httpClient: server.Client()}
	authorization, err := client.Begin(t.Context(), cfg)
	if err != nil {
		t.Fatalf("begin GitHub authorization: %v", err)
	}
	parsed, _ := url.Parse(authorization.URL("state"))
	if scope := parsed.Query().Get("scope"); scope != "read:user user:email" || strings.Contains(scope, "repo") {
		t.Fatalf("unexpected GitHub scopes %q", scope)
	}
	if parsed.Query().Get("code_challenge_method") != "S256" || parsed.Query().Get("nonce") != "" {
		t.Fatalf("unexpected GitHub authorization parameters: %s", parsed.String())
	}
	identity, err := client.Complete(t.Context(), cfg, authorization.FlowState, "code")
	if err != nil {
		t.Fatalf("complete GitHub authorization: %v", err)
	}
	if identity.Subject != "42" || identity.Email != "octo@example.com" || !identity.EmailVerified {
		t.Fatalf("unexpected GitHub identity: %+v", identity)
	}
	if receivedVerifier != authorization.FlowState.CodeVerifier {
		t.Fatal("GitHub token exchange did not send PKCE verifier")
	}

	emails = []map[string]any{{"email": "unverified@example.com", "primary": true, "verified": false}}
	missingEmailFlow, err := client.Begin(t.Context(), cfg)
	if err != nil {
		t.Fatalf("begin GitHub missing-email flow: %v", err)
	}
	if _, err := client.Complete(t.Context(), cfg, missingEmailFlow.FlowState, "code"); !errors.Is(err, ErrEmailNotVerified) {
		t.Fatalf("expected missing primary verified GitHub email rejection, got %v", err)
	}
}

func TestMicrosoftIdentityUsesTenantAndObjectIDsAndTreatsEmailAsMetadata(t *testing.T) {
	tenantID := "11111111-2222-3333-4444-555555555555"
	objectID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	idToken := &oidc.IDToken{Issuer: microsoftIssuerBase + "/" + tenantID + "/v2.0", Subject: "mutable-subject"}
	identity, err := microsoftIdentity(idToken, map[string]any{
		"tid": tenantID, "oid": strings.ToUpper(objectID), "preferred_username": "User@Example.com", "name": "Microsoft User",
	}, ProviderConfig{MicrosoftTenant: "common"})
	if err != nil {
		t.Fatalf("validate Microsoft identity: %v", err)
	}
	if identity.Subject != tenantID+":"+objectID || identity.Email != "user@example.com" || identity.EmailVerified {
		t.Fatalf("unexpected Microsoft identity: %+v", identity)
	}

	idToken.Issuer = microsoftIssuerBase + "/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee/v2.0"
	if _, err := microsoftIdentity(idToken, map[string]any{"tid": tenantID, "oid": objectID}, ProviderConfig{MicrosoftTenant: "common"}); !errors.Is(err, ErrTokenValidation) {
		t.Fatalf("expected tenant issuer mismatch rejection, got %v", err)
	}
	personal := &oidc.IDToken{Issuer: microsoftIssuerBase + "/" + microsoftConsumerID + "/v2.0"}
	personalIdentity, err := microsoftIdentity(personal, map[string]any{"tid": microsoftConsumerID, "oid": objectID, "email": "personal@example.com"}, ProviderConfig{MicrosoftTenant: "common"})
	if err != nil || personalIdentity.Subject != microsoftConsumerID+":"+objectID || personalIdentity.EmailVerified {
		t.Fatalf("expected valid personal Microsoft account metadata, identity=%+v err=%v", personalIdentity, err)
	}
	if _, err := microsoftIdentity(personal, map[string]any{"tid": microsoftConsumerID, "oid": objectID}, ProviderConfig{MicrosoftTenant: "organizations"}); !errors.Is(err, ErrTokenValidation) {
		t.Fatalf("expected personal account rejection for organizations tenant, got %v", err)
	}
}

func TestMicrosoftOIDCValidatesPKCENonceAudienceTenantAndSigningKeyIssuer(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	const (
		clientID = "microsoft-client"
		keyID    = "microsoft-key"
		objectID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
		workID   = "11111111-2222-3333-4444-555555555555"
	)
	tokenTenant := workID
	tokenNonce := ""
	receivedVerifier := ""
	keyIssuer := microsoftIssuerBase + "/{tenantid}/v2.0"
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, map[string]any{
				"issuer": microsoftIssuerBase + "/{tenantid}/v2.0", "authorization_endpoint": server.URL + "/authorize",
				"token_endpoint": server.URL + "/token", "jwks_uri": server.URL + "/keys",
				"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"},
				"id_token_signing_alg_values_supported": []string{oidc.RS256},
			})

		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid token request", http.StatusBadRequest)
				return
			}
			receivedVerifier = r.Form.Get("code_verifier")
			claims, _ := json.Marshal(map[string]any{
				"iss": microsoftIssuerBase + "/" + tokenTenant + "/v2.0", "sub": "tenant-subject", "aud": clientID,
				"exp": time.Now().Add(5 * time.Minute).Unix(), "iat": time.Now().Unix(), "nonce": tokenNonce,
				"tid": tokenTenant, "oid": objectID, "preferred_username": "Mutable@Example.com",
			})
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, map[string]any{
				"access_token": "discard-me", "token_type": "Bearer",
				"id_token": oidctest.SignIDToken(privateKey, keyID, oidc.RS256, string(claims)),
			})

		case "/keys":
			payload, _ := json.Marshal(jose.JSONWebKey{
				Key: &privateKey.PublicKey, KeyID: keyID, Algorithm: oidc.RS256, Use: "sig",
			})
			var key map[string]any
			_ = json.Unmarshal(payload, &key)
			key["issuer"] = keyIssuer
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, map[string]any{"keys": []any{key}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(sso.NewClient(server.Client()))
	cfg := ProviderConfig{
		Provider: ProviderMicrosoft, ClientID: clientID, ClientSecret: "secret",
		RedirectURL: "https://hitkeep.test/api/auth/social/microsoft/callback",
		IssuerURL:   server.URL, MicrosoftTenant: "common",
	}
	complete := func() (Identity, error) {
		authorization, err := client.Begin(t.Context(), cfg)
		if err != nil {
			t.Fatalf("begin Microsoft authorization: %v", err)
		}
		parsed, _ := url.Parse(authorization.URL("state"))
		if parsed.Query().Get("scope") != "openid email profile" || parsed.Query().Get("code_challenge_method") != "S256" || parsed.Query().Get("nonce") == "" {
			t.Fatalf("unexpected Microsoft authorization parameters: %s", parsed.String())
		}
		tokenNonce = authorization.FlowState.Nonce
		identity, completeErr := client.Complete(t.Context(), cfg, authorization.FlowState, "code")
		if completeErr == nil && receivedVerifier != authorization.FlowState.CodeVerifier {
			t.Fatal("Microsoft token exchange did not send PKCE verifier")
		}
		return identity, completeErr
	}

	identity, err := complete()
	if err != nil || identity.Subject != workID+":"+objectID || identity.Email != "mutable@example.com" || identity.EmailVerified {
		t.Fatalf("unexpected work-account identity=%+v err=%v", identity, err)
	}
	tokenTenant = microsoftConsumerID
	identity, err = complete()
	if err != nil || identity.Subject != microsoftConsumerID+":"+objectID {
		t.Fatalf("unexpected personal-account identity=%+v err=%v", identity, err)
	}

	tokenTenant = workID
	keyIssuer = microsoftIssuerBase + "/99999999-8888-7777-6666-555555555555/v2.0"
	if _, err := complete(); !errors.Is(err, ErrTokenValidation) {
		t.Fatalf("expected Microsoft signing-key issuer rejection, got %v", err)
	}
}
