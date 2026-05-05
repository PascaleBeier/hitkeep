package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/searchconsole"
)

func TestGoogleSearchConsoleStatusReportsMissingSelfHostedCredentials(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	h.ctx.Config.CloudHosted = false
	h.ctx.Config.GoogleSearchConsoleClientID = ""
	h.ctx.Config.GoogleSearchConsoleClientSecret = ""

	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams/"+teamID.String()+"/integrations/google-search-console/status", nil), userID)
	req.SetPathValue("id", teamID.String())
	w := httptest.NewRecorder()

	h.handleGetGoogleSearchConsoleStatus().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp api.GoogleSearchConsoleStatus
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "credentials_missing" {
		t.Fatalf("expected credentials_missing status, got %q", resp.Status)
	}
	if resp.Configured {
		t.Fatalf("expected configured=false")
	}
	if resp.CredentialStatus != "missing" {
		t.Fatalf("expected missing credential status, got %q", resp.CredentialStatus)
	}
	if strings.Contains(w.Body.String(), "client") || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("status response should not echo credential values or names: %s", w.Body.String())
	}
}

func TestGoogleSearchConsoleStatusRejectsUsersOutsideTeam(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	otherTeamID := uuid.New()
	if _, err := store.DB().ExecContext(context.Background(),
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, now())",
		otherTeamID, "Other Team",
	); err != nil {
		t.Fatalf("insert other team: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams/"+otherTeamID.String()+"/integrations/google-search-console/status", nil), userID)
	req.SetPathValue("id", otherTeamID.String())
	w := httptest.NewRecorder()

	h.handleGetGoogleSearchConsoleStatus().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
}

func TestGoogleSearchConsoleConnectReturnsStateBoundOAuthURL(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	h.ctx.Config.GoogleSearchConsoleClientID = "client-id"
	h.ctx.Config.GoogleSearchConsoleClientSecret = "client-secret"
	h.ctx.SearchConsole = &fakeSearchConsoleClient{}

	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+teamID.String()+"/integrations/google-search-console/connect", strings.NewReader(`{"return_path":"/integration/google-search-console"}`)), userID)
	req.SetPathValue("id", teamID.String())
	w := httptest.NewRecorder()

	h.handleConnectGoogleSearchConsole().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp api.GoogleSearchConsoleConnectResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(resp.AuthURL, "https://accounts.example.test/oauth") {
		t.Fatalf("expected fake OAuth URL, got %q", resp.AuthURL)
	}
	if !strings.Contains(resp.AuthURL, "state=") {
		t.Fatalf("expected OAuth URL to include state, got %q", resp.AuthURL)
	}
	if !strings.Contains(resp.AuthURL, "webmasters.readonly") {
		t.Fatalf("expected OAuth URL to include read-only Search Console scope, got %q", resp.AuthURL)
	}
}

