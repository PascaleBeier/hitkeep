package main

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

func TestEnsureUserResetsExistingUserPassword(t *testing.T) {
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "seed.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	userID := ensureUser(ctx, store, "demo@example.com", "old-password")
	original, err := store.GetUserByEmail(ctx, "demo@example.com")
	if err != nil {
		t.Fatalf("load original user: %v", err)
	}
	if original == nil {
		t.Fatal("expected original user")
	}
	if !seedPasswordMatches(t, "old-password", original.Password) {
		t.Fatal("expected original password to match")
	}

	reusedID := ensureUser(ctx, store, "demo@example.com", "demo1234")
	if reusedID != userID {
		t.Fatalf("expected existing user id %s, got %s", userID, reusedID)
	}

	updated, err := store.GetUserByEmail(ctx, "demo@example.com")
	if err != nil {
		t.Fatalf("load updated user: %v", err)
	}
	if updated == nil {
		t.Fatal("expected updated user")
	}
	if !seedPasswordMatches(t, "demo1234", updated.Password) {
		t.Fatal("expected reseeded password to match")
	}
	if seedPasswordMatches(t, "old-password", updated.Password) {
		t.Fatal("expected old password to stop matching")
	}
}

func TestSeedActivationFixturesKeepsPrimaryDemoSiteFirst(t *testing.T) {
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "seed.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	userID := ensureUser(ctx, store, "demo@example.com", "demo1234")
	seedTeam(ctx, store, userID)

	primary, err := ensureSiteInActiveTeam(ctx, store, userID, "acme-analytics.io")
	if err != nil {
		t.Fatalf("ensure primary site: %v", err)
	}

	seedActivationFixtures(ctx, store, userID, primary.ID)

	sites, err := store.GetSites(ctx, userID)
	if err != nil {
		t.Fatalf("get sites: %v", err)
	}
	if len(sites) < 3 {
		t.Fatalf("expected primary and activation fixture sites, got %d", len(sites))
	}
	if sites[0].ID != primary.ID {
		t.Fatalf("expected primary demo site first, got %s (%s)", sites[0].Domain, sites[0].ID)
	}
}

func TestSeedGoogleSearchConsoleAndActivationFixturesKeepMappedPrimarySiteFirst(t *testing.T) {
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "seed.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	tenantMgr := database.NewTenantStoreManager(store, t.TempDir())
	defer tenantMgr.Close()

	userID := ensureUser(ctx, store, "demo@example.com", "demo1234")
	seedTeam(ctx, store, userID)
	primary, err := ensureSiteInActiveTeam(ctx, store, userID, "acme-analytics.io")
	if err != nil {
		t.Fatalf("ensure primary site: %v", err)
	}
	if err := tenantMgr.SyncSite(ctx, primary.ID); err != nil {
		t.Fatalf("sync primary site: %v", err)
	}

	seedGoogleSearchConsoleFixtures(ctx, store, tenantMgr, userID, primary.ID, 90)
	seedActivationFixtures(ctx, store, userID, primary.ID)

	sites, err := store.GetSites(ctx, userID)
	if err != nil {
		t.Fatalf("get sites: %v", err)
	}
	if len(sites) < 6 {
		t.Fatalf("expected primary plus Search Console and activation fixture sites, got %d", len(sites))
	}
	if sites[0].ID != primary.ID {
		t.Fatalf("expected mapped primary demo site first, got %s (%s)", sites[0].Domain, sites[0].ID)
	}
}

