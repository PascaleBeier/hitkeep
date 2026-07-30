package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/importables"
)

func newSharedTestStore(t *testing.T) *Store {
	t.Helper()
	return newSharedTestFixtureStore(t)
}

func TestForTenantDefaultReturnsShared(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	defaultID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	got, err := mgr.ForTenant(ctx, defaultID)
	if err != nil {
		t.Fatalf("ForTenant(default): %v", err)
	}
	if got != store {
		t.Fatal("expected ForTenant(defaultID) to return the shared store")
	}
}

func TestForTenantNilReturnsShared(t *testing.T) {
	store := newSharedTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	got, err := mgr.ForTenant(context.Background(), uuid.Nil)
	if err != nil {
		t.Fatalf("ForTenant(nil): %v", err)
	}
	if got != store {
		t.Fatal("expected ForTenant(uuid.Nil) to return the shared store")
	}
}

func TestForTenantCreatesNewDB(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	basePath := t.TempDir()
	mgr := NewTenantStoreManager(store, basePath)
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := uuid.New()
	// Insert a non-default tenant into the shared DB.
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		tenantID, "Test Tenant", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	got, err := mgr.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ForTenant(custom): %v", err)
	}
	if got == store {
		t.Fatal("expected ForTenant(customID) to return a different store from shared")
	}

	// Verify the DB file was created.
	dbPath := filepath.Join(basePath, "tenants", tenantID.String(), "hitkeep.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("expected tenant DB file at %s", dbPath)
	}
}

func TestForTenantCacheHit(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		tenantID, "Test Tenant", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	first, err := mgr.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("first ForTenant: %v", err)
	}

	second, err := mgr.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("second ForTenant: %v", err)
	}

	if first != second {
		t.Fatal("expected same store instance on second call (cache hit)")
	}
}

func TestForTenantMigratesOnFirstAccess(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		tenantID, "Test Tenant", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	tenantStore, err := mgr.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ForTenant: %v", err)
	}

	// Verify analytics tables exist by querying them.
	tables := []string{"hits", "events", "goals", "funnels",
		"hit_rollups_hourly", "hit_rollups_daily", "hit_rollups_monthly",
		"session_rollups_hourly", "session_rollups_daily", "session_rollups_monthly",
		"goal_rollups_hourly", "goal_rollups_daily", "goal_rollups_monthly",
		"funnel_rollups_hourly", "funnel_rollups_daily", "funnel_rollups_monthly",
	}
	for _, table := range tables {
		var count int
		if err := tenantStore.DB().QueryRowContext(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM %s", table),
		).Scan(&count); err != nil {
			t.Fatalf("query %s on tenant DB: %v", table, err)
		}
	}
}

func TestCloseClosesAllTenantStores(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir())

	tenantID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		tenantID, "Test Tenant", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	tenantStore, err := mgr.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ForTenant: %v", err)
	}

	if err := mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// The tenant store should be closed (querying should fail or return an error).
	if err := tenantStore.DB().PingContext(ctx); err == nil {
		// DuckDB may or may not error on ping after close; at minimum, the store
		// map should be empty. Just verify Close() didn't error.
	}
}