func TestGoogleSearchConsolePropertiesUseConnectedTeamOnly(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	h.ctx.SearchConsole = &fakeSearchConsoleClient{
		properties: []searchconsole.Property{
			{URI: "sc-domain:example.com", PermissionLevel: "SITE_OWNER"},
			{URI: "https://www.example.com/", PermissionLevel: "SITE_FULL_USER"},
		},
	}

	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleConnection(context.Background(), database.GoogleSearchConsoleConnectionInput{
		TeamID:             teamID,
		ConnectedByUserID:  userID,
		GoogleAccountEmail: "owner@example.com",
		AccessToken:        "access-token",
		RefreshToken:       "refresh-token",
		TokenType:          "Bearer",
		Scope:              searchconsole.ReadOnlyScope,
		TokenExpiry:        time.Now().UTC().Add(time.Hour),
		ConnectedAt:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams/"+teamID.String()+"/integrations/google-search-console/properties", nil), userID)
	req.SetPathValue("id", teamID.String())
	w := httptest.NewRecorder()

	h.handleListGoogleSearchConsoleProperties().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp api.GoogleSearchConsolePropertiesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Properties) != 2 {
		t.Fatalf("expected two properties, got %+v", resp.Properties)
	}
	if resp.Properties[0].URI != "sc-domain:example.com" || resp.Properties[0].PermissionLevel != "SITE_OWNER" {
		t.Fatalf("unexpected first property: %+v", resp.Properties[0])
	}
	if h.ctx.SearchConsole.(*fakeSearchConsoleClient).listedToken.RefreshToken != "refresh-token" {
		t.Fatalf("expected property list to use connected team token")
	}
	entries := requireGoogleSearchConsoleAuditEntries(t, store, teamID, "google_search_console.properties_refreshed")
	if entries[0].Outcome != "success" {
		t.Fatalf("expected safe property refresh audit, got outcome=%q details=%q", entries[0].Outcome, entries[0].Details)
	}
	if strings.Contains(entries[0].Details, "access-token") || strings.Contains(entries[0].Details, "refresh-token") {
		t.Fatalf("property refresh audit leaked token material: %q", entries[0].Details)
	}
}

func TestGoogleSearchConsolePropertyListClassifiesDisabledAPI(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	h.ctx.SearchConsole = &fakeSearchConsoleClient{
		listErr: searchconsole.ClassifiedError(searchconsole.CategoryAPIDisabled, errors.New("provider details must stay out of the response")),
	}

	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleConnection(context.Background(), database.GoogleSearchConsoleConnectionInput{
		TeamID:             teamID,
		ConnectedByUserID:  userID,
		GoogleAccountEmail: "owner@example.com",
		AccessToken:        "access-token",
		RefreshToken:       "refresh-token",
		TokenType:          "Bearer",
		Scope:              searchconsole.ReadOnlyScope,
		TokenExpiry:        time.Now().UTC().Add(time.Hour),
		ConnectedAt:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams/"+teamID.String()+"/integrations/google-search-console/properties", nil), userID)
	req.SetPathValue("id", teamID.String())
	w := httptest.NewRecorder()

	h.handleListGoogleSearchConsoleProperties().ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadGateway, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":"api_disabled"`) {
		t.Fatalf("expected api_disabled response, got %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "provider details") {
		t.Fatalf("response leaked provider details: %s", w.Body.String())
	}
	entries := requireGoogleSearchConsoleAuditEntries(t, store, teamID, "google_search_console.properties_refresh_failed")
	if entries[0].Outcome != "failure" || !strings.Contains(entries[0].Details, "outcome=api_disabled") {
		t.Fatalf("expected api_disabled failure audit, got outcome=%q details=%q", entries[0].Outcome, entries[0].Details)
	}
	if strings.Contains(entries[0].Details, "provider details") {
		t.Fatalf("audit leaked provider details: %q", entries[0].Details)
	}
}

func TestGoogleSearchConsoleCallbackStoresConnectionAndAudit(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	h.ctx.Config.GoogleSearchConsoleClientID = "client-id"
	h.ctx.Config.GoogleSearchConsoleClientSecret = "client-secret"
	tokenExpiry := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	h.ctx.SearchConsole = &fakeSearchConsoleClient{
		token: searchconsole.Token{
			AccessToken:        "access-token",
			RefreshToken:       "refresh-token",
			TokenType:          "Bearer",
			Scope:              searchconsole.ReadOnlyScope,
			Expiry:             tokenExpiry,
			GoogleAccountEmail: "owner@example.com",
			GoogleAccountID:    "google-account",
		},
	}

	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	state := h.ctx.AuthState.CreateGoogleSearchConsoleOAuthState(userID, teamID, "/integration/google-search-console", time.Now().UTC().Add(time.Minute))

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/integrations/google-search-console/oauth/callback?state="+state+"&code=oauth-code", nil), userID)
	w := httptest.NewRecorder()

	h.handleGoogleSearchConsoleCallback().ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusFound, w.Code, w.Body.String())
	}
	if location := w.Header().Get("Location"); location != "/integration/google-search-console" {
		t.Fatalf("expected redirect to integration page, got %q", location)
	}

	conn := requireGoogleSearchConsoleConnection(t, store, teamID)
	if conn.GoogleAccountEmail != "owner@example.com" {
		t.Fatalf("expected account label, got %q", conn.GoogleAccountEmail)
	}
	if conn.AccessToken != "access-token" || conn.RefreshToken != "refresh-token" {
		t.Fatalf("expected token material to be stored")
	}

	entries := requireGoogleSearchConsoleAuditEntries(t, store, teamID, "google_search_console.connected")
	if entries[0].Outcome != "success" {
		t.Fatalf("expected success audit outcome, got %q", entries[0].Outcome)
	}
	if strings.Contains(entries[0].Details, "access-token") || strings.Contains(entries[0].Details, "refresh-token") || strings.Contains(entries[0].Details, "oauth-code") {
		t.Fatalf("audit details leaked token/code material: %q", entries[0].Details)
	}
}

func TestGoogleSearchConsoleAdminCanMapAndReadSiteProperty(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	teamID, site := seedGoogleSearchConsoleMappableSite(t, store, userID, "map-site.example.com", "sc-domain:example.com")
	mux := googleSearchConsoleUserMux(h)

	body := strings.NewReader(`{"property_uri":"sc-domain:example.com"}`)
	mapReq := withTestUser(httptest.NewRequest(http.MethodPut, "/api/sites/"+site.ID.String()+"/integrations/google-search-console/property", body), userID)
	mapW := httptest.NewRecorder()
	mux.ServeHTTP(mapW, mapReq)
	if mapW.Code != http.StatusOK {
		t.Fatalf("expected map status %d, got %d: %s", http.StatusOK, mapW.Code, mapW.Body.String())
	}

	var mapResp api.GoogleSearchConsoleSiteMappingResponse
	if err := json.NewDecoder(mapW.Body).Decode(&mapResp); err != nil {
		t.Fatalf("decode map response: %v", err)
	}
	if !mapResp.Mapped || mapResp.PropertyURI != "sc-domain:example.com" || mapResp.TeamID != teamID {
		t.Fatalf("unexpected mapping response: %+v", mapResp)
	}

	getReq := withTestUser(httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/integrations/google-search-console", nil), userID)
	getW := httptest.NewRecorder()
	mux.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("expected read status %d, got %d: %s", http.StatusOK, getW.Code, getW.Body.String())
	}
	var getResp api.GoogleSearchConsoleSiteMappingResponse
	if err := json.NewDecoder(getW.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if !getResp.Mapped || getResp.PropertyURI != "sc-domain:example.com" {
		t.Fatalf("expected active mapping, got %+v", getResp)
	}

	entries := requireGoogleSearchConsoleAuditEntries(t, store, teamID, "google_search_console.property_mapped")
	if entries[0].Outcome != "success" || !strings.Contains(entries[0].Details, "new_property_uri=sc-domain:example.com") {
		t.Fatalf("expected safe map audit details, got outcome=%q details=%q", entries[0].Outcome, entries[0].Details)
	}
	if strings.Contains(entries[0].Details, "access-token") || strings.Contains(entries[0].Details, "refresh-token") {
		t.Fatalf("audit details leaked token material: %q", entries[0].Details)
	}
}

func TestGoogleSearchConsoleViewerCannotMapOrUnmapSiteProperty(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()
	_, site := seedGoogleSearchConsoleMappableSite(t, store, ownerID, "viewer-map.example.com", "sc-domain:viewer-map.example.com")
	viewerID, err := store.CreateUser(context.Background(), "viewer@example.com", "hashed")
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := store.AddSiteMember(context.Background(), site.ID, viewerID, authcore.SiteViewer, ownerID); err != nil {
		t.Fatalf("add viewer site member: %v", err)
	}
	mux := googleSearchConsoleUserMux(h)

	mapReq := withTestUser(httptest.NewRequest(http.MethodPut, "/api/sites/"+site.ID.String()+"/integrations/google-search-console/property", strings.NewReader(`{"property_uri":"sc-domain:viewer-map.example.com"}`)), viewerID)
	mapW := httptest.NewRecorder()
	mux.ServeHTTP(mapW, mapReq)
	if mapW.Code != http.StatusForbidden {
		t.Fatalf("expected map status %d, got %d: %s", http.StatusForbidden, mapW.Code, mapW.Body.String())
	}

	deleteReq := withTestUser(httptest.NewRequest(http.MethodDelete, "/api/sites/"+site.ID.String()+"/integrations/google-search-console/property", nil), viewerID)
	deleteW := httptest.NewRecorder()
	mux.ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusForbidden {
		t.Fatalf("expected unmap status %d, got %d: %s", http.StatusForbidden, deleteW.Code, deleteW.Body.String())
	}
}

func TestGoogleSearchConsoleMappingRejectsSiteOutsideActiveTeam(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	_, site := seedGoogleSearchConsoleMappableSite(t, store, userID, "wrong-team.example.com", "sc-domain:wrong-team.example.com")
	otherTeamID := uuid.New()
	if _, err := store.DB().ExecContext(context.Background(), "INSERT INTO tenants (id, name, created_at) VALUES (?, ?, now())", otherTeamID, "Other Team"); err != nil {
		t.Fatalf("insert other team: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), otherTeamID, userID, database.TenantRoleAdmin, userID); err != nil {
		t.Fatalf("add other team member: %v", err)
	}
	if err := store.SetActiveTenantID(context.Background(), userID, otherTeamID); err != nil {
		t.Fatalf("set active team: %v", err)
	}
	mux := googleSearchConsoleUserMux(h)

	req := withTestUser(httptest.NewRequest(http.MethodPut, "/api/sites/"+site.ID.String()+"/integrations/google-search-console/property", strings.NewReader(`{"property_uri":"sc-domain:wrong-team.example.com"}`)), userID)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected wrong-team status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
}

func TestGoogleSearchConsoleMappingRequiresConnectedTeam(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	teamID, site := seedGoogleSearchConsoleMappableSite(t, store, userID, "disconnected-map.example.com", "sc-domain:disconnected-map.example.com")
	if err := store.DisconnectGoogleSearchConsoleConnection(context.Background(), teamID, time.Now().UTC()); err != nil {
		t.Fatalf("disconnect Search Console: %v", err)
	}
	mux := googleSearchConsoleUserMux(h)

	req := withTestUser(httptest.NewRequest(http.MethodPut, "/api/sites/"+site.ID.String()+"/integrations/google-search-console/property", strings.NewReader(`{"property_uri":"sc-domain:disconnected-map.example.com"}`)), userID)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected disconnected status %d, got %d: %s", http.StatusPreconditionFailed, w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "access-token") || strings.Contains(w.Body.String(), "refresh-token") {
		t.Fatalf("disconnected response leaked token material: %q", w.Body.String())
	}
}

func TestGoogleSearchConsoleMappingIgnoresStaleMappingFromPreviousTeam(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	currentTeamID, site := seedGoogleSearchConsoleMappableSite(t, store, userID, "stale-map.example.com", "sc-domain:current-team.example.com")
	seedStaleGoogleSearchConsoleMapping(t, store, site.ID, userID)
	mux := googleSearchConsoleUserMux(h)

	requireGoogleSearchConsoleSiteUnmapped(t, mux, site.ID, userID, currentTeamID)

	mapReq := withTestUser(httptest.NewRequest(http.MethodPut, "/api/sites/"+site.ID.String()+"/integrations/google-search-console/property", strings.NewReader(`{"property_uri":"sc-domain:current-team.example.com"}`)), userID)
	mapW := httptest.NewRecorder()
	mux.ServeHTTP(mapW, mapReq)
	if mapW.Code != http.StatusOK {
		t.Fatalf("expected map status %d, got %d: %s", http.StatusOK, mapW.Code, mapW.Body.String())
	}
	entries := requireGoogleSearchConsoleAuditEntries(t, store, currentTeamID, "google_search_console.property_mapped")
	if strings.Contains(entries[0].Details, "previous-team") {
		t.Fatalf("mapping audit leaked previous-team property URI: %q", entries[0].Details)
	}
}

func TestGoogleSearchConsoleUnmapAuditsOldPropertyWithoutSecrets(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	teamID, site := seedGoogleSearchConsoleMappableSite(t, store, userID, "unmap-site.example.com", "sc-domain:unmap-site.example.com")
	if err := store.UpsertGoogleSearchConsoleSiteMapping(context.Background(), database.GoogleSearchConsoleSiteMappingInput{
		SiteID:      site.ID,
		TeamID:      teamID,
		PropertyURI: "sc-domain:unmap-site.example.com",
		MappedBy:    userID,
		MappedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed mapping: %v", err)
	}
	mux := googleSearchConsoleUserMux(h)

	req := withTestUser(httptest.NewRequest(http.MethodDelete, "/api/sites/"+site.ID.String()+"/integrations/google-search-console/property", nil), userID)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected unmap status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	entries := requireGoogleSearchConsoleAuditEntries(t, store, teamID, "google_search_console.property_unmapped")
	if entries[0].Outcome != "success" || !strings.Contains(entries[0].Details, "old_property_uri=sc-domain:unmap-site.example.com") {
		t.Fatalf("expected safe unmap audit details, got outcome=%q details=%q", entries[0].Outcome, entries[0].Details)
	}
	if strings.Contains(entries[0].Details, "access-token") || strings.Contains(entries[0].Details, "refresh-token") {
		t.Fatalf("audit details leaked token material: %q", entries[0].Details)
	}
}

func TestGoogleSearchConsoleManualSyncRunsImmediatelyAndAudits(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	h.ctx.SearchConsole = &fakeSearchConsoleClient{
		rows: []searchconsole.SearchAnalyticsRow{
			{
				Date:            time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
				Query:           "hitkeep analytics",
				Page:            "https://manual-sync.example.com/",
				Country:         "usa",
				Device:          "desktop",
				Clicks:          7,
				Impressions:     70,
				CTR:             0.1,
				Position:        3.4,
				AggregationType: "auto",
				DataState:       searchconsole.DataStateFinal,
			},
		},
	}
	teamID, site := seedGoogleSearchConsoleMappableSite(t, store, userID, "manual-sync.example.com", "sc-domain:manual-sync.example.com")
	if err := store.UpsertGoogleSearchConsoleSiteMapping(context.Background(), database.GoogleSearchConsoleSiteMappingInput{
		SiteID:      site.ID,
		TeamID:      teamID,
		PropertyURI: "sc-domain:manual-sync.example.com",
		MappedBy:    userID,
		MappedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed mapping: %v", err)
	}
	importedStart, _, lastSuccess := seedSucceededGoogleSearchConsoleSyncState(t, store, site.ID, teamID)
	mux := googleSearchConsoleUserMux(h)

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/integrations/google-search-console/sync", nil), userID)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected sync status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp api.GoogleSearchConsoleSiteMappingResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	if resp.SyncStatus == nil || resp.SyncStatus.State != "succeeded" || resp.SyncStatus.Manual {
		t.Fatalf("expected immediate successful sync status, got %+v", resp.SyncStatus)
	}
	state, err := store.GetGoogleSearchConsoleSyncState(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("get sync state: %v", err)
	}
	if state == nil || state.State != "succeeded" || state.Manual {
		t.Fatalf("expected immediate successful sync state, got %+v", state)
	}
	if !state.ImportedStartDate.Equal(importedStart) {
		t.Fatalf("expected imported start %s, got %+v", importedStart, state)
	}
	if state.ImportedEndDate == nil || state.ImportedEndDate.Before(importedStart) {
		t.Fatalf("expected imported end to cover immediate sync window, got %+v", state)
	}
	if state.LastSuccessAt == nil || !state.LastSuccessAt.After(lastSuccess) {
		t.Fatalf("expected last success to refresh after immediate sync, got %+v", state)
	}
	entries := requireGoogleSearchConsoleAuditEntries(t, store, teamID, "google_search_console.sync_requested")
	if entries[0].Outcome != "success" || strings.Contains(entries[0].Details, "access-token") || strings.Contains(entries[0].Details, "refresh-token") {
		t.Fatalf("expected safe sync audit, got outcome=%q details=%q", entries[0].Outcome, entries[0].Details)
	}
	_ = requireGoogleSearchConsoleAuditEntries(t, store, teamID, "google_search_console.sync_started")
	_ = requireGoogleSearchConsoleAuditEntries(t, store, teamID, "google_search_console.sync_imported")
	tenantStore, _, err := h.ctx.TenantStores.ResolveSiteStore(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("resolve tenant store: %v", err)
	}
	overview, err := tenantStore.GetSearchConsoleOverview(context.Background(), api.SearchConsoleReportParams{
		SiteID:      site.ID,
		PropertyURI: "sc-domain:manual-sync.example.com",
		Start:       time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		End:         time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("get imported overview: %v", err)
	}
	if overview.Clicks != 7 || overview.Impressions != 70 {
		t.Fatalf("expected imported facts, got %+v", overview)
	}
}

func TestGoogleSearchConsoleCallbackRejectsUserWhoLostTeamManagerRole(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	h.ctx.Config.GoogleSearchConsoleClientID = "client-id"
	h.ctx.Config.GoogleSearchConsoleClientSecret = "client-secret"
	h.ctx.SearchConsole = &fakeSearchConsoleClient{
		token: searchconsole.Token{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			TokenType:    "Bearer",
			Scope:        searchconsole.ReadOnlyScope,
			Expiry:       time.Now().UTC().Add(time.Hour),
		},
	}

	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	state := h.ctx.AuthState.CreateGoogleSearchConsoleOAuthState(userID, teamID, "/integration/google-search-console", time.Now().UTC().Add(time.Minute))
	if _, err := store.DB().ExecContext(context.Background(), "UPDATE tenant_members SET role = ? WHERE tenant_id = ? AND user_id = ?", database.TenantRoleMember, teamID, userID); err != nil {
		t.Fatalf("demote team role: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/integrations/google-search-console/oauth/callback?state="+state+"&code=oauth-code", nil), userID)
	w := httptest.NewRecorder()

	h.handleGoogleSearchConsoleCallback().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
	conn, err := store.GetGoogleSearchConsoleConnection(context.Background(), teamID)
	if err != nil {
		t.Fatalf("get connection: %v", err)
	}
	if conn != nil {
		t.Fatalf("expected no connection after permission loss, got %+v", conn)
	}
	entries := requireGoogleSearchConsoleAuditEntries(t, store, teamID, "google_search_console.connect_failed")
	if entries[0].Outcome != "failure" || !strings.Contains(entries[0].Details, "permission_lost") {
		t.Fatalf("expected permission_lost failure audit, got outcome=%q details=%q", entries[0].Outcome, entries[0].Details)
	}
}

func TestGoogleSearchConsoleCallbackRejectsMissingRefreshToken(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	h.ctx.Config.GoogleSearchConsoleClientID = "client-id"
	h.ctx.Config.GoogleSearchConsoleClientSecret = "client-secret"
	h.ctx.SearchConsole = &fakeSearchConsoleClient{
		token: searchconsole.Token{
			AccessToken: "access-token",
			TokenType:   "Bearer",
			Scope:       searchconsole.ReadOnlyScope,
			Expiry:      time.Now().UTC().Add(time.Hour),
		},
	}

	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	state := h.ctx.AuthState.CreateGoogleSearchConsoleOAuthState(userID, teamID, "/integration/google-search-console", time.Now().UTC().Add(time.Minute))

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/integrations/google-search-console/oauth/callback?state="+state+"&code=oauth-code", nil), userID)
	w := httptest.NewRecorder()

	h.handleGoogleSearchConsoleCallback().ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadGateway, w.Code, w.Body.String())
	}
	conn, err := store.GetGoogleSearchConsoleConnection(context.Background(), teamID)
	if err != nil {
		t.Fatalf("get connection: %v", err)
	}
	if conn != nil {
		t.Fatalf("expected no connection without refresh token, got %+v", conn)
	}
	entries := requireGoogleSearchConsoleAuditEntries(t, store, teamID, "google_search_console.connect_failed")
	if entries[0].Outcome != "failure" || !strings.Contains(entries[0].Details, "refresh_token_missing") {
		t.Fatalf("expected refresh_token_missing failure audit, got outcome=%q details=%q", entries[0].Outcome, entries[0].Details)
	}
}

func TestGoogleSearchConsoleConnectAuditsMissingCredentials(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	h.ctx.Config.GoogleSearchConsoleClientID = ""
	h.ctx.Config.GoogleSearchConsoleClientSecret = ""

	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+teamID.String()+"/integrations/google-search-console/connect", strings.NewReader(`{}`)), userID)
	req.SetPathValue("id", teamID.String())
	w := httptest.NewRecorder()

	h.handleConnectGoogleSearchConsole().ServeHTTP(w, req)
	if w.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected status %d, got %d: %s", http.StatusPreconditionFailed, w.Code, w.Body.String())
	}
	entries := requireGoogleSearchConsoleAuditEntries(t, store, teamID, "google_search_console.connect_failed")
	if entries[0].Outcome != "failure" || !strings.Contains(entries[0].Details, "credentials_missing") {
		t.Fatalf("expected credentials_missing failure audit, got outcome=%q details=%q", entries[0].Outcome, entries[0].Details)
	}
}

