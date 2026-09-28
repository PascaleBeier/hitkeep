package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDeleteSiteRemovesAllSiteData(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestFixtureStore(t)

	siteID, goalID, funnelID := seedSiteData(t, ctx, store)

	if err := store.DeleteSite(ctx, siteID); err != nil {
		t.Fatalf("delete site: %v", err)
	}

	assertTableCount(t, ctx, store, "sites", "id", siteID, 0)

	tables, err := listSiteIDTables(ctx, store.DB())
	if err != nil {
		t.Fatalf("list site tables: %v", err)
	}
	for _, table := range tables {
		if table == "sites" {
			continue
		}
		assertTableCount(t, ctx, store, table, "site_id", siteID, 0)
	}

	_ = goalID
	_ = funnelID
}

func TestResetSiteStatsClearsMeasuredDataOnly(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestFixtureStore(t)

	siteID, goalID, funnelID := seedSiteData(t, ctx, store)
	seedSiteStatsResetData(t, ctx, store, siteID)

	result, err := store.ResetSiteStats(ctx, siteID)
	if err != nil {
		t.Fatalf("reset site stats: %v", err)
	}
	if result.Status != "reset" {
		t.Fatalf("expected reset status, got %q", result.Status)
	}
	if result.RowsCleared == 0 {
		t.Fatalf("expected measured rows to be cleared")
	}
	if result.ImportsMarkedDeleted != 1 {
		t.Fatalf("expected 1 completed import to be marked deleted, got %d", result.ImportsMarkedDeleted)
	}
	assertResetFamilies(t, result.FamiliesCleared, "native", "rollups", "web_vitals", "imports", "search_console", "activity", "ai", "qr")

	for _, table := range []string{
		"hits",
		"events",
		"web_vitals",
		"hit_rollups_hourly",
		"hit_rollups_daily",
		"hit_rollups_monthly",
		"session_rollups_hourly",
		"session_rollups_daily",
		"session_rollups_monthly",
		"goal_rollups_hourly",
		"goal_rollups_daily",
		"goal_rollups_monthly",
		"funnel_rollups_hourly",
		"funnel_rollups_daily",
		"funnel_rollups_monthly",
		"rollup_dirty_buckets",
		"imported_traffic_daily",
		"imported_dimension_daily",
		"imported_event_daily",
		"imported_event_dimensions_daily",
		"imported_event_properties_daily",
		"search_console_facts",
		"site_activity_summary",
		"site_activity_hourly_counts",
		"ai_fetches",
		"ai_runs",
		"opportunities",
		"qr_code_opens",
	} {
		assertTableCount(t, ctx, store, table, "site_id", siteID, 0)
	}

	for _, table := range []string{
		"site_members",
		"site_tenants",
		"traffic_exclusions",
		"share_links",
		"goals",
		"funnels",
		"api_client_site_roles",
		"qr_codes",
		"qr_code_assets",
		"qr_code_share_links",
		"google_search_console_site_mappings",
	} {
		assertTableCount(t, ctx, store, table, "site_id", siteID, 1)
	}
	assertTableCount(t, ctx, store, "sites", "id", siteID, 1)
	assertTableCount(t, ctx, store, "goals", "id", goalID, 1)
	assertTableCount(t, ctx, store, "funnels", "id", funnelID, 1)
	assertSiteRetentionDays(t, ctx, store, siteID, 90)
	assertSiteImportStatuses(t, ctx, store, siteID, map[string]int{
		ImportStatusDeleted: 1,
		ImportStatusQueued:  1,
	})
}

