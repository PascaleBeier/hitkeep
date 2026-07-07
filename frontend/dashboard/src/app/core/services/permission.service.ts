import { Injectable, inject, signal, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { tap } from 'rxjs';
import type { InstanceRole, SiteRole, TeamRole } from '@core/access/capabilities';

export interface UserPermissions {
    instance_role: InstanceRole;
    permissions: Record<string, SiteRole>;
    instance_permissions?: string[];
    instance_capabilities?: string[];
    site_capabilities?: Record<string, string[]>;
    active_team_id?: string;
    active_team_role?: TeamRole | '';
    active_team_capabilities?: string[];
    can_create_teams?: boolean;
}

@Injectable({ providedIn: 'root' })
export class PermissionService {
    private http = inject(HttpClient);

    readonly permissions = signal<UserPermissions | null>(null);

    readonly isInstanceOwner = computed(() => this.permissions()?.instance_role === 'owner');

    readonly isInstanceAdmin = computed(() => ['owner', 'admin'].includes(this.permissions()?.instance_role || ''));

    /** Server-derived: whether the current user may create another team. */
    readonly canCreateTeams = computed(() => this.permissions()?.can_create_teams ?? false);

    loadPermissions() {
        return this.http.get<UserPermissions>('/api/user/permissions').pipe(tap((perms) => this.applyPermissions(perms)));
    }

    applyPermissions(perms: UserPermissions) {
        this.permissions.set(perms);
    }
}
