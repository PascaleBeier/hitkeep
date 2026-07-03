package database

import (
	"context"
	"testing"
)

func TestBuildTeamUsageSummary(t *testing.T) {
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "usage-owner@test.dev", "hash")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	memberID, err := store.CreateUser(ctx, "usage-member@test.dev", "hash")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}

	team, err := store.CreateTenant(ctx, ownerID, "Usage Team", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, ownerID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	if err := store.AddTeamMember(ctx, team.ID, memberID, TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add member: %v", err)
	}

	_, err = store.CreateSite(ctx, ownerID, "usage-team.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if _, err := store.CreateTeamInvite(ctx, team.ID, "pending-usage@test.dev", TenantRoleMember, nil, ownerID, false); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	summary, err := store.BuildTeamUsageSummary(ctx, team.ID, store)
	if err != nil {
		t.Fatalf("build team usage summary: %v", err)
	}

	if summary.CurrentSites != 1 {
		t.Fatalf("expected 1 site, got %d", summary.CurrentSites)
	}
	if summary.CurrentMembers != 2 {
		t.Fatalf("expected 2 members, got %d", summary.CurrentMembers)
	}
	if summary.CurrentPendingInvites != 1 {
		t.Fatalf("expected 1 pending invite, got %d", summary.CurrentPendingInvites)
	}
}
