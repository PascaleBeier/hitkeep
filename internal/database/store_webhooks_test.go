package database

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/webhooks"
)

func TestWebhookConfigurationLifecycle(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()

	created, secret, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name:        "CRM sync",
		Description: "Notify the customer workflow",
		URL:         "https://hooks.example.com/hitkeep",
		Enabled:     true,
		Events:      []string{webhooks.EventGoalCreated, webhooks.EventImportCompleted},
	})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	if created == nil || created.ID == uuid.Nil || created.SiteID == nil || *created.SiteID != site.ID {
		t.Fatalf("unexpected created webhook: %+v", created)
	}
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("expected one-time whsec secret, got %q", secret)
	}
	if len(created.Events) != 2 || created.Events[0] != webhooks.EventGoalCreated || created.Events[1] != webhooks.EventImportCompleted {
		t.Fatalf("unexpected event selection: %+v", created.Events)
	}

	listed, err := store.ListWebhooks(ctx, &site.ID)
	if err != nil {
		t.Fatalf("list webhooks: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("unexpected webhook list: %+v", listed)
	}

	updated, err := store.UpdateWebhook(ctx, created.ID, &site.ID, api.WebhookInput{
		Name:        "CRM operations",
		Description: "Updated description",
		URL:         "https://hooks.example.com/operations",
		Enabled:     false,
		Events:      []string{webhooks.EventGoalDeleted},
	})
	if err != nil {
		t.Fatalf("update webhook: %v", err)
	}
	if updated == nil || updated.Name != "CRM operations" || updated.Enabled || len(updated.Events) != 1 {
		t.Fatalf("unexpected updated webhook: %+v", updated)
	}

	rotated, nextSecret, err := store.RotateWebhookSecret(ctx, created.ID, &site.ID)
	if err != nil {
		t.Fatalf("rotate webhook secret: %v", err)
	}
	if rotated == nil || nextSecret == secret || !strings.HasPrefix(nextSecret, "whsec_") {
		t.Fatalf("unexpected rotation result: webhook=%+v secret=%q", rotated, nextSecret)
	}
	storedSecret, err := store.getWebhookSecret(ctx, created.ID)
	if err != nil {
		t.Fatalf("get stored secret: %v", err)
	}
	if storedSecret != nextSecret {
		t.Fatal("rotation must immediately replace the previous secret")
	}

	if _, err := store.UpdateWebhook(ctx, created.ID, nil, api.WebhookInput{
		Name: "wrong scope", URL: "https://hooks.example.com", Enabled: true, Events: []string{webhooks.EventSiteCreated},
	}); err != ErrWebhookNotFound {
		t.Fatalf("expected scope-safe not found error, got %v", err)
	}

	if err := store.DeleteWebhook(ctx, created.ID, &site.ID); err != nil {
		t.Fatalf("delete webhook: %v", err)
	}
	listed, err = store.ListWebhooks(ctx, &site.ID)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected webhook deletion, got %+v", listed)
	}
}

func TestWebhookConfigurationValidatesScopeAndCleansUpWithSite(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()

	if _, _, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "invalid", URL: "https://hooks.example.com", Enabled: true, Events: []string{webhooks.EventSiteCreated},
	}); err == nil {
		t.Fatal("expected site webhook to reject instance-only event")
	}

	created, _, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "cleanup", URL: "https://hooks.example.com", Enabled: true, Events: []string{webhooks.EventSiteDeleted},
	})
	if err != nil {
		t.Fatalf("create cleanup webhook: %v", err)
	}
	if err := store.DeleteSite(ctx, site.ID); err != nil {
		t.Fatalf("delete site: %v", err)
	}
	if found, err := store.GetWebhook(ctx, created.ID, &site.ID); err != nil || found != nil {
		t.Fatalf("site deletion must remove webhook configuration, found=%+v err=%v", found, err)
	}
}

