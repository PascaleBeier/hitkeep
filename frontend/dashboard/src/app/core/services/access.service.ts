import { computed, inject, Service } from '@angular/core';
import { SiteService } from '@features/sites/services/site.service';
import { INSTANCE_ROLE_CAPABILITIES, InstanceCapability, SITE_ROLE_CAPABILITIES, SiteCapability, TEAM_ROLE_CAPABILITIES, TeamCapability } from '@core/access/capabilities';
import { PermissionService } from '@services/permission.service';
import { TeamService } from '@services/team.service';

@Service()
export class AccessService {
    private readonly permissions = inject(PermissionService);
    private readonly sites = inject(SiteService);
    private readonly teams = inject(TeamService);

    readonly context = this.permissions.permissions;
    readonly activeTeamRole = computed(() => this.context()?.active_team_role ?? '');

    hasInstance(capability: InstanceCapability | string): boolean {
        const context = this.context();
        if (!context) return false;
        if (context.instance_capabilities !== undefined) {
            return context.instance_capabilities.includes(capability);
        }
        if (context.instance_permissions !== undefined && context.instance_permissions.includes(capability)) {
            return true;
        }
        return this.roleHasCapability(INSTANCE_ROLE_CAPABILITIES, context.instance_role, capability);
    }

    canSite(siteID: string, capability: SiteCapability | string): boolean {
        const context = this.context();
        if (!context) return false;
        if (this.hasInstance(capability)) return true;
        if (context.site_capabilities !== undefined) {
            return context.site_capabilities[siteID]?.includes(capability) ?? false;
        }
        return this.roleHasCapability(SITE_ROLE_CAPABILITIES, context.permissions[siteID], capability);
    }

    canActiveSite(capability: SiteCapability | string): boolean {
        const site = this.sites.activeSite();
        return !!site && this.canSite(site.id, capability);
    }

    canActiveTeam(capability: TeamCapability | string): boolean {
        const context = this.context();
        if (!context) return false;
        const activeTeamId = this.teams.activeTeamId();
        const contextMatchesActiveTeam = !!context.active_team_id && !!activeTeamId && context.active_team_id === activeTeamId;
        if (contextMatchesActiveTeam && context.active_team_capabilities !== undefined) {
            return context.active_team_capabilities.includes(capability);
        }
        const fallbackRole = contextMatchesActiveTeam || !context.active_team_id ? context.active_team_role : this.teams.activeTeam()?.role;
        return this.roleHasCapability(TEAM_ROLE_CAPABILITIES, fallbackRole || this.teams.activeTeam()?.role, capability);
    }

    private roleHasCapability(roleCapabilities: Record<string, readonly string[]>, role: string | undefined, capability: string): boolean {
        return !!role && (roleCapabilities[role]?.includes(capability) ?? false);
    }
}
