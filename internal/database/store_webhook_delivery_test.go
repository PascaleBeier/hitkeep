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

func TestHasEnabledWebhookSubscribersRespectsSiteAndEvent(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()

	if _, _, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "Conversions", URL: "https://site.example/hook", Enabled: true, Events: []string{webhooks.EventGoalConverted},
	}); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	otherSiteID := uuid.New()

	for _, test := range []struct {
		name      string
		siteID    *uuid.UUID
		eventType string
		want      bool
	}{
		{name: "subscribed site", siteID: &site.ID, eventType: webhooks.EventGoalConverted, want: true},
		{name: "other site", siteID: &otherSiteID, eventType: webhooks.EventGoalConverted, want: false},
		{name: "other event", siteID: &site.ID, eventType: webhooks.EventGoalCreated, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := store.HasEnabledWebhookSubscribers(ctx, test.siteID, test.eventType)
			if err != nil {
				t.Fatalf("check subscriptions: %v", err)
			}
			if got != test.want {
				t.Fatalf("got %t want %t", got, test.want)
			}
		})
	}
}

func TestEnqueueWebhookEventsPersistsBatchAtomically(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()

	if _, _, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "Conversions", URL: "https://site.example/hook", Enabled: true, Events: []string{webhooks.EventGoalConverted},
	}); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	inputs := []WebhookEventInput{
		{ID: uuid.New(), SiteID: &site.ID, EventType: webhooks.EventGoalConverted, APIVersion: "2.10", Data: map[string]any{"goal_id": "one"}},
		{ID: uuid.New(), SiteID: &site.ID, EventType: webhooks.EventGoalConverted, APIVersion: "2.10", Data: map[string]any{"goal_id": "two"}},
	}
	jobsByEvent, err := store.EnqueueWebhookEvents(ctx, inputs)
	if err != nil {
		t.Fatalf("enqueue webhook event batch: %v", err)
	}
	if len(jobsByEvent) != 2 || len(jobsByEvent[0]) != 1 || len(jobsByEvent[1]) != 1 {
		t.Fatalf("unexpected jobs: %+v", jobsByEvent)
	}
	var count int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM webhook_events").Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected two durable event rows, got %d", count)
	}
}

func TestEnqueueWebhookEventDeduplicatesStableEventIDs(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()
	if _, _, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "Conversions", URL: "https://site.example/hook", Enabled: true, Events: []string{webhooks.EventGoalConverted},
	}); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	input := WebhookEventInput{
		ID: uuid.New(), SiteID: &site.ID, EventType: webhooks.EventGoalConverted, APIVersion: "2.10",
		Data: map[string]any{"goal_id": "one"}, Deduplicate: true,
	}
	first, err := store.EnqueueWebhookEvent(ctx, input)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	second, err := store.EnqueueWebhookEvent(ctx, input)
	if err != nil {
		t.Fatalf("deduplicated enqueue: %v", err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].DeliveryID != second[0].DeliveryID {
		t.Fatalf("stable event created duplicate deliveries: first=%+v second=%+v", first, second)
	}
	var count int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM webhook_events WHERE id = ?", input.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("expected one event row, count=%d err=%v", count, err)
	}
}

func TestDeleteSiteWithWebhookEventDurablyCoordinatesDeletionAndFinalDeliveries(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()

	instanceWebhook, _, err := store.CreateWebhook(ctx, nil, api.WebhookInput{
		Name: "Instance lifecycle", URL: "https://instance.example/hook", Enabled: true, Events: []string{webhooks.EventSiteDeleted},
	})
	if err != nil {
		t.Fatalf("create instance webhook: %v", err)
	}
	siteWebhook, _, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "Site lifecycle", URL: "https://site.example/hook", Enabled: true, Events: []string{webhooks.EventSiteDeleted},
	})
	if err != nil {
		t.Fatalf("create site webhook: %v", err)
	}

	jobs, err := store.DeleteSiteWithWebhookEvent(ctx, site.ID, WebhookEventInput{
		EventType: webhooks.EventSiteDeleted, APIVersion: "2.10", Data: map[string]any{"site_id": site.ID.String(), "domain": site.Domain},
	})
	if err != nil {
		t.Fatalf("delete site with lifecycle event: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected instance and final site delivery, got %+v", jobs)
	}
	seen := map[uuid.UUID]bool{}
	for _, job := range jobs {
		seen[job.WebhookID] = true
		if delivery, getErr := store.GetWebhookDelivery(ctx, job.DeliveryID); getErr != nil || delivery == nil {
			t.Fatalf("final delivery was not preserved: delivery=%+v err=%v", delivery, getErr)
		}
	}
	if !seen[instanceWebhook.ID] || !seen[siteWebhook.ID] {
		t.Fatalf("missing lifecycle subscriber snapshot: %+v", seen)
	}
	if found, getErr := store.GetSiteByID(ctx, site.ID); getErr != nil || found != nil {
		t.Fatalf("site deletion did not commit: site=%+v err=%v", found, getErr)
	}
}

func TestRecoverStagedSiteDeletionWebhookEventsAfterInterruptedMaterialization(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()

	if _, _, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "Site lifecycle", URL: "https://site.example/hook", Enabled: true, Events: []string{webhooks.EventSiteDeleted},
	}); err != nil {
		t.Fatalf("create site webhook: %v", err)
	}
	now := time.Now().UTC()
	input, eventBody, _, err := prepareWebhookEventInput(WebhookEventInput{
		SiteID: &site.ID, EventType: webhooks.EventSiteDeleted, APIVersion: "2.10", Data: map[string]any{"site_id": site.ID.String()}, PreserveAfterSiteDeletion: true,
	}, now)
	if err != nil {
		t.Fatalf("prepare event: %v", err)
	}
	subscribers, err := store.enabledWebhookSubscribers(ctx, &site.ID, nil, webhooks.EventSiteDeleted)
	if err != nil {
		t.Fatalf("load subscribers: %v", err)
	}
	if err := store.stageSiteDeletionWebhookEvent(ctx, site.ID, input, eventBody, subscribers, now); err != nil {
		t.Fatalf("stage event: %v", err)
	}
	if err := store.DeleteSite(ctx, site.ID); err != nil {
		t.Fatalf("delete site before simulated interruption: %v", err)
	}

	recovered, err := store.RecoverStagedSiteDeletionWebhookEvents(ctx, now.Add(time.Second))
	if err != nil {
		t.Fatalf("recover staged event: %v", err)
	}
	if len(recovered) != 1 || len(recovered[0]) != 1 {
		t.Fatalf("unexpected recovered deliveries: %+v", recovered)
	}
	if delivery, err := store.GetWebhookDelivery(ctx, recovered[0][0].DeliveryID); err != nil || delivery == nil {
		t.Fatalf("recovered delivery is not durable: delivery=%+v err=%v", delivery, err)
	}
}

func TestDeleteSiteWithWebhookEventRollsBackForInvalidEvent(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()

	if _, err := store.DeleteSiteWithWebhookEvent(ctx, site.ID, WebhookEventInput{EventType: webhooks.EventSiteDeleted}); err == nil {
		t.Fatal("expected invalid event to fail deletion")
	}
	if found, err := store.GetSiteByID(ctx, site.ID); err != nil || found == nil {
		t.Fatalf("invalid lifecycle event deleted site: site=%+v err=%v", found, err)
	}
}
