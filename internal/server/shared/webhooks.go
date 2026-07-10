package shared

import (
	"context"
	"log/slog"

	"hitkeep/internal/webhooks"
)

func (c *Context) EmitWebhookEvent(ctx context.Context, event webhooks.Event) {
	if c == nil || c.Webhooks == nil {
		return
	}
	if _, err := c.Webhooks.Emit(ctx, event); err != nil {
		slog.Warn("Operational action completed but webhook emission was deferred", "error", err, "event_type", event.Type)
	}
}
