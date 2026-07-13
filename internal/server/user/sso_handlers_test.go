package user

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hitkeep/internal/api"
	"hitkeep/internal/sso"
)

func TestTeamSSOConfigurationIsValidatedEncryptedAndRedacted(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}

	issuer := newOIDCDiscoveryServer(t)
	h.ctx.SSO = sso.NewClient(issuer.Client())
	body, _ := json.Marshal(map[string]any{
		"provider_type":      "oidc",
		"issuer_url":         issuer.URL,
		"client_id":          "hitkeep-dashboard",
		"client_secret":      "super-secret-value",
		"allowed_domains":    []string{"Example.COM", "example.org"},
		"email_claim":        "user.email",
		"display_name_claim": "user.name",
		"enabled":            true,
	})
	req := withTestUser(httptest.NewRequest(http.MethodPut, "/api/user/teams/"+teamID.String()+"/sso", bytes.NewReader(body)), userID)
	req.SetPathValue("id", teamID.String())
	w := httptest.NewRecorder()

	h.handleUpsertTeamSSO().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "super-secret-value") || strings.Contains(w.Body.String(), "client_secret\"") {
		t.Fatalf("response exposed the client secret: %s", w.Body.String())
	}
	var response api.TeamSSOConfig
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Enabled || !response.ClientSecretConfigured || response.CallbackURL != "http://localhost:8080/api/auth/sso/callback" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if len(response.AllowedDomains) != 2 || response.AllowedDomains[0] != "example.com" {
		t.Fatalf("domains were not normalized: %+v", response.AllowedDomains)
	}

	stored, err := store.GetTeamSSOConfig(context.Background(), teamID)
	if err != nil || stored == nil {
		t.Fatalf("load stored config: config=%+v err=%v", stored, err)
	}
	if stored.ClientSecretEncrypted == "super-secret-value" || strings.Contains(stored.ClientSecretEncrypted, "super-secret-value") {
		t.Fatal("database stored a plaintext client secret")
	}
	box, _ := sso.NewSecretBox(h.ctx.Config.JWTSecret)
	opened, err := box.Open(stored.ClientSecretEncrypted)
	if err != nil || opened != "super-secret-value" {
		t.Fatalf("stored secret did not decrypt correctly: value=%q err=%v", opened, err)
	}
}

func TestTeamSSOConfigurationRejectsUnknownIssuer(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	teamID, _ := store.GetActiveTenantID(context.Background(), userID)

	issuer := httptest.NewTLSServer(http.NotFoundHandler())
	defer issuer.Close()
	h.ctx.SSO = sso.NewClient(issuer.Client())
	body, _ := json.Marshal(map[string]any{
		"issuer_url":      issuer.URL,
		"client_id":       "hitkeep-dashboard",
		"client_secret":   "super-secret-value",
		"allowed_domains": []string{"example.com"},
		"enabled":         true,
	})
	req := withTestUser(httptest.NewRequest(http.MethodPut, "/api/user/teams/"+teamID.String()+"/sso", bytes.NewReader(body)), userID)
	req.SetPathValue("id", teamID.String())
	w := httptest.NewRecorder()

	h.handleUpsertTeamSSO().ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadGateway, w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "super-secret-value") {
		t.Fatal("provider validation error exposed the client secret")
	}
}

func newOIDCDiscoveryServer(t *testing.T) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                server.URL,
			"authorization_endpoint":                server.URL + "/authorize",
			"token_endpoint":                        server.URL + "/token",
			"jwks_uri":                              server.URL + "/keys",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	t.Cleanup(server.Close)
	return server
}
