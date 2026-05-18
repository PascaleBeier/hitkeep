package takeout

import (
	"archive/zip"
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/exportfmt"
	"hitkeep/internal/importables"
)

type takeoutSentinel struct {
	RecordType string
	Path       string
}

func TestExportSiteDataCSVIncludesUTMFields(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "takeout.db")
	exportDir := filepath.Join(t.TempDir(), "exports")

	store := database.NewStore(dbPath)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	userID, err := store.CreateUser(ctx, "takeout@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "takeout.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	isUnique := true
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:      site.ID,
		SessionID:   uuid.New(),
		PageID:      uuid.New(),
		Timestamp:   now,
		Path:        "/utm",
		UTMSource:   new("newsletter"),
		UTMMedium:   new("email"),
		UTMCampaign: new("feb-launch"),
		UTMTerm:     new("feature"),
		UTMContent:  new("button-a"),
		IsUnique:    &isUnique,
	}); err != nil {
		t.Fatalf("create hit: %v", err)
	}

	service := NewTakeoutService(store, exportDir)
	filename, err := service.ExportSiteData(ctx, site.ID, "csv")
	if err != nil {
		t.Fatalf("export site data: %v", err)
	}

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open export file: %v", err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least header and one row, got %d rows", len(rows))
	}

	header := rows[0]
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}
	if _, ok := index["record_type"]; !ok {
		t.Fatalf("expected column %q in takeout export header", "record_type")
	}
	for _, col := range []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"} {
		if _, ok := index[col]; !ok {
			t.Fatalf("expected column %q in takeout export header", col)
		}
	}

	hitRow := -1
	for i := 1; i < len(rows); i++ {
		if rows[i][index["record_type"]] == "hit" {
			hitRow = i
			break
		}
	}
	if hitRow == -1 {
		t.Fatalf("expected at least one hit row in takeout export")
	}

	row := rows[hitRow]
	if got := row[index["utm_source"]]; got != "newsletter" {
		t.Fatalf("expected utm_source=newsletter, got %q", got)
	}
	if got := row[index["utm_medium"]]; got != "email" {
		t.Fatalf("expected utm_medium=email, got %q", got)
	}
	if got := row[index["utm_campaign"]]; got != "feb-launch" {
		t.Fatalf("expected utm_campaign=feb-launch, got %q", got)
	}
	if got := row[index["utm_term"]]; got != "feature" {
		t.Fatalf("expected utm_term=feature, got %q", got)
	}
	if got := row[index["utm_content"]]; got != "button-a" {
		t.Fatalf("expected utm_content=button-a, got %q", got)
	}
}

func TestExportSiteDataCSVIncludesGeoNetworkFields(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "takeout.db")
	exportDir := filepath.Join(t.TempDir(), "exports")

	store := database.NewStore(dbPath)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	userID, err := store.CreateUser(ctx, "geo-takeout@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "geo-takeout.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	region := "California"
	city := "Mountain View"
	provider := "Google LLC"
	asn := 15169
	asnOrg := "Google LLC"
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: time.Now().UTC(),
		Path:      "/geo",
		Region:    &region,
		City:      &city,
		Provider:  &provider,
		ASN:       &asn,
		ASNOrg:    &asnOrg,
	}); err != nil {
		t.Fatalf("create hit: %v", err)
	}

	service := NewTakeoutService(store, exportDir)
	filename, err := service.ExportSiteData(ctx, site.ID, "csv")
	if err != nil {
		t.Fatalf("export site data: %v", err)
	}

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open export file: %v", err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least header and one row, got %d rows", len(rows))
	}

	index := takeoutHeaderIndex(t, rows[0], "record_type", "region", "city", "provider", "asn", "asn_org")
	for _, row := range rows[1:] {
		if row[index["record_type"]] != "hit" {
			continue
		}
		if got := row[index["region"]]; got != "California" {
			t.Fatalf("expected region California, got %q", got)
		}
		if got := row[index["city"]]; got != "Mountain View" {
			t.Fatalf("expected city Mountain View, got %q", got)
		}
		if got := row[index["provider"]]; got != "Google LLC" {
			t.Fatalf("expected provider Google LLC, got %q", got)
		}
		if got := row[index["asn"]]; got != "15169" {
			t.Fatalf("expected asn 15169, got %q", got)
		}
		if got := row[index["asn_org"]]; got != "Google LLC" {
			t.Fatalf("expected asn_org Google LLC, got %q", got)
		}
		return
	}

	t.Fatalf("expected at least one hit row in takeout export")
}

func TestExportUserDataCSVIncludesGeoNetworkFields(t *testing.T) {
	ctx, store, service, userID, siteID := setupTakeoutFixture(t)
	t.Cleanup(func() { _ = store.Close() })

	region := "California"
	city := "Mountain View"
	provider := "Google LLC"
	asn := 15169
	asnOrg := "Google LLC"
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    siteID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: time.Now().UTC(),
		Path:      "/geo-user",
		Region:    &region,
		City:      &city,
		Provider:  &provider,
		ASN:       &asn,
		ASNOrg:    &asnOrg,
	}); err != nil {
		t.Fatalf("create geo hit: %v", err)
	}

	filename, err := service.ExportUserData(ctx, userID, "csv")
	if err != nil {
		t.Fatalf("export user data: %v", err)
	}

	requireCSVTakeoutRow(t, readCSVTakeout(t, filename),
		map[string]string{
			"record_type": "hit",
			"path":        "/geo-user",
		},
		map[string]string{
			"region":   "California",
			"city":     "Mountain View",
			"provider": "Google LLC",
			"asn":      "15169",
			"asn_org":  "Google LLC",
		},
	)
}

