package searchconsolereports

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

func TestSearchConsoleOverviewReturnsMappedSiteMetrics(t *testing.T) {
	ctx := context.Background()
	store, appCtx, _, siteID, token := setupSearchConsoleReportsTestEnv(t)
	seedSearchConsoleReportMapping(t, store, siteID)
	seedSearchConsoleReportFact(t, store, database.SearchConsoleFactInput{
		SiteID:          siteID,
		PropertyURI:     "sc-domain:reports.example.com",
		Date:            time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Query:           "privacy analytics",
		Page:            "https://reports.example.com/",
		Country:         "US",
		Device:          "DESKTOP",
		Clicks:          8,
		Impressions:     100,
		CTR:             0.08,
		Position:        3.5,
		AggregationType: "auto",
		DataState:       "final",
		ImportedAt:      time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
	})

	mux := http.NewServeMux()
	Register(mux, appCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/search-console/overview?from=2026-05-01T00:00:00Z&to=2026-05-02T00:00:00Z", nil)
	req.Header.Set("X-API-Key", token)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var overview api.SearchConsoleOverview
	if err := json.NewDecoder(rec.Body).Decode(&overview); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if overview.DataSource != "google_search_console" {
		t.Fatalf("expected Search Console data source, got %q", overview.DataSource)
	}
	if overview.Clicks != 8 || overview.Impressions != 100 || overview.CTR != 0.08 || overview.AveragePosition != 3.5 {
		t.Fatalf("unexpected overview metrics: %+v", overview)
	}

	if _, err := store.GetGoogleSearchConsoleSiteMapping(ctx, siteID); err != nil {
		t.Fatalf("mapping should remain readable after report request: %v", err)
	}
}

func TestSearchConsoleSeriesReturnsDailyRowsSortedByDate(t *testing.T) {
	store, appCtx, _, siteID, token := setupSearchConsoleReportsTestEnv(t)
	seedSearchConsoleReportMapping(t, store, siteID)
	for _, fact := range []database.SearchConsoleFactInput{
		{
			SiteID:          siteID,
			PropertyURI:     "sc-domain:reports.example.com",
			Date:            time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
			Query:           "hitkeep reports",
			Page:            "https://reports.example.com/docs",
			Country:         "US",
			Device:          "DESKTOP",
			Clicks:          5,
			Impressions:     50,
			CTR:             0.1,
			Position:        4,
			AggregationType: "auto",
			DataState:       "final",
			ImportedAt:      time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
		},
		{
			SiteID:          siteID,
			PropertyURI:     "sc-domain:reports.example.com",
			Date:            time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			Query:           "hitkeep reports",
			Page:            "https://reports.example.com/docs",
			Country:         "US",
			Device:          "MOBILE",
			Clicks:          3,
			Impressions:     30,
			CTR:             0.1,
			Position:        6,
			AggregationType: "auto",
			DataState:       "final",
			ImportedAt:      time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
		},
	} {
		seedSearchConsoleReportFact(t, store, fact)
	}

	mux := http.NewServeMux()
	Register(mux, appCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/search-console/series?from=2026-05-01T00:00:00Z&to=2026-05-02T00:00:00Z", nil)
	req.Header.Set("X-API-Key", token)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	var resp api.SearchConsoleSeriesResponse
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&resp); err != nil {
		t.Fatalf("decode series: %v", err)
	}
	if resp.DataSource != "google_search_console" || len(resp.Series) != 2 {
		t.Fatalf("unexpected series response: %+v", resp)
	}
	if time.Time(resp.Series[0].Date).Format(time.DateOnly) != "2026-05-01" || resp.Series[0].Clicks != 3 {
		t.Fatalf("expected first sorted point for 2026-05-01, got %+v", resp.Series[0])
	}
	if time.Time(resp.Series[1].Date).Format(time.DateOnly) != "2026-05-02" || resp.Series[1].Clicks != 5 {
		t.Fatalf("expected second sorted point for 2026-05-02, got %+v", resp.Series[1])
	}
}

func TestSearchConsoleSeriesReturnsEmptyArrayWhenNoRowsMatch(t *testing.T) {
	store, appCtx, _, siteID, token := setupSearchConsoleReportsTestEnv(t)
	seedSearchConsoleReportMapping(t, store, siteID)

	mux := http.NewServeMux()
	Register(mux, appCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/search-console/series?from=2026-05-01T00:00:00Z&to=2026-05-02T00:00:00Z", nil)
	req.Header.Set("X-API-Key", token)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	var resp api.SearchConsoleSeriesResponse
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&resp); err != nil {
		t.Fatalf("decode series: %v", err)
	}
	if resp.Series == nil || len(resp.Series) != 0 {
		t.Fatalf("expected empty series array, got %#v", resp.Series)
	}
	if !strings.Contains(body, `"series":[]`) {
		t.Fatalf("expected JSON series array, got %s", body)
	}
}

func TestSearchConsoleQueriesHonorDateFilters(t *testing.T) {
	store, appCtx, _, siteID, token := setupSearchConsoleReportsTestEnv(t)
	seedSearchConsoleReportMapping(t, store, siteID)
	inside := database.SearchConsoleFactInput{
		SiteID:          siteID,
		PropertyURI:     "sc-domain:reports.example.com",
		Date:            time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
		Query:           "inside query",
		Page:            "https://reports.example.com/docs",
		Country:         "US",
		Device:          "DESKTOP",
		Clicks:          12,
		Impressions:     120,
		CTR:             0.1,
		Position:        4,
		AggregationType: "auto",
		DataState:       "final",
		ImportedAt:      time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
	}
	outside := inside
	outside.Date = time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)
	outside.Query = "outside query"
	outside.Clicks = 99
	outside.Impressions = 990
	seedSearchConsoleReportFact(t, store, inside)
	seedSearchConsoleReportFact(t, store, outside)

	mux := http.NewServeMux()
	Register(mux, appCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/search-console/queries?from=2026-05-01T00:00:00Z&to=2026-05-03T00:00:00Z", nil)
	req.Header.Set("X-API-Key", token)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	var resp api.SearchConsoleDimensionResponse
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&resp); err != nil {
		t.Fatalf("decode query rows: %v", err)
	}
	if resp.DataSource != "google_search_console" || resp.Dimension != "query" || len(resp.Rows) != 1 {
		t.Fatalf("unexpected query response: %+v", resp)
	}
	if resp.Rows[0].Value != "inside query" || resp.Rows[0].Clicks != 12 {
		t.Fatalf("expected date-filtered inside query row, got %+v", resp.Rows[0])
	}
}

