package database

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func setupExclusionStore(t *testing.T) (*Store, uuid.UUID, uuid.UUID) {
	t.Helper()

	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	userID, err := store.CreateUser(context.Background(), "owner@example.com", "hashed-secret")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	site, err := store.CreateSite(context.Background(), userID, "exclusion-test.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	return store, userID, site.ID
}

func TestExclusionCRUD(t *testing.T) {
	store, userID, siteID := setupExclusionStore(t)
	defer store.Close()

	ctx := context.Background()

	instanceRule, err := store.CreateInstanceExclusion(ctx, "203.0.113.5/32", "monitor", userID)
	if err != nil {
		t.Fatalf("create instance exclusion: %v", err)
	}
	if instanceRule.ID == uuid.Nil {
		t.Fatalf("expected instance exclusion id")
	}

	siteRule, err := store.CreateSiteExclusion(ctx, siteID, "10.0.0.0/8", "internal", userID)
	if err != nil {
		t.Fatalf("create site exclusion: %v", err)
	}
	if siteRule.ID == uuid.Nil {
		t.Fatalf("expected site exclusion id")
	}
	if siteRule.SiteID == nil || *siteRule.SiteID != siteID {
		t.Fatalf("expected site id %s on site exclusion", siteID)
	}

	instanceRules, err := store.ListInstanceExclusions(ctx)
	if err != nil {
		t.Fatalf("list instance exclusions: %v", err)
	}
	if len(instanceRules) != 1 {
		t.Fatalf("expected 1 instance exclusion, got %d", len(instanceRules))
	}

	siteRules, err := store.ListSiteExclusions(ctx, siteID)
	if err != nil {
		t.Fatalf("list site exclusions: %v", err)
	}
	if len(siteRules) != 1 {
		t.Fatalf("expected 1 site exclusion, got %d", len(siteRules))
	}

	instanceCIDRs, err := store.ListInstanceExclusionCIDRs(ctx)
	if err != nil {
		t.Fatalf("list instance exclusion cidrs: %v", err)
	}
	if len(instanceCIDRs) != 1 || instanceCIDRs[0] != "203.0.113.5/32" {
		t.Fatalf("unexpected instance cidrs: %#v", instanceCIDRs)
	}

	siteCIDRs, err := store.ListSiteExclusionCIDRs(ctx)
	if err != nil {
		t.Fatalf("list site exclusion cidrs: %v", err)
	}
	if len(siteCIDRs) != 1 {
		t.Fatalf("expected 1 site cidr rule, got %d", len(siteCIDRs))
	}
	if siteCIDRs[0].SiteID != siteID || siteCIDRs[0].CIDR != "10.0.0.0/8" {
		t.Fatalf("unexpected site cidr rule: %#v", siteCIDRs[0])
	}

	deleted, err := store.DeleteSiteExclusion(ctx, siteID, siteRule.ID)
	if err != nil {
		t.Fatalf("delete site exclusion: %v", err)
	}
	if !deleted {
		t.Fatalf("expected site exclusion to be deleted")
	}

	deleted, err = store.DeleteInstanceExclusion(ctx, instanceRule.ID)
	if err != nil {
		t.Fatalf("delete instance exclusion: %v", err)
	}
	if !deleted {
		t.Fatalf("expected instance exclusion to be deleted")
	}

	siteRules, err = store.ListSiteExclusions(ctx, siteID)
	if err != nil {
		t.Fatalf("list site exclusions after delete: %v", err)
	}
	if len(siteRules) != 0 {
		t.Fatalf("expected no site exclusions after delete, got %d", len(siteRules))
	}

	instanceRules, err = store.ListInstanceExclusions(ctx)
	if err != nil {
		t.Fatalf("list instance exclusions after delete: %v", err)
	}
	if len(instanceRules) != 0 {
		t.Fatalf("expected no instance exclusions after delete, got %d", len(instanceRules))
	}
}

func TestInstanceCountryExclusionsAppearInMergedLists(t *testing.T) {
	store, userID, siteID := setupExclusionStore(t)
	defer store.Close()

	ctx := context.Background()

	createCountryExclusionFixture(t, store, userID, siteID)

	instanceRules, err := store.ListInstanceExclusions(ctx)
	if err != nil {
		t.Fatalf("list instance exclusions: %v", err)
	}
	if len(instanceRules) != 2 {
		t.Fatalf("expected 2 instance exclusions, got %d", len(instanceRules))
	}
	if instanceRules[0].Type != "country" || instanceRules[0].CountryCode != "DE" {
		t.Fatalf("expected country rule first, got %#v", instanceRules[0])
	}
	if instanceRules[1].Type != "cidr" || instanceRules[1].CIDR != "203.0.113.5/32" {
		t.Fatalf("expected cidr rule second, got %#v", instanceRules[1])
	}
}

func TestSiteCountryExclusionsAppearInMergedLists(t *testing.T) {
	store, userID, siteID := setupExclusionStore(t)
	defer store.Close()

	ctx := context.Background()

	createCountryExclusionFixture(t, store, userID, siteID)

	siteRules, err := store.ListSiteExclusions(ctx, siteID)
	if err != nil {
		t.Fatalf("list site exclusions: %v", err)
	}
	if len(siteRules) != 1 || siteRules[0].Type != "country" || siteRules[0].CountryCode != "US" {
		t.Fatalf("unexpected site country rules: %#v", siteRules)
	}
}

func TestCountryExclusionListsNormalizeCodes(t *testing.T) {
	store, userID, siteID := setupExclusionStore(t)
	defer store.Close()

	ctx := context.Background()

	createCountryExclusionFixture(t, store, userID, siteID)

	instanceCountries, err := store.ListInstanceExclusionCountries(ctx)
	if err != nil {
		t.Fatalf("list instance countries: %v", err)
	}
	if len(instanceCountries) != 1 || instanceCountries[0] != "DE" {
		t.Fatalf("unexpected instance countries: %#v", instanceCountries)
	}

	siteCountries, err := store.ListSiteExclusionCountries(ctx)
	if err != nil {
		t.Fatalf("list site countries: %v", err)
	}
	if len(siteCountries) != 1 || siteCountries[0].SiteID != siteID || siteCountries[0].CountryCode != "US" {
		t.Fatalf("unexpected site countries: %#v", siteCountries)
	}
}

func TestCountryExclusionsDeleteThroughSharedDeletionMethods(t *testing.T) {
	store, userID, siteID := setupExclusionStore(t)
	defer store.Close()

	ctx := context.Background()

	instanceCIDR, instanceCountry, siteCountry := createCountryExclusionFixture(t, store, userID, siteID)

	deleted, err := store.DeleteSiteExclusion(ctx, siteID, siteCountry.ID)
	if err != nil {
		t.Fatalf("delete site country exclusion: %v", err)
	}
	if !deleted {
		t.Fatal("expected site country exclusion to be deleted")
	}

	deleted, err = store.DeleteInstanceExclusion(ctx, instanceCountry.ID)
	if err != nil {
		t.Fatalf("delete instance country exclusion: %v", err)
	}
	if !deleted {
		t.Fatal("expected instance country exclusion to be deleted")
	}

	deleted, err = store.DeleteInstanceExclusion(ctx, instanceCIDR.ID)
	if err != nil {
		t.Fatalf("delete instance cidr exclusion: %v", err)
	}
	if !deleted {
		t.Fatal("expected instance cidr exclusion to be deleted")
	}
}

func createCountryExclusionFixture(t *testing.T, store *Store, userID uuid.UUID, siteID uuid.UUID) (*api.IPExclusion, *api.IPExclusion, *api.IPExclusion) {
	t.Helper()

	ctx := context.Background()
	instanceCIDR, err := store.CreateInstanceExclusion(ctx, "203.0.113.5/32", "monitor", userID)
	if err != nil {
		t.Fatalf("create instance cidr exclusion: %v", err)
	}
	instanceCountry, err := store.CreateInstanceCountryExclusion(ctx, "de", "Germany", userID)
	if err != nil {
		t.Fatalf("create instance country exclusion: %v", err)
	}
	siteCountry, err := store.CreateSiteCountryExclusion(ctx, siteID, "us", "United States", userID)
	if err != nil {
		t.Fatalf("create site country exclusion: %v", err)
	}
	return instanceCIDR, instanceCountry, siteCountry
}
