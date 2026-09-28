package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestShareLinksCreateListRevoke(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestFixtureStore(t)

	userID := uuid.New()
	siteID := uuid.New()
	now := time.Now().UTC()

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB().ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}

	exec("INSERT INTO users (id, email, password, created_at) VALUES (?, ?, ?, ?)", userID, "owner@example.com", "hash", now)
	exec("INSERT INTO sites (id, user_id, domain, created_at) VALUES (?, ?, ?, ?)", siteID, userID, "example.com", now)
	exec("INSERT INTO site_members (id, site_id, user_id, role, added_at, added_by) VALUES (?, ?, ?, ?, ?, ?)", uuid.New(), siteID, userID, "owner", now, userID)

	link, token, err := store.CreateShareLink(ctx, siteID, userID)
	if err != nil {
		t.Fatalf("create share link: %v", err)
	}
	if link == nil {
		t.Fatalf("expected share link, got nil")
	}
	if token == "" {
		t.Fatalf("expected share token, got empty value")
	}

	links, err := store.ListShareLinks(ctx, siteID)
	if err != nil {
		t.Fatalf("list share links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 share link, got %d", len(links))
	}
	if links[0].ID != link.ID {
		t.Fatalf("expected link ID %s, got %s", link.ID, links[0].ID)
	}
	if len(links[0].TokenHint) != 8 {
		t.Fatalf("expected token hint length 8, got %d", len(links[0].TokenHint))
	}

	sharedSite, err := store.GetShareSiteByToken(ctx, token)
	if err != nil {
		t.Fatalf("lookup share token: %v", err)
	}
	if sharedSite == nil {
		t.Fatalf("expected shared site, got nil")
	}
	if sharedSite.ID != siteID {
		t.Fatalf("expected site ID %s, got %s", siteID, sharedSite.ID)
	}

	revoked, err := store.RevokeShareLink(ctx, siteID, link.ID)
	if err != nil {
		t.Fatalf("revoke share link: %v", err)
	}
	if !revoked {
		t.Fatalf("expected share link to be revoked")
	}

	revokedAgain, err := store.RevokeShareLink(ctx, siteID, link.ID)
	if err != nil {
		t.Fatalf("revoke share link second attempt: %v", err)
	}
	if revokedAgain {
		t.Fatalf("expected second revoke to return false")
	}

	sharedSiteAfterRevoke, err := store.GetShareSiteByToken(ctx, token)
	if err != nil {
		t.Fatalf("lookup revoked share token: %v", err)
	}
	if sharedSiteAfterRevoke != nil {
		t.Fatalf("expected revoked token to resolve to nil site")
	}
}
