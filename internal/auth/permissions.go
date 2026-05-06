package auth

import "slices"

type InstanceRole string

const (
	InstanceOwner InstanceRole = "owner"
	InstanceAdmin InstanceRole = "admin"
	InstanceUser  InstanceRole = "user"
)

type SiteRole string

const (
	SiteOwner  SiteRole = "owner"
	SiteAdmin  SiteRole = "admin"
	SiteEditor SiteRole = "editor"
	SiteViewer SiteRole = "viewer"
)

// Permission matrix
type Permission string

const (
	// Instance permissions
	PermInstanceManageUsers          Permission = "instance.manage_users"
	PermInstanceViewAllSites         Permission = "instance.view_all_sites"
	PermInstanceManageSettings       Permission = "instance.manage_settings"
	PermInstanceViewSystem           Permission = "instance.view_system"
	PermInstanceViewActivation       Permission = "instance.view_activation"
	PermInstanceManageSystem         Permission = "instance.manage_system"
	PermInstanceRunMaintenance       Permission = "instance.run_maintenance"
	PermInstanceViewAudit            Permission = "instance.view_audit"
	PermInstanceExportAudit          Permission = "instance.export_audit"
	PermInstanceManageSiteExclusions Permission = "instance.manage_site_exclusions"
	PermInstanceManageImports        Permission = "instance.manage_imports"

	// Site permissions
	PermSiteView        Permission = "site.view"
	PermSiteManageData  Permission = "site.manage_data"
	PermSiteManageGoals Permission = "site.manage_goals"
	PermSiteManageTeam  Permission = "site.manage_team"
	PermSiteDelete      Permission = "site.delete"
)

// Role to permissions mapping
var instancePermissions = map[InstanceRole][]Permission{
	InstanceOwner: {
		PermInstanceManageUsers,
		PermInstanceViewAllSites,
		PermInstanceManageSettings,
		PermInstanceViewSystem,
		PermInstanceViewActivation,
		PermInstanceManageSystem,
		PermInstanceRunMaintenance,
		PermInstanceViewAudit,
		PermInstanceExportAudit,
		PermInstanceManageSiteExclusions,
		PermInstanceManageImports,
		PermSiteView,
		PermSiteManageData,
		PermSiteManageGoals,
		PermSiteManageTeam,
		PermSiteDelete,
	},
	InstanceAdmin: {
		PermInstanceViewAllSites,
		PermInstanceViewSystem,
		PermInstanceRunMaintenance,
		PermInstanceViewAudit,
		PermInstanceManageSiteExclusions,
		PermInstanceManageImports,
		PermSiteView,
	},
	InstanceUser: {},
}

var sitePermissions = map[SiteRole][]Permission{
	SiteOwner: {
		PermSiteView,
		PermSiteManageData,
		PermSiteManageGoals,
		PermSiteManageTeam,
		PermSiteDelete,
	},
	SiteAdmin: {
		PermSiteView,
		PermSiteManageData,
		PermSiteManageGoals,
		PermSiteManageTeam,
	},
	SiteEditor: {
		PermSiteView,
		PermSiteManageGoals,
	},
	SiteViewer: {
		PermSiteView,
	},
}

func (r InstanceRole) HasPermission(perm Permission) bool {
	perms, ok := instancePermissions[r]
	if !ok {
		return false
	}
	return slices.Contains(perms, perm)
}

func (r InstanceRole) Permissions() []Permission {
	return slices.Clone(instancePermissions[r])
}

func (r SiteRole) HasPermission(perm Permission) bool {
	perms, ok := sitePermissions[r]
	if !ok {
		return false
	}
	return slices.Contains(perms, perm)
}

func (r SiteRole) Permissions() []Permission {
	return slices.Clone(sitePermissions[r])
}

func IsValidInstanceRole(role InstanceRole) bool {
	switch role {
	case InstanceOwner, InstanceAdmin, InstanceUser:
		return true
	default:
		return false
	}
}

func IsValidSiteRole(role SiteRole) bool {
	switch role {
	case SiteOwner, SiteAdmin, SiteEditor, SiteViewer:
		return true
	default:
		return false
	}
}

func MinInstanceRole(a, b InstanceRole) InstanceRole {
	if instanceRoleRank(a) >= instanceRoleRank(b) {
		return a
	}
	return b
}

func MinSiteRole(a, b SiteRole) SiteRole {
	if siteRoleRank(a) >= siteRoleRank(b) {
		return a
	}
	return b
}

func CanAssignInstanceRole(actor, requested InstanceRole) bool {
	if !IsValidInstanceRole(actor) || !IsValidInstanceRole(requested) {
		return false
	}
	return instanceRoleRank(requested) >= instanceRoleRank(actor)
}

func CanAssignSiteRole(actor, requested SiteRole) bool {
	if !IsValidSiteRole(actor) || !IsValidSiteRole(requested) {
		return false
	}
	return siteRoleRank(requested) >= siteRoleRank(actor)
}

func instanceRoleRank(role InstanceRole) int {
	switch role {
	case InstanceOwner:
		return 0
	case InstanceAdmin:
		return 1
	case InstanceUser:
		return 2
	default:
		return 99
	}
}

func siteRoleRank(role SiteRole) int {
	switch role {
	case SiteOwner:
		return 0
	case SiteAdmin:
		return 1
	case SiteEditor:
		return 2
	case SiteViewer:
		return 3
	default:
		return 99
	}
}
