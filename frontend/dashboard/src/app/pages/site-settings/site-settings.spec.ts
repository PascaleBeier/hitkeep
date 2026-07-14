import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';

import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { SiteService } from '@features/sites/services/site.service';
import { AccessService } from '@services/access.service';
import { SiteSettingsPage } from './site-settings';

@Component({ template: '<p>settings child</p>' })
class SettingsTestChild {}

describe('SiteSettingsPage', () => {
    const site = signal({
        id: 'site-1',
        user_id: 'user-1',
        domain: 'example.com',
        created_at: '2026-01-01T00:00:00Z'
    });
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

    beforeEach(async () => {
        access.canSite.mockReset();
        access.canSite.mockReturnValue(true);
        access.hasInstance.mockReset();
        access.hasInstance.mockReturnValue(false);

        await TestBed.configureTestingModule({
            imports: [
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            sites: {
                                settings: {
                                    breadcrumb: { sites: 'Sites', settings: 'Settings' },
                                    tabs: {
                                        general: 'General',
                                        tracking: 'Tracking',
                                        filtering: 'Filtering',
                                        retention: 'Retention',
                                        access: 'Access',
                                        dangerZone: 'Danger zone'
                                    }
                                }
                            }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ],
            providers: [
                provideRouter([
                    {
                        path: 'sites/:siteId/settings',
                        component: SiteSettingsPage,
                        children: [
                            { path: 'general', component: SettingsTestChild },
                            { path: 'access', component: SettingsTestChild }
                        ]
                    }
                ]),
                { provide: SiteService, useValue: { activeSite: site } },
                { provide: AccessService, useValue: access }
            ]
        }).compileComponents();
    });

    it('reflects a directly activated settings section in the tab bar', async () => {
        const harness = await RouterTestingHarness.create('/sites/site-1/settings/access');
        const selected = harness.routeNativeElement!.ownerDocument.querySelector('[role="tab"][aria-selected="true"]');

        expect(selected?.textContent).toContain('Access');
        expect(harness.routeNativeElement?.textContent).toContain('settings child');
    });

    it('hides privileged settings tabs without their site capability', async () => {
        access.canSite.mockImplementation((_siteID: string, capability: string) => capability === SITE_CAPABILITIES.view);

        const harness = await RouterTestingHarness.create('/sites/site-1/settings/general');
        const text = harness.routeNativeElement?.textContent ?? '';

        expect(text).toContain('General');
        expect(text).toContain('Tracking');
        expect(text).not.toContain('Retention');
        expect(text).not.toContain('Access');
        expect(text).not.toContain('Danger zone');
    });
});
