package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

const unifyTrafficExclusionsMigrationFile = "2026_07_18_000000_unify_traffic_exclusions.sql"

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

func TestTrafficExclusionMigrationPreservesLegacyRules(t *testing.T) {
	ctx := context.Background()
	store := NewStore(filepath.Join(t.TempDir(), "traffic-exclusions-upgrade.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.DB().ExecContext(ctx, "CREATE TABLE IF NOT EXISTS migrations (migration VARCHAR PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL)"); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, "INSERT INTO migrations (migration, applied_at) VALUES (?, ?)", unifyTrafficExclusionsMigrationFile, time.Now().UTC()); err != nil {
		t.Fatalf("hold back migration: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate to legacy schema: %v", err)
	}

	userID, err := store.CreateUser(ctx, "legacy-exclusions@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "legacy-exclusions.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	legacyIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	legacyInserts := []struct {
		query string
		args  []any
	}{
		{"INSERT INTO instance_exclusions (id, cidr, description, created_at, created_by) VALUES (?, ?, ?, ?, ?)", []any{legacyIDs[0], "203.0.113.4/32", "instance cidr", now, userID}},
		{"INSERT INTO site_exclusions (id, site_id, cidr, description, created_at, created_by) VALUES (?, ?, ?, ?, ?, ?)", []any{legacyIDs[1], site.ID, "10.0.0.0/8", "site cidr", now, userID}},
		{"INSERT INTO instance_country_exclusions (id, country_code, description, created_at, created_by) VALUES (?, ?, ?, ?, ?)", []any{legacyIDs[2], "DE", "instance country", now, userID}},
		{"INSERT INTO site_country_exclusions (id, site_id, country_code, description, created_at, created_by) VALUES (?, ?, ?, ?, ?, ?)", []any{legacyIDs[3], site.ID, "US", "site country", now, userID}},
	}
	for _, insert := range legacyInserts {
		if _, err := store.DB().ExecContext(ctx, insert.query, insert.args...); err != nil {
			t.Fatalf("insert legacy exclusion: %v", err)
		}
	}

	if _, err := store.DB().ExecContext(ctx, "DELETE FROM migrations WHERE migration = ?", unifyTrafficExclusionsMigrationFile); err != nil {
		t.Fatalf("release migration: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("apply unified exclusion migration: %v", err)
	}

	rules, err := store.ListAllTrafficExclusions(ctx)
	if err != nil {
		t.Fatalf("list migrated exclusions: %v", err)
	}
	if len(rules) != 4 {
		t.Fatalf("expected four migrated rules, got %d", len(rules))
	}
	byID := make(map[uuid.UUID]api.IPExclusion, len(rules))
	for _, rule := range rules {
		byID[rule.ID] = rule
	}
	for _, id := range legacyIDs {
		rule, ok := byID[id]
		if !ok {
			t.Fatalf("missing migrated rule %s", id)
		}
		if rule.CreatedBy == nil || *rule.CreatedBy != userID || !rule.CreatedAt.Equal(now) {
			t.Fatalf("legacy metadata was not preserved for %s: %#v", id, rule)
		}
	}

	for _, legacyTable := range []string{"instance_exclusions", "site_exclusions", "instance_country_exclusions", "site_country_exclusions"} {
		var count int
		if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_name = ?", legacyTable).Scan(&count); err != nil {
			t.Fatalf("inspect legacy table %s: %v", legacyTable, err)
		}
		if count != 0 {
			t.Fatalf("expected legacy table %s to be removed", legacyTable)
		}
	}
}

func TestTrafficExclusionScopesTypesAndEffectiveOrdering(t *testing.T) {
	store, userID, siteID := setupExclusionStore(t)
	defer store.Close()
	ctx := context.Background()
	teamID, err := store.GetSiteTenantID(ctx, siteID)
	if err != nil {
		t.Fatalf("resolve site team: %v", err)
	}

	instanceRule, err := store.CreateInstanceTrafficExclusion(ctx, TrafficExclusionValues{Type: "user_agent", UserAgent: "HealthCheck", Description: "global monitor"}, userID)
	if err != nil {
		t.Fatalf("create instance user-agent rule: %v", err)
	}
	teamRule, err := store.CreateTeamTrafficExclusion(ctx, teamID, TrafficExclusionValues{Type: "path", Path: "/admin", Description: "team admin area"}, userID)
	if err != nil {
		t.Fatalf("create team path rule: %v", err)
	}
	siteRule, err := store.CreateSiteTrafficExclusion(ctx, siteID, TrafficExclusionValues{Type: "country", CountryCode: "de", Description: "site country"}, userID)
	if err != nil {
		t.Fatalf("create site country rule: %v", err)
	}

	effective, err := store.ListEffectiveSiteExclusions(ctx, teamID, siteID)
	if err != nil {
		t.Fatalf("list effective site exclusions: %v", err)
	}
	if len(effective) != 3 {
		t.Fatalf("expected three effective rules, got %#v", effective)
	}
	if effective[0].ID != instanceRule.ID || effective[1].ID != teamRule.ID || effective[2].ID != siteRule.ID {
		t.Fatalf("unexpected effective scope ordering: %#v", effective)
	}
	if !effective[0].Inherited || !effective[1].Inherited || effective[2].Inherited {
		t.Fatalf("unexpected inherited metadata: %#v", effective)
	}
	if effective[0].CreatedBy != nil || effective[1].CreatedBy != nil || effective[2].CreatedBy == nil {
		t.Fatalf("cross-scope creator metadata was not hidden: %#v", effective)
	}
	if effective[1].TeamID == nil || *effective[1].TeamID != teamID || effective[2].SiteID == nil || *effective[2].SiteID != siteID {
		t.Fatalf("scope owner metadata missing: %#v", effective)
	}

	if deleted, err := store.DeleteSiteExclusion(ctx, siteID, teamRule.ID); err != nil || deleted {
		t.Fatalf("child scope deleted inherited team rule: deleted=%v err=%v", deleted, err)
	}
	if deleted, err := store.DeleteTeamExclusion(ctx, teamID, teamRule.ID); err != nil || !deleted {
		t.Fatalf("team owner could not delete team rule: deleted=%v err=%v", deleted, err)
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
