package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestFindSiteByDomainCachesNegativeUntilInvalidated(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	const domain = "domain-cache-negative.test"

	site, err := store.FindSiteByDomain(ctx, domain)
	if err != nil {
		t.Fatalf("find site: %v", err)
	}
	if site != nil {
		t.Fatalf("expected no site, got %v", site)
	}

	userID, err := store.CreateUser(ctx, "domain-cache-negative@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	// Bypass the store API so the cache is not invalidated.
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO sites (id, user_id, domain, created_at) VALUES (?, ?, ?, ?)",
		uuid.New(), userID, domain, time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert site: %v", err)
	}

	site, err = store.FindSiteByDomain(ctx, domain)
	if err != nil {
		t.Fatalf("find site after raw insert: %v", err)
	}
	if site != nil {
		t.Fatalf("expected cached negative result, got %v", site)
	}

	store.invalidateSiteDomain(domain)

	site, err = store.FindSiteByDomain(ctx, domain)
	if err != nil {
		t.Fatalf("find site after invalidation: %v", err)
	}
	if site == nil || site.Domain != domain {
		t.Fatalf("expected site for %s after invalidation, got %v", domain, site)
	}
}

func TestCreateSiteInvalidatesNegativeSiteDomainCache(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	const domain = "domain-cache-create.test"

	if site, err := store.FindSiteByDomain(ctx, domain); err != nil || site != nil {
		t.Fatalf("expected no site before create, got %v (err %v)", site, err)
	}

	userID, err := store.CreateUser(ctx, "domain-cache-create@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := store.CreateSite(ctx, userID, domain); err != nil {
		t.Fatalf("create site: %v", err)
	}

	site, err := store.FindSiteByDomain(ctx, domain)
	if err != nil {
		t.Fatalf("find site after create: %v", err)
	}
	if site == nil || site.Domain != domain {
		t.Fatalf("expected site for %s after create, got %v", domain, site)
	}
}

