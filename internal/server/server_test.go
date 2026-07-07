package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"hitkeep/internal/api"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
)

func TestServerMountsMCPRouteWhenEnabled(t *testing.T) {
	conf := testServerConfig(t)
	conf.MCPEnabled = true
	store := testServerStore(t)
	defer store.Close()

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Host = "localhost:8080"
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected MCP route to require bearer auth with 401, got %d", rec.Code)
	}
}

func TestServerNormalizesRootMCPPath(t *testing.T) {
	conf := testServerConfig(t)
	conf.MCPEnabled = true
	conf.MCPPath = "/"
	store := testServerStore(t)
	defer store.Close()

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Host = "localhost:8080"
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected normalized MCP route to require bearer auth with 401, got %d", rec.Code)
	}
	if conf.MCPPath != "/mcp" {
		t.Fatalf("expected server to normalize root MCPPath to /mcp, got %q", conf.MCPPath)
	}
}

func TestServerDoesNotMountMCPRouteWhenDisabled(t *testing.T) {
	conf := testServerConfig(t)
	store := testServerStore(t)
	defer store.Close()

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hitkeep test shell") {
		t.Fatalf("expected disabled MCP path to fall through to SPA, got %d %q", rec.Code, rec.Body.String())
	}
}

func TestNormalizePublicBasePath(t *testing.T) {
	tests := []struct {
		name      string
		publicURL string
		want      string
	}{
		{name: "empty", publicURL: "", want: "/"},
		{name: "root", publicURL: "https://analytics.example.com", want: "/"},
		{name: "root slash", publicURL: "https://analytics.example.com/", want: "/"},
		{name: "relative value", publicURL: "hitkeep", want: "/"},
		{name: "single segment", publicURL: "https://www.example.net/hitkeep", want: "/hitkeep/"},
		{name: "single segment slash", publicURL: "https://www.example.net/hitkeep/", want: "/hitkeep/"},
		{name: "cleans repeated slashes and dots", publicURL: "https://www.example.net//tools/./hitkeep//", want: "/tools/hitkeep/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePublicBasePath(tt.publicURL); got != tt.want {
				t.Fatalf("normalizePublicBasePath(%q) = %q, want %q", tt.publicURL, got, tt.want)
			}
		})
	}
}

func TestServerInjectsPublicBasePathIntoDashboardIndex(t *testing.T) {
	conf := testServerConfig(t)
	conf.PublicURL = "https://www.example.net/hitkeep/"
	store := testServerStore(t)
	defer store.Close()

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	req := httptest.NewRequest(http.MethodGet, "/hitkeep/dashboard", nil)
	req.Host = "www.example.net"
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected dashboard shell, got status %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `<base href="/hitkeep/" />`) {
		t.Fatalf("expected injected subdirectory base href, got %q", body)
	}
}

func TestServerPreservesRootBasePathInDashboardIndex(t *testing.T) {
	conf := testServerConfig(t)
	store := testServerStore(t)
	defer store.Close()

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected dashboard shell, got status %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `<base href="/" />`) {
		t.Fatalf("expected root base href, got %q", body)
	}
}

func TestServerRoutesPrefixedAPIRequests(t *testing.T) {
	conf := testServerConfig(t)
	conf.PublicURL = "https://www.example.net/hitkeep/"
	store := testServerStore(t)
	defer store.Close()

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	for _, path := range []string{"/hitkeep/api/status"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Host = "www.example.net"
			rec := httptest.NewRecorder()

			srv.httpServer.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status endpoint, got status %d body %q", rec.Code, rec.Body.String())
			}
			if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
				t.Fatalf("expected JSON status response, got content type %q body %q", contentType, rec.Body.String())
			}
		})
	}
}

func TestServerRejectsUnprefixedAPIRequestsWhenPublicURLHasPath(t *testing.T) {
	conf := testServerConfig(t)
	conf.PublicURL = "https://www.example.net/hitkeep/"
	store := testServerStore(t)
	defer store.Close()

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Host = "www.example.net"
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected unprefixed API request to be rejected, got status %d body %q", rec.Code, rec.Body.String())
	}
}

