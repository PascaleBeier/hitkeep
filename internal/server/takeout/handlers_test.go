package takeout

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	takeoutsvc "hitkeep/internal/takeout"
)

type takeoutSentinel struct {
	RecordType string
	Path       string
}

func TestHandleUserTakeoutSupportsAllFormats(t *testing.T) {
	h, store, userID, _ := setupTakeoutHandlerTestEnv(t)
	t.Cleanup(func() { _ = store.Close() })

	tests := []struct {
		name            string
		queryFormat     string
		expectedExt     string
		expectedType    string
		expectedStatus  int
		withAuthContext bool
	}{
		{name: "csv", queryFormat: "csv", expectedExt: ".csv", expectedType: "text/csv; charset=utf-8", expectedStatus: http.StatusOK, withAuthContext: true},
		{name: "xlsx", queryFormat: "xlsx", expectedExt: ".xlsx", expectedType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", expectedStatus: http.StatusOK, withAuthContext: true},
		{name: "parquet", queryFormat: "parquet", expectedExt: ".parquet", expectedType: "application/octet-stream", expectedStatus: http.StatusOK, withAuthContext: true},
		{name: "json", queryFormat: "json", expectedExt: ".json", expectedType: "application/json; charset=utf-8", expectedStatus: http.StatusOK, withAuthContext: true},
		{name: "ndjson", queryFormat: "ndjson", expectedExt: ".ndjson", expectedType: "application/x-ndjson; charset=utf-8", expectedStatus: http.StatusOK, withAuthContext: true},
		{name: "unknown defaults to xlsx", queryFormat: "xml", expectedExt: ".xlsx", expectedType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", expectedStatus: http.StatusOK, withAuthContext: true},
		{name: "unauthorized", queryFormat: "csv", expectedStatus: http.StatusUnauthorized, withAuthContext: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/user/takeout?format="+tc.queryFormat, nil)
			if tc.withAuthContext {
				req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
			}

			w := httptest.NewRecorder()
			h.handleUserTakeout().ServeHTTP(w, req)

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
				t.Fatalf("expected non-empty takeout response body")
			}
			assertTakeoutResponseContainsSentinel(t, store, w.Body.Bytes(), exportfmt.Normalize(tc.queryFormat, exportfmt.FormatXLSX), takeoutSentinel{
				RecordType: "hit",
				Path:       "/takeout",
			})
		})
	}
}

func TestHandleSiteTakeoutSupportsAllFormats(t *testing.T) {
	h, store, _, siteID := setupTakeoutHandlerTestEnv(t)
	t.Cleanup(func() { _ = store.Close() })

	tests := []struct {
		name           string
		siteID         string
		queryFormat    string
		expectedExt    string
		expectedType   string
		expectedStatus int
	}{
		{name: "csv", siteID: siteID.String(), queryFormat: "csv", expectedExt: ".csv", expectedType: "text/csv; charset=utf-8", expectedStatus: http.StatusOK},
		{name: "xlsx", siteID: siteID.String(), queryFormat: "xlsx", expectedExt: ".xlsx", expectedType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", expectedStatus: http.StatusOK},
		{name: "parquet", siteID: siteID.String(), queryFormat: "parquet", expectedExt: ".parquet", expectedType: "application/octet-stream", expectedStatus: http.StatusOK},
		{name: "json", siteID: siteID.String(), queryFormat: "json", expectedExt: ".json", expectedType: "application/json; charset=utf-8", expectedStatus: http.StatusOK},
		{name: "ndjson", siteID: siteID.String(), queryFormat: "ndjson", expectedExt: ".ndjson", expectedType: "application/x-ndjson; charset=utf-8", expectedStatus: http.StatusOK},
		{name: "unknown defaults to xlsx", siteID: siteID.String(), queryFormat: "xml", expectedExt: ".xlsx", expectedType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", expectedStatus: http.StatusOK},
		{name: "invalid site id", siteID: "invalid-uuid", queryFormat: "csv", expectedStatus: http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/sites/"+tc.siteID+"/takeout?format="+tc.queryFormat, nil)
			req.SetPathValue("id", tc.siteID)

			w := httptest.NewRecorder()
			h.handleSiteTakeout().ServeHTTP(w, req)

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
				t.Fatalf("expected non-empty takeout response body")
			}
			assertTakeoutResponseContainsSentinel(t, store, w.Body.Bytes(), exportfmt.Normalize(tc.queryFormat, exportfmt.FormatXLSX), takeoutSentinel{
				RecordType: "hit",
				Path:       "/takeout",
			})
		})
	}
}

