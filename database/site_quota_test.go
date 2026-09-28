package database

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestCreateSiteWithQuotaSerializesConcurrentCreates(t *testing.T) {
	ctx := context.Background()
	store := setupTenantStore(t)
	userID, err := store.CreateUser(ctx, "site-quota-create@test.dev", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	teamID, err := store.GetActiveTenantID(ctx, userID)
	if err != nil {
		t.Fatalf("resolve active team: %v", err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, domain := range []string{"site-quota-create-one.test", "site-quota-create-two.test"} {
		wg.Add(1)
		go func(domain string) {
			defer wg.Done()
			<-start
			_, err := store.CreateSiteWithQuota(ctx, userID, teamID, domain, 1)
			errs <- err
		}(domain)
	}
	close(start)
	wg.Wait()
	close(errs)

	var created, limited int
	for err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrSiteLimitReached):
			limited++
		default:
			t.Fatalf("create site: %v", err)
		}
	}
	if created != 1 || limited != 1 {
		t.Fatalf("expected one create and one quota denial, got created=%d limited=%d", created, limited)
	}
}

func TestSiteQuotaSerializesCreateAndTransfer(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	manager := NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = manager.Close() })

	userID, err := store.CreateUser(ctx, "site-quota-transfer@test.dev", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "site-quota-source.test")
	if err != nil {
		t.Fatalf("create source site: %v", err)
	}
	sourceTeamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("source team: %v", err)
	}
	destination, err := store.CreateTenant(ctx, userID, "quota destination", "")
	if err != nil {
		t.Fatalf("create destination team: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, destination.ID); err != nil {
		t.Fatalf("activate destination team: %v", err)
	}

	audits := []AuditEntryParams{
		{ActorID: userID, TeamID: sourceTeamID, Action: "site.transferred_out", TargetType: "site", TargetID: site.ID.String(), TargetLabel: site.Domain, Outcome: "success"},
		{ActorID: userID, TeamID: destination.ID, Action: "site.transferred_in", TargetType: "site", TargetID: site.ID.String(), TargetLabel: site.Domain, Outcome: "success"},
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		_, err := store.CreateSiteWithQuota(ctx, userID, destination.ID, "site-quota-created.test", 1)
		errs <- err
	}()
	go func() {
		<-start
		errs <- manager.TransferSiteWithQuota(ctx, site.ID, destination.ID, 1, audits...)
	}()
	close(start)

	var succeeded, limited int
	for range 2 {
		err := <-errs
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrSiteLimitReached):
			limited++
		default:
			t.Fatalf("quota operation: %v", err)
		}
	}
	if succeeded != 1 || limited != 1 {
		t.Fatalf("expected one success and one quota denial, got succeeded=%d limited=%d", succeeded, limited)
	}
	count, err := store.CountTeamSites(ctx, destination.ID)
	if err != nil {
		t.Fatalf("count destination sites: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected destination count 1, got %d", count)
	}
}

func TestCreateSiteWithQuotaRejectsChangedActiveTeam(t *testing.T) {
	ctx := context.Background()
	store := setupTenantStore(t)
	userID, err := store.CreateUser(ctx, "site-quota-drift@test.dev", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	target, err := store.CreateTenant(ctx, userID, "quota target", "")
	if err != nil {
		t.Fatalf("create target team: %v", err)
	}

	_, err = store.CreateSiteWithQuota(ctx, userID, target.ID, "site-quota-drift.test", 1)
	if !errors.Is(err, ErrActiveTenantChanged) {
		t.Fatalf("expected active tenant change error, got %v", err)
	}
	count, err := store.CountTeamSites(ctx, target.ID)
	if err != nil {
		t.Fatalf("count target sites: %v", err)
	}
	if count != 0 {
		t.Fatalf("changed active team created %d target sites", count)
	}
}

func TestTransferSiteWithQuotaRejectsBeforeMovingData(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	manager := NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = manager.Close() })

	userID, err := store.CreateUser(ctx, "site-quota-reject@test.dev", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "site-quota-rejected-source.test")
	if err != nil {
		t.Fatalf("create source site: %v", err)
	}
	sourceTeamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("source team: %v", err)
	}
	destination, err := store.CreateTenant(ctx, userID, "full destination", "")
	if err != nil {
		t.Fatalf("create destination team: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, destination.ID); err != nil {
		t.Fatalf("activate destination team: %v", err)
	}
	if _, err := store.CreateSite(ctx, userID, "site-quota-existing-destination.test"); err != nil {
		t.Fatalf("create destination site: %v", err)
	}

	err = manager.TransferSiteWithQuota(ctx, site.ID, destination.ID, 1)
	if !errors.Is(err, ErrSiteLimitReached) {
		t.Fatalf("expected site limit error, got %v", err)
	}
	teamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("resolve source site team after denial: %v", err)
	}
	if teamID != sourceTeamID {
		t.Fatalf("denied transfer moved site to %s, expected %s", teamID, sourceTeamID)
	}
}
