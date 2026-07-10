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
