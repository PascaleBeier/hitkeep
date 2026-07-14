import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { INSTANCE_CAPABILITIES, TEAM_CAPABILITIES } from '@core/access/capabilities';
import { PermissionService } from '@services/permission.service';
import { SiteService } from '@features/sites/services/site.service';
import { routes } from './app.routes';
import { overviewDefaultGuard } from '@pages/overview/overview-default.guard';
import { SETTINGS_ROUTES } from '@pages/settings/settings.routes';

describe('routes', () => {
    it('should be accepted by Angular Router', () => {
        TestBed.configureTestingModule({
            providers: [provideRouter(routes)]
        });

        expect(TestBed.inject(Router).config.length).toBe(routes.length);
    });

    it('should expose Import & Export as one routed hub with addressable tabs', () => {
        const mainRoute = routes.find((route) => route.path === '');
        const children = mainRoute?.children ?? [];
        const importExportRoute = children.find((route) => route.path === 'import-export');

        expect(importExportRoute).toBeTruthy();
        expect(importExportRoute?.children?.some((route) => route.path === '' && route.pathMatch === 'full' && !!route.canActivate?.length)).toBe(true);
        expect(importExportRoute?.children?.some((route) => route.path === 'import')).toBe(true);
        expect(importExportRoute?.children?.some((route) => route.path === 'export')).toBe(true);
        expect(children.some((route) => route.path === 'imports')).toBe(false);
    });

    it('gates system status and system settings by their backend capabilities', () => {
        const adminChildren = routes.find((route) => route.path === '')?.children?.find((route) => route.path === 'admin')?.children ?? [];
        const statusRoute = adminChildren.find((route) => route.path === 'status');
        const settingsRoute = adminChildren.find((route) => route.path === 'system');

        expect(statusRoute?.data?.['instanceCapability']).toBe(INSTANCE_CAPABILITIES.viewSystem);
        expect(settingsRoute?.data?.['instanceCapability']).toBe(INSTANCE_CAPABILITIES.manageUsers);
    });

    it('exposes team admin sections as addressable child routes with descriptive titles', () => {
        const adminChildren = routes.find((route) => route.path === '')?.children?.find((route) => route.path === 'admin')?.children ?? [];
        const teamRoute = adminChildren.find((route) => route.path === 'team');
        const teamChildren = teamRoute?.children ?? [];
        const child = (path: string) => teamChildren.find((route) => route.path === path);

        expect(teamRoute?.data?.['activeTeamCapability']).toBe(TEAM_CAPABILITIES.manageSettings);
        expect(teamRoute?.loadComponent).toBeTruthy();

        // Default entry redirects to the overview child, not to /team itself.
        expect(child('')?.redirectTo).toBe('overview');
        expect(child('')?.pathMatch).toBe('full');

        // Every visible section is a real, addressable child route (no redirect) with its own title key.
        for (const [path, titleKey] of [
            ['overview', 'admin.team.tabs.overview'],
            ['sso', 'admin.team.tabs.sso'],
            ['api-clients', 'admin.team.tabs.apiClients'],
            ['custom-domains', 'admin.team.tabs.customDomains'],
            ['branding', 'admin.team.tabs.branding'],
            ['activity', 'admin.team.tabs.activity'],
            ['danger-zone', 'admin.team.tabs.dangerZone']
        ] as const) {
            expect(child(path)?.redirectTo).toBeUndefined();
            expect(child(path)?.loadComponent).toBeTruthy();
            expect(child(path)?.data?.['titleKey']).toBe(titleKey);
            expect(child(path)?.data?.['titleScope']).toBe('team');
        }

        const membersRoute = child('members');
        const membersChild = (path: string) => membersRoute?.children?.find((route) => route.path === path);
        expect(membersRoute?.loadComponent).toBeUndefined();
        expect(membersChild('')?.loadComponent).toBeTruthy();
        expect(membersChild('')?.data?.['titleKey']).toBe('admin.team.tabs.members');
        expect(membersChild('invite')?.loadComponent).toBeTruthy();
        expect(membersChild('invite')?.data?.['openInvite']).toBe(true);

        // Activity additionally requires the audit capability.
        expect(child('activity')?.data?.['activeTeamCapability']).toBe(TEAM_CAPABILITIES.viewAudit);
        expect(child('activity')?.canActivate?.length).toBeTruthy();

        // Legacy deep links keep working via redirects.
        expect(child('settings')?.redirectTo).toBe('api-clients');
        expect(child('tracking-domains')?.redirectTo).toBe('custom-domains');
    });

    it('redirects the obsolete user-settings team path to the routed team invitation flow', () => {
        const route = SETTINGS_ROUTES.find((candidate) => candidate.path === 'team');

        expect(route?.redirectTo).toBe('/admin/team/members/invite');
    });

    it('exposes site settings as lazy addressable sections', () => {
        const mainChildren = routes.find((route) => route.path === '')?.children ?? [];
        const settingsRoute = mainChildren.find((route) => route.path === 'sites/:siteId/settings');
        const children = settingsRoute?.children ?? [];
        const child = (path: string) => children.find((route) => route.path === path);

        expect(settingsRoute?.canActivate?.length).toBeTruthy();
        expect(settingsRoute?.loadComponent).toBeTruthy();
        expect(child('')?.redirectTo).toBe('general');

        for (const path of ['general', 'tracking', 'filtering', 'retention', 'access', 'danger-zone']) {
            expect(child(path)?.loadComponent).toBeTruthy();
            expect(child(path)?.data?.['titleScope']).toBe('site');
        }

        for (const path of ['filtering', 'retention', 'access', 'danger-zone']) {
            expect(child(path)?.canActivate?.length).toBeTruthy();
        }
    });

    it('exposes accept-invite as a public auth page route', () => {
        const route = routes.find((route) => route.path === 'accept-invite');

        expect(route).toBeTruthy();
        expect(route?.canActivate).toBeUndefined();
    });

    it('exposes the multi-site overview page and delegates the authenticated start page to its default guard', () => {
        const children = routes.find((route) => route.path === '')?.children ?? [];
        const overviewRoute = children.find((route) => route.path === 'overview');
        const defaultRoute = children.find((route) => route.path === '' && route.pathMatch === 'full');

        expect(overviewRoute).toBeTruthy();
        expect(defaultRoute?.redirectTo).toBeUndefined();
        expect(defaultRoute?.canActivate).toContain(overviewDefaultGuard);
    });

    it('should navigate /import-export to Import for active-site managers', async () => {
        await configureImportExportRouter();
        seedSiteRole('admin');

        await RouterTestingHarness.create('/import-export');

        expect(TestBed.inject(Router).url).toBe('/import-export/import');
    });

    it('should navigate /import-export to Export for active-site viewers', async () => {
        await configureImportExportRouter();
        seedSiteRole('viewer');

        await RouterTestingHarness.create('/import-export');

        expect(TestBed.inject(Router).url).toBe('/import-export/export');
    });

    it('should render the existing importer workflow on the Import tab', async () => {
        await configureImportExportRouter();
        const siteId = seedSiteRole('admin');

        const harness = await RouterTestingHarness.create('/import-export/import');
        const http = TestBed.inject(HttpTestingController);

        http.expectOne(`/api/sites/${siteId}/importers`).flush([{ key: 'plausible', name: 'Plausible', accepted_extensions: ['.zip'], capabilities: [] }]);
        http.expectOne(`/api/sites/${siteId}/imports`).flush({ imports: [] });
        harness.detectChanges();

        expect(harness.routeNativeElement?.textContent).toContain('Choose importer');
        expect(harness.routeNativeElement?.textContent).toContain('Plausible');
        http.verify();
    });

    it('should preserve the Import tab no-site state without calling importer APIs', async () => {
        await configureImportExportRouter();
        TestBed.inject(SiteService).applySites([]);
        TestBed.inject(PermissionService).applyPermissions({
            instance_role: 'owner',
            permissions: {}
        });

        const harness = await RouterTestingHarness.create('/import-export/import');
        const http = TestBed.inject(HttpTestingController);

        expect(harness.routeNativeElement?.textContent).toContain('No site selected');
        expect(harness.routeNativeElement?.textContent).toContain('Select a site before importing historical data.');
        http.expectNone((request) => request.url.includes('/importers'));
        http.expectNone((request) => request.url.endsWith('/imports'));
        http.verify();
    });

    it('should preserve the Import tab permission state without calling importer APIs', async () => {
        await configureImportExportRouter();
        const siteId = seedSiteRole('viewer');

        const harness = await RouterTestingHarness.create('/import-export/import');
        const http = TestBed.inject(HttpTestingController);

        expect(harness.routeNativeElement?.textContent).toContain('Import access required');
        expect(harness.routeNativeElement?.textContent).toContain('Site owners, site admins, and instance admins can import data.');
        http.expectNone(`/api/sites/${siteId}/importers`);
        http.expectNone(`/api/sites/${siteId}/imports`);
        http.verify();
    });
});

