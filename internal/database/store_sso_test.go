package database

import (
	"context"
	"errors"
	"testing"
)

func TestTeamSSOConfigLifecycleAndAvailability(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	userID, err := store.CreateUser(ctx, "owner@sso.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	teamID, err := store.GetActiveTenantID(ctx, userID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}

	input := TeamSSOConfig{
		TeamID:                teamID,
		ProviderType:          "oidc",
		IssuerURL:             "https://id.example.com",
		ClientID:              "hitkeep",
		ClientSecretEncrypted: "v1.encrypted",
		AllowedDomains:        []string{"example.com", "example.org"},
		EmailClaim:            "email",
		DisplayNameClaim:      "name",
		AutoProvision:         true,
		Enabled:               true,
	}
	if err := store.UpsertTeamSSOConfig(ctx, input); err != nil {
		t.Fatalf("upsert config: %v", err)
	}

	available, err := store.HasEnabledTeamSSO(ctx)
	if err != nil || !available {
		t.Fatalf("expected SSO availability, available=%v err=%v", available, err)
	}
	enabledTeamIDs, err := store.ListEnabledTeamSSOTeamIDs(ctx)
	if err != nil || len(enabledTeamIDs) != 1 || enabledTeamIDs[0] != teamID {
		t.Fatalf("expected enabled SSO team %s, team_ids=%v err=%v", teamID, enabledTeamIDs, err)
	}
	resolved, err := store.GetEnabledTeamSSOConfigByDomain(ctx, "example.org")
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if resolved == nil || resolved.TeamID != teamID || resolved.ClientSecretEncrypted != "v1.encrypted" || !resolved.AutoProvision {
		t.Fatalf("unexpected resolved config: %+v", resolved)
	}
	if len(resolved.AllowedDomains) != 2 || resolved.AllowedDomains[0] != "example.com" {
		t.Fatalf("unexpected domains: %+v", resolved.AllowedDomains)
	}
	if err := store.UpsertTeamSSOConfig(ctx, input); err != nil {
		t.Fatalf("save unchanged SSO domains: %v", err)
	}

	input.ClientSecretEncrypted = "v1.rotated"
	input.AllowedDomains = []string{"example.net"}
	input.Enabled = false
	if err := store.UpsertTeamSSOConfig(ctx, input); err != nil {
		t.Fatalf("update config: %v", err)
	}
	available, err = store.HasEnabledTeamSSO(ctx)
	if err != nil || available {
		t.Fatalf("expected SSO to be unavailable, available=%v err=%v", available, err)
	}
	enabledTeamIDs, err = store.ListEnabledTeamSSOTeamIDs(ctx)
	if err != nil || len(enabledTeamIDs) != 0 {
		t.Fatalf("expected no enabled SSO teams, team_ids=%v err=%v", enabledTeamIDs, err)
	}
	stored, err := store.GetTeamSSOConfig(ctx, teamID)
	if err != nil || stored == nil {
		t.Fatalf("get stored config: config=%+v err=%v", stored, err)
	}
	if stored.ClientSecretEncrypted != "v1.rotated" || len(stored.AllowedDomains) != 1 || stored.AllowedDomains[0] != "example.net" || !stored.AutoProvision {
		t.Fatalf("unexpected updated config: %+v", stored)
	}

	if err := store.DeleteTeamSSOConfig(ctx, teamID); err != nil {
		t.Fatalf("delete config: %v", err)
	}
	if config, err := store.GetTeamSSOConfig(ctx, teamID); err != nil || config != nil {
		t.Fatalf("expected deleted config, config=%+v err=%v", config, err)
	}
}

func TestTeamSSODomainsCannotRouteToMultipleTeams(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	userID, _ := store.CreateUser(ctx, "owner@domain-conflict.test", "hash")
	firstTeamID, _ := store.GetActiveTenantID(ctx, userID)
	secondTeam, err := store.CreateTenant(ctx, userID, "Second team", "")
	if err != nil {
		t.Fatalf("create second team: %v", err)
	}

	config := TeamSSOConfig{
		ProviderType:          "oidc",
		IssuerURL:             "https://id.example.com",
		ClientID:              "hitkeep",
		ClientSecretEncrypted: "v1.encrypted",
		AllowedDomains:        []string{"example.com"},
		EmailClaim:            "email",
		DisplayNameClaim:      "name",
		Enabled:               true,
		TeamID:                firstTeamID,
	}
	if err := store.UpsertTeamSSOConfig(ctx, config); err != nil {
		t.Fatalf("save first config: %v", err)
	}
	config.TeamID = secondTeam.ID
	if err := store.UpsertTeamSSOConfig(ctx, config); !errors.Is(err, ErrTeamSSODomainConflict) {
		t.Fatalf("expected domain conflict, got %v", err)
	}
}