func TestExportSiteDataCSVIncludesImportedEventDimensions(t *testing.T) {
	ctx, store, service, _, siteID := setupTakeoutFixture(t)
	t.Cleanup(func() { _ = store.Close() })

	day := time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC)
	sink, err := database.NewImportedDataSink(ctx, store, siteID, uuid.New())
	if err != nil {
		t.Fatalf("new imported sink: %v", err)
	}
	for _, row := range []importables.EventDimensionRow{
		{Date: day, EventName: "signup", Dimension: "city", Name: "Dortmund", Visitors: 7, Events: 9, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "provider", Name: "Deutsche Telekom AG", Visitors: 6, Events: 8, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "asn", Name: "AS3320 Deutsche Telekom AG", Visitors: 5, Events: 7, SourceFile: "imported_event_dimensions.csv"},
	} {
		if err := sink.PutEventDimension(ctx, row); err != nil {
			t.Fatalf("put imported event dimension %s: %v", row.Dimension, err)
		}
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush imported event dimensions: %v", err)
	}

	filename, err := service.ExportSiteData(ctx, siteID, "csv")
	if err != nil {
		t.Fatalf("export site data: %v", err)
	}

	requireCSVTakeoutRow(t, readCSVTakeout(t, filename),
		map[string]string{"record_type": "imported_event_dimension", "dimension": "city"},
		map[string]string{"event_name": "signup", "name": "Dortmund", "visitors": "7", "events": "9"},
	)
}

func TestExportSiteDataCSVIncludesAIFetchesAndAIChatbotEvents(t *testing.T) {
	ctx, store, service, _, siteID := setupTakeoutFixture(t)
	t.Cleanup(func() { _ = store.Close() })

	filename, err := service.ExportSiteData(ctx, siteID, "csv")
	if err != nil {
		t.Fatalf("export site data: %v", err)
	}

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open export file: %v", err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least header and one row, got %d rows", len(rows))
	}

	header := rows[0]
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}

	for _, col := range []string{"assistant_name", "assistant_family", "resource_type", "status_code", "name", "properties"} {
		if _, ok := index[col]; !ok {
			t.Fatalf("expected column %q in takeout export header", col)
		}
	}

	var foundAIFetch bool
	var foundChatbotEvent bool
	for _, row := range rows[1:] {
		switch row[index["record_type"]] {
		case "ai_fetch":
			if row[index["assistant_name"]] == "GPTBot" && row[index["resource_type"]] == "html" && row[index["status_code"]] == "200" {
				foundAIFetch = true
			}
		case "event":
			if row[index["name"]] == "assistant.chat_started" && strings.Contains(row[index["properties"]], "\"provider\":\"openai\"") {
				foundChatbotEvent = true
			}
		}
	}

	if !foundAIFetch {
		t.Fatalf("expected ai_fetch row in site takeout")
	}
	if !foundChatbotEvent {
		t.Fatalf("expected chatbot event row in site takeout")
	}
}

func TestExportUserDataCSVIncludesTenantManagedSites(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "takeout.db")
	exportDir := filepath.Join(t.TempDir(), "exports")

	store := database.NewStore(dbPath)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	tenantOwnerID, err := store.CreateUser(ctx, "tenant-owner-takeout@example.com", "hash")
	if err != nil {
		t.Fatalf("create tenant owner user: %v", err)
	}
	memberID, err := store.CreateUser(ctx, "tenant-member-takeout@example.com", "hash")
	if err != nil {
		t.Fatalf("create tenant member user: %v", err)
	}

	ownedSite, err := store.CreateSite(ctx, tenantOwnerID, "owned-takeout.test")
	if err != nil {
		t.Fatalf("create owned site: %v", err)
	}
	tenantManagedSite, err := store.CreateSite(ctx, memberID, "tenant-managed-takeout.test")
	if err != nil {
		t.Fatalf("create tenant managed site: %v", err)
	}

	createTakeoutHit(t, store, ownedSite.ID, "/owned")
	createTakeoutHit(t, store, tenantManagedSite.ID, "/tenant-managed")

	service := NewTakeoutService(store, exportDir)
	filename, err := service.ExportUserData(ctx, tenantOwnerID, "csv")
	if err != nil {
		t.Fatalf("export user data: %v", err)
	}

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open export file: %v", err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(rows) < 3 {
		t.Fatalf("expected header and two hit rows, got %d rows", len(rows))
	}

	header := rows[0]
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}
	for _, col := range []string{"record_type", "path"} {
		if _, ok := index[col]; !ok {
			t.Fatalf("expected column %q in takeout export header", col)
		}
	}

	paths := make(map[string]bool)
	for _, row := range rows[1:] {
		if row[index["record_type"]] == "hit" {
			paths[row[index["path"]]] = true
		}
	}

	if !paths["/owned"] {
		t.Fatalf("expected user takeout to include directly owned site hit, got paths %#v", paths)
	}
	if !paths["/tenant-managed"] {
		t.Fatalf("expected user takeout to include tenant-managed accessible site hit, got paths %#v", paths)
	}
}

