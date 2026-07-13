package entitlements_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/auth"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
)

type serviceEnv struct {
	store   *database.Store
	cfg     *config.Config
	ownerID uuid.UUID
	teamID  uuid.UUID
}

func newServiceEnv(t *testing.T) serviceEnv {
	t.Helper()

	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	ownerID, err := store.CreateUser(context.Background(), "owner@example.test", "hash")
	if err != nil {
		t.Fatalf("create instance owner: %v", err)
	}
	teamID, err := store.GetActiveTenantID(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("resolve default team: %v", err)
	}

	return serviceEnv{
		store:   store,
		cfg:     &config.Config{CloudHosted: true},
		ownerID: ownerID,
		teamID:  teamID,
	}
}

func (e serviceEnv) service(provider entitlements.Provider) *entitlements.Service {
	return entitlements.NewService(e.store, provider, e.cfg)
}

func (e serviceEnv) createMember(t *testing.T, email string) uuid.UUID {
	t.Helper()
	userID, err := e.store.CreateUser(context.Background(), email, "hash")
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return userID
}

func freePlanProvider() entitlements.Provider {
	return entitlements.NewStaticProvider(entitlements.Entitlements{}, entitlements.PlanInfo{Code: entitlements.PlanCodeFree, Name: "Free"})
}

