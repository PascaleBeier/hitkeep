package main

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/hklog"
	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/opportunities"
)

func seedOpportunities(ctx context.Context, sharedStore, analyticsStore *database.Store, site api.Site, actorID uuid.UUID, from, to time.Time) (int, error) {
	if sharedStore == nil {
		return 0, fmt.Errorf("shared store is required")
	}
	if analyticsStore == nil {
		return 0, fmt.Errorf("analytics store is required")
	}
	teamID, err := sharedStore.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		return 0, fmt.Errorf("resolve site tenant: %w", err)
	}

	items, _, status, err := (opportunities.Service{Shared: sharedStore}).Generate(ctx, opportunities.GenerateInput{
		TeamID:          teamID,
		Site:            site,
		Store:           analyticsStore,
		From:            from,
		To:              to,
		ActorID:         actorID,
		ActorType:       "user",
		EffectiveUserID: actorID,
	})
	if err != nil {
		return 0, fmt.Errorf("generate demo opportunities: %w", err)
	}
	hklog.LoggerFromContext(ctx).Info("Opportunities seeded", "count", len(items), "ai_status", status)
	return len(items), nil
}
