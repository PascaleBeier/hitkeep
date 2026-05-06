import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { Dashboard } from '@pages/dashboard/dashboard';
import { SiteService } from '@features/sites/services/site.service';
import { StatsService } from '@features/analytics/services/stats.service';
import { HitService } from '@features/hits/services/hit.service';
import { TeamService } from '@services/team.service';
import { OnboardingService } from '@services/onboarding.service';
import { GoogleSearchConsoleService } from '@services/google-search-console.service';

describe('Dashboard', () => {
    let component: Dashboard;
    let fixture: ComponentFixture<Dashboard>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                Dashboard,
                TranslocoTestingModule.forRoot({
                    langs: { en: {} },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                provideHttpClient(),
                provideRouter([]),
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: {
                        en: 'en-US',
                        'en-US': 'en-US'
                    }
                }),
                {
                    provide: GoogleSearchConsoleService,
                    useValue: {
                        getSiteMapping: vi.fn(() => of({ site_id: 'site-1', team_id: 'team-1', mapped: false, can_manage: false })),
                        getOverview: vi.fn(),
                        getSeries: vi.fn(),
                        getQueries: vi.fn(),
                        getPages: vi.fn(),
                        getBreakdown: vi.fn()
                    }
                }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(Dashboard);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should show team onboarding copy when the active team has no sites', () => {
        const siteService = TestBed.inject(SiteService);
        const teamService = TestBed.inject(TeamService);

        siteService.sites.set([]);
        siteService.activeSite.set(null);
        teamService.teams.set([
            {
                id: 'team-1',
                name: 'Acme Growth',
                logo_url: '',
                role: 'owner',
                created_at: '2026-01-01T00:00:00Z'
            }
        ]);
        teamService.activeTeamId.set('team-1');

        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('dashboard.empty.teamTitle');
    });

    it('should render onboarding progress as a non-clickable rail', () => {
        const onboardingService = TestBed.inject(OnboardingService);

        onboardingService.onboarding.set({
            dismissed: false,
            complete: false,
            steps: [
                { key: 'create_site', complete: true },
                { key: 'verify_tracking', complete: false },
                { key: 'automatic_events', complete: false },
                { key: 'invite_teammate', complete: false },
                { key: 'schedule_report', complete: false }
            ]
        });

        fixture.detectChanges();

        const rail = fixture.nativeElement.querySelector('app-workflow-progress') as HTMLElement;
        expect(rail).toBeTruthy();
        expect(rail.querySelectorAll('button, a, [role="button"]').length).toBe(0);
        expect(rail.querySelectorAll('[aria-current="step"]').length).toBe(1);
        expect(rail.querySelector('[aria-current="step"]')?.textContent).toContain('dashboard.onboarding.steps.verify_tracking');
    });

    it('should switch the pages card data between top, landing, and exit pages', () => {
        const siteService = TestBed.inject(SiteService);
        const statsService = TestBed.inject(StatsService);
        const hitService = TestBed.inject(HitService);

        vi.spyOn(statsService, 'loadStats').mockImplementation(() => undefined);
        vi.spyOn(hitService, 'loadHits').mockImplementation(() => undefined);

        siteService.activeSite.set({
            id: 'site-1',
            user_id: 'user-1',
            domain: 'example.com',
            created_at: '2026-01-01T00:00:00Z'
        });

        statsService.stats.set({
            live_visitors: 0,
            total_pageviews: 10,
            unique_sessions: 5,
            bounce_rate: 40,
            avg_session_duration: 12,
            pages_per_session: 2,
            chart_data: [],
            top_pages: [{ name: '/pricing', value: 4 }],
            top_landing_pages: [{ name: '/blog', value: 3 }],
            top_exit_pages: [{ name: '/signup', value: 2 }],
            top_referrers: [],
            top_devices: [],
            top_countries: [],
            top_browsers: [],
            top_ai_bots: [],
            top_ai_sources: [],
            top_languages: [{ name: 'de', value: 4 }],
            top_utm_campaigns: [],
            top_utm_contents: [],
            top_utm_mediums: [],
            top_utm_sources: [],
            top_utm_terms: [],
            ai_bot_hits: 0,
            ai_source_visits: 0,
            utm_campaign_hits: 0,
            utm_content_hits: 0,
            utm_medium_hits: 0,
            utm_source_hits: 0,
            utm_term_hits: 0,
            goals: [],
            funnels: []
        });

        expect((component as unknown as { pageMetricData: () => { name: string; value: number }[] }).pageMetricData()).toEqual([{ name: '/pricing', value: 4 }]);

        (component as unknown as { onPageMetricModeChange: (mode: string) => void }).onPageMetricModeChange('top_landing_pages');
        expect((component as unknown as { pageMetricData: () => { name: string; value: number }[] }).pageMetricData()).toEqual([{ name: '/blog', value: 3 }]);

        (component as unknown as { onPageMetricModeChange: (mode: string) => void }).onPageMetricModeChange('top_exit_pages');
        expect((component as unknown as { pageMetricData: () => { name: string; value: number }[] }).pageMetricData()).toEqual([{ name: '/signup', value: 2 }]);
    });

    it('should expose countries and languages from stats as separate data sources', () => {
        const siteService = TestBed.inject(SiteService);
        const statsService = TestBed.inject(StatsService);
        const hitService = TestBed.inject(HitService);

        vi.spyOn(statsService, 'loadStats').mockImplementation(() => undefined);
        vi.spyOn(hitService, 'loadHits').mockImplementation(() => undefined);

        siteService.activeSite.set({
            id: 'site-1',
            user_id: 'user-1',
            domain: 'example.com',
            created_at: '2026-01-01T00:00:00Z'
        });

        statsService.stats.set({
            live_visitors: 0,
            total_pageviews: 10,
            unique_sessions: 5,
            bounce_rate: 40,
            avg_session_duration: 12,
            pages_per_session: 2,
            chart_data: [],
            top_pages: [],
            top_landing_pages: [],
            top_exit_pages: [],
            top_referrers: [],
            top_devices: [],
            top_browsers: [],
            top_countries: [{ name: 'DE', value: 4 }],
            top_ai_bots: [],
            top_ai_sources: [],
            top_languages: [{ name: 'de', value: 3 }],
            top_utm_campaigns: [],
            top_utm_contents: [],
            top_utm_mediums: [],
            top_utm_sources: [],
            top_utm_terms: [],
            ai_bot_hits: 0,
            ai_source_visits: 0,
            utm_campaign_hits: 0,
            utm_content_hits: 0,
            utm_medium_hits: 0,
            utm_source_hits: 0,
            utm_term_hits: 0,
            goals: [],
            funnels: []
        });

        expect(statsService.stats()?.top_countries).toEqual([{ name: 'DE', value: 4 }]);
        expect(statsService.stats()?.top_languages).toEqual([{ name: 'de', value: 3 }]);
    });

    it('should refresh Search Console drilldown data from the shared dashboard refresh action', async () => {
        const siteService = TestBed.inject(SiteService);
        const statsService = TestBed.inject(StatsService);
        const hitService = TestBed.inject(HitService);
        const searchConsoleService = TestBed.inject(GoogleSearchConsoleService) as unknown as {
            getSiteMapping: ReturnType<typeof vi.fn>;
            getOverview: ReturnType<typeof vi.fn>;
            getSeries: ReturnType<typeof vi.fn>;
            getQueries: ReturnType<typeof vi.fn>;
            getPages: ReturnType<typeof vi.fn>;
            getBreakdown: ReturnType<typeof vi.fn>;
        };

        vi.spyOn(statsService, 'loadStats').mockImplementation(() => undefined);
        vi.spyOn(hitService, 'loadHits').mockImplementation(() => undefined);
        searchConsoleService.getSiteMapping.mockReturnValue(
            of({
                site_id: 'site-1',
                team_id: 'team-1',
                mapped: true,
                property_uri: 'sc-domain:example.com',
                can_manage: true
            })
        );
        searchConsoleService.getOverview.mockReturnValue(of({ data_source: 'google_search_console', clicks: 1, impressions: 10, ctr: 0.1, average_position: 2 }));
        searchConsoleService.getSeries.mockReturnValue(of({ data_source: 'google_search_console', series: [] }));
        searchConsoleService.getQueries.mockReturnValue(of({ data_source: 'google_search_console', rows: [] }));
        searchConsoleService.getPages.mockReturnValue(of({ data_source: 'google_search_console', rows: [] }));
        searchConsoleService.getBreakdown.mockReturnValue(of({ data_source: 'google_search_console', rows: [] }));

        siteService.activeSite.set({
            id: 'site-1',
            user_id: 'user-1',
            domain: 'example.com',
            created_at: '2026-01-01T00:00:00Z'
        });
        fixture.detectChanges();
        await fixture.whenStable();

        searchConsoleService.getSiteMapping.mockClear();
        searchConsoleService.getOverview.mockClear();
        component.refreshAll();
        fixture.detectChanges();
        await fixture.whenStable();

        expect(searchConsoleService.getSiteMapping).not.toHaveBeenCalled();
        expect(searchConsoleService.getOverview).toHaveBeenCalledTimes(1);
    });

    it('should render Search Console data for a mapped site even when HitKeep has no hits', async () => {
        const siteService = TestBed.inject(SiteService);
        const statsService = TestBed.inject(StatsService);
        const hitService = TestBed.inject(HitService);
        const searchConsoleService = TestBed.inject(GoogleSearchConsoleService) as unknown as {
            getSiteMapping: ReturnType<typeof vi.fn>;
            getOverview: ReturnType<typeof vi.fn>;
            getSeries: ReturnType<typeof vi.fn>;
            getQueries: ReturnType<typeof vi.fn>;
            getPages: ReturnType<typeof vi.fn>;
            getBreakdown: ReturnType<typeof vi.fn>;
        };

        vi.spyOn(statsService, 'loadStats').mockImplementation(() => undefined);
        vi.spyOn(hitService, 'loadHits').mockImplementation(() => undefined);
        statsService.stats.set({
            live_visitors: 0,
            total_pageviews: 0,
            unique_sessions: 0,
            bounce_rate: 0,
            avg_session_duration: 0,
            pages_per_session: 0,
            chart_data: [],
            top_pages: [],
            top_landing_pages: [],
            top_exit_pages: [],
            top_referrers: [],
            top_devices: [],
            top_countries: [],
            top_browsers: [],
            top_ai_bots: [],
            top_ai_sources: [],
            top_languages: [],
            top_utm_campaigns: [],
            top_utm_contents: [],
            top_utm_mediums: [],
            top_utm_sources: [],
            top_utm_terms: [],
            ai_bot_hits: 0,
            ai_source_visits: 0,
            utm_campaign_hits: 0,
            utm_content_hits: 0,
            utm_medium_hits: 0,
            utm_source_hits: 0,
            utm_term_hits: 0,
            goals: [],
            funnels: []
        });
        hitService.hits.set([]);
        hitService.total.set(0);
        searchConsoleService.getSiteMapping.mockReturnValue(
            of({
                site_id: 'site-1',
                team_id: 'team-1',
                mapped: true,
                property_uri: 'sc-domain:example.com',
                can_manage: true
            })
        );
        searchConsoleService.getOverview.mockReturnValue(of({ data_source: 'google_search_console', clicks: 24, impressions: 180, ctr: 0.1333, average_position: 3.4 }));
        searchConsoleService.getSeries.mockReturnValue(
            of({
                data_source: 'google_search_console',
                series: [{ date: '2026-05-01', clicks: 24, impressions: 180, ctr: 0.1333, average_position: 3.4 }]
            })
        );
        searchConsoleService.getQueries.mockReturnValue(of({ data_source: 'google_search_console', rows: [{ value: 'privacy analytics', clicks: 24, impressions: 180, ctr: 0.1333, average_position: 3.4 }] }));
        searchConsoleService.getPages.mockReturnValue(of({ data_source: 'google_search_console', rows: [{ value: 'https://example.com/pricing', clicks: 10, impressions: 90, ctr: 0.1111, average_position: 4.1 }] }));
        searchConsoleService.getBreakdown.mockImplementation((_siteID: string, dimension: string) =>
            of({
                data_source: 'google_search_console',
                rows: dimension === 'country' ? [{ value: 'usa', clicks: 14, impressions: 100, ctr: 0.14, average_position: 3 }] : [{ value: 'desktop', clicks: 12, impressions: 80, ctr: 0.15, average_position: 2.8 }]
            })
        );

        siteService.activeSite.set({
            id: 'site-1',
            user_id: 'user-1',
            domain: 'example.com',
            created_at: '2026-01-01T00:00:00Z'
        });
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('privacy analytics');
        expect(fixture.nativeElement.textContent).toContain('/pricing');
        expect(searchConsoleService.getOverview).toHaveBeenCalled();
        expect(hitService.hits()).toEqual([]);
    });

    it('should render Search Console after the primary dashboard sections', async () => {
        const siteService = TestBed.inject(SiteService);
        const statsService = TestBed.inject(StatsService);
        const hitService = TestBed.inject(HitService);

        vi.spyOn(statsService, 'loadStats').mockImplementation(() => undefined);
        vi.spyOn(hitService, 'loadHits').mockImplementation(() => undefined);

        siteService.activeSite.set({
            id: 'site-1',
            user_id: 'user-1',
            domain: 'example.com',
            created_at: '2026-01-01T00:00:00Z'
        });
        fixture.detectChanges();
        await fixture.whenStable();

        const rawHitsTable = fixture.nativeElement.querySelector('p-table') as HTMLElement | null;
        const searchConsoleDrilldown = fixture.nativeElement.querySelector('app-search-console-drilldown') as HTMLElement | null;

        expect(rawHitsTable).toBeTruthy();
        expect(searchConsoleDrilldown).toBeTruthy();
        expect(rawHitsTable!.compareDocumentPosition(searchConsoleDrilldown!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });
});
