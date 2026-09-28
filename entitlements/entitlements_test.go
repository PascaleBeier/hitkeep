package entitlements

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestDefaultProviderReturnsUnlimited(t *testing.T) {
	provider := NewDefaultProvider()
	ent, err := provider.ForTenant(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("ForTenant: %v", err)
	}

	if ent.MaxTeams != 0 {
		t.Fatalf("expected MaxTeams=0 (unlimited), got %d", ent.MaxTeams)
	}
	if ent.MaxSitesPerTeam != 0 {
		t.Fatalf("expected MaxSitesPerTeam=0 (unlimited), got %d", ent.MaxSitesPerTeam)
	}
	if ent.MaxRetentionDays != 0 {
		t.Fatalf("expected MaxRetentionDays=0 (unlimited), got %d", ent.MaxRetentionDays)
	}
	if ent.MaxTeamMembers != 0 {
		t.Fatalf("expected MaxTeamMembers=0 (unlimited), got %d", ent.MaxTeamMembers)
	}
	if !ent.AllowSSO {
		t.Fatal("expected AllowSSO=true")
	}
	if !ent.AllowCustomBranding {
		t.Fatal("expected AllowCustomBranding=true")
	}
	if !ent.AllowExternalReportRecipients {
		t.Fatal("expected AllowExternalReportRecipients=true")
	}
}

func TestDefaultProviderImplementsInterface(t *testing.T) {
	var _ Provider = NewDefaultProvider()
}
