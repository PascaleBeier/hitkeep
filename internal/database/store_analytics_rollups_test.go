package database

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestCreateHitsBulkMarksDirtyRollupBuckets(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "dirty-buckets.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	ts := time.Date(2026, time.March, 31, 8, 17, 0, 0, time.UTC)
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: ts,
		Path:      "/pricing",
	}); err != nil {
		t.Fatalf("create hit: %v", err)
	}

	expected := map[string]struct{}{
		fmt.Sprintf("hit|hour|%s", time.Date(2026, time.March, 31, 8, 0, 0, 0, time.UTC).Format(time.RFC3339)):     {},
		fmt.Sprintf("hit|day|%s", time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)):      {},
		fmt.Sprintf("hit|month|%s", time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)):     {},
		fmt.Sprintf("session|hour|%s", time.Date(2026, time.March, 31, 8, 0, 0, 0, time.UTC).Format(time.RFC3339)): {},
		fmt.Sprintf("session|day|%s", time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)):  {},
		fmt.Sprintf("session|month|%s", time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)): {},
	}

	rows, err := store.DB().QueryContext(ctx, `
		SELECT rollup_type, bucket_unit, bucket
		FROM rollup_dirty_buckets
		WHERE site_id = ?
	`, site.ID)
	if err != nil {
		t.Fatalf("query dirty buckets: %v", err)
	}
	defer rows.Close()

	actual := map[string]struct{}{}
	for rows.Next() {
		var rollupType string
		var bucketUnit string
		var bucket time.Time
		if err := rows.Scan(&rollupType, &bucketUnit, &bucket); err != nil {
			t.Fatalf("scan dirty bucket: %v", err)
		}
		actual[fmt.Sprintf("%s|%s|%s", rollupType, bucketUnit, bucket.UTC().Format(time.RFC3339))] = struct{}{}
	}

	if len(actual) != len(expected) {
		t.Fatalf("expected %d dirty buckets, got %d", len(expected), len(actual))
	}
	for key := range expected {
		if _, ok := actual[key]; !ok {
			t.Fatalf("missing dirty bucket %s", key)
		}
	}
}

func TestGetSiteStatsRefreshesStaleDailyRollups(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "daily-rollups.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	dayBucket := time.Date(2026, time.March, 30, 0, 0, 0, 0, time.UTC)
	queryEnd := time.Date(2026, time.March, 31, 8, 30, 0, 0, time.UTC)

	sessions := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	for i := range 18 {
		sessionID := sessions[i/6]
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID:    site.ID,
			SessionID: sessionID,
			PageID:    uuid.New(),
			Timestamp: dayBucket.Add(4*time.Hour + time.Duration(i)*time.Minute),
			Path:      "/",
		}); err != nil {
			t.Fatalf("create hit %d: %v", i, err)
		}
	}

	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}

	mustExec(
		"INSERT INTO hit_rollups_daily (site_id, bucket, pageviews, visitors) VALUES (?, ?, ?, ?)",
		site.ID,
		dayBucket,
		2,
		1,
	)
	mustExec(
		"INSERT INTO session_rollups_daily (site_id, bucket, sessions, bounced_sessions, duration_sum_seconds, pageviews) VALUES (?, ?, ?, ?, ?, ?)",
		site.ID,
		dayBucket,
		1,
		0,
		10.0,
		2,
	)

	stats, err := store.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  queryEnd.AddDate(0, 0, -7),
		End:    queryEnd,
	})
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}

	if stats.TotalPageviews != 18 {
		t.Fatalf("expected 18 pageviews, got %d", stats.TotalPageviews)
	}
	if stats.UniqueSessions != 3 {
		t.Fatalf("expected 3 sessions, got %d", stats.UniqueSessions)
	}

	var mar30 api.ChartDataPoint
	found := false
	for _, point := range stats.ChartData {
		if point.Time.Equal(dayBucket) {
			mar30 = point
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected chart bucket for %s", dayBucket.Format(time.RFC3339))
	}
	if mar30.Pageviews != 18 || mar30.Visitors != 3 {
		t.Fatalf("expected refreshed chart bucket to be 18/3, got %d/%d", mar30.Pageviews, mar30.Visitors)
	}

	var pageviews int
	var visitors int
	if err := store.db.QueryRowContext(
		ctx,
		"SELECT pageviews, visitors FROM hit_rollups_daily WHERE site_id = ? AND bucket = ?",
		site.ID,
		dayBucket,
	).Scan(&pageviews, &visitors); err != nil {
		t.Fatalf("query refreshed hit rollup: %v", err)
	}
	if pageviews != 18 || visitors != 3 {
		t.Fatalf("expected daily hit rollup to refresh to 18/3, got %d/%d", pageviews, visitors)
	}

	var sessionsCount int
	var rollupPageviews int
	if err := store.db.QueryRowContext(
		ctx,
		"SELECT sessions, pageviews FROM session_rollups_daily WHERE site_id = ? AND bucket = ?",
		site.ID,
		dayBucket,
	).Scan(&sessionsCount, &rollupPageviews); err != nil {
		t.Fatalf("query refreshed session rollup: %v", err)
	}
	if sessionsCount != 3 || rollupPageviews != 18 {
		t.Fatalf("expected daily session rollup to refresh to 3/18, got %d/%d", sessionsCount, rollupPageviews)
	}
}

