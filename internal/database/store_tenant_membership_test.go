package database

import (
	"context"
	"testing"

	"hitkeep/internal/auth"
)

func TestAcceptInviteWithPasswordSetsActiveTenantToAcceptedTeam(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "invite-owner@example.com", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	team, err := store.CreateTenant(ctx, ownerID, "Password Invite Team", "")
	if err != nil {
		t.Fatalf("create invite team: %v", err)
	}
	inviteeID, err := store.CreateUserWithoutDefaultTenant(ctx, "password-site-invite@example.com", "temporary-hash")
	if err != nil {
		t.Fatalf("create placeholder user: %v", err)
	}
	if _, err := store.CreateTeamInvite(ctx, team.ID, "password-site-invite@example.com", TenantRoleMember, &inviteeID, ownerID, true); err != nil {
		t.Fatalf("create team invite: %v", err)
	}
	token, err := store.CreatePasswordResetToken(ctx, "password-site-invite@example.com")
	if err != nil {
		t.Fatalf("create invite token: %v", err)
	}

	_, acceptedUserID, acceptedInvites, err := store.AcceptInviteWithPassword(ctx, token, "new-hash")
	if err != nil {
		t.Fatalf("accept invite with password: %v", err)
	}
	if acceptedUserID != inviteeID {
		t.Fatalf("expected accepted user %s, got %s", inviteeID, acceptedUserID)
	}
	if len(acceptedInvites) != 1 || acceptedInvites[0].TeamID != team.ID {
		t.Fatalf("expected accepted invite for team %s, got %+v", team.ID, acceptedInvites)
	}
	activeTenantID, err := store.GetActiveTenantID(ctx, inviteeID)
	if err != nil {
		t.Fatalf("get active tenant: %v", err)
	}
	if activeTenantID != team.ID {
		t.Fatalf("expected active tenant %s, got %s", team.ID, activeTenantID)
	}
}

func TestAcceptInviteForAuthenticatedUserSetsActiveTenantToAcceptedTeam(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "existing-invite-owner@example.com", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	inviteeID, err := store.CreateUser(ctx, "existing-site-invite@example.com", "existing-hash")
	if err != nil {
		t.Fatalf("create existing user: %v", err)
	}
	defaultTenantID, err := store.GetActiveTenantID(ctx, inviteeID)
	if err != nil {
		t.Fatalf("get default active tenant: %v", err)
	}
	team, err := store.CreateTenant(ctx, ownerID, "Authenticated Invite Team", "")
	if err != nil {
		t.Fatalf("create invite team: %v", err)
	}
	if _, err := store.CreateTeamInvite(ctx, team.ID, "existing-site-invite@example.com", TenantRoleMember, &inviteeID, ownerID, false); err != nil {
		t.Fatalf("create team invite: %v", err)
	}
	token, err := store.CreatePasswordResetToken(ctx, "existing-site-invite@example.com")
	if err != nil {
		t.Fatalf("create invite token: %v", err)
	}

	_, acceptedInvites, err := store.AcceptInviteForAuthenticatedUser(ctx, token, inviteeID)
	if err != nil {
		t.Fatalf("accept invite for authenticated user: %v", err)
	}
	if len(acceptedInvites) != 1 || acceptedInvites[0].TeamID != team.ID {
		t.Fatalf("expected accepted invite for team %s, got %+v", team.ID, acceptedInvites)
	}
	activeTenantID, err := store.GetActiveTenantID(ctx, inviteeID)
	if err != nil {
		t.Fatalf("get active tenant: %v", err)
	}
	if activeTenantID == defaultTenantID {
		t.Fatalf("expected active tenant to change from default %s", defaultTenantID)
	}
	if activeTenantID != team.ID {
		t.Fatalf("expected active tenant %s, got %s", team.ID, activeTenantID)
	}
}

