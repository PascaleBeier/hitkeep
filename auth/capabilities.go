package auth

import (
	"slices"
	"strings"
)

type Capability string

const (
	CapTeamViewMembers        Capability = "team.view_members"
	CapTeamManageMembers      Capability = "team.manage_members"
	CapTeamViewAudit          Capability = "team.view_audit"
	CapTeamManageSettings     Capability = "team.manage_settings"
	CapTeamManageAPIClients   Capability = "team.manage_api_clients"
	CapTeamManageIntegrations Capability = "team.manage_integrations"
	CapTeamTransferOwnership  Capability = "team.transfer_ownership"
	CapTeamArchive            Capability = "team.archive"
)

func InstanceCapabilities(role InstanceRole) []string {
	permissions := role.Permissions()
	capabilities := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		capabilities = append(capabilities, string(permission))
	}
	return capabilities
}

func SiteCapabilities(role SiteRole) []string {
	permissions := role.Permissions()
	capabilities := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		capabilities = append(capabilities, string(permission))
	}
	return capabilities
}

func TeamCapabilities(role string) []string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "owner":
		return []string{
			string(CapTeamViewMembers),
			string(CapTeamManageMembers),
			string(CapTeamViewAudit),
			string(CapTeamManageSettings),
			string(CapTeamManageAPIClients),
			string(CapTeamManageIntegrations),
			string(CapTeamTransferOwnership),
			string(CapTeamArchive),
		}
	case "admin":
		return []string{
			string(CapTeamViewMembers),
			string(CapTeamManageMembers),
			string(CapTeamViewAudit),
			string(CapTeamManageSettings),
			string(CapTeamManageAPIClients),
			string(CapTeamManageIntegrations),
		}
	case "member":
		return []string{string(CapTeamViewMembers)}
	default:
		return []string{}
	}
}

func TeamRoleHasCapability(role string, capability Capability) bool {
	return slices.Contains(TeamCapabilities(role), string(capability))
}
