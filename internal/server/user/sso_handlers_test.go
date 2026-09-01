package user

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/sso"
	json "hitkeep/jsonapi"
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
	issuerURL := issuer.URL + "/"
	body, _ := json.Marshal(map[string]any{
		"provider_type":      "oidc",
		"issuer_url":         issuerURL,
		"client_id":          "hitkeep-dashboard",
		"client_secret":      "super-secret-value",
		"allowed_domains":    []string{"Example.COM", "example.org"},
		"email_claim":        "user.email",
		"display_name_claim": "user.name",
		"auto_provision":     true,
		"enabled":            true,
	})
	req := withTestUser(httptest.NewRequest(http.MethodPut, "/api/user/teams/"+teamID.String()+"/sso", bytes.NewReader(body)), userID)
	req.SetPathValue("id", teamID.String())
	req.Header.Set("X-Request-Id", "sso-config-request")
	w := httptest.NewRecorder()

	h.handleUpsertTeamSSO().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "super-secret-value") || strings.Contains(w.Body.String(), "client_secret\"") {
		t.Fatalf("response exposed the client secret: %s", w.Body.String())
	}
	var response api.TeamSSOConfig
	if err := json.UnmarshalRead(w.Body, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Enabled || !response.AutoProvision || !response.ClientSecretConfigured || response.CallbackURL != "http://localhost:8080/api/auth/sso/callback" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.IssuerURL != issuerURL {
		t.Fatalf("issuer identifier changed in response: got %q want %q", response.IssuerURL, issuerURL)
	}
	var metadataJSON, requestID string
	if err := store.DB().QueryRowContext(context.Background(), `
		SELECT metadata_json, request_id
		FROM instance_audit_log
		WHERE action = 'sso.configuration_updated'
		ORDER BY created_at DESC
		LIMIT 1
	`).Scan(&metadataJSON, &requestID); err != nil {
		t.Fatalf("load SSO configuration audit metadata: %v", err)
	}
	var metadata map[string]string
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		t.Fatalf("decode SSO configuration audit metadata: %v", err)
	}
	if metadata["flow"] != "configuration" || metadata["reason"] != "configuration_updated" || metadata["team_id"] != teamID.String() || requestID != "sso-config-request" {
		t.Fatalf("unexpected SSO configuration audit metadata: metadata=%+v request_id=%q", metadata, requestID)
	}
	if strings.Contains(metadataJSON, "super-secret-value") {
		t.Fatalf("SSO configuration audit metadata exposed secret: %s", metadataJSON)
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
	if stored.IssuerURL != issuerURL {
		t.Fatalf("issuer identifier changed in storage: got %q want %q", stored.IssuerURL, issuerURL)
	}
	if !stored.AutoProvision {
		t.Fatal("automatic provisioning setting was not stored")
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

func TestTeamSSOSecretDecryptionFailureDoesNotLogRawError(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	if err := store.UpsertTeamSSOConfig(context.Background(), database.TeamSSOConfig{
		TeamID:                teamID,
		ProviderType:          "oidc",
		IssuerURL:             "https://issuer.example.test/",
		ClientID:              "hitkeep-dashboard",
		ClientSecretEncrypted: "not-a-valid-ciphertext",
		AllowedDomains:        []string{"example.com"},
		Enabled:               true,
	}); err != nil {
		t.Fatalf("store invalid SSO configuration: %v", err)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+teamID.String()+"/sso/test", nil), userID)
	req.SetPathValue("id", teamID.String())
	req = req.WithContext(shared.WithLogger(req.Context(), logger))
	w := httptest.NewRecorder()

	h.handleTestTeamSSO().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d: %s", http.StatusInternalServerError, w.Code, w.Body.String())
	}
	if strings.Contains(logs.String(), "error=") || !strings.Contains(logs.String(), "error_kind=secret_decryption_failed") {
		t.Fatalf("expected stable secret decryption diagnostics without raw error, got %q", logs.String())
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
		_ = json.MarshalWrite(w, map[string]any{
			"issuer":                                server.URL + "/",
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
