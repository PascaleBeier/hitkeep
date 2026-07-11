package webhooks

import (
	"context"

	"github.com/google/uuid"
)

type Event struct {
	ID                        uuid.UUID
	Type                      string
	SiteID                    *uuid.UUID
	TargetWebhookID           *uuid.UUID
	Data                      map[string]any
	PreserveAfterSiteDeletion bool
}

type EventEmitter interface {
	Emit(ctx context.Context, event Event) (Emission, error)
}

// BatchEventEmitter is an optional extension used by high-volume producers to
// persist multiple events in one outbox transaction.
type BatchEventEmitter interface {
	EmitBatch(ctx context.Context, events []Event) ([]Emission, error)
}

// SubscriptionChecker is an optional extension used by producers to avoid
// computing event payloads when no enabled webhook can receive them.
type SubscriptionChecker interface {
	HasSubscribers(ctx context.Context, siteID *uuid.UUID, eventType string) (bool, error)
}

// DeliveryPublisher publishes delivery IDs that were persisted by a domain
// transaction rather than through EventEmitter.Emit.
type DeliveryPublisher interface {
	PublishDeliveries(ctx context.Context, deliveryIDs []uuid.UUID)
}

type Emission struct {
	EventID     uuid.UUID
	DeliveryIDs []uuid.UUID
}
