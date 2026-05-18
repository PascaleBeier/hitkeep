package worker

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/importables"
)

// newTestStore creates a file-backed DuckDB store for testing.
func newTestStore(t *testing.T) *database.Store {
	t.Helper()
	tmpDir := t.TempDir()
	store := database.NewStore(filepath.Join(tmpDir, "test.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store
}

// newTestTenantMgr creates a TenantStoreManager backed by the given shared store.
func newTestTenantMgr(t *testing.T, store *database.Store) *database.TenantStoreManager {
	t.Helper()
	mgr := database.NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })
	return mgr
}

// seedSite creates a user and site with the given retention policy in days.
func seedSite(t *testing.T, ctx context.Context, store *database.Store, retentionDays int) (siteID uuid.UUID) {
	t.Helper()
	userID, err := store.CreateUser(ctx, fmt.Sprintf("user-%s@example.com", uuid.New()), "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "test.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, "UPDATE sites SET data_retention_days = ? WHERE id = ?", retentionDays, site.ID); err != nil {
		t.Fatalf("set retention policy: %v", err)
	}
	return site.ID
}

func findParquetFiles(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".parquet") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return files, nil
}

// TestRetentionArchivesAndPrunesUTMHits verifies that old hits and events are
// exported to a Parquet file and pruned from hot storage, and that UTM fields
// survive the archive round-trip.
func TestRetentionArchivesAndPrunesUTMHits(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	archiveDir := filepath.Join(t.TempDir(), "archive")
	siteID := seedSite(t, ctx, store, 1)

	old := time.Now().UTC().Add(-48 * time.Hour)
	isUnique := true
	region := "California"
	city := "Mountain View"
	provider := "Google LLC"
	asn := 15169
	asnOrg := "Google LLC"
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:      siteID,
		SessionID:   uuid.New(),
		PageID:      uuid.New(),
		Timestamp:   old,
		Path:        "/old-utm",
		UTMSource:   new("search"),
		UTMMedium:   new("paid"),
		UTMCampaign: new("retention-check"),
		UTMTerm:     new("audit"),
		UTMContent:  new("copy-a"),
		IsUnique:    &isUnique,
		Region:      &region,
		City:        &city,
		Provider:    &provider,
		ASN:         &asn,
		ASNOrg:      &asnOrg,
	}); err != nil {
		t.Fatalf("create hit: %v", err)
	}
	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:     siteID,
		SessionID:  uuid.New(),
		Name:       "old_event",
		Properties: map[string]any{"kind": "test"},
		Timestamp:  old,
	}); err != nil {
		t.Fatalf("create event: %v", err)
	}

	w := NewRetentionWorker(newTestTenantMgr(t, store), archiveDir, 365, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("run retention: %v", err)
	}

	// Archive file must exist.
	files, err := findParquetFiles(archiveDir)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected parquet archive file, found: %v", files)
	}
	archivePath := files[0]

	// UTM fields survive in cold storage.
	safePath := strings.ReplaceAll(archivePath, "'", "''")
	var utmSource, utmCampaign sql.NullString
	if err := store.DB().QueryRowContext(ctx,
		fmt.Sprintf("SELECT utm_source, utm_campaign FROM read_parquet('%s') WHERE utm_source IS NOT NULL LIMIT 1", safePath),
	).Scan(&utmSource, &utmCampaign); err != nil {
		t.Fatalf("query archived parquet: %v", err)
	}
	if !utmSource.Valid || utmSource.String != "search" {
		t.Fatalf("expected utm_source=search, got %q (valid=%v)", utmSource.String, utmSource.Valid)
	}
	if !utmCampaign.Valid || utmCampaign.String != "retention-check" {
		t.Fatalf("expected utm_campaign=retention-check, got %q (valid=%v)", utmCampaign.String, utmCampaign.Valid)
	}
	var archivedCity, archivedProvider, archivedASNOrg sql.NullString
	var archivedASN sql.NullInt32
	if err := store.DB().QueryRowContext(ctx,
		fmt.Sprintf("SELECT city, provider, asn, asn_org FROM read_parquet('%s') WHERE city IS NOT NULL LIMIT 1", safePath),
	).Scan(&archivedCity, &archivedProvider, &archivedASN, &archivedASNOrg); err != nil {
		t.Fatalf("query archived geo/network fields: %v", err)
	}
	if !archivedCity.Valid || archivedCity.String != "Mountain View" {
		t.Fatalf("expected archived city Mountain View, got %q (valid=%v)", archivedCity.String, archivedCity.Valid)
	}
	if !archivedProvider.Valid || archivedProvider.String != "Google LLC" {
		t.Fatalf("expected archived provider Google LLC, got %q (valid=%v)", archivedProvider.String, archivedProvider.Valid)
	}
	if !archivedASN.Valid || archivedASN.Int32 != 15169 {
		t.Fatalf("expected archived asn 15169, got %d (valid=%v)", archivedASN.Int32, archivedASN.Valid)
	}
	if !archivedASNOrg.Valid || archivedASNOrg.String != "Google LLC" {
		t.Fatalf("expected archived asn_org Google LLC, got %q (valid=%v)", archivedASNOrg.String, archivedASNOrg.Valid)
	}
	var source string
	if err := store.DB().QueryRowContext(ctx,
		fmt.Sprintf("SELECT _source FROM read_parquet('%s') WHERE utm_source IS NOT NULL LIMIT 1", safePath),
	).Scan(&source); err != nil {
		t.Fatalf("query archive source discriminator: %v", err)
	}
	if source != "hits" {
		t.Fatalf("expected _source=hits for archived hit row, got %q", source)
	}

	// Hot storage is empty.
	var remaining int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", siteID).Scan(&remaining); err != nil {
		t.Fatalf("count remaining hits: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected 0 hits in hot storage after retention, got %d", remaining)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE site_id = ?", siteID).Scan(&remaining); err != nil {
		t.Fatalf("count remaining events: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected 0 events in hot storage after retention, got %d", remaining)
	}
}

