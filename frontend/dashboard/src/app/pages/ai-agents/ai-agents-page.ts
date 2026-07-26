import { ChangeDetectionStrategy, Component, computed, DestroyRef, effect, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { ButtonModule } from '@openng/optimus-ui/button';
import { CardModule } from '@openng/optimus-ui/card';
import { finalize, Subscription } from 'rxjs';

import { ExportSplitButton, ExportStatusBanner } from '@components/export-split-button/export-split-button';
import { FilterChipItem, FilterChipRow } from '@components/filter-chip-row/filter-chip-row';
import { NoSiteSelected } from '@components/no-site-selected/no-site-selected';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { ReportRangeToolbar } from '@components/report-range-toolbar/report-range-toolbar';
import { SetupCallout } from '@components/setup-callout/setup-callout';
import { StatGroup, StatGroups } from '@components/stat-groups/stat-groups';
import { calcDelta, safeRate } from '@core/analytics/delta-utils';
import { buildTakeoutExportFilename, DEFAULT_HITS_EXPORT_FORMAT, TakeoutExportFormat, withTakeoutExportFormat } from '@core/export/export-formats';
import { injectActiveLang } from '@core/i18n/active-lang';
import { localeForLanguage } from '@core/i18n/duration-format';
import { AnalyticsService } from '@core/services/analytics.service';
import { AIActivityFilterType, buildAIActivityCardGroups, type AICorrelationRows } from '@features/analytics/ai-activity-cards';
import { aiFilterChipLabel } from '@features/analytics/ai-category-labels';
import { KPI_PERCENT_FORMAT, KpiCard, KpiCardModel } from '@features/analytics/components/kpi-card';
import { MetricCardGroup, MetricCardGroupRowClick, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import { SeriesChart, SeriesChartPoint, SeriesDefinition } from '@features/analytics/components/series-chart';
import { injectAIActivityQuery, type AIActivityQueryMode } from '@features/analytics/services/ai-activity-query';
import { StatsService } from '@features/analytics/services/stats.service';
import { SiteService } from '@features/sites/services/site.service';
import type { AIFetchCorrelationReport } from '@models/analytics.types';
import { formatBytes, formatResponseMs } from '@pages/ai-agents/ai-agents-page.utils';
import { RealtimeRefreshCoordinator } from '@services/realtime-refresh-coordinator.service';
import { REALTIME_KINDS } from '@services/realtime.service';
import { injectReportRange } from '@services/report-range-preferences.service';
import { injectSkeletonGate } from '@services/report-subject.service';
import { SetupStateService } from '@services/setup-state.service';
import { ShareService } from '@services/share.service';
import { TakeoutDownloadService } from '@services/takeout-download.service';

type AIAgentKpiID = 'ai_requests' | 'referral_visits' | 'paths_crawled' | 'share_of_pageviews';

interface AIAgentKpiCard extends KpiCardModel {
    id: AIAgentKpiID;
}

interface AIPageFilter {
    type: AIActivityFilterType;
    value: string;
}

/**
 * The one AI analytics surface. A single `ai-activity` request per site, range
 * and filter set carries the merged picture — tracked hits plus whatever
 * forwarded crawler logs add — so the KPI band, the hero chart and every
 * breakdown read from the same report and stay consistent behind a share token.
 *
 * Only the two fetch-only extras still talk to the `ai-fetch/*` endpoints: the
 * correlation report and the raw export. Both are hidden in share mode and stay
 * silent until the site actually forwards logs.
 */
@Component({
    selector: 'app-ai-agents-page',
    imports: [
        TranslocoPipe,
        ButtonModule,
        CardModule,
        StatGroups,
        PageHeader,
        PageHeaderLeft,
        PageBreadcrumb,
        ReportRangeToolbar,
        NoSiteSelected,
        SetupCallout,
        FilterChipRow,
        KpiCard,
        SeriesChart,
        MetricCardGroup,
        ExportSplitButton,
        ExportStatusBanner
    ],
    templateUrl: './ai-agents-page.html',
    styleUrl: './ai-agents-page.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AIAgentsPage {
    protected readonly docsUrl = 'https://hitkeep.com/guides/tracking/ai-fetch-ingest/';

    private readonly siteService = inject(SiteService);
    private readonly analyticsService = inject(AnalyticsService);
    private readonly statsService = inject(StatsService);
    private readonly transloco = inject(TranslocoService);
    private readonly localeService = inject(TranslocoLocaleService);
    private readonly takeoutDownloadService = inject(TakeoutDownloadService);
    private readonly share = inject(ShareService);
    private readonly setupState = inject(SetupStateService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly realtimeRefresh = inject(RealtimeRefreshCoordinator);
    private readonly reportRange = injectReportRange();
    private readonly activeLanguage = injectActiveLang();

    private readonly activityQuery = injectAIActivityQuery();

    protected readonly site = this.siteService.activeSite;
    protected readonly noSite = computed(() => !this.site());
    protected readonly shareMode = this.share.isShareMode;
    protected readonly report = this.activityQuery.report;
    protected readonly isLoading = this.activityQuery.isLoading;
    protected readonly isShortRange = this.reportRange.isShortRange;
    protected readonly kpiUpdateKey = signal(0);

    /** Every filter narrows both sides of the report; the server decides what a dimension can scope. */
    protected readonly pageFilters = signal<AIPageFilter[]>([]);
    /** The agent chip doubles as `assistant_name` for the fetch-only correlation and export. */
    protected readonly agentName = computed(() => this.activeFilterValue('ai_bot'));
    /** Same for the page chip, which the fetch-only endpoints know as `path`. */
    protected readonly pathName = computed(() => this.activeFilterValue('path'));
    private readonly requestFilters = computed<{ type: string; value: string }[]>(() => this.pageFilters().map((filter) => ({ type: filter.type, value: filter.value })));

    protected readonly correlation = signal<AIFetchCorrelationReport | null>(null);
    protected readonly isLoadingCorrelation = signal(false);
    private correlationRequest: Subscription | null = null;

    // The fetch-depth strips are plain text, so they cannot tween like the KPI
    // cards. Gating them on the same rule at least keeps them from pulsing next
    // to numbers that stayed on screen.
    private readonly showReportSkeleton = injectSkeletonGate(this.isLoading, () => this.report() !== null);
    private readonly showCorrelationSkeleton = injectSkeletonGate(this.isLoadingCorrelation, () => this.correlation() !== null);

    protected readonly isExporting = signal(false);
    protected readonly exportState = signal<'idle' | 'success' | 'error'>('idle');

    /** Merged fetch volume in the selected range; zero means nothing log-derived to show. */
    private readonly fetchCount = computed(() => this.report()?.fetch_count ?? 0);
    /**
     * Tri-state: `null` while the shared setup lookup is pending, so neither the
     * pitch nor the fetch-only blocks commit to a decision on a guess. Share
     * links never ask — the lookup is not share-exposed.
     */
    private readonly hasAIFetches = computed(() => (this.shareMode() ? null : (this.setupState.state(this.site()?.id)?.has_ai_fetches ?? null)));

    /** Pitch log ingest only once we know this site has never forwarded a single fetch. */
    protected readonly showEnrichCallout = computed(() => !this.shareMode() && this.hasAIFetches() === false && this.fetchCount() === 0);
    /** Fetch scalars come from the share-exposed report, so this survives a share token. */
    protected readonly showHealthStrip = computed(() => this.fetchCount() > 0);
    /** Correlation and export read the fetch-only endpoints, which share links cannot. */
    protected readonly showFetchTools = computed(() => !this.shareMode() && this.hasAIFetches() === true && this.fetchCount() > 0);

    protected readonly heroChartData = computed<SeriesChartPoint[]>(() =>
        (this.report()?.series ?? []).map((point) => ({
            time: point.time,
            ai_requests: point.ai_requests,
            tracked_hits: point.tracked_hits,
            fetch_count: point.fetch_count,
            referral_visits: point.referral_visits
        }))
    );

    protected readonly chartConfig = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        const series: SeriesDefinition[] = [
            {
                key: 'ai_requests',
                label: this.transloco.translate('aiAgents.hero.series.aiRequests'),
                color: '#6366f1',
                gradientFrom: 'rgba(99, 102, 241, 0.5)',
                gradientTo: 'rgba(99, 102, 241, 0.0)'
            },
            {
                key: 'referral_visits',
                label: this.transloco.translate('aiAgents.hero.series.referralVisits'),
                color: '#14b8a6',
                gradientFrom: 'rgba(20, 184, 166, 0.5)',
                gradientTo: 'rgba(20, 184, 166, 0.0)'
            }
        ];
        // The log-derived share of the total only earns a line once it exists.
        if (this.fetchCount() > 0) {
            series.push({
                key: 'fetch_count',
                label: this.transloco.translate('aiAgents.hero.series.crawlerFetches'),
                color: '#0f766e',
                gradientFrom: 'rgba(15, 118, 110, 0.4)',
                gradientTo: 'rgba(15, 118, 110, 0.0)',
                dashed: true
            });
        }
        return series;
    });

    protected readonly kpiCards = computed<AIAgentKpiCard[]>(() => {
        this.activeLanguage();
        const report = this.report();
        const previous = report?.comparison ?? null;
        const loading = this.isLoading();
        const updateKey = this.kpiUpdateKey();
        const aiRequests = report?.ai_requests ?? 0;
        const shareOfPageviews = safeRate(aiRequests, report?.pageviews ?? 0);
        return [
            {
                id: 'ai_requests',
                label: this.transloco.translate('aiAgents.kpis.aiRequests'),
                value: aiRequests,
                loading,
                updateKey,
                delta: previous ? calcDelta(aiRequests, previous.ai_requests) : null
            },
            {
                id: 'referral_visits',
                label: this.transloco.translate('aiAgents.kpis.referralVisits'),
                value: report?.referral_visits ?? 0,
                loading,
                updateKey,
                delta: previous ? calcDelta(report?.referral_visits ?? 0, previous.referral_visits) : null
            },
            {
                id: 'paths_crawled',
                label: this.transloco.translate('aiAgents.kpis.pathsCrawled'),
                value: report?.paths_crawled ?? 0,
                loading,
                updateKey,
                delta: previous ? calcDelta(report?.paths_crawled ?? 0, previous.paths_crawled) : null
            },
            {
                id: 'share_of_pageviews',
                label: this.transloco.translate('aiAgents.kpis.shareOfPageviews'),
                value: shareOfPageviews,
                loading,
                updateKey,
                format: KPI_PERCENT_FORMAT,
                suffix: '%',
                delta: previous ? calcDelta(shareOfPageviews, safeRate(previous.ai_requests, previous.pageviews)) : null
            }
        ];
    });

    /**
     * Mapped once per correlation report, not once per card rebuild: the metric
     * list re-observes its scroll frame whenever `data` changes identity, and the
     * card models are rebuilt on every report, filter and language change.
     */
    private readonly correlationRows = computed<AICorrelationRows | null>(() => {
        const correlation = this.correlation();
        if (!correlation) return null;
        return {
            citationPaths: correlation.citation_paths.map((row) => ({ name: row.path, value: row.ai_referred_visits })),
            opportunityPages: correlation.opportunity_pages.map((row) => ({ name: row.path, value: row.fetch_count })),
            failureHotspots: correlation.failure_hotspots.map((row) => ({ name: `${row.assistant_name} · ${row.path_prefix}`, value: row.error_requests }))
        };
    });

    protected readonly cardGroups = computed<MetricCardGroupTab<AIActivityFilterType>[]>(() => {
        this.activeLanguage();
        // The correlation breakdowns are one group of this same grid, so the page
        // renders a single card surface. Passing `null` is what withholds the group
        // wherever its endpoint is out of reach.
        return buildAIActivityCardGroups({
            transloco: this.transloco,
            report: this.report(),
            isLoading: this.isLoading(),
            activeValueFor: (type) => this.activeFilterValue(type),
            siteDomain: this.site()?.domain ?? null,
            correlation: this.showFetchTools() ? { rows: this.correlationRows(), isLoading: this.isLoadingCorrelation() } : null
        });
    });

    /**
     * The fetch-depth card's scalars, in themed strips a reader can compare
     * within: how much was fetched, what those fetches were worth, and how they
     * were served. The merged counters (paths_crawled, unique_agents) stay
     * headline KPIs of the whole page and would claim to be log-derived here.
     *
     * The middle strip crosses over to the tracked side, so it only exists where
     * the correlation endpoint is reachable and carries that request's loading
     * flag: a share token reads two strips, everyone else three.
     */
    protected readonly fetchDepthGroups = computed<StatGroup[]>(() => {
        this.activeLanguage();
        const report = this.report();
        const locale = this.localeTag();
        const isLoading = this.showReportSkeleton();
        const summary = this.correlation()?.summary;
        const kpi = (key: string) => this.transloco.translate(`aiAgents.fetchDepth.kpis.${key}`);
        const groupLabel = (key: string) => this.transloco.translate(`aiAgents.fetchDepth.groups.${key}`);

        return [
            {
                id: 'volume',
                label: groupLabel('volume'),
                isLoading,
                stats: [
                    { label: kpi('totalFetches'), value: this.localizeCount(report?.fetch_count ?? 0) },
                    { label: kpi('bytesServed'), value: formatBytes(report?.total_bytes ?? 0, locale) }
                ]
            },
            ...(this.showFetchTools()
                ? [
                      {
                          id: 'correlation',
                          label: groupLabel('correlation'),
                          isLoading: this.showCorrelationSkeleton(),
                          stats: [
                              { label: kpi('correlatedPaths'), value: this.localizeCount(summary?.correlated_paths ?? 0) },
                              { label: kpi('aiReferredVisits'), value: this.localizeCount(summary?.ai_referred_visits ?? 0) },
                              { label: kpi('uncorrelatedFetches'), value: this.localizeCount(summary?.uncorrelated_fetches ?? 0) }
                          ]
                      }
                  ]
                : []),
            {
                id: 'health',
                label: groupLabel('health'),
                isLoading,
                stats: [
                    { label: kpi('errorRate4xx'), value: this.localizeRate(report?.error_rate_4xx ?? 0) },
                    { label: kpi('errorRate5xx'), value: this.localizeRate(report?.error_rate_5xx ?? 0) },
                    { label: kpi('medianResponse'), value: formatResponseMs(report?.median_response_ms ?? 0, locale) }
                ]
            }
        ];
    });

    protected readonly filterChips = computed<FilterChipItem[]>(() => {
        this.activeLanguage();
        return this.pageFilters().map((filter) => ({
            key: `${filter.type}:${filter.value}`,
            label: this.filterLabel(filter),
            remove: () => this.removeFilter(filter.type, filter.value)
        }));
    });

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        const site = this.site();
        const current: PageBreadcrumbItem = { label: this.transloco.translate('aiAgents.title'), isCurrent: true };
        if (!site) return [current];
        return [{ label: site.domain, favicon: site, routerLink: '/dashboard' }, current];
    });

    protected readonly exportUrl = computed(() => {
        const site = this.site();
        const dates = this.reportRange.currentDateRange();
        if (!site || !dates || this.shareMode()) return '';

        const params = new URLSearchParams({ from: dates.from, to: dates.to });
        const agentName = this.agentName();
        const pathName = this.pathName();
        if (agentName) params.set('assistant_name', agentName);
        if (pathName) params.set('path', pathName);
        return `/api/sites/${site.id}/ai-fetch/export?${params.toString()}`;
    });

    constructor() {
        effect(() => {
            const site = this.site();
            const dates = this.reportRange.currentDateRange();
            if (!site || !dates) return;
            this.loadReport(site.id, dates.from, dates.to, this.requestFilters());
        });

        effect(() => {
            const site = this.site();
            const dates = this.reportRange.currentDateRange();
            const agentName = this.agentName();
            const pathName = this.pathName();
            if (!site || !dates || !this.showFetchTools()) {
                // A request still in flight would otherwise land after the zone is
                // gone and re-populate what this branch just cleared.
                this.correlationRequest?.unsubscribe();
                this.correlation.set(null);
                return;
            }
            this.loadCorrelation(site.id, dates.from, dates.to, agentName, pathName);
        });

        // One registration for both kinds: every event reloads the same single
        // report, so splitting them would only refetch what the other half owns.
        this.realtimeRefresh.registerUntilDestroyed(this.destroyRef, {
            siteId: () => this.site()?.id ?? null,
            kinds: [REALTIME_KINDS.hits, REALTIME_KINDS.aiFetch],
            enabled: () => !!this.site() && !!this.reportRange.currentDateRange(),
            refresh: () => this.refresh('background'),
            debounceMs: 700
        });
    }

    protected refresh(mode: AIActivityQueryMode = 'blocking'): void {
        const site = this.site();
        const dates = this.reportRange.currentDateRange();
        if (!site || !dates) return;
        this.loadReport(site.id, dates.from, dates.to, this.requestFilters(), mode);
        if (this.showFetchTools()) {
            this.loadCorrelation(site.id, dates.from, dates.to, this.agentName(), this.pathName());
        }
    }

    protected onCardRowClick(event: MetricCardGroupRowClick): void {
        this.applyFilter(event.filterType as AIActivityFilterType, event.metric.name);
    }

    protected clearAllFilters(): void {
        this.pageFilters.set([]);
    }

    protected activeFilterValue(type: AIActivityFilterType): string | null {
        return this.pageFilters().find((filter) => filter.type === type)?.value ?? null;
    }

    protected exportFiltered(format: TakeoutExportFormat = DEFAULT_HITS_EXPORT_FORMAT): void {
        const url = withTakeoutExportFormat(this.exportUrl(), format);
        if (!url || this.isExporting()) return;

        this.isExporting.set(true);
        this.exportState.set('idle');

        this.takeoutDownloadService
            .downloadFromUrl(url, buildTakeoutExportFilename(this.site()?.domain, 'ai-fetches', format))
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isExporting.set(false))
            )
            .subscribe({
                next: () => this.exportState.set('success'),
                error: () => this.exportState.set('error')
            });
    }

    /** Replaces an existing filter of the same dimension; the same value toggles off. */
    private applyFilter(type: AIActivityFilterType, value: string): void {
        if (!value) return;
        this.pageFilters.update((filters) => {
            const existingIndex = filters.findIndex((filter) => filter.type === type);
            if (existingIndex < 0) return [...filters, { type, value }];
            if (filters[existingIndex].value === value) return filters.filter((_, index) => index !== existingIndex);
            const next = [...filters];
            next[existingIndex] = { type, value };
            return next;
        });
    }

    private removeFilter(type: AIActivityFilterType, value: string): void {
        this.pageFilters.update((filters) => filters.filter((filter) => !(filter.type === type && filter.value === value)));
    }

    /**
     * Agent, category and page chips narrow the whole report, so they read as
     * plain filters. Only the referrer dimension exists on the tracked side
     * alone, which the chip says out loud.
     */
    private filterLabel(filter: AIPageFilter): string {
        if (filter.type === 'path') {
            return this.transloco.translate('common.filters.page', { value: filter.value });
        }
        const label = aiFilterChipLabel(this.transloco, filter.type, filter.value);
        if (filter.type !== 'ai_source') return label;
        return `${label} · ${this.transloco.translate('aiAgents.filters.scopeHits')}`;
    }

    private loadReport(siteId: string, from: string, to: string, filters: { type: string; value: string }[], mode: AIActivityQueryMode = 'blocking'): void {
        // Reading `report()` behind the `background` guard keeps the loading
        // effect from re-triggering itself on every committed result.
        const effectiveMode: AIActivityQueryMode = mode === 'background' && this.report() && !this.isLoading() ? 'background' : 'blocking';
        this.activityQuery.load(
            {
                siteId,
                from,
                to,
                filters,
                comparison: this.statsService.comparisonRange(from, to),
                onSuccess: effectiveMode === 'background' ? () => this.kpiUpdateKey.update((key) => key + 1) : undefined
            },
            effectiveMode
        );
    }

    private loadCorrelation(siteId: string, from: string, to: string, agentName: string | null, pathName: string | null): void {
        this.correlationRequest?.unsubscribe();
        this.isLoadingCorrelation.set(true);
        this.correlationRequest = this.analyticsService
            .getAIFetchCorrelation(siteId, from, to, { assistantName: agentName, path: pathName })
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isLoadingCorrelation.set(false))
            )
            .subscribe({
                next: (report) => this.correlation.set(report),
                error: () => this.correlation.set(null)
            });
    }

    private localizeCount(value: number): string {
        return this.localeService.localizeNumber(value, 'decimal');
    }

    /** Error rates arrive pre-rounded to two decimals; the strip shows one. */
    private localizeRate(value: number): string {
        return `${this.localeService.localizeNumber(value, 'decimal', undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 })}%`;
    }

    /** Locale tag for the two `Intl`-based byte/duration formatters. */
    private localeTag(): string {
        const locale = this.localeService.getLocale();
        return typeof locale === 'string' && locale.trim() ? locale : localeForLanguage(this.activeLanguage());
    }
}