func seedSiteData(t *testing.T, ctx context.Context, store *Store) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()

	userID := uuid.New()
	siteID := uuid.New()
	goalID := uuid.New()
	funnelID := uuid.New()
	sessionID := uuid.New()
	pageID := uuid.New()
	now := time.Now().UTC()
	date := now.Format("2006-01-02")

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB().ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}

	exec("INSERT INTO users (id, email, password, created_at) VALUES (?, ?, ?, ?)", userID, "test@example.com", "hash", now)
	exec("INSERT INTO sites (id, user_id, domain, created_at) VALUES (?, ?, ?, ?)", siteID, userID, "example.com", now)
	exec("INSERT INTO site_members (id, site_id, user_id, role, added_at, added_by) VALUES (?, ?, ?, ?, ?, ?)", uuid.New(), siteID, userID, "owner", now, userID)
	exec("INSERT INTO share_links (id, site_id, token_hash, created_by, created_at) VALUES (?, ?, ?, ?, ?)", uuid.New(), siteID, "token", userID, now)

	exec("INSERT INTO goals (id, site_id, name, type, value, created_at) VALUES (?, ?, ?, ?, ?, ?)", goalID, siteID, "Signup", "event", "signup", now)
	exec("INSERT INTO funnels (id, site_id, name, steps, created_at) VALUES (?, ?, ?, ?, ?)", funnelID, siteID, "Main", "[]", now)
	exec("INSERT INTO hits (id, site_id, session_id, page_id, timestamp, path, region, city, provider, asn, asn_org) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", uuid.New(), siteID, sessionID, pageID, now, "/", "California", "Mountain View", "Google LLC", 15169, "Google LLC")
	exec("INSERT INTO events (id, site_id, session_id, name, properties, timestamp) VALUES (?, ?, ?, ?, ?, ?)", uuid.New(), siteID, sessionID, "signup", "{}", now)
	exec("INSERT INTO web_vitals (id, site_id, session_id, page_id, metric, value, rating, path, timestamp) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", uuid.New(), siteID, sessionID, pageID, "LCP", 2600, "needs_improvement", "/pricing", now)

	exec("INSERT INTO hit_rollups_hourly (site_id, bucket, pageviews, visitors) VALUES (?, ?, ?, ?)", siteID, now, 1, 1)
	exec("INSERT INTO hit_rollups_daily (site_id, bucket, pageviews, visitors) VALUES (?, ?, ?, ?)", siteID, date, 1, 1)
	exec("INSERT INTO hit_rollups_monthly (site_id, bucket, pageviews, visitors) VALUES (?, ?, ?, ?)", siteID, date, 1, 1)

	exec("INSERT INTO session_rollups_hourly (site_id, bucket, sessions, bounced_sessions, duration_sum_seconds, pageviews) VALUES (?, ?, ?, ?, ?, ?)", siteID, now, 1, 0, 10.0, 1)
	exec("INSERT INTO session_rollups_daily (site_id, bucket, sessions, bounced_sessions, duration_sum_seconds, pageviews) VALUES (?, ?, ?, ?, ?, ?)", siteID, date, 1, 0, 10.0, 1)
	exec("INSERT INTO session_rollups_monthly (site_id, bucket, sessions, bounced_sessions, duration_sum_seconds, pageviews) VALUES (?, ?, ?, ?, ?, ?)", siteID, date, 1, 0, 10.0, 1)

	exec("INSERT INTO goal_rollups_hourly (site_id, goal_id, bucket, conversions) VALUES (?, ?, ?, ?)", siteID, goalID, now, 1)
	exec("INSERT INTO goal_rollups_daily (site_id, goal_id, bucket, conversions) VALUES (?, ?, ?, ?)", siteID, goalID, date, 1)
	exec("INSERT INTO goal_rollups_monthly (site_id, goal_id, bucket, conversions) VALUES (?, ?, ?, ?)", siteID, goalID, date, 1)

	exec("INSERT INTO funnel_rollups_hourly (site_id, funnel_id, bucket, entries, completions) VALUES (?, ?, ?, ?, ?)", siteID, funnelID, now, 1, 1)
	exec("INSERT INTO funnel_rollups_daily (site_id, funnel_id, bucket, entries, completions) VALUES (?, ?, ?, ?, ?)", siteID, funnelID, date, 1, 1)
	exec("INSERT INTO funnel_rollups_monthly (site_id, funnel_id, bucket, entries, completions) VALUES (?, ?, ?, ?, ?)", siteID, funnelID, date, 1, 1)

	return siteID, goalID, funnelID
}