func TestGoogleSearchConsoleCallbackAuditsExchangeFailureOutcome(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	h.ctx.Config.GoogleSearchConsoleClientID = "client-id"
	h.ctx.Config.GoogleSearchConsoleClientSecret = "client-secret"
	h.ctx.SearchConsole = &fakeSearchConsoleClient{err: errors.New("token endpoint unavailable")}

	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	state := h.ctx.AuthState.CreateGoogleSearchConsoleOAuthState(userID, teamID, "/integration/google-search-console", time.Now().UTC().Add(time.Minute))

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/integrations/google-search-console/oauth/callback?state="+state+"&code=oauth-code", nil), userID)
	w := httptest.NewRecorder()

	h.handleGoogleSearchConsoleCallback().ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadGateway, w.Code, w.Body.String())
	}
	entries := requireGoogleSearchConsoleAuditEntries(t, store, teamID, "google_search_console.connect_failed")
	if entries[0].Outcome != "failure" || !strings.Contains(entries[0].Details, "exchange_failed") {
		t.Fatalf("expected exchange_failed failure audit, got outcome=%q details=%q", entries[0].Outcome, entries[0].Details)
	}
}

func TestGoogleSearchConsoleDisconnectClearsTokensAndAudits(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleConnection(context.Background(), database.GoogleSearchConsoleConnectionInput{
		TeamID:             teamID,
		ConnectedByUserID:  userID,
		GoogleAccountEmail: "owner@example.com",
		AccessToken:        "access-token",
		RefreshToken:       "refresh-token",
		TokenType:          "Bearer",
		Scope:              searchconsole.ReadOnlyScope,
		TokenExpiry:        time.Now().UTC().Add(time.Hour),
		ConnectedAt:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	req := withTestUser(httptest.NewRequest(http.MethodDelete, "/api/user/teams/"+teamID.String()+"/integrations/google-search-console", nil), userID)
	req.SetPathValue("id", teamID.String())
	w := httptest.NewRecorder()

	h.handleDisconnectGoogleSearchConsole().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	conn, err := store.GetGoogleSearchConsoleConnection(context.Background(), teamID)
	if err != nil {
		t.Fatalf("get connection: %v", err)
	}
	if conn == nil || conn.Connected {
		t.Fatalf("expected disconnected metadata, got %+v", conn)
	}
	if conn.AccessToken != "" || conn.RefreshToken != "" {
		t.Fatalf("expected token material cleared on disconnect")
	}

	_ = requireGoogleSearchConsoleAuditEntries(t, store, teamID, "google_search_console.disconnected")
}

type fakeSearchConsoleClient struct {
	token       searchconsole.Token
	err         error
	listErr     error
	properties  []searchconsole.Property
	listedToken searchconsole.Token
	rows        []searchconsole.SearchAnalyticsRow
	queryErr    error
}

func (c *fakeSearchConsoleClient) AuthCodeURL(state, redirectURL string) (string, error) {
	return "https://accounts.example.test/oauth?scope=" + searchconsole.ReadOnlyScope + "&state=" + state + "&redirect_uri=" + redirectURL, nil
}

func (c *fakeSearchConsoleClient) ExchangeCode(ctx context.Context, code, redirectURL string) (searchconsole.Token, error) {
	if c.err != nil {
		return searchconsole.Token{}, c.err
	}
	return c.token, nil
}

func (c *fakeSearchConsoleClient) ListProperties(ctx context.Context, token searchconsole.Token) ([]searchconsole.Property, error) {
	c.listedToken = token
	if c.listErr != nil {
		return nil, c.listErr
	}
	return c.properties, nil
}

func (c *fakeSearchConsoleClient) QuerySearchAnalytics(ctx context.Context, token searchconsole.Token, query searchconsole.SearchAnalyticsQuery) ([]searchconsole.SearchAnalyticsRow, error) {
	if c.queryErr != nil {
		return nil, c.queryErr
	}
	return c.rows, nil
}

func requireGoogleSearchConsoleConnection(t *testing.T, store *database.Store, teamID uuid.UUID) *database.GoogleSearchConsoleConnection {
	t.Helper()
	conn, err := store.GetGoogleSearchConsoleConnection(context.Background(), teamID)
	if err != nil {
		t.Fatalf("get connection: %v", err)
	}
	if conn == nil || !conn.Connected {
		t.Fatalf("expected connected connection, got %+v", conn)
	}
	return conn
}

func requireGoogleSearchConsoleAuditEntries(t *testing.T, store *database.Store, teamID uuid.UUID, action string) []api.TeamAuditEntry {
	t.Helper()
	entries, _, err := store.ListTeamAuditEntries(context.Background(), teamID, action, 5, 0)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one %s audit entry, got %d", action, len(entries))
	}
	return entries
}

func seedGoogleSearchConsoleMappableSite(t *testing.T, store *database.Store, userID uuid.UUID, domain, propertyURI string) (uuid.UUID, *api.Site) {
	t.Helper()
	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, domain)
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleConnection(context.Background(), database.GoogleSearchConsoleConnectionInput{
		TeamID:             teamID,
		ConnectedByUserID:  userID,
		GoogleAccountEmail: "owner@example.com",
		AccessToken:        "access-token",
		RefreshToken:       "refresh-token",
		TokenType:          "Bearer",
		Scope:              searchconsole.ReadOnlyScope,
		TokenExpiry:        time.Now().UTC().Add(time.Hour),
		ConnectedAt:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleProperty(context.Background(), database.GoogleSearchConsolePropertyInput{
		TeamID:          teamID,
		URI:             propertyURI,
		PermissionLevel: "SITE_OWNER",
		SeenAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed property: %v", err)
	}
	return teamID, site
}

func seedStaleGoogleSearchConsoleMapping(t *testing.T, store *database.Store, siteID, userID uuid.UUID) {
	t.Helper()
	previousTeamID := uuid.New()
	if _, err := store.DB().ExecContext(context.Background(), "INSERT INTO tenants (id, name, created_at) VALUES (?, ?, now())", previousTeamID, "Previous Team"); err != nil {
		t.Fatalf("insert previous team: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSiteMapping(context.Background(), database.GoogleSearchConsoleSiteMappingInput{
		SiteID:      siteID,
		TeamID:      previousTeamID,
		PropertyURI: "sc-domain:previous-team.example.com",
		MappedBy:    userID,
		MappedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed stale mapping: %v", err)
	}
}

func seedSucceededGoogleSearchConsoleSyncState(t *testing.T, store *database.Store, siteID, teamID uuid.UUID) (time.Time, time.Time, time.Time) {
	t.Helper()
	importedStart := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	importedEnd := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	lastSuccess := time.Date(2026, 5, 4, 2, 0, 0, 0, time.UTC)
	if err := store.UpsertGoogleSearchConsoleSyncState(context.Background(), database.GoogleSearchConsoleSyncStateInput{
		SiteID:            siteID,
		TeamID:            teamID,
		State:             "succeeded",
		ImportedStartDate: &importedStart,
		ImportedEndDate:   &importedEnd,
		LastSuccessAt:     &lastSuccess,
	}); err != nil {
		t.Fatalf("seed sync status: %v", err)
	}
	return importedStart, importedEnd, lastSuccess
}

func requireGoogleSearchConsoleSiteUnmapped(t *testing.T, mux *http.ServeMux, siteID, userID, teamID uuid.UUID) {
	t.Helper()
	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/integrations/google-search-console", nil), userID)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected read status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
	var resp api.GoogleSearchConsoleSiteMappingResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if resp.Mapped || resp.PropertyURI != "" || resp.TeamID != teamID {
		t.Fatalf("expected site to be unmapped for team %s, got %+v", teamID, resp)
	}
}

func googleSearchConsoleUserMux(h *handler) *http.ServeMux {
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	return mux
}