func TestExportUserDataCSVIncludesAccessibleSitesOutsideActiveTenant(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "takeout.db")
	exportDir := filepath.Join(t.TempDir(), "exports")

	store := database.NewStore(dbPath)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ownerID, err := store.CreateUser(ctx, "cross-tenant-owner-takeout@example.com", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	userID, err := store.CreateUser(ctx, "cross-tenant-user-takeout@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defaultTenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	defaultSite, err := store.CreateSite(ctx, userID, "cross-tenant-default.test")
	if err != nil {
		t.Fatalf("create default site: %v", err)
	}

	team, err := store.CreateTenant(ctx, ownerID, "Cross Tenant Takeout", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := store.AddTeamMember(ctx, team.ID, userID, database.TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add user to team: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, ownerID, team.ID); err != nil {
		t.Fatalf("set owner active tenant: %v", err)
	}
	teamSite, err := store.CreateSite(ctx, ownerID, "cross-tenant-shared.test")
	if err != nil {
		t.Fatalf("create team site: %v", err)
	}
	if err := store.AddSiteMember(ctx, teamSite.ID, userID, auth.SiteViewer, ownerID); err != nil {
		t.Fatalf("add user as site viewer: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, defaultTenantID); err != nil {
		t.Fatalf("set user active tenant back to default: %v", err)
	}

	createTakeoutHit(t, store, defaultSite.ID, "/default-tenant")
	createTakeoutHit(t, store, teamSite.ID, "/other-tenant")

	service := NewTakeoutService(store, exportDir)
	filename, err := service.ExportUserData(ctx, userID, "csv")
	if err != nil {
		t.Fatalf("export user data: %v", err)
	}

	paths := readTakeoutHitPaths(t, filename)
	if !paths["/default-tenant"] {
		t.Fatalf("expected default tenant hit in user takeout, got paths %#v", paths)
	}
	if !paths["/other-tenant"] {
		t.Fatalf("expected other tenant accessible hit in user takeout, got paths %#v", paths)
	}
}

func TestExportSiteDataNDJSONIncludesUTMFields(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "takeout.db")
	exportDir := filepath.Join(t.TempDir(), "exports")

	store := database.NewStore(dbPath)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	userID, err := store.CreateUser(ctx, "takeout@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "takeout.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	isUnique := true
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:      site.ID,
		SessionID:   uuid.New(),
		PageID:      uuid.New(),
		Timestamp:   now,
		Path:        "/utm",
		UTMSource:   new("newsletter"),
		UTMMedium:   new("email"),
		UTMCampaign: new("feb-launch"),
		UTMTerm:     new("feature"),
		UTMContent:  new("button-a"),
		IsUnique:    &isUnique,
	}); err != nil {
		t.Fatalf("create hit: %v", err)
	}

	service := NewTakeoutService(store, exportDir)
	filename, err := service.ExportSiteData(ctx, site.ID, "ndjson")
	if err != nil {
		t.Fatalf("export site data: %v", err)
	}
	if ext := strings.ToLower(filepath.Ext(filename)); ext != ".ndjson" {
		t.Fatalf("expected ndjson export extension, got %q", ext)
	}

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open export file: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	foundHit := false
	for scanner.Scan() {
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatalf("decode ndjson row: %v", err)
		}

		recordType, _ := row["record_type"].(string)
		if recordType != "hit" {
			continue
		}
		foundHit = true

		if got, _ := row["utm_source"].(string); got != "newsletter" {
			t.Fatalf("expected utm_source=newsletter, got %q", got)
		}
		if got, _ := row["utm_medium"].(string); got != "email" {
			t.Fatalf("expected utm_medium=email, got %q", got)
		}
		if got, _ := row["utm_campaign"].(string); got != "feb-launch" {
			t.Fatalf("expected utm_campaign=feb-launch, got %q", got)
		}
		if got, _ := row["utm_term"].(string); got != "feature" {
			t.Fatalf("expected utm_term=feature, got %q", got)
		}
		if got, _ := row["utm_content"].(string); got != "button-a" {
			t.Fatalf("expected utm_content=button-a, got %q", got)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan ndjson export: %v", err)
	}
	if !foundHit {
		t.Fatalf("expected at least one hit row in ndjson export")
	}
}

func TestResolveExportFormatCoversAllSupportedFormats(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		wantExt    string
		wantFormat string
	}{
		{name: "csv", format: "csv", wantExt: "csv", wantFormat: "CSV, HEADER"},
		{name: "xlsx", format: "xlsx", wantExt: "xlsx", wantFormat: "XLSX, HEADER true"},
		{name: "parquet", format: "parquet", wantExt: "parquet", wantFormat: "PARQUET, COMPRESSION 'SNAPPY'"},
		{name: "json", format: "json", wantExt: "json", wantFormat: "JSON, ARRAY true"},
		{name: "ndjson", format: "ndjson", wantExt: "ndjson", wantFormat: "JSON"},
		{name: "unknown defaults to xlsx", format: "unknown", wantExt: "xlsx", wantFormat: "XLSX, HEADER true"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotExt := exportfmt.Normalize(tc.format, exportfmt.FormatXLSX)
			gotFormat := exportfmt.DuckDBCopyOptions(gotExt)
			if gotExt != tc.wantExt {
				t.Fatalf("expected ext %q, got %q", tc.wantExt, gotExt)
			}
			if gotFormat != tc.wantFormat {
				t.Fatalf("expected format %q, got %q", tc.wantFormat, gotFormat)
			}
		})
	}
}

func TestExportSiteDataSupportsAllFormats(t *testing.T) {
	ctx, store, service, _, siteID := setupTakeoutFixture(t)
	t.Cleanup(func() { _ = store.Close() })

	tests := []struct {
		name   string
		format string
		ext    string
	}{
		{name: "csv", format: "csv", ext: ".csv"},
		{name: "xlsx", format: "xlsx", ext: ".xlsx"},
		{name: "parquet", format: "parquet", ext: ".parquet"},
		{name: "json", format: "json", ext: ".json"},
		{name: "ndjson", format: "ndjson", ext: ".ndjson"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filename, err := service.ExportSiteData(ctx, siteID, tc.format)
			if err != nil {
				t.Fatalf("export site data %s: %v", tc.format, err)
			}
			if ext := strings.ToLower(filepath.Ext(filename)); ext != tc.ext {
				t.Fatalf("expected extension %q, got %q", tc.ext, ext)
			}
			assertTakeoutContainsSentinel(t, store, filename, tc.format, takeoutSentinel{RecordType: "hit", Path: "/utm"})
		})
	}
}

