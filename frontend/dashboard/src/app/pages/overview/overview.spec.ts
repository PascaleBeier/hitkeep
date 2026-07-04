import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { OverviewPage } from '@pages/overview/overview';
import { SiteService } from '@features/sites/services/site.service';
import { StatsService } from '@features/analytics/services/stats.service';
import { SiteOverviewStats, SitesOverviewStatsResponse } from '@models/analytics.types';

describe('OverviewPage', () => {
    let fixture: ComponentFixture<OverviewPage>;
    let statsService: StatsService;
    let siteService: SiteService;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                OverviewPage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            overview: {
                                title: 'Overview',
                                toolbarAria: 'Site overview controls',
                                gridAria: 'Sites overview',
                                chartAria: 'Traffic trend for {{site}}',
                                searchPlaceholder: 'Filter sites',
                                searchLabel: 'Filter sites',
                                sortLabel: 'Sort sites',
                                sort: {
                                    domain: 'Name',
                                    pageviews: 'Pageviews',
                                    visitors: 'Visitors'
                                },
                                metrics: {
                                    pageviews: 'Pageviews',
                                    visitors: 'Visitors',
                                    bounceRate: 'Bounce'
                                },
                                actions: {
                                    addSite: 'Add site',
                                    openDashboard: 'Open dashboard',
                                    openDashboardFor: 'Open dashboard for {{site}}'
                                },
                                empty: {
                                    noSitesTitle: 'No sites yet',
                                    noSitesDescription: 'Add a site to start collecting analytics.',
                                    noMatchesTitle: 'No matching sites',
                                    noMatchesDescription: 'Adjust the filter to show more sites.',
                                    noTraffic: 'No traffic'
                                },
                                errors: {
                                    siteStatsFailed: 'Stats unavailable'
                                }
                            },
                            common: {
                                timeRanges: {
                                    today: 'Today',
                                    yesterday: 'Yesterday',
                                    last24Hours: '24h',
                                    last7Days: '7d',
                                    last30Days: '30d',
                                    customShort: 'Custom',
                                    moreRanges: 'More ranges',
                                    searchRanges: 'Search ranges'
                                },
                                actions: {
                                    refresh: 'Refresh',
                                    apply: 'Apply',
                                    cancel: 'Cancel'
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
            providers: [
                provideHttpClient(),
                provideHttpClientTesting(),
                provideRouter([]),
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: {
                        en: 'en-US',
                        'en-US': 'en-US'
                    }
                })
            ]
        }).compileComponents();

        statsService = TestBed.inject(StatsService);
        siteService = TestBed.inject(SiteService);
        localStorage.clear();
    });

    it('loads and renders stats for each accessible site', () => {
        const fetchOverview = vi.spyOn(statsService, 'fetchSitesOverviewStats').mockReturnValue(of(overviewResponse(statsWithTraffic('site-alpha', 120, 45), statsWithTraffic('site-beta', 30, 12))));
        siteService.applySites([
            {
                id: 'site-alpha',
                user_id: 'user-1',
                domain: 'alpha.example.com',
                created_at: '2026-01-01T00:00:00Z'
            },
            {
                id: 'site-beta',
                user_id: 'user-1',
                domain: 'beta.example.com',
                created_at: '2026-01-02T00:00:00Z'
            }
        ]);

        fixture = TestBed.createComponent(OverviewPage);
        fixture.detectChanges();

        const text = fixture.nativeElement.textContent;
        expect(fetchOverview).toHaveBeenCalledTimes(1);
        expect(text).toContain('alpha.example.com');
        expect(text).toContain('beta.example.com');
        expect(text).toContain('120');
        expect(text).toContain('45');
        expect(text).toContain('30');
        expect(text).toContain('12');
    });

    it('keeps successful site cards visible when one site stats request fails', () => {
        vi.spyOn(statsService, 'fetchSitesOverviewStats').mockReturnValue(of(overviewResponse(statsWithTraffic('site-alpha', 120, 45), statsError('site-beta'))));
        siteService.applySites([
            {
                id: 'site-alpha',
                user_id: 'user-1',
                domain: 'alpha.example.com',
                created_at: '2026-01-01T00:00:00Z'
            },
            {
                id: 'site-beta',
                user_id: 'user-1',
                domain: 'beta.example.com',
                created_at: '2026-01-02T00:00:00Z'
            }
        ]);

        fixture = TestBed.createComponent(OverviewPage);
        fixture.detectChanges();

        const text = fixture.nativeElement.textContent;
        expect(text).toContain('alpha.example.com');
        expect(text).toContain('120');
        expect(text).toContain('beta.example.com');
        expect(text).toContain('Stats unavailable');
    });

    it('filters site cards by domain', () => {
        vi.spyOn(statsService, 'fetchSitesOverviewStats').mockReturnValue(of(overviewResponse(statsWithTraffic('site-alpha', 10, 4), statsWithTraffic('site-beta', 10, 4))));
        siteService.applySites([
            {
                id: 'site-alpha',
                user_id: 'user-1',
                domain: 'alpha.example.com',
                created_at: '2026-01-01T00:00:00Z'
            },
            {
                id: 'site-beta',
                user_id: 'user-1',
                domain: 'beta.example.com',
                created_at: '2026-01-02T00:00:00Z'
            }
        ]);
        fixture = TestBed.createComponent(OverviewPage);
        fixture.detectChanges();

        const input = fixture.nativeElement.querySelector('input[type="search"]') as HTMLInputElement;
        input.value = 'beta';
        input.dispatchEvent(new Event('input'));
        fixture.detectChanges();

        const text = fixture.nativeElement.textContent;
        expect(text).not.toContain('alpha.example.com');
        expect(text).toContain('beta.example.com');
    });

    it('does not label totals-only stats as no traffic', () => {
        vi.spyOn(statsService, 'fetchSitesOverviewStats').mockReturnValue(of(overviewResponse({ ...statsWithTraffic('site-alpha', 10, 4), chart_data: [] })));
        siteService.applySites([
            {
                id: 'site-alpha',
                user_id: 'user-1',
                domain: 'alpha.example.com',
                created_at: '2026-01-01T00:00:00Z'
            }
        ]);

        fixture = TestBed.createComponent(OverviewPage);
        fixture.detectChanges();

        const text = fixture.nativeElement.textContent;
        const chart = fixture.nativeElement.querySelector('.hk-overview-card__chart[role="img"]') as HTMLElement;
        expect(text).toContain('10');
        expect(text).not.toContain('No traffic');
        expect(chart.getAttribute('aria-label')).toBe('Traffic trend for alpha.example.com');
    });

    it('selects a site before opening its dashboard', () => {
        vi.spyOn(statsService, 'fetchSitesOverviewStats').mockReturnValue(of(overviewResponse(statsWithTraffic('site-alpha', 120, 45), statsWithTraffic('site-beta', 30, 12))));
        siteService.applySites([
            {
                id: 'site-alpha',
                user_id: 'user-1',
                domain: 'alpha.example.com',
                created_at: '2026-01-01T00:00:00Z'
            },
            {
                id: 'site-beta',
                user_id: 'user-1',
                domain: 'beta.example.com',
                created_at: '2026-01-02T00:00:00Z'
            }
        ]);
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        fixture = TestBed.createComponent(OverviewPage);
        fixture.detectChanges();

        const cards = Array.from<HTMLElement>(fixture.nativeElement.querySelectorAll('.hk-overview-card__content'));
        const betaCard = cards.find((card) => card.textContent?.includes('beta.example.com'));
        expect(betaCard).toBeTruthy();

        betaCard?.querySelector('button')?.click();
        fixture.detectChanges();

        expect(siteService.activeSite()?.id).toBe('site-beta');
        expect(navigate).toHaveBeenCalledWith('/dashboard');
    });

    it('can sort cards by pageviews', () => {
        vi.spyOn(statsService, 'fetchSitesOverviewStats').mockReturnValue(of(overviewResponse(statsWithTraffic('site-alpha', 30, 12), statsWithTraffic('site-beta', 120, 45))));
        siteService.applySites([
            {
                id: 'site-alpha',
                user_id: 'user-1',
                domain: 'alpha.example.com',
                created_at: '2026-01-01T00:00:00Z'
            },
            {
                id: 'site-beta',
                user_id: 'user-1',
                domain: 'beta.example.com',
                created_at: '2026-01-02T00:00:00Z'
            }
        ]);
        fixture = TestBed.createComponent(OverviewPage);
        fixture.detectChanges();

        (fixture.componentInstance as unknown as { setSort: (value: 'pageviews') => void }).setSort('pageviews');
        fixture.detectChanges();

        const headings = Array.from<HTMLElement>(fixture.nativeElement.querySelectorAll('.hk-overview-card__identity h2')).map((heading) => heading.textContent?.trim());
        expect(headings).toEqual(['beta.example.com', 'alpha.example.com']);
    });

    it('labels search and repeated dashboard actions for assistive technology', () => {
        vi.spyOn(statsService, 'fetchSitesOverviewStats').mockReturnValue(of(overviewResponse(statsWithTraffic('site-alpha', 120, 45), statsWithTraffic('site-beta', 30, 12))));
        siteService.applySites([
            {
                id: 'site-alpha',
                user_id: 'user-1',
                domain: 'alpha.example.com',
                created_at: '2026-01-01T00:00:00Z'
            },
            {
                id: 'site-beta',
                user_id: 'user-1',
                domain: 'beta.example.com',
                created_at: '2026-01-02T00:00:00Z'
            }
        ]);

        fixture = TestBed.createComponent(OverviewPage);
        fixture.detectChanges();

        const searchLabel = fixture.nativeElement.querySelector('label[for="overview-site-search"]') as HTMLLabelElement;
        const dashboardButtons = Array.from<HTMLButtonElement>(fixture.nativeElement.querySelectorAll('button[aria-label^="Open dashboard for"]'));

        expect(searchLabel.textContent?.trim()).toBe('Filter sites');
        expect(dashboardButtons.map((button) => button.getAttribute('aria-label'))).toEqual(['Open dashboard for alpha.example.com', 'Open dashboard for beta.example.com']);
    });

    it('recomputes relative ranges when the overview is refreshed', () => {
        vi.useFakeTimers();
        try {
            vi.setSystemTime(new Date('2026-07-01T12:00:00.000Z'));
            const fetchOverview = vi.spyOn(statsService, 'fetchSitesOverviewStats').mockReturnValue(of(overviewResponse(statsWithTraffic('site-alpha', 10, 4))));
            siteService.applySites([
                {
                    id: 'site-alpha',
                    user_id: 'user-1',
                    domain: 'alpha.example.com',
                    created_at: '2026-01-01T00:00:00Z'
                }
            ]);

            fixture = TestBed.createComponent(OverviewPage);
            fixture.detectChanges();

            expect(fetchOverview.mock.calls[0]?.[1]).toBe('2026-07-01T12:00:00.000Z');

            vi.setSystemTime(new Date('2026-07-02T12:00:00.000Z'));
            (fixture.componentInstance as unknown as { refreshOverview: () => void }).refreshOverview();

            expect(fetchOverview).toHaveBeenCalledTimes(2);
            expect(fetchOverview.mock.calls[1]?.[1]).toBe('2026-07-02T12:00:00.000Z');
        } finally {
            vi.useRealTimers();
        }
    });
});

function overviewResponse(...sites: SiteOverviewStats[]): SitesOverviewStatsResponse {
    return { sites };
}

function statsError(siteId: string): SiteOverviewStats {
    return {
        site_id: siteId,
        status: 'error',
        total_pageviews: 0,
        unique_sessions: 0,
        bounce_rate: 0,
        chart_data: [],
        error: 'stats_unavailable'
    };
}

function statsWithTraffic(siteId: string, pageviews: number, visitors: number): SiteOverviewStats {
    return {
        site_id: siteId,
        status: 'ready',
        total_pageviews: pageviews,
        unique_sessions: visitors,
        bounce_rate: 42.5,
        chart_data: [
            { time: '2026-07-01T00:00:00Z', pageviews: Math.max(1, Math.round(pageviews / 3)), visitors: Math.max(1, Math.round(visitors / 3)) },
            { time: '2026-07-02T00:00:00Z', pageviews, visitors }
        ]
    };
}
