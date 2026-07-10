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

type Emission struct {
	EventID     uuid.UUID
	DeliveryIDs []uuid.UUID
}