// TestRetentionColdDataReadback verifies that archived Parquet files are fully
// queryable as cold storage: row counts, field values, and event properties all
// round-trip correctly through the archive-and-prune cycle.
func TestRetentionColdDataReadback(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	archiveDir := filepath.Join(t.TempDir(), "archive")
	siteID := seedSite(t, ctx, store, 1)

	old := time.Now().UTC().Add(-72 * time.Hour)
	isUnique := true

	// Insert 3 old hits with distinct paths.
	for i, path := range []string{"/page-a", "/page-b", "/page-c"} {
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID:    siteID,
			SessionID: uuid.New(),
			PageID:    uuid.New(),
			Timestamp: old.Add(time.Duration(i) * time.Minute),
			Path:      path,
			IsUnique:  &isUnique,
		}); err != nil {
			t.Fatalf("create hit %d: %v", i, err)
		}
	}

	// Insert 2 old events with distinct names.
	for _, name := range []string{"signup", "purchase"} {
		if err := store.CreateEvent(ctx, &api.Event{
			SiteID:     siteID,
			SessionID:  uuid.New(),
			Name:       name,
			Properties: map[string]any{"plan": "pro"},
			Timestamp:  old,
		}); err != nil {
			t.Fatalf("create event %q: %v", name, err)
		}
	}

	w := NewRetentionWorker(newTestTenantMgr(t, store), archiveDir, 365, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("run retention: %v", err)
	}

	// Locate archive file.
	files, err := findParquetFiles(archiveDir)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no parquet archive written")
	}
	archivePath := files[0]
	safePath := strings.ReplaceAll(archivePath, "'", "''")

	// Total rows = 3 hits + 2 events = 5.
	var total int
	if err := store.DB().QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM read_parquet('%s')", safePath),
	).Scan(&total); err != nil {
		t.Fatalf("count archived rows: %v", err)
	}
	if total != 5 {
		t.Fatalf("expected 5 archived rows (3 hits + 2 events), got %d", total)
	}

	// Hit paths are preserved.
	var hitCount int
	if err := store.DB().QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(DISTINCT path) FROM read_parquet('%s') WHERE path IS NOT NULL AND path != ''", safePath),
	).Scan(&hitCount); err != nil {
		t.Fatalf("count distinct paths: %v", err)
	}
	if hitCount != 3 {
		t.Fatalf("expected 3 distinct paths in archive, got %d", hitCount)
	}

	// Hot storage is fully pruned.
	var hotHits, hotEvents int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", siteID).Scan(&hotHits); err != nil {
		t.Fatalf("count hot hits: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE site_id = ?", siteID).Scan(&hotEvents); err != nil {
		t.Fatalf("count hot events: %v", err)
	}
	if hotHits != 0 || hotEvents != 0 {
		t.Fatalf("expected hot storage empty after retention, got %d hits %d events", hotHits, hotEvents)
	}
}

