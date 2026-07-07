package database

import (
	"context"
	"strings"
	"testing"

	"hitkeep/internal/api"
)

func TestCreateCustomTrackingDomainNormalizesAndEnforcesGlobalUniqueness(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "custom-domain-owner@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defaultTeamID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	otherTeam, err := store.CreateTenant(ctx, ownerID, "Other Domain Team", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	domain, err := store.CreateCustomTrackingDomain(ctx, CustomTrackingDomainInput{
		TeamID:  defaultTeamID,
		Host:    "Track.Example.COM.",
		TLSMode: string(api.CustomTrackingTLSModeCaddyOnDemand),
	})
	if err != nil {
		t.Fatalf("create custom tracking domain: %v", err)
	}
	if domain.Hostname != "track.example.com" {
		t.Fatalf("expected normalized hostname, got %q", domain.Hostname)
	}
	if domain.TLSMode != api.CustomTrackingTLSModeCaddyOnDemand {
		t.Fatalf("expected caddy-on-demand TLS mode, got %q", domain.TLSMode)
	}
	if domain.VerificationStatus != api.CustomTrackingDomainStatusPending || domain.TargetStatus != api.CustomTrackingDomainStatusPending || domain.TLSStatus != api.CustomTrackingDomainStatusPending {
		t.Fatalf("expected pending statuses, got verification=%q target=%q tls=%q", domain.VerificationStatus, domain.TargetStatus, domain.TLSStatus)
	}
	if domain.Active {
		t.Fatal("expected new domain not to be active before verification")
	}
	if domain.DNSTXTName != "_hitkeep-tracking.track.example.com" {
		t.Fatalf("expected DNS TXT name for normalized host, got %q", domain.DNSTXTName)
	}
	if !strings.HasPrefix(domain.DNSTXTValue, "hitkeep-domain-verification=") {
		t.Fatalf("expected DNS TXT value to include verification prefix, got %q", domain.DNSTXTValue)
	}

	if _, err := store.CreateCustomTrackingDomain(ctx, CustomTrackingDomainInput{
		TeamID:  otherTeam.ID,
		Host:    "track.example.com",
		TLSMode: string(api.CustomTrackingTLSModeExternal),
	}); err == nil {
		t.Fatal("expected globally duplicate custom tracking domain to be rejected")
	}
}

func TestListCustomTrackingDomainsIsTenantScoped(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	ownerID, err := store.CreateUser(ctx, "tenant-scoped-domain-owner@tenant.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defaultTeamID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	otherTeam, err := store.CreateTenant(ctx, ownerID, "Scoped Domain Team", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	defaultDomain, err := store.CreateCustomTrackingDomain(ctx, CustomTrackingDomainInput{TeamID: defaultTeamID, Host: "track-default.example.com"})
	if err != nil {
		t.Fatalf("create default custom tracking domain: %v", err)
	}
	otherDomain, err := store.CreateCustomTrackingDomain(ctx, CustomTrackingDomainInput{TeamID: otherTeam.ID, Host: "track-other.example.com"})
	if err != nil {
		t.Fatalf("create other custom tracking domain: %v", err)
	}

	defaultDomains, err := store.ListCustomTrackingDomains(ctx, defaultTeamID)
	if err != nil {
		t.Fatalf("list default domains: %v", err)
	}
	if len(defaultDomains) != 1 || defaultDomains[0].ID != defaultDomain.ID {
		t.Fatalf("expected default team to see only %s, got %+v", defaultDomain.ID, defaultDomains)
	}

	otherDomains, err := store.ListCustomTrackingDomains(ctx, otherTeam.ID)
	if err != nil {
		t.Fatalf("list other domains: %v", err)
	}
	if len(otherDomains) != 1 || otherDomains[0].ID != otherDomain.ID {
		t.Fatalf("expected other team to see only %s, got %+v", otherDomain.ID, otherDomains)
	}
}