func TestSearchConsolePagesApplyCountryAndDeviceFilters(t *testing.T) {
	store, appCtx, _, siteID, token := setupSearchConsoleReportsTestEnv(t)
	seedSearchConsoleReportMapping(t, store, siteID)
	base := database.SearchConsoleFactInput{
		SiteID:          siteID,
		PropertyURI:     "sc-domain:reports.example.com",
		Date:            time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
		Query:           "filtered page",
		Page:            "https://reports.example.com/docs",
		Country:         "usa",
		Device:          "DESKTOP",
		Clicks:          7,
		Impressions:     70,
		CTR:             0.1,
		Position:        3,
		AggregationType: "auto",
		DataState:       "final",
		ImportedAt:      time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
	}
	otherCountry := base
	otherCountry.Page = "https://reports.example.com/fr"
	otherCountry.Country = "fra"
	otherCountry.Clicks = 40
	otherDevice := base
	otherDevice.Page = "https://reports.example.com/mobile"
	otherDevice.Device = "MOBILE"
	otherDevice.Clicks = 50
	seedSearchConsoleReportFact(t, store, base)
	seedSearchConsoleReportFact(t, store, otherCountry)
	seedSearchConsoleReportFact(t, store, otherDevice)

	mux := http.NewServeMux()
	Register(mux, appCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/search-console/pages?from=2026-05-01T00:00:00Z&to=2026-05-03T00:00:00Z&country=us&device=desktop", nil)
	req.Header.Set("X-API-Key", token)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	var resp api.SearchConsoleDimensionResponse
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&resp); err != nil {
		t.Fatalf("decode page rows: %v", err)
	}
	if resp.Dimension != "page" || len(resp.Rows) != 1 {
		t.Fatalf("unexpected page response: %+v", resp)
	}
	if resp.Rows[0].Value != "https://reports.example.com/docs" || resp.Rows[0].Clicks != 7 {
		t.Fatalf("expected filtered page row, got %+v", resp.Rows[0])
	}
}

