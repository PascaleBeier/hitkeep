package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

func seedAdditionalUsers(ctx context.Context, store *database.Store) {
	extra := []struct{ email, domain string }{
		{"bob@devtools.co", "devtools.co"},
		{"diana@saaslaunch.com", "saaslaunch.com"},
	}

	created := 0
	for _, u := range extra {
		hash, err := hashPassword("demoPwd!1")
		if err != nil {
			continue
		}
		id, err := store.CreateUser(ctx, u.email, hash)
		if err != nil {
			continue
		}
		_, _ = store.CreateSite(ctx, id, u.domain)
		created++
	}
	slog.Info("Additional users seeded for admin panel", "count", created)
}

func seedTeam(ctx context.Context, store *database.Store, ownerID uuid.UUID) {
	teamName := "Acme Analytics"
	teamLogo := ""
	var team *api.Team

	teams, _, err := store.ListUserTeams(ctx, ownerID)
	if err == nil {
		for _, candidate := range teams {
			if strings.EqualFold(strings.TrimSpace(candidate.Name), teamName) {
				c := candidate
				team = &c
				break
			}
		}
	}

	if team == nil {
		created, createErr := store.CreateTenant(ctx, ownerID, teamName, teamLogo)
		if createErr != nil {
			slog.Warn("Failed to create demo team; retrying lookup", "error", createErr)
			teams, _, err = store.ListUserTeams(ctx, ownerID)
			if err == nil {
				for _, candidate := range teams {
					if strings.EqualFold(strings.TrimSpace(candidate.Name), teamName) {
						c := candidate
						team = &c
						break
					}
				}
			}
			if team == nil {
				slog.Warn("Failed to resolve demo team", "error", createErr)
				return
			}
		} else {
			team = created
		}
	}

	if err := store.SetActiveTenantID(ctx, ownerID, team.ID); err != nil {
		slog.Warn("Failed to set active tenant", "error", err)
	}

	memberRoles := []struct {
		email string
		role  string
	}{
		{"bob@devtools.co", "admin"},
		{"diana@saaslaunch.com", "member"},
	}
	for _, m := range memberRoles {
		user, err := store.GetUserByEmail(ctx, m.email)
		if err != nil || user == nil {
			slog.Warn("Team member user not found, skipping", "email", m.email)
			continue
		}
		if err := store.AddTeamMember(ctx, team.ID, user.ID, m.role, ownerID); err != nil {
			slog.Warn("Failed to add team member", "email", m.email, "error", err)
		}
	}

	secondaryTeamName := "Northwind Studio"
	secondaryTeam, err := store.CreateTenant(ctx, ownerID, secondaryTeamName, "")
	if err != nil {
		teams, _, listErr := store.ListUserTeams(ctx, ownerID)
		if listErr == nil {
			for _, candidate := range teams {
				if strings.EqualFold(strings.TrimSpace(candidate.Name), secondaryTeamName) {
					c := candidate
					secondaryTeam = &c
					break
				}
			}
		}
	}
	if secondaryTeam != nil {
		slog.Info("Secondary demo team available", "name", secondaryTeamName, "id", secondaryTeam.ID)
	}

	if err := store.SetActiveTenantID(ctx, ownerID, team.ID); err != nil {
		slog.Warn("Failed to restore active tenant after seeding extra teams", "error", err, "team_id", team.ID)
	}

	slog.Info("Demo team seeded", "name", teamName, "id", team.ID, "members", len(memberRoles)+1)
}

func ensureSiteInActiveTeam(ctx context.Context, store *database.Store, userID uuid.UUID, domain string) (*api.Site, error) {
	normalized := strings.TrimSpace(strings.ToLower(domain))
	activeTenantID, err := store.GetActiveTenantID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("resolve active tenant: %w", err)
	}

	var existing api.Site
	err = store.DB().QueryRowContext(ctx,
		`SELECT id, user_id, domain, data_retention_days, created_at
		 FROM sites
		 WHERE lower(domain) = ?
		 LIMIT 1`,
		normalized,
	).Scan(&existing.ID, &existing.UserID, &existing.Domain, &existing.DataRetentionDays, &existing.CreatedAt)
	if err == nil {
		if existing.UserID != userID {
			return nil, fmt.Errorf("domain %q already exists under a different user", domain)
		}

		if _, err := store.DB().ExecContext(ctx, `
			INSERT INTO site_tenants (site_id, tenant_id, created_at)
			VALUES (?, ?, NOW())
			ON CONFLICT (site_id) DO UPDATE SET
				tenant_id = excluded.tenant_id
		`, existing.ID, activeTenantID); err != nil {
			return nil, fmt.Errorf("rebind existing site to active tenant: %w", err)
		}

		slog.Info("Reusing existing demo site", "site_id", existing.ID, "domain", existing.Domain, "tenant_id", activeTenantID)
		return &existing, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("lookup existing site: %w", err)
	}

	sites, err := store.GetSites(ctx, userID)
	if err == nil {
		for _, site := range sites {
			if strings.EqualFold(strings.TrimSpace(site.Domain), normalized) {
				slog.Info("Reusing existing demo site", "site_id", site.ID, "domain", site.Domain)
				siteCopy := site
				return &siteCopy, nil
			}
		}
	}

	return store.CreateSite(ctx, userID, domain)
}

