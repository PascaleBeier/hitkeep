package migrationacceptance_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/controlstore"
	"hitkeep/internal/database"
	"hitkeep/internal/takeout"
)

func TestSQLiteControlAndTenantAnalyticsTakeoutMerge(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	control, err := controlstore.Open(ctx, filepath.Join(root, "hitkeep.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	userID, err := control.CreateUserWithNamesAndDefaultTenantName(ctx, "sqlite-takeout@example.invalid", "disabled", "SQLite", "Takeout", "Takeout")
	if err != nil {
		t.Fatal(err)
	}
	site, err := control.CreateSite(ctx, userID, "sqlite-takeout.example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	qr, _, err := control.CreateQRCode(ctx, site.ID, userID, api.QRCodeCreateRequest{
		Name:           "SQLite control QR",
		DestinationURL: "https://example.invalid/qr",
		UTMSource:      "release notes",
		UTMMedium:      "qr",
		UTMCampaign:    "SQLite migration",
		UTMTerm:        "open source analytics",
		UTMContent:     "control plane",
	})
	if err != nil {
		t.Fatal(err)
	}

	manager := database.NewTenantStoreManager(control, filepath.Join(root, "data"), nil)
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := manager.ForTenant(ctx, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if err := manager.SyncAllTenants(ctx); err != nil {
		t.Fatal(err)
	}

	service := takeout.NewTakeoutServiceWithTenantStores(control, manager, filepath.Join(root, "exports"))
	filename, err := service.ExportSiteData(ctx, site.ID, "json")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, expected := range []string{`"record_type":"qr_code"`, qr.ID.String(), "SQLite control QR", "open source analytics"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("merged takeout does not contain %q: %s", expected, text)
		}
	}
}
