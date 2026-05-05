import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { RouterLink, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { GoogleSearchConsoleService } from '@services/google-search-console.service';
import { SearchConsoleDrilldown } from './search-console-drilldown';

describe('SearchConsoleDrilldown', () => {
    let fixture: ComponentFixture<SearchConsoleDrilldown>;
    let service: {
        getSiteMapping: ReturnType<typeof vi.fn>;
        getOverview: ReturnType<typeof vi.fn>;
        getSeries: ReturnType<typeof vi.fn>;
        getQueries: ReturnType<typeof vi.fn>;
        getPages: ReturnType<typeof vi.fn>;
        getBreakdown: ReturnType<typeof vi.fn>;
    };

    beforeEach(async () => {
        service = {
            getSiteMapping: vi.fn(() =>
                of({
                    site_id: 'site-1',
                    team_id: 'team-1',
                    mapped: true,
                    property_uri: 'sc-domain:example.com',
                    can_manage: true,
                    sync_status: { state: 'succeeded', manual: false }
                })
            ),
            getOverview: vi.fn(() => of({ data_source: 'google_search_console', clicks: 42, impressions: 420, ctr: 0.1, average_position: 3.2 })),
            getSeries: vi.fn(() => of({ data_source: 'google_search_console', series: [{ date: '2026-05-01', clicks: 12, impressions: 120, ctr: 0.1, average_position: 3.4 }] })),
            getQueries: vi.fn(() => of({ data_source: 'google_search_console', rows: [{ value: 'hitkeep analytics', clicks: 20, impressions: 200, ctr: 0.1, average_position: 2.8 }] })),
            getPages: vi.fn(() => of({ data_source: 'google_search_console', rows: [{ value: 'https://example.com/docs', clicks: 11, impressions: 90, ctr: 0.12, average_position: 4.1 }] })),
            getBreakdown: vi.fn((_siteID: string, dimension: 'country' | 'device') =>
                of({
                    data_source: 'google_search_console',
                    rows: [{ value: dimension === 'country' ? 'usa' : 'desktop', clicks: 9, impressions: 80, ctr: 0.11, average_position: 5 }]
                })
            )
        };

        await TestBed.configureTestingModule({
            imports: [
                SearchConsoleDrilldown,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                empty: {
                                    noDataTitle: 'No data',
                                    noDataDescription: 'No data for this range'
                                },
                                openInNewTabAria: 'Open in new tab',
                                seriesChartAria: 'Series chart with {{count}} points',
                                filters: {
                                    country: 'Country: {{value}}',
                                    device: 'Device: {{value}}',
                                    page: 'Page: {{value}}'
                                }
                            },
                            comparison: {
                                vsLabel: 'vs.'
                            },
                            searchConsole: {
                                title: 'Search Console',
                                description: 'Delayed, aggregated Search Console data.',
                                kpis: {
                                    clicks: 'Clicks',
                                    impressions: 'Impressions',
                                    ctr: 'CTR',
                                    position: 'Avg. position'
                                },
                                sections: {
                                    trend: 'Trend',
                                    topQueries: 'Top queries',
                                    topPages: 'Top pages',
                                    countries: 'Countries',
                                    devices: 'Devices'
                                },
                                context: {
                                    label: 'Search Console scope',
                                    range: 'Range: {{value}}'
                                },
                                states: {
                                    pending: 'Search Console data is being imported.',
                                    empty: 'No Search Console rows match this range.',
                                    error: 'Could not load Search Console data.'
                                },
                                setup: {
                                    title: 'Connect Search Console',
                                    manageText: 'Connect or map Search Console to add search query data here.',
                                    readonlyText: 'Ask your team or site operator to connect Search Console.',
                                    action: 'Set up Search Console'
                                }
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    }
                })
            ],
            providers: [
                { provide: GoogleSearchConsoleService, useValue: service },
                provideRouter([]),
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: {
                        en: 'en-US'
                    }
                })
            ]
        }).compileComponents();
    });

    it('renders Search Console KPIs and top rows for a mapped site', async () => {
        fixture = TestBed.createComponent(SearchConsoleDrilldown);
        fixture.componentRef.setInput('siteId', 'site-1');
        fixture.componentRef.setInput('siteDomain', 'example.com');
        fixture.componentRef.setInput('from', '2026-05-01T00:00:00Z');
        fixture.componentRef.setInput('to', '2026-05-05T00:00:00Z');
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Search Console');
        expect(fixture.nativeElement.textContent).toContain('42');
        expect(fixture.nativeElement.textContent).toContain('hitkeep analytics');
        expect(fixture.nativeElement.textContent).toContain('/docs');
        expect(fixture.nativeElement.textContent).toContain('United States');
        expect(fixture.nativeElement.textContent).toContain('Range:');
        expect(fixture.nativeElement.textContent).not.toContain('2026-05-01');
        const pageLink = fixture.nativeElement.querySelector('a[href="https://example.com/docs"]') as HTMLAnchorElement | null;
        expect(pageLink).not.toBeNull();
        expect(service.getOverview).toHaveBeenCalledWith('site-1', {
            from: '2026-05-01T00:00:00Z',
            to: '2026-05-05T00:00:00Z',
            path: null,
            country: null,
            device: null
        });
    });

    it('shows the dashboard date range and Search Console-supported filters', async () => {
        fixture = TestBed.createComponent(SearchConsoleDrilldown);
        fixture.componentRef.setInput('siteId', 'site-1');
        fixture.componentRef.setInput('siteDomain', 'example.com');
        fixture.componentRef.setInput('from', '2026-05-01T00:00:00Z');
        fixture.componentRef.setInput('to', '2026-05-05T00:00:00Z');
        fixture.componentRef.setInput('path', '/docs');
        fixture.componentRef.setInput('country', 'usa');
        fixture.componentRef.setInput('device', 'desktop');
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Range:');
        expect(fixture.nativeElement.textContent).toContain('May 1');
        expect(fixture.nativeElement.textContent).toContain('May 5, 2026');
        expect(fixture.nativeElement.textContent).toContain('Page: /docs');
        expect(fixture.nativeElement.textContent).toContain('Country: United States');
        expect(fixture.nativeElement.textContent).toContain('Device: Desktop');
        expect(service.getOverview).toHaveBeenCalledWith('site-1', {
            from: '2026-05-01T00:00:00Z',
            to: '2026-05-05T00:00:00Z',
            path: '/docs',
            country: 'usa',
            device: 'desktop'
        });
    });

    it('reloads reports when the dashboard refresh token changes', async () => {
        fixture = TestBed.createComponent(SearchConsoleDrilldown);
        fixture.componentRef.setInput('siteId', 'site-1');
        fixture.componentRef.setInput('from', '2026-05-01T00:00:00Z');
        fixture.componentRef.setInput('to', '2026-05-05T00:00:00Z');
        fixture.componentRef.setInput('refreshKey', 0);
        fixture.detectChanges();
        await fixture.whenStable();

        expect(service.getOverview).toHaveBeenCalledTimes(1);
        expect(service.getSiteMapping).toHaveBeenCalledTimes(1);

        fixture.componentRef.setInput('refreshKey', 1);
        fixture.detectChanges();
        await fixture.whenStable();

        expect(service.getOverview).toHaveBeenCalledTimes(2);
        expect(service.getSiteMapping).toHaveBeenCalledTimes(1);
    });

    it('reloads reports without re-fetching mapping when filters change', async () => {
        fixture = TestBed.createComponent(SearchConsoleDrilldown);
        fixture.componentRef.setInput('siteId', 'site-1');
        fixture.componentRef.setInput('from', '2026-05-01T00:00:00Z');
        fixture.componentRef.setInput('to', '2026-05-05T00:00:00Z');
        fixture.detectChanges();
        await fixture.whenStable();

        service.getOverview.mockClear();
        service.getSiteMapping.mockClear();

        fixture.componentRef.setInput('path', '/docs');
        fixture.detectChanges();
        await fixture.whenStable();

        expect(service.getSiteMapping).not.toHaveBeenCalled();
        expect(service.getOverview).toHaveBeenCalledWith('site-1', {
            from: '2026-05-01T00:00:00Z',
            to: '2026-05-05T00:00:00Z',
            path: '/docs',
            country: null,
            device: null
        });
    });

    it('shows an operator note for readonly unmapped sites without calling report APIs', async () => {
        service.getSiteMapping.mockReturnValueOnce(of({ site_id: 'site-1', team_id: 'team-1', mapped: false, can_manage: false }));
        fixture = TestBed.createComponent(SearchConsoleDrilldown);
        fixture.componentRef.setInput('siteId', 'site-1');
        fixture.componentRef.setInput('from', '2026-05-01T00:00:00Z');
        fixture.componentRef.setInput('to', '2026-05-05T00:00:00Z');
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Connect Search Console');
        expect(fixture.nativeElement.textContent).toContain('Ask your team or site operator');
        expect(fixture.nativeElement.textContent).not.toContain('Set up Search Console');
        expect(fixture.debugElement.query(By.css('[data-testid="search-console-setup-action"]'))).toBeNull();
        expect(service.getOverview).not.toHaveBeenCalled();
    });

    it('shows a setup link for managers when the site is not mapped', async () => {
        service.getSiteMapping.mockReturnValueOnce(of({ site_id: 'site-1', team_id: 'team-1', mapped: false, can_manage: true }));
        fixture = TestBed.createComponent(SearchConsoleDrilldown);
        fixture.componentRef.setInput('siteId', 'site-1');
        fixture.componentRef.setInput('from', '2026-05-01T00:00:00Z');
        fixture.componentRef.setInput('to', '2026-05-05T00:00:00Z');
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Set up Search Console');
        const action = fixture.debugElement.query(By.css('[data-testid="search-console-setup-action"]'));
        expect(action).not.toBeNull();
        const routerLink = action.injector.get(RouterLink);
        expect(routerLink.urlTree?.toString()).toBe('/integration/google-search-console');
        expect(service.getOverview).not.toHaveBeenCalled();
    });

    it('does not show Search Console setup in share mode', async () => {
        service.getSiteMapping.mockReturnValueOnce(of({ site_id: 'site-1', team_id: 'team-1', mapped: false, can_manage: true }));
        fixture = TestBed.createComponent(SearchConsoleDrilldown);
        fixture.componentRef.setInput('siteId', 'site-1');
        fixture.componentRef.setInput('shareMode', true);
        fixture.detectChanges();
        await fixture.whenStable();

        expect(fixture.nativeElement.textContent).not.toContain('Connect Search Console');
        expect(service.getSiteMapping).not.toHaveBeenCalled();

        service.getSiteMapping.mockClear();
        fixture.componentRef.setInput('shareMode', true);
        fixture.detectChanges();
        await fixture.whenStable();

        expect(service.getSiteMapping).not.toHaveBeenCalled();
        expect(service.getOverview).not.toHaveBeenCalled();
    });
});