func seedActivationFixtures(ctx context.Context, store *database.Store, userID, primarySiteID uuid.UUID) {
	now := time.Now().UTC()

	primary, err := store.GetSiteByID(ctx, primarySiteID)
	if err != nil || primary == nil {
		slog.Warn("Skipping activation fixtures; primary site unavailable", "site_id", primarySiteID, "error", err)
		return
	}

	waiting, err := ensureSiteInActiveTeam(ctx, store, userID, "launch-waiting.example.com")
	if err != nil {
		slog.Warn("Failed to ensure waiting activation site", "error", err)
	}
	dormant, err := ensureSiteInActiveTeam(ctx, store, userID, "legacy-dormant.example.com")
	if err != nil {
		slog.Warn("Failed to ensure dormant activation site", "error", err)
	}

	sitesToReset := []uuid.UUID{primarySiteID}
	if waiting != nil {
		sitesToReset = append(sitesToReset, waiting.ID)
	}
	if dormant != nil {
		sitesToReset = append(sitesToReset, dormant.ID)
	}

	// Keep the analytics-rich primary site first in freshly seeded dashboards.
	if err := store.Exec(ctx, "UPDATE sites SET created_at = ? WHERE id = ?", now, primarySiteID); err != nil {
		slog.Warn("Failed to refresh primary demo site timestamp", "site_id", primarySiteID, "error", err)
	}
	if waiting != nil {
		if err := store.Exec(ctx, "UPDATE sites SET created_at = ? WHERE id = ?", now.Add(-2*time.Hour), waiting.ID); err != nil {
			slog.Warn("Failed to age waiting activation site", "site_id", waiting.ID, "error", err)
		}
	}
	if dormant != nil {
		if err := store.Exec(ctx, "UPDATE sites SET created_at = ? WHERE id = ?", now.Add(-3*time.Hour), dormant.ID); err != nil {
			slog.Warn("Failed to age dormant activation site", "site_id", dormant.ID, "error", err)
		}
	}

	for _, siteID := range sitesToReset {
		_ = store.Exec(ctx, "DELETE FROM site_activity_hourly_counts WHERE site_id = ?", siteID)
		_ = store.Exec(ctx, "DELETE FROM site_activity_summary WHERE site_id = ?", siteID)
	}

	primaryHost := primary.Domain
	if err := store.RecordHitActivity(ctx, []*api.Hit{
		{SiteID: primarySiteID, Timestamp: now.AddDate(0, 0, -30), Hostname: &primaryHost, TrackerSource: "hk.js"},
		{SiteID: primarySiteID, Timestamp: now.Add(-9 * time.Minute), Hostname: &primaryHost, TrackerSource: "hk.js"},
	}); err != nil {
		slog.Warn("Failed to seed primary activation hits", "error", err)
	}
	if err := store.RecordEventActivity(ctx, []*api.Event{
		{SiteID: primarySiteID, Name: "outbound_click", Timestamp: now.Add(-7 * time.Minute), TrackerSource: "hk.js"},
		{SiteID: primarySiteID, Name: "file_download", Timestamp: now.Add(-4 * time.Minute), TrackerSource: "hk.js"},
	}); err != nil {
		slog.Warn("Failed to seed primary activation events", "error", err)
	}
	seedActivationCount(ctx, store, primarySiteID, now, 184, 27)
	seedActivationCount(ctx, store, primarySiteID, now.Add(-24*time.Hour), 612, 84)
	seedActivationCount(ctx, store, primarySiteID, now.AddDate(0, 0, -6), 944, 131)

	if dormant != nil {
		dormantHost := dormant.Domain
		dormantAt := now.AddDate(0, 0, -12)
		if err := store.RecordHitActivity(ctx, []*api.Hit{
			{SiteID: dormant.ID, Timestamp: dormantAt, Hostname: &dormantHost, TrackerSource: "wordpress", TrackerVersion: "2.3.0-demo"},
		}); err != nil {
			slog.Warn("Failed to seed dormant activation hit", "error", err)
		}
		if err := store.RecordEventActivity(ctx, []*api.Event{
			{SiteID: dormant.ID, Name: "form_submit", Timestamp: dormantAt.Add(2 * time.Minute), TrackerSource: "wordpress", TrackerVersion: "2.3.0-demo"},
		}); err != nil {
			slog.Warn("Failed to seed dormant activation event", "error", err)
		}
	}

	slog.Info("Activation fixtures seeded", "sites", len(sitesToReset))
}

func seedActivationCount(ctx context.Context, store *database.Store, siteID uuid.UUID, ts time.Time, hits, events int) {
	tenantID, err := store.GetSiteTenantID(ctx, siteID)
	if err != nil {
		slog.Warn("Failed to resolve activation fixture tenant", "site_id", siteID, "error", err)
		return
	}
	if err := store.Exec(ctx, `
		INSERT INTO site_activity_hourly_counts (site_id, tenant_id, bucket, hits, events, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (site_id, bucket) DO UPDATE SET
			tenant_id = excluded.tenant_id,
			hits = excluded.hits,
			events = excluded.events,
			updated_at = excluded.updated_at
	`, siteID, tenantID, ts.UTC().Truncate(time.Hour), hits, events, time.Now().UTC()); err != nil {
		slog.Warn("Failed to seed activation fixture counts", "site_id", siteID, "error", err)
	}
}