func TestServiceBypassesCloudLimits(t *testing.T) {
	env := newServiceEnv(t)
	svc := env.service(freePlanProvider())
	ctx := context.Background()

	memberID := env.createMember(t, "member@example.test")
	adminID := env.createMember(t, "instance-admin@example.test")
	if err := env.store.UpdateInstanceRole(ctx, adminID, auth.InstanceAdmin, env.ownerID); err != nil {
		t.Fatalf("grant instance admin: %v", err)
	}

	tests := []struct {
		name   string
		userID uuid.UUID
		want   bool
	}{
		{name: "instance owner", userID: env.ownerID, want: true},
		{name: "instance admin", userID: adminID, want: true},
		{name: "regular user", userID: memberID, want: false},
		{name: "anonymous", userID: uuid.Nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := svc.BypassesCloudLimits(ctx, tt.userID); got != tt.want {
				t.Fatalf("BypassesCloudLimits(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}

	env.cfg.CloudHosted = false
	if svc.BypassesCloudLimits(ctx, env.ownerID) {
		t.Fatalf("expected no bypass on self-hosted deployments")
	}
}

func TestServiceTeamEntitlements(t *testing.T) {
	env := newServiceEnv(t)
	ctx := context.Background()

	// Without a provider, teams get the permissive defaults.
	ent := env.service(nil).TeamEntitlements(ctx, env.teamID)
	if ent == nil || !ent.AllowSSO || !ent.AllowCustomBranding || ent.MaxTeamMembers != 0 {
		t.Fatalf("expected permissive default entitlements, got %+v", ent)
	}

	provider := entitlements.NewStaticProvider(entitlements.Entitlements{MaxTeamMembers: 3, MaxSitesPerTeam: 5}, entitlements.PlanInfo{})
	ent = env.service(provider).TeamEntitlements(ctx, env.teamID)
	if ent == nil || ent.MaxTeamMembers != 3 || ent.MaxSitesPerTeam != 5 {
		t.Fatalf("expected provider entitlements, got %+v", ent)
	}
}

func TestServiceTeamPlan(t *testing.T) {
	env := newServiceEnv(t)
	ctx := context.Background()

	if plan := env.service(nil).TeamPlan(ctx, env.teamID); plan != nil {
		t.Fatalf("expected no plan without a describing provider, got %+v", plan)
	}
	if plan := env.service(entitlements.NewStaticProvider(entitlements.Entitlements{}, entitlements.PlanInfo{})).TeamPlan(ctx, env.teamID); plan != nil {
		t.Fatalf("expected no plan for empty plan info, got %+v", plan)
	}

	plan := env.service(freePlanProvider()).TeamPlan(ctx, env.teamID)
	if plan == nil || plan.Code != entitlements.PlanCodeFree || plan.Name != "Free" {
		t.Fatalf("expected free plan, got %+v", plan)
	}
}

func TestServiceAllowsCustomTrackingDomains(t *testing.T) {
	env := newServiceEnv(t)
	ctx := context.Background()
	memberID := env.createMember(t, "member@example.test")

	svc := env.service(freePlanProvider())
	if svc.AllowsCustomTrackingDomains(ctx, memberID, env.teamID) {
		t.Fatalf("expected free cloud plan to block custom tracking domains")
	}
	if !svc.AllowsCustomTrackingDomains(ctx, env.ownerID, env.teamID) {
		t.Fatalf("expected instance owner to bypass the plan gate")
	}

	pro := entitlements.NewStaticProvider(entitlements.Entitlements{}, entitlements.PlanInfo{Code: "pro", Name: "Pro"})
	if !env.service(pro).AllowsCustomTrackingDomains(ctx, memberID, env.teamID) {
		t.Fatalf("expected pro plan to allow custom tracking domains")
	}

	env.cfg.CloudHosted = false
	if !svc.AllowsCustomTrackingDomains(ctx, memberID, env.teamID) {
		t.Fatalf("expected self-hosted deployments to allow custom tracking domains")
	}
}

func TestServiceAllowsSSO(t *testing.T) {
	env := newServiceEnv(t)
	ctx := context.Background()
	memberID := env.createMember(t, "sso-member@example.test")

	withoutSSO := entitlements.NewStaticProvider(entitlements.Entitlements{}, entitlements.PlanInfo{Code: "pro", Name: "Pro"})
	if env.service(withoutSSO).AllowsSSO(ctx, memberID, env.teamID) {
		t.Fatal("expected a cloud plan without the SSO entitlement to be blocked")
	}

	withSSO := entitlements.NewStaticProvider(entitlements.Entitlements{AllowSSO: true}, entitlements.PlanInfo{Code: "business", Name: "Business"})
	if !env.service(withSSO).AllowsSSO(ctx, memberID, env.teamID) {
		t.Fatal("expected a cloud plan with the SSO entitlement to be allowed")
	}
	if !env.service(withoutSSO).AllowsSSO(ctx, env.ownerID, env.teamID) {
		t.Fatal("expected the instance owner to bypass the SSO plan gate")
	}

	env.cfg.CloudHosted = false
	if !env.service(withoutSSO).AllowsSSO(ctx, memberID, env.teamID) {
		t.Fatal("expected self-hosted deployments to allow SSO")
	}
}

func TestServiceRequireTeamMemberCapacity(t *testing.T) {
	env := newServiceEnv(t)
	ctx := context.Background()

	// The default team holds one member (the instance owner).
	limited := env.service(entitlements.NewStaticProvider(entitlements.Entitlements{MaxTeamMembers: 1}, entitlements.PlanInfo{}))
	if err := limited.RequireTeamMemberCapacity(ctx, env.teamID); !errors.Is(err, entitlements.ErrTeamMemberLimitReached) {
		t.Fatalf("expected ErrTeamMemberLimitReached, got %v", err)
	}

	roomy := env.service(entitlements.NewStaticProvider(entitlements.Entitlements{MaxTeamMembers: 2}, entitlements.PlanInfo{}))
	if err := roomy.RequireTeamMemberCapacity(ctx, env.teamID); err != nil {
		t.Fatalf("expected capacity below the limit, got %v", err)
	}

	unlimited := env.service(entitlements.NewStaticProvider(entitlements.Entitlements{}, entitlements.PlanInfo{}))
	if err := unlimited.RequireTeamMemberCapacity(ctx, env.teamID); err != nil {
		t.Fatalf("expected zero limit to mean unlimited, got %v", err)
	}
}

func TestServiceCanCreateTeam(t *testing.T) {
	env := newServiceEnv(t)
	ctx := context.Background()
	// The member already belongs to one team (the default team).
	memberID := env.createMember(t, "member@example.test")

	capped := env.service(entitlements.NewStaticProvider(entitlements.Entitlements{MaxTeams: 1}, entitlements.PlanInfo{Code: entitlements.PlanCodeFree, Name: "Free"}))
	if err := capped.CanCreateTeam(ctx, memberID); !errors.Is(err, entitlements.ErrTeamLimitReached) {
		t.Fatalf("expected ErrTeamLimitReached at the team cap, got %v", err)
	}
	if err := capped.CanCreateTeam(ctx, env.ownerID); err != nil {
		t.Fatalf("expected instance owner to bypass the team cap, got %v", err)
	}

	// Cloud users may create teams up to their MaxTeams entitlement.
	roomy := env.service(entitlements.NewStaticProvider(entitlements.Entitlements{MaxTeams: 2}, entitlements.PlanInfo{Code: entitlements.PlanCodeFree, Name: "Free"}))
	if err := roomy.CanCreateTeam(ctx, memberID); err != nil {
		t.Fatalf("expected member below the team cap to create teams, got %v", err)
	}

	// Zero means unlimited; no provider means no limits at all.
	if err := env.service(freePlanProvider()).CanCreateTeam(ctx, memberID); err != nil {
		t.Fatalf("expected unlimited teams without a MaxTeams entitlement, got %v", err)
	}
	if err := env.service(nil).CanCreateTeam(ctx, memberID); err != nil {
		t.Fatalf("expected no limit without a provider, got %v", err)
	}

	// Self-hosted deployments honor the cap the same way.
	env.cfg.CloudHosted = false
	if err := capped.CanCreateTeam(ctx, memberID); !errors.Is(err, entitlements.ErrTeamLimitReached) {
		t.Fatalf("expected self-hosted team cap to apply, got %v", err)
	}
}

func TestServiceRequireTeamMembershipCapacity(t *testing.T) {
	env := newServiceEnv(t)
	ctx := context.Background()

	memberID := env.createMember(t, "member@example.test")
	if _, err := env.store.CreateTenant(ctx, memberID, "Member Team", ""); err != nil {
		t.Fatalf("create member team: %v", err)
	}

	capped := env.service(entitlements.NewStaticProvider(entitlements.Entitlements{MaxTeams: 1}, entitlements.PlanInfo{Code: entitlements.PlanCodeFree, Name: "Free"}))
	if err := capped.RequireTeamMembershipCapacity(ctx, memberID, 1); !errors.Is(err, entitlements.ErrTeamMembershipLimitReached) {
		t.Fatalf("expected ErrTeamMembershipLimitReached at the membership cap, got %v", err)
	}

	roomy := env.service(entitlements.NewStaticProvider(entitlements.Entitlements{MaxTeams: 2}, entitlements.PlanInfo{Code: entitlements.PlanCodeFree, Name: "Free"}))
	if err := roomy.RequireTeamMembershipCapacity(ctx, memberID, 1); err != nil {
		t.Fatalf("expected membership below the cap, got %v", err)
	}

	freshID := env.createMember(t, "fresh@example.test")
	if err := capped.RequireTeamMembershipCapacity(ctx, freshID, 1); err != nil {
		t.Fatalf("expected fresh user to join a first team, got %v", err)
	}

	// Instance staff are exempt from membership caps.
	if _, err := env.store.CreateTenant(ctx, env.ownerID, "Owner Team", ""); err != nil {
		t.Fatalf("create owner team: %v", err)
	}
	if err := capped.RequireTeamMembershipCapacity(ctx, env.ownerID, 1); err != nil {
		t.Fatalf("expected instance owner to bypass the membership cap, got %v", err)
	}

	// Membership caps are a managed-cloud concept only.
	env.cfg.CloudHosted = false
	if err := capped.RequireTeamMembershipCapacity(ctx, memberID, 1); err != nil {
		t.Fatalf("expected no membership cap on self-hosted deployments, got %v", err)
	}
}
