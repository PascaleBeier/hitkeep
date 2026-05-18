package database

import (
	"bytes"
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/exportfmt"
)

func TestExportHitsCSVIncludesUTMFields(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "hits-export.db")

	store := NewStore(dbPath)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	userID, err := store.CreateUser(ctx, "utm-export@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "utm-export.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	isUnique := true
	hit := &api.Hit{
		SiteID:      site.ID,
		SessionID:   uuid.New(),
		PageID:      uuid.New(),
		Timestamp:   now,
		Path:        "/landing",
		UTMSource:   new("google"),
		UTMMedium:   new("cpc"),
		UTMCampaign: new("spring-launch"),
		UTMTerm:     new("privacy analytics"),
		UTMContent:  new("hero-cta"),
		IsUnique:    &isUnique,
	}
	if err := store.CreateHit(ctx, hit); err != nil {
		t.Fatalf("create hit: %v", err)
	}

	params := api.HitQueryParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  now.Add(-time.Hour),
		End:    now.Add(time.Hour),
		Limit:  10,
		Offset: 0,
	}

	var buf bytes.Buffer
	if err := store.ExportHitsCSV(ctx, params, &buf); err != nil {
		t.Fatalf("export csv: %v", err)
	}

	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least header and one row, got %d rows", len(rows))
	}

	header := rows[0]
	row := rows[1]
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}

	for _, col := range []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"} {
		if _, ok := index[col]; !ok {
			t.Fatalf("expected header column %q to exist", col)
		}
	}

	if got := row[index["utm_source"]]; got != "google" {
		t.Fatalf("expected utm_source=google, got %q", got)
	}
	if got := row[index["utm_medium"]]; got != "cpc" {
		t.Fatalf("expected utm_medium=cpc, got %q", got)
	}
	if got := row[index["utm_campaign"]]; got != "spring-launch" {
		t.Fatalf("expected utm_campaign=spring-launch, got %q", got)
	}
	if got := row[index["utm_term"]]; got != "privacy analytics" {
		t.Fatalf("expected utm_term=privacy analytics, got %q", got)
	}
	if got := row[index["utm_content"]]; got != "hero-cta" {
		t.Fatalf("expected utm_content=hero-cta, got %q", got)
	}
}

func TestExportHitsCSVIncludesGeoNetworkFields(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "hits-geo-export.db")

	store := NewStore(dbPath)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	userID, err := store.CreateUser(ctx, "geo-export@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "geo-export.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	region := "California"
	city := "Mountain View"
	provider := "Google LLC"
	asn := 15169
	asnOrg := "Google LLC"
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: now,
		Path:      "/geo",
		Region:    &region,
		City:      &city,
		Provider:  &provider,
		ASN:       &asn,
		ASNOrg:    &asnOrg,
	}); err != nil {
		t.Fatalf("create hit: %v", err)
	}

	var buf bytes.Buffer
	if err := store.ExportHitsCSV(ctx, api.HitQueryParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  now.Add(-time.Hour),
		End:    now.Add(time.Hour),
		Limit:  10,
	}, &buf); err != nil {
		t.Fatalf("export csv: %v", err)
	}

	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least header and one row, got %d rows", len(rows))
	}

	index := make(map[string]int, len(rows[0]))
	for i, name := range rows[0] {
		index[name] = i
	}
	for _, col := range []string{"region", "city", "provider", "asn", "asn_org"} {
		if _, ok := index[col]; !ok {
			t.Fatalf("expected header column %q to exist", col)
		}
	}

	row := rows[1]
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
}

func TestExportHitsFileSupportsAllFormats(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "hits-export-file.db")

	store := NewStore(dbPath)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	userID, err := store.CreateUser(ctx, "hits-file-export@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "hits-file-export.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	isUnique := true
	hit := &api.Hit{
		SiteID:      site.ID,
		SessionID:   uuid.New(),
		PageID:      uuid.New(),
		Timestamp:   now,
		Path:        "/landing",
		UTMSource:   new("google"),
		UTMMedium:   new("cpc"),
		UTMCampaign: new("spring-launch"),
		UTMTerm:     new("privacy analytics"),
		UTMContent:  new("hero-cta"),
		IsUnique:    &isUnique,
	}
	if err := store.CreateHit(ctx, hit); err != nil {
		t.Fatalf("create hit: %v", err)
	}

	params := api.HitQueryParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  now.Add(-time.Hour),
		End:    now.Add(time.Hour),
		Limit:  10,
		Offset: 0,
	}

	tests := []struct {
		name   string
		format string
		want   string
	}{
		{name: "csv", format: "csv", want: ".csv"},
		{name: "xlsx", format: "xlsx", want: ".xlsx"},
		{name: "parquet", format: "parquet", want: ".parquet"},
		{name: "json", format: "json", want: ".json"},
		{name: "ndjson", format: "ndjson", want: ".ndjson"},
		{name: "unknown defaults to csv", format: "xml", want: ".csv"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filename, err := store.ExportHitsFile(ctx, params, tc.format)
			if err != nil {
				t.Fatalf("export hits file: %v", err)
			}
			t.Cleanup(func() { _ = os.Remove(filename) })

			if got := strings.ToLower(filepath.Ext(filename)); got != tc.want {
				t.Fatalf("expected extension %q, got %q", tc.want, got)
			}

			contentType := exportfmt.ContentType(strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), "."))
			if contentType == "" {
				t.Fatalf("expected content type mapping for %q", filename)
			}

			info, err := os.Stat(filename)
			if err != nil {
				t.Fatalf("stat export file: %v", err)
			}
			if info.Size() == 0 {
				t.Fatalf("expected non-empty export file for format %q", tc.format)
			}
		})
	}
}
