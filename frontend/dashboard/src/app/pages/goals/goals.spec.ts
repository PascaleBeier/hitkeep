import { TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { TRANSLOCO_LOCALE_CONFIG, TRANSLOCO_LOCALE_LANG_MAPPING, TranslocoLocaleService } from '@jsverse/transloco-locale';
import { Observable, of, Subject } from 'rxjs';
import { vi } from 'vitest';
import { provideRouter } from '@angular/router';

import { AnalyticsService } from '@services/analytics.service';
import { StatsService } from '@features/analytics/services/stats.service';
import { SiteService } from '@features/sites/services/site.service';
import { Goals } from './goals';
import { AccessService } from '@services/access.service';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { GoalSeriesPoint } from '@models/analytics.types';

describe('Goals', () => {
    const stats = signal({
        live_visitors: 0,
        total_pageviews: 0,
        unique_sessions: 4,
        bounce_rate: 0,
        avg_session_duration: 0,
        pages_per_session: 0,
        chart_data: [],
        top_pages: [{ name: '/pricing', value: 8 }],
        top_landing_pages: [],
        top_exit_pages: [],
        top_referrers: [{ name: 'https://google.com', value: 5 }],
        top_devices: [{ name: 'Desktop', value: 7 }],
        top_countries: [{ name: 'US', value: 4 }],
        top_cities: [{ name: 'Berlin', value: 3 }],
        top_providers: [{ name: 'Hetzner Online GmbH', value: 2 }],
        top_asns: [{ name: 'AS24940 Hetzner Online GmbH', value: 2 }],
        top_languages: [],
        top_utm_campaigns: [],
        top_utm_contents: [],
        top_utm_mediums: [],
        top_utm_sources: [],
        top_utm_terms: [],
        utm_campaign_hits: 0,
        utm_content_hits: 0,
        utm_medium_hits: 0,
        utm_source_hits: 0,
        utm_term_hits: 0,
        goals: [],
        funnels: []
    });
    const statsServiceStub = {
        stats,
        isLoading: signal(false),
        currentComparisonRange: signal(null),
        comparisonRange: () => null,
        loadStats: vi.fn(),
        fetchStats: vi.fn(() => of(stats()))
    };
    const analyticsServiceStub = {
        getGoals: vi.fn(() => of([])),
        getGoalTimeseries: vi.fn<() => Observable<GoalSeriesPoint[]>>(() => of<GoalSeriesPoint[]>([]))
    };

    beforeEach(async () => {
        statsServiceStub.loadStats.mockClear();
        statsServiceStub.fetchStats.mockClear();
        analyticsServiceStub.getGoals.mockClear();
        analyticsServiceStub.getGoalTimeseries.mockClear();

        await TestBed.configureTestingModule({
            imports: [
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { goals: 'Goals' },
                            common: {
                                kpis: { conversionRate: 'Conversion rate' },
                                metricGroups: {
                                    content: 'Content',
                                    acquisition: 'Acquisition',
                                    audience: 'Audience',
                                    location: 'Location',
                                    network: 'Network'
                                },
                                metrics: {
                                    topPages: 'Top pages',
                                    topSources: 'Top sources',
                                    devices: 'Devices',
                                    countries: 'Countries',
                                    cities: 'Cities',
                                    providers: 'Providers',
                                    asns: 'ASNs'
                                },
                                filters: {
                                    page: 'Page: {{value}}',
                                    source: 'Source: {{value}}',
                                    device: 'Device: {{value}}',
                                    country: 'Country: {{value}}',
                                    city: 'City: {{value}}',
                                    provider: 'Provider: {{value}}',
                                    asn: 'ASN: {{value}}'
                                }
                            },
                            dashboard: { kpis: { uniqueSessions: 'Unique sessions' } },
                            goals: {
                                kpis: {
                                    totalGoals: 'Goals',
                                    conversions: 'Conversions'
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
                {
                    provide: SiteService,
                    useValue: {
                        activeSite: signal({
                            id: 'site-1',
                            user_id: 'user-1',
                            domain: 'goals.test',
                            created_at: '2026-05-01T00:00:00Z'
                        }),
                        isLoading: signal(false)
                    }
                },
                { provide: StatsService, useValue: statsServiceStub },
                { provide: AnalyticsService, useValue: analyticsServiceStub },
                { provide: AccessService, useValue: { canSite: () => true } },
                ConfirmationService,
                provideRouter([]),
                {
                    provide: TranslocoLocaleService,
                    useValue: {
                        langChanges$: of('en'),
                        localeChanges$: of('en'),
                        getLocale: () => 'en-US',
                        localizeNumber: (value: number) => value.toString(),
                        localizeDate: (value: Date) => value.toISOString()
                    }
                },
                { provide: TRANSLOCO_LOCALE_CONFIG, useValue: {} },
                { provide: TRANSLOCO_LOCALE_LANG_MAPPING, useValue: { en: 'en-US' } }
            ]
        }).compileComponents();
    });

    it('presents one reporting subject and exactly three conversion KPIs', () => {
        const component = TestBed.runInInjectionContext(() => new Goals()) as unknown as {
            kpis: () => { label: string }[];
            subjectOptions: () => { label: string; value: string | null }[];
            selectGoal: (id: string | null) => void;
            selectedGoalId: () => string | null;
            goals: { set: (goals: { id: string; name: string; type: 'path'; value: string; created_at: string }[]) => void };
            trafficGoalIds: () => string[];
            trafficEnabled: () => boolean;
        };
        expect(component.kpis().length).toBe(3);
        expect(component.subjectOptions()[0].value).toBeNull();
        expect(component.trafficEnabled()).toBe(false);
        component.goals.set([
            { id: 'goal-1', name: 'Pricing', type: 'path', value: '/pricing', created_at: '2026-01-01T00:00:00Z' },
            { id: 'goal-2', name: 'Checkout', type: 'path', value: '/checkout', created_at: '2026-01-01T00:00:00Z' }
        ]);
        expect(component.trafficGoalIds()).toEqual(['goal-1', 'goal-2']);
        component.selectGoal('goal-2');
        expect(component.trafficGoalIds()).toEqual(['goal-2']);
        component.selectGoal(null);
        expect(component.selectedGoalId()).toBeNull();
    });

    it('ignores a late goal series response after the reporting scope changes', () => {
        const component = TestBed.runInInjectionContext(() => new Goals()) as unknown as {
            loadReporting: (siteID: string, from: string, to: string, cohortIDs: string[], seriesIDs: string[]) => void;
            goalSeries: () => { time: string; conversions: number }[];
        };
        const first = new Subject<GoalSeriesPoint[]>();
        const second = new Subject<GoalSeriesPoint[]>();
        analyticsServiceStub.getGoalTimeseries.mockReset().mockReturnValue(of<GoalSeriesPoint[]>([])).mockReturnValueOnce(first).mockReturnValueOnce(second);

        component.loadReporting('site-1', '2026-07-01', '2026-07-02', ['goal-1'], ['goal-1']);
        component.loadReporting('site-1', '2026-07-01', '2026-07-02', ['goal-2'], ['goal-2']);
        first.next([{ time: '2026-07-01', conversions: 1 }]);
        second.next([{ time: '2026-07-01', conversions: 2 }]);

        expect(component.goalSeries()).toEqual([{ time: '2026-07-01', conversions: 2 }]);
    });
});