// TestRetentionHotDataNotArchived verifies that data within the retention window
// is never touched by the retention worker — it stays in hot storage and is not
// written to any archive file.
func TestRetentionHotDataNotArchived(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	archiveDir := filepath.Join(t.TempDir(), "archive")
	siteID := seedSite(t, ctx, store, 30)

	// "recent" = 1 day ago, within 30-day retention window.
	recent := time.Now().UTC().Add(-24 * time.Hour)
	isUnique := true
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    siteID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: recent,
		Path:      "/recent-page",
		IsUnique:  &isUnique,
	}); err != nil {
		t.Fatalf("create recent hit: %v", err)
	}

	w := NewRetentionWorker(newTestTenantMgr(t, store), archiveDir, 365, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("run retention: %v", err)
	}

	// No archive files should have been created — nothing was past the cutoff.
	entries, err := findParquetFiles(archiveDir)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(strings.ToLower(e), ".parquet") {
			t.Fatalf("unexpected parquet file for in-window data: %s", e)
		}
	}

	// Recent hit must still be in hot storage.
	var count int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", siteID).Scan(&count); err != nil {
		t.Fatalf("count hot hits: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 recent hit still in hot storage, got %d", count)
	}
}

// TestRetentionMixedWindowArchivesOnlyStale verifies that when a site has both
// stale and recent data, only stale data is archived and recent data remains in
// hot storage untouched.
func TestRetentionMixedWindowArchivesOnlyStale(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	archiveDir := filepath.Join(t.TempDir(), "archive")
	siteID := seedSite(t, ctx, store, 7) // 7-day retention

	isUnique := true
	old := time.Now().UTC().Add(-14 * 24 * time.Hour)   // 14 days ago — stale
	recent := time.Now().UTC().Add(-2 * 24 * time.Hour) // 2 days ago — within window

	if err := store.CreateHit(ctx, &api.Hit{
		SiteID: siteID, SessionID: uuid.New(), PageID: uuid.New(),
		Timestamp: old, Path: "/stale", IsUnique: &isUnique,
	}); err != nil {
		t.Fatalf("create stale hit: %v", err)
	}
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID: siteID, SessionID: uuid.New(), PageID: uuid.New(),
		Timestamp: recent, Path: "/recent", IsUnique: &isUnique,
	}); err != nil {
		t.Fatalf("create recent hit: %v", err)
	}

	w := NewRetentionWorker(newTestTenantMgr(t, store), archiveDir, 365, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("run retention: %v", err)
	}

	// Exactly 1 row archived (the stale hit).
	files, err := findParquetFiles(archiveDir)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected archive file for stale data")
	}
	archivePath := files[0]
	safePath := strings.ReplaceAll(archivePath, "'", "''")
	var archived int
	if err := store.DB().QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM read_parquet('%s')", safePath),
	).Scan(&archived); err != nil {
		t.Fatalf("count archived: %v", err)
	}
	if archived != 1 {
		t.Fatalf("expected 1 archived row, got %d", archived)
	}

	// Recent hit must remain in hot storage.
	var hot int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM hits WHERE site_id = ? AND path = '/recent'", siteID,
	).Scan(&hot); err != nil {
		t.Fatalf("count hot recent hits: %v", err)
	}
	if hot != 1 {
		t.Fatalf("expected recent hit in hot storage, got %d", hot)
	}
}

