// seed populates a HitKeep database with deterministic demo data.
//
// Usage:
//
//	go run ./cmd/seed -db hitkeep.db -email demo@example.com -password demo1234
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log/slog"
	mrand "math/rand"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"

	"hitkeep/internal/api"
	"hitkeep/internal/assetstore"
	"hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/hklog"
	"hitkeep/internal/worker"
)

func hashPassword(password string) (string, error) {
	const (
		timeCost = 1
		memory   = 64 * 1024
		threads  = 4
		keyLen   = 32
		saltLen  = 16
	)
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, keyLen)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, memory, timeCost, threads, b64Salt, b64Hash), nil
}

type weightedEntry[T any] struct {
	value  T
	weight int
}

func pickWeighted[T any](rng *mrand.Rand, entries []weightedEntry[T]) T {
	total := 0
	for _, e := range entries {
		total += e.weight
	}
	n := rng.Intn(total)
	for _, e := range entries {
		n -= e.weight
		if n < 0 {
			return e.value
		}
	}
	return entries[len(entries)-1].value
}

func main() {
	dbPath := flag.String("db", "hitkeep.db", "Path to hitkeep.db")
	defaultDataPath := os.Getenv("HITKEEP_DATA_PATH")
	if strings.TrimSpace(defaultDataPath) == "" {
		defaultDataPath = "data"
	}
	dataPath := flag.String("data-path", defaultDataPath, "Base directory for per-tenant data files")
	email := flag.String("email", "demo@example.com", "Demo user email")
	password := flag.String("password", "demo1234", "Demo user password")
	days := flag.Int("days", 90, "Days of demo traffic to generate")
	domain := flag.String("domain", "acme-analytics.io", "Demo site domain")
	seed := flag.Int64("seed", 42, "Random seed for reproducibility")
	shareToken := flag.String("share-token", "", "Create a share link with this exact token (64-char hex string)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := hklog.WithLogger(context.Background(), logger)

	store, err := database.OpenDefaultSplitControlStore(ctx, *dbPath, database.WithLogger(logger))
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	tenantBasePath := strings.TrimSpace(*dataPath)
	complete, err := store.DefaultTenantSplitComplete(ctx)
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to inspect default tenant migration", "error", err)
		os.Exit(1)
	}
	if !complete {
		if err := store.Close(); err != nil {
			hklog.LoggerFromContext(ctx).Error("Failed to close control database before default tenant migration", "error", err)
			os.Exit(1)
		}
		if err := database.RunDefaultTenantSplit(ctx, *dbPath, tenantBasePath, database.WithLogger(logger)); err != nil {
			hklog.LoggerFromContext(ctx).Error("Failed to migrate default tenant analytics", "error", err)
			os.Exit(1)
		}
		store, err = database.OpenMigratedStore(ctx, *dbPath, database.WithLogger(logger))
		if err != nil {
			hklog.LoggerFromContext(ctx).Error("Failed to reopen control database", "error", err)
			os.Exit(1)
		}
	}

	tenantMgr := database.NewTenantStoreManager(store, tenantBasePath, database.WithTenantDataPlane(true))
	defer tenantMgr.Close()

	rng := mrand.New(mrand.NewSource(*seed)) // #nosec G404 -- demo data seeding uses reproducible randomness.

	seedAdditionalUsers(ctx, store)

	hklog.LoggerFromContext(ctx).Info("Creating demo user")
	userID := ensureUser(ctx, store, *email, *password)

	if err := store.UpdateInstanceRole(ctx, userID, auth.InstanceOwner, userID); err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to set demo user as instance owner", "error", err)
		os.Exit(1)
	}
	hklog.LoggerFromContext(ctx).Info("Demo user promoted to instance owner", "user_id", userID)

	adminID := ensureAdminUser(ctx, store, userID)

	seedTeam(ctx, store, userID)

	ownerTeamID, err := store.GetActiveTenantID(ctx, userID)
	if err == nil && ownerTeamID != uuid.Nil {
		if err := store.AddTeamMember(ctx, ownerTeamID, adminID, database.TenantRoleAdmin, userID); err != nil {
			hklog.LoggerFromContext(ctx).Warn("Failed to add admin user to demo team", "error", err)
		} else {
			hklog.LoggerFromContext(ctx).Info("Admin user added to demo team", "team_id", ownerTeamID, "role", "admin")
		}
	}

	siteContext := ensureSeedSiteContext(ctx, store, tenantMgr, userID, *domain, *shareToken)
	siteID := siteContext.siteID
	analyticsStore := siteContext.analyticsStore

	deleteSiteQRCampaignData(ctx, store, analyticsStore, siteID)
	if err := assetstore.New(tenantBasePath).DeleteQRCodeAssetsForSite(siteID); err != nil {
		hklog.LoggerFromContext(ctx).Warn("Failed to remove existing QR campaign assets before seeding", "error", err, "site_id", siteID)
	}
	deleteSiteAnalyticsData(ctx, analyticsStore, siteID)
	deleteSiteGoalsAndFunnels(ctx, analyticsStore, siteID)

	goalIDs := createGoals(ctx, analyticsStore, siteID)
	createFunnels(ctx, analyticsStore, siteID)

	hklog.LoggerFromContext(ctx).Info("Seeding traffic", "days", *days)
	stats, err := seedTraffic(ctx, analyticsStore, siteID, goalIDs, *days, rng)
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to seed traffic", "error", err)
		os.Exit(1)
	}
	qrStats, err := seedQRCampaigns(ctx, store, analyticsStore, siteID, userID, siteContext.site.Domain, *days, tenantBasePath, rng)
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to seed QR campaigns", "error", err)
		os.Exit(1)
	}
	stats = mergeQRSeedStats(stats, qrStats)

	webVitals, err := seedWebVitals(ctx, analyticsStore, siteID, *days, rng)
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to seed Web Vitals", "error", err)
		os.Exit(1)
	}
	stats.webVitals = webVitals
	aiSeedStats, err := seedAIVisibility(ctx, analyticsStore, siteID, *days, rng)
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to seed AI visibility", "error", err)
		os.Exit(1)
	}
	stats.aiFetches = aiSeedStats.fetches
	stats.hits += aiSeedStats.hits + aiSeedStats.botHits
	stats.sessions += aiSeedStats.sessions + aiSeedStats.botHits

	searchConsoleStats := seedGoogleSearchConsoleFixtures(ctx, store, tenantMgr, userID, siteID, *days)
	seedActivationFixtures(ctx, store, userID, siteID)

	hklog.LoggerFromContext(ctx).Info("Running rollup backfill...")
	rollupWorker := worker.NewRollupBackfillWorker(tenantMgr)
	if err := rollupWorker.Run(ctx); err != nil {
		hklog.LoggerFromContext(ctx).Error("Rollup backfill failed — charts may be incomplete", "error", err)
	}
	opportunityCount, err := seedOpportunities(ctx, store, analyticsStore, siteContext.site, userID, time.Now().UTC().AddDate(0, 0, -(*days)), time.Now().UTC())
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to seed Opportunities", "error", err)
		os.Exit(1)
	}
	stats.opportunities = opportunityCount

	printSeedSummary(*email, *domain, siteID, tenantBasePath, stats, searchConsoleStats, *days)
}