async function configureImportExportRouter(): Promise<void> {
    const importExportRoute = routes.find((route) => route.path === '')?.children?.find((route) => route.path === 'import-export');

    expect(importExportRoute).toBeTruthy();

    await TestBed.configureTestingModule({
        imports: [
            TranslocoTestingModule.forRoot({
                langs: {
                    en: {
                        importExport: {
                            title: 'Import & Export',
                            tabs: {
                                import: 'Import',
                                export: 'Export'
                            },
                            import: {
                                title: 'Import'
                            },
                            export: {
                                title: 'Export'
                            }
                        },
                        common: {
                            noSiteSelected: 'No site selected',
                            actions: {
                                refresh: 'Refresh'
                            }
                        },
                        imports: {
                            title: 'Imports',
                            noSiteDescription: 'Select a site before importing historical data.',
                            providerFallback: 'Selected importer',
                            flow: {
                                title: 'Site import'
                            },
                            providers: {
                                title: 'Choose importer',
                                loading: 'Loading importers...',
                                selectAria: 'Select {{provider}} importer',
                                guide: 'Import guide',
                                guideAria: 'Open {{provider}} import guide',
                                empty: 'No importers are available for this site.'
                            },
                            history: {
                                title: 'Import history',
                                empty: 'No imports yet.',
                                importer: 'Importer',
                                status: 'Status',
                                rows: 'Rows'
                            },
                            permission: {
                                title: 'Import access required',
                                description: 'Site owners, site admins, and instance admins can import data.'
                            }
                        }
                    }
                },
                translocoConfig: {
                    availableLangs: ['en'],
                    defaultLang: 'en'
                },
                preloadLangs: true
            })
        ],
        providers: [provideRouter([importExportRoute!]), provideHttpClient(), provideHttpClientTesting()]
    }).compileComponents();
}

function seedSiteRole(role: 'admin' | 'viewer'): string {
    const siteId = '00000000-0000-0000-0000-0000000000aa';
    TestBed.inject(SiteService).applySites([
        {
            id: siteId,
            user_id: '00000000-0000-0000-0000-000000000001',
            domain: `${role}.example.com`,
            created_at: '2026-01-01T00:00:00Z'
        }
    ]);
    TestBed.inject(PermissionService).applyPermissions({
        instance_role: 'user',
        permissions: {
            [siteId]: role
        }
    });
    return siteId;
}