func TestRetentionArchivesAndPrunesWebVitals(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	archiveDir := filepath.Join(t.TempDir(), "archive")
	siteID := seedSite(t, ctx, store, 7)

	old := time.Now().UTC().Add(-14 * 24 * time.Hour)
	recent := time.Now().UTC().Add(-2 * 24 * time.Hour)
	seedRetentionWebVital(t, ctx, store, siteID, api.WebVitalLCP, 2600, "/stale-vitals", old)
	seedRetentionWebVital(t, ctx, store, siteID, api.WebVitalINP, 180, "/recent-vitals", recent)

	w := NewRetentionWorker(newTestTenantMgr(t, store), archiveDir, 365, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("run retention: %v", err)
	}

	files, err := findParquetFiles(archiveDir)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected archive file for stale web vital")
	}

	assertArchivedWebVital(t, ctx, store, files[0], api.WebVitalLCP, "/stale-vitals", api.WebVitalRatingNeedsImprovement)
	assertHotWebVitalCount(t, ctx, store, siteID, "/recent-vitals", 1)
	assertHotWebVitalCount(t, ctx, store, siteID, "/stale-vitals", 0)
}

func TestRetentionArchivesAndPrunesImportedEventDimensions(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	archiveDir := filepath.Join(t.TempDir(), "archive")
	siteID := seedSite(t, ctx, store, 7)

	old := time.Now().UTC().Add(-14 * 24 * time.Hour)
	sink, err := database.NewImportedDataSink(ctx, store, siteID, uuid.New())
	if err != nil {
		t.Fatalf("new imported sink: %v", err)
	}
	for _, row := range []importables.EventDimensionRow{
		{Date: old, EventName: "signup", Dimension: "city", Name: "Dortmund", Visitors: 7, Events: 9, SourceFile: "imported_event_dimensions.csv"},
		{Date: old, EventName: "signup", Dimension: "provider", Name: "Deutsche Telekom AG", Visitors: 6, Events: 8, SourceFile: "imported_event_dimensions.csv"},
		{Date: old, EventName: "signup", Dimension: "asn", Name: "AS3320 Deutsche Telekom AG", Visitors: 5, Events: 7, SourceFile: "imported_event_dimensions.csv"},
	} {
		if err := sink.PutEventDimension(ctx, row); err != nil {
			t.Fatalf("put imported event dimension %s: %v", row.Dimension, err)
		}
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush imported event dimensions: %v", err)
	}

	w := NewRetentionWorker(newTestTenantMgr(t, store), archiveDir, 365, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("run retention: %v", err)
	}

	files, err := findParquetFiles(archiveDir)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected archive file for stale imported event dimensions")
	}
	safePath := strings.ReplaceAll(files[0], "'", "''")

	var archivedCity string
	if err := store.DB().QueryRowContext(ctx,
		fmt.Sprintf("SELECT name FROM read_parquet('%s') WHERE _source = 'imported_event_dimensions_daily' AND dimension = 'city'", safePath),
	).Scan(&archivedCity); err != nil {
		t.Fatalf("query archived imported city dimension: %v", err)
	}
	if archivedCity != "Dortmund" {
		t.Fatalf("expected archived imported city Dortmund, got %q", archivedCity)
	}

	var remaining int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM imported_event_dimensions_daily WHERE site_id = ?", siteID).Scan(&remaining); err != nil {
		t.Fatalf("count hot imported event dimensions: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected imported event dimensions to be pruned from hot storage, got %d", remaining)
	}
}

