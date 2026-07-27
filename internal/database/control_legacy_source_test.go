package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"hitkeep/internal/api"
	"hitkeep/internal/controlstore"
	"hitkeep/internal/webhooks"
)

func TestCurrentDuckDBControlSchemaMatchesClosedSQLiteImportRegistry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "hitkeep.db")
	store := NewStore(sourcePath, WithCheckpointInterval(0))
	if err := store.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	userID, err := store.CreateUserWithNames(ctx, "sqlite-import+例@example.test", "hash", "Ada", "Lovelace")
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "sqlite-import.example.test")
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	qr, _, err := store.CreateQRCode(ctx, site.ID, userID, api.QRCodeCreateRequest{
		Name: "Fixture 例", DestinationURL: "https://example.test/qr",
		CustomParams: map[string]string{"utm": "夏"}, Style: map[string]any{"dots": "rounded"},
	})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if _, err := store.UpsertQRCodeAsset(ctx, api.QRCodeAsset{
		QRCodeID: qr.ID, SiteID: site.ID, Filename: "fixture.png", ContentType: "image/png",
		ByteSize: 4, Checksum: "fixture-checksum", Data: []byte{0x89, 'P', 'N', 'G'},
	}); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if _, _, err := store.CreateWebhook(ctx, &site.ID, api.WebhookInput{
		Name: "fixture", URL: "https://hooks.example.test/hitkeep", Enabled: true,
		Events: []string{webhooks.EventGoalCreated},
	}); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO data_migrations(name, applied_at) VALUES
			('default_tenant_split_v1', ?),
			('default_tenant_split_compacted_v1', ?)
	`, time.Now().UTC(), time.Now().UTC()); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	source, sourceSHA, schemaSHA, err := OpenLegacyControlSource(ctx, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	tables, err := source.BaseTables(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlstore.ClassifyLegacyTables(tables); err != nil {
		t.Fatal(err)
	}
	workPath := filepath.Join(dir, "hitkeep.db.sqlite-work")
	if _, err := controlstore.ImportLegacy(ctx, workPath, source, sourceSHA, schemaSHA); err != nil {
		t.Fatal(err)
	}
}
