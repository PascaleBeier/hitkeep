package database

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/webhooks"
)

func TestEnqueueWebhookEventCreatesDurableDeliveriesForEnabledSubscribers(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()

	instanceWebhook, _, err := store.CreateWebhook(ctx, nil, api.WebhookInput{
		Name: "Instance operations", URL: "https://instance.example/hook", Enabled: true, Events: []string{webhooks.EventGoalCreated},
	})
	if err != nil {
		t.Fatalf("create instance webhook: %v", err)
	}
	siteWebhook, _, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "Site operations", URL: "https://site.example/hook", Enabled: true, Events: []string{webhooks.EventGoalCreated},
	})
	if err != nil {
		t.Fatalf("create site webhook: %v", err)
	}
	_, _, err = store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "Disabled", URL: "https://disabled.example/hook", Enabled: false, Events: []string{webhooks.EventGoalCreated},
	})
	if err != nil {
		t.Fatalf("create disabled webhook: %v", err)
	}
	_, _, err = store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "Other event", URL: "https://other.example/hook", Enabled: true, Events: []string{webhooks.EventGoalDeleted},
	})
	if err != nil {
		t.Fatalf("create other webhook: %v", err)
	}

	eventID := uuid.New()
	goalID := uuid.New()
	occurredAt := time.Date(2026, 7, 10, 10, 30, 0, 0, time.UTC)
	jobs, err := store.EnqueueWebhookEvent(ctx, WebhookEventInput{
		ID:         eventID,
		SiteID:     &site.ID,
		EventType:  webhooks.EventGoalCreated,
		APIVersion: "2.10",
		OccurredAt: occurredAt,
		Data: map[string]any{
			"site_id": site.ID.String(),
			"goal_id": goalID.String(),
			"name":    "Signup",
		},
	})
	if err != nil {
		t.Fatalf("enqueue webhook event: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 enabled subscribers, got %+v", jobs)
	}
	seen := map[uuid.UUID]bool{}
	for _, job := range jobs {
		seen[job.WebhookID] = true
		if job.EventID != eventID || job.DeliveryID == uuid.Nil {
			t.Fatalf("unexpected stable IDs in %+v", job)
		}
		var payload struct {
			APIVersion string         `json:"api_version"`
			ID         uuid.UUID      `json:"id"`
			DeliveryID uuid.UUID      `json:"delivery_id"`
			Type       string         `json:"type"`
			CreatedAt  time.Time      `json:"created_at"`
			Data       map[string]any `json:"data"`
		}
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.APIVersion != "2.10" || payload.ID != eventID || payload.DeliveryID != job.DeliveryID || payload.Type != webhooks.EventGoalCreated {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		if payload.Data["goal_id"] != goalID.String() {
			t.Fatalf("unexpected payload data: %+v", payload.Data)
		}
	}
	if !seen[instanceWebhook.ID] || !seen[siteWebhook.ID] {
		t.Fatalf("expected instance and site subscribers, got %+v", seen)
	}

	due, err := store.ListDueWebhookDeliveryJobs(ctx, time.Now().UTC().Add(time.Minute), 20)
	if err != nil {
		t.Fatalf("list due deliveries: %v", err)
	}
	if len(due) != 2 {
		t.Fatalf("expected durable due rows, got %+v", due)
	}
}

func TestEnqueueWebhookEventDoesNotPersistWithoutEnabledSubscribers(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()

	if _, _, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "Disabled", URL: "https://disabled.example/hook", Enabled: false, Events: []string{webhooks.EventImportFailed},
	}); err != nil {
		t.Fatalf("create disabled webhook: %v", err)
	}
	jobs, err := store.EnqueueWebhookEvent(ctx, WebhookEventInput{
		SiteID: &site.ID, EventType: webhooks.EventImportFailed, APIVersion: "2.10", Data: map[string]any{"site_id": site.ID.String()},
	})
	if err != nil {
		t.Fatalf("enqueue event: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("disabled webhook must not create jobs: %+v", jobs)
	}
	var count int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM webhook_events").Scan(&count); err != nil {
		t.Fatalf("count webhook events: %v", err)
	}
	if count != 0 {
		t.Fatalf("disabled webhook must not create durable event rows, got %d", count)
	}
}
