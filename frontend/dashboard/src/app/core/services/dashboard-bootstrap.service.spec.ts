import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { vi } from 'vitest';

import { SKIP_AUTH_REDIRECT } from '@core/interceptors/auth.interceptor';
import { SiteService } from '@features/sites/services/site.service';
import { AuthService } from '@services/auth.service';
import { DashboardBootstrap, DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { PermissionService } from '@services/permission.service';
import { TeamService } from '@services/team.service';
import { UserPreferencesService } from '@services/user-preferences.service';
import { UserProfileService } from '@services/user-profile.service';

const bootstrap: DashboardBootstrap = {
    session: {
        expires_at: '2026-05-03T12:15:00Z',
        issued_at: '2026-05-03T12:00:00Z',
        duration_seconds: 900,
        warning_seconds: 120,
        extendable: true,
        timing_adjustable: true,
        remembered: false,
        remember_expires_at: null,
        remember_me_duration_days: 30
    },
    profile: {
        id: '00000000-0000-0000-0000-0000000000aa',
        email: 'owner@example.com',
        display_name: 'Owner',
        avatar_url: ''
    },
    preferences: { default_locale: 'en' },
    teams: {
        active_team_id: '00000000-0000-0000-0000-000000000001',
        teams: []
    },
    permissions: {
        instance_role: 'owner',
        permissions: {}
    },
    sites: [],
    status: {
        needs_setup: false,
        version: 'v2.0.0',
        cloud: {
            hosted: true,
            signup_enabled: false,
            support_url: 'https://hitkeep.com/support/help/'
        }
    }
};

describe('DashboardBootstrapService', () => {
    let service: DashboardBootstrapService;
    let httpMock: HttpTestingController;
    const auth = { applySession: vi.fn() };
    const profile = { applyProfile: vi.fn() };
    const preferences = { applyPreferences: vi.fn() };
    const teams = { applyTeams: vi.fn() };
    const permissions = { applyPermissions: vi.fn() };
    const sites = { applySites: vi.fn() };

    beforeEach(() => {
        auth.applySession.mockReset();
        profile.applyProfile.mockReset();
        preferences.applyPreferences.mockReset();
        teams.applyTeams.mockReset();
        permissions.applyPermissions.mockReset();
        sites.applySites.mockReset();

        TestBed.configureTestingModule({
            providers: [
                provideHttpClient(),
                provideHttpClientTesting(),
                { provide: AuthService, useValue: auth },
                { provide: UserProfileService, useValue: profile },
                { provide: UserPreferencesService, useValue: preferences },
                { provide: TeamService, useValue: teams },
                { provide: PermissionService, useValue: permissions },
                { provide: SiteService, useValue: sites }
            ]
        });
        service = TestBed.inject(DashboardBootstrapService);
        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('loads dashboard bootstrap once and hydrates app services', () => {
        service.load().subscribe();

        const req = httpMock.expectOne('/api/user/bootstrap');
        expect(req.request.context.get(SKIP_AUTH_REDIRECT)).toBe(true);
        req.flush(bootstrap);

        expect(auth.applySession).toHaveBeenCalledWith(bootstrap.session);
        expect(profile.applyProfile).toHaveBeenCalledWith(bootstrap.profile);
        expect(preferences.applyPreferences).toHaveBeenCalledWith(bootstrap.preferences);
        expect(teams.applyTeams).toHaveBeenCalledWith(bootstrap.teams);
        expect(permissions.applyPermissions).toHaveBeenCalledWith(bootstrap.permissions);
        expect(sites.applySites).toHaveBeenCalledWith(bootstrap.sites);
        expect(service.cloudHosted()).toBe(true);
        expect(service.cloudSupportUrl()).toBe('https://hitkeep.com/support/help/');
        expect(service.isLoading()).toBe(false);
    });
});