func TestRegisteredSiteTakeoutRequiresSiteViewPermission(t *testing.T) {
	mux, store, viewerToken, noSiteAccessToken, siteID := setupTakeoutRouteTestEnv(t)
	t.Cleanup(func() { _ = store.Close() })

	viewerResp := requestSiteTakeout(t, mux, siteID, viewerToken)
	if viewerResp.Code != http.StatusOK {
		t.Fatalf("expected viewer status %d, got %d (body: %s)", http.StatusOK, viewerResp.Code, viewerResp.Body.String())
	}
	if viewerResp.Body.Len() == 0 {
		t.Fatalf("expected non-empty takeout response body")
	}

	noAccessResp := requestSiteTakeout(t, mux, siteID, noSiteAccessToken)
	if noAccessResp.Code != http.StatusForbidden {
		t.Fatalf("expected no-site-access status %d, got %d (body: %s)", http.StatusForbidden, noAccessResp.Code, noAccessResp.Body.String())
	}

	unauthenticatedResp := requestSiteTakeout(t, mux, siteID, "")
	if unauthenticatedResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated status %d, got %d (body: %s)", http.StatusUnauthorized, unauthenticatedResp.Code, unauthenticatedResp.Body.String())
	}
}

func TestContentTypeForExportUsesSharedMappings(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{filename: "export.csv", want: exportfmt.ContentType(exportfmt.FormatCSV)},
		{filename: "export.xlsx", want: exportfmt.ContentType(exportfmt.FormatXLSX)},
		{filename: "export.parquet", want: exportfmt.ContentType(exportfmt.FormatParquet)},
		{filename: "export.json", want: exportfmt.ContentType(exportfmt.FormatJSON)},
		{filename: "export.ndjson", want: exportfmt.ContentType(exportfmt.FormatNDJSON)},
		{filename: "export.bin", want: "application/octet-stream"},
	}

	for _, tc := range tests {
		if got := exportfmt.ContentTypeForFilename(tc.filename); got != tc.want {
			t.Fatalf("expected content type %q for filename %q, got %q", tc.want, tc.filename, got)
		}
	}
}

func setupTakeoutHandlerTestEnv(t *testing.T) (*TakeoutHandler, *database.Store, uuid.UUID, uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	userID, err := store.CreateUser(ctx, "takeout-handler@example.com", "hash")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "takeout-handler.test")
	if err != nil {
		t.Fatalf("failed to create test site: %v", err)
	}

	now := time.Now().UTC()
	isUnique := true
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:      site.ID,
		SessionID:   uuid.New(),
		PageID:      uuid.New(),
		Timestamp:   now,
		Path:        "/takeout",
		UTMSource:   new("newsletter"),
		UTMMedium:   new("email"),
		UTMCampaign: new("launch"),
		UTMTerm:     new("term"),
		UTMContent:  new("cta"),
		IsUnique:    &isUnique,
	}); err != nil {
		t.Fatalf("failed to seed hit: %v", err)
	}

	service := takeoutsvc.NewTakeoutService(store, filepath.Join(t.TempDir(), "exports"))
	return NewTakeoutHandler(service), store, userID, site.ID
}

func setupTakeoutRouteTestEnv(t *testing.T) (*http.ServeMux, *database.Store, string, string, uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	userID, err := store.CreateUser(ctx, "takeout-route-owner@example.com", "hash")
	if err != nil {
		t.Fatalf("failed to create owner user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "takeout-route.test")
	if err != nil {
		t.Fatalf("failed to create test site: %v", err)
	}

	now := time.Now().UTC()
	isUnique := true
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: now,
		Path:      "/route-takeout",
		IsUnique:  &isUnique,
	}); err != nil {
		t.Fatalf("failed to seed hit: %v", err)
	}

	_, viewerToken, err := store.CreateAPIClient(ctx, userID, "Viewer Takeout", "test", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
		site.ID: auth.SiteViewer,
	}, nil)
	if err != nil {
		t.Fatalf("failed to create viewer api client: %v", err)
	}

	_, noSiteAccessToken, err := store.CreateAPIClient(ctx, userID, "No Site Takeout", "test", auth.InstanceUser, nil, nil)
	if err != nil {
		t.Fatalf("failed to create no-site-access api client: %v", err)
	}

	appCtx := &shared.Context{
		Store:   store,
		Config:  &config.Config{},
		Takeout: takeoutsvc.NewTakeoutService(store, filepath.Join(t.TempDir(), "exports")),
	}

	mux := http.NewServeMux()
	Register(mux, appCtx)
	return mux, store, viewerToken, noSiteAccessToken, site.ID
}

