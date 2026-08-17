package sites

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/exportfmt"
	"hitkeep/internal/server/shared"
)

// setupTestEnv initializes an in-memory database and a handler instance.
func setupTestEnv(t *testing.T) (*handler, *database.Store, uuid.UUID) {
	t.Helper()

	// Use in-memory DuckDB
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	// Create a dummy user
	userID, err := store.CreateUser(context.Background(), "test@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	tenantStores := database.NewTenantStoreManager(store, t.TempDir(), database.WithTenantDataPlane(false))
	t.Cleanup(func() { _ = tenantStores.Close() })

	ctx := &shared.Context{
		Store:        store,
		TenantStores: tenantStores,
		Config:       &config.Config{},
	}

	return &handler{ctx: ctx}, store, userID
}

func TestNormalizeSiteDomain(t *testing.T) {
	tooLongLabel := strings.Repeat("a", 64) + ".example"
	tooLongDomain := strings.Repeat("a.", 126) + "a.com"
	tests := []struct {
		name       string
		raw        string
		wantDomain string
		wantError  bool
	}{
		{name: "apex domain", raw: "example.com", wantDomain: "example.com"},
		{name: "hyphenated multi-level domain", raw: "sub.example-app.com.br", wantDomain: "sub.example-app.com.br"},
		{name: "staged service domain", raw: "app-staging.example-service.co.uk", wantDomain: "app-staging.example-service.co.uk"},
		{name: "single-character label", raw: "a.example.com", wantDomain: "a.example.com"},
		{name: "digits and interior hyphens", raw: "api2.service-3.example.com", wantDomain: "api2.service-3.example.com"},
		{name: "punycode final label", raw: "example.xn--p1ai", wantDomain: "example.xn--p1ai"},
		{name: "uppercase and whitespace", raw: "  EXAMPLE.COM  ", wantDomain: "example.com"},
		{name: "www2 is not www", raw: "www2.example.com", wantDomain: "www2.example.com"},
		{name: "www subdomain is not first label", raw: "blog.www.example.com", wantDomain: "blog.www.example.com"},
		{name: "empty", raw: "", wantError: true},
		{name: "www prefix", raw: "www.example.com", wantError: true},
		{name: "uppercase www prefix", raw: "WWW.example.com", wantError: true},
		{name: "http protocol", raw: "http://example.com", wantError: true},
		{name: "https protocol", raw: "https://example.com", wantError: true},
		{name: "path", raw: "example.com/path", wantError: true},
		{name: "port", raw: "example.com:443", wantError: true},
		{name: "query", raw: "example.com?query=1", wantError: true},
		{name: "fragment", raw: "example.com#section", wantError: true},
		{name: "wildcard", raw: "*.example.com", wantError: true},
		{name: "ipv4", raw: "192.0.2.1", wantError: true},
		{name: "ipv6", raw: "2001:db8::1", wantError: true},
		{name: "localhost", raw: "localhost", wantError: true},
		{name: "unicode", raw: "münich.de", wantError: true},
		{name: "leading dot", raw: ".example.com", wantError: true},
		{name: "trailing dot", raw: "example.com.", wantError: true},
		{name: "empty label", raw: "example..com", wantError: true},
		{name: "leading hyphen", raw: "-example.com", wantError: true},
		{name: "trailing hyphen", raw: "example-.com", wantError: true},
		{name: "invalid character", raw: "inva lid.com", wantError: true},
		{name: "label too long", raw: tooLongLabel, wantError: true},
		{name: "hostname too long", raw: tooLongDomain, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDomain, gotError := normalizeSiteDomain(tt.raw)
			if tt.wantError {
				if gotDomain != "" || gotError == "" {
					t.Fatalf("expected validation error, got domain=%q error=%q", gotDomain, gotError)
				}
				return
			}
			if gotError != "" || gotDomain != tt.wantDomain {
				t.Fatalf("expected domain=%q without error, got domain=%q error=%q", tt.wantDomain, gotDomain, gotError)
			}
		})
	}
}

func setupFileBackedTransferEnv(t *testing.T) (*handler, *database.Store, *database.TenantStoreManager, uuid.UUID) {
	t.Helper()

	tmpDir := t.TempDir()
	store := database.NewStore(filepath.Join(tmpDir, "hitkeep.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("failed to connect to file-backed test db: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate file-backed test db: %v", err)
	}

	userID, err := store.CreateUser(context.Background(), "transfer@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	tenantStores := database.NewTenantStoreManager(store, filepath.Join(tmpDir, "tenant-data"))
	ctx := &shared.Context{
		Store:        store,
		TenantStores: tenantStores,
		Config:       &config.Config{},
	}

	return &handler{ctx: ctx}, store, tenantStores, userID
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestHandleGetFaviconUsesDuckDuckGoIconPath(t *testing.T) {
	h, store, _ := setupTestEnv(t)
	defer store.Close()

	originalTransport := faviconProxyTransport
	defer func() {
		faviconProxyTransport = originalTransport
	}()

	var capturedPath string
	var capturedHost string
	faviconProxyTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedPath = req.URL.Path
		capturedHost = req.URL.Host
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/favicon/example.com", nil)
	req.SetPathValue("domain", "example.com")
	w := httptest.NewRecorder()

	h.handleGetFavicon().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if capturedHost != "icons.duckduckgo.com" {
		t.Fatalf("expected upstream host %q, got %q", "icons.duckduckgo.com", capturedHost)
	}
	if capturedPath != "/ip3/example.com.ico" {
		t.Fatalf("expected upstream path %q, got %q", "/ip3/example.com.ico", capturedPath)
	}
}

func TestHandleGetFaviconGracefullyFallsBackWhenUpstreamHasNoIcon(t *testing.T) {
	h, store, _ := setupTestEnv(t)
	defer store.Close()

	originalTransport := faviconProxyTransport
	defer func() {
		faviconProxyTransport = originalTransport
	}()

	faviconProxyTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("not found")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/favicon/missing.example", nil)
	req.SetPathValue("domain", "missing.example")
	w := httptest.NewRecorder()

	h.handleGetFavicon().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("expected empty fallback response, got %q", w.Body.String())
	}
}

func TestFaviconProxyErrorKindUsesStableCategories(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{name: "canceled", err: context.Canceled, want: "canceled"},
		{name: "timeout", err: context.DeadlineExceeded, want: "timeout"},
		{name: "upstream", err: errors.New("upstream response included internal transport details"), want: "upstream_request_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := faviconProxyErrorKind(test.err); got != test.want {
				t.Fatalf("faviconProxyErrorKind() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestHandleCreateSite(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	// Pre-create a site to test conflict
	_, _ = store.CreateSite(context.Background(), userID, "taken.com")

	tests := []struct {
		name           string
		body           map[string]string
		injectAuth     bool
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "Unauthorized",
			body:           map[string]string{"domain": "new.com"},
			injectAuth:     false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Empty Body",
			body:           nil,
			injectAuth:     true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Empty Domain",
			body:           map[string]string{"domain": ""},
			injectAuth:     true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Protocol - HTTP",
			body:           map[string]string{"domain": "http://example.com"},
			injectAuth:     true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Protocol - HTTPS",
			body:           map[string]string{"domain": "https://example.com"},
			injectAuth:     true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Prefix - WWW",
			body:           map[string]string{"domain": "www.example.com"},
			injectAuth:     true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid Characters",
			body:           map[string]string{"domain": "inva lid.com"},
			injectAuth:     true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Duplicate Domain",
			body:           map[string]string{"domain": "taken.com"}, // Already exists
			injectAuth:     true,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "Success",
			body:           map[string]string{"domain": "example.com"},
			injectAuth:     true,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var site api.Site
				if err := json.NewDecoder(w.Body).Decode(&site); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if site.Domain != "example.com" {
					t.Errorf("expected domain example.com, got %s", site.Domain)
				}
				if site.ID == uuid.Nil {
					t.Error("expected valid UUID, got nil")
				}
			},
		},
		{
			name:           "Success - Case Insensitive",
			body:           map[string]string{"domain": "UPPERCASE.com"},
			injectAuth:     true,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var site api.Site
				if err := json.NewDecoder(w.Body).Decode(&site); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if site.Domain != "uppercase.com" {
					t.Errorf("expected normalized domain uppercase.com, got %s", site.Domain)
				}
			},
		},
		{
			name:           "Success",
			body:           map[string]string{"domain": "sub.example.com"},
			injectAuth:     true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Success",
			body:           map[string]string{"domain": "sub.sub.example.com"},
			injectAuth:     true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Success - Hyphenated Multi-Level Domain",
			body:           map[string]string{"domain": "sub.example-app.com.br"},
			injectAuth:     true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Success - Hyphenated Staging Domain",
			body:           map[string]string{"domain": "app-staging.example-service.co.uk"},
			injectAuth:     true,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var bodyBytes []byte
			if tc.body != nil {
				bodyBytes, _ = json.Marshal(tc.body)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/sites", bytes.NewReader(bodyBytes))

			if tc.injectAuth {
				ctx := context.WithValue(req.Context(), shared.UserIDKey, userID)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()
			handler := h.handleCreateSite()
			handler.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d. Body: %s", tc.expectedStatus, w.Code, w.Body.String())
			}

			if tc.checkResponse != nil {
				tc.checkResponse(t, w)
			}
		})
	}
}

func TestHandleGetSites(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	_, _ = store.CreateSite(context.Background(), userID, "site1.com")
	_, _ = store.CreateSite(context.Background(), userID, "site2.com")

	otherUserID := uuid.New()
	_, _ = store.CreateSite(context.Background(), otherUserID, "other.com")

	tests := []struct {
		name           string
		injectAuth     bool
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "Unauthorized",
			injectAuth:     false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Authorized - Returns User Sites",
			injectAuth:     true,
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
			if tc.injectAuth {
				ctx := context.WithValue(req.Context(), shared.UserIDKey, userID)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()
			handler := h.handleGetSites()
			handler.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, w.Code)
			}

			if tc.expectedStatus == http.StatusOK {
				var sites []api.Site
				if err := json.NewDecoder(w.Body).Decode(&sites); err != nil {
					t.Fatalf("failed to decode sites: %v", err)
				}
				if len(sites) != tc.expectedCount {
					t.Errorf("expected %d sites, got %d", tc.expectedCount, len(sites))
				}
			}
		})
	}
}

