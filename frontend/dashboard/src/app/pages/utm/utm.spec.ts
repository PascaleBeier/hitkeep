import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { TRANSLOCO_LOCALE_CONFIG, TRANSLOCO_LOCALE_LANG_MAPPING, TranslocoLocaleService } from '@jsverse/transloco-locale';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { StatsService } from '@features/analytics/services/stats.service';
import { SiteService } from '@features/sites/services/site.service';
import { UtmDashboard } from './utm';

describe('UtmDashboard', () => {
    let fixture: ComponentFixture<UtmDashboard>;
    const stats = signal({
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
        top_cities: [],
        top_providers: [],
        top_asns: [],
        top_languages: [],
        top_utm_campaigns: [{ name: 'launch', value: 12 }],
        top_utm_contents: [{ name: 'hero', value: 7 }],
        top_utm_mediums: [{ name: 'cpc', value: 8 }],
        top_utm_sources: [{ name: 'google', value: 10 }],
        top_utm_terms: [{ name: 'analytics', value: 4 }],
        utm_campaign_hits: 12,
        utm_content_hits: 7,
        utm_medium_hits: 8,
        utm_source_hits: 10,
        utm_term_hits: 4,
        goals: [],
        funnels: []
    });
    const statsServiceStub = {
        stats,
        isLoading: signal(false),
        currentComparisonRange: signal(null),
        loadStats: vi.fn()
    };

    beforeEach(async () => {
        statsServiceStub.loadStats.mockClear();

        await TestBed.configureTestingModule({
            imports: [
                UtmDashboard,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { utm: 'UTM' },
                            common: {
                                loadingSiteData: 'Loading site data...',
                                noSiteSelected: 'No site selected',
                                noActiveFilter: 'No active filter',
                                removeFilterAria: 'Remove filter',
                                selectDateRange: 'Select date range',
                                actions: { apply: 'Apply', cancel: 'Cancel', clearAll: 'Clear all' },
                                metricGroups: { acquisition: 'Acquisition' }
                            },
                            dashboard: {
                                kpis: { pageviews: 'Pageviews' },
                                traffic: { visitors: 'Visitors' }
                            },
                            utm: {
                                kpis: {
                                    campaign: 'Campaign',
                                    content: 'Content',
                                    medium: 'Medium',
                                    source: 'Source',
                                    term: 'Term'
                                },
                                metrics: {
                                    topCampaigns: 'Campaigns',
                                    topSources: 'Sources',
                                    topMediums: 'Mediums',
                                    topContents: 'Contents',
                                    topTerms: 'Terms'
                                },
                                filters: {
                                    campaign: 'Campaign: {{value}}',
                                    content: 'Content: {{value}}',
                                    medium: 'Medium: {{value}}',
                                    source: 'Source: {{value}}',
                                    term: 'Term: {{value}}'
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
                provideRouter([]),
                {
                    provide: SiteService,
                    useValue: {
                        activeSite: signal({
                            id: 'site-1',
                            user_id: 'user-1',
                            domain: 'utm.test',
                            created_at: '2026-05-01T00:00:00Z'
                        }),
                        isLoading: signal(false)
                    }
                },
                { provide: StatsService, useValue: statsServiceStub },
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

        fixture = TestBed.createComponent(UtmDashboard);
        fixture.detectChanges();
    });

    it('keeps UTM metric cards acquisition-only and preserves metric filter clicks', () => {
        const component = fixture.componentInstance as unknown as {
            metricCardTabs: () => { id: string; cards: { id: string; linkMode?: string }[] }[];
            onMetricCardClick: (event: { tabId: string; cardId: string; filterType: string; metric: { name: string; value: number } }) => void;
            activeFilters: () => { type: string; value: string }[];
            activeFilterValue: (type: 'utm_source') => string | null;
        };

        const tabs = component.metricCardTabs();

        expect(tabs.map((tab) => tab.id)).toEqual(['acquisition']);
        expect(tabs[0]?.cards.map((card) => card.id)).toEqual(['campaigns', 'sources', 'mediums', 'contents', 'terms']);
        expect(tabs[0]?.cards.find((card) => card.id === 'sources')?.linkMode).toBeUndefined();

        component.onMetricCardClick({
            tabId: 'acquisition',
            cardId: 'sources',
            filterType: 'utm_source',
            metric: { name: 'google', value: 10 }
        });

        expect(component.activeFilters()).toEqual([{ type: 'utm_source', value: 'google' }]);
        expect(component.activeFilterValue('utm_source')).toBe('google');
    });
});