func TestAcceptTeamInviteForSSOPreservesRoleAndConsumesOnlyTheTargetInvite(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "sso-invite-owner@example.com", "hash")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	firstTeam, err := store.CreateTenant(ctx, ownerID, "SSO Admin Team", "")
	if err != nil {
		t.Fatalf("create first team: %v", err)
	}
	secondTeam, err := store.CreateTenant(ctx, ownerID, "Other Pending Team", "")
	if err != nil {
		t.Fatalf("create second team: %v", err)
	}
	inviteeID, err := store.CreateUserWithoutDefaultTenant(ctx, "sso-invitee@example.com", "temporary-hash")
	if err != nil {
		t.Fatalf("create invitee: %v", err)
	}
	if _, err := store.CreateTeamInvite(ctx, firstTeam.ID, "sso-invitee@example.com", TenantRoleAdmin, &inviteeID, ownerID, true); err != nil {
		t.Fatalf("create admin invite: %v", err)
	}
	if _, err := store.CreateTeamInvite(ctx, secondTeam.ID, "sso-invitee@example.com", TenantRoleMember, &inviteeID, ownerID, true); err != nil {
		t.Fatalf("create second invite: %v", err)
	}
	token, err := store.CreatePasswordResetToken(ctx, "sso-invitee@example.com")
	if err != nil {
		t.Fatalf("create invite token: %v", err)
	}

	accepted, err := store.AcceptTeamInviteForSSO(ctx, token, firstTeam.ID, inviteeID)
	if err != nil {
		t.Fatalf("accept SSO invite: %v", err)
	}
	if accepted.TeamID != firstTeam.ID || accepted.Role != TenantRoleAdmin {
		t.Fatalf("expected scoped admin invite, got %+v", accepted)
	}
	role, err := store.GetTenantRole(ctx, firstTeam.ID, inviteeID)
	if err != nil || role != TenantRoleAdmin {
		t.Fatalf("expected retained admin role, role=%q err=%v", role, err)
	}
	pending, err := store.ListPendingTeamInvitesByEmail(ctx, "sso-invitee@example.com")
	if err != nil || len(pending) != 1 || pending[0].TeamID != secondTeam.ID {
		t.Fatalf("expected only the other invite to remain pending, invites=%+v err=%v", pending, err)
	}
	if _, err := store.ResolvePasswordResetEmail(ctx, token); err == nil {
		t.Fatal("expected accepted SSO invite token to be consumed")
	}
}

func TestAcceptTeamInviteForSSOAllowsVerifiedEmailWithoutToken(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	ownerID, _ := store.CreateUser(ctx, "sso-email-owner@example.com", "hash")
	team, _ := store.CreateTenant(ctx, ownerID, "SSO Email Team", "")
	inviteeID, _ := store.CreateUserWithoutDefaultTenant(ctx, "verified-sso@example.com", "temporary-hash")
	if _, err := store.CreateTeamInvite(ctx, team.ID, "verified-sso@example.com", TenantRoleMember, &inviteeID, ownerID, true); err != nil {
		t.Fatalf("create invite: %v", err)
	}
	token, err := store.CreatePasswordResetToken(ctx, "verified-sso@example.com")
	if err != nil {
		t.Fatalf("create invite token: %v", err)
	}

	accepted, err := store.AcceptTeamInviteForSSO(ctx, "", team.ID, inviteeID)
	if err != nil || accepted.TeamID != team.ID {
		t.Fatalf("accept invite after verified SSO email: invite=%+v err=%v", accepted, err)
	}
	if _, err := store.ResolvePasswordResetEmail(ctx, token); err == nil {
		t.Fatal("expected the password setup token to be invalidated after verified SSO acceptance")
	}
}

func TestAcceptedTeamInviteMakesPendingSiteAccessEffective(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "site-invite-owner@example.com", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	inviteeID, err := store.CreateUser(ctx, "pending-site-access@example.com", "hash")
	if err != nil {
		t.Fatalf("create invitee user: %v", err)
	}
	team, err := store.CreateTenant(ctx, ownerID, "Pending Site Access Team", "")
	if err != nil {
		t.Fatalf("create invite team: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, ownerID, team.ID); err != nil {
		t.Fatalf("set owner active tenant: %v", err)
	}
	site, err := store.CreateSite(ctx, ownerID, "pending-site-access.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.AddPendingSiteMemberInviteAccess(ctx, site.ID, inviteeID, auth.SiteViewer, ownerID); err != nil {
		t.Fatalf("add pending site member access: %v", err)
	}
	if _, err := store.CreateTeamInvite(ctx, team.ID, "pending-site-access@example.com", TenantRoleMember, &inviteeID, ownerID, false); err != nil {
		t.Fatalf("create team invite: %v", err)
	}
	token, err := store.CreatePasswordResetToken(ctx, "pending-site-access@example.com")
	if err != nil {
		t.Fatalf("create invite token: %v", err)
	}

	if _, _, err := store.AcceptInviteForAuthenticatedUser(ctx, token, inviteeID); err != nil {
		t.Fatalf("accept invite for authenticated user: %v", err)
	}
	role, err := store.GetSiteRole(ctx, inviteeID, site.ID)
	if err != nil {
		t.Fatalf("get accepted site role: %v", err)
	}
	if role != auth.SiteViewer {
		t.Fatalf("expected effective site role %q, got %q", auth.SiteViewer, role)
	}
}
