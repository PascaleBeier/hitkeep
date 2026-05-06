import type { Mock } from 'vitest';
import { HttpHeaders, provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { SiteService } from '@features/sites/services/site.service';
import { MenuItem } from 'primeng/api';
import { vi } from 'vitest';

import { ImportExportExportPage } from './import-export-export';

interface ExportPageTestAccess {
    allSitesExportMenuItems: () => MenuItem[];
    siteExportMenuItems: (siteID: string) => MenuItem[];
}

describe('ImportExportExportPage', () => {
    let fixture: ComponentFixture<ImportExportExportPage>;
    let httpMock: HttpTestingController;
    let createObjectURLSpy: Mock;
    let revokeObjectURLSpy: Mock;
    let clickSpy: Mock;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                ImportExportExportPage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                exportFormats: {
                                    csv: 'CSV',
                                    xlsx: 'XLSX',
                                    parquet: 'Parquet',
                                    json: 'JSON',
                                    ndjson: 'NDJSON'
                                },
                                preparing: 'Preparing...'
                            },
                            importExport: {
                                export: {
                                    title: 'Export',
                                    allSites: {
                                        title: 'All accessible sites',
                                        description: 'Download analytics data for every site you can access.',
                                        primaryAction: 'Download all sites',
                                        otherFormats: 'Other formats',
                                        success: 'All accessible site analytics export downloaded.',
                                        error: 'Export failed. Try again.'
                                    },
                                    siteExports: {
                                        title: 'Site exports',
                                        description: 'Download one site at a time when you only need a specific property.',
                                        primaryAction: 'Download site',
                                        loading: 'Loading sites...',
                                        empty: 'No sites available to export.',
                                        success: 'Site export downloaded.',
                                        error: 'Export failed. Try again or choose another format.'
                                    }
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
            providers: [provideHttpClient(), provideHttpClientTesting()]
        }).compileComponents();

        fixture = TestBed.createComponent(ImportExportExportPage);
        httpMock = TestBed.inject(HttpTestingController);
        createObjectURLSpy = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:test-url');
        revokeObjectURLSpy = vi.spyOn(URL, 'revokeObjectURL');
        clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click');
        fixture.detectChanges();
    });

    afterEach(() => {
        httpMock.verify();
        vi.restoreAllMocks();
    });

    it('renders all-sites analytics export copy without account-export framing', () => {
        const text = fixture.nativeElement.textContent;

        expect(text).toContain('All accessible sites');
        expect(text).toContain('Download analytics data for every site you can access.');
        expect(text).not.toContain('account export');
        expect(text).not.toContain('profile export');
    });

    it('downloads all accessible sites as xlsx by default and blocks double-submit while pending', () => {
        primaryButton().click();
        fixture.detectChanges();

        const req = httpMock.expectOne('/api/user/takeout?format=xlsx');
        expect(req.request.method).toBe('GET');
        expect(primaryButton().disabled).toBe(true);

        primaryButton().click();
        httpMock.expectNone('/api/user/takeout?format=xlsx');

        req.flush(new Blob(['xlsx'], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), {
            headers: new HttpHeaders({
                'content-disposition': 'attachment; filename="all-sites.xlsx"'
            })
        });
        fixture.detectChanges();

        expect(primaryButton().disabled).toBe(false);
        expect(fixture.nativeElement.textContent).toContain('All accessible site analytics export downloaded.');
        expect(createObjectURLSpy).toHaveBeenCalled();
        expect(clickSpy).toHaveBeenCalled();
        expect(revokeObjectURLSpy).toHaveBeenCalledWith('blob:test-url');
    });

    it('keeps per-site rows usable while the all-sites export is pending', () => {
        setSites([site('site-1', 'alpha.example.com')]);

        primaryButton().click();
        fixture.detectChanges();

        const req = httpMock.expectOne('/api/user/takeout?format=xlsx');
        expect(sitePrimaryButton('site-1').disabled).toBe(false);

        req.flush(new Blob(['xlsx'], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), {
            headers: new HttpHeaders({
                'content-disposition': 'attachment; filename="all-sites.xlsx"'
            })
        });
    });

    it('downloads all accessible sites in alternate formats', () => {
        for (const [format, label] of [
            ['csv', 'CSV'],
            ['parquet', 'Parquet'],
            ['json', 'JSON'],
            ['ndjson', 'NDJSON']
        ] as const) {
            runFormatMenuCommand(label);

            const req = httpMock.expectOne(`/api/user/takeout?format=${format}`);
            expect(req.request.method).toBe('GET');
            req.flush(new Blob([format], { type: 'application/octet-stream' }), {
                headers: new HttpHeaders({
                    'content-disposition': `attachment; filename="all-sites.${format}"`
                })
            });
            fixture.detectChanges();
        }

        expect(clickSpy).toHaveBeenCalledTimes(4);
    });

    it('shows local error feedback after a failed all-sites export', () => {
        runFormatMenuCommand('JSON');

        const req = httpMock.expectOne('/api/user/takeout?format=json');
        req.flush(new Blob(['failed'], { type: 'text/plain' }), { status: 500, statusText: 'Server Error' });
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Export failed. Try again.');
        expect(primaryButton().disabled).toBe(false);
    });

    it('lists accessible sites and downloads one site as xlsx by default', () => {
        setSites([site('site-1', 'alpha.example.com'), site('site-2', 'beta.example.com')]);

        expect(fixture.nativeElement.textContent).toContain('Site exports');
        expect(fixture.nativeElement.textContent).toContain('alpha.example.com');
        expect(fixture.nativeElement.textContent).toContain('beta.example.com');

        sitePrimaryButton('site-1').click();
        fixture.detectChanges();

        const req = httpMock.expectOne('/api/sites/site-1/takeout?format=xlsx');
        expect(req.request.method).toBe('GET');
        expect(sitePrimaryButton('site-1').disabled).toBe(true);
        expect(sitePrimaryButton('site-2').disabled).toBe(false);

        req.flush(new Blob(['site'], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), {
            headers: new HttpHeaders({
                'content-disposition': 'attachment; filename="alpha.xlsx"'
            })
        });
        fixture.detectChanges();

        expect(siteExportRow('site-1').textContent).toContain('Site export downloaded.');
        expect(siteExportRow('site-2').textContent).not.toContain('Site export downloaded.');
        expect(clickSpy).toHaveBeenCalled();
    });

    it('downloads one site in alternate formats without changing other row state', () => {
        setSites([site('site-1', 'alpha.example.com'), site('site-2', 'beta.example.com')]);

        runSiteFormatMenuCommand('site-2', 'JSON');

        const req = httpMock.expectOne('/api/sites/site-2/takeout?format=json');
        expect(req.request.method).toBe('GET');
        req.flush(new Blob(['json'], { type: 'application/json' }), {
            headers: new HttpHeaders({
                'content-disposition': 'attachment; filename="beta.json"'
            })
        });
        fixture.detectChanges();

        expect(siteExportRow('site-2').textContent).toContain('Site export downloaded.');
        expect(siteExportRow('site-1').textContent).not.toContain('Site export downloaded.');
    });

    it('keeps per-site error state isolated to the selected row', () => {
        setSites([site('site-1', 'alpha.example.com'), site('site-2', 'beta.example.com')]);

        sitePrimaryButton('site-1').click();

        const req = httpMock.expectOne('/api/sites/site-1/takeout?format=xlsx');
        req.flush(new Blob(['failed'], { type: 'text/plain' }), { status: 403, statusText: 'Forbidden' });
        fixture.detectChanges();

        expect(siteExportRow('site-1').textContent).toContain('Export failed. Try again or choose another format.');
        expect(siteExportRow('site-2').textContent).not.toContain('Export failed. Try again or choose another format.');
        expect(sitePrimaryButton('site-1').disabled).toBe(false);
    });

    it('shows compact loading and empty states for site exports', () => {
        const siteService = TestBed.inject(SiteService);

        siteService.sites.set([]);
        siteService.isLoading.set(true);
        fixture.detectChanges();
        expect(fixture.nativeElement.textContent).toContain('Loading sites...');

        siteService.isLoading.set(false);
        fixture.detectChanges();
        expect(fixture.nativeElement.textContent).toContain('No sites available to export.');
    });

    function primaryButton(): HTMLButtonElement {
        const button = fixture.nativeElement.querySelector('[data-testid="all-sites-export-primary"] button') as HTMLButtonElement | null;
        expect(button).not.toBeNull();
        return button!;
    }

    function runFormatMenuCommand(label: string): void {
        const menuItems = (fixture.componentInstance as unknown as ExportPageTestAccess).allSitesExportMenuItems();
        const item = menuItems.find((entry) => entry.label === label);
        expect(item).toBeTruthy();
        (item!.command as (() => void) | undefined)?.();
        fixture.detectChanges();
    }

    function runSiteFormatMenuCommand(siteID: string, label: string): void {
        const menuItems = (fixture.componentInstance as unknown as ExportPageTestAccess).siteExportMenuItems(siteID);
        const item = menuItems.find((entry) => entry.label === label);
        expect(item).toBeTruthy();
        (item!.command as (() => void) | undefined)?.();
        fixture.detectChanges();
    }

    function setSites(sites: ReturnType<typeof site>[]): void {
        const siteService = TestBed.inject(SiteService);
        siteService.applySites(sites);
        siteService.isLoading.set(false);
        fixture.detectChanges();
    }

    function site(id: string, domain: string) {
        return {
            id,
            user_id: 'user-1',
            domain,
            created_at: '2026-05-06T00:00:00Z'
        };
    }

    function sitePrimaryButton(siteID: string): HTMLButtonElement {
        const button = fixture.nativeElement.querySelector(`[data-testid="site-export-primary-${siteID}"] button`) as HTMLButtonElement | null;
        expect(button).not.toBeNull();
        return button!;
    }

    function siteExportRow(siteID: string): HTMLElement {
        const row = fixture.nativeElement.querySelector(`[data-testid="site-export-row-${siteID}"]`) as HTMLElement | null;
        expect(row).not.toBeNull();
        return row!;
    }
});
