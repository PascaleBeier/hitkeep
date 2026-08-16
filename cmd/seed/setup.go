package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/hklog"
)

func deleteSiteAnalyticsData(ctx context.Context, store *database.Store, siteID uuid.UUID) {
	tables := []string{
		"hits", "events", "web_vitals", "ai_fetches", "qr_code_opens", "rollup_dirty_buckets",
		"hit_rollups_hourly", "hit_rollups_daily", "hit_rollups_monthly",
		"session_rollups_hourly", "session_rollups_daily", "session_rollups_monthly",
		"goal_rollups_hourly", "goal_rollups_daily", "goal_rollups_monthly",
		"funnel_rollups_hourly", "funnel_rollups_daily", "funnel_rollups_monthly",
	}
	for _, table := range tables {
		if err := store.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE site_id = ?", table), siteID); err != nil {
			hklog.LoggerFromContext(ctx).Warn("Failed to clear analytics data", "table", table, "error", err)
		}
	}
	hklog.LoggerFromContext(ctx).Info("Cleared existing analytics data", "site_id", siteID)
}

func deleteSiteQRCampaignData(ctx context.Context, sharedStore, analyticsStore *database.Store, siteID uuid.UUID) {
	if analyticsStore != nil {
		if err := analyticsStore.Exec(ctx, "DELETE FROM qr_code_opens WHERE site_id = ?", siteID); err != nil {
			hklog.LoggerFromContext(ctx).Warn("Failed to clear QR opens", "error", err)
		}
	}

	for _, table := range []string{"qr_code_share_links", "qr_code_assets", "qr_codes"} {
		if err := sharedStore.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE site_id = ?", table), siteID); err != nil {
			hklog.LoggerFromContext(ctx).Warn("Failed to clear QR campaign data", "table", table, "error", err)
		}
	}
	hklog.LoggerFromContext(ctx).Info("Cleared existing QR campaign data", "site_id", siteID)
}

func deleteSiteGoalsAndFunnels(ctx context.Context, store *database.Store, siteID uuid.UUID) {
	if err := store.Exec(ctx, "DELETE FROM goals WHERE site_id = ?", siteID); err != nil {
		hklog.LoggerFromContext(ctx).Warn("Failed to clear goals", "error", err)
	}
	if err := store.Exec(ctx, "DELETE FROM funnels WHERE site_id = ?", siteID); err != nil {
		hklog.LoggerFromContext(ctx).Warn("Failed to clear funnels", "error", err)
	}
	hklog.LoggerFromContext(ctx).Info("Cleared existing goals and funnels", "site_id", siteID)
}

func ensureUser(ctx context.Context, store *database.Store, email, password string) uuid.UUID {
	existing, err := store.GetUserByEmail(ctx, email)
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to look up user", "error", err)
		os.Exit(1)
	}
	if existing != nil {
		hash, err := hashPassword(password)
		if err != nil {
			hklog.LoggerFromContext(ctx).Error("Failed to hash password", "error", err)
			os.Exit(1)
		}
		if err := store.UpdatePasswordByID(ctx, existing.ID.String(), hash); err != nil {
			hklog.LoggerFromContext(ctx).Error("Failed to reset existing demo user password", "error", err, "id", existing.ID)
			os.Exit(1)
		}
		hklog.LoggerFromContext(ctx).Info("Reusing existing user and reset password", "id", existing.ID)
		return existing.ID
	}

	hash, err := hashPassword(password)
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to hash password", "error", err)
		os.Exit(1)
	}

	id, err := store.CreateUser(ctx, email, hash)
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to create demo user", "error", err)
		os.Exit(1)
	}
	return id
}

func seedAPIClients(ctx context.Context, store *database.Store, userID, tenantID, siteID uuid.UUID) {
	personalName := "Personal Export Token"
	personalClients, err := store.ListAPIClients(ctx, userID)
	if err != nil {
		hklog.LoggerFromContext(ctx).Warn("Failed to list personal API clients", "error", err)
	} else if !hasAPIClientNamed(personalClients, personalName) {
		_, _, err := store.CreateAPIClient(ctx, userID, personalName, "Read-only export token for personal automation", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
			siteID: auth.SiteViewer,
		}, nil)
		if err != nil {
			hklog.LoggerFromContext(ctx).Warn("Failed to seed personal API client", "error", err)
		}
	}

	teamName := "Shared Team Integration"
	teamClients, err := store.ListTeamAPIClients(ctx, tenantID)
	if err != nil {
		hklog.LoggerFromContext(ctx).Warn("Failed to list team API clients", "tenant_id", tenantID, "error", err)
	} else if !hasAPIClientNamed(teamClients, teamName) {
		_, _, err := store.CreateTeamAPIClient(ctx, tenantID, teamName, "Team-owned token for shared dashboards and CI exports", map[uuid.UUID]auth.SiteRole{
			siteID: auth.SiteAdmin,
		}, nil)
		if err != nil {
			hklog.LoggerFromContext(ctx).Warn("Failed to seed team API client", "tenant_id", tenantID, "error", err)
		}
	}

	hklog.LoggerFromContext(ctx).Info("Demo API clients ensured", "user_id", userID, "tenant_id", tenantID)
}

