package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestGetDailyPageviewsForPeriodRefreshesStaleDailyRollups(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "report-chart.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	dayBucket := time.Date(2026, time.March, 30, 0, 0, 0, 0, time.UTC)
	for i := range 4 {
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID:    site.ID,
			SessionID: uuid.New(),
			PageID:    uuid.New(),
			Timestamp: dayBucket.Add(4*time.Hour + time.Duration(i)*time.Minute),
			Path:      "/",
		}); err != nil {
			t.Fatalf("create hit %d: %v", i, err)
		}
	}

	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO hit_rollups_daily (site_id, bucket, pageviews, visitors) VALUES (?, ?, ?, ?)
	`, site.ID, dayBucket, 1, 1); err != nil {
		t.Fatalf("insert stale daily rollup: %v", err)
	}

	got, err := store.GetDailyPageviewsForPeriod(ctx, site.ID, dayBucket, dayBucket.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("GetDailyPageviewsForPeriod: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 daily point, got %d", len(got))
	}
	if got[0] != 4 {
		t.Fatalf("expected refreshed daily pageviews 4, got %d", got[0])
	}
}