func TestHandleGetSitesOverviewStats(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()
	ctx := context.Background()

	siteAlpha, err := store.CreateSite(ctx, userID, "alpha-overview.test")
	if err != nil {
		t.Fatalf("create alpha site: %v", err)
	}
	siteBeta, err := store.CreateSite(ctx, userID, "beta-overview.test")
	if err != nil {
		t.Fatalf("create beta site: %v", err)
	}

	base := time.Date(2026, time.July, 1, 12, 0, 0, 0, time.UTC)
	sessionID := uuid.New()
	for _, path := range []string{"/", "/pricing"} {
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID:    siteAlpha.ID,
			SessionID: sessionID,
			PageID:    uuid.New(),
			Timestamp: base.Add(-1 * time.Hour),
			Path:      path,
		}); err != nil {
			t.Fatalf("create alpha hit: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/overview", nil)
	w := httptest.NewRecorder()
	h.handleGetSitesOverviewStats().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	statsURL := fmt.Sprintf(
		"/api/sites/overview?from=%s&to=%s",
		url.QueryEscape(base.Add(-24*time.Hour).Format(time.RFC3339)),
		url.QueryEscape(base.Format(time.RFC3339)),
	)
	req = httptest.NewRequest(http.MethodGet, statsURL, nil)
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
	w = httptest.NewRecorder()
	h.handleGetSitesOverviewStats().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var response api.SitesOverviewStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode overview response: %v", err)
	}
	if len(response.Sites) != 2 {
		t.Fatalf("expected two accessible site rows, got %+v", response.Sites)
	}
	alphaStats := findOverviewStats(response.Sites, siteAlpha.ID)
	if alphaStats == nil {
		t.Fatalf("expected alpha stats in response: %+v", response.Sites)
	}
	if alphaStats.Status != api.SiteOverviewStatsReady || alphaStats.TotalPageviews != 2 || alphaStats.UniqueSessions != 1 {
		t.Fatalf("unexpected alpha overview stats: %+v", alphaStats)
	}
	if findOverviewStats(response.Sites, siteBeta.ID) == nil {
		t.Fatalf("expected beta row with no traffic in response: %+v", response.Sites)
	}

	req = httptest.NewRequest(http.MethodGet, statsURL, nil)
	apiClientAuth := &database.APIClientAuth{
		UserID:    userID,
		SiteRoles: map[uuid.UUID]auth.SiteRole{siteBeta.ID: auth.SiteViewer},
	}
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
	req = req.WithContext(context.WithValue(req.Context(), shared.APIClientAuthKey, apiClientAuth))
	w = httptest.NewRecorder()
	h.handleGetSitesOverviewStats().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected API client status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	response = api.SitesOverviewStatsResponse{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode API client overview response: %v", err)
	}
	if len(response.Sites) != 1 || response.Sites[0].SiteID != siteBeta.ID {
		t.Fatalf("expected API client response to be scoped to beta, got %+v", response.Sites)
	}
}

func findOverviewStats(rows []api.SiteOverviewStats, siteID uuid.UUID) *api.SiteOverviewStats {
	for idx := range rows {
		if rows[idx].SiteID == siteID {
			return &rows[idx]
		}
	}
	return nil
}