func TestServerPreservesRootHealthEndpointsForLocalChecksWhenPublicURLHasPath(t *testing.T) {
	conf := testServerConfig(t)
	conf.PublicURL = "https://www.example.net/hitkeep/"
	store := testServerStore(t)
	defer store.Close()

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			srv.httpServer.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected root health endpoint to remain available, got status %d body %q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestServerServesPrefixedStaticAssets(t *testing.T) {
	conf := testServerConfig(t)
	conf.PublicURL = "https://www.example.net/hitkeep/"
	store := testServerStore(t)
	defer store.Close()

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	req := httptest.NewRequest(http.MethodGet, "/hitkeep/main.abc123.js", nil)
	req.Host = "www.example.net"
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected prefixed asset, got status %d body %q", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body != "console.log('hitkeep');" {
		t.Fatalf("expected static asset body, got %q", body)
	}
	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != cacheControlImmutable {
		t.Fatalf("expected immutable cache control, got %q", cacheControl)
	}
}

func TestServerRoutesPrefixedIngestPreflight(t *testing.T) {
	conf := testServerConfig(t)
	conf.PublicURL = "https://www.example.net/hitkeep/"
	store := testServerStore(t)
	defer store.Close()

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	req := httptest.NewRequest(http.MethodOptions, "/hitkeep/ingest", nil)
	req.Host = "www.example.net"
	req.Header.Set("Origin", "https://app.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected prefixed ingest preflight, got status %d body %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("expected echoed origin, got %q", got)
	}
}

func TestServerAppliesFetchMetadataAfterPrefixStripping(t *testing.T) {
	conf := testServerConfig(t)
	conf.PublicURL = "https://www.example.net/hitkeep/"
	store := testServerStore(t)
	defer store.Close()

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	req := httptest.NewRequest(http.MethodPost, "/hitkeep/api/login", nil)
	req.Host = "www.example.net"
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected prefixed cross-site API request to be blocked with %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestServerBackupStatusReflectsConfig(t *testing.T) {
	conf := testServerConfig(t)
	conf.BackupPath = "s3://hitkeep-backups/backup"
	conf.BackupIntervalMinutes = 60
	conf.BackupRetentionCount = 24
	store := testServerStore(t)
	defer store.Close()

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	tracker := srv.BackupStatus()
	if tracker == nil {
		t.Fatal("expected backup status tracker")
	}
	status := tracker.Status()
	if !status.Enabled {
		t.Fatal("expected backup status to be enabled")
	}
	if status.ConfigPath != conf.BackupPath {
		t.Fatalf("expected backup path %q, got %q", conf.BackupPath, status.ConfigPath)
	}
	if status.IntervalMin != 60 {
		t.Fatalf("expected interval 60, got %d", status.IntervalMin)
	}
	if status.Retention != 24 {
		t.Fatalf("expected retention 24, got %d", status.Retention)
	}
}

func TestServerCustomTrackingHostServesOnlyTrackerRoutes(t *testing.T) {
	conf := testServerConfig(t)
	conf.PublicURL = "https://app.example.net/hitkeep/"
	store := testServerStore(t)
	defer store.Close()
	createRouteableCustomTrackingDomain(t, store, "track.example.net")

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "tracker asset", method: http.MethodGet, path: "/hk.js", wantStatus: http.StatusOK, wantBody: "tracker asset"},
		{name: "vitals asset", method: http.MethodHead, path: "/hk-vitals.js", wantStatus: http.StatusOK},
		{name: "ingest preflight", method: http.MethodOptions, path: "/ingest", wantStatus: http.StatusNoContent},
		{name: "api denied", method: http.MethodGet, path: "/api/status", wantStatus: http.StatusNotFound},
		{name: "dashboard denied", method: http.MethodGet, path: "/", wantStatus: http.StatusNotFound},
		{name: "prefixed asset denied", method: http.MethodGet, path: "/hitkeep/hk.js", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Host = "track.example.net"
			if tt.method == http.MethodOptions {
				req.Header.Set("Origin", "https://site.example")
				req.Header.Set("Access-Control-Request-Method", http.MethodPost)
				req.Header.Set("Access-Control-Request-Headers", "content-type")
			}
			rec := httptest.NewRecorder()

			srv.httpServer.Handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d body %q", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Fatalf("expected body %q, got %q", tt.wantBody, rec.Body.String())
			}
		})
	}
}