func TestGetSiteStatsLast24HoursAcrossMonthBoundary(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "month-boundary.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	queryEnd := time.Date(2026, time.March, 31, 8, 30, 0, 0, time.UTC)
	queryStart := queryEnd.Add(-24 * time.Hour)

	beforeWindow := time.Date(2026, time.March, 30, 4, 52, 53, 0, time.UTC)
	insideWindow := time.Date(2026, time.March, 30, 9, 15, 0, 0, time.UTC)

	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: beforeWindow,
		Path:      "/before-window",
	}); err != nil {
		t.Fatalf("create hit before window: %v", err)
	}

	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: insideWindow,
		Path:      "/inside-window",
	}); err != nil {
		t.Fatalf("create hit inside window: %v", err)
	}

	stats, err := store.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  queryStart,
		End:    queryEnd,
	})
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}

	if stats.TotalPageviews != 1 {
		t.Fatalf("expected 1 pageview in rolling 24h window, got %d", stats.TotalPageviews)
	}
	if stats.UniqueSessions != 1 {
		t.Fatalf("expected 1 session in rolling 24h window, got %d", stats.UniqueSessions)
	}

	beforeBucket := time.Date(2026, time.March, 30, 4, 0, 0, 0, time.UTC)
	insideBucket := time.Date(2026, time.March, 30, 9, 0, 0, 0, time.UTC)
	for _, point := range stats.ChartData {
		switch {
		case point.Time.Equal(beforeBucket):
			if point.Pageviews != 0 || point.Visitors != 0 {
				t.Fatalf("expected %s bucket to be empty, got %d/%d", beforeBucket.Format(time.RFC3339), point.Pageviews, point.Visitors)
			}
		case point.Time.Equal(insideBucket):
			if point.Pageviews != 1 || point.Visitors != 1 {
				t.Fatalf("expected %s bucket to be 1/1, got %d/%d", insideBucket.Format(time.RFC3339), point.Pageviews, point.Visitors)
			}
		}
	}
}

func TestGetSiteStatsLastHourUsesFiveMinuteBuckets(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "minute-buckets.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	queryEnd := time.Date(2026, time.March, 31, 8, 30, 0, 0, time.UTC)
	queryStart := queryEnd.Add(-time.Hour)
	hitTimes := []time.Time{
		time.Date(2026, time.March, 31, 7, 35, 0, 0, time.UTC),
		time.Date(2026, time.March, 31, 7, 40, 0, 0, time.UTC),
		time.Date(2026, time.March, 31, 8, 5, 0, 0, time.UTC),
	}
	for i, ts := range hitTimes {
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID:    site.ID,
			SessionID: uuid.New(),
			PageID:    uuid.New(),
			Timestamp: ts,
			Path:      "/minute-bucket",
		}); err != nil {
			t.Fatalf("create hit %d: %v", i, err)
		}
	}

	stats, err := store.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  queryStart,
		End:    queryEnd,
	})
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}

	if len(stats.ChartData) != 13 {
		t.Fatalf("expected 13 five-minute buckets, got %d: %+v", len(stats.ChartData), stats.ChartData)
	}
	wantCounts := map[time.Time]int{
		time.Date(2026, time.March, 31, 7, 35, 0, 0, time.UTC): 1,
		time.Date(2026, time.March, 31, 7, 40, 0, 0, time.UTC): 1,
		time.Date(2026, time.March, 31, 8, 5, 0, 0, time.UTC):  1,
	}
	for _, point := range stats.ChartData {
		if want, ok := wantCounts[point.Time]; ok && point.Pageviews != want {
			t.Fatalf("bucket %s pageviews = %d, want %d", point.Time.Format(time.RFC3339), point.Pageviews, want)
		}
	}
}

