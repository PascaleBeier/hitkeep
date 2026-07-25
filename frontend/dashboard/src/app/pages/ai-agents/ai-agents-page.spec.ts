import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { Subject } from 'rxjs';
import { vi } from 'vitest';

import type { FilterChipItem } from '@components/filter-chip-row/filter-chip-row';
import type { PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import type { MetricCardGroupRowClick, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import type { SeriesChartPoint, SeriesDefinition } from '@features/analytics/components/series-chart';
import { SiteService } from '@features/sites/services/site.service';
import type { AIActivityReport, AIFetchCorrelationReport } from '@models/analytics.types';
import { RealtimeEvent, RealtimeService } from '@services/realtime.service';
import { ShareService } from '@services/share.service';
import { aiActivityStat, emptyAIActivityComparison, emptyAIActivityReport } from '@testing/empty-ai-activity-report';
import { flushSetupState } from '@testing/setup-state';
import { AIAgentsPage } from './ai-agents-page';

interface AgentKpiCard {
    id: string;
    label: string;
    value: number | string;
    loading: boolean;
    delta?: number | null;
    suffix?: string;
}

interface StatStrip {
    label: string;
    value: string;
}

interface PageInternals {
    breadcrumbItems: () => PageBreadcrumbItem[];
    kpiCards: () => AgentKpiCard[];
    cardGroups: () => MetricCardGroupTab[];
    correlationTabs: () => MetricCardGroupTab[];
    correlation: () => AIFetchCorrelationReport | null;
    healthStats: () => StatStrip[];
    correlationSummaryStats: () => StatStrip[];
    filterChips: () => FilterChipItem[];
    chartConfig: () => SeriesDefinition[];
    heroChartData: () => SeriesChartPoint[];
    agentName: () => string | null;
    pathName: () => string | null;
    showEnrichCallout: () => boolean;
    showHealthStrip: () => boolean;
    showFetchTools: () => boolean;
    exportUrl: () => string;
    onCardRowClick: (event: MetricCardGroupRowClick) => void;
    clearAllFilters: () => void;
}

const ACTIVITY_URL = '/api/sites/site-1/ai-activity';
const CORRELATION_URL = '/api/sites/site-1/ai-fetch/correlation';
const SETUP_STATE_URL = '/api/sites/site-1/setup-state';
const DOCS_URL = 'https://hitkeep.com/guides/tracking/ai-fetch-ingest/';

const BUCKET = '2026-07-10T00:00:00Z';

const emptyCorrelation = (overrides: Partial<AIFetchCorrelationReport> = {}): AIFetchCorrelationReport => ({
    summary: {
        total_fetches: 0,
        fetched_paths: 0,
        correlated_paths: 0,
        ai_referred_visits: 0,
        uncorrelated_fetches: 0
    },
    citation_yield: [],
    citation_paths: [],
    opportunity_pages: [],
    failure_hotspots: [],
    ...overrides
});

describe('AIAgentsPage', () => {
    let fixture: ComponentFixture<AIAgentsPage>;
    let httpMock: HttpTestingController;
    let realtimeEvents: Subject<RealtimeEvent>;

    const instance = () => fixture.componentInstance as AIAgentsPage & PageInternals;

    const site = () => TestBed.inject(SiteService).activeSite();

    const selectSite = (siteId: string | null = 'site-1'): void => {
        TestBed.inject(SiteService).activeSite.set(siteId ? { id: siteId, user_id: 'user-1', domain: 'example.com', created_at: '2026-01-01T00:00:00Z' } : null);
    };

    const create = (): void => {
        fixture = TestBed.createComponent(AIAgentsPage);
        fixture.detectChanges();
    };

    const matching = (url: string) => httpMock.match((request) => request.url === url);

    /** Answers the pending AI activity request(s) and returns their `filter` params. */
    const flushActivity = (report: AIActivityReport = emptyAIActivityReport()): string[][] => {
        const requests = matching(ACTIVITY_URL);
        const filters = requests.map((request) => request.request.params.getAll('filter') ?? []);
        for (const request of requests) request.flush(report);
        fixture.detectChanges();
        return filters;
    };

    const flushCorrelation = (report: AIFetchCorrelationReport = emptyCorrelation()): number => {
        const requests = matching(CORRELATION_URL);
        for (const request of requests) request.flush(report);
        fixture.detectChanges();
        return requests.length;
    };

    const answerSetupState = (hasAIFetches: boolean): void => flushSetupState(httpMock, 'site-1', { has_ai_fetches: hasAIFetches }, fixture);

    const clickRow = (filterType: string, name: string, cardId = 'agents'): void => {
        instance().onCardRowClick({ tabId: 'ai-activity', cardId, filterType, metric: { name, value: 1 } });
        fixture.detectChanges();
    };

    beforeEach(async () => {
        localStorage.clear();
        realtimeEvents = new Subject<RealtimeEvent>();

        await TestBed.configureTestingModule({
            imports: [
                AIAgentsPage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            aiAgents: {
                                title: 'AI Agents',
                                noSiteDescription: 'Select a site to see AI agent traffic.',
                                kpis: {
                                    aiRequests: 'AI requests',
                                    referralVisits: 'AI referral visits',
                                    pathsCrawled: 'Pages crawled',
                                    shareOfPageviews: 'Share of pageviews',
                                    note: 'Tracked hits and forwarded logs are merged.'
                                },
                                hero: {
                                    title: 'AI activity over time',
                                    description: 'How AI agents move through your site.',
                                    series: {
                                        aiRequests: 'AI requests',
                                        referralVisits: 'AI referral visits',
                                        crawlerFetches: 'Crawler fetches (log ingest)'
                                    }
                                },
                                groups: { activity: 'AI activity', byCategory: 'By category', fetchDepth: 'Fetch depth' },
                                cards: { paths: 'Pages crawled', families: 'Operators', resourceTypes: 'Resource types', errorPaths: 'Error paths' },
                                provenance: { hint: 'tracked {{tracked}} · logs {{fetched}}' },
                                enrich: {
                                    title: 'See the crawlers your tracker misses',
                                    description: 'Forward your CDN, edge or server logs.',
                                    docsAction: 'Read the setup guide'
                                },
                                filters: { scopeHits: 'tracked visits only' },
                                fetchDepth: {
                                    docsAction: 'Setup guide',
                                    kpis: {
                                        totalFetches: 'Total fetches',
                                        errorRate4xx: '4xx rate',
                                        errorRate5xx: '5xx rate',
                                        medianResponse: 'Median response',
                                        bytesServed: 'Bytes served'
                                    },
                                    correlation: {
                                        title: 'Fetch-to-visit correlation',
                                        description: 'Which fetched paths attract AI-referred visits.',
                                        note: 'Correlated visits include later AI-referred visits on the same path.',
                                        kpis: { correlatedPaths: 'Correlated paths', aiReferredVisits: 'Later AI-referred visits', uncorrelatedFetches: 'Uncorrelated fetches' }
                                    },
                                    tables: {
                                        title: 'Correlation breakdowns',
                                        citationYield: { title: 'Citation yield' },
                                        opportunityPages: { title: 'Opportunity pages' },
                                        failureHotspots: { title: 'Failure hotspots' }
                                    },
                                    exportStatus: { success: 'Export download started.', error: 'Failed to export.' }
                                }
                            },
                            common: {
                                noSiteSelected: 'No site selected',
                                noActiveFilter: 'No active filter',
                                removeFilterAria: 'Remove filter',
                                preparing: 'Preparing…',
                                actions: { clearAll: 'Clear all', refresh: 'Refresh', exportCsv: 'Export CSV' },
                                exportFormats: { csv: 'CSV', xlsx: 'Excel', parquet: 'Parquet', json: 'JSON', ndjson: 'NDJSON' },
                                filters: {
                                    aiBot: 'AI agent: {{value}}',
                                    aiBotCategory: 'AI category: {{value}}',
                                    aiSource: 'AI source: {{value}}',
                                    page: 'Page: {{value}}'
                                },
                                metrics: { aiBots: 'AI agents', aiBotCategories: 'Agent categories', aiSources: 'AI referrers' },
                                aiCategories: {
                                    ai_training_crawler: 'Training crawlers',
                                    ai_search_indexer: 'Search indexers',
                                    ai_assistant: 'Assistants',
                                    ai_agent: 'Agents',
                                    ai_coding_agent: 'Coding agents',
                                    other_ai: 'Other AI'
                                }
                            }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ],
            providers: [
                provideHttpClient(),
                provideHttpClientTesting(),
                provideRouter([]),
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: { en: 'en-US', 'en-US': 'en-US' }
                }),
                {
                    provide: RealtimeService,
                    useValue: {
                        events$: realtimeEvents.asObservable(),
                        isOpen: () => false,
                        activeSiteId: () => null
                    }
                }
            ]
        }).compileComponents();

        httpMock = TestBed.inject(HttpTestingController);
        selectSite();
    });

    afterEach(() => {
        // AI agent icons are a decorative, page-agnostic lookup; drain it plus any
        // untouched setup-state probe so `verify()` only reports real leftovers.
        for (const request of httpMock.match((candidate) => candidate.url === '/api/ai-agents')) {
            request.flush({ agents: [], ai_referrers: [] });
        }
        flushSetupState(httpMock, 'site-1');
        httpMock.verify();
    });

    it('renders a single toolbar-driven surface without the routed tab shell', () => {
        create();
        flushActivity();

        expect(fixture.nativeElement.querySelector('[data-testid="ai-agents-nav-tabs"]')).toBeNull();
        expect(fixture.nativeElement.querySelector('router-outlet')).toBeNull();
        expect(fixture.nativeElement.querySelectorAll('app-report-range-toolbar').length).toBe(1);
        expect(fixture.nativeElement.querySelector('app-page-header')).not.toBeNull();
    });

    it('breadcrumbs the active site domain followed by the page title', () => {
        create();
        flushActivity();

        expect(instance().breadcrumbItems()).toEqual([
            { label: 'example.com', favicon: site(), routerLink: '/dashboard' },
            { label: 'AI Agents', isCurrent: true }
        ]);
    });

    it('falls back to the page title alone when no site is selected', () => {
        selectSite(null);

        create();

        expect(instance().breadcrumbItems()).toEqual([{ label: 'AI Agents', isCurrent: true }]);
    });

    it('loads the range with exactly one ai-activity request carrying the comparison window', () => {
        create();

        const requests = matching(ACTIVITY_URL);
        expect(requests.length).toBe(1);
        expect(requests[0].request.method).toBe('GET');
        expect(requests[0].request.params.has('from')).toBe(true);
        expect(requests[0].request.params.has('to')).toBe(true);
        expect(requests[0].request.params.has('compare_from')).toBe(true);
        expect(requests[0].request.params.has('compare_to')).toBe(true);
        expect(requests[0].request.params.has('filter')).toBe(false);
        requests[0].flush(emptyAIActivityReport());
        fixture.detectChanges();

        httpMock.expectNone(CORRELATION_URL);
    });

    it('maps four merged KPIs with deltas from the report comparison', () => {
        create();
        flushActivity(
            emptyAIActivityReport({
                ai_requests: 120,
                referral_visits: 30,
                paths_crawled: 8,
                pageviews: 600,
                comparison: emptyAIActivityComparison({ ai_requests: 100, referral_visits: 60, paths_crawled: 4, pageviews: 400 })
            })
        );

        const cards = instance().kpiCards();
        expect(cards.map((card) => card.id)).toEqual(['ai_requests', 'referral_visits', 'paths_crawled', 'share_of_pageviews']);
        expect(cards.map((card) => card.label)).toEqual(['AI requests', 'AI referral visits', 'Pages crawled', 'Share of pageviews']);
        expect(cards.map((card) => card.value)).toEqual([120, 30, 8, 20]);
        expect(cards.map((card) => card.delta)).toEqual([20, -50, 100, -20]);
        expect(cards[3].suffix).toBe('%');
        expect(fixture.nativeElement.querySelectorAll('[data-testid="ai-agent-traffic-kpis"] app-kpi-card').length).toBe(4);
        expect(fixture.nativeElement.textContent).toContain('Tracked hits and forwarded logs are merged.');
    });

    it('leaves deltas empty without a comparison baseline', () => {
        create();
        flushActivity(emptyAIActivityReport({ ai_requests: 120, pageviews: 600 }));

        expect(
            instance()
                .kpiCards()
                .map((card) => card.delta)
        ).toEqual([null, null, null, null]);
    });

    it('renders the share of pageviews as zero without pageviews', () => {
        create();
        flushActivity(emptyAIActivityReport({ ai_requests: 5, pageviews: 0 }));

        expect(instance().kpiCards()[3].value).toBe(0);
    });

    it('feeds the hero chart from the report series with two series while no logs contribute', () => {
        create();
        flushActivity(
            emptyAIActivityReport({
                series: [{ time: BUCKET, ai_requests: 12, tracked_hits: 12, fetch_count: 0, referral_visits: 3 }]
            })
        );

        expect(instance().heroChartData()).toEqual([{ time: BUCKET, ai_requests: 12, tracked_hits: 12, fetch_count: 0, referral_visits: 3 }]);
        expect(
            instance()
                .chartConfig()
                .map((series) => series.key)
        ).toEqual(['ai_requests', 'referral_visits']);
        expect(fixture.nativeElement.querySelector('[data-testid="ai-agents-hero-chart"]')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('[data-testid="ai-agents-page-chips"]')).not.toBeNull();
    });

    it('adds the dashed fetch sub-series once forwarded logs contribute', () => {
        create();
        flushActivity(
            emptyAIActivityReport({
                fetch_count: 40,
                series: [{ time: BUCKET, ai_requests: 52, tracked_hits: 12, fetch_count: 40, referral_visits: 3 }]
            })
        );
        answerSetupState(true);
        flushCorrelation();

        const series = instance().chartConfig();
        expect(series.map((definition) => definition.key)).toEqual(['ai_requests', 'referral_visits', 'fetch_count']);
        expect(series[2].dashed).toBe(true);
        expect(series[2].label).toBe('Crawler fetches (log ingest)');
    });

    it('builds the merged breakdowns as one filterable card group set', () => {
        create();
        const report = emptyAIActivityReport({
            top_agents: [aiActivityStat('GPTBot', 10, 40)],
            top_categories: [aiActivityStat('ai_training_crawler', 10, 40)],
            top_paths: [aiActivityStat('/docs', 2, 8)],
            top_sources: [aiActivityStat('chatgpt.com', 4, 0)]
        });
        flushActivity(report);

        const groups = instance().cardGroups();
        expect(groups.map((group) => group.id)).toEqual(['ai-activity', 'by-category', 'fetch-depth']);
        const activity = groups[0].cards;
        expect(activity.map((card) => card.id)).toEqual(['agents', 'categories', 'paths', 'sources']);
        expect(activity[0].data).toBe(report.top_agents);
        expect(activity[2].siteDomain).toBe('example.com');
        expect(activity.every((card) => card.showProvenance === true)).toBe(true);
        expect(fixture.nativeElement.querySelector('app-metric-card-group')).not.toBeNull();
    });

    it('chips agent, category and path filters without a scope suffix and scopes only the referrer chip', () => {
        create();
        flushActivity();

        clickRow('ai_bot', 'GPTBot');
        flushActivity();
        clickRow('ai_bot_category', 'ai_training_crawler', 'categories');
        flushActivity();
        clickRow('path', '/docs', 'paths');
        flushActivity();
        clickRow('ai_source', 'chatgpt.com', 'sources');
        flushActivity();

        expect(
            instance()
                .filterChips()
                .map((chip) => chip.label)
        ).toEqual(['AI agent: GPTBot', 'AI category: Training crawlers', 'Page: /docs', 'AI source: chatgpt.com · tracked visits only']);
    });

    it('re-requests the report exactly once per agent row click', () => {
        create();
        flushActivity(emptyAIActivityReport({ top_agents: [aiActivityStat('GPTBot', 10, 40)] }));

        clickRow('ai_bot', 'GPTBot');

        expect(instance().agentName()).toBe('GPTBot');
        expect(flushActivity()).toEqual([['ai_bot:GPTBot']]);
    });

    it('re-requests the report exactly once per path row click', () => {
        create();
        flushActivity(emptyAIActivityReport({ top_paths: [aiActivityStat('/docs', 2, 8)] }));

        clickRow('path', '/docs', 'paths');

        expect(instance().pathName()).toBe('/docs');
        expect(
            instance()
                .filterChips()
                .map((chip) => chip.key)
        ).toEqual(['path:/docs']);
        expect(flushActivity()).toEqual([['path:/docs']]);
    });

    it('toggles the same row off again and clears every chip at once', () => {
        create();
        flushActivity();

        clickRow('ai_bot', 'GPTBot');
        flushActivity();
        expect(instance().filterChips().length).toBe(1);

        clickRow('ai_bot', 'GPTBot');
        flushActivity();
        expect(instance().filterChips()).toEqual([]);

        clickRow('ai_source', 'chatgpt.com', 'sources');
        flushActivity();
        clickRow('path', '/docs', 'paths');
        flushActivity();
        expect(instance().filterChips().length).toBe(2);

        instance().clearAllFilters();
        fixture.detectChanges();
        flushActivity();
        expect(instance().filterChips()).toEqual([]);
        expect(instance().agentName()).toBeNull();
        expect(instance().pathName()).toBeNull();
    });

    it('pitches log ingest and asks for no ai-fetch data when the site never forwarded logs', () => {
        create();
        flushActivity(emptyAIActivityReport({ ai_requests: 12, fetch_count: 0 }));
        answerSetupState(false);

        expect(instance().showEnrichCallout()).toBe(true);
        const callout = fixture.nativeElement.querySelector('[data-testid="ai-agents-enrich-callout"]');
        expect(callout).not.toBeNull();
        expect(callout.textContent).toContain('See the crawlers your tracker misses');
        expect(callout.querySelector('a').getAttribute('href')).toBe(DOCS_URL);
        expect(instance().showHealthStrip()).toBe(false);
        expect(instance().showFetchTools()).toBe(false);
        httpMock.expectNone(CORRELATION_URL);
    });

    it('keeps the pitch out of the page while the setup lookup is still pending', () => {
        create();
        flushActivity(emptyAIActivityReport({ fetch_count: 0 }));

        expect(instance().showEnrichCallout()).toBe(false);
        expect(fixture.nativeElement.querySelector('[data-testid="ai-agents-enrich-callout"]')).toBeNull();
    });

    it('renders normally without the pitch when fetches exist outside the selected range', () => {
        create();
        flushActivity(emptyAIActivityReport({ ai_requests: 12, fetch_count: 0 }));
        answerSetupState(true);

        expect(instance().showEnrichCallout()).toBe(false);
        expect(fixture.nativeElement.querySelector('[data-testid="ai-agents-enrich-callout"]')).toBeNull();
        expect(fixture.nativeElement.querySelector('app-metric-card-group')).not.toBeNull();
        // Nothing to correlate in this range, so the fetch-only endpoints stay untouched.
        httpMock.expectNone(CORRELATION_URL);
    });

    it('renders one fetch-only strip and no second stat block over it', () => {
        create();
        flushActivity(
            emptyAIActivityReport({
                fetch_count: 12500,
                paths_crawled: 14,
                unique_agents: 3,
                error_rate_4xx: 2.5,
                error_rate_5xx: 0.5,
                median_response_ms: 842,
                total_bytes: 1024
            })
        );
        answerSetupState(true);
        flushCorrelation();

        expect(instance().showHealthStrip()).toBe(true);
        // Every entry is a fetch-only scalar: no merged number (paths_crawled,
        // unique_agents) sneaks in under the log-ingest heading.
        expect(instance().healthStats()).toEqual([
            { label: 'Total fetches', value: '12,500' },
            { label: '4xx rate', value: '2.5%' },
            { label: '5xx rate', value: '0.5%' },
            { label: 'Median response', value: '842 ms' },
            { label: 'Bytes served', value: '1 KB' }
        ]);
        const strips = fixture.nativeElement.querySelectorAll('[data-testid="ai-visibility-health-strip"]');
        expect(strips.length).toBe(1);
        // The duplicated KPI row above the strip is gone for good.
        expect(fixture.nativeElement.querySelector('[data-testid="ai-visibility-headline-kpis"]')).toBeNull();
        expect(strips[0].textContent).not.toContain('Unique paths');
        expect(strips[0].textContent).not.toContain('14');
    });

    it('loads the correlation report once logs contribute and renders the screenshot blocks', () => {
        create();
        flushActivity(emptyAIActivityReport({ fetch_count: 120, unique_agents: 3, error_rate_4xx: 1, error_rate_5xx: 1 }));
        answerSetupState(true);

        const requests = matching(CORRELATION_URL);
        expect(requests.length).toBe(1);
        requests[0].flush(
            emptyCorrelation({
                summary: { total_fetches: 120, fetched_paths: 14, correlated_paths: 5, ai_referred_visits: 9, uncorrelated_fetches: 40 },
                citation_yield: [{ path: '/docs', assistant_name: 'GPTBot', fetch_count: 40, ai_referred_visits: 6, citation_yield_pct: 15 }],
                citation_paths: [{ path: '/docs', fetch_count: 40, ai_referred_visits: 6 }],
                opportunity_pages: [{ path: '/pricing', fetch_count: 30, ai_referred_visits: 0, error_requests: 2, error_rate_pct: 6.6 }],
                failure_hotspots: [{ assistant_name: 'ClaudeBot', path_prefix: '/api', total_requests: 12, error_requests: 4, error_rate_pct: 33.3 }]
            })
        );
        fixture.detectChanges();

        expect(instance().showFetchTools()).toBe(true);
        // The AI-referred visits number lives here, next to what it correlates.
        expect(instance().correlationSummaryStats()).toEqual([
            { label: 'Correlated paths', value: '5' },
            { label: 'Later AI-referred visits', value: '9' },
            { label: 'Uncorrelated fetches', value: '40' }
        ]);
        const groups = instance().correlationTabs();
        // The card group carries its own label so the section heading is not repeated.
        expect(groups.map((group) => group.label)).toEqual(['Correlation breakdowns']);
        expect(groups.flatMap((group) => group.cards).map((card) => card.title)).toEqual(['Citation yield', 'Opportunity pages', 'Failure hotspots']);
        // Share-of-total is meaningless for correlation rows, so the column is off.
        expect(groups.flatMap((group) => group.cards).map((card) => card.showShare)).toEqual([false, false, false]);
        const headings = [...fixture.nativeElement.querySelectorAll('h2, h3')].map((heading: HTMLElement) => heading.textContent?.trim());
        expect(headings.filter((heading) => heading === 'Fetch-to-visit correlation').length).toBe(1);
        expect(headings).toContain('Correlation breakdowns');
        for (const testId of ['ai-visibility-correlation-shot', 'ai-visibility-correlation-kpis', 'ai-visibility-correlation-tables']) {
            expect(fixture.nativeElement.querySelector(`[data-testid="${testId}"]`)).not.toBeNull();
        }
    });

    it('takes the per-path citation list from the server and names hotspots by agent and prefix', () => {
        create();
        flushActivity(emptyAIActivityReport({ fetch_count: 120 }));
        answerSetupState(true);
        flushCorrelation(
            emptyCorrelation({
                // Per-pair rows are session-overlapping and capped by yield before
                // aggregation: summing them here would report 5 and 5 visits.
                citation_yield: [
                    { path: '/docs/configuration', assistant_name: 'GPTBot', fetch_count: 40, ai_referred_visits: 3, citation_yield_pct: 7.5 },
                    { path: '/docs/configuration', assistant_name: 'ClaudeBot', fetch_count: 10, ai_referred_visits: 2, citation_yield_pct: 20 },
                    { path: '/', assistant_name: 'GPTBot', fetch_count: 20, ai_referred_visits: 4, citation_yield_pct: 20 },
                    { path: '/', assistant_name: 'PerplexityBot', fetch_count: 5, ai_referred_visits: 1, citation_yield_pct: 20 }
                ],
                citation_paths: [
                    { path: '/', fetch_count: 25, ai_referred_visits: 4 },
                    { path: '/docs/configuration', fetch_count: 50, ai_referred_visits: 3 }
                ],
                opportunity_pages: [{ path: '/pricing', fetch_count: 30, ai_referred_visits: 0, error_requests: 2, error_rate_pct: 6.6 }],
                failure_hotspots: [
                    { assistant_name: 'ClaudeBot', path_prefix: '/api', total_requests: 12, error_requests: 4, error_rate_pct: 33.3 },
                    { assistant_name: 'ClaudeBot', path_prefix: '/docs', total_requests: 8, error_requests: 2, error_rate_pct: 25 }
                ]
            })
        );

        const cards = instance()
            .correlationTabs()
            .flatMap((group) => group.cards);
        // The distinct per-path counts and their server ranking pass through untouched.
        expect(cards[0].data).toEqual([
            { name: '/', value: 4 },
            { name: '/docs/configuration', value: 3 }
        ]);
        expect(cards[1].data).toEqual([{ name: '/pricing', value: 30 }]);
        // Hotspot rows are per agent and prefix, so the label says both.
        expect(cards[2].data).toEqual([
            { name: 'ClaudeBot · /api', value: 4 },
            { name: 'ClaudeBot · /docs', value: 2 }
        ]);
    });

    it('forwards the agent and path chips to the correlation request and the export url', () => {
        create();
        flushActivity(emptyAIActivityReport({ fetch_count: 120 }));
        answerSetupState(true);
        flushCorrelation();

        clickRow('ai_bot', 'GPTBot');
        flushActivity(emptyAIActivityReport({ fetch_count: 120 }));
        clickRow('path', '/docs', 'paths');
        flushActivity(emptyAIActivityReport({ fetch_count: 120 }));

        const requests = matching(CORRELATION_URL);
        expect(requests.length).toBeGreaterThan(0);
        const last = requests[requests.length - 1].request;
        expect(last.params.get('assistant_name')).toBe('GPTBot');
        expect(last.params.get('path')).toBe('/docs');
        // Superseded correlation requests are cancelled, so only the live one answers.
        expect(requests.slice(0, -1).every((request) => request.cancelled)).toBe(true);
        requests[requests.length - 1].flush(emptyCorrelation());
        fixture.detectChanges();

        const exportUrl = instance().exportUrl();
        expect(exportUrl).toContain('/api/sites/site-1/ai-fetch/export?');
        expect(exportUrl).toContain('assistant_name=GPTBot');
        expect(exportUrl).toContain('path=%2Fdocs');
        expect(fixture.nativeElement.querySelector('app-export-split-button')).not.toBeNull();
    });

    it('serves merged fetch counts in share mode without correlation or export', () => {
        TestBed.inject(ShareService).setToken('share-token');

        create();
        flushActivity(
            emptyAIActivityReport({
                ai_requests: 52,
                fetch_count: 40,
                paths_crawled: 14,
                total_bytes: 1024,
                top_agents: [aiActivityStat('GPTBot', 12, 40)],
                series: [{ time: BUCKET, ai_requests: 52, tracked_hits: 12, fetch_count: 40, referral_visits: 3 }]
            })
        );

        expect(instance().kpiCards()[0].value).toBe(52);
        expect(
            instance()
                .chartConfig()
                .map((series) => series.key)
        ).toEqual(['ai_requests', 'referral_visits', 'fetch_count']);
        expect(instance().showHealthStrip()).toBe(true);
        expect(instance().showFetchTools()).toBe(false);
        expect(instance().showEnrichCallout()).toBe(false);
        expect(fixture.nativeElement.querySelector('app-export-split-button')).toBeNull();
        httpMock.expectNone(SETUP_STATE_URL);
        httpMock.expectNone(CORRELATION_URL);
    });

    it('reloads the report in the background from one realtime registration', async () => {
        vi.useFakeTimers();
        try {
            create();
            flushActivity();
            answerSetupState(false);

            realtimeEvents.next({
                type: 'analytics.changed',
                site_id: 'site-1',
                kinds: ['hits'],
                changed_at: BUCKET,
                bucket_start: BUCKET,
                counts: { hits: 4 }
            });
            await vi.advanceTimersByTimeAsync(800);
            expect(matching(ACTIVITY_URL).length).toBe(1);

            realtimeEvents.next({
                type: 'analytics.changed',
                site_id: 'site-1',
                kinds: ['ai_fetch'],
                changed_at: BUCKET,
                bucket_start: BUCKET,
                counts: { ai_fetch: 2 }
            });
            await vi.advanceTimersByTimeAsync(800);
            expect(matching(ACTIVITY_URL).length).toBe(1);
        } finally {
            vi.useRealTimers();
        }
    });

    it('drops an in-flight correlation request when the fetch-only zone disappears', async () => {
        vi.useFakeTimers();
        try {
            create();
            flushActivity(emptyAIActivityReport({ fetch_count: 120 }));
            answerSetupState(true);
            flushCorrelation(emptyCorrelation({ summary: { total_fetches: 120, fetched_paths: 4, correlated_paths: 2, ai_referred_visits: 3, uncorrelated_fetches: 1 } }));
            expect(instance().correlation()).not.toBeNull();

            // A realtime refresh reissues both requests; the correlation one is left open.
            realtimeEvents.next({ type: 'analytics.changed', site_id: 'site-1', kinds: ['ai_fetch'], changed_at: BUCKET, bucket_start: BUCKET, counts: { ai_fetch: 2 } });
            await vi.advanceTimersByTimeAsync(800);
            const pending = matching(CORRELATION_URL);
            expect(pending.length).toBe(1);

            // The refreshed range has no fetches left, so the fetch-only zone goes
            // away while that correlation request is still in flight.
            flushActivity(emptyAIActivityReport({ fetch_count: 0 }));

            expect(instance().showFetchTools()).toBe(false);
            expect(pending[0].cancelled).toBe(true);
            expect(instance().correlation()).toBeNull();
        } finally {
            vi.useRealTimers();
        }
    });

    it('applies only the last of two rapid filter clicks', () => {
        create();
        flushActivity();

        clickRow('ai_bot', 'GPTBot');
        const stale = matching(ACTIVITY_URL);
        expect(stale.length).toBe(1);

        clickRow('ai_bot', 'ClaudeBot');
        expect(stale[0].cancelled).toBe(true);

        const fresh = matching(ACTIVITY_URL);
        expect(fresh.length).toBe(1);
        expect(fresh[0].request.params.getAll('filter')).toEqual(['ai_bot:ClaudeBot']);
        fresh[0].flush(emptyAIActivityReport({ ai_requests: 222 }));
        fixture.detectChanges();

        expect(instance().kpiCards()[0].value).toBe(222);
        expect(instance().agentName()).toBe('ClaudeBot');
    });

    it('shows the no-site placeholder and issues no requests at all', () => {
        selectSite(null);

        create();

        expect(fixture.nativeElement.querySelector('app-no-site-selected')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('app-metric-card-group')).toBeNull();
        expect(httpMock.match(() => true).length).toBe(0);
    });
});
