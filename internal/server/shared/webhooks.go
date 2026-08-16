package shared

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"hitkeep/internal/database"
	"hitkeep/internal/webhooks"
)

func (c *Context) EmitWebhookEvent(ctx context.Context, event webhooks.Event) {
	if c == nil || c.Webhooks == nil {
		return
	}
	if _, err := c.Webhooks.Emit(ctx, event); err != nil {
		LoggerFromContext(ctx).Warn("Operational action completed but webhook emission was deferred", "error", err, "event_type", event.Type)
	}
}

func (c *Context) DeleteSiteWithWebhookEvent(ctx context.Context, siteID uuid.UUID, data map[string]any) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("site store is not configured")
	}
	version := "0.0"
	if c.Config != nil {
		version = webhooks.MinorAPIVersion(c.Config.Version)
	}
	input := database.WebhookEventInput{
		SiteID:                    &siteID,
		EventType:                 webhooks.EventSiteDeleted,
		APIVersion:                version,
		Data:                      data,
		PreserveAfterSiteDeletion: true,
	}
	var (
		jobs []database.WebhookDeliveryJob
		err  error
	)
	if c.TenantStores != nil {
		jobs, err = c.TenantStores.DeleteSiteWithWebhookEvent(ctx, siteID, input)
	} else {
		jobs, err = c.Store.DeleteSiteWithWebhookEvent(ctx, siteID, input)
	}
	if err != nil {
		return err
	}
	if publisher, ok := c.Webhooks.(webhooks.DeliveryPublisher); ok {
		ids := make([]uuid.UUID, 0, len(jobs))
		for _, job := range jobs {
			ids = append(ids, job.DeliveryID)
		}
		publisher.PublishDeliveries(ctx, ids)
	}
	return nil
}