func TestGetSiteStatsRefreshesStaleHourlyRollups(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "hourly-rollups.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	queryEnd := time.Date(2026, time.March, 31, 8, 30, 0, 0, time.UTC)
	hourBucket := time.Date(2026, time.March, 31, 6, 0, 0, 0, time.UTC)
	sessionID := uuid.New()
	for i := range 3 {
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID:    site.ID,
			SessionID: sessionID,
			PageID:    uuid.New(),
			Timestamp: hourBucket.Add(10*time.Minute + time.Duration(i)*5*time.Minute),
			Path:      "/pricing",
		}); err != nil {
			t.Fatalf("create hit %d: %v", i, err)
		}
	}

	if _, err := store.db.ExecContext(
		ctx,
		"INSERT INTO hit_rollups_hourly (site_id, bucket, pageviews, visitors) VALUES (?, ?, ?, ?)",
		site.ID,
		hourBucket,
		1,
		1,
	); err != nil {
		t.Fatalf("insert stale hourly rollup: %v", err)
	}

	stats, err := store.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  queryEnd.Add(-24 * time.Hour),
		End:    queryEnd,
	})
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}

	if stats.TotalPageviews != 3 {
		t.Fatalf("expected 3 pageviews, got %d", stats.TotalPageviews)
	}

	var sixAM api.ChartDataPoint
	found := false
	for _, point := range stats.ChartData {
		if point.Time.Equal(hourBucket) {
			sixAM = point
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected chart bucket for %s", hourBucket.Format(time.RFC3339))
	}
	if sixAM.Pageviews != 3 || sixAM.Visitors != 1 {
		t.Fatalf("expected refreshed hourly chart bucket to be 3/1, got %d/%d", sixAM.Pageviews, sixAM.Visitors)
	}

	var pageviews int
	if err := store.db.QueryRowContext(
		ctx,
		"SELECT pageviews FROM hit_rollups_hourly WHERE site_id = ? AND bucket = ?",
		site.ID,
		hourBucket,
	).Scan(&pageviews); err != nil {
		t.Fatalf("query refreshed hourly rollup: %v", err)
	}
	if pageviews != 3 {
		t.Fatalf("expected hourly hit rollup to refresh to 3, got %d", pageviews)
	}
}

func TestGetSiteStatsCountsCrossBucketSessionOnce(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "cross-bucket-session.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	queryEnd := time.Date(2026, time.March, 31, 8, 30, 0, 0, time.UTC)
	sessionID := uuid.New()
	timestamps := []time.Time{
		time.Date(2026, time.March, 31, 6, 55, 0, 0, time.UTC),
		time.Date(2026, time.March, 31, 7, 5, 0, 0, time.UTC),
	}
	for i, ts := range timestamps {
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID:    site.ID,
			SessionID: sessionID,
			PageID:    uuid.New(),
			Timestamp: ts,
			Path:      "/pricing",
		}); err != nil {
			t.Fatalf("create hit %d: %v", i, err)
		}
	}

	if err := store.ProcessDirtyRollups(ctx, site.ID); err != nil {
		t.Fatalf("ProcessDirtyRollups: %v", err)
	}

	stats, err := store.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  queryEnd.Add(-24 * time.Hour),
		End:    queryEnd,
	})
	if err != nil {
		t.Fatalf("GetSiteStats: %v", err)
	}

	if stats.TotalPageviews != 2 {
		t.Fatalf("expected 2 pageviews, got %d", stats.TotalPageviews)
	}
	if stats.UniqueSessions != 1 {
		t.Fatalf("expected 1 unique session across hourly buckets, got %d", stats.UniqueSessions)
	}
	if math.Abs(stats.BounceRate-0) > 0.001 {
		t.Fatalf("expected bounce rate 0, got %f", stats.BounceRate)
	}
	if math.Abs(stats.AvgSessionDuration-600) > 0.001 {
		t.Fatalf("expected avg session duration 600s, got %f", stats.AvgSessionDuration)
	}
	if math.Abs(stats.PagesPerSession-2) > 0.001 {
		t.Fatalf("expected pages per session 2, got %f", stats.PagesPerSession)
	}
}