func TestSearchConsoleOverviewAppliesPathFilterToPageURLs(t *testing.T) {
	store, appCtx, _, siteID, token := setupSearchConsoleReportsTestEnv(t)
	seedSearchConsoleReportMapping(t, store, siteID)
	matching := database.SearchConsoleFactInput{
		SiteID:          siteID,
		PropertyURI:     "sc-domain:reports.example.com",
		Date:            time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
		Query:           "path report",
		Page:            "https://reports.example.com/docs?utm_source=google",
		Country:         "US",
		Device:          "DESKTOP",
		Clicks:          6,
		Impressions:     60,
		CTR:             0.1,
		Position:        2,
		AggregationType: "auto",
		DataState:       "final",
		ImportedAt:      time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
	}
	other := matching
	other.Page = "https://reports.example.com/archive/docs"
	other.Clicks = 30
	other.Impressions = 300
	seedSearchConsoleReportFact(t, store, matching)
	seedSearchConsoleReportFact(t, store, other)

	mux := http.NewServeMux()
	Register(mux, appCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/search-console/overview?from=2026-05-01T00:00:00Z&to=2026-05-03T00:00:00Z&path=/docs", nil)
	req.Header.Set("X-API-Key", token)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var overview api.SearchConsoleOverview
	if err := json.NewDecoder(rec.Body).Decode(&overview); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if overview.Clicks != 6 || overview.Impressions != 60 {
		t.Fatalf("expected path-filtered overview, got %+v", overview)
	}
}

func TestSearchConsoleQueriesReturnEmptyRowsArrayWhenNoRowsMatch(t *testing.T) {
	store, appCtx, _, siteID, token := setupSearchConsoleReportsTestEnv(t)
	seedSearchConsoleReportMapping(t, store, siteID)

	mux := http.NewServeMux()
	Register(mux, appCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/search-console/queries?from=2026-05-01T00:00:00Z&to=2026-05-02T00:00:00Z", nil)
	req.Header.Set("X-API-Key", token)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	var resp api.SearchConsoleDimensionResponse
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&resp); err != nil {
		t.Fatalf("decode query rows: %v", err)
	}
	if resp.Rows == nil || len(resp.Rows) != 0 {
		t.Fatalf("expected empty rows array, got %#v", resp.Rows)
	}
	if !strings.Contains(body, `"rows":[]`) {
		t.Fatalf("expected JSON rows array, got %s", body)
	}
}

func TestSearchConsoleBreakdownsRejectUnsupportedDimensions(t *testing.T) {
	_, appCtx, _, siteID, token := setupSearchConsoleReportsTestEnv(t)

	mux := http.NewServeMux()
	Register(mux, appCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/search-console/breakdowns?dimension=query", nil)
	req.Header.Set("X-API-Key", token)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "Invalid dimension\n" {
		t.Fatalf("expected stable invalid dimension error, got %q", rec.Body.String())
	}
}

func TestSearchConsoleBreakdownsReturnCountryRows(t *testing.T) {
	store, appCtx, _, siteID, token := setupSearchConsoleReportsTestEnv(t)
	seedSearchConsoleReportMapping(t, store, siteID)
	base := database.SearchConsoleFactInput{
		SiteID:          siteID,
		PropertyURI:     "sc-domain:reports.example.com",
		Date:            time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
		Query:           "country report",
		Page:            "https://reports.example.com/",
		Country:         "US",
		Device:          "DESKTOP",
		Clicks:          9,
		Impressions:     90,
		CTR:             0.1,
		Position:        2,
		AggregationType: "auto",
		DataState:       "final",
		ImportedAt:      time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
	}
	other := base
	other.Country = "DE"
	other.Clicks = 3
	other.Impressions = 30
	seedSearchConsoleReportFact(t, store, base)
	seedSearchConsoleReportFact(t, store, other)

	mux := http.NewServeMux()
	Register(mux, appCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/search-console/breakdowns?dimension=country&from=2026-05-01T00:00:00Z&to=2026-05-03T00:00:00Z", nil)
	req.Header.Set("X-API-Key", token)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var resp api.SearchConsoleDimensionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode country rows: %v", err)
	}
	if resp.Dimension != "country" || len(resp.Rows) != 2 {
		t.Fatalf("unexpected country response: %+v", resp)
	}
	if resp.Rows[0].Value != "USA" || resp.Rows[0].Clicks != 9 {
		t.Fatalf("expected US first by clicks, got %+v", resp.Rows[0])
	}
}

func TestSearchConsoleReportsRejectCrossSiteAccess(t *testing.T) {
	store, appCtx, _, siteID, _ := setupSearchConsoleReportsTestEnv(t)
	seedSearchConsoleReportMapping(t, store, siteID)
	otherUserID, err := store.CreateUser(context.Background(), "other-gsc-reports@example.com", "hashed")
	if err != nil {
		t.Fatalf("CreateUser(other): %v", err)
	}
	otherSite, err := store.CreateSite(context.Background(), otherUserID, "other-reports.example.com")
	if err != nil {
		t.Fatalf("CreateSite(other): %v", err)
	}
	_, otherToken, err := store.CreateAPIClient(context.Background(), otherUserID, "Other Reports", "test", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		otherSite.ID: auth.SiteOwner,
	}, nil)
	if err != nil {
		t.Fatalf("CreateAPIClient(other): %v", err)
	}

	mux := http.NewServeMux()
	Register(mux, appCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/search-console/overview", nil)
	req.Header.Set("X-API-Key", otherToken)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
}

func TestSearchConsoleReportsDoNotRegisterShareRoutes(t *testing.T) {
	_, appCtx, _, siteID, _ := setupSearchConsoleReportsTestEnv(t)

	mux := http.NewServeMux()
	Register(mux, appCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/share/public-token/sites/"+siteID.String()+"/search-console/overview", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected no Search Console share route status %d, got %d body=%s", http.StatusNotFound, rec.Code, rec.Body.String())
	}
}

func TestSearchConsoleReportsReadTenantScopedFacts(t *testing.T) {
	ctx := context.Background()
	store, appCtx, userID, _, _ := setupSearchConsoleReportsTestEnv(t)
	tenantMgr := database.NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = tenantMgr.Close() })
	appCtx.TenantStores = tenantMgr
	team, err := store.CreateTenant(ctx, userID, "Tenant Reports", "")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, team.ID); err != nil {
		t.Fatalf("SetActiveTenantID: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "tenant-reports.example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}
	_, token, err := store.CreateAPIClient(ctx, userID, "Tenant Search Console Reports", "test", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		site.ID: auth.SiteOwner,
	}, nil)
	if err != nil {
		t.Fatalf("CreateAPIClient: %v", err)
	}
	seedSearchConsoleReportMapping(t, store, site.ID)
	sharedFact := database.SearchConsoleFactInput{
		SiteID:          site.ID,
		PropertyURI:     "sc-domain:reports.example.com",
		Date:            time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
		Query:           "shared stale",
		Page:            "https://tenant-reports.example.com/",
		Country:         "US",
		Device:          "DESKTOP",
		Clicks:          99,
		Impressions:     990,
		CTR:             0.1,
		Position:        9,
		AggregationType: "auto",
		DataState:       "final",
		ImportedAt:      time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
	}
	seedSearchConsoleReportFact(t, store, sharedFact)
	tenantStore, _, err := tenantMgr.ResolveSiteStore(ctx, site.ID)
	if err != nil {
		t.Fatalf("ResolveSiteStore: %v", err)
	}
	tenantFact := sharedFact
	tenantFact.Query = "tenant scoped"
	tenantFact.Clicks = 4
	tenantFact.Impressions = 40
	seedSearchConsoleReportFact(t, tenantStore, tenantFact)

	mux := http.NewServeMux()
	Register(mux, appCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/search-console/overview?from=2026-05-01T00:00:00Z&to=2026-05-03T00:00:00Z", nil)
	req.Header.Set("X-API-Key", token)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var overview api.SearchConsoleOverview
	if err := json.NewDecoder(rec.Body).Decode(&overview); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if overview.Clicks != 4 || overview.Impressions != 40 {
		t.Fatalf("expected tenant-scoped facts only, got %+v", overview)
	}
}

