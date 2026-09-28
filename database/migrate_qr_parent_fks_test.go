package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

const relaxQRCodeParentFKsMigrationFile = "2026_07_13_010000_relax_qr_code_parent_fks.sql"

func TestRelaxQRCodeParentFKsMigrationPreservesData(t *testing.T) {
	ctx := context.Background()
	store := NewStore(filepath.Join(t.TempDir(), "qr-fk-upgrade.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Hold the new migration back so Migrate reproduces the schema that
	// rejected site-domain renames when a QR code had child rows.
	if _, err := store.DB().ExecContext(ctx,
		"CREATE TABLE IF NOT EXISTS migrations (migration VARCHAR PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL)"); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO migrations (migration, applied_at) VALUES (?, ?)", relaxQRCodeParentFKsMigrationFile, time.Now().UTC()); err != nil {
		t.Fatalf("hold back migration: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate to pre-upgrade state: %v", err)
	}

	userID, err := store.CreateUser(ctx, "qr-fk-upgrade@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "qr-fk-upgrade.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	now := time.Now().UTC()
	qrCodeID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO qr_codes (id, site_id, created_by, name, destination_url, token, token_hash, token_hint, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		qrCodeID, site.ID, userID, "Upgrade", "https://qr-fk-upgrade.test", "upgrade-token", "upgrade-token-hash", "upgrade...", now, now,
	); err != nil {
		t.Fatalf("insert qr code: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO qr_code_assets (qr_code_id, site_id, filename, content_type, byte_size, checksum, storage_key, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		qrCodeID, site.ID, "upgrade.png", "image/png", 42, "upgrade-checksum", "qr/upgrade.png", now, now,
	); err != nil {
		t.Fatalf("insert qr code asset: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO qr_code_share_links (id, site_id, qr_code_id, token_hash, token_hint, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		uuid.New(), site.ID, qrCodeID, "upgrade-share-hash", "upgrade...", userID, now,
	); err != nil {
		t.Fatalf("insert qr code share link: %v", err)
	}

	if _, err := store.DB().ExecContext(ctx,
		"DELETE FROM migrations WHERE migration = ?", relaxQRCodeParentFKsMigrationFile); err != nil {
		t.Fatalf("release migration: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("apply QR foreign-key migration: %v", err)
	}

	for _, table := range []string{"qr_code_assets", "qr_code_share_links"} {
		var rows int
		if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE qr_code_id = ?", qrCodeID).Scan(&rows); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if rows != 1 {
			t.Fatalf("expected one preserved %s row, got %d", table, rows)
		}

		var parentConstraints int
		if err := store.DB().QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM duckdb_constraints()
			WHERE table_name = ? AND constraint_type = 'FOREIGN KEY' AND referenced_table = 'qr_codes'
		`, table).Scan(&parentConstraints); err != nil {
			t.Fatalf("count %s parent constraints: %v", table, err)
		}
		if parentConstraints != 0 {
			t.Fatalf("expected %s QR parent constraint removed, got %d", table, parentConstraints)
		}
	}

	if err := store.UpdateSiteDomain(ctx, site.ID, "qr-fk-upgraded.test"); err != nil {
		t.Fatalf("rename upgraded site: %v", err)
	}
	if err := store.DeleteSite(ctx, site.ID); err != nil {
		t.Fatalf("delete upgraded site: %v", err)
	}
}