func seedSiteStatsResetData(t *testing.T, ctx context.Context, store *Store, siteID uuid.UUID) {
	t.Helper()

	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	userID := siteUserID(t, ctx, store, siteID)
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("default tenant: %v", err)
	}
	completedImportID := uuid.New()
	queuedImportID := uuid.New()
	apiClientID := uuid.New()
	qrCodeID := uuid.New()
	aiRunID := uuid.New()

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB().ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}

	exec("INSERT INTO site_tenants (site_id, tenant_id, created_at) VALUES (?, ?, ?)", siteID, tenantID, now)
	exec("INSERT INTO tenant_members (tenant_id, user_id, role, added_at, added_by) VALUES (?, ?, ?, ?, ?)", tenantID, userID, "owner", now, userID)
	exec("UPDATE sites SET data_retention_days = ? WHERE id = ?", 90, siteID)
	exec("INSERT INTO traffic_exclusions (id, scope, site_id, rule_type, cidr, description, created_at, created_by) VALUES (?, 'site', ?, 'cidr', ?, ?, ?, ?)", uuid.New(), siteID, "203.0.113.0/24", "office", now, userID)

	exec("INSERT INTO api_clients (id, user_id, name, secret_hash, instance_role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)", apiClientID, userID, "Site API", "secret-hash", "user", now, now)
	exec("INSERT INTO api_client_site_roles (id, api_client_id, site_id, role, created_at) VALUES (?, ?, ?, ?, ?)", uuid.New(), apiClientID, siteID, "viewer", now)

	exec("INSERT INTO site_imports (id, site_id, provider, status, source_hash, bytes_total, bytes_received, rows_scanned, rows_imported, created_by, created_at, updated_at, finished_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", completedImportID, siteID, "plausible", ImportStatusCompleted, "completed-hash", 10, 10, 10, 9, userID, now, now, now)
	exec("INSERT INTO site_imports (id, site_id, provider, status, source_hash, bytes_total, bytes_received, rows_scanned, rows_imported, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", queuedImportID, siteID, "plausible", ImportStatusQueued, "queued-hash", 10, 0, 0, 0, userID, now, now)
	exec("INSERT INTO site_import_files (import_id, file_id, filename, relative_path, size_bytes, bytes_received, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", completedImportID, uuid.New(), "visits.csv", "visits.csv", 10, 10, "completed", now, now)
	exec("INSERT INTO site_import_files (import_id, file_id, filename, relative_path, size_bytes, bytes_received, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", queuedImportID, uuid.New(), "visits.csv", "visits.csv", 10, 0, "queued", now, now)
	exec("INSERT INTO imported_traffic_daily (site_id, import_id, date, visitors, visits, pageviews, bounces, visit_duration, source_file) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", siteID, completedImportID, date, 1, 1, 1, 0, 10, "visits.csv")
	exec("INSERT INTO imported_dimension_daily (site_id, import_id, date, dimension, name, visitors, visits, pageviews, source_file) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", siteID, completedImportID, date, "country", "DE", 1, 1, 1, "visits.csv")
	exec("INSERT INTO imported_event_daily (site_id, import_id, date, event_name, visitors, events, source_file) VALUES (?, ?, ?, ?, ?, ?, ?)", siteID, completedImportID, date, "signup", 1, 1, "events.csv")
	exec("INSERT INTO imported_event_dimensions_daily (site_id, import_id, date, event_name, dimension, name, visitors, events, source_file) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", siteID, completedImportID, date, "signup", "country", "DE", 1, 1, "events.csv")
	exec("INSERT INTO imported_event_properties_daily (site_id, import_id, date, event_name, property_key, property_value, visitors, events, source_file) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", siteID, completedImportID, date, "signup", "plan", "pro", 1, 1, "events.csv")

	exec("INSERT INTO google_search_console_properties (team_id, property_uri, permission_level, last_seen_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)", tenantID, "sc-domain:example.com", "siteFullUser", now, now, now)
	exec("INSERT INTO google_search_console_site_mappings (site_id, team_id, property_uri, mapped_by_user_id, mapped_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)", siteID, tenantID, "sc-domain:example.com", userID, now, now, now)
	exec("INSERT INTO search_console_facts (site_id, property_uri, date, query, page, country, device, clicks, impressions, ctr, position, aggregation_type, data_state, imported_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", siteID, "sc-domain:example.com", date, "hitkeep", "https://example.com/", "deu", "DESKTOP", 1, 10, 0.1, 2.5, "byPage", "final", now)

	exec("INSERT INTO site_activity_summary (site_id, tenant_id, first_hit_at, last_hit_at, last_event_at, last_hostname, last_event_name, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", siteID, tenantID, now, now, now, "example.com", "signup", now)
	exec("INSERT INTO site_activity_hourly_counts (site_id, tenant_id, bucket, hits, events, updated_at) VALUES (?, ?, ?, ?, ?, ?)", siteID, tenantID, now.Truncate(time.Hour), 1, 1, now)
	exec("INSERT INTO rollup_dirty_buckets (site_id, rollup_type, bucket_unit, bucket, updated_at) VALUES (?, ?, ?, ?, ?)", siteID, "hit", "hour", now.Truncate(time.Hour), now)

	exec("INSERT INTO ai_runs (id, team_id, site_id, actor_id, actor_type, feature, provider, model, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", aiRunID, tenantID, siteID, userID, "user", "opportunities", "test", "test-model", "completed", now)
	exec("INSERT INTO opportunities (id, team_id, site_id, kind, type_key, title_key, summary_key, action_key, impact_value, impact_label_key, confidence, status, ai_run_id, generated_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", uuid.New(), tenantID, siteID, "analytics", "opportunity.low_traffic", "title", "summary", "action", "10", "impact", "high", "open", aiRunID, now, now, now)
	exec("INSERT INTO ai_fetches (id, site_id, timestamp, assistant_name, assistant_family, path, hostname, status_code, content_type, resource_type, response_ms, bytes_served, user_agent) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", uuid.New(), siteID, now, "GPTBot", "openai", "/", "example.com", 200, "text/html", "page", 20, 1024, "GPTBot")

	exec("INSERT INTO qr_codes (id, site_id, created_by, name, destination_url, token, token_hash, token_hint, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", qrCodeID, siteID, userID, "Launch", "https://example.com/launch", "qr-token", "qr-token-hash", "qr-t...", now, now)
	exec("INSERT INTO qr_code_assets (qr_code_id, site_id, filename, content_type, byte_size, width, height, checksum, storage_key, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", qrCodeID, siteID, "launch.png", "image/png", 42, 128, 128, "checksum", "qr/launch.png", now, now)
	exec("INSERT INTO qr_code_share_links (id, site_id, qr_code_id, token_hash, token_hint, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)", uuid.New(), siteID, qrCodeID, "qr-share-hash", "qr-s...", userID, now)
	exec("INSERT INTO qr_code_opens (id, site_id, qr_code_id, timestamp, referrer, user_agent, country_code) VALUES (?, ?, ?, ?, ?, ?, ?)", uuid.New(), siteID, qrCodeID, now, "https://ref.example", "Mozilla/5.0", "DE")
}