func TestServerCaddyOnDemandTLSAsk(t *testing.T) {
	conf := testServerConfig(t)
	conf.CaddyTLSAskToken = "ask-token"
	store := testServerStore(t)
	defer store.Close()
	allowed := createRouteableCustomTrackingDomain(t, store, "allowed.example.net")
	disabled := createRouteableCustomTrackingDomain(t, store, "disabled.example.net")
	if _, err := store.UpdateCustomTrackingDomainEnabled(context.Background(), disabled.TeamID, disabled.ID, false); err != nil {
		t.Fatalf("disable custom tracking domain: %v", err)
	}

	srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "allowed", path: "/internal/caddy/on-demand-tls/ask-token?domain=allowed.example.net", wantStatus: http.StatusNoContent},
		{name: "disabled", path: "/internal/caddy/on-demand-tls/ask-token?domain=disabled.example.net", wantStatus: http.StatusForbidden},
		{name: "unknown", path: "/internal/caddy/on-demand-tls/ask-token?domain=unknown.example.net", wantStatus: http.StatusForbidden},
		{name: "invalid token", path: "/internal/caddy/on-demand-tls/wrong-token?domain=allowed.example.net", wantStatus: http.StatusNotFound},
		{name: "malformed domain", path: "/internal/caddy/on-demand-tls/ask-token?domain=localhost", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Host = "app.example.net"
			rec := httptest.NewRecorder()

			srv.httpServer.Handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d body %q", tt.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}

	refreshed, err := store.GetCustomTrackingDomain(context.Background(), allowed.ID)
	if err != nil {
		t.Fatalf("reload allowed custom tracking domain: %v", err)
	}
	if refreshed == nil || refreshed.LastTLSAskAt == nil {
		t.Fatalf("expected successful ask to record last_tls_ask_at, got %+v", refreshed)
	}
}

func testServerConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		ApiBurst:         1000,
		ApiRateLimit:     1000,
		AuthBurst:        1000,
		AuthRateLimit:    1000,
		DataPath:         t.TempDir(),
		HTTPAddr:         "127.0.0.1:0",
		IngestBurst:      1000,
		IngestRateLimit:  1000,
		MCPPath:          "/mcp",
		PublicURL:        "http://localhost:8080",
		SpamFilterPath:   filepath.Join(t.TempDir(), "spam-filter.json"),
		Version:          "test",
		WebhookBurst:     1000,
		WebhookRateLimit: 1000,
	}
}

func testServerStore(t *testing.T) *database.Store {
	t.Helper()
	store := database.NewStore(filepath.Join(t.TempDir(), "hitkeep.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		store.Close()
		t.Fatalf("Migrate: %v", err)
	}
	return store
}

func testPublicFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": {
			Data: []byte(`<html><head><base href="/" /></head><body>hitkeep test shell</body></html>`),
		},
		"main.abc123.js": {
			Data: []byte("console.log('hitkeep');"),
		},
		"hk.js": {
			Data: []byte("tracker asset"),
		},
		"hk-vitals.js": {
			Data: []byte("vitals tracker asset"),
		},
	}
}

func createRouteableCustomTrackingDomain(t *testing.T, store *database.Store, hostname string) *api.CustomTrackingDomain {
	t.Helper()

	ctx := context.Background()
	teamID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	domain, err := store.CreateCustomTrackingDomain(ctx, database.CustomTrackingDomainInput{
		TeamID: teamID,
		Host:   hostname,
	})
	if err != nil {
		t.Fatalf("create custom tracking domain %q: %v", hostname, err)
	}
	now := time.Now().UTC()
	verified, err := store.UpdateCustomTrackingDomainVerification(ctx, domain.ID, database.CustomTrackingDomainVerificationResult{
		VerificationStatus: database.CustomTrackingVerificationVerified,
		TargetStatus:       database.CustomTrackingVerificationVerified,
		TLSStatus:          database.CustomTrackingVerificationPending,
		VerifiedAt:         &now,
		LastCheckedAt:      now,
	})
	if err != nil {
		t.Fatalf("verify custom tracking domain %q: %v", hostname, err)
	}
	if verified == nil {
		t.Fatalf("expected custom tracking domain %q to exist after verification", hostname)
	}
	return verified
}
