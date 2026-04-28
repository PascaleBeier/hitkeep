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

	"golang.org/x/crypto/argon2"

	"hitkeep/internal/database"
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

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx := context.Background()

	store := database.NewStore(*dbPath)
	if err := store.Connect(); err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		slog.Error("Failed to run migrations", "error", err)
		os.Exit(1)
	}
	tenantBasePath := strings.TrimSpace(*dataPath)

	tenantMgr := database.NewTenantStoreManager(store, tenantBasePath)
	defer tenantMgr.Close()

	rng := mrand.New(mrand.NewSource(*seed)) // #nosec G404 -- demo data seeding uses reproducible randomness.

	seedAdditionalUsers(ctx, store)

	slog.Info("Creating demo user", "email", *email)
	userID := ensureUser(ctx, store, *email, *password)

	seedTeam(ctx, store, userID)

	slog.Info("Creating demo site", "domain", *domain)
	site, err := ensureSiteInActiveTeam(ctx, store, userID, *domain)
	if err != nil {
		slog.Error("Failed to ensure demo site", "error", err)
		os.Exit(1)
	}
	siteID := site.ID
	slog.Info("Site created", "site_id", siteID)

	siteTenantID, err := store.GetSiteTenantID(ctx, siteID)
	if err != nil {
		slog.Error("Failed to resolve site tenant", "site_id", siteID, "error", err)
		os.Exit(1)
	}
	seedAPIClients(ctx, store, userID, siteTenantID, siteID)
	if *shareToken != "" {
		seedShareLink(ctx, store, siteID, userID, *shareToken)
	}
	if err := tenantMgr.SyncSite(ctx, siteID); err != nil {
		slog.Error("Failed to sync tenant site metadata", "site_id", siteID, "tenant_id", siteTenantID, "error", err)
		os.Exit(1)
	}
	analyticsStore, err := tenantMgr.ForTenant(ctx, siteTenantID)
	if err != nil {
		slog.Error("Failed to resolve tenant analytics store", "tenant_id", siteTenantID, "error", err)
		os.Exit(1)
	}
	slog.Info("Resolved tenant analytics store", "tenant_id", siteTenantID)

	deleteSiteAnalyticsData(ctx, analyticsStore, siteID)
	deleteSiteGoalsAndFunnels(ctx, analyticsStore, siteID)

	goalIDs := createGoals(ctx, analyticsStore, siteID)
	createFunnels(ctx, analyticsStore, siteID)

	slog.Info("Seeding traffic", "days", *days)
	stats, err := seedTraffic(ctx, analyticsStore, siteID, goalIDs, *days, rng)
	if err != nil {
		slog.Error("Failed to seed traffic", "error", err)
		os.Exit(1)
	}
	aiSeedStats, err := seedAIFetches(ctx, analyticsStore, siteID, *days, rng)
	if err != nil {
		slog.Error("Failed to seed AI visibility", "error", err)
		os.Exit(1)
	}
	stats.aiFetches = aiSeedStats.fetches
	stats.hits += aiSeedStats.hits
	stats.sessions += aiSeedStats.sessions

	slog.Info("Running rollup backfill...")
	rollupWorker := worker.NewRollupBackfillWorker(tenantMgr)
	if err := rollupWorker.Run(ctx); err != nil {
		slog.Error("Rollup backfill failed — charts may be incomplete", "error", err)
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║       Demo data seeded successfully!         ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Printf("  Email:         %s\n", *email)
	fmt.Printf("  Password:      %s\n", *password)
	fmt.Printf("  Site:          %s (%s)\n", *domain, siteID)
	fmt.Printf("  Tenant Data:   %s\n", tenantBasePath)
	fmt.Printf("  Pageviews:     %d\n", stats.hits)
	fmt.Printf("  Sessions:      %d\n", stats.sessions)
	fmt.Printf("  Events:        %d\n", stats.events)
	fmt.Printf("  AI Fetches:    %d\n", stats.aiFetches)
	fmt.Printf("  Period:        last %d days\n", *days)
	fmt.Println()
}
