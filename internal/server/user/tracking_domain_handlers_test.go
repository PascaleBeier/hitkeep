package user

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	json "hitkeep/internal/jsonapi"
)

func TestRegisteredTrackingDomainRoutesAllowTeamAdminsAndOwners(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	ctx := context.Background()
	teamID, err := store.GetActiveTenantID(ctx, ownerID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	adminID, err := store.CreateUser(ctx, "tracking-domain-admin@example.test", "hash")
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if err := store.AddTeamMember(ctx, teamID, adminID, database.TenantRoleAdmin, ownerID); err != nil {
		t.Fatalf("add admin to team: %v", err)
	}

	tests := []struct {
		name     string
		userID   uuid.UUID
		hostname string
	}{
		{name: "owner", userID: ownerID, hostname: "Track-Owner.Example.Test"},
		{name: "admin", userID: adminID, hostname: "track-admin.example.test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := serveRegisteredCreateTrackingDomain(t, h, tt.userID, teamID, tt.hostname)
			if w.Code != http.StatusCreated {
				t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
			}

			var resp api.CustomTrackingDomain
			if err := json.UnmarshalRead(w.Body, &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.TeamID != teamID {
				t.Fatalf("expected team %s, got %s", teamID, resp.TeamID)
			}
			if resp.Hostname != database.NormalizeCustomTrackingHostname(tt.hostname) {
				t.Fatalf("expected normalized hostname %q, got %q", database.NormalizeCustomTrackingHostname(tt.hostname), resp.Hostname)
			}
		})
	}
}

func TestRegisteredTrackingDomainRoutesRejectTeamMembers(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	ctx := context.Background()
	teamID, err := store.GetActiveTenantID(ctx, ownerID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	memberID, err := store.CreateUser(ctx, "tracking-domain-member@example.test", "hash")
	if err != nil {
		t.Fatalf("create member user: %v", err)
	}
	if err := store.AddTeamMember(ctx, teamID, memberID, database.TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add member to team: %v", err)
	}

	w := serveRegisteredCreateTrackingDomain(t, h, memberID, teamID, "track-member.example.test")
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
	if domain, err := store.FindCustomTrackingDomainByHostname(ctx, "track-member.example.test"); err != nil {
		t.Fatalf("check tracking domain: %v", err)
	} else if domain != nil {
		t.Fatalf("expected forbidden request not to create tracking domain, got %+v", domain)
	}
}

func serveRegisteredCreateTrackingDomain(t *testing.T, h *handler, userID, teamID uuid.UUID, hostname string) *httptest.ResponseRecorder {
	t.Helper()

	body, _ := json.Marshal(api.CreateCustomTrackingDomainRequest{Hostname: hostname})
	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/user/teams/"+teamID.String()+"/tracking-domains", bytes.NewReader(body)), userID)
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	mux.ServeHTTP(w, req)
	return w
}

func TestCreateCustomTrackingDomainRequiresPaidCloudPlan(t *testing.T) {
	h, store, _ := setupUserSecurityTestEnv(t)
	defer store.Close()

	// The first user is the instance owner and therefore exempt from plan
	// limits, so plan gating is exercised with a regular customer team.
	ctx := context.Background()
	customerID, err := store.CreateUser(ctx, "tracking-domain-customer@example.test", "hash")
	if err != nil {
		t.Fatalf("create customer user: %v", err)
	}
	customerTeam, err := store.CreateTenant(ctx, customerID, "Customer Team", "")
	if err != nil {
		t.Fatalf("create customer team: %v", err)
	}
	teamID := customerTeam.ID

	h.ctx.Config.CloudHosted = true
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{}, entitlements.PlanInfo{Code: "free", Name: "Free"})

	if w := serveRegisteredCreateTrackingDomain(t, h, customerID, teamID, "blocked.example.test"); w.Code != http.StatusForbidden {
		t.Fatalf("expected free cloud plan to be blocked with %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}

	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{}, entitlements.PlanInfo{Code: "pro", Name: "Pro"})

	if w := serveRegisteredCreateTrackingDomain(t, h, customerID, teamID, "allowed.example.test"); w.Code != http.StatusCreated {
		t.Fatalf("expected pro cloud plan to create with %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	// Self-hosted deployments are never plan-gated.
	h.ctx.Config.CloudHosted = false
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{}, entitlements.PlanInfo{Code: "free", Name: "Free"})

	if w := serveRegisteredCreateTrackingDomain(t, h, customerID, teamID, "selfhosted.example.test"); w.Code != http.StatusCreated {
		t.Fatalf("expected self-hosted create with %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}
}

func TestCreateCustomTrackingDomainExemptsInstanceStaff(t *testing.T) {
	h, store, instanceOwnerID := setupUserSecurityTestEnv(t)
	defer store.Close()

	ctx := context.Background()
	teamID, err := store.GetActiveTenantID(ctx, instanceOwnerID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	if role, err := store.GetInstanceRole(ctx, instanceOwnerID); err != nil || role != auth.InstanceOwner {
		t.Fatalf("expected first user to be instance owner, got %q (err %v)", role, err)
	}

	h.ctx.Config.CloudHosted = true
	h.ctx.Entitlements = entitlements.NewStaticProvider(entitlements.Entitlements{}, entitlements.PlanInfo{Code: "free", Name: "Free"})

	// Instance owners are never plan-gated, even on the free plan.
	if w := serveRegisteredCreateTrackingDomain(t, h, instanceOwnerID, teamID, "operator.example.test"); w.Code != http.StatusCreated {
		t.Fatalf("expected instance owner create with %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	// Instance admins bypass plan limits as well.
	instanceAdminID, err := store.CreateUser(ctx, "tracking-domain-instance-admin@example.test", "hash")
	if err != nil {
		t.Fatalf("create instance admin user: %v", err)
	}
	if err := store.AddTeamMember(ctx, teamID, instanceAdminID, database.TenantRoleAdmin, instanceOwnerID); err != nil {
		t.Fatalf("add instance admin to team: %v", err)
	}
	if err := store.UpdateInstanceRole(ctx, instanceAdminID, auth.InstanceAdmin, instanceOwnerID); err != nil {
		t.Fatalf("grant instance admin role: %v", err)
	}

	if w := serveRegisteredCreateTrackingDomain(t, h, instanceAdminID, teamID, "instance-admin.example.test"); w.Code != http.StatusCreated {
		t.Fatalf("expected instance admin create with %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	// Team admins without an instance role stay plan-gated, even on a team
	// owned by the instance owner.
	teamAdminID, err := store.CreateUser(ctx, "tracking-domain-team-admin@example.test", "hash")
	if err != nil {
		t.Fatalf("create team admin user: %v", err)
	}
	if err := store.AddTeamMember(ctx, teamID, teamAdminID, database.TenantRoleAdmin, instanceOwnerID); err != nil {
		t.Fatalf("add team admin to team: %v", err)
	}

	if w := serveRegisteredCreateTrackingDomain(t, h, teamAdminID, teamID, "team-admin.example.test"); w.Code != http.StatusForbidden {
		t.Fatalf("expected non-staff team admin to be blocked with %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
}
