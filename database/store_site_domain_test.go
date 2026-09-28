package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUpdateSiteDomainKeepsForeignKeyReferences(t *testing.T) {
	ctx := context.Background()

	store := newSharedTestFixtureStore(t)

	userID, err := store.CreateUser(ctx, "domain-update@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "domain-old.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	goalID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO goals (id, site_id, name, type, value, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		goalID, site.ID, "Signups", "event", "signup", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert goal: %v", err)
	}

	now := time.Now().UTC()
	qrCodeID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO qr_codes (id, site_id, created_by, name, destination_url, token, token_hash, token_hint, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		qrCodeID, site.ID, userID, "Launch", "https://domain-old.test/launch", "domain-update-token", "domain-update-token-hash", "domain...", now, now,
	); err != nil {
		t.Fatalf("insert qr code: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO qr_code_assets (qr_code_id, site_id, filename, content_type, byte_size, checksum, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		qrCodeID, site.ID, "launch.png", "image/png", 42, "domain-update-checksum", now, now,
	); err != nil {
		t.Fatalf("insert qr code asset: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO qr_code_share_links (id, site_id, qr_code_id, token_hash, token_hint, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		uuid.New(), site.ID, qrCodeID, "domain-update-share-hash", "domain...", userID, now,
	); err != nil {
		t.Fatalf("insert qr code share link: %v", err)
	}

	if err := store.UpdateSiteDomain(ctx, site.ID, "domain-new.test"); err != nil {
		t.Fatalf("update site domain: %v", err)
	}

	updated, err := store.GetSiteByID(ctx, site.ID)
	if err != nil || updated == nil {
		t.Fatalf("get updated site: %v", err)
	}
	if updated.Domain != "domain-new.test" {
		t.Fatalf("expected domain domain-new.test, got %s", updated.Domain)
	}

	// The shadow site used to park foreign keys must be gone again.
	var siteCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM sites").Scan(&siteCount); err != nil {
		t.Fatalf("count sites: %v", err)
	}
	if siteCount != 1 {
		t.Fatalf("expected exactly one site row after rename, got %d", siteCount)
	}

	// Every foreign key reference must still point at the renamed site.
	refs, err := listSiteFKReferences(ctx, store.DB())
	if err != nil {
		t.Fatalf("list site fk references: %v", err)
	}
	if len(refs) == 0 {
		t.Fatal("expected foreign key references on sites, got none")
	}
	expected := map[string]int{
		"goals":               1,
		"qr_code_assets":      1,
		"qr_code_share_links": 1,
		"qr_codes":            1,
		"site_tenants":        1,
		"site_members":        1,
	}
	for _, ref := range refs {
		want, ok := expected[ref.table]
		if !ok {
			continue
		}
		var count int
		if err := store.DB().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM "+ref.table+" WHERE "+ref.column+" = ?", site.ID,
		).Scan(&count); err != nil {
			t.Fatalf("count %s references: %v", ref.table, err)
		}
		if count != want {
			t.Fatalf("expected %d %s reference(s) for renamed site, got %d", want, ref.table, count)
		}
		delete(expected, ref.table)
	}
	if len(expected) > 0 {
		t.Fatalf("expected fk discovery to include tables %v", expected)
	}
}
