package database

import (
	"context"
	"testing"
)

func TestResolveSSOUserCreatesAndReusesIdentity(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	ownerID, _ := store.CreateUser(ctx, "owner@sso-identity.test", "hash")
	teamID, _ := store.GetActiveTenantID(ctx, ownerID)
	input := ResolveSSOUserInput{
		TeamID:       teamID,
		IssuerURL:    "https://id.example.com",
		Subject:      "subject-123",
		Email:        "new.user@example.com",
		GivenName:    "New",
		LastName:     "User",
		PasswordHash: "random-password-hash",
	}

	first, err := store.ResolveSSOUser(ctx, input)
	if err != nil {
		t.Fatalf("resolve new SSO user: %v", err)
	}
	if first.UserID == ownerID || !first.Created {
		t.Fatalf("expected a new SSO user, got %+v", first)
	}
	role, err := store.GetTenantRole(ctx, teamID, first.UserID)
	if err != nil || role != TenantRoleMember {
		t.Fatalf("expected member role, role=%q err=%v", role, err)
	}
	activeTeamID, err := store.GetActiveTenantID(ctx, first.UserID)
	if err != nil || activeTeamID != teamID {
		t.Fatalf("expected SSO team to be active, team=%s err=%v", activeTeamID, err)
	}

	input.Email = "renamed.user@example.com"
	second, err := store.ResolveSSOUser(ctx, input)
	if err != nil {
		t.Fatalf("resolve existing SSO identity: %v", err)
	}
	if second.UserID != first.UserID || second.Created {
		t.Fatalf("expected linked user %s, got %+v", first.UserID, second)
	}
}

func TestResolveSSOUserLinksExistingVerifiedEmail(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	ownerID, _ := store.CreateUser(ctx, "owner@sso-link.test", "hash")
	team, _ := store.CreateTenant(ctx, ownerID, "SSO team", "")
	existingID, _ := store.CreateUser(ctx, "existing@example.com", "local-password-hash")

	result, err := store.ResolveSSOUser(ctx, ResolveSSOUserInput{
		TeamID:       team.ID,
		IssuerURL:    "https://id.example.com",
		Subject:      "existing-subject",
		Email:        "EXISTING@example.com",
		GivenName:    "Existing",
		LastName:     "Person",
		PasswordHash: "unused-random-password-hash",
	})
	if err != nil {
		t.Fatalf("link existing user: %v", err)
	}
	if result.UserID != existingID || result.Created {
		t.Fatalf("expected existing user %s, got %+v", existingID, result)
	}
	existing, err := store.GetUserByID(ctx, existingID)
	if err != nil || existing == nil || existing.Password != "local-password-hash" || existing.GivenName != "Existing" || existing.LastName != "Person" {
		t.Fatalf("SSO link changed local credentials: user=%+v err=%v", existing, err)
	}
	role, err := store.GetTenantRole(ctx, team.ID, existingID)
	if err != nil || role != TenantRoleMember {
		t.Fatalf("expected linked team membership, role=%q err=%v", role, err)
	}
}

func TestResolveSSOUserDoesNotRelinkUserToDifferentSubject(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	ownerID, _ := store.CreateUser(ctx, "owner@sso-subject.test", "hash")
	teamID, _ := store.GetActiveTenantID(ctx, ownerID)
	input := ResolveSSOUserInput{
		TeamID:       teamID,
		IssuerURL:    "https://id.example.com",
		Subject:      "first-subject",
		Email:        "stable@example.com",
		PasswordHash: "random-password-hash",
	}
	if _, err := store.ResolveSSOUser(ctx, input); err != nil {
		t.Fatalf("link first subject: %v", err)
	}
	input.Subject = "different-subject"
	if _, err := store.ResolveSSOUser(ctx, input); err == nil {
		t.Fatal("expected a different subject for the linked user to fail closed")
	}

	var count int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM sso_identities WHERE tenant_id = ?", teamID).Scan(&count); err != nil {
		t.Fatalf("count identities: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one stable identity link, got %d", count)
	}
}
