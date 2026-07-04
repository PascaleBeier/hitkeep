package database

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestGetSiteOverviewStatsReturnsLightweightKpisAndChart(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "overview-stats.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Date(2026, time.July, 1, 12, 0, 0, 0, time.UTC)
	returningSession := uuid.New()
	bouncedSession := uuid.New()
	for _, hit := range []api.Hit{
		{SiteID: site.ID, SessionID: returningSession, PageID: uuid.New(), Timestamp: base.Add(-2 * time.Hour), Path: "/"},
		{SiteID: site.ID, SessionID: returningSession, PageID: uuid.New(), Timestamp: base.Add(-90 * time.Minute), Path: "/pricing"},
		{SiteID: site.ID, SessionID: bouncedSession, PageID: uuid.New(), Timestamp: base.Add(-30 * time.Minute), Path: "/docs"},
	} {
		if err := store.CreateHit(ctx, &hit); err != nil {
			t.Fatalf("create hit: %v", err)
		}
	}

	stats, err := store.GetSiteOverviewStats(ctx, api.AnalyticsParams{
		SiteID: site.ID,
		UserID: userID,
		Start:  base.Add(-24 * time.Hour),
		End:    base,
	})
	if err != nil {
		t.Fatalf("GetSiteOverviewStats: %v", err)
	}

	if stats.SiteID != site.ID || stats.Status != api.SiteOverviewStatsReady {
		t.Fatalf("unexpected overview status: %+v", stats)
	}
	if stats.TotalPageviews != 3 {
		t.Fatalf("expected 3 pageviews, got %d", stats.TotalPageviews)
	}
	if stats.UniqueSessions != 2 {
		t.Fatalf("expected 2 sessions, got %d", stats.UniqueSessions)
	}
	if math.Abs(stats.BounceRate-50) > 0.001 {
		t.Fatalf("expected 50%% bounce rate, got %.4f", stats.BounceRate)
	}
	if len(stats.ChartData) == 0 {
		t.Fatalf("expected chart data")
	}
}
