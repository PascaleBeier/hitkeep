package sites

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
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

	ctx := &shared.Context{
		Store:  store,
		Config: &config.Config{},
	}

	return &handler{ctx: ctx}, store, userID
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

func TestHandleTransferSiteTeam(t *testing.T) {
	h, store, tenantStores, userID := setupFileBackedTransferEnv(t)
	defer store.Close()
	defer tenantStores.Close()

	site, err := store.CreateSite(context.Background(), userID, "move-me.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
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