func ensureAdminUser(ctx context.Context, store *database.Store, actorID uuid.UUID) uuid.UUID {
	adminEmail := "admin@example.com"
	adminID := ensureUser(ctx, store, adminEmail, "admin1234")
	if err := store.UpdateInstanceRole(ctx, adminID, auth.InstanceAdmin, actorID); err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to set admin user role", "error", err)
		os.Exit(1)
	}
	hklog.LoggerFromContext(ctx).Info("Admin user created with instance admin role", "user_id", adminID)
	return adminID
}

type seedSiteContext struct {
	site           api.Site
	siteID         uuid.UUID
	analyticsStore *database.Store
}

func ensureSeedSiteContext(ctx context.Context, store *database.Store, tenantMgr *database.TenantStoreManager, userID uuid.UUID, domain, shareToken string) seedSiteContext {
	hklog.LoggerFromContext(ctx).Info("Creating demo site", "domain", domain)
	site, err := ensureSiteInActiveTeam(ctx, store, userID, domain)
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to ensure demo site", "error", err)
		os.Exit(1)
	}
	siteID := site.ID
	hklog.LoggerFromContext(ctx).Info("Site created", "site_id", siteID)

	siteTenantID, err := store.GetSiteTenantID(ctx, siteID)
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to resolve site tenant", "site_id", siteID, "error", err)
		os.Exit(1)
	}
	seedAPIClients(ctx, store, userID, siteTenantID, siteID)
	if shareToken != "" {
		seedShareLink(ctx, store, siteID, userID, shareToken)
	}
	if err := tenantMgr.SyncSite(ctx, siteID); err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to sync tenant site metadata", "site_id", siteID, "tenant_id", siteTenantID, "error", err)
		os.Exit(1)
	}
	analyticsStore, err := tenantMgr.ForTenant(ctx, siteTenantID)
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to resolve tenant analytics store", "tenant_id", siteTenantID, "error", err)
		os.Exit(1)
	}
	hklog.LoggerFromContext(ctx).Info("Resolved tenant analytics store", "tenant_id", siteTenantID)
	return seedSiteContext{site: *site, siteID: siteID, analyticsStore: analyticsStore}
}

func printSeedSummary(email, domain string, siteID uuid.UUID, tenantBasePath string, stats seedStats, searchConsoleStats searchConsoleSeedStats, days int) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║       Demo data seeded successfully!         ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Printf("  Email:         %s\n", email)
	fmt.Println("  Password:      [set from -password flag]")
	fmt.Printf("  Site:          %s (%s)\n", domain, siteID)
	fmt.Printf("  Tenant Data:   %s\n", tenantBasePath)
	fmt.Printf("  Pageviews:     %d\n", stats.hits)
	fmt.Printf("  Sessions:      %d\n", stats.sessions)
	fmt.Printf("  Events:        %d\n", stats.events)
	fmt.Printf("  QR Codes:      %d\n", stats.qrCodes)
	fmt.Printf("  QR Opens:      %d\n", stats.qrOpens)
	fmt.Printf("  Web Vitals:    %d\n", stats.webVitals)
	fmt.Printf("  Opportunities: %d\n", stats.opportunities)
	fmt.Printf("  AI Fetches:    %d\n", stats.aiFetches)
	fmt.Printf("  Search Console Rows: %d\n", searchConsoleStats.facts)
	fmt.Printf("  Period:        last %d days\n", days)
	fmt.Println()
}