func seedRetentionWebVital(t *testing.T, ctx context.Context, store *database.Store, siteID uuid.UUID, metric api.WebVitalMetric, value float64, path string, ts time.Time) {
	t.Helper()
	if err := store.CreateWebVital(ctx, &api.WebVital{
		SiteID:         siteID,
		SessionID:      uuid.New(),
		PageID:         uuid.New(),
		Metric:         metric,
		Value:          value,
		Path:           path,
		Timestamp:      ts,
		TrackerSource:  "browser",
		TrackerVersion: "test",
	}); err != nil {
		t.Fatalf("create web vital: %v", err)
	}
}

func assertArchivedWebVital(t *testing.T, ctx context.Context, store *database.Store, archivePath string, metric api.WebVitalMetric, path string, rating api.WebVitalRating) {
	t.Helper()
	safePath := strings.ReplaceAll(archivePath, "'", "''")
	var archivedMetric, archivedPath, archivedRating string
	if err := store.DB().QueryRowContext(ctx,
		fmt.Sprintf("SELECT metric, path, rating FROM read_parquet('%s') WHERE _source = 'web_vitals'", safePath),
	).Scan(&archivedMetric, &archivedPath, &archivedRating); err != nil {
		t.Fatalf("query archived web vital: %v", err)
	}
	if archivedMetric != string(metric) || archivedPath != path || archivedRating != string(rating) {
		t.Fatalf("unexpected archived web vital: metric=%q path=%q rating=%q", archivedMetric, archivedPath, archivedRating)
	}
}

func assertHotWebVitalCount(t *testing.T, ctx context.Context, store *database.Store, siteID uuid.UUID, path string, expected int) {
	t.Helper()
	var count int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM web_vitals WHERE site_id = ? AND path = ?", siteID, path,
	).Scan(&count); err != nil {
		t.Fatalf("count hot web vitals: %v", err)
	}
	if count != expected {
		t.Fatalf("expected %d web vitals for %q in hot storage, got %d", expected, path, count)
	}
}

func TestRetentionPrunesOldAIFetches(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	archiveDir := filepath.Join(t.TempDir(), "archive")
	siteID := seedSite(t, ctx, store, 7)

	old := time.Now().UTC().Add(-14 * 24 * time.Hour)
	recent := time.Now().UTC().Add(-2 * 24 * time.Hour)

	if err := store.CreateAIFetch(ctx, &api.AIFetch{
		SiteID:          siteID,
		Timestamp:       old,
		AssistantName:   "GPTBot",
		AssistantFamily: "OpenAI",
		Path:            "/old-doc",
		StatusCode:      200,
		ResourceType:    "html",
	}); err != nil {
		t.Fatalf("create old ai fetch: %v", err)
	}
	if err := store.CreateAIFetch(ctx, &api.AIFetch{
		SiteID:          siteID,
		Timestamp:       recent,
		AssistantName:   "GPTBot",
		AssistantFamily: "OpenAI",
		Path:            "/recent-doc",
		StatusCode:      200,
		ResourceType:    "html",
	}); err != nil {
		t.Fatalf("create recent ai fetch: %v", err)
	}

	w := NewRetentionWorker(newTestTenantMgr(t, store), archiveDir, 365, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("run retention: %v", err)
	}

	var oldCount, recentCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_fetches WHERE site_id = ? AND path = '/old-doc'", siteID).Scan(&oldCount); err != nil {
		t.Fatalf("count old ai fetches: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_fetches WHERE site_id = ? AND path = '/recent-doc'", siteID).Scan(&recentCount); err != nil {
		t.Fatalf("count recent ai fetches: %v", err)
	}
	if oldCount != 0 {
		t.Fatalf("expected old ai fetch to be pruned, got %d rows", oldCount)
	}
	if recentCount != 1 {
		t.Fatalf("expected recent ai fetch to remain, got %d rows", recentCount)
	}
}