func TestSeedGoogleSearchConsoleFixturesCreatesMappedReportRows(t *testing.T) {
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "seed.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	tenantMgr := database.NewTenantStoreManager(store, t.TempDir())
	defer tenantMgr.Close()

	userID := ensureUser(ctx, store, "demo@example.com", "demo1234")
	seedTeam(ctx, store, userID)
	primary, err := ensureSiteInActiveTeam(ctx, store, userID, "acme-analytics.io")
	if err != nil {
		t.Fatalf("ensure primary site: %v", err)
	}
	if err := tenantMgr.SyncSite(ctx, primary.ID); err != nil {
		t.Fatalf("sync primary site: %v", err)
	}

	stats := seedGoogleSearchConsoleFixtures(ctx, store, tenantMgr, userID, primary.ID, 90)
	if stats.facts == 0 {
		t.Fatalf("expected seeded Search Console facts")
	}

	teamID, err := store.GetSiteTenantID(ctx, primary.ID)
	if err != nil {
		t.Fatalf("GetSiteTenantID: %v", err)
	}
	conn, err := store.GetGoogleSearchConsoleConnection(ctx, teamID)
	if err != nil {
		t.Fatalf("GetGoogleSearchConsoleConnection: %v", err)
	}
	if conn == nil || !conn.Connected || conn.GoogleAccountEmail != "demo-search-console@example.com" {
		t.Fatalf("expected connected demo Search Console account, got %+v", conn)
	}
	mapping, err := store.GetGoogleSearchConsoleSiteMappingForTeam(ctx, primary.ID, teamID)
	if err != nil {
		t.Fatalf("GetGoogleSearchConsoleSiteMappingForTeam: %v", err)
	}
	if mapping == nil || mapping.PropertyURI != "sc-domain:acme-analytics.io" {
		t.Fatalf("expected primary Search Console mapping, got %+v", mapping)
	}
	state, err := store.GetGoogleSearchConsoleSyncState(ctx, primary.ID)
	if err != nil {
		t.Fatalf("GetGoogleSearchConsoleSyncState: %v", err)
	}
	if state == nil || state.State != "succeeded" || state.LastSuccessAt == nil {
		t.Fatalf("expected successful primary sync state, got %+v", state)
	}

	tenantStore, _, err := tenantMgr.ResolveSiteStore(ctx, primary.ID)
	if err != nil {
		t.Fatalf("ResolveSiteStore: %v", err)
	}
	overview, err := tenantStore.GetSearchConsoleOverview(ctx, api.SearchConsoleReportParams{
		SiteID:      primary.ID,
		PropertyURI: "sc-domain:acme-analytics.io",
		Start:       time.Now().UTC().AddDate(0, 0, -30),
		End:         time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("GetSearchConsoleOverview: %v", err)
	}
	if overview.Clicks == 0 || overview.Impressions == 0 || overview.DataSource != "google_search_console" {
		t.Fatalf("expected useful Search Console overview, got %+v", overview)
	}
}

func TestSeedGoogleSearchConsoleFixturesCreatesStatusExamples(t *testing.T) {
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "seed.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	tenantMgr := database.NewTenantStoreManager(store, t.TempDir())
	defer tenantMgr.Close()

	userID := ensureUser(ctx, store, "demo@example.com", "demo1234")
	seedTeam(ctx, store, userID)
	primary, err := ensureSiteInActiveTeam(ctx, store, userID, "acme-analytics.io")
	if err != nil {
		t.Fatalf("ensure primary site: %v", err)
	}

	seedGoogleSearchConsoleFixtures(ctx, store, tenantMgr, userID, primary.ID, 90)

	expectedStates := map[string]string{
		"search-pending.example.com":   "pending",
		"search-quota.example.com":     "failed",
		"search-reconnect.example.com": "needs_attention",
	}
	for domain, expectedState := range expectedStates {
		site := requireSeedSiteByDomain(t, ctx, store, domain)
		state, err := store.GetGoogleSearchConsoleSyncState(ctx, site.ID)
		if err != nil {
			t.Fatalf("GetGoogleSearchConsoleSyncState(%s): %v", domain, err)
		}
		if state == nil || state.State != expectedState {
			t.Fatalf("expected %s state %q, got %+v", domain, expectedState, state)
		}
	}

	unmapped := requireSeedSiteByDomain(t, ctx, store, "search-unmapped.example.com")
	mapping, err := store.GetGoogleSearchConsoleSiteMapping(ctx, unmapped.ID)
	if err != nil {
		t.Fatalf("GetGoogleSearchConsoleSiteMapping: %v", err)
	}
	if mapping != nil {
		t.Fatalf("expected unmapped fixture site, got %+v", mapping)
	}
}

func requireSeedSiteByDomain(t *testing.T, ctx context.Context, store *database.Store, domain string) *api.Site {
	t.Helper()
	var site api.Site
	if err := store.DB().QueryRowContext(ctx, `
		SELECT id, user_id, domain, data_retention_days, created_at
		FROM sites
		WHERE lower(domain) = lower(?)
		LIMIT 1
	`, domain).Scan(&site.ID, &site.UserID, &site.Domain, &site.DataRetentionDays, &site.CreatedAt); err != nil {
		t.Fatalf("load seed site %s: %v", domain, err)
	}
	return &site
}

func seedPasswordMatches(t *testing.T, password string, encoded string) bool {
	t.Helper()

	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		t.Fatalf("invalid password hash format: %q", encoded)
	}
	if parts[1] != "argon2id" {
		t.Fatalf("unexpected password algorithm: %q", parts[1])
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		t.Fatalf("parse argon2 version: %v", err)
	}
	if version != argon2.Version {
		t.Fatalf("unexpected argon2 version: %d", version)
	}

	var memory uint32
	var timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		t.Fatalf("parse argon2 params: %v", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		t.Fatalf("decode salt: %v", err)
	}
	decodedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		t.Fatalf("decode hash: %v", err)
	}

	comparisonHash := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(decodedHash)))
	return subtle.ConstantTimeCompare(decodedHash, comparisonHash) == 1
}