func TestExportUserDataSupportsAllFormats(t *testing.T) {
	ctx, store, service, userID, _ := setupTakeoutFixture(t)
	t.Cleanup(func() { _ = store.Close() })

	tests := []struct {
		name   string
		format string
		ext    string
	}{
		{name: "csv", format: "csv", ext: ".csv"},
		{name: "xlsx", format: "xlsx", ext: ".xlsx"},
		{name: "parquet", format: "parquet", ext: ".parquet"},
		{name: "json", format: "json", ext: ".json"},
		{name: "ndjson", format: "ndjson", ext: ".ndjson"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filename, err := service.ExportUserData(ctx, userID, tc.format)
			if err != nil {
				t.Fatalf("export user data %s: %v", tc.format, err)
			}
			if ext := strings.ToLower(filepath.Ext(filename)); ext != tc.ext {
				t.Fatalf("expected extension %q, got %q", tc.ext, ext)
			}
			assertTakeoutContainsSentinel(t, store, filename, tc.format, takeoutSentinel{RecordType: "hit", Path: "/utm"})
		})
	}
}

func TestExportUserDataWithTenantStoresIncludesCrossTenantRowsInAllFormats(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "shared.db")
	exportDir := filepath.Join(tmpDir, "exports")

	store := database.NewStore(dbPath)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect shared store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate shared store: %v", err)
	}

	tenantStores := database.NewTenantStoreManager(store, filepath.Join(tmpDir, "tenants"))
	t.Cleanup(func() { _ = tenantStores.Close() })

	userID, err := store.CreateUser(ctx, "tenant-aware-takeout@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defaultSite, err := store.CreateSite(ctx, userID, "tenant-aware-default.test")
	if err != nil {
		t.Fatalf("create default site: %v", err)
	}
	createTakeoutHit(t, store, defaultSite.ID, "/default-scope")

	team, err := store.CreateTenant(ctx, userID, "Tenant Takeout", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	teamSite, err := store.CreateSite(ctx, userID, "tenant-aware-team.test")
	if err != nil {
		t.Fatalf("create team site: %v", err)
	}
	teamStore, _, err := tenantStores.ResolveSiteStore(ctx, teamSite.ID)
	if err != nil {
		t.Fatalf("resolve team analytics store: %v", err)
	}
	createTakeoutHit(t, teamStore, teamSite.ID, "/team-scope")

	service := NewTakeoutServiceWithTenantStores(store, tenantStores, exportDir)
	for _, format := range []string{"csv", "xlsx", "parquet", "json", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			filename, err := service.ExportUserData(ctx, userID, format)
			if err != nil {
				t.Fatalf("export user data %s: %v", format, err)
			}
			assertTakeoutContainsSentinel(t, store, filename, format, takeoutSentinel{RecordType: "hit", Path: "/default-scope"})
			assertTakeoutContainsSentinel(t, store, filename, format, takeoutSentinel{RecordType: "hit", Path: "/team-scope"})
		})
	}
}

