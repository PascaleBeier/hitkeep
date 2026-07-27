package controlstore

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/webhooks"
)

func TestWriteTakeoutNDJSONExportsControlRecordsWithoutSecrets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	userID, err := store.CreateUserWithNamesAndDefaultTenantName(ctx, "takeout@example.invalid", "password-hash-must-not-export", "Take", "Out", "Takeout")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "takeout.example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	qr, _, err := store.CreateQRCode(ctx, site.ID, userID, api.QRCodeCreateRequest{Name: "QR takeout", DestinationURL: "https://example.invalid/destination"})
	if err != nil {
		t.Fatal(err)
	}
	_, webhookSecret, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{Name: "Takeout webhook", URL: "https://hooks.example.invalid/hitkeep", Enabled: true, Events: []string{webhooks.EventGoalCreated}})
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	count, err := store.WriteTakeoutNDJSON(ctx, &output, TakeoutScope{SiteIDs: []uuid.UUID{site.ID}, UserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	if count < 2 {
		t.Fatalf("expected at least QR and webhook records, got %d", count)
	}
	text := output.String()
	for _, expected := range []string{`"record_type":"qr_code"`, `"record_type":"webhook"`, qr.ID.String()} {
		if !strings.Contains(text, expected) {
			t.Fatalf("takeout does not contain %q: %s", expected, text)
		}
	}
	for _, forbidden := range []string{"password-hash-must-not-export", webhookSecret, "secret_hash"} {
		if forbidden != "" && strings.Contains(text, forbidden) {
			t.Fatalf("takeout leaked forbidden value %q", forbidden)
		}
	}
}
