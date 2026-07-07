package access

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
)

type Builder struct {
	Store *database.Store
	// Limits derives plan-limit capabilities such as CanCreateTeams. A nil
	// Limits is permissive, matching deployments without managed-cloud limits.
	Limits *entitlements.Service
}

func (b Builder) ForUser(ctx context.Context, userID uuid.UUID) (api.PermissionContext, error) {
	sites, err := b.Store.GetSites(ctx, userID)
	if err != nil {
		return api.PermissionContext{}, fmt.Errorf("list sites: %w", err)
	}
	return b.ForUserSites(ctx, userID, sites)
}

func (b Builder) ForUserSites(ctx context.Context, userID uuid.UUID, sites []api.Site) (api.PermissionContext, error) {
	instanceRole, err := b.Store.GetInstanceRole(ctx, userID)
	if err != nil {
		return api.PermissionContext{}, fmt.Errorf("get instance role: %w", err)
	}

	siteRoles, siteCapabilities, err := b.siteAccess(ctx, userID, sites, instanceRole)
	if err != nil {
		return api.PermissionContext{}, err
	}
	activeTeamID, activeTeamRole := b.activeTeamContext(ctx, userID)
	var activeTeamIDValue *uuid.UUID
	if activeTeamID != uuid.Nil {
		activeTeamIDValue = &activeTeamID
	}
	instanceCapabilities := authcore.InstanceCapabilities(instanceRole)

	return api.PermissionContext{
		InstanceRole:           string(instanceRole),
		Permissions:            siteRoles,
		InstancePermissions:    instanceCapabilities,
		InstanceCapabilities:   instanceCapabilities,
		SiteCapabilities:       siteCapabilities,
		ActiveTeamID:           activeTeamIDValue,
		ActiveTeamRole:         activeTeamRole,
		ActiveTeamCapabilities: authcore.TeamCapabilities(activeTeamRole),
		CanCreateTeams:         b.canCreateTeams(ctx, userID),
	}, nil
}

func (b Builder) canCreateTeams(ctx context.Context, userID uuid.UUID) bool {
	if b.Limits == nil {
		return true
	}
	return b.Limits.CanCreateTeam(ctx, userID) == nil
}

func (b Builder) siteAccess(ctx context.Context, userID uuid.UUID, sites []api.Site, instanceRole authcore.InstanceRole) (map[string]string, map[string][]string, error) {
	siteRoles := map[string]string{}
	siteCapabilities := map[string][]string{}
	for _, site := range sites {
		role, err := b.Store.GetSiteRole(ctx, userID, site.ID)
		if err != nil {
			if !instanceRole.HasPermission(authcore.PermInstanceViewAllSites) {
				return nil, nil, fmt.Errorf("resolve site role %s: %w", site.ID, err)
			}
			continue
		}
		siteID := site.ID.String()
		siteRoles[siteID] = string(role)
		siteCapabilities[siteID] = authcore.SiteCapabilities(role)
	}
	return siteRoles, siteCapabilities, nil
}

func (b Builder) activeTeamContext(ctx context.Context, userID uuid.UUID) (uuid.UUID, string) {
	activeTeamID, err := b.Store.GetActiveTenantID(ctx, userID)
	if err != nil || activeTeamID == uuid.Nil {
		return uuid.Nil, ""
	}
	role, err := b.Store.GetTenantRole(ctx, activeTeamID, userID)
	if err != nil {
		return activeTeamID, ""
	}
	return activeTeamID, role
}
