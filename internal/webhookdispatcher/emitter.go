package webhookdispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"hitkeep/internal/database"
	"hitkeep/internal/webhooks"
)

const Topic = "webhook_deliveries"

type Producer interface {
	Publish(topic string, body []byte) error
}

type Emitter struct {
	store      *database.Store
	producer   Producer
	apiVersion string
}

func NewEmitter(store *database.Store, producer Producer, runtimeVersion string) *Emitter {
	return &Emitter{store: store, producer: producer, apiVersion: webhooks.MinorAPIVersion(runtimeVersion)}
}

func (e *Emitter) Emit(ctx context.Context, event webhooks.Event) (webhooks.Emission, error) {
	if e == nil || e.store == nil {
		return webhooks.Emission{}, nil
	}
	eventID := event.ID
	if eventID == uuid.Nil {
		eventID = uuid.New()
	}
	jobs, err := e.store.EnqueueWebhookEvent(ctx, database.WebhookEventInput{
		ID:                        eventID,
		SiteID:                    event.SiteID,
		TargetWebhookID:           event.TargetWebhookID,
		EventType:                 event.Type,
		APIVersion:                e.apiVersion,
		Data:                      event.Data,
		PreserveAfterSiteDeletion: event.PreserveAfterSiteDeletion,
	})
	if err != nil {
		return webhooks.Emission{}, fmt.Errorf("persist webhook event: %w", err)
	}

	emission := webhooks.Emission{EventID: eventID, DeliveryIDs: make([]uuid.UUID, 0, len(jobs))}
	for _, job := range jobs {
		emission.DeliveryIDs = append(emission.DeliveryIDs, job.DeliveryID)
		if e.producer == nil {
			continue
		}
		body, err := json.Marshal(WebhookDeliveryMessage{DeliveryID: job.DeliveryID})
		if err != nil {
			slog.Error("Failed to marshal webhook delivery job", "error", err, "delivery_id", job.DeliveryID)
			continue
		}
		if err := e.producer.Publish(Topic, body); err != nil {
			slog.Warn("Webhook delivery publish deferred to sweeper", "error", err, "delivery_id", job.DeliveryID)
		}
	}
	return emission, nil
}

type WebhookDeliveryMessage struct {
	DeliveryID uuid.UUID `json:"delivery_id"`
}
