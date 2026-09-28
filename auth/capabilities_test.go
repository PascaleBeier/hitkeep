package auth

import (
	"reflect"
	"testing"
)

func TestInstanceCapabilitiesMirrorInstancePermissions(t *testing.T) {
	for _, role := range []InstanceRole{InstanceOwner, InstanceAdmin, InstanceUser, InstanceRole("unknown")} {
		if got, want := InstanceCapabilities(role), permissionsAsStrings(role.Permissions()); !reflect.DeepEqual(got, want) {
			t.Fatalf("InstanceCapabilities(%q) = %+v, want %+v", role, got, want)
		}
	}
}

func TestSiteCapabilitiesMirrorSitePermissions(t *testing.T) {
	for _, role := range []SiteRole{SiteOwner, SiteAdmin, SiteEditor, SiteViewer, SiteRole("unknown")} {
		if got, want := SiteCapabilities(role), permissionsAsStrings(role.Permissions()); !reflect.DeepEqual(got, want) {
			t.Fatalf("SiteCapabilities(%q) = %+v, want %+v", role, got, want)
		}
	}
}

func TestTeamCapabilities(t *testing.T) {
	tests := []struct {
		role string
		want []string
	}{
		{" owner ", []string{string(CapTeamViewMembers), string(CapTeamManageMembers), string(CapTeamViewAudit), string(CapTeamManageSettings), string(CapTeamManageAPIClients), string(CapTeamManageIntegrations), string(CapTeamTransferOwnership), string(CapTeamArchive)}},
		{"ADMIN", []string{string(CapTeamViewMembers), string(CapTeamManageMembers), string(CapTeamViewAudit), string(CapTeamManageSettings), string(CapTeamManageAPIClients), string(CapTeamManageIntegrations)}},
		{"member", []string{string(CapTeamViewMembers)}},
		{"viewer", []string{}},
	}

	for _, tc := range tests {
		if got := TeamCapabilities(tc.role); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("TeamCapabilities(%q) = %+v, want %+v", tc.role, got, tc.want)
		}
	}
}

func TestTeamRoleHasCapability(t *testing.T) {
	if !TeamRoleHasCapability("owner", CapTeamArchive) {
		t.Fatalf("expected owner to archive teams")
	}
	if TeamRoleHasCapability("admin", CapTeamArchive) {
		t.Fatalf("did not expect admin to archive teams")
	}
	if TeamRoleHasCapability("member", CapTeamManageMembers) {
		t.Fatalf("did not expect member to manage members")
	}
}

func permissionsAsStrings(perms []Permission) []string {
	out := make([]string, 0, len(perms))
	for _, perm := range perms {
		out = append(out, string(perm))
	}
	return out
}
