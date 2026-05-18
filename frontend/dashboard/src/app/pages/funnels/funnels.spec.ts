import { TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { TRANSLOCO_LOCALE_CONFIG, TRANSLOCO_LOCALE_LANG_MAPPING, TranslocoLocaleService } from '@jsverse/transloco-locale';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { AnalyticsService } from '@services/analytics.service';
import { StatsService } from '@features/analytics/services/stats.service';
import { SiteService } from '@features/sites/services/site.service';
import { Funnels } from './funnels';

describe('Funnels', () => {
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
        loadStats: vi.fn()
    };
    const analyticsServiceStub = {
        getFunnels: vi.fn(() => of([])),
        getFunnelTimeseries: vi.fn(() => of([]))
    };

    beforeEach(async () => {
        statsServiceStub.loadStats.mockClear();
        analyticsServiceStub.getFunnels.mockClear();
        analyticsServiceStub.getFunnelTimeseries.mockClear();

        await TestBed.configureTestingModule({
            imports: [
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { funnels: 'Funnels' },
                            common: {
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
                            funnels: {
                                kpis: {
                                    funnels: 'Funnels',
                                    entries: 'Entries',
                                    completions: 'Completions',
                                    completionRate: 'Completion rate'
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
                            domain: 'funnels.test',
                            created_at: '2026-05-01T00:00:00Z'
                        }),
                        isLoading: signal(false)
                    }
                },
                { provide: StatsService, useValue: statsServiceStub },
                { provide: AnalyticsService, useValue: analyticsServiceStub },
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

    it('keeps funnel metric cards in canonical groups and preserves metric filter clicks', () => {
        const component = TestBed.runInInjectionContext(() => new Funnels()) as unknown as {
            metricCardTabs: () => { id: string; cards: { id: string }[] }[];
            onMetricCardClick: (event: { tabId: string; cardId: string; filterType: string; metric: { name: string; value: number } }) => void;
            activeFilters: () => { type: string; value: string }[];
            activeFilterValue: (type: 'provider') => string | null;
        };

        const tabs = component.metricCardTabs();

        expect(tabs.map((tab) => tab.id)).toEqual(['content', 'acquisition', 'audience', 'location', 'network']);
        expect(tabs.find((tab) => tab.id === 'location')?.cards.map((card) => card.id)).toEqual(['countries', 'cities']);
        expect(tabs.find((tab) => tab.id === 'network')?.cards.map((card) => card.id)).toEqual(['providers', 'asns']);

        component.onMetricCardClick({
            tabId: 'network',
            cardId: 'providers',
            filterType: 'provider',
            metric: { name: 'Hetzner Online GmbH', value: 2 }
        });

        expect(component.activeFilters()).toEqual([{ type: 'provider', value: 'Hetzner Online GmbH' }]);
        expect(component.activeFilterValue('provider')).toBe('Hetzner Online GmbH');
    });
});
