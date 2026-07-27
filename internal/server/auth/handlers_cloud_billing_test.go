//go:build billing

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
)

func TestHandleAcceptInviteRejectsSecondHostedCloudTeam(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()

	h.ctx.Config.CloudHosted = true
	// Mirrors the managed-cloud default of HITKEEP_CLOUD_MAX_TEAMS=1.
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{MaxTeams: 1}, entitlements.PlanInfo{})

	ownerHash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("hash owner password: %v", err)
	}
	ownerID, err := store.CreateUser(context.Background(), "owner-cloud@example.com", ownerHash)
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	inviteeHash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("hash invitee password: %v", err)
	}
	inviteeID, err := store.CreateUserWithoutDefaultTenant(context.Background(), "invite-cloud@example.com", inviteeHash)
	if err != nil {
		t.Fatalf("create invitee user: %v", err)
	}

	existingTeam, err := store.CreateTenant(context.Background(), inviteeID, "Existing Cloud Team", "")
	if err != nil {
		t.Fatalf("create existing team: %v", err)
	}
	_ = existingTeam

	targetTeam, err := store.CreateTenant(context.Background(), ownerID, "Target Cloud Team", "")
	if err != nil {
		t.Fatalf("create target team: %v", err)
	}
	targetTeamID := targetTeam.ID
	if _, err := store.CreateTeamInvite(context.Background(), targetTeamID, "invite-cloud@example.com", database.TenantRoleAdmin, &inviteeID, ownerID, true); err != nil {
		t.Fatalf("create team invite: %v", err)
	}

	token, err := store.CreatePasswordResetToken(context.Background(), "invite-cloud@example.com")
	if err != nil {
		t.Fatalf("create password reset token: %v", err)
	}

	body, err := json.Marshal(map[string]string{
		"token":    token,
		"password": "new-password-123",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/accept-invite", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleAcceptInvite().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}

	isMember, err := store.IsTenantMember(context.Background(), targetTeamID, inviteeID)
	if err != nil {
		t.Fatalf("check target team membership: %v", err)
	}
	if isMember {
		t.Fatalf("expected invitee not to join second hosted cloud team")
	}

	user, err := store.GetUserByID(context.Background(), inviteeID)
	if err != nil {
		t.Fatalf("load invitee after rejected invite: %v", err)
	}
	passwordStillWorks, err := verifyPassword("password123", user.Password)
	if err != nil {
		t.Fatalf("verify invitee password after rejected invite: %v", err)
	}
	if !passwordStillWorks {
		t.Fatalf("expected rejected invite to leave invitee password unchanged")
	}

	email, err := store.ResolvePasswordResetEmail(context.Background(), token)
	if err != nil {
		t.Fatalf("expected rejected invite to preserve token: %v", err)
	}
	if email != "invite-cloud@example.com" {
		t.Fatalf("expected preserved token email invite-cloud@example.com, got %q", email)
	}
}
