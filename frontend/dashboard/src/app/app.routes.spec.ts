import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { PermissionService } from '@services/permission.service';
import { SiteService } from '@features/sites/services/site.service';
import { routes } from './app.routes';

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
