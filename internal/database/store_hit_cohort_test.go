package database

import (
	"bytes"
	"context"
	"encoding/csv"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestHitQueriesAndExportsUseConversionSessionCohorts(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestFixtureStore(t)
	userID, err := store.CreateUser(ctx, "cohorts@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "cohorts.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	matchedSession := uuid.New()
	unmatchedSession := uuid.New()
	for _, hit := range []*api.Hit{
		{SiteID: site.ID, SessionID: matchedSession, PageID: uuid.New(), Timestamp: now, Path: "/landing"},
		{SiteID: site.ID, SessionID: matchedSession, PageID: uuid.New(), Timestamp: now.Add(time.Minute), Path: "/after-event"},
		{SiteID: site.ID, SessionID: unmatchedSession, PageID: uuid.New(), Timestamp: now, Path: "/other"},
	} {
		if err := store.CreateHit(ctx, hit); err != nil {
			t.Fatalf("create hit: %v", err)
		}
	}
	if err := store.CreateEvent(ctx, &api.Event{SiteID: site.ID, SessionID: matchedSession, Name: "signup_completed", Timestamp: now.Add(30 * time.Second)}); err != nil {
		t.Fatalf("create event: %v", err)
	}
	goal := api.Goal{SiteID: site.ID, Name: "Signup", Type: "event", Value: "signup_completed"}
	if err := store.CreateGoal(ctx, &goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	funnel := api.Funnel{SiteID: site.ID, Name: "Event-only", Steps: []api.FunnelStep{{Type: "event", Value: "signup_completed"}, {Type: "event", Value: "activated"}}}
	if err := store.CreateFunnel(ctx, &funnel); err != nil {
		t.Fatalf("create funnel: %v", err)
	}

	base := api.HitQueryParams{SiteID: site.ID, UserID: userID, Start: now.Add(-time.Hour), End: now.Add(time.Hour), Limit: 10}
	base.GoalIDs = []uuid.UUID{goal.ID}
	goalHits, err := store.GetHits(ctx, base)
	if err != nil {
		t.Fatalf("get goal cohort hits: %v", err)
	}
	if goalHits.Total != 2 {
		t.Fatalf("expected both page hits from the matching event-goal session, got %d", goalHits.Total)
	}

	base.GoalIDs = nil
	base.FunnelIDs = []uuid.UUID{funnel.ID}
	funnelHits, err := store.GetHits(ctx, base)
	if err != nil {
		t.Fatalf("get funnel cohort hits: %v", err)
	}
	if funnelHits.Total != 2 || funnelHits.Data[0].Path != "/after-event" {
		t.Fatalf("expected complete event-only funnel session traffic, got %+v", funnelHits.Data)
	}

	base.Filters = []api.Filter{{Type: "path", Value: "/after-event"}}
	filtered, err := store.GetHits(ctx, base)
	if err != nil {
		t.Fatalf("get intersected funnel cohort hits: %v", err)
	}
	if filtered.Total != 1 || filtered.Data[0].Path != "/after-event" {
		t.Fatalf("expected dimension filter to intersect the cohort, got %+v", filtered.Data)
	}

	var output bytes.Buffer
	if err := store.ExportHitsCSV(ctx, base, &output); err != nil {
		t.Fatalf("export cohort hits: %v", err)
	}
	rows, err := csv.NewReader(&output).ReadAll()
	if err != nil {
		t.Fatalf("read cohort export: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected export header plus the same one matching hit, got %d rows", len(rows))
	}
}