func TestRotateWebhookSecretUpdatesOutstandingDeliveries(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()

	created, previousSecret, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "rotation", URL: "https://hooks.example.com", Enabled: true, Events: []string{webhooks.EventGoalCreated},
	})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	jobs, err := store.EnqueueWebhookEvent(ctx, WebhookEventInput{
		SiteID: &site.ID, EventType: webhooks.EventGoalCreated, APIVersion: "2.10", Data: map[string]any{"site_id": site.ID.String()},
	})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("enqueue delivery: jobs=%+v err=%v", jobs, err)
	}

	_, nextSecret, err := store.RotateWebhookSecret(ctx, created.ID, &site.ID)
	if err != nil {
		t.Fatalf("rotate webhook secret: %v", err)
	}
	delivery, err := store.GetWebhookDelivery(ctx, jobs[0].DeliveryID)
	if err != nil {
		t.Fatalf("get outstanding delivery: %v", err)
	}
	if delivery.SigningSecret != nextSecret || delivery.SigningSecret == previousSecret {
		t.Fatalf("outstanding delivery retained revoked secret: got %q want %q", delivery.SigningSecret, nextSecret)
	}
}

func TestCreateWebhookWithAuditRollsBackWhenAuditFails(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()

	_, _, err := store.CreateWebhookWithAudit(ctx, &site.ID, api.WebhookInput{
		Name: "must roll back", URL: "https://hooks.example.com", Enabled: true, Events: []string{webhooks.EventGoalCreated},
	}, AuditEntryParams{})
	if err == nil {
		t.Fatal("expected invalid audit entry to fail the mutation")
	}
	configured, listErr := store.ListWebhooks(ctx, &site.ID)
	if listErr != nil {
		t.Fatalf("list webhooks after rollback: %v", listErr)
	}
	if len(configured) != 0 {
		t.Fatalf("audit failure committed webhook configuration: %+v", configured)
	}
}

func TestRotateWebhookSecretWithAuditRollsBackWhenAuditFails(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()

	created, previousSecret, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "must keep secret", URL: "https://hooks.example.com", Enabled: true, Events: []string{webhooks.EventGoalCreated},
	})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	if _, _, err := store.RotateWebhookSecretWithAudit(ctx, created.ID, &site.ID, AuditEntryParams{}); err == nil {
		t.Fatal("expected invalid audit entry to fail rotation")
	}
	storedSecret, err := store.getWebhookSecret(ctx, created.ID)
	if err != nil {
		t.Fatalf("get secret after rollback: %v", err)
	}
	if storedSecret != previousSecret {
		t.Fatalf("audit failure rotated secret: got %q want %q", storedSecret, previousSecret)
	}
}

func TestUpdateAndDeleteWebhookWithAuditRollBackWhenAuditFails(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()

	created, _, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "original", URL: "https://hooks.example.com", Enabled: true, Events: []string{webhooks.EventGoalCreated},
	})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	if _, err := store.UpdateWebhookWithAudit(ctx, created.ID, &site.ID, api.WebhookInput{
		Name: "changed", URL: "https://hooks.example.com", Enabled: true, Events: []string{webhooks.EventGoalDeleted},
	}, AuditEntryParams{}); err == nil {
		t.Fatal("expected invalid audit entry to fail update")
	}
	afterUpdate, err := store.GetWebhook(ctx, created.ID, &site.ID)
	if err != nil || afterUpdate == nil || afterUpdate.Name != "original" || afterUpdate.Events[0] != webhooks.EventGoalCreated {
		t.Fatalf("audit failure committed update: webhook=%+v err=%v", afterUpdate, err)
	}

	if err := store.DeleteWebhookWithAudit(ctx, created.ID, &site.ID, AuditEntryParams{}); err == nil {
		t.Fatal("expected invalid audit entry to fail delete")
	}
	afterDelete, err := store.GetWebhook(ctx, created.ID, &site.ID)
	if err != nil || afterDelete == nil {
		t.Fatalf("audit failure committed delete: webhook=%+v err=%v", afterDelete, err)
	}
}