func TestExportUserDataWithTenantStoresCleansEmptyMergeFiles(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	exportDir := filepath.Join(tmpDir, "exports")

	store := database.NewStore(filepath.Join(tmpDir, "shared.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect shared store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate shared store: %v", err)
	}
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		t.Fatalf("create export dir: %v", err)
	}

	service := NewTakeoutService(store, exportDir)
	filename := filepath.Join(exportDir, "user_takeout_empty.csv")
	if _, err := service.exportTakeoutFromSources(ctx, "user", filename, exportfmt.FormatCSV, exportfmt.DuckDBCopyOptions(exportfmt.FormatCSV), []takeoutStoreSource{
		{Store: store, Source: takeoutQuerySource{WhereClause: "FALSE"}},
		{Store: store, Source: takeoutQuerySource{WhereClause: "FALSE"}},
	}); err != nil {
		t.Fatalf("export empty merge sources: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(exportDir, "takeout_merge_*"))
	if err != nil {
		t.Fatalf("glob merge files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected merge temp files to be cleaned up, got %v", matches)
	}
}

func TestExportUserDataWithTenantStoresPreservesParquetSchema(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	exportDir := filepath.Join(tmpDir, "exports")

	store := database.NewStore(filepath.Join(tmpDir, "shared.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect shared store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate shared store: %v", err)
	}

	userID, err := store.CreateUser(ctx, "schema-takeout@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defaultSite, err := store.CreateSite(ctx, userID, "schema-default.test")
	if err != nil {
		t.Fatalf("create default site: %v", err)
	}
	createTakeoutHit(t, store, defaultSite.ID, "/schema-default")

	plainService := NewTakeoutService(store, filepath.Join(tmpDir, "plain-exports"))
	plainFilename, err := plainService.ExportUserData(ctx, userID, "parquet")
	if err != nil {
		t.Fatalf("export direct user data: %v", err)
	}
	wantSchema := parquetTakeoutSchema(t, store, plainFilename)

	tenantStores := database.NewTenantStoreManager(store, filepath.Join(tmpDir, "tenants"))
	t.Cleanup(func() { _ = tenantStores.Close() })
	team, err := store.CreateTenant(ctx, userID, "Schema Team", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	teamSite, err := store.CreateSite(ctx, userID, "schema-team.test")
	if err != nil {
		t.Fatalf("create team site: %v", err)
	}
	teamStore, _, err := tenantStores.ResolveSiteStore(ctx, teamSite.ID)
	if err != nil {
		t.Fatalf("resolve team analytics store: %v", err)
	}
	createTakeoutHit(t, teamStore, teamSite.ID, "/schema-team")

	tenantService := NewTakeoutServiceWithTenantStores(store, tenantStores, exportDir)
	mergedFilename, err := tenantService.ExportUserData(ctx, userID, "parquet")
	if err != nil {
		t.Fatalf("export tenant-aware user data: %v", err)
	}
	gotSchema := parquetTakeoutSchema(t, store, mergedFilename)

	for column, wantType := range wantSchema {
		if gotType, ok := gotSchema[column]; !ok {
			t.Fatalf("expected merged parquet schema to include column %q", column)
		} else if gotType != wantType {
			t.Fatalf("expected merged parquet column %q type %q, got %q", column, wantType, gotType)
		}
	}
}

func TestExportSiteDataWithTenantStoresIncludesTeamSiteRowsInAllFormats(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	exportDir := filepath.Join(tmpDir, "exports")

	store := database.NewStore(filepath.Join(tmpDir, "shared.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect shared store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate shared store: %v", err)
	}
	tenantStores := database.NewTenantStoreManager(store, filepath.Join(tmpDir, "tenants"))
	t.Cleanup(func() { _ = tenantStores.Close() })

	userID, err := store.CreateUser(ctx, "site-tenant-aware-takeout@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	team, err := store.CreateTenant(ctx, userID, "Site Takeout Team", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	teamSite, err := store.CreateSite(ctx, userID, "site-tenant-aware.test")
	if err != nil {
		t.Fatalf("create team site: %v", err)
	}
	teamStore, _, err := tenantStores.ResolveSiteStore(ctx, teamSite.ID)
	if err != nil {
		t.Fatalf("resolve team analytics store: %v", err)
	}
	createTakeoutHit(t, teamStore, teamSite.ID, "/team-site-export")

	service := NewTakeoutServiceWithTenantStores(store, tenantStores, exportDir)
	for _, format := range []string{"csv", "xlsx", "parquet", "json", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			filename, err := service.ExportSiteData(ctx, teamSite.ID, format)
			if err != nil {
				t.Fatalf("export site data %s: %v", format, err)
			}
			assertTakeoutContainsSentinel(t, store, filename, format, takeoutSentinel{RecordType: "hit", Path: "/team-site-export"})
		})
	}
}

func TestExportSiteDataJSONIncludesUTMFields(t *testing.T) {
	ctx, store, service, _, siteID := setupTakeoutFixture(t)
	t.Cleanup(func() { _ = store.Close() })

	filename, err := service.ExportSiteData(ctx, siteID, "json")
	if err != nil {
		t.Fatalf("export site data json: %v", err)
	}
	if ext := strings.ToLower(filepath.Ext(filename)); ext != ".json" {
		t.Fatalf("expected json export extension, got %q", ext)
	}

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open export file: %v", err)
	}
	defer f.Close()

	var rows []map[string]any
	if err := json.NewDecoder(f).Decode(&rows); err != nil {
		t.Fatalf("decode json export: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected at least one row in json export")
	}

	foundHit := false
	for _, row := range rows {
		recordType, _ := row["record_type"].(string)
		if recordType != "hit" {
			continue
		}
		foundHit = true

		if got, _ := row["utm_source"].(string); got != "newsletter" {
			t.Fatalf("expected utm_source=newsletter, got %q", got)
		}
		if got, _ := row["utm_medium"].(string); got != "email" {
			t.Fatalf("expected utm_medium=email, got %q", got)
		}
		if got, _ := row["utm_campaign"].(string); got != "feb-launch" {
			t.Fatalf("expected utm_campaign=feb-launch, got %q", got)
		}
		if got, _ := row["utm_term"].(string); got != "feature" {
			t.Fatalf("expected utm_term=feature, got %q", got)
		}
		if got, _ := row["utm_content"].(string); got != "button-a" {
			t.Fatalf("expected utm_content=button-a, got %q", got)
		}
	}
	if !foundHit {
		t.Fatalf("expected at least one hit row in json export")
	}
}

func TestExportUserDataJSONIncludesAIFetchesAndAIChatbotEvents(t *testing.T) {
	ctx, store, service, userID, _ := setupTakeoutFixture(t)
	t.Cleanup(func() { _ = store.Close() })

	filename, err := service.ExportUserData(ctx, userID, "json")
	if err != nil {
		t.Fatalf("export user data json: %v", err)
	}

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open export file: %v", err)
	}
	defer f.Close()

	var rows []map[string]any
	if err := json.NewDecoder(f).Decode(&rows); err != nil {
		t.Fatalf("decode json export: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected at least one row in json export")
	}

	var foundAIFetch bool
	var foundChatbotEvent bool
	for _, row := range rows {
		recordType, _ := row["record_type"].(string)
		switch recordType {
		case "ai_fetch":
			if assistantName, _ := row["assistant_name"].(string); assistantName == "GPTBot" {
				if resourceType, _ := row["resource_type"].(string); resourceType == "html" {
					foundAIFetch = true
				}
			}
		case "event":
			if name, _ := row["name"].(string); name == "assistant.chat_started" {
				if properties, ok := row["properties"].(map[string]any); ok {
					if provider, _ := properties["provider"].(string); provider == "openai" {
						foundChatbotEvent = true
					}
				}
			}
		}
	}

	if !foundAIFetch {
		t.Fatalf("expected ai_fetch row in user takeout")
	}
	if !foundChatbotEvent {
		t.Fatalf("expected chatbot event row in user takeout")
	}
}