func TestSiteExclusionsAllowInstanceAdmin(t *testing.T) {
	h, store, ownerID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(context.Background(), ownerID, "instance-admin-exclusions.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	adminID, err := store.CreateUser(context.Background(), "instance-admin-exclusions@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if err := store.UpdateInstanceRole(context.Background(), adminID, auth.InstanceAdmin, ownerID); err != nil {
		t.Fatalf("promote instance admin: %v", err)
	}

	body := bytes.NewReader([]byte(`{"cidr":"203.0.113.7","description":"office"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/exclusions", body)
	req.SetPathValue("id", site.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, adminID))
	w := httptest.NewRecorder()

	h.ctx.RequireSiteOrInstancePermission(auth.PermSiteManageData, auth.PermInstanceManageSiteExclusions)(h.handleCreateSiteExclusion()).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected instance admin create status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/exclusions", nil)
	listReq.SetPathValue("id", site.ID.String())
	listReq = listReq.WithContext(context.WithValue(listReq.Context(), shared.UserIDKey, adminID))
	listW := httptest.NewRecorder()

	h.ctx.RequireSiteOrInstancePermission(auth.PermSiteManageData, auth.PermInstanceManageSiteExclusions)(h.handleListSiteExclusions()).ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("expected instance admin list status %d, got %d: %s", http.StatusOK, listW.Code, listW.Body.String())
	}

	retentionReq := httptest.NewRequest(http.MethodPut, "/api/sites/"+site.ID.String()+"/retention", bytes.NewReader([]byte(`{"days":30}`)))
	retentionReq.SetPathValue("id", site.ID.String())
	retentionReq = retentionReq.WithContext(context.WithValue(retentionReq.Context(), shared.UserIDKey, adminID))
	retentionW := httptest.NewRecorder()

	h.ctx.RequirePermission(auth.PermSiteManageData)(h.handleUpdateSiteRetention()).ServeHTTP(retentionW, retentionReq)
	if retentionW.Code != http.StatusForbidden {
		t.Fatalf("expected instance admin retention status %d, got %d: %s", http.StatusForbidden, retentionW.Code, retentionW.Body.String())
	}
}

func TestSiteExclusionCountryRules(t *testing.T) {
	h, store, ownerID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(context.Background(), ownerID, "country-exclusions.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	body := bytes.NewReader([]byte(`{"type":"country","country_code":"de","description":"Germany"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/exclusions", body)
	req.SetPathValue("id", site.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, ownerID))
	w := httptest.NewRecorder()

	h.ctx.RequireSiteOrInstancePermission(auth.PermSiteManageData, auth.PermInstanceManageSiteExclusions)(h.handleCreateSiteExclusion()).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected country create status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}
	var created api.IPExclusion
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode created country exclusion: %v", err)
	}
	if created.Type != "country" || created.CountryCode != "DE" || created.CIDR != "" {
		t.Fatalf("unexpected created country exclusion: %+v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/exclusions", nil)
	listReq.SetPathValue("id", site.ID.String())
	listReq = listReq.WithContext(context.WithValue(listReq.Context(), shared.UserIDKey, ownerID))
	listW := httptest.NewRecorder()
	h.ctx.RequireSiteOrInstancePermission(auth.PermSiteManageData, auth.PermInstanceManageSiteExclusions)(h.handleListSiteExclusions()).ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d: %s", http.StatusOK, listW.Code, listW.Body.String())
	}
	var rules []api.IPExclusion
	if err := json.NewDecoder(listW.Body).Decode(&rules); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(rules) != 1 || rules[0].Type != "country" || rules[0].CountryCode != "DE" {
		t.Fatalf("unexpected listed rules: %+v", rules)
	}

	badReq := httptest.NewRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/exclusions", bytes.NewReader([]byte(`{"type":"country","country_code":"deu"}`)))
	badReq.SetPathValue("id", site.ID.String())
	badReq = badReq.WithContext(context.WithValue(badReq.Context(), shared.UserIDKey, ownerID))
	badW := httptest.NewRecorder()
	h.ctx.RequireSiteOrInstancePermission(auth.PermSiteManageData, auth.PermInstanceManageSiteExclusions)(h.handleCreateSiteExclusion()).ServeHTTP(badW, badReq)
	if badW.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid country status %d, got %d: %s", http.StatusBadRequest, badW.Code, badW.Body.String())
	}
}

