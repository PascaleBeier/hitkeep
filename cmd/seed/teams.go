package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

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