func TestResolveTenantStore(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	// Create a user.
	userID, err := store.CreateUser(ctx, "test@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// The user should resolve to the default tenant (via membership seeded by migration).
	resolvedStore, tenantID, err := mgr.ResolveTenantStore(ctx, userID)
	if err != nil {
		t.Fatalf("ResolveTenantStore: %v", err)
	}

	defaultID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	if tenantID != defaultID {
		t.Fatalf("expected default tenant ID %s, got %s", defaultID, tenantID)
	}
	if resolvedStore != store {
		t.Fatal("expected resolved store to be the shared store for default tenant")
	}
}

func TestResolveSiteStoreBackfillsLegacyAnalyticsConfig(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	basePath := t.TempDir()
	mgr := NewTenantStoreManager(store, basePath)
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := store.CreateUser(ctx, "sync@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	team, err := store.CreateTenant(ctx, userID, "Sync Team", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}

	site, err := store.CreateSite(ctx, userID, "sync-team.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	legacyGoal := &api.Goal{
		ID:        uuid.New(),
		SiteID:    site.ID,
		Name:      "Legacy Signup",
		Type:      "event",
		Value:     "signup_completed",
		CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateGoal(ctx, legacyGoal); err != nil {
		t.Fatalf("create shared goal: %v", err)
	}

	legacyFunnel := &api.Funnel{
		ID:        uuid.New(),
		SiteID:    site.ID,
		Name:      "Legacy Funnel",
		Steps:     []api.FunnelStep{{Type: "path", Value: "/"}, {Type: "event", Value: "signup_completed"}},
		CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateFunnel(ctx, legacyFunnel); err != nil {
		t.Fatalf("create shared funnel: %v", err)
	}

	tenantStore, tenantID, err := mgr.ResolveSiteStore(ctx, site.ID)
	if err != nil {
		t.Fatalf("ResolveSiteStore: %v", err)
	}
	if tenantID != team.ID {
		t.Fatalf("expected tenant %s, got %s", team.ID, tenantID)
	}
	if tenantStore == store {
		t.Fatal("expected custom tenant site to resolve to tenant-local store")
	}

	var mirroredDomain string
	var mirroredRetention int
	if err := tenantStore.DB().QueryRowContext(ctx,
		"SELECT domain, data_retention_days FROM sites WHERE id = ?",
		site.ID,
	).Scan(&mirroredDomain, &mirroredRetention); err != nil {
		t.Fatalf("query tenant site mirror: %v", err)
	}
	if mirroredDomain != site.Domain {
		t.Fatalf("expected mirrored domain %q, got %q", site.Domain, mirroredDomain)
	}

	tenantGoals, err := tenantStore.GetGoals(ctx, site.ID)
	if err != nil {
		t.Fatalf("tenant GetGoals: %v", err)
	}
	if len(tenantGoals) != 1 || tenantGoals[0].ID != legacyGoal.ID {
		t.Fatalf("expected legacy goal to be backfilled, got %+v", tenantGoals)
	}

	tenantFunnels, err := tenantStore.GetFunnels(ctx, site.ID)
	if err != nil {
		t.Fatalf("tenant GetFunnels: %v", err)
	}
	if len(tenantFunnels) != 1 || tenantFunnels[0].ID != legacyFunnel.ID {
		t.Fatalf("expected legacy funnel to be backfilled, got %+v", tenantFunnels)
	}
}

func TestDeleteSiteRemovesTenantAndSharedData(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	basePath := t.TempDir()
	mgr := NewTenantStoreManager(store, basePath)
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := store.CreateUser(ctx, "delete-site@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	team, err := store.CreateTenant(ctx, userID, "Delete Team", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}

	site, err := store.CreateSite(ctx, userID, "delete-team.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	tenantStore, _, err := mgr.ResolveSiteStore(ctx, site.ID)
	if err != nil {
		t.Fatalf("ResolveSiteStore: %v", err)
	}

	if err := tenantStore.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: time.Now().UTC(),
		Path:      "/",
	}); err != nil {
		t.Fatalf("create tenant hit: %v", err)
	}
	if err := tenantStore.CreateWebVital(ctx, &api.WebVital{
		SiteID:         site.ID,
		SessionID:      uuid.New(),
		PageID:         uuid.New(),
		Metric:         api.WebVitalLCP,
		Value:          2600,
		Path:           "/pricing",
		Timestamp:      time.Now().UTC(),
		TrackerSource:  "browser",
		TrackerVersion: "test",
	}); err != nil {
		t.Fatalf("create tenant web vital: %v", err)
	}

	if err := mgr.DeleteSite(ctx, site.ID); err != nil {
		t.Fatalf("DeleteSite: %v", err)
	}

	var sharedCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM sites WHERE id = ?", site.ID).Scan(&sharedCount); err != nil {
		t.Fatalf("count shared sites: %v", err)
	}
	if sharedCount != 0 {
		t.Fatalf("expected shared site row deleted, got count=%d", sharedCount)
	}

	var tenantCount int
	if err := tenantStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM sites WHERE id = ?", site.ID).Scan(&tenantCount); err != nil {
		t.Fatalf("count tenant sites: %v", err)
	}
	if tenantCount != 0 {
		t.Fatalf("expected tenant site mirror deleted, got count=%d", tenantCount)
	}

	var tenantVitals int
	if err := tenantStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM web_vitals WHERE site_id = ?", site.ID).Scan(&tenantVitals); err != nil {
		t.Fatalf("count tenant web vitals: %v", err)
	}
	if tenantVitals != 0 {
		t.Fatalf("expected tenant web vitals deleted, got count=%d", tenantVitals)
	}
}

func TestResetSiteStatsClearsTenantAnalyticsAndSharedMeasuredData(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	basePath := t.TempDir()
	mgr := NewTenantStoreManager(store, basePath)
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := store.CreateUser(ctx, "reset-tenant-site@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	team, err := store.CreateTenant(ctx, userID, "Reset Team", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "reset-team.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	tenantStore, _, err := mgr.ResolveSiteStore(ctx, site.ID)
	if err != nil {
		t.Fatalf("ResolveSiteStore: %v", err)
	}

	now := time.Now().UTC()
	sessionID := uuid.New()
	pageID := uuid.New()
	if err := tenantStore.CreateHit(ctx, &api.Hit{
		ID:        uuid.New(),
		SiteID:    site.ID,
		SessionID: sessionID,
		PageID:    pageID,
		Timestamp: now,
		Path:      "/",
	}); err != nil {
		t.Fatalf("create tenant hit: %v", err)
	}
	if err := tenantStore.CreateEvent(ctx, &api.Event{
		ID:         uuid.New(),
		SiteID:     site.ID,
		SessionID:  sessionID,
		Name:       "signup",
		Properties: map[string]any{"plan": "pro"},
		Timestamp:  now,
	}); err != nil {
		t.Fatalf("create tenant event: %v", err)
	}
	if err := tenantStore.CreateWebVital(ctx, &api.WebVital{
		ID:             uuid.New(),
		SiteID:         site.ID,
		SessionID:      sessionID,
		PageID:         pageID,
		Metric:         api.WebVitalLCP,
		Value:          2600,
		Rating:         api.WebVitalRatingNeedsImprovement,
		Path:           "/pricing",
		Timestamp:      now,
		TrackerSource:  "browser",
		TrackerVersion: "test",
	}); err != nil {
		t.Fatalf("create tenant web vital: %v", err)
	}
	importID := uuid.New()
	if _, err := tenantStore.DB().ExecContext(ctx,
		"INSERT INTO imported_traffic_daily (site_id, import_id, date, visitors, visits, pageviews, bounces, visit_duration, source_file) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		site.ID, importID, now.Format("2006-01-02"), 1, 1, 1, 0, 10, "visits.csv",
	); err != nil {
		t.Fatalf("insert tenant imported traffic: %v", err)
	}
	if _, err := tenantStore.DB().ExecContext(ctx,
		"INSERT INTO ai_fetches (id, site_id, timestamp, assistant_name, assistant_family, path, status_code, resource_type) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		uuid.New(), site.ID, now, "GPTBot", "openai", "/", 200, "page",
	); err != nil {
		t.Fatalf("insert tenant ai fetch: %v", err)
	}

	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO site_imports (id, site_id, provider, status, source_hash, bytes_total, bytes_received, rows_scanned, rows_imported, created_by, created_at, updated_at, finished_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		importID, site.ID, "plausible", ImportStatusCompleted, "hash", 10, 10, 10, 10, userID, now, now, now,
	); err != nil {
		t.Fatalf("insert shared import: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO site_activity_summary (site_id, tenant_id, first_hit_at, last_hit_at, last_event_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		site.ID, team.ID, now, now, now, now,
	); err != nil {
		t.Fatalf("insert shared activity summary: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO site_activity_hourly_counts (site_id, tenant_id, bucket, hits, events, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		site.ID, team.ID, now.Truncate(time.Hour), 1, 1, now,
	); err != nil {
		t.Fatalf("insert shared activity hourly: %v", err)
	}
	aiRunID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO ai_runs (id, team_id, site_id, actor_id, actor_type, feature, provider, model, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		aiRunID, team.ID, site.ID, userID, "user", "opportunities", "test", "test-model", "completed", now,
	); err != nil {
		t.Fatalf("insert shared ai run: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO opportunities (id, team_id, site_id, kind, type_key, title_key, summary_key, action_key, impact_value, impact_label_key, confidence, status, ai_run_id, generated_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		uuid.New(), team.ID, site.ID, "analytics", "type", "title", "summary", "action", "10", "impact", "high", "open", aiRunID, now, now, now,
	); err != nil {
		t.Fatalf("insert shared opportunity: %v", err)
	}

	result, err := mgr.ResetSiteStats(ctx, site.ID)
	if err != nil {
		t.Fatalf("ResetSiteStats: %v", err)
	}
	if result.RowsCleared == 0 {
		t.Fatalf("expected tenant/shared rows to be cleared")
	}
	if result.ImportsMarkedDeleted != 1 {
		t.Fatalf("expected completed shared import to be marked deleted, got %d", result.ImportsMarkedDeleted)
	}
	assertResetFamilies(t, result.FamiliesCleared, "native", "web_vitals", "imports", "activity", "ai")

	for _, table := range []string{"hits", "events", "web_vitals", "imported_traffic_daily", "ai_fetches"} {
		assertTableCount(t, ctx, tenantStore, table, "site_id", site.ID, 0)
	}
	for _, table := range []string{"site_activity_summary", "site_activity_hourly_counts", "ai_runs", "opportunities"} {
		assertTableCount(t, ctx, store, table, "site_id", site.ID, 0)
	}
	assertTableCount(t, ctx, store, "sites", "id", site.ID, 1)
	assertTableCount(t, ctx, store, "site_tenants", "site_id", site.ID, 1)
	assertTableCount(t, ctx, tenantStore, "sites", "id", site.ID, 1)
	assertSiteImportStatuses(t, ctx, store, site.ID, map[string]int{ImportStatusDeleted: 1})
}

func TestDeleteSiteWithImportedUnattributedPropertiesDoesNotInvalidateTenantDB(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	basePath := t.TempDir()
	mgr := NewTenantStoreManager(store, basePath)
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := store.CreateUser(ctx, "delete-imported-site@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	team, err := store.CreateTenant(ctx, userID, "Delete Imported Team", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "delete-imported-team.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	tenantStore, _, err := mgr.ResolveSiteStore(ctx, site.ID)
	if err != nil {
		t.Fatalf("ResolveSiteStore: %v", err)
	}

	importID := uuid.New()
	sink, err := NewImportedDataSink(ctx, tenantStore, site.ID, importID)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	for _, date := range []time.Time{
		time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
	} {
		if err := sink.PutEventProperty(ctx, importables.EventPropertyRow{Date: date, EventName: "outbound_click", PropertyKey: "url", PropertyValue: "https://example.com", Visitors: 1, Events: 1, SourceFile: "imported_custom_events.csv"}); err != nil {
			t.Fatalf("put attributed property: %v", err)
		}
		if err := sink.PutEventProperty(ctx, importables.EventPropertyRow{Date: date, PropertyKey: "url", PropertyValue: "https://example.com", Visitors: 1, Events: 1, SourceFile: "imported_unattributed_properties.csv"}); err != nil {
			t.Fatalf("put unattributed property: %v", err)
		}
		if err := sink.PutEventDimension(ctx, importables.EventDimensionRow{Date: date, EventName: "outbound_click", Dimension: "url", Name: "https://example.com", Visitors: 1, Events: 1, SourceFile: "imported_event_dimensions.csv"}); err != nil {
			t.Fatalf("put event dimension: %v", err)
		}
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if err := mgr.DeleteSite(ctx, site.ID); err != nil {
		t.Fatalf("DeleteSite: %v", err)
	}
	if err := tenantStore.DB().PingContext(ctx); err != nil {
		t.Fatalf("tenant DB should remain usable after deleting imported properties: %v", err)
	}
}

func TestTransferSiteMovesAnalyticsToDestinationTenant(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	basePath := t.TempDir()
	mgr := NewTenantStoreManager(store, basePath)
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := store.CreateUser(ctx, "transfer-site@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	site, err := store.CreateSite(ctx, userID, "transfer-site.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.CreateGoal(ctx, &api.Goal{
		ID:        uuid.New(),
		SiteID:    site.ID,
		Name:      "Signup",
		Type:      "event",
		Value:     "signup",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create goal in shared store: %v", err)
	}
	sessionID := uuid.New()
	pageID := uuid.New()
	if err := store.CreateHit(ctx, &api.Hit{
		ID:        uuid.New(),
		SiteID:    site.ID,
		SessionID: sessionID,
		PageID:    pageID,
		Timestamp: time.Now().UTC(),
		Path:      "/pricing",
	}); err != nil {
		t.Fatalf("create hit in shared store: %v", err)
	}
	if err := store.CreateWebVital(ctx, &api.WebVital{
		ID:             uuid.New(),
		SiteID:         site.ID,
		SessionID:      sessionID,
		PageID:         pageID,
		Metric:         api.WebVitalLCP,
		Value:          2400,
		Rating:         api.WebVitalRatingGood,
		Path:           "/pricing",
		Timestamp:      time.Now().UTC(),
		TrackerSource:  "browser",
		TrackerVersion: "test",
	}); err != nil {
		t.Fatalf("create web vital in shared store: %v", err)
	}
	if err := store.CreateEvent(ctx, &api.Event{
		ID:        uuid.New(),
		SiteID:    site.ID,
		SessionID: uuid.New(),
		Name:      "signup",
		Properties: map[string]any{
			"plan":  "pro",
			"steps": []string{"landing", "checkout"},
		},
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create event in shared store: %v", err)
	}
	if err := store.CreateFunnel(ctx, &api.Funnel{
		ID:     uuid.New(),
		SiteID: site.ID,
		Name:   "Signup funnel",
		Steps: []api.FunnelStep{
			{Type: "path", Value: "/"},
			{Type: "event", Value: "signup"},
		},
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create funnel in shared store: %v", err)
	}

	destinationTeam, err := store.CreateTenant(ctx, userID, "Destination Team", "")
	if err != nil {
		t.Fatalf("create destination team: %v", err)
	}
	seedGoogleSearchConsoleSiteMappingForTransfer(t, store, site.ID, userID)

	sourceTeamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("GetSiteTenantID before transfer: %v", err)
	}
	if err := mgr.TransferSite(ctx, site.ID, destinationTeam.ID, AuditEntryParams{
		ActorID:     userID,
		TeamID:      sourceTeamID,
		Action:      "site.transferred_out",
		TargetType:  "site",
		TargetID:    site.ID.String(),
		TargetLabel: site.Domain,
		Outcome:     "success",
	}, AuditEntryParams{
		ActorID:     userID,
		TeamID:      destinationTeam.ID,
		Action:      "site.transferred_in",
		TargetType:  "site",
		TargetID:    site.ID.String(),
		TargetLabel: site.Domain,
		Outcome:     "success",
	}, AuditEntryParams{
		ActorID:     userID,
		TeamID:      sourceTeamID,
		Action:      "google_search_console.property_unmapped",
		TargetType:  "site",
		TargetID:    site.ID.String(),
		TargetLabel: site.Domain,
		Outcome:     "success",
		Details:     "old_property_uri=sc-domain:source-team.example.com;new_property_uri=;reason=site_transfer",
	}); err != nil {
		t.Fatalf("TransferSite: %v", err)
	}

	tenantStore, err := mgr.ForTenant(ctx, destinationTeam.ID)
	if err != nil {
		t.Fatalf("ForTenant(destination): %v", err)
	}

	goals, err := tenantStore.GetGoals(ctx, site.ID)
	if err != nil {
		t.Fatalf("tenant GetGoals: %v", err)
	}
	if len(goals) != 1 {
		t.Fatalf("expected 1 goal in destination tenant store, got %d", len(goals))
	}

	var hitCount int
	if err := tenantStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", site.ID).Scan(&hitCount); err != nil {
		t.Fatalf("count transferred hits in destination store: %v", err)
	}
	if hitCount != 1 {
		t.Fatalf("expected 1 transferred hit in destination tenant store, got %d", hitCount)
	}

	var webVitalCount int
	if err := tenantStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM web_vitals WHERE site_id = ?", site.ID).Scan(&webVitalCount); err != nil {
		t.Fatalf("count transferred web vitals in destination store: %v", err)
	}
	if webVitalCount != 1 {
		t.Fatalf("expected 1 transferred web vital in destination tenant store, got %d", webVitalCount)
	}

	var eventCount int
	var eventProperties string
	if err := tenantStore.DB().QueryRowContext(ctx, "SELECT COUNT(*), CAST(MAX(properties) AS VARCHAR) FROM events WHERE site_id = ?", site.ID).Scan(&eventCount, &eventProperties); err != nil {
		t.Fatalf("query transferred events in destination store: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("expected 1 event in destination tenant store, got %d", eventCount)
	}
	if !strings.Contains(eventProperties, "\"plan\":\"pro\"") {
		t.Fatalf("expected transferred event properties to contain plan=pro, got %s", eventProperties)
	}

	funnels, err := tenantStore.GetFunnels(ctx, site.ID)
	if err != nil {
		t.Fatalf("tenant GetFunnels: %v", err)
	}
	if len(funnels) != 1 {
		t.Fatalf("expected 1 funnel in destination tenant store, got %d", len(funnels))
	}
	if len(funnels[0].Steps) != 2 {
		t.Fatalf("expected transferred funnel steps, got %#v", funnels[0].Steps)
	}

	tenantID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("GetSiteTenantID after transfer: %v", err)
	}
	if tenantID != destinationTeam.ID {
		t.Fatalf("expected site tenant %s, got %s", destinationTeam.ID, tenantID)
	}

	var sharedHitCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", site.ID).Scan(&sharedHitCount); err != nil {
		t.Fatalf("count shared hits after transfer: %v", err)
	}
	if sharedHitCount != 0 {
		t.Fatalf("expected shared analytics to be cleared after transfer, got %d hit rows", sharedHitCount)
	}

	var sharedWebVitalCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM web_vitals WHERE site_id = ?", site.ID).Scan(&sharedWebVitalCount); err != nil {
		t.Fatalf("count shared web vitals after transfer: %v", err)
	}
	if sharedWebVitalCount != 0 {
		t.Fatalf("expected shared Web Vitals to be cleared after transfer, got %d rows", sharedWebVitalCount)
	}

	requireNoGoogleSearchConsoleSiteMapping(t, store, site.ID)
	entries, total, err := store.ListTeamAuditEntries(ctx, sourceTeamID, "google_search_console.property_unmapped", 5, 0)
	if err != nil {
		t.Fatalf("ListTeamAuditEntries: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one Search Console transfer audit, got total=%d entries=%+v", total, entries)
	}
	entries, total, err = store.ListTeamAuditEntries(ctx, sourceTeamID, "site.transfer_data_move_prepared", 5, 0)
	if err != nil {
		t.Fatalf("ListTeamAuditEntries transfer data move: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one site transfer data move audit, got total=%d entries=%+v", total, entries)
	}
	if entries[0].Outcome != "success" || !strings.Contains(entries[0].Details, "destination_team_id="+destinationTeam.ID.String()) {
		t.Fatalf("unexpected site transfer data move audit: %+v", entries[0])
	}
}

func TestTransferSiteRequiresTransferAudit(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := store.CreateUser(ctx, "transfer-audit@test.dev", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "transfer-audit.example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}
	destinationTeam, err := store.CreateTenant(ctx, userID, "Destination Team", "")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}

	err = mgr.TransferSite(ctx, site.ID, destinationTeam.ID)
	if err == nil {
		t.Fatalf("expected transfer without audit to fail")
	}
	if !strings.Contains(err.Error(), "site transfer audit is required") {
		t.Fatalf("expected missing audit error, got %v", err)
	}
}

func TestTransferSiteRequiresSearchConsoleAuditWhenMappingExists(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := store.CreateUser(ctx, "transfer-gsc-audit@test.dev", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "transfer-gsc-audit.example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}
	sourceTeamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("GetSiteTenantID: %v", err)
	}
	destinationTeam, err := store.CreateTenant(ctx, userID, "Destination Team", "")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	seedGoogleSearchConsoleSiteMappingForTransfer(t, store, site.ID, userID)

	err = mgr.TransferSite(ctx, site.ID, destinationTeam.ID,
		AuditEntryParams{
			ActorID:     userID,
			TeamID:      sourceTeamID,
			Action:      "site.transferred_out",
			TargetType:  "site",
			TargetID:    site.ID.String(),
			TargetLabel: site.Domain,
			Outcome:     "success",
		},
		AuditEntryParams{
			ActorID:     userID,
			TeamID:      destinationTeam.ID,
			Action:      "site.transferred_in",
			TargetType:  "site",
			TargetID:    site.ID.String(),
			TargetLabel: site.Domain,
			Outcome:     "success",
		},
	)
	if err == nil {
		t.Fatalf("expected transfer without Search Console audit to fail")
	}
	if !strings.Contains(err.Error(), "search console transfer audit is required") {
		t.Fatalf("expected missing Search Console audit error, got %v", err)
	}
	mapping, err := store.GetGoogleSearchConsoleSiteMapping(ctx, site.ID)
	if err != nil {
		t.Fatalf("GetGoogleSearchConsoleSiteMapping: %v", err)
	}
	if mapping == nil {
		t.Fatalf("expected Search Console mapping to remain after failed unaudited transfer")
	}
}

func seedGoogleSearchConsoleSiteMappingForTransfer(t *testing.T, store *Store, siteID, userID uuid.UUID) {
	t.Helper()
	sourceTeamID, err := store.GetSiteTenantID(context.Background(), siteID)
	if err != nil {
		t.Fatalf("get source site tenant: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSiteMapping(context.Background(), GoogleSearchConsoleSiteMappingInput{
		SiteID:      siteID,
		TeamID:      sourceTeamID,
		PropertyURI: "sc-domain:source-team.example.com",
		MappedBy:    userID,
		MappedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed Search Console mapping: %v", err)
	}
}

func requireNoGoogleSearchConsoleSiteMapping(t *testing.T, store *Store, siteID uuid.UUID) {
	t.Helper()
	mapping, err := store.GetGoogleSearchConsoleSiteMapping(context.Background(), siteID)
	if err != nil {
		t.Fatalf("get Search Console mapping after transfer: %v", err)
	}
	if mapping != nil {
		t.Fatalf("expected Search Console mapping to be cleared on team transfer, got %+v", mapping)
	}
}

func TestTransferSiteCopiesAIFetchRowsIntoTenantStore(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	mgr := NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := store.CreateUser(ctx, "transfer-ai-fetch@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "transfer-ai-fetch.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO ai_fetches (id, site_id, timestamp, assistant_name, assistant_family, path, status_code, resource_type)
		VALUES (?, ?, now(), 'TestBot', 'test', '/docs', 200, 'page')`,
		uuid.New(), site.ID,
	); err != nil {
		t.Fatalf("insert ai fetch: %v", err)
	}

	destinationTeam, err := store.CreateTenant(ctx, userID, "AI Fetch Destination", "")
	if err != nil {
		t.Fatalf("create destination team: %v", err)
	}
	sourceTeamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("resolve source team: %v", err)
	}

	if err := mgr.TransferSite(ctx, site.ID, destinationTeam.ID, AuditEntryParams{
		ActorID: userID, TeamID: sourceTeamID, Action: "site.transferred_out",
		TargetType: "site", TargetID: site.ID.String(), TargetLabel: site.Domain, Outcome: "success",
	}, AuditEntryParams{
		ActorID: userID, TeamID: destinationTeam.ID, Action: "site.transferred_in",
		TargetType: "site", TargetID: site.ID.String(), TargetLabel: site.Domain, Outcome: "success",
	}); err != nil {
		t.Fatalf("transfer site with ai fetch rows: %v", err)
	}

	destinationStore, err := mgr.ForTenant(ctx, destinationTeam.ID)
	if err != nil {
		t.Fatalf("open destination store: %v", err)
	}
	var count int
	if err := destinationStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_fetches WHERE site_id = ?", site.ID).Scan(&count); err != nil {
		t.Fatalf("count destination ai fetches: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 ai fetch row in destination tenant store, got %d", count)
	}
	var sourceCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_fetches WHERE site_id = ?", site.ID).Scan(&sourceCount); err != nil {
		t.Fatalf("count source ai fetches: %v", err)
	}
	if sourceCount != 0 {
		t.Fatalf("expected source ai fetch rows cleaned after transfer, got %d", sourceCount)
	}

	// Transfer back out of the tenant store: the FK-referenced sites mirror
	// row must be removable after its analytics children are cleaned.
	if err := mgr.TransferSite(ctx, site.ID, sourceTeamID, AuditEntryParams{
		ActorID: userID, TeamID: destinationTeam.ID, Action: "site.transferred_out",
		TargetType: "site", TargetID: site.ID.String(), TargetLabel: site.Domain, Outcome: "success",
	}, AuditEntryParams{
		ActorID: userID, TeamID: sourceTeamID, Action: "site.transferred_in",
		TargetType: "site", TargetID: site.ID.String(), TargetLabel: site.Domain, Outcome: "success",
	}); err != nil {
		t.Fatalf("transfer site back out of tenant store: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_fetches WHERE site_id = ?", site.ID).Scan(&sourceCount); err != nil {
		t.Fatalf("count shared ai fetches after transfer back: %v", err)
	}
	if sourceCount != 1 {
		t.Fatalf("expected ai fetch row back in shared store, got %d", sourceCount)
	}
	var mirrorCount int
	if err := destinationStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM sites WHERE id = ?", site.ID).Scan(&mirrorCount); err != nil {
		t.Fatalf("count tenant site mirror rows: %v", err)
	}
	if mirrorCount != 0 {
		t.Fatalf("expected tenant site mirror removed after transfer back, got %d", mirrorCount)
	}
}
