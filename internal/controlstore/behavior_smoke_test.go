package controlstore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/webhooks"
)

func TestSQLiteControlBehaviorSmoke(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "hitkeep.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	userID, err := store.CreateUserWithNamesAndDefaultTenantName(ctx, "sqlite@example.test", "hash", "Ada", "Lovelace", "SQLite Team")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "sqlite.example.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if siteTenant, err := store.GetSiteTenantID(ctx, site.ID); err != nil || siteTenant == uuid.Nil {
		t.Fatalf("site tenant=%s err=%v", siteTenant, err)
	}

	qr, token, err := store.CreateQRCode(ctx, site.ID, userID, api.QRCodeCreateRequest{
		Name:           "Unicode 例",
		DestinationURL: "https://example.test/qr",
		CustomParams:   map[string]string{"campaign": "夏"},
		Style:          map[string]any{"dots": "rounded"},
	})
	if err != nil {
		t.Fatalf("create QR: %v", err)
	}
	if found, err := store.GetQRCodeByToken(ctx, token); err != nil || found == nil || found.ID != qr.ID {
		t.Fatalf("QR lookup=%+v err=%v", found, err)
	}

	hook, secret, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "smoke", URL: "https://hooks.example.test/hitkeep", Enabled: true,
		Events: []string{webhooks.EventGoalCreated},
	})
	if err != nil || hook == nil || secret == "" {
		t.Fatalf("create webhook=%+v secret=%q err=%v", hook, secret, err)
	}

	if err := store.Optimize(ctx); err != nil {
		t.Fatalf("optimize: %v", err)
	}
}