func TestRetentionArchivesToTenantScopedPathForNonDefaultTenant(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	archiveDir := filepath.Join(t.TempDir(), "archive")
	siteID := seedSite(t, ctx, store, 1)

	customTenantID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		customTenantID,
		"Custom Tenant",
		time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert custom tenant: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, "UPDATE site_tenants SET tenant_id = ? WHERE site_id = ?", customTenantID, siteID); err != nil {
		t.Fatalf("update site tenant mapping: %v", err)
	}

	mgr := newTestTenantMgr(t, store)

	// Insert hit into the non-default tenant's store (where retention now looks).
	tenantStore, err := mgr.ForTenant(ctx, customTenantID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}

	old := time.Now().UTC().Add(-48 * time.Hour)
	isUnique := true
	if err := tenantStore.CreateHit(ctx, &api.Hit{
		SiteID: siteID, SessionID: uuid.New(), PageID: uuid.New(),
		Timestamp: old, Path: "/tenant-isolated", IsUnique: &isUnique,
	}); err != nil {
		t.Fatalf("create stale hit: %v", err)
	}

	w := NewRetentionWorker(mgr, archiveDir, 365, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("run retention: %v", err)
	}

	files, err := findParquetFiles(archiveDir)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 parquet archive file, got %d (%v)", len(files), files)
	}

	expectedTenantSegment := filepath.Join("tenants", customTenantID.String(), "sites", siteID.String())
	if !strings.Contains(files[0], expectedTenantSegment) {
		t.Fatalf("expected archive path to include tenant isolation segment %q, got %q", expectedTenantSegment, files[0])
	}
}

func TestArchiveFilenameLocalDefaultTenantKeepsLegacyLayout(t *testing.T) {
	w := NewRetentionWorker(nil, filepath.Join("tmp", "archive"), 365, nil)
	siteID := uuid.New()
	defaultTenantID := uuid.New()

	path := w.archiveFilename(siteID, defaultTenantID, defaultTenantID)
	if strings.Contains(path, string(filepath.Separator)+"tenants"+string(filepath.Separator)) {
		t.Fatalf("expected legacy local path for default tenant, got %q", path)
	}
	if !strings.Contains(path, fmt.Sprintf("site_%s_", siteID)) {
		t.Fatalf("expected legacy filename format in %q", path)
	}
}

func TestArchiveFilenameLocalNonDefaultTenantIsIsolated(t *testing.T) {
	w := NewRetentionWorker(nil, filepath.Join("tmp", "archive"), 365, nil)
	siteID := uuid.New()
	defaultTenantID := uuid.New()
	tenantID := uuid.New()

	path := w.archiveFilename(siteID, tenantID, defaultTenantID)
	expectedSegment := filepath.Join("tenants", tenantID.String(), "sites", siteID.String())
	if !strings.Contains(path, expectedSegment) {
		t.Fatalf("expected tenant-isolated local path segment %q, got %q", expectedSegment, path)
	}
}

func TestArchiveFilenameS3AlwaysIsolatesTenants(t *testing.T) {
	w := NewRetentionWorker(nil, "s3://hitkeep-bucket/datalake", 365, nil)
	siteID := uuid.New()
	defaultTenantID := uuid.New()

	path := w.archiveFilename(siteID, defaultTenantID, defaultTenantID)
	expectedPrefix := fmt.Sprintf("s3://hitkeep-bucket/datalake/tenants/%s/sites/%s/", defaultTenantID, siteID)
	if !strings.HasPrefix(path, expectedPrefix) {
		t.Fatalf("expected S3 tenant-isolated path prefix %q, got %q", expectedPrefix, path)
	}
}

func TestBuildS3SecretQueryNilConfig(t *testing.T) {
	query := database.BuildS3SecretQuery(nil)
	if query != "" {
		t.Fatalf("expected empty string for nil config, got %q", query)
	}
}