func setupSearchConsoleReportsTestEnv(t *testing.T) (*database.Store, *shared.Context, uuid.UUID, uuid.UUID, string) {
	t.Helper()
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	userID, err := store.CreateUser(context.Background(), "gsc-reports@example.com", "hashed")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "reports.example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}
	_, token, err := store.CreateAPIClient(context.Background(), userID, "Search Console Reports", "test", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		site.ID: auth.SiteOwner,
	}, nil)
	if err != nil {
		t.Fatalf("CreateAPIClient: %v", err)
	}

	appCtx := &shared.Context{
		Store:  store,
		Config: &config.Config{},
	}
	return store, appCtx, userID, site.ID, token
}

func seedSearchConsoleReportMapping(t *testing.T, store *database.Store, siteID uuid.UUID) uuid.UUID {
	t.Helper()
	teamID, err := store.GetSiteTenantID(context.Background(), siteID)
	if err != nil {
		t.Fatalf("GetSiteTenantID: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSiteMapping(context.Background(), database.GoogleSearchConsoleSiteMappingInput{
		SiteID:      siteID,
		TeamID:      teamID,
		PropertyURI: "sc-domain:reports.example.com",
		MappedBy:    uuid.New(),
		MappedAt:    time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed mapping: %v", err)
	}
	return teamID
}

func seedSearchConsoleReportFact(t *testing.T, store *database.Store, input database.SearchConsoleFactInput) {
	t.Helper()
	if err := store.UpsertSearchConsoleFact(context.Background(), input); err != nil {
		t.Fatalf("seed Search Console fact: %v", err)
	}
}
