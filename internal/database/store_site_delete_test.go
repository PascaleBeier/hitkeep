package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDeleteSiteRemovesAllSiteData(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "delete-site.db")

	store := NewStore(dbPath)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

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