func TestExportSiteDataJSONIncludesWebVitals(t *testing.T) {
	ctx, store, service, _, siteID := setupTakeoutFixture(t)
	t.Cleanup(func() { _ = store.Close() })

	filename, err := service.ExportSiteData(ctx, siteID, "json")
	if err != nil {
		t.Fatalf("export site data json: %v", err)
	}

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open export file: %v", err)
	}
	defer f.Close()

	var rows []map[string]any
	if err := json.NewDecoder(f).Decode(&rows); err != nil {
		t.Fatalf("decode json export: %v", err)
	}

	for _, row := range rows {
		if recordType, _ := row["record_type"].(string); recordType == "web_vital" {
			if metric, _ := row["metric"].(string); metric != "LCP" {
				t.Fatalf("expected LCP web_vital metric, got %q", metric)
			}
			if path, _ := row["path"].(string); path != "/pricing" {
				t.Fatalf("expected /pricing web_vital path, got %q", path)
			}
			return
		}
	}
	t.Fatalf("expected web_vital row in site takeout")
}

func TestExportUserDataJSONIncludesWebVitals(t *testing.T) {
	ctx, store, service, userID, _ := setupTakeoutFixture(t)
	t.Cleanup(func() { _ = store.Close() })

	filename, err := service.ExportUserData(ctx, userID, "json")
	if err != nil {
		t.Fatalf("export user data json: %v", err)
	}

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open export file: %v", err)
	}
	defer f.Close()

	var rows []map[string]any
	if err := json.NewDecoder(f).Decode(&rows); err != nil {
		t.Fatalf("decode json export: %v", err)
	}

	for _, row := range rows {
		if recordType, _ := row["record_type"].(string); recordType == "web_vital" {
			if metric, _ := row["metric"].(string); metric != "LCP" {
				t.Fatalf("expected LCP web_vital metric, got %q", metric)
			}
			if path, _ := row["path"].(string); path != "/pricing" {
				t.Fatalf("expected /pricing web_vital path, got %q", path)
			}
			return
		}
	}
	t.Fatalf("expected web_vital row in user takeout")
}

func TestExportSiteDataJSONIncludesSafeOpportunitiesAndAIRunMetadata(t *testing.T) {
	ctx, store, service, _, siteID := setupTakeoutFixture(t)
	t.Cleanup(func() { _ = store.Close() })

	teamID, err := store.GetSiteTenantID(ctx, siteID)
	if err != nil {
		t.Fatalf("resolve team: %v", err)
	}
	requireTakeoutAIRunRejectsRawProviderPayload(t, ctx, store, teamID, siteID)

	runID, err := store.AppendAIRun(ctx, database.AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		EvidenceIDs:     []string{"checkout_starts"},
		InputHash:       "input-hash",
		OutputHash:      "output-hash",
		OutputJSON:      `{"title_key":"opportunities.catalog.checkout_conversion.title"}`,
		TotalTokens:     42,
		Status:          "success",
	})
	if err != nil {
		t.Fatalf("append ai run: %v", err)
	}
	_, err = store.UpsertOpportunities(ctx, []database.OpportunityInput{{
		TeamID:           teamID,
		SiteID:           siteID,
		Kind:             "conversion",
		TypeKey:          "opportunities.types.checkout_conversion",
		TitleKey:         "opportunities.catalog.checkout_conversion.title",
		SummaryKey:       "opportunities.catalog.checkout_conversion.summary",
		ActionKey:        "opportunities.catalog.checkout_conversion.action",
		DigestKey:        "opportunities.catalog.checkout_conversion.digest",
		CopyParams:       map[string]any{"conversion_rate": "42%"},
		ImpactValue:      "$8,500",
		ImpactLabelKey:   "opportunities.impact.checkout_starts",
		Confidence:       "high",
		Score:            92,
		Status:           "new",
		RouteLabelKey:    "opportunities.routes.checkout",
		RouteParams:      map[string]any{"path": "/checkout"},
		DetectorVersion:  "opportunities-detectors-v1",
		Evidence:         []api.OpportunityEvidence{{ID: "checkout_starts", LabelKey: "opportunities.evidence.checkout_starts", Value: "120"}},
		CitedEvidenceIDs: []string{"checkout_starts"},
		AIRunID:          runID,
	}})
	if err != nil {
		t.Fatalf("upsert opportunity: %v", err)
	}

	filename, err := service.ExportSiteData(ctx, siteID, "json")
	if err != nil {
		t.Fatalf("export site json: %v", err)
	}
	rows := readJSONTakeoutRows(t, filename)

	opportunity := findTakeoutRow(t, rows, "opportunity")
	if takeoutString(opportunity["title_key"]) != "opportunities.catalog.checkout_conversion.title" {
		t.Fatalf("expected opportunity title key in takeout, got %#v", opportunity)
	}
	if !strings.Contains(takeoutString(opportunity["copy_params_json"]), "conversion_rate") {
		t.Fatalf("expected opportunity copy params in takeout, got %#v", opportunity)
	}
	if !strings.Contains(takeoutString(opportunity["cited_evidence_ids_json"]), "checkout_starts") {
		t.Fatalf("expected opportunity cited evidence ids in takeout, got %#v", opportunity)
	}

	aiRun := findTakeoutRow(t, rows, "ai_run")
	if takeoutString(aiRun["feature"]) != "opportunities" || takeoutString(aiRun["output_hash"]) != "output-hash" {
		t.Fatalf("expected safe ai run metadata in takeout, got %#v", aiRun)
	}
	if _, ok := aiRun["output_json"]; ok {
		t.Fatalf("did not expect raw ai run output_json in takeout: %#v", aiRun)
	}
	encodedRows, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("marshal rows: %v", err)
	}
	for _, forbidden := range []string{"do not export", "raw_provider_response", "sk-", "provider_error_body", "raw_prompt"} {
		if strings.Contains(string(encodedRows), forbidden) {
			t.Fatalf("takeout leaked forbidden AI payload marker %q: %s", forbidden, string(encodedRows))
		}
	}
}

