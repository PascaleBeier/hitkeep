package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

// The batched activity writer must preserve the per-hit upsert semantics:
// first_hit_at = min, last_hit_at = max, hostname follows the latest hit but
// never regresses to empty, and hourly counts sum across the batch.
func TestRecordHitActivityAggregatesBatchPerSite(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "activity-batch@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "activity-batch.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour)
	hits := []*api.Hit{
		{SiteID: site.ID, Timestamp: base.Add(2 * time.Minute), Hostname: new("late.activity-batch.test"), TrackerVersion: "2.0"},
		{SiteID: site.ID, Timestamp: base, Hostname: new("early.activity-batch.test"), TrackerVersion: "1.0"},
		{SiteID: site.ID, Timestamp: base.Add(50 * time.Minute)}, // latest, no hostname: must keep previous hostname
		{SiteID: site.ID, Timestamp: base.Add(time.Hour)},        // second hourly bucket
		nil,
		{SiteID: uuid.Nil, Timestamp: base},
	}

	if err := store.RecordHitActivity(ctx, hits); err != nil {
		t.Fatalf("record hit activity: %v", err)
	}

	status, err := store.GetSiteTrackingStatus(ctx, site.ID, base.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("get tracking status: %v", err)
	}
	if status == nil {
		t.Fatal("expected tracking status")
	}
	if status.FirstHitAt == nil || !status.FirstHitAt.Equal(base) {
		t.Fatalf("expected first hit at %s, got %v", base, status.FirstHitAt)
	}
	wantLast := base.Add(time.Hour)
	if status.LastHitAt == nil || !status.LastHitAt.Equal(wantLast) {
		t.Fatalf("expected last hit at %s, got %v", wantLast, status.LastHitAt)
	}
	if status.LastHostname != "late.activity-batch.test" {
		t.Fatalf("expected hostname of latest hit that carried one, got %q", status.LastHostname)
	}

	var bucketCount, totalHits int
	rows, err := store.DB().QueryContext(ctx,
		"SELECT hits FROM site_activity_hourly_counts WHERE site_id = ?", site.ID)
	if err != nil {
		t.Fatalf("query hourly counts: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var hitsInBucket int
		if err := rows.Scan(&hitsInBucket); err != nil {
			t.Fatalf("scan hourly count: %v", err)
		}
		bucketCount++
		totalHits += hitsInBucket
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read hourly counts: %v", err)
	}
	if bucketCount != 2 {
		t.Fatalf("expected 2 hourly buckets, got %d", bucketCount)
	}
	if totalHits != 4 {
		t.Fatalf("expected 4 counted hits, got %d", totalHits)
	}
}

func TestRecordEventActivityAggregatesBatchPerSite(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "event-activity-batch@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "event-activity-batch.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour)
	events := []*api.Event{
		{SiteID: site.ID, Timestamp: base.Add(time.Minute), Name: "signup"},
		{SiteID: site.ID, Timestamp: base, Name: "outbound_click"}, // automatic event
		{SiteID: site.ID, Timestamp: base.Add(2 * time.Minute), Name: "purchase"},
	}

	if err := store.RecordEventActivity(ctx, events); err != nil {
		t.Fatalf("record event activity: %v", err)
	}

	status, err := store.GetSiteTrackingStatus(ctx, site.ID, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("get tracking status: %v", err)
	}
	if status == nil {
		t.Fatal("expected tracking status")
	}
	wantLast := base.Add(2 * time.Minute)
	if status.LastEventAt == nil || !status.LastEventAt.Equal(wantLast) {
		t.Fatalf("expected last event at %s, got %v", wantLast, status.LastEventAt)
	}
	if status.LastEventName != "purchase" {
		t.Fatalf("expected last event name purchase, got %q", status.LastEventName)
	}
	if status.LastAutomaticEventAt == nil || !status.LastAutomaticEventAt.Equal(base) {
		t.Fatalf("expected last automatic event at %s, got %v", base, status.LastAutomaticEventAt)
	}
	if status.LastAutomaticEventName != "outbound_click" {
		t.Fatalf("expected automatic event outbound_click, got %q", status.LastAutomaticEventName)
	}

	var totalEvents int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COALESCE(SUM(events), 0) FROM site_activity_hourly_counts WHERE site_id = ?", site.ID,
	).Scan(&totalEvents); err != nil {
		t.Fatalf("query event counts: %v", err)
	}
	if totalEvents != 3 {
		t.Fatalf("expected 3 counted events, got %d", totalEvents)
	}
}
