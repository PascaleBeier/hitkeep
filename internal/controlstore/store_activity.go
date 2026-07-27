package controlstore

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

// RecordFirstHitCloudConversions records the first hit in a batch as a
// control-plane billing milestone without giving the control store access to
// any analytics table.
func (s *Store) RecordFirstHitCloudConversions(ctx context.Context, hits []*api.Hit) error {
	firstBySite := make(map[uuid.UUID]time.Time)
	for _, hit := range hits {
		if hit == nil || hit.SiteID == uuid.Nil {
			continue
		}
		at := hit.Timestamp.UTC()
		if previous, ok := firstBySite[hit.SiteID]; !ok || at.Before(previous) {
			firstBySite[hit.SiteID] = at
		}
	}
	for siteID, occurredAt := range firstBySite {
		tenantID, err := s.GetSiteTenantID(ctx, siteID)
		if err != nil {
			return fmt.Errorf("resolve site tenant for first hit conversion: %w", err)
		}
		if _, err := s.RecordCloudConversionEvent(ctx, CloudConversionEvent{
			TenantID:   tenantID,
			EventName:  CloudConversionFirstHitReceived,
			OccurredAt: occurredAt,
		}); err != nil {
			return fmt.Errorf("record first hit conversion: %w", err)
		}
	}
	return nil
}