func assertTableCount(t *testing.T, ctx context.Context, store *Store, table, column string, id uuid.UUID, expected int) {
	t.Helper()
	query := "SELECT COUNT(*) FROM " + table + " WHERE " + column + " = ?"
	var count int
	if err := store.DB().QueryRowContext(ctx, query, id).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if count != expected {
		t.Fatalf("expected %s.%s count %d, got %d", table, column, expected, count)
	}
}

func siteUserID(t *testing.T, ctx context.Context, store *Store, siteID uuid.UUID) uuid.UUID {
	t.Helper()
	var raw string
	if err := store.DB().QueryRowContext(ctx, "SELECT CAST(user_id AS VARCHAR) FROM sites WHERE id = ?", siteID).Scan(&raw); err != nil {
		t.Fatalf("site user id: %v", err)
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		t.Fatalf("parse site user id %q: %v", raw, err)
	}
	return id
}

func assertResetFamilies(t *testing.T, actual []string, expected ...string) {
	t.Helper()
	seen := make(map[string]struct{}, len(actual))
	for _, family := range actual {
		seen[family] = struct{}{}
	}
	for _, family := range expected {
		if _, ok := seen[family]; !ok {
			t.Fatalf("expected reset family %q in %v", family, actual)
		}
	}
}

func assertSiteRetentionDays(t *testing.T, ctx context.Context, store *Store, siteID uuid.UUID, expected int) {
	t.Helper()
	var actual int
	if err := store.DB().QueryRowContext(ctx, "SELECT data_retention_days FROM sites WHERE id = ?", siteID).Scan(&actual); err != nil {
		t.Fatalf("site retention days: %v", err)
	}
	if actual != expected {
		t.Fatalf("expected site retention days %d, got %d", expected, actual)
	}
}

func assertSiteImportStatuses(t *testing.T, ctx context.Context, store *Store, siteID uuid.UUID, expected map[string]int) {
	t.Helper()
	rows, err := store.DB().QueryContext(ctx, `
		SELECT status, COUNT(*)
		FROM site_imports
		WHERE site_id = ?
		GROUP BY status
	`, siteID)
	if err != nil {
		t.Fatalf("query site import statuses: %v", err)
	}
	defer func() { _ = rows.Close() }()

	actual := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			t.Fatalf("scan site import status: %v", err)
		}
		actual[status] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate site import statuses: %v", err)
	}

	for status, expectedCount := range expected {
		if actual[status] != expectedCount {
			t.Fatalf("expected %d imports with status %q, got %d in %v", expectedCount, status, actual[status], actual)
		}
	}
	if len(actual) != len(expected) {
		t.Fatalf("expected import statuses %v, got %v", expected, actual)
	}
}
