import { HttpClient, HttpContext } from '@angular/common/http';
import { Injectable, computed, inject, signal } from '@angular/core';
import { finalize, tap } from 'rxjs';

import { Site, SystemStatus, UserTeamsResponse } from '@models/analytics.types';
import { SKIP_AUTH_REDIRECT } from '@core/interceptors/auth.interceptor';
import { SiteService } from '@features/sites/services/site.service';
import { AuthSession, AuthService } from '@services/auth.service';
import { PermissionService, UserPermissions } from '@services/permission.service';
import { TeamService } from '@services/team.service';
import { UserPreferences, UserPreferencesService } from '@services/user-preferences.service';
import { UserProfile, UserProfileService } from '@services/user-profile.service';

export interface DashboardBootstrap {
    session: AuthSession;
    profile: UserProfile;
    preferences: UserPreferences;
    teams: UserTeamsResponse;
    permissions: UserPermissions;
    sites: Site[];
    status: SystemStatus;
}

@Injectable({ providedIn: 'root' })
export class DashboardBootstrapService {
    private http = inject(HttpClient);
    private auth = inject(AuthService);
    private profile = inject(UserProfileService);
    private preferences = inject(UserPreferencesService);
    private teams = inject(TeamService);
    private permissions = inject(PermissionService);
    private sites = inject(SiteService);

    readonly status = signal<SystemStatus | null>(null);
    readonly isLoading = signal(false);
    readonly cloudHosted = computed(() => Boolean(this.status()?.cloud?.hosted));
    readonly cloudSupportUrl = computed(() => this.status()?.cloud?.support_url?.trim() ?? '');

    load() {
        this.isLoading.set(true);
        const context = new HttpContext().set(SKIP_AUTH_REDIRECT, true);
        return this.http.get<DashboardBootstrap>('/api/user/bootstrap', { context }).pipe(
            tap((bootstrap) => this.applyBootstrap(bootstrap)),
            finalize(() => this.isLoading.set(false))
        );
    }

    applyBootstrap(bootstrap: DashboardBootstrap) {
        this.auth.applySession(bootstrap.session);
        this.profile.applyProfile(bootstrap.profile);
        this.preferences.applyPreferences(bootstrap.preferences);
        this.teams.applyTeams(bootstrap.teams);
        this.permissions.applyPermissions(bootstrap.permissions);
        this.sites.applySites(bootstrap.sites);
        this.status.set(bootstrap.status);
    }
}
