import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { vi } from 'vitest';

import { Dashboard } from '@pages/dashboard/dashboard';
import { SiteService } from '@features/sites/services/site.service';
import { StatsService } from '@features/analytics/services/stats.service';
import { HitService } from '@features/hits/services/hit.service';
import { TeamService } from '@services/team.service';
import { OnboardingService } from '@services/onboarding.service';

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
                })
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
});