func seedShareLink(ctx context.Context, store *database.Store, siteID, createdBy uuid.UUID, token string) {
	if len(token) != 64 {
		hklog.LoggerFromContext(ctx).Error("Share token must be a 64-character hex string", "len", len(token))
		os.Exit(1)
	}
	if _, err := hex.DecodeString(token); err != nil {
		hklog.LoggerFromContext(ctx).Error("Share token is not valid hex", "error", err)
		os.Exit(1)
	}

	sum := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(sum[:])

	var exists bool
	_ = store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) > 0 FROM share_links WHERE token_hash = ? AND revoked_at IS NULL",
		tokenHash,
	).Scan(&exists)
	if exists {
		hklog.LoggerFromContext(ctx).Info("Share link already exists, skipping", "token_hint", tokenHash[:8])
		return
	}

	linkID := uuid.New()
	now := time.Now().UTC()
	if err := store.Exec(ctx,
		"INSERT INTO share_links (id, site_id, token_hash, created_by, created_at) VALUES (?, ?, ?, ?, ?)",
		linkID, siteID, tokenHash, createdBy, now,
	); err != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to create share link", "error", err)
		os.Exit(1)
	}
	hklog.LoggerFromContext(ctx).Info("Share link seeded", "token_hint", tokenHash[:8])
}

type goalIDs struct {
	newsletter uuid.UUID
	trial      uuid.UUID
	purchase   uuid.UUID
	demo       uuid.UUID
	pricing    uuid.UUID
}

func createGoals(ctx context.Context, store *database.Store, siteID uuid.UUID) goalIDs {
	create := func(name, typ, value string) uuid.UUID {
		g := &api.Goal{SiteID: siteID, Name: name, Type: typ, Value: value}
		if err := store.CreateGoal(ctx, g); err != nil {
			hklog.LoggerFromContext(ctx).Error("Failed to create goal", "name", name, "error", err)
			os.Exit(1)
		}
		goals, err := store.GetGoals(ctx, siteID)
		if err != nil || len(goals) == 0 {
			hklog.LoggerFromContext(ctx).Error("Failed to fetch goals after creation", "error", err)
			os.Exit(1)
		}
		for _, goal := range goals {
			if goal.Name == name {
				return goal.ID
			}
		}
		return uuid.Nil
	}

	ids := goalIDs{
		newsletter: create("Newsletter Signup", "event", "newsletter_signup"),
		trial:      create("Free Trial Started", "event", "trial_started"),
		purchase:   create("Purchase Completed", "event", "purchase_completed"),
		demo:       create("Demo Requested", "event", "demo_requested"),
		pricing:    create("Pricing Page", "path", "/pricing"),
	}

	hklog.LoggerFromContext(ctx).Info("Goals created", "count", 5)
	return ids
}

func createFunnels(ctx context.Context, store *database.Store, siteID uuid.UUID) {
	funnels := []api.Funnel{
		{
			SiteID: siteID,
			Name:   "Acquisition Funnel",
			Steps: []api.FunnelStep{
				{Type: "path", Value: "/"},
				{Type: "path", Value: "/pricing"},
				{Type: "path", Value: "/signup"},
				{Type: "event", Value: "trial_started"},
			},
		},
		{
			SiteID: siteID,
			Name:   "Purchase Funnel",
			Steps: []api.FunnelStep{
				{Type: "path", Value: "/pricing"},
				{Type: "path", Value: "/signup"},
				{Type: "event", Value: "purchase_completed"},
			},
		},
	}

	for _, f := range funnels {
		if err := store.CreateFunnel(ctx, &f); err != nil {
			hklog.LoggerFromContext(ctx).Error("Failed to create funnel", "name", f.Name, "error", err)
			os.Exit(1)
		}
	}
	hklog.LoggerFromContext(ctx).Info("Funnels created", "count", len(funnels))
}

type seedStats struct {
	hits          int
	sessions      int
	events        int
	qrCodes       int
	qrOpens       int
	webVitals     int
	opportunities int
	aiFetches     int
}

func mergeQRSeedStats(stats, qrStats seedStats) seedStats {
	stats.hits += qrStats.hits
	stats.sessions += qrStats.sessions
	stats.events += qrStats.events
	stats.qrCodes = qrStats.qrCodes
	stats.qrOpens = qrStats.qrOpens
	return stats
}

type aiFetchSeedStats struct {
	fetches int
	// hits and sessions count the human visits that followed an AI referral.
	hits     int
	sessions int
	// botHits counts tracked pageview hits from AI crawler user agents.
	botHits int
}

type seedWriteBatch struct {
	hits      []*api.Hit
	events    []*api.Event
	aiFetches []*api.AIFetch
}

func (b *seedWriteBatch) addHit(hit *api.Hit) {
	if hit != nil {
		b.hits = append(b.hits, hit)
	}
}

func (b *seedWriteBatch) addEvent(event *api.Event) {
	if event != nil {
		b.events = append(b.events, event)
	}
}

func (b *seedWriteBatch) addAIFetch(fetch *api.AIFetch) {
	if fetch != nil {
		b.aiFetches = append(b.aiFetches, fetch)
	}
}

func (b *seedWriteBatch) flush(ctx context.Context, store *database.Store) error {
	if len(b.hits) > 0 {
		if err := store.CreateHitsBulkUnsafe(ctx, b.hits); err != nil {
			return fmt.Errorf("insert %d hits: %w", len(b.hits), err)
		}
		b.hits = b.hits[:0]
	}
	if len(b.events) > 0 {
		if err := store.CreateEventsBulk(ctx, b.events); err != nil {
			return fmt.Errorf("insert %d events: %w", len(b.events), err)
		}
		b.events = b.events[:0]
	}
	if len(b.aiFetches) > 0 {
		if err := store.CreateAIFetchesBulk(ctx, b.aiFetches); err != nil {
			return fmt.Errorf("insert %d ai fetches: %w", len(b.aiFetches), err)
		}
		b.aiFetches = b.aiFetches[:0]
	}
	return nil
}