func TestBuildS3SecretQueryStaticCredentials(t *testing.T) {
	cfg := &S3Config{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Region:          "eu-west-1",
		UseSSL:          true,
	}
	query := database.BuildS3SecretQuery(cfg)

	if !strings.Contains(query, "PROVIDER config") {
		t.Fatalf("expected PROVIDER config, got %q", query)
	}
	if !strings.Contains(query, "KEY_ID 'AKIAIOSFODNN7EXAMPLE'") {
		t.Fatalf("expected KEY_ID in query, got %q", query)
	}
	if !strings.Contains(query, "SECRET 'wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY'") {
		t.Fatalf("expected SECRET in query, got %q", query)
	}
	if !strings.Contains(query, "REGION 'eu-west-1'") {
		t.Fatalf("expected REGION in query, got %q", query)
	}
	if strings.Contains(query, "SESSION_TOKEN") {
		t.Fatalf("did not expect SESSION_TOKEN when not set, got %q", query)
	}
	if strings.Contains(query, "USE_SSL false") {
		t.Fatalf("did not expect USE_SSL false when UseSSL=true, got %q", query)
	}
}

func TestBuildS3SecretQueryCredentialChain(t *testing.T) {
	cfg := &S3Config{
		Region: "us-east-1",
		UseSSL: true,
	}
	query := database.BuildS3SecretQuery(cfg)

	if !strings.Contains(query, "PROVIDER credential_chain") {
		t.Fatalf("expected PROVIDER credential_chain, got %q", query)
	}
	if strings.Contains(query, "KEY_ID") {
		t.Fatalf("did not expect KEY_ID for credential_chain, got %q", query)
	}
	if strings.Contains(query, "SECRET '") {
		t.Fatalf("did not expect SECRET key for credential_chain, got %q", query)
	}
	if !strings.Contains(query, "REGION 'us-east-1'") {
		t.Fatalf("expected REGION in query, got %q", query)
	}
}

func TestBuildS3SecretQueryWithEndpoint(t *testing.T) {
	cfg := &S3Config{
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "minioadmin",
		Region:          "us-east-1",
		Endpoint:        "localhost:9000",
		URLStyle:        "path",
		UseSSL:          false,
	}
	query := database.BuildS3SecretQuery(cfg)

	if !strings.Contains(query, "ENDPOINT 'localhost:9000'") {
		t.Fatalf("expected ENDPOINT in query, got %q", query)
	}
	if !strings.Contains(query, "URL_STYLE 'path'") {
		t.Fatalf("expected URL_STYLE in query, got %q", query)
	}
	if !strings.Contains(query, "USE_SSL false") {
		t.Fatalf("expected USE_SSL false in query, got %q", query)
	}
}

func TestBuildS3SecretQueryWithSessionToken(t *testing.T) {
	cfg := &S3Config{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "FwoGZXIvYXdzEBYaDH...",
		Region:          "us-east-1",
		UseSSL:          true,
	}
	query := database.BuildS3SecretQuery(cfg)

	if !strings.Contains(query, "SESSION_TOKEN 'FwoGZXIvYXdzEBYaDH...'") {
		t.Fatalf("expected SESSION_TOKEN in query, got %q", query)
	}
}

func TestBuildS3SecretQueryEscapesSingleQuotes(t *testing.T) {
	cfg := &S3Config{
		AccessKeyID:     "key'with'quotes",
		SecretAccessKey: "secret'value",
		Region:          "us-east-1",
		UseSSL:          true,
	}
	query := database.BuildS3SecretQuery(cfg)

	if !strings.Contains(query, "KEY_ID 'key''with''quotes'") {
		t.Fatalf("expected escaped KEY_ID, got %q", query)
	}
	if !strings.Contains(query, "SECRET 'secret''value'") {
		t.Fatalf("expected escaped SECRET, got %q", query)
	}
}