func TestSiteExclusionsAllowTeamAdminAndRejectUnscopedMember(t *testing.T) {
	h, store, ownerID := setupTestEnv(t)
	defer store.Close()

	adminID, err := store.CreateUser(context.Background(), "team-admin-exclusions@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create team admin: %v", err)
	}
	memberID, err := store.CreateUser(context.Background(), "team-member-exclusions@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create team member: %v", err)
	}
	team, err := store.CreateTenant(context.Background(), ownerID, "Exclusion Team", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), team.ID, adminID, database.TenantRoleAdmin, ownerID); err != nil {
		t.Fatalf("add team admin: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), team.ID, memberID, database.TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add team member: %v", err)
	}
	if err := store.SetActiveTenantID(context.Background(), ownerID, team.ID); err != nil {
		t.Fatalf("set owner active team: %v", err)
	}
	site, err := store.CreateSite(context.Background(), ownerID, "team-admin-exclusions.test")
	if err != nil {
		t.Fatalf("create team site: %v", err)
	}
	if err := store.SetActiveTenantID(context.Background(), adminID, team.ID); err != nil {
		t.Fatalf("set admin active team: %v", err)
	}
	if err := store.SetActiveTenantID(context.Background(), memberID, team.ID); err != nil {
		t.Fatalf("set member active team: %v", err)
	}

	body := bytes.NewReader([]byte(`{"cidr":"198.51.100.0/24","description":"partner"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/exclusions", body)
	req.SetPathValue("id", site.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, adminID))
	w := httptest.NewRecorder()

	h.ctx.RequireSiteOrInstancePermission(auth.PermSiteManageData, auth.PermInstanceManageSiteExclusions)(h.handleCreateSiteExclusion()).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected team admin create status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/exclusions", nil)
	listReq.SetPathValue("id", site.ID.String())
	listReq = listReq.WithContext(context.WithValue(listReq.Context(), shared.UserIDKey, adminID))
	listW := httptest.NewRecorder()

	h.ctx.RequireSiteOrInstancePermission(auth.PermSiteManageData, auth.PermInstanceManageSiteExclusions)(h.handleListSiteExclusions()).ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("expected team admin list status %d, got %d: %s", http.StatusOK, listW.Code, listW.Body.String())
	}

	memberReq := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/exclusions", nil)
	memberReq.SetPathValue("id", site.ID.String())
	memberReq = memberReq.WithContext(context.WithValue(memberReq.Context(), shared.UserIDKey, memberID))
	memberW := httptest.NewRecorder()

	h.ctx.RequireSiteOrInstancePermission(auth.PermSiteManageData, auth.PermInstanceManageSiteExclusions)(h.handleListSiteExclusions()).ServeHTTP(memberW, memberReq)
	if memberW.Code != http.StatusForbidden {
		t.Fatalf("expected unscoped team member status %d, got %d: %s", http.StatusForbidden, memberW.Code, memberW.Body.String())
	}
}

func TestHandleTransferSiteTeam(t *testing.T) {
	h, store, tenantStores, userID := setupFileBackedTransferEnv(t)
	defer store.Close()
	defer tenantStores.Close()

	site, err := store.CreateSite(context.Background(), userID, "move-me.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	sourceTeamID, err := store.GetSiteTenantID(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("get source team: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSiteMapping(context.Background(), database.GoogleSearchConsoleSiteMappingInput{
		SiteID:      site.ID,
		TeamID:      sourceTeamID,
		PropertyURI: "sc-domain:move-me.test",
		MappedBy:    userID,
		MappedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed Search Console mapping: %v", err)
	}

	destinationTeam, err := store.CreateTenant(context.Background(), userID, "Destination", "")
	if err != nil {
		t.Fatalf("create destination team: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"team_id": destinationTeam.ID.String()})
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/transfer-team", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
	req.SetPathValue("id", site.ID.String())
	w := httptest.NewRecorder()

	h.handleTransferSiteTeam().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	tenantID, err := store.GetSiteTenantID(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("get site tenant after transfer: %v", err)
	}
	if tenantID != destinationTeam.ID {
		t.Fatalf("expected destination team %s, got %s", destinationTeam.ID, tenantID)
	}
	entries, total, err := store.ListTeamAuditEntries(context.Background(), sourceTeamID, "google_search_console.property_unmapped", 5, 0)
	if err != nil {
		t.Fatalf("list Search Console transfer audit: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one Search Console unmap audit on transfer, got total=%d entries=%+v", total, entries)
	}
	if entries[0].TargetID != site.ID.String() || !strings.Contains(entries[0].Details, "old_property_uri=sc-domain:move-me.test") || !strings.Contains(entries[0].Details, "reason=site_transfer") {
		t.Fatalf("unexpected Search Console transfer audit: %+v", entries[0])
	}
}

func TestHandleTransferSiteTeamLogsMappingFailureAtHTTPBoundary(t *testing.T) {
	h, store, tenantStores, userID := setupFileBackedTransferEnv(t)
	defer store.Close()
	defer tenantStores.Close()

	site, err := store.CreateSite(context.Background(), userID, "mapping-failure.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	sourceTeamID, err := store.GetSiteTenantID(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("get source team: %v", err)
	}
	destinationTeam, err := store.CreateTenant(context.Background(), userID, "Destination", "")
	if err != nil {
		t.Fatalf("create destination team: %v", err)
	}
	if err := store.Exec(context.Background(), "DROP TABLE google_search_console_site_mappings"); err != nil {
		t.Fatalf("drop Search Console mapping table: %v", err)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	body, _ := json.Marshal(map[string]string{"team_id": destinationTeam.ID.String()})
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/transfer-team", bytes.NewReader(body))
	req.SetPathValue("id", site.ID.String())
	ctx := context.WithValue(req.Context(), shared.UserIDKey, userID)
	req = req.WithContext(shared.WithLogger(ctx, logger))
	w := httptest.NewRecorder()

	h.handleTransferSiteTeam().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d: %s", http.StatusInternalServerError, w.Code, w.Body.String())
	}
	if count := strings.Count(logs.String(), "msg=\"Failed to load Search Console mapping before site transfer\""); count != 1 {
		t.Fatalf("expected one boundary log entry, got %d: %s", count, logs.String())
	}
	if !strings.Contains(logs.String(), "site_id="+site.ID.String()) || !strings.Contains(logs.String(), "source_team_id="+sourceTeamID.String()) {
		t.Fatalf("expected transfer identifiers in boundary log: %s", logs.String())
	}
}

func TestHandleResetSiteStatsValidatesRequest(t *testing.T) {
	ctx := context.Background()
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(ctx, userID, "reset-validation.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	tests := []struct {
		name           string
		siteID         string
		body           string
		apiClientAuth  *database.APIClientAuth
		expectedStatus int
	}{
		{
			name:           "invalid site id",
			siteID:         "not-a-uuid",
			body:           `{"confirm_domain":"reset-validation.test"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing site",
			siteID:         uuid.New().String(),
			body:           `{"confirm_domain":"reset-validation.test"}`,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "missing body",
			siteID:         site.ID.String(),
			body:           ``,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "domain mismatch",
			siteID:         site.ID.String(),
			body:           `{"confirm_domain":"other.test"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "api client rejected",
			siteID: site.ID.String(),
			body:   `{"confirm_domain":"reset-validation.test"}`,
			apiClientAuth: &database.APIClientAuth{
				ClientID:     uuid.New(),
				InstanceRole: auth.InstanceUser,
				SiteRoles:    map[uuid.UUID]auth.SiteRole{site.ID: auth.SiteOwner},
			},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/sites/"+tt.siteID+"/stats/reset", strings.NewReader(tt.body))
			req.SetPathValue("id", tt.siteID)
			req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
			if tt.apiClientAuth != nil {
				req = req.WithContext(context.WithValue(req.Context(), shared.APIClientAuthKey, tt.apiClientAuth))
			}
			w := httptest.NewRecorder()

			h.handleResetSiteStats().ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestResetSiteStatsRequiresSiteDeletePermission(t *testing.T) {
	ctx := context.Background()
	h, store, ownerID := setupTestEnv(t)
	defer store.Close()

	adminID, err := store.CreateUser(ctx, "site-admin-reset@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create site admin: %v", err)
	}
	defaultTeamID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default team: %v", err)
	}
	if err := store.AddTeamMember(ctx, defaultTeamID, adminID, database.TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add team member: %v", err)
	}
	site, err := store.CreateSite(ctx, ownerID, "reset-permission.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.AddSiteMember(ctx, site.ID, adminID, auth.SiteAdmin, ownerID); err != nil {
		t.Fatalf("add site admin: %v", err)
	}

	handler := h.ctx.RequirePermission(auth.PermSiteDelete)(h.handleResetSiteStats())

	unauthReq := newResetSiteStatsRequest(site.ID, site.Domain)
	unauthW := httptest.NewRecorder()
	handler.ServeHTTP(unauthW, unauthReq)
	if unauthW.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated status %d, got %d: %s", http.StatusUnauthorized, unauthW.Code, unauthW.Body.String())
	}

	adminReq := newResetSiteStatsRequest(site.ID, site.Domain)
	adminReq = adminReq.WithContext(context.WithValue(adminReq.Context(), shared.UserIDKey, adminID))
	adminW := httptest.NewRecorder()
	handler.ServeHTTP(adminW, adminReq)
	if adminW.Code != http.StatusForbidden {
		t.Fatalf("expected site admin status %d, got %d: %s", http.StatusForbidden, adminW.Code, adminW.Body.String())
	}

	apiClientReq := newResetSiteStatsRequest(site.ID, site.Domain)
	apiClientReq = apiClientReq.WithContext(context.WithValue(apiClientReq.Context(), shared.APIClientAuthKey, &database.APIClientAuth{
		ClientID:     uuid.New(),
		InstanceRole: auth.InstanceUser,
		SiteRoles:    map[uuid.UUID]auth.SiteRole{site.ID: auth.SiteOwner},
	}))
	apiClientW := httptest.NewRecorder()
	handler.ServeHTTP(apiClientW, apiClientReq)
	if apiClientW.Code != http.StatusForbidden {
		t.Fatalf("expected api client status %d, got %d: %s", http.StatusForbidden, apiClientW.Code, apiClientW.Body.String())
	}
}

func TestHandleResetSiteStatsSuccessClearsRowsAndAudits(t *testing.T) {
	ctx := context.Background()
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(ctx, userID, "reset-success.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	now := time.Now().UTC()
	if err := store.CreateHit(ctx, &api.Hit{
		ID:        uuid.New(),
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: now,
		Path:      "/",
	}); err != nil {
		t.Fatalf("create hit: %v", err)
	}
	importID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO site_imports (id, site_id, provider, status, source_hash, bytes_total, bytes_received, rows_scanned, rows_imported, created_by, created_at, updated_at, finished_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		importID, site.ID, "plausible", database.ImportStatusCompleted, "reset-success", 10, 10, 10, 10, userID, now, now, now,
	); err != nil {
		t.Fatalf("insert import: %v", err)
	}

	req := newResetSiteStatsRequest(site.ID, "RESET-SUCCESS.TEST")
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
	w := httptest.NewRecorder()

	h.handleResetSiteStats().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
	var response api.SiteStatsResetResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "reset" || response.RowsCleared == 0 || response.ImportsMarkedDeleted != 1 {
		t.Fatalf("unexpected reset response: %+v", response)
	}

	var hits int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", site.ID).Scan(&hits); err != nil {
		t.Fatalf("count hits: %v", err)
	}
	if hits != 0 {
		t.Fatalf("expected hits cleared, got %d", hits)
	}
	var deletedImports int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM site_imports WHERE site_id = ? AND status = ?", site.ID, database.ImportStatusDeleted).Scan(&deletedImports); err != nil {
		t.Fatalf("count deleted imports: %v", err)
	}
	if deletedImports != 1 {
		t.Fatalf("expected completed import marked deleted, got %d", deletedImports)
	}

	teamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get site team: %v", err)
	}
	entries, total, err := store.ListTeamAuditEntries(ctx, teamID, "site.stats_reset", 5, 0)
	if err != nil {
		t.Fatalf("list audit entries: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one stats reset audit entry, got total=%d entries=%+v", total, entries)
	}
	entry := entries[0]
	if entry.TargetType != "site" || entry.TargetID != site.ID.String() || entry.TargetLabel != site.Domain || entry.Outcome != "success" {
		t.Fatalf("unexpected audit entry: %+v", entry)
	}
	if !strings.Contains(entry.Details, "rows_cleared=") || !strings.Contains(entry.Details, "imports_marked_deleted=1") {
		t.Fatalf("expected reset summary in audit details, got %q", entry.Details)
	}
}

func newResetSiteStatsRequest(siteID uuid.UUID, confirmDomain string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/stats/reset", strings.NewReader(fmt.Sprintf(`{"confirm_domain":%q}`, confirmDomain)))
	req.SetPathValue("id", siteID.String())
	return req
}

func TestHandleGetSiteStats(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, _ := store.CreateSite(context.Background(), userID, "stats.com")

	tests := []struct {
		name           string
		siteID         string
		injectAuth     bool
		expectedStatus int
	}{
		{
			name:           "Unauthorized",
			siteID:         site.ID.String(),
			injectAuth:     false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Invalid Site ID",
			siteID:         "not-a-uuid",
			injectAuth:     true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Success",
			siteID:         site.ID.String(),
			injectAuth:     true,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/sites/"+tc.siteID+"/stats", nil)
			// Manually inject PathValue since we are bypassing the mux
			req.SetPathValue("id", tc.siteID)

			if tc.injectAuth {
				ctx := context.WithValue(req.Context(), shared.UserIDKey, userID)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()
			handler := h.handleGetSiteStats()
			handler.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, w.Code)
			}
		})
	}
}

func TestHandleGetSiteStatsIncludesPageModes(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(context.Background(), userID, "stats-pages.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	for _, hit := range []struct {
		sessionID uuid.UUID
		path      string
		timestamp time.Time
	}{
		{sessionID: uuid.New(), path: "/home", timestamp: base.Add(-2 * time.Hour)},
		{sessionID: uuid.New(), path: "/pricing", timestamp: base.Add(-90 * time.Minute)},
	} {
		if err := store.CreateHit(context.Background(), &api.Hit{
			SiteID:    site.ID,
			SessionID: hit.sessionID,
			PageID:    uuid.New(),
			Timestamp: hit.timestamp,
			Path:      hit.path,
		}); err != nil {
			t.Fatalf("create hit %s: %v", hit.path, err)
		}
	}

	statsURL := fmt.Sprintf(
		"/api/sites/%s/stats?from=%s&to=%s",
		site.ID,
		base.Add(-24*time.Hour).Format(time.RFC3339),
		base.Add(24*time.Hour).Format(time.RFC3339),
	)
	req := httptest.NewRequest(http.MethodGet, statsURL, nil)
	req.SetPathValue("id", site.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))

	w := httptest.NewRecorder()
	h.handleGetSiteStats().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var stats api.SiteStats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}

	if len(stats.TopLandingPages) == 0 {
		t.Fatalf("expected top_landing_pages in response, got %+v", stats)
	}
	if len(stats.TopExitPages) == 0 {
		t.Fatalf("expected top_exit_pages in response, got %+v", stats)
	}
	if stats.TopLanguages == nil {
		t.Fatalf("expected top_languages in response, got %+v", stats)
	}
}

func TestHandleGetSiteStatsAcceptsAIFilters(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(context.Background(), userID, "stats-ai-filter.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	for _, filter := range []string{"ai_bot:GPTBot", "ai_bot_category:Training", "ai_source:ChatGPT"} {
		t.Run(filter, func(t *testing.T) {
			statsURL := "/api/sites/" + site.ID.String() + "/stats?filter=" + url.QueryEscape(filter)
			req := httptest.NewRequest(http.MethodGet, statsURL, nil)
			req.SetPathValue("id", site.ID.String())
			req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))

			w := httptest.NewRecorder()
			h.handleGetSiteStats().ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("expected status %d for %s, got %d: %s", http.StatusOK, filter, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleGetSiteEcommerceSummary(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(context.Background(), userID, "shop-summary.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	sessionID := uuid.New()
	isUnique := true
	timestamp := time.Date(2026, 3, 7, 9, 0, 0, 0, time.UTC)

	if err := store.CreateHit(context.Background(), &api.Hit{
		SiteID:        site.ID,
		SessionID:     sessionID,
		PageID:        uuid.New(),
		Path:          "/pricing",
		Timestamp:     timestamp,
		ViewportWidth: new(1440),
		CountryCode:   new("US"),
		UTMSource:     new("google"),
		UTMMedium:     new("cpc"),
		UTMCampaign:   new("launch"),
		IsUnique:      &isUnique,
	}); err != nil {
		t.Fatalf("create hit: %v", err)
	}

	if err := store.CreateEvent(context.Background(), &api.Event{
		SiteID:    site.ID,
		SessionID: sessionID,
		Name:      "begin_checkout",
		Timestamp: timestamp.Add(10 * time.Minute),
		Properties: map[string]any{
			"items": []map[string]any{
				{"item_id": "pro", "item_name": "Pro", "quantity": 1, "price": 79.0},
			},
		},
	}); err != nil {
		t.Fatalf("create checkout: %v", err)
	}

	if err := store.CreateEvent(context.Background(), &api.Event{
		SiteID:    site.ID,
		SessionID: sessionID,
		Name:      "purchase",
		Timestamp: timestamp.Add(20 * time.Minute),
		Properties: map[string]any{
			"transaction_id": "ord_2001",
			"value":          79.0,
			"currency":       "USD",
			"items": []map[string]any{
				{"item_id": "pro", "item_name": "Pro", "quantity": 1, "price": 79.0},
			},
		},
	}); err != nil {
		t.Fatalf("create purchase: %v", err)
	}

	from := timestamp.Add(-time.Hour).Format(time.RFC3339)
	to := timestamp.Add(24 * time.Hour).Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/ecommerce?from="+url.QueryEscape(from)+"&to="+url.QueryEscape(to), nil)
	req.SetPathValue("id", site.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))

	w := httptest.NewRecorder()
	h.handleGetSiteEcommerceSummary().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var summary api.EcommerceSummary
	if err := json.NewDecoder(w.Body).Decode(&summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.Orders != 1 || summary.CheckoutStarts != 1 || summary.Revenue != 79 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestHandleGetSiteWebVitalsSummary(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(context.Background(), userID, "vitals-summary.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	base := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	if err := store.CreateWebVitalsBulk(context.Background(), []*api.WebVital{
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Metric: api.WebVitalLCP, Value: 1200, Path: "/"},
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Metric: api.WebVitalLCP, Value: 2800, Path: "/pricing"},
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Metric: api.WebVitalLCP, Value: 5200, Path: "/checkout"},
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Metric: api.WebVitalCLS, Value: 0.08, Path: "/", Timestamp: base},
	}); err != nil {
		t.Fatalf("CreateWebVitalsBulk: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/web-vitals/summary", nil)
	req.SetPathValue("id", site.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))

	w := httptest.NewRecorder()
	h.handleGetSiteWebVitalsSummary().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var summary []api.WebVitalSummaryMetric
	if err := json.NewDecoder(w.Body).Decode(&summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	lcp := findWebVitalSummaryMetric(summary, api.WebVitalLCP)
	if lcp == nil {
		t.Fatalf("expected LCP summary in %+v", summary)
	}
	if lcp.Samples != 3 || lcp.Good != 1 || lcp.NeedsImprove != 1 || lcp.Poor != 1 {
		t.Fatalf("unexpected LCP distribution: %+v", *lcp)
	}
	if lcp.Rating != api.WebVitalRatingNeedsImprovement {
		t.Fatalf("expected LCP p75 rating needs_improvement, got %q", lcp.Rating)
	}
}

func TestHandleGetSiteWebVitalsTimeseriesRequiresMetric(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(context.Background(), userID, "vitals-metric.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/web-vitals/timeseries", nil)
	req.SetPathValue("id", site.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))

	w := httptest.NewRecorder()
	h.handleGetSiteWebVitalsTimeseries().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleGetSiteWebVitalsPagesSupportsFilters(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(context.Background(), userID, "vitals-pages.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	base := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	if err := store.CreateWebVitalsBulk(context.Background(), []*api.WebVital{
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Metric: api.WebVitalINP, Value: 180, Path: "/pricing", Timestamp: base},
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Metric: api.WebVitalINP, Value: 640, Path: "/checkout", Timestamp: base.Add(time.Hour)},
	}); err != nil {
		t.Fatalf("CreateWebVitalsBulk: %v", err)
	}

	from := base.Add(-time.Hour).Format(time.RFC3339)
	to := base.Add(2 * time.Hour).Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/web-vitals/pages?metric=INP&rating=poor&from="+url.QueryEscape(from)+"&to="+url.QueryEscape(to), nil)
	req.SetPathValue("id", site.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))

	w := httptest.NewRecorder()
	h.handleGetSiteWebVitalsPages().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var rows []api.WebVitalPageRow
	if err := json.NewDecoder(w.Body).Decode(&rows); err != nil {
		t.Fatalf("decode pages: %v", err)
	}
	if len(rows) != 1 || rows[0].Path != "/checkout" || rows[0].Rating != api.WebVitalRatingPoor {
		t.Fatalf("unexpected rows: %+v", rows)
	}
	if rows[0].Metrics[api.WebVitalINP].Samples != 1 {
		t.Fatalf("expected INP metric cell on page row, got %+v", rows[0].Metrics)
	}
}

func TestHandleGetSiteWebVitalsBreakdownReturnsVisitorContext(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(context.Background(), userID, "vitals-breakdown.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	base := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	sessionID := uuid.New()
	pageID := uuid.New()
	ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_2) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	lang := "en-US"
	country := "US"
	viewportWidth := 1440
	if err := store.CreateHit(context.Background(), &api.Hit{
		SiteID:        site.ID,
		SessionID:     sessionID,
		PageID:        pageID,
		Timestamp:     base.Add(-time.Second),
		Path:          "/pricing",
		UserAgent:     &ua,
		Language:      &lang,
		CountryCode:   &country,
		ViewportWidth: &viewportWidth,
	}); err != nil {
		t.Fatalf("CreateHit: %v", err)
	}
	if err := store.CreateWebVitalsBulk(context.Background(), []*api.WebVital{
		{SiteID: site.ID, SessionID: sessionID, PageID: pageID, Metric: api.WebVitalLCP, Value: 2800, Path: "/pricing", Timestamp: base},
	}); err != nil {
		t.Fatalf("CreateWebVitalsBulk: %v", err)
	}

	from := base.Add(-time.Hour).Format(time.RFC3339)
	to := base.Add(time.Hour).Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/web-vitals/breakdown?metric=LCP&dimension=browser&from="+url.QueryEscape(from)+"&to="+url.QueryEscape(to), nil)
	req.SetPathValue("id", site.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))

	w := httptest.NewRecorder()
	h.handleGetSiteWebVitalsBreakdown().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var rows []api.WebVitalDimensionRow
	if err := json.NewDecoder(w.Body).Decode(&rows); err != nil {
		t.Fatalf("decode breakdown: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "Chrome" || rows[0].Samples != 1 {
		t.Fatalf("unexpected breakdown rows: %+v", rows)
	}
}

func TestHandleGetSiteEcommerceProductsSupportsItemFilter(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(context.Background(), userID, "shop-products.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	sessionID := uuid.New()
	isUnique := true
	timestamp := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	if err := store.CreateHit(context.Background(), &api.Hit{
		SiteID:      site.ID,
		SessionID:   sessionID,
		PageID:      uuid.New(),
		Path:        "/checkout",
		Timestamp:   timestamp,
		UTMSource:   new("newsletter"),
		UTMMedium:   new("email"),
		UTMCampaign: new("digest"),
		IsUnique:    &isUnique,
	}); err != nil {
		t.Fatalf("create hit: %v", err)
	}

	if err := store.CreateEvent(context.Background(), &api.Event{
		SiteID:    site.ID,
		SessionID: sessionID,
		Name:      "order_completed",
		Timestamp: timestamp.Add(15 * time.Minute),
		Properties: map[string]any{
			"order_id": "ord_3001",
			"amount":   100.0,
			"currency": "USD",
			"items": []map[string]any{
				{"product_id": "starter", "product_name": "Starter", "quantity": 1, "price": 40.0},
				{"product_id": "addon", "product_name": "Addon", "quantity": 2, "price": 30.0},
			},
		},
	}); err != nil {
		t.Fatalf("create purchase: %v", err)
	}

	from := timestamp.Add(-time.Hour).Format(time.RFC3339)
	to := timestamp.Add(24 * time.Hour).Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/ecommerce/products?item_id=addon&from="+url.QueryEscape(from)+"&to="+url.QueryEscape(to), nil)
	req.SetPathValue("id", site.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))

	w := httptest.NewRecorder()
	h.handleGetSiteEcommerceProducts().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var products []api.EcommerceProductStat
	if err := json.NewDecoder(w.Body).Decode(&products); err != nil {
		t.Fatalf("decode products: %v", err)
	}
	if len(products) != 2 {
		t.Fatalf("expected both products from the filtered purchase, got %+v", products)
	}
}

func findWebVitalSummaryMetric(metrics []api.WebVitalSummaryMetric, metric api.WebVitalMetric) *api.WebVitalSummaryMetric {
	for i := range metrics {
		if metrics[i].Metric == metric {
			return &metrics[i]
		}
	}
	return nil
}

func TestHandleGetSiteHits(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, _ := store.CreateSite(context.Background(), userID, "hits.com")

	tests := []struct {
		name           string
		siteID         string
		injectAuth     bool
		queryParams    string
		expectedStatus int
	}{
		{
			name:           "Unauthorized",
			siteID:         site.ID.String(),
			injectAuth:     false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Success - Defaults",
			siteID:         site.ID.String(),
			injectAuth:     true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Success - With Params",
			siteID:         site.ID.String(),
			injectAuth:     true,
			queryParams:    "?limit=5&offset=0",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/sites/"+tc.siteID+"/hits"+tc.queryParams, nil)
			req.SetPathValue("id", tc.siteID)

			if tc.injectAuth {
				ctx := context.WithValue(req.Context(), shared.UserIDKey, userID)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()
			handler := h.handleGetSiteHits()
			handler.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, w.Code)
			}
		})
	}
}

func TestHandleExportSiteHitsSupportsAllFormats(t *testing.T) {
	h, store, userID := setupTestEnv(t)
	t.Cleanup(func() { _ = store.Close() })

	site, err := store.CreateSite(context.Background(), userID, "export-hits.test")
	if err != nil {
		t.Fatalf("failed to create export site: %v", err)
	}

	now := time.Now().UTC()
	isUnique := true
	if err := store.CreateHit(context.Background(), &api.Hit{
		SiteID:      site.ID,
		SessionID:   uuid.New(),
		PageID:      uuid.New(),
		Timestamp:   now,
		Path:        "/export",
		UTMSource:   new("newsletter"),
		UTMMedium:   new("email"),
		UTMCampaign: new("launch"),
		UTMTerm:     new("format"),
		UTMContent:  new("cta"),
		IsUnique:    &isUnique,
	}); err != nil {
		t.Fatalf("failed to seed export hit: %v", err)
	}

	tests := []struct {
		name           string
		siteID         string
		queryFormat    string
		expectedExt    string
		expectedType   string
		expectedStatus int
		withAuth       bool
	}{
		{name: "csv", siteID: site.ID.String(), queryFormat: "csv", expectedExt: ".csv", expectedType: exportfmt.ContentType(exportfmt.FormatCSV), expectedStatus: http.StatusOK, withAuth: true},
		{name: "xlsx", siteID: site.ID.String(), queryFormat: "xlsx", expectedExt: ".xlsx", expectedType: exportfmt.ContentType(exportfmt.FormatXLSX), expectedStatus: http.StatusOK, withAuth: true},
		{name: "parquet", siteID: site.ID.String(), queryFormat: "parquet", expectedExt: ".parquet", expectedType: exportfmt.ContentType(exportfmt.FormatParquet), expectedStatus: http.StatusOK, withAuth: true},
		{name: "json", siteID: site.ID.String(), queryFormat: "json", expectedExt: ".json", expectedType: exportfmt.ContentType(exportfmt.FormatJSON), expectedStatus: http.StatusOK, withAuth: true},
		{name: "ndjson", siteID: site.ID.String(), queryFormat: "ndjson", expectedExt: ".ndjson", expectedType: exportfmt.ContentType(exportfmt.FormatNDJSON), expectedStatus: http.StatusOK, withAuth: true},
		{name: "unknown defaults to csv", siteID: site.ID.String(), queryFormat: "xml", expectedExt: ".csv", expectedType: exportfmt.ContentType(exportfmt.FormatCSV), expectedStatus: http.StatusOK, withAuth: true},
		{name: "unauthorized", siteID: site.ID.String(), queryFormat: "csv", expectedStatus: http.StatusUnauthorized},
		{name: "invalid site id", siteID: "invalid-uuid", queryFormat: "csv", expectedStatus: http.StatusBadRequest, withAuth: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/sites/"+tc.siteID+"/hits/export?format="+tc.queryFormat, nil)
			req.SetPathValue("id", tc.siteID)
			if tc.withAuth {
				req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
			}

			w := httptest.NewRecorder()
			h.handleExportSiteHits().ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Fatalf("expected status %d, got %d (body: %s)", tc.expectedStatus, w.Code, w.Body.String())
			}
			if tc.expectedStatus != http.StatusOK {
				return
			}

			if got := w.Header().Get("Content-Type"); got != tc.expectedType {
				t.Fatalf("expected content-type %q, got %q", tc.expectedType, got)
			}

			disposition := w.Header().Get("Content-Disposition")
			if !strings.Contains(disposition, tc.expectedExt) {
				t.Fatalf("expected content-disposition %q to contain extension %q", disposition, tc.expectedExt)
			}

			if w.Body.Len() == 0 {
				t.Fatalf("expected non-empty export response body")
			}
		})
	}
}

func newRenameSiteDomainRequest(siteID string, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPut, "/api/sites/"+siteID+"/domain", strings.NewReader(body))
	req.SetPathValue("id", siteID)
	return req
}

func TestRenameSiteDomainRequiresSiteAdminOrHigher(t *testing.T) {
	ctx := context.Background()
	h, store, ownerID := setupTestEnv(t)
	defer store.Close()

	editorID, err := store.CreateUser(ctx, "site-editor-rename@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create site editor: %v", err)
	}
	adminID, err := store.CreateUser(ctx, "site-admin-rename@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create site admin: %v", err)
	}
	defaultTeamID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default team: %v", err)
	}
	for _, memberID := range []uuid.UUID{editorID, adminID} {
		if err := store.AddTeamMember(ctx, defaultTeamID, memberID, database.TenantRoleMember, ownerID); err != nil {
			t.Fatalf("add team member: %v", err)
		}
	}
	site, err := store.CreateSite(ctx, ownerID, "rename-permission.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.AddSiteMember(ctx, site.ID, editorID, auth.SiteEditor, ownerID); err != nil {
		t.Fatalf("add site editor: %v", err)
	}
	if err := store.AddSiteMember(ctx, site.ID, adminID, auth.SiteAdmin, ownerID); err != nil {
		t.Fatalf("add site admin: %v", err)
	}

	handler := h.ctx.RequirePermission(auth.PermSiteManageData)(h.handleRenameSiteDomain())

	unauthW := httptest.NewRecorder()
	handler.ServeHTTP(unauthW, newRenameSiteDomainRequest(site.ID.String(), `{"domain":"unauth-rename.test"}`))
	if unauthW.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated status %d, got %d: %s", http.StatusUnauthorized, unauthW.Code, unauthW.Body.String())
	}

	editorReq := newRenameSiteDomainRequest(site.ID.String(), `{"domain":"editor-rename.test"}`)
	editorReq = editorReq.WithContext(context.WithValue(editorReq.Context(), shared.UserIDKey, editorID))
	editorW := httptest.NewRecorder()
	handler.ServeHTTP(editorW, editorReq)
	if editorW.Code != http.StatusForbidden {
		t.Fatalf("expected site editor status %d, got %d: %s", http.StatusForbidden, editorW.Code, editorW.Body.String())
	}

	adminReq := newRenameSiteDomainRequest(site.ID.String(), `{"domain":"admin-rename.test"}`)
	adminReq = adminReq.WithContext(context.WithValue(adminReq.Context(), shared.UserIDKey, adminID))
	adminW := httptest.NewRecorder()
	handler.ServeHTTP(adminW, adminReq)
	if adminW.Code != http.StatusOK {
		t.Fatalf("expected site admin status %d, got %d: %s", http.StatusOK, adminW.Code, adminW.Body.String())
	}

	ownerReq := newRenameSiteDomainRequest(site.ID.String(), `{"domain":"owner-rename.test"}`)
	ownerReq = ownerReq.WithContext(context.WithValue(ownerReq.Context(), shared.UserIDKey, ownerID))
	ownerW := httptest.NewRecorder()
	handler.ServeHTTP(ownerW, ownerReq)
	if ownerW.Code != http.StatusOK {
		t.Fatalf("expected site owner status %d, got %d: %s", http.StatusOK, ownerW.Code, ownerW.Body.String())
	}

	renamed, err := store.GetSiteByID(ctx, site.ID)
	if err != nil || renamed == nil {
		t.Fatalf("get renamed site: %v", err)
	}
	if renamed.Domain != "owner-rename.test" {
		t.Fatalf("expected domain owner-rename.test, got %s", renamed.Domain)
	}
}

func TestHandleRenameSiteDomainValidatesRequest(t *testing.T) {
	ctx := context.Background()
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(ctx, userID, "rename-validation.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if _, err := store.CreateSite(ctx, userID, "rename-taken.test"); err != nil {
		t.Fatalf("create conflicting site: %v", err)
	}

	tests := []struct {
		name           string
		siteID         string
		body           string
		expectedStatus int
	}{
		{"invalid site id", "not-a-uuid", `{"domain":"valid-rename.test"}`, http.StatusBadRequest},
		{"missing site", uuid.New().String(), `{"domain":"valid-rename.test"}`, http.StatusNotFound},
		{"missing body", site.ID.String(), ``, http.StatusBadRequest},
		{"empty domain", site.ID.String(), `{"domain":""}`, http.StatusBadRequest},
		{"protocol", site.ID.String(), `{"domain":"https://example.com"}`, http.StatusBadRequest},
		{"www prefix", site.ID.String(), `{"domain":"www.example.com"}`, http.StatusBadRequest},
		{"invalid characters", site.ID.String(), `{"domain":"inva lid.com"}`, http.StatusBadRequest},
		{"duplicate domain", site.ID.String(), `{"domain":"rename-taken.test"}`, http.StatusConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newRenameSiteDomainRequest(tt.siteID, tt.body)
			req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
			w := httptest.NewRecorder()

			h.handleRenameSiteDomain().ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleRenameSiteDomainAcceptsIssueDomains(t *testing.T) {
	ctx := context.Background()
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	tests := []string{
		"sub.example-app.com.br",
		"app-staging.example-service.co.uk",
	}
	for i, domain := range tests {
		site, err := store.CreateSite(ctx, userID, fmt.Sprintf("rename-issue-%d.test", i))
		if err != nil {
			t.Fatalf("create site: %v", err)
		}

		req := newRenameSiteDomainRequest(site.ID.String(), fmt.Sprintf(`{"domain":%q}`, domain))
		req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
		w := httptest.NewRecorder()

		h.handleRenameSiteDomain().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("domain %q: expected status %d, got %d: %s", domain, http.StatusOK, w.Code, w.Body.String())
		}
		stored, err := store.GetSiteByID(ctx, site.ID)
		if err != nil || stored == nil {
			t.Fatalf("domain %q: get renamed site: %v", domain, err)
		}
		if stored.Domain != domain {
			t.Fatalf("domain %q: expected stored domain %q, got %q", domain, domain, stored.Domain)
		}
	}
}

func TestHandleRenameSiteDomainSuccessUpdatesAndAudits(t *testing.T) {
	ctx := context.Background()
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(ctx, userID, "rename-old.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	req := newRenameSiteDomainRequest(site.ID.String(), `{"domain":"Rename-NEW.test"}`)
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
	w := httptest.NewRecorder()

	h.handleRenameSiteDomain().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
	var response api.Site
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ID != site.ID || response.Domain != "rename-new.test" {
		t.Fatalf("unexpected rename response: %+v", response)
	}

	stored, err := store.GetSiteByID(ctx, site.ID)
	if err != nil || stored == nil {
		t.Fatalf("get renamed site: %v", err)
	}
	if stored.Domain != "rename-new.test" {
		t.Fatalf("expected stored domain rename-new.test, got %s", stored.Domain)
	}

	teamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get site team: %v", err)
	}
	entries, total, err := store.ListTeamAuditEntries(ctx, teamID, "site.domain_renamed", 5, 0)
	if err != nil {
		t.Fatalf("list audit entries: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one domain rename audit entry, got total=%d entries=%+v", total, entries)
	}
	entry := entries[0]
	if entry.TargetType != "site" || entry.TargetID != site.ID.String() || entry.TargetLabel != "rename-new.test" || entry.Outcome != "success" {
		t.Fatalf("unexpected audit entry: %+v", entry)
	}
	if !strings.Contains(entry.Details, "rename-old.test") || !strings.Contains(entry.Details, "rename-new.test") {
		t.Fatalf("expected old and new domain in audit details, got %q", entry.Details)
	}

	// Renaming to the current domain is a no-op that must not add audit noise.
	noopReq := newRenameSiteDomainRequest(site.ID.String(), `{"domain":"rename-new.test"}`)
	noopReq = noopReq.WithContext(context.WithValue(noopReq.Context(), shared.UserIDKey, userID))
	noopW := httptest.NewRecorder()
	h.handleRenameSiteDomain().ServeHTTP(noopW, noopReq)
	if noopW.Code != http.StatusOK {
		t.Fatalf("expected no-op status %d, got %d: %s", http.StatusOK, noopW.Code, noopW.Body.String())
	}
	if _, total, err := store.ListTeamAuditEntries(ctx, teamID, "site.domain_renamed", 5, 0); err != nil || total != 1 {
		t.Fatalf("expected audit total to stay 1 after no-op, got total=%d err=%v", total, err)
	}
}

func TestHandleGetSiteSetupStateReportsRecordedSurfaces(t *testing.T) {
	ctx := context.Background()
	h, store, userID := setupTestEnv(t)
	defer store.Close()

	site, err := store.CreateSite(ctx, userID, "setup-state-handler.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.CreateWebVital(ctx, &api.WebVital{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Metric:    api.WebVitalLCP,
		Value:     1500,
		Path:      "/",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create web vital: %v", err)
	}

	newRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/setup-state", nil)
		req.SetPathValue("id", site.ID.String())
		return req
	}

	guarded := h.ctx.RequirePermission(auth.PermSiteView)(h.handleGetSiteSetupState())
	unauthW := httptest.NewRecorder()
	guarded.ServeHTTP(unauthW, newRequest())
	if unauthW.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated status %d, got %d: %s", http.StatusUnauthorized, unauthW.Code, unauthW.Body.String())
	}

	w := httptest.NewRecorder()
	guarded.ServeHTTP(w, newRequest().WithContext(context.WithValue(ctx, shared.UserIDKey, userID)))
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var state map[string]any
	if err := json.NewDecoder(w.Body).Decode(&state); err != nil {
		t.Fatalf("decode setup state: %v", err)
	}
	want := map[string]any{
		"has_ai_fetches":       false,
		"has_chatbot_events":   false,
		"has_custom_events":    false,
		"has_ecommerce_events": false,
		"has_web_vitals":       true,
	}
	if !reflect.DeepEqual(state, want) {
		t.Fatalf("unexpected setup state payload: got %+v, want %+v", state, want)
	}
}
