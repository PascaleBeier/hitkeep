package webhookdispatcher

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"hitkeep/internal/api"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/webhooks"
)

type recordingProducer struct {
	messages []WebhookDeliveryMessage
}

func (p *recordingProducer) Publish(topic string, body []byte) error {
	if topic != Topic {
		return nil
	}
	var message WebhookDeliveryMessage
	if err := json.Unmarshal(body, &message); err != nil {
		return err
	}
	p.messages = append(p.messages, message)
	return nil
}

func TestSweeperRecoversStaleProcessingAndPublishesDueDeliveries(t *testing.T) {
	store, _, site := setupDispatcherStore(t)
	defer store.Close()
	if _, _, err := store.CreateWebhook(context.Background(), &site.ID, api.WebhookInput{
		Name: "Recovery", URL: "https://recovery.example/hook", Enabled: true, Events: []string{webhooks.EventGoalCreated},
	}); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	jobs, err := store.EnqueueWebhookEvent(context.Background(), database.WebhookEventInput{
		SiteID: &site.ID, EventType: webhooks.EventGoalCreated, APIVersion: "2.10", Data: map[string]any{},
	})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("enqueue: jobs=%+v err=%v", jobs, err)
	}
	stale := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := store.DB().ExecContext(context.Background(), `
		UPDATE webhook_deliveries SET status = ?, last_attempt_at = ?, next_attempt_at = NULL WHERE id = ?
	`, database.WebhookDeliveryProcessing, stale, jobs[0].DeliveryID); err != nil {
		t.Fatalf("mark stale processing: %v", err)
	}

	producer := &recordingProducer{}
	sweeper := NewSweeper(store, producer, config.Config{
		WebhookDeliveryTimeoutSeconds: 2,
		WebhookRetentionDays:          30,
	}, testLogger())
	if err := sweeper.RunOnce(context.Background(), time.Now().UTC()); err != nil {
		t.Fatalf("run sweeper: %v", err)
	}
	if len(producer.messages) != 1 || producer.messages[0].DeliveryID != jobs[0].DeliveryID {
		t.Fatalf("expected recovered delivery publish, got %+v", producer.messages)
	}
	delivery, err := store.GetWebhookDelivery(context.Background(), jobs[0].DeliveryID)
	if err != nil || delivery == nil || delivery.Status != database.WebhookDeliveryRetrying {
		t.Fatalf("expected recovered retrying delivery, got %+v err=%v", delivery, err)
	}
}

func TestSweeperDoesNotRepublishRecentlyQueuedDelivery(t *testing.T) {
	store, _, site := setupDispatcherStore(t)
	defer store.Close()
	if _, _, err := store.CreateWebhook(context.Background(), &site.ID, api.WebhookInput{
		Name: "Recovery", URL: "https://recovery.example/hook", Enabled: true, Events: []string{webhooks.EventGoalCreated},
	}); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	if _, err := store.EnqueueWebhookEvent(context.Background(), database.WebhookEventInput{
		SiteID: &site.ID, EventType: webhooks.EventGoalCreated, APIVersion: "2.10", Data: map[string]any{},
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	producer := &recordingProducer{}
	sweeper := NewSweeper(store, producer, config.Config{WebhookSweepSeconds: 30}, testLogger())
	now := time.Now().UTC()
	if err := sweeper.RunOnce(context.Background(), now); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if err := sweeper.RunOnce(context.Background(), now.Add(10*time.Second)); err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if len(producer.messages) != 1 {
		t.Fatalf("expected one publish while queue lease is active, got %d", len(producer.messages))
	}
}