func TestUpdateSiteDomainInvalidatesSiteDomainCache(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	const oldDomain = "domain-cache-rename-old.test"
	const newDomain = "domain-cache-rename-new.test"

	userID, err := store.CreateUser(ctx, "domain-cache-rename@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, oldDomain)
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	// Prime both a positive entry (old domain) and a negative entry (new domain).
	if got, err := store.FindSiteByDomain(ctx, oldDomain); err != nil || got == nil {
		t.Fatalf("expected site for old domain, got %v (err %v)", got, err)
	}
	if got, err := store.FindSiteByDomain(ctx, newDomain); err != nil || got != nil {
		t.Fatalf("expected no site for new domain, got %v (err %v)", got, err)
	}

	if err := store.UpdateSiteDomain(ctx, site.ID, newDomain); err != nil {
		t.Fatalf("update site domain: %v", err)
	}

	if got, err := store.FindSiteByDomain(ctx, oldDomain); err != nil || got != nil {
		t.Fatalf("expected no site for old domain after rename, got %v (err %v)", got, err)
	}
	got, err := store.FindSiteByDomain(ctx, newDomain)
	if err != nil {
		t.Fatalf("find site by new domain: %v", err)
	}
	if got == nil || got.ID != site.ID {
		t.Fatalf("expected site %s for new domain after rename, got %v", site.ID, got)
	}
}

func TestDeleteSiteInvalidatesSiteDomainCache(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	const domain = "domain-cache-delete.test"

	userID, err := store.CreateUser(ctx, "domain-cache-delete@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, domain)
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	if got, err := store.FindSiteByDomain(ctx, domain); err != nil || got == nil {
		t.Fatalf("expected site before delete, got %v (err %v)", got, err)
	}

	if err := store.DeleteSite(ctx, site.ID); err != nil {
		t.Fatalf("delete site: %v", err)
	}

	if got, err := store.FindSiteByDomain(ctx, domain); err != nil || got != nil {
		t.Fatalf("expected no site after delete, got %v (err %v)", got, err)
	}
}

func TestFindCustomTrackingDomainByHostnameCachedUntilInvalidated(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	const hostname = "tracking-cache.test"

	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	if got, err := store.FindCustomTrackingDomainByHostname(ctx, hostname); err != nil || got != nil {
		t.Fatalf("expected no tracking domain, got %v (err %v)", got, err)
	}

	// Bypass the store API so the cache is not invalidated.
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO custom_tracking_domains (
			id, tenant_id, hostname, verification_token,
			verification_status, target_status, tls_mode, tls_status,
			enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'pending', 'pending', 'external', 'pending', TRUE, ?, ?)
	`, uuid.New(), tenantID, hostname, "token", time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("insert tracking domain: %v", err)
	}

	if got, err := store.FindCustomTrackingDomainByHostname(ctx, hostname); err != nil || got != nil {
		t.Fatalf("expected cached negative tracking domain, got %v (err %v)", got, err)
	}

	store.invalidateCustomTrackingDomainHost(hostname)

	got, err := store.FindCustomTrackingDomainByHostname(ctx, hostname)
	if err != nil {
		t.Fatalf("find tracking domain after invalidation: %v", err)
	}
	if got == nil || got.Hostname != hostname {
		t.Fatalf("expected tracking domain %s after invalidation, got %v", hostname, got)
	}
}

func TestCustomTrackingDomainMutationsInvalidateHostCache(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	const hostname = "tracking-cache-mutations.test"

	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}

	// Prime a negative entry, then create: create must invalidate it.
	if got, err := store.FindCustomTrackingDomainByHostname(ctx, hostname); err != nil || got != nil {
		t.Fatalf("expected no tracking domain, got %v (err %v)", got, err)
	}
	created, err := store.CreateCustomTrackingDomain(ctx, CustomTrackingDomainInput{TeamID: tenantID, Host: hostname})
	if err != nil {
		t.Fatalf("create tracking domain: %v", err)
	}
	got, err := store.FindCustomTrackingDomainByHostname(ctx, hostname)
	if err != nil || got == nil {
		t.Fatalf("expected tracking domain after create, got %v (err %v)", got, err)
	}
	if !got.Enabled {
		t.Fatal("expected tracking domain enabled after create")
	}

	// Disable: the cached copy must not survive.
	if _, err := store.UpdateCustomTrackingDomainEnabled(ctx, tenantID, created.ID, false); err != nil {
		t.Fatalf("disable tracking domain: %v", err)
	}
	got, err = store.FindCustomTrackingDomainByHostname(ctx, hostname)
	if err != nil || got == nil {
		t.Fatalf("expected tracking domain after disable, got %v (err %v)", got, err)
	}
	if got.Enabled {
		t.Fatal("expected tracking domain disabled after update")
	}

	// Verification update: status change must be visible immediately.
	now := time.Now().UTC()
	if _, err := store.UpdateCustomTrackingDomainVerification(ctx, created.ID, CustomTrackingDomainVerificationResult{
		VerificationStatus: CustomTrackingVerificationVerified,
		TargetStatus:       CustomTrackingVerificationVerified,
		TLSStatus:          CustomTrackingVerificationVerified,
		VerifiedAt:         &now,
		LastCheckedAt:      now,
	}); err != nil {
		t.Fatalf("update verification: %v", err)
	}
	got, err = store.FindCustomTrackingDomainByHostname(ctx, hostname)
	if err != nil || got == nil {
		t.Fatalf("expected tracking domain after verification, got %v (err %v)", got, err)
	}
	if got.VerificationStatus != "verified" {
		t.Fatalf("expected verified status, got %q", got.VerificationStatus)
	}

	// Delete: the cached copy must not survive.
	if err := store.DeleteCustomTrackingDomain(ctx, tenantID, created.ID); err != nil {
		t.Fatalf("delete tracking domain: %v", err)
	}
	if got, err := store.FindCustomTrackingDomainByHostname(ctx, hostname); err != nil || got != nil {
		t.Fatalf("expected no tracking domain after delete, got %v (err %v)", got, err)
	}
}