func TestProcessDirtyRollupsRefreshesBucketsAndClearsState(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "dirty-refresh.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Date(2026, time.March, 31, 8, 0, 0, 0, time.UTC)
	sessionID := uuid.New()
	for i := range 2 {
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID:    site.ID,
			SessionID: sessionID,
			PageID:    uuid.New(),
			Timestamp: base.Add(time.Duration(i) * 10 * time.Minute),
			Path:      "/",
		}); err != nil {
			t.Fatalf("create hit %d: %v", i, err)
		}
	}

	if err := store.ProcessDirtyRollups(ctx, site.ID); err != nil {
		t.Fatalf("ProcessDirtyRollups: %v", err)
	}

	var hourlyPageviews int
	if err := store.DB().QueryRowContext(ctx, `
		SELECT pageviews FROM hit_rollups_hourly WHERE site_id = ? AND bucket = ?
	`, site.ID, base).Scan(&hourlyPageviews); err != nil {
		t.Fatalf("query hourly rollup: %v", err)
	}
	if hourlyPageviews != 2 {
		t.Fatalf("expected refreshed hourly pageviews 2, got %d", hourlyPageviews)
	}

	var dirtyCount int
	if err := store.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM rollup_dirty_buckets WHERE site_id = ?
	`, site.ID).Scan(&dirtyCount); err != nil {
		t.Fatalf("count dirty buckets: %v", err)
	}
	if dirtyCount != 0 {
		t.Fatalf("expected dirty buckets to be cleared, got %d", dirtyCount)
	}
}

func TestClearDirtyRollupBucketRangePreservesNewerMarkers(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "dirty-cutoff.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	oldBucket := time.Date(2026, time.March, 31, 8, 0, 0, 0, time.UTC)
	newBucket := time.Date(2026, time.March, 31, 9, 0, 0, 0, time.UTC)
	refreshStartedAt := time.Date(2026, time.March, 31, 10, 0, 0, 0, time.UTC)

	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO rollup_dirty_buckets (site_id, rollup_type, bucket_unit, bucket, updated_at)
		VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)
	`,
		site.ID, "hit", "hour", oldBucket, refreshStartedAt.Add(-time.Minute),
		site.ID, "hit", "hour", newBucket, refreshStartedAt.Add(time.Minute),
	); err != nil {
		t.Fatalf("insert dirty buckets: %v", err)
	}

	if err := store.clearDirtyRollupBucketRange(ctx, site.ID, dirtyRollupHit, "hour", oldBucket, newBucket, refreshStartedAt); err != nil {
		t.Fatalf("clearDirtyRollupBucketRange: %v", err)
	}

	var remaining []time.Time
	rows, err := store.DB().QueryContext(ctx, `
		SELECT bucket
		FROM rollup_dirty_buckets
		WHERE site_id = ?
		ORDER BY bucket ASC
	`, site.ID)
	if err != nil {
		t.Fatalf("query dirty buckets: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var bucket time.Time
		if err := rows.Scan(&bucket); err != nil {
			t.Fatalf("scan dirty bucket: %v", err)
		}
		remaining = append(remaining, bucket.UTC())
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate dirty buckets: %v", err)
	}

	if len(remaining) != 1 {
		t.Fatalf("expected 1 dirty bucket to remain, got %d", len(remaining))
	}
	if !remaining[0].Equal(newBucket) {
		t.Fatalf("expected newer dirty bucket %s to remain, got %s", newBucket.Format(time.RFC3339), remaining[0].Format(time.RFC3339))
	}
}
