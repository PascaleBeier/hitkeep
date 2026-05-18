import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';

import { AIVisibility } from '@pages/ai-visibility/ai-visibility';
import { SiteService } from '@features/sites/services/site.service';

describe('AIVisibility', () => {
    let component: AIVisibility;
    let fixture: ComponentFixture<AIVisibility>;
    let httpMock: HttpTestingController;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                AIVisibility,
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

        httpMock = TestBed.inject(HttpTestingController);
        fixture = TestBed.createComponent(AIVisibility);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should expose correlation rows as filterable metric cards', () => {
        const instance = component as AIVisibility & {
            correlation: { set(value: unknown): void };
            metricCardTabs: () => { id: string; cards: { id: string; activeValue?: string | null; filterType?: string; data: { name: string; value: number }[] }[] }[];
            onMetricCardClick: (event: { tabId: string; cardId: string; filterType: 'path' | 'assistantName'; metric: { name: string; value: number } }) => void;
            filters: () => { path?: string | null; assistantName?: string | null };
        };

        instance.correlation.set({
            summary: {
                total_fetches: 0,
                fetched_paths: 0,
                correlated_paths: 0,
                ai_referred_visits: 0,
                uncorrelated_fetches: 0
            },
            citation_yield: [{ path: '/docs', assistant_name: 'GPTBot', fetch_count: 12, ai_referred_visits: 4, citation_yield_pct: 33.3 }],
            opportunity_pages: [{ path: '/pricing', fetch_count: 21, ai_referred_visits: 0, error_requests: 1, error_rate_pct: 4.8 }],
            failure_hotspots: [{ assistant_name: 'ClaudeBot', path_prefix: '/api', total_requests: 9, error_requests: 2, error_rate_pct: 22.2 }]
        });

        const correlationGroup = instance.metricCardTabs().find((tab) => tab.id === 'correlation');

        expect(correlationGroup?.cards.map((card) => card.id)).toEqual(['citation-yield', 'opportunity-pages', 'failure-hotspots']);
        expect(correlationGroup?.cards[0]?.filterType).toBe('path');
        expect(correlationGroup?.cards[0]?.data).toEqual([{ name: '/docs', value: 4 }]);
        expect(correlationGroup?.cards[1]?.data).toEqual([{ name: '/pricing', value: 21 }]);
        expect(correlationGroup?.cards[2]?.filterType).toBe('assistantName');
        expect(correlationGroup?.cards[2]?.data).toEqual([{ name: 'ClaudeBot', value: 2 }]);

        instance.onMetricCardClick({
            tabId: 'correlation',
            cardId: 'citation-yield',
            filterType: 'path',
            metric: { name: '/docs', value: 4 }
        });

        expect(instance.filters().path).toBe('/docs');
    });

    it('should keep the headline KPIs focused and move supporting metrics into compact strips', () => {
        const instance = component as AIVisibility & {
            overview: { set(value: unknown): void };
            correlation: { set(value: unknown): void };
            primaryKpiCards: () => { label: string; value: string | number; loading: boolean }[];
            healthStats: () => { label: string; value: string | number; loading: boolean }[];
            correlationSummaryStats: () => { label: string; value: string | number; loading: boolean }[];
        };

        instance.overview.set({
            total_requests: 42,
            unique_paths: 4,
            unique_assistants: 3,
            error_rate_4xx: 1.2,
            error_rate_5xx: 0.8,
            median_response_ms: 123,
            total_bytes: 2048,
            top_assistants: [],
            top_families: [],
            top_paths: [],
            top_error_paths: [],
            resource_type_split: []
        });
        instance.correlation.set({
            summary: {
                total_fetches: 42,
                fetched_paths: 4,
                correlated_paths: 2,
                ai_referred_visits: 5,
                uncorrelated_fetches: 7
            },
            citation_yield: [{ path: '/docs', assistant_name: 'GPTBot', fetch_count: 12, ai_referred_visits: 4, citation_yield_pct: 33.3 }],
            opportunity_pages: [{ path: '/pricing', fetch_count: 21, ai_referred_visits: 0, error_requests: 1, error_rate_pct: 4.8 }],
            failure_hotspots: [{ assistant_name: 'ClaudeBot', path_prefix: '/api', total_requests: 9, error_requests: 2, error_rate_pct: 22.2 }]
        });

        expect(instance.primaryKpiCards().map((card) => card.value)).toEqual([42, 5, 3, '2.0%']);
        expect(instance.healthStats().map((stat) => stat.value)).toEqual([4, '1.2%', '0.8%', '123 ms', '2 KB']);
        expect(instance.correlationSummaryStats().map((stat) => stat.value)).toEqual([2, 7]);
    });

    it('should render one headline row and compact supporting metric strips', () => {
        const siteService = TestBed.inject(SiteService);
        siteService.activeSite.set({
            id: 'site-1',
            domain: 'example.com',
            created_at: '',
            user_id: 'user-1'
        });

        fixture.detectChanges();

        httpMock
            .expectOne((req) => req.url === '/api/sites/site-1/ai-fetch/overview')
            .flush({
                total_requests: 42,
                unique_paths: 4,
                unique_assistants: 3,
                error_rate_4xx: 1.2,
                error_rate_5xx: 0.8,
                median_response_ms: 123,
                total_bytes: 2048,
                top_assistants: [{ name: 'GPTBot', value: 12 }],
                top_families: [{ name: 'OpenAI', value: 12 }],
                top_paths: [{ name: '/docs', value: 8 }],
                top_error_paths: [{ name: '/api', value: 2 }],
                resource_type_split: [{ name: 'html', value: 10 }]
            });
        httpMock.expectOne((req) => req.url === '/api/sites/site-1/ai-fetch/timeseries').flush([{ time: '2026-05-18T00:00:00Z', count: 42 }]);
        httpMock
            .expectOne((req) => req.url === '/api/sites/site-1/ai-fetch/correlation')
            .flush({
                summary: {
                    total_fetches: 42,
                    fetched_paths: 4,
                    correlated_paths: 2,
                    ai_referred_visits: 5,
                    uncorrelated_fetches: 7
                },
                citation_yield: [{ path: '/docs', assistant_name: 'GPTBot', fetch_count: 12, ai_referred_visits: 4, citation_yield_pct: 33.3 }],
                opportunity_pages: [{ path: '/pricing', fetch_count: 21, ai_referred_visits: 0, error_requests: 1, error_rate_pct: 4.8 }],
                failure_hotspots: [{ assistant_name: 'ClaudeBot', path_prefix: '/api', total_requests: 9, error_requests: 2, error_rate_pct: 22.2 }]
            });

        fixture.detectChanges();

        expect(fixture.nativeElement.querySelectorAll('[data-testid="ai-visibility-headline-kpis"] app-kpi-card').length).toBe(4);
        expect(fixture.nativeElement.querySelectorAll('[data-testid="ai-visibility-health-strip"] .ai-visibility__stat').length).toBe(5);
        expect(fixture.nativeElement.querySelectorAll('[data-testid="ai-visibility-correlation-summary"] .ai-visibility__stat').length).toBe(2);
    });

    it('should link to the AI fetch setup guide from the toolbar', () => {
        const setupGuideLink = fixture.nativeElement.querySelector('a[href="https://hitkeep.com/guides/tracking/ai-fetch-ingest/"]');

        expect(setupGuideLink).not.toBeNull();
        expect(setupGuideLink.getAttribute('target')).toBe('_blank');
        expect(setupGuideLink.textContent).toContain('aiVisibility.docsAction');
    });

    it('should not render a setup-required callout when a site is selected but no AI fetches exist', () => {
        const siteService = TestBed.inject(SiteService);
        siteService.activeSite.set({
            id: 'site-1',
            domain: 'example.com',
            created_at: '',
            user_id: 'user-1'
        });

        fixture.detectChanges();

        httpMock
            .expectOne((req) => req.url === '/api/sites/site-1/ai-fetch/overview')
            .flush({
                total_requests: 0,
                unique_paths: 0,
                unique_assistants: 0,
                error_rate_4xx: 0,
                error_rate_5xx: 0,
                median_response_ms: 0,
                total_bytes: 0,
                top_assistants: [],
                top_families: [],
                top_paths: [],
                top_error_paths: [],
                resource_type_split: []
            });
        httpMock.expectOne((req) => req.url === '/api/sites/site-1/ai-fetch/timeseries').flush([]);
        httpMock
            .expectOne((req) => req.url === '/api/sites/site-1/ai-fetch/correlation')
            .flush({
                summary: {
                    total_fetches: 0,
                    fetched_paths: 0,
                    correlated_paths: 0,
                    ai_referred_visits: 0,
                    uncorrelated_fetches: 0
                },
                citation_yield: [],
                opportunity_pages: [],
                failure_hotspots: []
            });

        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).not.toContain('Server-side fetch capture required');
    });
});
