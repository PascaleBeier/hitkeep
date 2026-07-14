import { TestBed } from '@angular/core/testing';
import { ActivatedRouteSnapshot, Router, RouterStateSnapshot, convertToParamMap, provideRouter } from '@angular/router';
import { signal } from '@angular/core';
import { vi } from 'vitest';

import { INSTANCE_CAPABILITIES, SITE_CAPABILITIES } from '@core/access/capabilities';
import { SiteService } from '@features/sites/services/site.service';
import { AccessService } from '@services/access.service';
import { NavigationNoticeService } from '@services/navigation-notice.service';
import { siteSettingsSectionGuard, siteSettingsSiteGuard } from './site-settings.guard';

describe('site settings guards', () => {
    const site = {
        id: 'site-1',
        user_id: 'user-1',
        domain: 'example.com',
        created_at: '2026-01-01T00:00:00Z'
    };
    const sites = signal([site]);
    const activeSite = signal<typeof site | null>(null);
    const siteService = {
        sites,
        activeSite,
        selectSite: vi.fn((selected: typeof site) => activeSite.set(selected))
    };
    const access = {
        canSite: vi.fn((siteID: string, capability: string) => {
            void siteID;
            void capability;
            return true;
        }),
        hasInstance: vi.fn((capability: string) => {
            void capability;
            return false;
        })
    };

    beforeEach(() => {
        sites.set([site]);
        activeSite.set(null);
        siteService.selectSite.mockClear();
        access.canSite.mockReset();
        access.canSite.mockReturnValue(true);
        access.hasInstance.mockReset();
        access.hasInstance.mockReturnValue(false);

        TestBed.configureTestingModule({
            providers: [provideRouter([]), { provide: SiteService, useValue: siteService }, { provide: AccessService, useValue: access }]
        });
    });

    it('selects the accessible site named by the settings URL', () => {
        const result = runGuard(siteSettingsSiteGuard, route({ siteId: 'site-1' }));

        expect(result).toBe(true);
        expect(siteService.selectSite).toHaveBeenCalledWith(site);
    });

    it('redirects an unavailable site to overview with a generic notice', () => {
        const result = runGuard(siteSettingsSiteGuard, route({ siteId: 'missing' }));

        expect(TestBed.inject(Router).serializeUrl(result as never)).toBe('/overview');
        expect(TestBed.inject(NavigationNoticeService).consume()).toBe('sites.settings.notices.siteUnavailable');
    });

    it('allows filtering for site data managers or instance exclusion managers', () => {
        activeSite.set(site);
        access.canSite.mockReturnValue(false);
        access.hasInstance.mockImplementation((capability: string) => capability === INSTANCE_CAPABILITIES.manageSiteExclusions);

        const result = runGuard(siteSettingsSectionGuard, route({}, { siteSettingsSection: 'filtering' }));

        expect(result).toBe(true);
        expect(access.canSite).toHaveBeenCalledWith('site-1', SITE_CAPABILITIES.manageData);
    });

    it('redirects an unavailable section to general settings with a notice', () => {
        activeSite.set(site);
        access.canSite.mockReturnValue(false);

        const result = runGuard(siteSettingsSectionGuard, route({}, { siteSettingsSection: 'access' }));

        expect(TestBed.inject(Router).serializeUrl(result as never)).toBe('/sites/site-1/settings/general');
        expect(TestBed.inject(NavigationNoticeService).consume()).toBe('sites.settings.notices.sectionUnavailable');
    });
});

function route(params: Record<string, string>, data: Record<string, unknown> = {}): ActivatedRouteSnapshot {
    return {
        paramMap: convertToParamMap(params),
        data
    } as ActivatedRouteSnapshot;
}

function runGuard(guard: typeof siteSettingsSiteGuard, snapshot: ActivatedRouteSnapshot) {
    return TestBed.runInInjectionContext(() => guard(snapshot, { url: '/test' } as RouterStateSnapshot));
}
