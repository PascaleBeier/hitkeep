package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestConversionDefinitionUpdatesPreserveIdentityAndInvalidateRollups(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	userID, err := store.CreateUser(ctx, "definition-update@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "definition-update.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	goal := api.Goal{SiteID: site.ID, Name: "Old goal", Type: "path", Value: "/old", CreatedAt: now}
	if err := store.CreateGoal(ctx, &goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	funnel := api.Funnel{SiteID: site.ID, Name: "Old funnel", Steps: []api.FunnelStep{{Type: "path", Value: "/one"}, {Type: "path", Value: "/two"}}, CreatedAt: now}
	if err := store.CreateFunnel(ctx, &funnel); err != nil {
		t.Fatalf("create funnel: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, "INSERT INTO goal_rollups_hourly (site_id, goal_id, bucket, conversions) VALUES (?, ?, ?, ?)", site.ID, goal.ID, now, 3); err != nil {
		t.Fatalf("seed goal rollup: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, "INSERT INTO funnel_rollups_hourly (site_id, funnel_id, bucket, entries, completions) VALUES (?, ?, ?, ?, ?)", site.ID, funnel.ID, now, 5, 2); err != nil {
		t.Fatalf("seed funnel rollup: %v", err)
	}

	goal.Name, goal.Type, goal.Value = "New goal", "event", "signup"
	if err := store.UpdateGoal(ctx, &goal); err != nil {
		t.Fatalf("update goal: %v", err)
	}
	funnel.Name = "New funnel"
	funnel.Steps = []api.FunnelStep{{Type: "event", Value: "started"}, {Type: "event", Value: "finished"}}
	if err := store.UpdateFunnel(ctx, &funnel); err != nil {
		t.Fatalf("update funnel: %v", err)
	}
	if !goal.CreatedAt.Equal(now) || !funnel.CreatedAt.Equal(now) {
		t.Fatalf("expected creation timestamps to remain stable: goal=%s funnel=%s", goal.CreatedAt, funnel.CreatedAt)
	}
	for table, id := range map[string]uuid.UUID{"goal_rollups_hourly": goal.ID, "funnel_rollups_hourly": funnel.ID} {
		var count int
		column := "goal_id"
		if table == "funnel_rollups_hourly" {
			column = "funnel_id"
		}
		if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE site_id = ? AND "+column+" = ?", site.ID, id).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("expected %s to be invalidated", table)
		}
	}

	missing := api.Goal{ID: uuid.New(), SiteID: site.ID, Name: "Missing", Type: "path", Value: "/missing"}
	if err := store.UpdateGoal(ctx, &missing); !errors.Is(err, ErrGoalNotFound) {
		t.Fatalf("expected ErrGoalNotFound, got %v", err)
	}
}
