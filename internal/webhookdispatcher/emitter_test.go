package webhookdispatcher

import (
	"context"
	"errors"
	"testing"
	"time"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/webhooks"
)

type failingProducer struct {
	publishes int
}

func (p *failingProducer) Publish(topic string, body []byte) error {
	p.publishes++
	if topic != Topic || len(body) == 0 {
		return errors.New("unexpected publish")
	}
	return errors.New("nsq unavailable")
}

func TestEmitterPersistsBeforePublishingAndIgnoresProducerFailure(t *testing.T) {
	store, _, site := setupDispatcherStore(t)
	defer store.Close()
	if _, _, err := store.CreateWebhook(context.Background(), &site.ID, api.WebhookInput{
		Name: "Ops", URL: "https://ops.example/hook", Enabled: true, Events: []string{webhooks.EventGoalCreated},
	}); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	producer := &failingProducer{}
	emitter := NewEmitter(store, producer, "2.10.2")
	emission, err := emitter.Emit(context.Background(), webhooks.Event{
		Type:   webhooks.EventGoalCreated,
		SiteID: &site.ID,
		Data:   map[string]any{"site_id": site.ID.String(), "goal_id": "goal-1"},
	})
	if err != nil {
		t.Fatalf("producer failure must not fail emission: %v", err)
	}
	if emission.EventID.String() == "00000000-0000-0000-0000-000000000000" || len(emission.DeliveryIDs) != 1 {
		t.Fatalf("unexpected emission: %+v", emission)
	}
	if producer.publishes != 1 {
		t.Fatalf("expected one best-effort publish, got %d", producer.publishes)
	}
	due, err := store.ListDueWebhookDeliveryJobs(context.Background(), time.Now().UTC().Add(time.Minute), 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("delivery intent must remain durable, due=%+v err=%v", due, err)
	}
}

func setupDispatcherStore(t *testing.T) (*database.Store, string, api.Site) {
	t.Helper()
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "dispatcher@example.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "dispatcher.example.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	return store, userID.String(), *site
}