func requestSiteTakeout(t *testing.T, mux *http.ServeMux, siteID uuid.UUID, token string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/takeout?format=csv", nil)
	if token != "" {
		req.Header.Set("X-Api-Key", token)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func assertTakeoutResponseContainsSentinel(t *testing.T, store *database.Store, body []byte, format string, sentinel takeoutSentinel) {
	t.Helper()
	if len(body) == 0 {
		t.Fatalf("expected non-empty takeout response body")
	}

	filename := filepath.Join(t.TempDir(), "takeout-response."+format)
	if err := os.WriteFile(filename, body, 0600); err != nil {
		t.Fatalf("write takeout response fixture: %v", err)
	}

	switch format {
	case exportfmt.FormatCSV:
		assertCSVTakeoutBodyContainsSentinel(t, body, sentinel)
	case exportfmt.FormatJSON:
		assertJSONTakeoutBodyContainsSentinel(t, body, sentinel)
	case exportfmt.FormatNDJSON:
		assertNDJSONTakeoutBodyContainsSentinel(t, body, sentinel)
	case exportfmt.FormatParquet:
		assertParquetTakeoutFileContainsSentinel(t, store, filename, sentinel)
	case exportfmt.FormatXLSX:
		assertXLSXTakeoutFileContainsSentinel(t, filename, sentinel)
	default:
		t.Fatalf("unsupported takeout format %q", format)
	}
}

func assertCSVTakeoutBodyContainsSentinel(t *testing.T, body []byte, sentinel takeoutSentinel) {
	t.Helper()
	rows, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	if err != nil {
		t.Fatalf("read csv takeout response: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected csv takeout header and data row, got %d rows", len(rows))
	}

	index := takeoutHeaderIndex(t, rows[0], "record_type", "path")
	for _, row := range rows[1:] {
		if row[index["record_type"]] == sentinel.RecordType && row[index["path"]] == sentinel.Path {
			return
		}
	}
	t.Fatalf("expected csv takeout response to contain %s path %q", sentinel.RecordType, sentinel.Path)
}

func assertJSONTakeoutBodyContainsSentinel(t *testing.T, body []byte, sentinel takeoutSentinel) {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("decode json takeout response: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected json takeout response data rows")
	}
	for _, row := range rows {
		if takeoutString(row["record_type"]) == sentinel.RecordType && takeoutString(row["path"]) == sentinel.Path {
			return
		}
	}
	t.Fatalf("expected json takeout response to contain %s path %q", sentinel.RecordType, sentinel.Path)
}

func assertNDJSONTakeoutBodyContainsSentinel(t *testing.T, body []byte, sentinel takeoutSentinel) {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	rows := 0
	for scanner.Scan() {
		rows++
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatalf("decode ndjson takeout response row: %v", err)
		}
		if takeoutString(row["record_type"]) == sentinel.RecordType && takeoutString(row["path"]) == sentinel.Path {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan ndjson takeout response: %v", err)
	}
	if rows == 0 {
		t.Fatalf("expected ndjson takeout response data rows")
	}
	t.Fatalf("expected ndjson takeout response to contain %s path %q", sentinel.RecordType, sentinel.Path)
}

func assertParquetTakeoutFileContainsSentinel(t *testing.T, store *database.Store, filename string, sentinel takeoutSentinel) {
	t.Helper()
	safePath := strings.ReplaceAll(filename, "'", "''")
	var count int
	if err := store.DB().QueryRowContext(context.Background(),
		fmt.Sprintf("SELECT COUNT(*) FROM read_parquet('%s') WHERE record_type = ? AND path = ?", safePath),
		sentinel.RecordType,
		sentinel.Path,
	).Scan(&count); err != nil {
		t.Fatalf("query parquet takeout response: %v", err)
	}
	if count == 0 {
		t.Fatalf("expected parquet takeout response to contain %s path %q", sentinel.RecordType, sentinel.Path)
	}
}

func assertXLSXTakeoutFileContainsSentinel(t *testing.T, filename string, sentinel takeoutSentinel) {
	t.Helper()
	archive, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatalf("open xlsx takeout response: %v", err)
	}
	defer archive.Close()

	var xmlText strings.Builder
	for _, file := range archive.File {
		if !strings.HasSuffix(strings.ToLower(file.Name), ".xml") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open xlsx response xml %s: %v", file.Name, err)
		}
		content, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			t.Fatalf("read xlsx response xml %s: %v", file.Name, readErr)
		}
		if closeErr != nil {
			t.Fatalf("close xlsx response xml %s: %v", file.Name, closeErr)
		}
		xmlText.Write(content)
		xmlText.WriteByte('\n')
	}

	payload := xmlText.String()
	for _, value := range []string{"record_type", sentinel.RecordType, sentinel.Path} {
		if !strings.Contains(payload, value) {
			t.Fatalf("expected xlsx takeout response XML to contain %q", value)
		}
	}
}

func takeoutHeaderIndex(t *testing.T, header []string, columns ...string) map[string]int {
	t.Helper()
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}
	for _, col := range columns {
		if _, ok := index[col]; !ok {
			t.Fatalf("expected column %q in takeout export header", col)
		}
	}
	return index
}

func takeoutString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}
