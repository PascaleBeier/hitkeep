package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/database"
)

func ensureUser(ctx context.Context, store *database.Store, email, password string) uuid.UUID {
	existing, err := store.GetUserByEmail(ctx, email)
	if err != nil {
		slog.Error("Failed to look up user", "error", err)
		os.Exit(1)
	}
	if existing != nil {
		hash, err := hashPassword(password)
		if err != nil {
			slog.Error("Failed to hash password", "error", err)
			os.Exit(1)
		}
		if err := store.UpdatePasswordByID(ctx, existing.ID.String(), hash); err != nil {
			slog.Error("Failed to reset existing demo user password", "error", err, "id", existing.ID)
			os.Exit(1)
		}
		slog.Info("Reusing existing user and reset password", "id", existing.ID)
		return existing.ID
	}

	hash, err := hashPassword(password)
	if err != nil {
		slog.Error("Failed to hash password", "error", err)
		os.Exit(1)
	}

	id, err := store.CreateUser(ctx, email, hash)
	if err != nil {
		slog.Error("Failed to create demo user", "error", err)
		os.Exit(1)
	}
	return id
}

func seedAPIClients(ctx context.Context, store *database.Store, userID, tenantID, siteID uuid.UUID) {
	personalName := "Personal Export Token"
	personalClients, err := store.ListAPIClients(ctx, userID)
	if err != nil {
		slog.Warn("Failed to list personal API clients", "error", err)
	} else if !hasAPIClientNamed(personalClients, personalName) {
		_, _, err := store.CreateAPIClient(ctx, userID, personalName, "Read-only export token for personal automation", auth.InstanceUser, map[uuid.UUID]auth.SiteRole{
			siteID: auth.SiteViewer,
		}, nil)
		if err != nil {
			slog.Warn("Failed to seed personal API client", "error", err)
		}
	}

	teamName := "Shared Team Integration"
	teamClients, err := store.ListTeamAPIClients(ctx, tenantID)
	if err != nil {
		slog.Warn("Failed to list team API clients", "tenant_id", tenantID, "error", err)
	} else if !hasAPIClientNamed(teamClients, teamName) {
		_, _, err := store.CreateTeamAPIClient(ctx, tenantID, teamName, "Team-owned token for shared dashboards and CI exports", map[uuid.UUID]auth.SiteRole{
			siteID: auth.SiteAdmin,
		}, nil)
		if err != nil {
			slog.Warn("Failed to seed team API client", "tenant_id", tenantID, "error", err)
		}
	}

	slog.Info("Demo API clients ensured", "user_id", userID, "tenant_id", tenantID)
}

func seedShareLink(ctx context.Context, store *database.Store, siteID, createdBy uuid.UUID, token string) {
	if len(token) != 64 {
		slog.Error("Share token must be a 64-character hex string", "len", len(token))
		os.Exit(1)
	}
	if _, err := hex.DecodeString(token); err != nil {
		slog.Error("Share token is not valid hex", "error", err)
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
		slog.Info("Share link already exists, skipping", "token_hint", tokenHash[:8])
		return
	}

	linkID := uuid.New()
	now := time.Now().UTC()
	if err := store.Exec(ctx,
		"INSERT INTO share_links (id, site_id, token_hash, created_by, created_at) VALUES (?, ?, ?, ?, ?)",
		linkID, siteID, tokenHash, createdBy, now,
	); err != nil {
		slog.Error("Failed to create share link", "error", err)
		os.Exit(1)
	}
	slog.Info("Share link seeded", "token_hint", tokenHash[:8])
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
			slog.Error("Failed to create goal", "name", name, "error", err)
			os.Exit(1)
		}
		goals, err := store.GetGoals(ctx, siteID)
		if err != nil || len(goals) == 0 {
			slog.Error("Failed to fetch goals after creation", "error", err)
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

	slog.Info("Goals created", "count", 5)
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
			slog.Error("Failed to create funnel", "name", f.Name, "error", err)
			os.Exit(1)
		}
	}
	slog.Info("Funnels created", "count", len(funnels))
}

type seedStats struct {
	hits      int
	sessions  int
	events    int
	aiFetches int
}

type aiFetchSeedStats struct {
	fetches  int
	hits     int
	sessions int
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
		if err := store.CreateHitsBulk(ctx, b.hits); err != nil {
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
