package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

const dropIndexesMigrationFile = "2026_07_08_000000_drop_analytics_art_indexes.sql"
const dropMutableControlIndexesMigrationFile = "2026_07_17_000000_drop_mutable_control_art_indexes.sql"

var rebuiltAnalyticsTables = []string{"hits", "events", "web_vitals"}
var removedGoogleSearchConsoleIndexes = []string{
	"idx_gsc_connections_connected",
	"idx_gsc_properties_team",
	"idx_gsc_site_mappings_team",
	"idx_gsc_sync_state_next_retry",
	"idx_gsc_sync_state_team_state",
}

func countArtIndexState(t *testing.T, store *Store, table string) (indexes int, keyConstraints int) {
	t.Helper()
	ctx := context.Background()
	if err := store.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM duckdb_indexes() WHERE table_name = ?", table).Scan(&indexes); err != nil {
		t.Fatalf("count indexes for %s: %v", table, err)
	}
	if err := store.DB().QueryRowContext(ctx, `
		SELECT count(*) FROM duckdb_constraints()
		WHERE table_name = ? AND constraint_type IN ('PRIMARY KEY', 'UNIQUE', 'FOREIGN KEY')`, table).Scan(&keyConstraints); err != nil {
		t.Fatalf("count key constraints for %s: %v", table, err)
	}
	return indexes, keyConstraints
}

