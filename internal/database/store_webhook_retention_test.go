package database

import (
	"context"
	"testing"
	"time"

	"hitkeep/internal/api"
	"hitkeep/internal/webhooks"
)

func TestDeleteWebhookHistoryBeforeKeepsPendingAndRemovesOldTerminalRows(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()
	if _, _, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "Retention", URL: "https://retention.example/hook", Enabled: true, Events: []string{webhooks.EventGoalCreated},
	}); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	oldJobs, _ := store.EnqueueWebhookEvent(ctx, WebhookEventInput{SiteID: &site.ID, EventType: webhooks.EventGoalCreated, APIVersion: "2.10", Data: map[string]any{"kind": "old"}})
	pendingJobs, _ := store.EnqueueWebhookEvent(ctx, WebhookEventInput{SiteID: &site.ID, EventType: webhooks.EventGoalCreated, APIVersion: "2.10", Data: map[string]any{"kind": "pending"}})
	old := time.Now().UTC().AddDate(0, 0, -31)
	if _, err := store.DB().ExecContext(ctx, `
		UPDATE webhook_deliveries SET status = ?, completed_at = ?, created_at = ?, updated_at = ? WHERE id = ?
	`, WebhookDeliverySucceeded, old, old, old, oldJobs[0].DeliveryID); err != nil {
		t.Fatalf("age terminal delivery: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `UPDATE webhook_events SET created_at = ? WHERE id = ?`, old, oldJobs[0].EventID); err != nil {
		t.Fatalf("age event: %v", err)
	}

	deleted, err := store.DeleteWebhookHistoryBefore(ctx, time.Now().UTC().AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("delete old webhook history: %v", err)
	}
	if deleted.Deliveries != 1 || deleted.Events != 1 {
		t.Fatalf("unexpected retention result: %+v", deleted)
	}
	if delivery, _ := store.GetWebhookDelivery(ctx, oldJobs[0].DeliveryID); delivery != nil {
		t.Fatalf("old terminal delivery was retained: %+v", delivery)
	}
	if delivery, _ := store.GetWebhookDelivery(ctx, pendingJobs[0].DeliveryID); delivery == nil {
		t.Fatal("pending delivery must not be removed by retention")
	}
}
