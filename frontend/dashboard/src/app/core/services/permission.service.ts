import { Injectable, inject, signal, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { tap } from 'rxjs';

export interface UserPermissions {
    instance_role: 'owner' | 'admin' | 'user';
    permissions: Record<string, 'owner' | 'admin' | 'editor' | 'viewer'>;
    instance_permissions?: string[];
}

@Injectable({ providedIn: 'root' })
export class PermissionService {
    private http = inject(HttpClient);

    readonly permissions = signal<UserPermissions | null>(null);

    readonly isInstanceOwner = computed(() => this.permissions()?.instance_role === 'owner');

    readonly isInstanceAdmin = computed(() => ['owner', 'admin'].includes(this.permissions()?.instance_role || ''));

    loadPermissions() {
        return this.http.get<UserPermissions>('/api/user/permissions').pipe(tap((perms) => this.applyPermissions(perms)));
    }

    applyPermissions(perms: UserPermissions) {
        this.permissions.set(perms);
    }

    canManageSite(siteId: string): boolean {
        const perms = this.permissions();
        if (!perms) return false;

        if (['owner', 'admin'].includes(perms.instance_role)) return true;

        const siteRole = perms.permissions[siteId];
        return ['owner', 'admin'].includes(siteRole);
    }

    canViewSite(siteId: string): boolean {
        const perms = this.permissions();
        if (!perms) return false;

        if (['owner', 'admin'].includes(perms.instance_role)) return true;

        return !!perms.permissions[siteId];
    }

    canDeleteSite(siteId: string): boolean {
        const perms = this.permissions();
        if (!perms) return false;

        if (perms.instance_role === 'owner') return true;

        return perms.permissions[siteId] === 'owner';
    }
}