// TestDropAnalyticsIndexesMigrationPreservesData replays the upgrade path: it
// migrates a database to the state just before the index-drop migration,
// writes traffic, then applies only the new migration and verifies the data,
// defaults, and NOT NULL constraints survive while every ART index is gone.
func TestDropAnalyticsIndexesMigrationPreservesData(t *testing.T) {
	ctx := context.Background()
	store := NewStore(filepath.Join(t.TempDir(), "upgrade.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Hold the new migration back so Migrate() reproduces the pre-upgrade schema.
	if _, err := store.DB().ExecContext(ctx,
		"CREATE TABLE IF NOT EXISTS migrations (migration VARCHAR PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL)"); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO migrations (migration, applied_at) VALUES (?, ?)", dropIndexesMigrationFile, time.Now().UTC()); err != nil {
		t.Fatalf("hold back migration: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate to pre-upgrade state: %v", err)
	}

	for _, table := range rebuiltAnalyticsTables {
		indexes, keys := countArtIndexState(t, store, table)
		if indexes == 0 && keys == 0 {
			t.Fatalf("expected pre-upgrade %s to carry ART indexes", table)
		}
	}

	userID, err := store.CreateUser(ctx, "upgrade@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "upgrade.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	sessionID := uuid.New()
	pageID := uuid.New()
	if err := store.CreateHitsBulk(ctx, []*api.Hit{
		{ID: uuid.New(), SiteID: site.ID, SessionID: sessionID, PageID: pageID, Timestamp: time.Now().UTC(), Path: "/a"},
		{ID: uuid.New(), SiteID: site.ID, SessionID: sessionID, PageID: uuid.New(), Timestamp: time.Now().UTC(), Path: "/b"},
	}); err != nil {
		t.Fatalf("insert hits pre-upgrade: %v", err)
	}
	if err := store.CreateEvent(ctx, &api.Event{
		ID: uuid.New(), SiteID: site.ID, SessionID: sessionID, Name: "signup",
		Properties: map[string]any{"plan": "pro"}, Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("insert event pre-upgrade: %v", err)
	}
	if err := store.CreateWebVital(ctx, &api.WebVital{
		ID: uuid.New(), SiteID: site.ID, SessionID: sessionID, PageID: pageID,
		Metric: api.WebVitalLCP, Value: 1200, Rating: api.WebVitalRatingGood,
		Path: "/a", Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("insert web vital pre-upgrade: %v", err)
	}

	// Release the held-back migration and apply it against populated tables.
	if _, err := store.DB().ExecContext(ctx,
		"DELETE FROM migrations WHERE migration = ?", dropIndexesMigrationFile); err != nil {
		t.Fatalf("release migration: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("apply index-drop migration: %v", err)
	}

	for _, table := range rebuiltAnalyticsTables {
		indexes, keys := countArtIndexState(t, store, table)
		if indexes != 0 || keys != 0 {
			t.Fatalf("expected %s to have no ART indexes after migration, got %d indexes and %d key constraints", table, indexes, keys)
		}
	}

	var hitCount, eventCount, vitalCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT count(*) FROM hits").Scan(&hitCount); err != nil {
		t.Fatalf("count hits: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT count(*) FROM events").Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT count(*) FROM web_vitals").Scan(&vitalCount); err != nil {
		t.Fatalf("count web vitals: %v", err)
	}
	if hitCount != 2 || eventCount != 1 || vitalCount != 1 {
		t.Fatalf("expected data preserved (2 hits, 1 event, 1 vital), got %d/%d/%d", hitCount, eventCount, vitalCount)
	}

	// The id default must survive the rebuild.
	var generatedID uuid.UUID
	if err := store.DB().QueryRowContext(ctx, `
		INSERT INTO hits (site_id, session_id, page_id, timestamp, path)
		VALUES (?, ?, ?, now(), '/default-check') RETURNING id`,
		site.ID, uuid.New(), uuid.New()).Scan(&generatedID); err != nil {
		t.Fatalf("insert relying on id default: %v", err)
	}
	if generatedID == uuid.Nil {
		t.Fatal("expected rebuilt hits to keep the uuidv7 id default")
	}

	// NOT NULL constraints must survive the rebuild.
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO hits (id, site_id, session_id, page_id, timestamp) VALUES (?, ?, ?, ?, now())",
		uuid.New(), site.ID, uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected NOT NULL on hits.path to survive the rebuild")
	}

	// The appender ingest path must keep working against the rebuilt tables.
	if err := store.CreateHitsBulk(ctx, []*api.Hit{
		{ID: uuid.New(), SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: time.Now().UTC(), Path: "/post-upgrade"},
	}); err != nil {
		t.Fatalf("insert hits post-upgrade: %v", err)
	}
}

func TestDroppingGSCControlIndexesPreservesSiteCleanup(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)

	var unsafeIndexes int
	if err := store.DB().QueryRowContext(ctx, `
		SELECT count(*)
		FROM duckdb_indexes()
		WHERE index_name IN (?, ?, ?, ?, ?)`,
		removedGoogleSearchConsoleIndexes[0],
		removedGoogleSearchConsoleIndexes[1],
		removedGoogleSearchConsoleIndexes[2],
		removedGoogleSearchConsoleIndexes[3],
		removedGoogleSearchConsoleIndexes[4],
	).Scan(&unsafeIndexes); err != nil {
		t.Fatalf("count GSC indexes: %v", err)
	}
	if unsafeIndexes != 0 {
		t.Fatalf("expected unsafe GSC indexes to be absent, got %d", unsafeIndexes)
	}

	userID, err := store.CreateUser(ctx, "gsc-cleanup@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "gsc-cleanup.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	teamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("resolve site team: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleProperty(ctx, GoogleSearchConsolePropertyInput{
		TeamID:          teamID,
		URI:             "sc-domain:gsc-cleanup.test",
		PermissionLevel: "siteOwner",
		SeenAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert GSC property: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSiteMapping(ctx, GoogleSearchConsoleSiteMappingInput{
		SiteID:      site.ID,
		TeamID:      teamID,
		PropertyURI: "sc-domain:gsc-cleanup.test",
		MappedBy:    userID,
		MappedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert GSC mapping: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSyncState(ctx, GoogleSearchConsoleSyncStateInput{
		SiteID: site.ID,
		TeamID: teamID,
		State:  "idle",
	}); err != nil {
		t.Fatalf("upsert GSC sync state: %v", err)
	}

	if err := store.DeleteSite(ctx, site.ID); err != nil {
		t.Fatalf("delete site: %v", err)
	}

	var mappings, syncStates, properties int
	if err := store.DB().QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM google_search_console_site_mappings WHERE site_id = ?),
			(SELECT count(*) FROM google_search_console_sync_state WHERE site_id = ?),
			(SELECT count(*) FROM google_search_console_properties WHERE team_id = ?)`,
		site.ID, site.ID, teamID,
	).Scan(&mappings, &syncStates, &properties); err != nil {
		t.Fatalf("inspect GSC cleanup: %v", err)
	}
	if mappings != 0 || syncStates != 0 {
		t.Fatalf("expected site-scoped GSC rows to be deleted, got mappings=%d sync_states=%d", mappings, syncStates)
	}
	if properties != 1 {
		t.Fatalf("expected team-scoped GSC property to remain after site deletion, got %d", properties)
	}
}

func TestDropMutableControlIndexesMigrationPreservesActivityAndCleanup(t *testing.T) {
	ctx := context.Background()
	store := NewStore(filepath.Join(t.TempDir(), "activity-upgrade.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.DB().ExecContext(ctx,
		"CREATE TABLE IF NOT EXISTS migrations (migration VARCHAR PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL)"); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO migrations (migration, applied_at) VALUES (?, ?)", dropMutableControlIndexesMigrationFile, time.Now().UTC()); err != nil {
		t.Fatalf("hold back activity index migration: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate to pre-upgrade state: %v", err)
	}

	userID, err := store.CreateUser(ctx, "activity-upgrade@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "activity-upgrade.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	firstHitAt := time.Now().UTC().Add(-time.Minute)
	if err := store.RecordHitActivity(ctx, []*api.Hit{{
		ID: uuid.New(), SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(),
		Timestamp: firstHitAt, Path: "/before-upgrade",
	}}); err != nil {
		t.Fatalf("record pre-upgrade activity: %v", err)
	}

	var beforeIndexes int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM duckdb_indexes() WHERE table_name IN ('site_activity_summary', 'site_activity_hourly_counts')").Scan(&beforeIndexes); err != nil {
		t.Fatalf("count pre-upgrade activity indexes: %v", err)
	}
	if beforeIndexes != 4 {
		t.Fatalf("expected four pre-upgrade activity indexes, got %d", beforeIndexes)
	}

	if _, err := store.DB().ExecContext(ctx,
		"DELETE FROM migrations WHERE migration = ?", dropMutableControlIndexesMigrationFile); err != nil {
		t.Fatalf("release activity index migration: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("apply activity index migration: %v", err)
	}

	var afterIndexes int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM duckdb_indexes() WHERE table_name IN ('site_activity_summary', 'site_activity_hourly_counts')").Scan(&afterIndexes); err != nil {
		t.Fatalf("count post-upgrade activity indexes: %v", err)
	}
	if afterIndexes != 0 {
		t.Fatalf("expected activity secondary indexes to be removed, got %d", afterIndexes)
	}

	lastHitAt := time.Now().UTC()
	if err := store.RecordHitActivity(ctx, []*api.Hit{{
		ID: uuid.New(), SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(),
		Timestamp: lastHitAt, Path: "/after-upgrade",
	}}); err != nil {
		t.Fatalf("record post-upgrade activity: %v", err)
	}
	status, err := store.GetSiteTrackingStatus(ctx, site.ID, lastHitAt)
	if err != nil {
		t.Fatalf("read post-upgrade activity: %v", err)
	}
	if status == nil || status.LastHitAt == nil || !status.LastHitAt.Equal(lastHitAt) {
		t.Fatalf("expected post-upgrade activity update at %v, got %+v", lastHitAt, status)
	}

	expiredBucket := time.Now().UTC().Add(-activityCountWindow - time.Hour).Truncate(time.Hour)
	tenantID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("resolve site tenant for expired activity count: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO site_activity_hourly_counts (site_id, tenant_id, bucket, hits, events, updated_at)
		VALUES (?, ?, ?, 1, 0, ?)`, site.ID, tenantID, expiredBucket, time.Now().UTC()); err != nil {
		t.Fatalf("seed expired activity count: %v", err)
	}
	store.activityPruneMu.Lock()
	store.lastActivityPrune = time.Time{}
	store.activityPruneMu.Unlock()
	if err := store.maybePruneSiteActivityCounts(ctx); err != nil {
		t.Fatalf("prune activity counts without indexes: %v", err)
	}
	var expiredRows, retainedRows int
	if err := store.DB().QueryRowContext(ctx, `
		SELECT
			count(*) FILTER (WHERE bucket = ?),
			count(*) FILTER (WHERE bucket > ?)
		FROM site_activity_hourly_counts
		WHERE site_id = ?`, expiredBucket, expiredBucket, site.ID).Scan(&expiredRows, &retainedRows); err != nil {
		t.Fatalf("inspect activity retention: %v", err)
	}
	if expiredRows != 0 || retainedRows == 0 {
		t.Fatalf("expected retention to prune only the expired bucket, got expired=%d retained=%d", expiredRows, retainedRows)
	}

	if err := store.DeleteSite(ctx, site.ID); err != nil {
		t.Fatalf("delete site without activity indexes: %v", err)
	}
	var summaryRows, countRows int
	if err := store.DB().QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM site_activity_summary WHERE site_id = ?),
			(SELECT count(*) FROM site_activity_hourly_counts WHERE site_id = ?)`,
		site.ID, site.ID).Scan(&summaryRows, &countRows); err != nil {
		t.Fatalf("inspect activity cleanup: %v", err)
	}
	if summaryRows != 0 || countRows != 0 {
		t.Fatalf("expected activity cleanup without indexes, got summary=%d counts=%d", summaryRows, countRows)
	}
}

// TestCopySiteAnalyticsIsIdempotentWithoutUniqueIndexes guards the site
// transfer path: with the ART indexes gone there is no conflict-based upsert,
// so repeating a copy must replace rather than duplicate the site's rows.
// The destination carries the tenant-local data-plane schema, matching the
// only pairings TransferSite produces.
func TestCopySiteAnalyticsIsIdempotentWithoutUniqueIndexes(t *testing.T) {
	ctx := context.Background()
	source := newSharedTestStore(t)
	destination := NewStore(":memory:")
	if err := destination.Connect(); err != nil {
		t.Fatalf("connect destination: %v", err)
	}
	t.Cleanup(func() { _ = destination.Close() })
	if err := destination.MigrateTenant(ctx); err != nil {
		t.Fatalf("migrate destination tenant schema: %v", err)
	}

	userID, err := source.CreateUser(ctx, "copy-idempotent@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := source.CreateSite(ctx, userID, "copy-idempotent.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := source.CreateHitsBulk(ctx, []*api.Hit{
		{ID: uuid.New(), SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: time.Now().UTC(), Path: "/one"},
		{ID: uuid.New(), SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: time.Now().UTC(), Path: "/two"},
	}); err != nil {
		t.Fatalf("insert hits: %v", err)
	}

	for range 2 {
		if _, err := copySiteAnalyticsBetweenStores(ctx, source, destination, site.ID); err != nil {
			t.Fatalf("copy site analytics: %v", err)
		}
	}

	var count int
	if err := destination.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM hits WHERE site_id = ?", site.ID).Scan(&count); err != nil {
		t.Fatalf("count destination hits: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected repeated copy to leave exactly 2 hits, got %d", count)
	}
}

// TestFreshSchemasCarryNoAnalyticsArtIndexes covers both fresh installs: the
// shared store after Migrate and a tenant store after MigrateTenant.
func TestFreshSchemasCarryNoAnalyticsArtIndexes(t *testing.T) {
	ctx := context.Background()

	shared := newSharedTestStore(t)
	for _, table := range rebuiltAnalyticsTables {
		indexes, keys := countArtIndexState(t, shared, table)
		if indexes != 0 || keys != 0 {
			t.Fatalf("expected fresh shared %s without ART indexes, got %d indexes and %d key constraints", table, indexes, keys)
		}
	}

	mgr := NewTenantStoreManager(shared, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })
	tenantStore, err := mgr.ForTenant(ctx, insertTestTenant(t, shared))
	if err != nil {
		t.Fatalf("ForTenant: %v", err)
	}
	for _, table := range rebuiltAnalyticsTables {
		indexes, keys := countArtIndexState(t, tenantStore, table)
		if indexes != 0 || keys != 0 {
			t.Fatalf("expected fresh tenant %s without ART indexes, got %d indexes and %d key constraints", table, indexes, keys)
		}
	}
}
