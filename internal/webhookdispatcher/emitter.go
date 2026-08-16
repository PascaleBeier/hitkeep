package webhookdispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

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
	logger     *slog.Logger
}

func NewEmitter(store *database.Store, producer Producer, runtimeVersion string, logger *slog.Logger) *Emitter {
	if logger == nil {
		panic("webhookdispatcher: logger is required")
	}
	return &Emitter{store: store, producer: producer, apiVersion: webhooks.MinorAPIVersion(runtimeVersion), logger: logger}
}

func (e *Emitter) Emit(ctx context.Context, event webhooks.Event) (webhooks.Emission, error) {
	emissions, err := e.EmitBatch(ctx, []webhooks.Event{event})
	if err != nil {
		return webhooks.Emission{}, err
	}
	return emissions[0], nil
}

func (e *Emitter) HasSubscribers(ctx context.Context, siteID *uuid.UUID, eventType string) (bool, error) {
	if e == nil || e.store == nil {
		return false, nil
	}
	return e.store.HasEnabledWebhookSubscribers(ctx, siteID, eventType)
}

func (e *Emitter) EmitBatch(ctx context.Context, events []webhooks.Event) ([]webhooks.Emission, error) {
	emissions := make([]webhooks.Emission, len(events))
	if e == nil || e.store == nil {
		return emissions, nil
	}
	inputs := make([]database.WebhookEventInput, len(events))
	for index, event := range events {
		eventID := event.ID
		deduplicate := eventID != uuid.Nil
		if eventID == uuid.Nil {
			eventID = uuid.New()
		}
		emissions[index] = webhooks.Emission{EventID: eventID}
		inputs[index] = database.WebhookEventInput{
			ID:                        eventID,
			SiteID:                    event.SiteID,
			TargetWebhookID:           event.TargetWebhookID,
			EventType:                 event.Type,
			APIVersion:                e.apiVersion,
			Data:                      event.Data,
			PreserveAfterSiteDeletion: event.PreserveAfterSiteDeletion,
			Deduplicate:               deduplicate,
		}
	}
	jobsByEvent, err := e.store.EnqueueWebhookEvents(ctx, inputs)
	if err != nil {
		return nil, fmt.Errorf("persist webhook event batch: %w", err)
	}

	for index, jobs := range jobsByEvent {
		emissions[index].DeliveryIDs = make([]uuid.UUID, 0, len(jobs))
		for _, job := range jobs {
			emissions[index].DeliveryIDs = append(emissions[index].DeliveryIDs, job.DeliveryID)
			e.publishDelivery(ctx, job.DeliveryID)
		}
	}
	return emissions, nil
}

func (e *Emitter) PublishDeliveries(ctx context.Context, deliveryIDs []uuid.UUID) {
	for _, deliveryID := range deliveryIDs {
		e.publishDelivery(ctx, deliveryID)
	}
}

func (e *Emitter) publishDelivery(ctx context.Context, deliveryID uuid.UUID) {
	if e == nil || e.producer == nil {
		return
	}
	body, err := json.Marshal(WebhookDeliveryMessage{DeliveryID: deliveryID})
	if err != nil {
		e.logger.Error("Failed to marshal webhook delivery job", "error", err, "delivery_id", deliveryID)
		return
	}
	now := time.Now().UTC()
	marked := true
	if err := e.store.MarkWebhookDeliveryQueued(ctx, deliveryID, now); err != nil {
		marked = false
		e.logger.Warn("Webhook delivery publish could not acquire queue marker", "error", err, "delivery_id", deliveryID)
	}
	if err := e.producer.Publish(Topic, body); err != nil {
		e.logger.Warn("Webhook delivery publish deferred to sweeper", "error", err, "delivery_id", deliveryID)
		if marked {
			if clearErr := e.store.ClearWebhookDeliveryQueued(ctx, deliveryID, time.Now().UTC()); clearErr != nil {
				e.logger.Warn("Failed to release webhook delivery queue marker", "error", clearErr, "delivery_id", deliveryID)
			}
		}
		return
	}
}

type WebhookDeliveryMessage struct {
	DeliveryID uuid.UUID `json:"delivery_id"`
}