func requireTakeoutAIRunRejectsRawProviderPayload(t *testing.T, ctx context.Context, store *database.Store, teamID, siteID uuid.UUID) {
	t.Helper()
	_, err := store.AppendAIRun(ctx, database.AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{"raw_provider_response":"do not persist","title_key":"opportunities.catalog.checkout_conversion.title"}`,
		Status:          "success",
	})
	if err == nil || !strings.Contains(err.Error(), "provider payload") {
		t.Fatalf("expected raw provider payload to be rejected before takeout, got %v", err)
	}
}

func setupTakeoutFixture(t *testing.T) (context.Context, *database.Store, *TakeoutService, uuid.UUID, uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "takeout.db")
	exportDir := filepath.Join(t.TempDir(), "exports")

	store := database.NewStore(dbPath)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	userID, err := store.CreateUser(ctx, "takeout@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "takeout.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	isUnique := true
	sessionID := uuid.New()
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:      site.ID,
		SessionID:   sessionID,
		PageID:      uuid.New(),
		Timestamp:   now,
		Path:        "/utm",
		UTMSource:   new("newsletter"),
		UTMMedium:   new("email"),
		UTMCampaign: new("feb-launch"),
		UTMTerm:     new("feature"),
		UTMContent:  new("button-a"),
		IsUnique:    &isUnique,
	}); err != nil {
		t.Fatalf("create hit: %v", err)
	}

	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:    site.ID,
		SessionID: sessionID,
		Name:      "assistant.chat_started",
		Properties: map[string]any{
			"provider": "openai",
			"bot_id":   "support-bot",
			"surface":  "pricing-assistant",
			"model":    "gpt-4.1-mini",
		},
		Timestamp: now,
	}); err != nil {
		t.Fatalf("create chatbot event: %v", err)
	}

	htmlType := "text/html; charset=utf-8"
	responseMs := 180
	bytesServed := int64(4096)
	userAgent := "Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)"
	if err := store.CreateAIFetch(ctx, &api.AIFetch{
		SiteID:          site.ID,
		Timestamp:       now,
		AssistantName:   "GPTBot",
		AssistantFamily: "OpenAI",
		Path:            "/pricing",
		StatusCode:      200,
		ContentType:     &htmlType,
		ResourceType:    "html",
		ResponseMs:      &responseMs,
		BytesServed:     &bytesServed,
		UserAgent:       &userAgent,
	}); err != nil {
		t.Fatalf("create ai fetch: %v", err)
	}

	if err := store.CreateWebVital(ctx, &api.WebVital{
		SiteID:         site.ID,
		SessionID:      sessionID,
		PageID:         uuid.New(),
		Metric:         api.WebVitalLCP,
		Value:          2600,
		Path:           "/pricing",
		Timestamp:      now,
		TrackerSource:  "browser",
		TrackerVersion: "test",
	}); err != nil {
		t.Fatalf("create web vital: %v", err)
	}

	service := NewTakeoutService(store, exportDir)
	return ctx, store, service, userID, site.ID
}

func createTakeoutHit(t *testing.T, store *database.Store, siteID uuid.UUID, path string) {
	t.Helper()

	isUnique := true
	if err := store.CreateHit(context.Background(), &api.Hit{
		SiteID:    siteID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: time.Now().UTC(),
		Path:      path,
		IsUnique:  &isUnique,
	}); err != nil {
		t.Fatalf("create hit for %s: %v", path, err)
	}
}

func readTakeoutHitPaths(t *testing.T, filename string) map[string]bool {
	t.Helper()

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open export file: %v", err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected header and at least one row, got %d rows", len(rows))
	}

	header := rows[0]
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}
	for _, col := range []string{"record_type", "path"} {
		if _, ok := index[col]; !ok {
			t.Fatalf("expected column %q in takeout export header", col)
		}
	}

	paths := make(map[string]bool)
	for _, row := range rows[1:] {
		if row[index["record_type"]] == "hit" {
			paths[row[index["path"]]] = true
		}
	}
	return paths
}

func assertTakeoutContainsSentinel(t *testing.T, store *database.Store, filename string, format string, sentinel takeoutSentinel) {
	t.Helper()

	info, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("stat exported file: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("expected non-empty exported file for format %s", format)
	}

	switch exportfmt.Normalize(format, exportfmt.FormatXLSX) {
	case exportfmt.FormatCSV:
		assertCSVTakeoutContainsSentinel(t, filename, sentinel)
	case exportfmt.FormatJSON:
		assertJSONTakeoutContainsSentinel(t, filename, sentinel)
	case exportfmt.FormatNDJSON:
		assertNDJSONTakeoutContainsSentinel(t, filename, sentinel)
	case exportfmt.FormatParquet:
		assertParquetTakeoutContainsSentinel(t, store, filename, sentinel)
	case exportfmt.FormatXLSX:
		assertXLSXTakeoutContainsSentinel(t, filename, sentinel)
	default:
		t.Fatalf("unsupported takeout format %q", format)
	}
}

func assertCSVTakeoutContainsSentinel(t *testing.T, filename string, sentinel takeoutSentinel) {
	t.Helper()

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open csv takeout: %v", err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read csv takeout: %v", err)
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
	t.Fatalf("expected csv takeout to contain %s path %q", sentinel.RecordType, sentinel.Path)
}

func assertJSONTakeoutContainsSentinel(t *testing.T, filename string, sentinel takeoutSentinel) {
	t.Helper()

	rows := readJSONTakeoutRows(t, filename)
	if len(rows) == 0 {
		t.Fatalf("expected json takeout data rows")
	}
	for _, row := range rows {
		if takeoutString(row["record_type"]) == sentinel.RecordType && takeoutString(row["path"]) == sentinel.Path {
			return
		}
	}
	t.Fatalf("expected json takeout to contain %s path %q", sentinel.RecordType, sentinel.Path)
}

func readJSONTakeoutRows(t *testing.T, filename string) []map[string]any {
	t.Helper()

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open json takeout: %v", err)
	}
	defer f.Close()

	var rows []map[string]any
	if err := json.NewDecoder(f).Decode(&rows); err != nil {
		t.Fatalf("decode json takeout: %v", err)
	}
	return rows
}

func findTakeoutRow(t *testing.T, rows []map[string]any, recordType string) map[string]any {
	t.Helper()

	for _, row := range rows {
		if takeoutString(row["record_type"]) == recordType {
			return row
		}
	}
	t.Fatalf("expected takeout row with record_type %q in %#v", recordType, rows)
	return nil
}

func assertNDJSONTakeoutContainsSentinel(t *testing.T, filename string, sentinel takeoutSentinel) {
	t.Helper()

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open ndjson takeout: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	rows := 0
	for scanner.Scan() {
		rows++
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatalf("decode ndjson takeout row: %v", err)
		}
		if takeoutString(row["record_type"]) == sentinel.RecordType && takeoutString(row["path"]) == sentinel.Path {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan ndjson takeout: %v", err)
	}
	if rows == 0 {
		t.Fatalf("expected ndjson takeout data rows")
	}
	t.Fatalf("expected ndjson takeout to contain %s path %q", sentinel.RecordType, sentinel.Path)
}

func assertParquetTakeoutContainsSentinel(t *testing.T, store *database.Store, filename string, sentinel takeoutSentinel) {
	t.Helper()

	safePath := strings.ReplaceAll(filename, "'", "''")
	var count int
	if err := store.DB().QueryRowContext(context.Background(),
		fmt.Sprintf("SELECT COUNT(*) FROM read_parquet('%s') WHERE record_type = ? AND path = ?", safePath),
		sentinel.RecordType,
		sentinel.Path,
	).Scan(&count); err != nil {
		t.Fatalf("query parquet takeout: %v", err)
	}
	if count == 0 {
		t.Fatalf("expected parquet takeout to contain %s path %q", sentinel.RecordType, sentinel.Path)
	}
}

func parquetTakeoutSchema(t *testing.T, store *database.Store, filename string) map[string]string {
	t.Helper()

	safePath := strings.ReplaceAll(filename, "'", "''")
	rows, err := store.DB().QueryContext(context.Background(), fmt.Sprintf("DESCRIBE SELECT * FROM read_parquet('%s')", safePath))
	if err != nil {
		t.Fatalf("describe parquet takeout: %v", err)
	}
	defer rows.Close()

	schema := make(map[string]string)
	for rows.Next() {
		var columnName, columnType, nullValue, keyValue, defaultValue, extraValue sql.NullString
		if err := rows.Scan(&columnName, &columnType, &nullValue, &keyValue, &defaultValue, &extraValue); err != nil {
			t.Fatalf("scan parquet schema row: %v", err)
		}
		if columnName.Valid && columnType.Valid {
			schema[columnName.String] = columnType.String
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read parquet schema rows: %v", err)
	}
	if len(schema) == 0 {
		t.Fatalf("expected parquet schema columns")
	}
	return schema
}

func assertXLSXTakeoutContainsSentinel(t *testing.T, filename string, sentinel takeoutSentinel) {
	t.Helper()

	archive, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatalf("open xlsx takeout: %v", err)
	}
	defer archive.Close()

	var xmlText strings.Builder
	for _, file := range archive.File {
		if !strings.HasSuffix(strings.ToLower(file.Name), ".xml") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open xlsx xml %s: %v", file.Name, err)
		}
		content, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			t.Fatalf("read xlsx xml %s: %v", file.Name, readErr)
		}
		if closeErr != nil {
			t.Fatalf("close xlsx xml %s: %v", file.Name, closeErr)
		}
		xmlText.Write(content)
		xmlText.WriteByte('\n')
	}

	payload := xmlText.String()
	for _, value := range []string{"record_type", sentinel.RecordType, sentinel.Path} {
		if !strings.Contains(payload, value) {
			t.Fatalf("expected xlsx takeout XML to contain %q", value)
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

func readCSVTakeout(t *testing.T, filename string) [][]string {
	t.Helper()

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open export file: %v", err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least header and one row, got %d rows", len(rows))
	}
	return rows
}

func requireCSVTakeoutRow(t *testing.T, rows [][]string, match map[string]string, want map[string]string) {
	t.Helper()

	columns := make([]string, 0, len(match)+len(want))
	for column := range match {
		columns = append(columns, column)
	}
	for column := range want {
		columns = append(columns, column)
	}

	index := takeoutHeaderIndex(t, rows[0], columns...)
	for _, row := range rows[1:] {
		if !csvRowMatches(row, index, match) {
			continue
		}
		assertCSVRowValues(t, row, index, want)
		return
	}
	t.Fatalf("expected takeout row matching %#v, got %#v", match, rows)
}

func csvRowMatches(row []string, index map[string]int, match map[string]string) bool {
	for column, want := range match {
		if row[index[column]] != want {
			return false
		}
	}
	return true
}

func assertCSVRowValues(t *testing.T, row []string, index map[string]int, want map[string]string) {
	t.Helper()

	for column, want := range want {
		if got := row[index[column]]; got != want {
			t.Fatalf("expected %s %q, got %q", column, want, got)
		}
	}
}

func takeoutString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}
