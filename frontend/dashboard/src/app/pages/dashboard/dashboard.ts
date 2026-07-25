import { Component, effect, inject, signal, computed, ChangeDetectionStrategy, DestroyRef } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router } from '@angular/router';
import { injectActiveLang } from '@core/i18n/active-lang';
import { DOCUMENT, NgOptimizedImage } from '@angular/common';
import { ReactiveFormsModule } from '@angular/forms';
import { debounceTime, distinctUntilChanged, finalize, Subject } from 'rxjs';
import { TranslocoService } from '@jsverse/transloco';
import { TranslocoPipe } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
// OptimusUI
import { CardModule } from '@openng/optimus-ui/card';
import { TableModule, TableLazyLoadEvent } from '@openng/optimus-ui/table';
import { SelectModule } from '@openng/optimus-ui/select';
import { ButtonModule } from '@openng/optimus-ui/button';
import { IconFieldModule } from '@openng/optimus-ui/iconfield';
import { InputIconModule } from '@openng/optimus-ui/inputicon';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { TooltipModule } from '@openng/optimus-ui/tooltip';
// Features
import { SiteService } from '@features/sites/services/site.service';
import { injectStatsQuery, type StatsQueryMode } from '@features/analytics/services/stats-query';
import { HitService } from '@features/hits/services/hit.service';
import { RealtimeRefreshCoordinator } from '@services/realtime-refresh-coordinator.service';
import { REALTIME_ALL_ANALYTICS_KINDS } from '@services/realtime.service';
import { TrafficChart } from '@features/analytics/components/traffic-chart';
import { MetricCardGroup, MetricCardGroupRowClick, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import { GoalList } from '@features/analytics/components/goal-list';
import { FunnelList } from '@features/analytics/components/funnel-list';
import { SearchConsoleDrilldown } from '@features/analytics/components/search-console-drilldown';
import { FunnelManager } from '@features/funnels/components/funnel-manager';
import { FunnelViewer } from '@features/funnels/components/funnel-viewer';
import type { Funnel, MetricStat } from '@models/analytics.types';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { WorkflowProgress, type WorkflowProgressStep } from '@components/workflow-progress/workflow-progress';
import { ExportSplitButton, ExportStatusBanner } from '@components/export-split-button/export-split-button';
import { FilterChipItem, FilterChipRow } from '@components/filter-chip-row/filter-chip-row';
import { KPI_PERCENT_FORMAT, KPI_SHORT_DECIMAL_FORMAT, KpiCard } from '@features/analytics/components/kpi-card';
import { ShareService } from '@services/share.service';
import { translateRangeLabel } from '@components/range-toolbar/range-toolbar';
import { ReportRangeToolbar } from '@components/report-range-toolbar/report-range-toolbar';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { buildTakeoutExportFilename, DEFAULT_HITS_EXPORT_FORMAT, TakeoutExportFormat, withTakeoutExportFormat } from '@core/export/export-formats';
import { calcDelta } from '@core/analytics/delta-utils';
import { aiFilterChipLabel } from '@features/analytics/ai-category-labels';
import { buildAIMetricCardTabs } from '@features/analytics/ai-metric-cards';
import { TakeoutDownloadService } from '@services/takeout-download.service';
import { AddSiteDialog } from '@features/sites/components/add-site-dialog';
import { TeamService } from '@services/team.service';
import { OnboardingService, OnboardingStep } from '@services/onboarding.service';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';
import { injectReportRange } from '@services/report-range-preferences.service';

type MetricFilterType = 'path' | 'referrer' | 'device' | 'country' | 'city' | 'provider' | 'asn' | 'browser' | 'language' | 'ai_bot' | 'ai_bot_category' | 'ai_source';
interface MetricFilter {
    type: MetricFilterType;
    value: string;
}
type KpiMetricID = 'live_visitors' | 'total_pageviews' | 'unique_sessions' | 'bounce_rate' | 'avg_session_duration' | 'pages_per_session';
interface KpiCardData {
    id: KpiMetricID;
    label: string;
    value: number | string;
    loading: boolean;
    valueClass: string;
    format?: Intl.NumberFormatOptions;
    suffix?: string;
    duration?: boolean;
    updateKey?: number;
    delta?: number | null;
    invertDelta?: boolean;
}
@Component({
    selector: 'app-dashboard',
    standalone: true,
    imports: [
        ReactiveFormsModule,
        TranslocoPipe,
        CardModule,
        TableModule,
        SelectModule,
        ButtonModule,
        IconFieldModule,
        InputIconModule,
        InputTextModule,
        SkeletonModule,
        TooltipModule,
        PageHeader,
        PageHeaderLeft,
        PageBreadcrumb,
        WorkflowProgress,
        ReportRangeToolbar,
        ExportSplitButton,
        ExportStatusBanner,
        FilterChipRow,
        RelativeDateTime,
        KpiCard,
        TrafficChart,
        MetricCardGroup,
        GoalList,
        FunnelList,
        SearchConsoleDrilldown,
        FunnelManager,
        FunnelViewer,
        NgOptimizedImage,
        AddSiteDialog
    ],
    templateUrl: './dashboard.html',
    styleUrl: './dashboard.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class Dashboard {
    protected siteService = inject(SiteService);
    protected hitService = inject(HitService);
    private shareService = inject(ShareService);
    private teamService = inject(TeamService);
    private takeoutDownloadService = inject(TakeoutDownloadService);
    private localeService = inject(TranslocoLocaleService);
    private transloco = inject(TranslocoService);
    private destroyRef = inject(DestroyRef);
    private router = inject(Router);
    private route = inject(ActivatedRoute);
    private onboarding = inject(OnboardingService);
    private document = inject(DOCUMENT);
    private realtimeRefresh = inject(RealtimeRefreshCoordinator);
    protected readonly reportRange = injectReportRange();
    private reportLinkApplied = false;
    private readonly activeLanguage = injectActiveLang();
    private statsQuery = injectStatsQuery();
    protected isShareMode = computed(() => this.shareService.isShareMode());
    protected stats = this.statsQuery.stats;
    protected isStatsLoading = this.statsQuery.isLoading;
    protected currentComparisonRange = this.statsQuery.comparisonRange;
    protected readonly kpiUpdateKey = signal(0);
    protected showFunnelManager = signal(false);
    protected showFunnelViewer = signal(false);
    protected selectedFunnelId = signal<string | null>(null);
    protected funnelEditRequestId = signal<string | null>(null);
    protected searchConsoleRefreshKey = signal(0);
    protected isAddSiteVisible = signal(false);
    protected funnelDateRange = computed(() => this.getCurrentDateRange());
    protected searchConsoleDateRange = computed(() => this.getCurrentDateRange());
    protected searchConsoleFilters = computed(() => ({
        path: this.activeFilterValue('path'),
        country: this.activeFilterValue('country'),
        device: this.activeFilterValue('device')
    }));
    protected siteDomain = computed(() => this.siteService.activeSite()?.domain ?? null);
    protected emptyTeamName = computed(() => this.teamService.activeTeam()?.name ?? null);
    protected siteFaviconUrl = computed(() => {
        const domain = this.siteDomain();
        return domain ? browserAppUrl(this.document, `/api/favicon/${encodeURIComponent(domain)}`) : '';
    });
    protected activeFilters = signal<MetricFilter[]>([]);
    protected hasFilters = computed(() => this.activeFilters().length > 0);
    protected isExportingFiltered = signal(false);
    protected filteredExportState = signal<'idle' | 'success' | 'error'>('idle');
    protected filterChips = computed<FilterChipItem[]>(() => {
        this.activeLanguage();
        return this.activeFilters().map((filter) => ({
            key: `${filter.type}:${filter.value}`,
            label: this.filterLabel(filter),
            remove: () => this.removeFilter(filter.type, filter.value)
        }));
    });
    protected readonly metricCardTabs = computed<MetricCardGroupTab<MetricFilterType>[]>(() => {
        this.activeLanguage();
        const stats = this.stats();
        const loading = this.isStatsLoading();
        const siteDomain = this.siteDomain();
        return [
            {
                id: 'content',
                label: this.transloco.translate('common.metricGroups.content'),
                icon: 'pi-file',
                cards: [
                    {
                        id: 'top-pages',
                        title: this.transloco.translate('common.metrics.topPages'),
                        icon: 'pi-file',
                        data: stats?.top_pages ?? [],
                        linkMode: 'path',
                        siteDomain,
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('path'),
                        filterType: 'path'
                    },
                    {
                        id: 'landing-pages',
                        title: this.transloco.translate('common.metrics.landingPages'),
                        icon: 'pi-sign-in',
                        data: stats?.top_landing_pages ?? [],
                        linkMode: 'path',
                        siteDomain,
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('path'),
                        filterType: 'path'
                    },
                    {
                        id: 'exit-pages',
                        title: this.transloco.translate('common.metrics.exitPages'),
                        icon: 'pi-sign-out',
                        data: stats?.top_exit_pages ?? [],
                        linkMode: 'path',
                        siteDomain,
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('path'),
                        filterType: 'path'
                    }
                ]
            },
            {
                id: 'acquisition',
                label: this.transloco.translate('common.metricGroups.acquisition'),
                icon: 'pi-link',
                cards: [
                    {
                        id: 'sources',
                        title: this.transloco.translate('common.metrics.topSources'),
                        icon: 'pi-link',
                        data: stats?.top_referrers ?? [],
                        linkMode: 'url',
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('referrer'),
                        filterType: 'referrer'
                    }
                ]
            },
            ...buildAIMetricCardTabs(this.transloco, stats, loading, (type) => this.activeFilterValue(type)),
            {
                id: 'audience',
                label: this.transloco.translate('common.metricGroups.audience'),
                icon: 'pi-users',
                cards: [
                    {
                        id: 'devices',
                        title: this.transloco.translate('common.metrics.devices'),
                        icon: 'pi-mobile',
                        data: stats?.top_devices ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('device'),
                        filterType: 'device'
                    },
                    {
                        id: 'browsers',
                        title: this.transloco.translate('common.metrics.browsers'),
                        icon: 'pi-globe',
                        data: stats?.top_browsers ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('browser'),
                        showBrowserIcons: true,
                        filterType: 'browser'
                    },
                    {
                        id: 'languages',
                        title: this.transloco.translate('common.metrics.languages'),
                        icon: 'pi-language',
                        data: stats?.top_languages ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('language'),
                        showLanguageFlags: true,
                        showLanguageNames: true,
                        filterType: 'language'
                    }
                ]
            },
            {
                id: 'location',
                label: this.transloco.translate('common.metricGroups.location'),
                icon: 'pi-map',
                cards: [
                    {
                        id: 'countries',
                        title: this.transloco.translate('common.metrics.countries'),
                        icon: 'pi-map',
                        data: stats?.top_countries ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('country'),
                        showCountryFlags: true,
                        showCountryNames: true,
                        filterType: 'country'
                    },
                    {
                        id: 'cities',
                        title: this.transloco.translate('common.metrics.cities'),
                        icon: 'pi-map-marker',
                        data: stats?.top_cities ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('city'),
                        filterType: 'city'
                    }
                ]
            },
            {
                id: 'network',
                label: this.transloco.translate('common.metricGroups.network'),
                icon: 'pi-server',
                cards: [
                    {
                        id: 'providers',
                        title: this.transloco.translate('common.metrics.providers'),
                        icon: 'pi-server',
                        data: stats?.top_providers ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('provider'),
                        filterType: 'provider'
                    },
                    {
                        id: 'asns',
                        title: this.transloco.translate('common.metrics.asns'),
                        icon: 'pi-sitemap',
                        data: stats?.top_asns ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('asn'),
                        filterType: 'asn'
                    }
                ]
            }
        ];
    });
    protected readonly showOnboarding = computed(() => {
        const onboarding = this.onboarding.onboarding();
        return !this.isShareMode() && !!onboarding && !onboarding.dismissed && !onboarding.complete;
    });
    protected readonly onboardingSteps = computed(() => this.onboarding.onboarding()?.steps ?? []);
    protected readonly onboardingRailSteps = computed<WorkflowProgressStep[]>(() => {
        this.activeLanguage();
        const steps = this.onboardingSteps();
        const activeIndex = steps.findIndex((step) => !step.complete);
        return steps.map((step, index) => {
            const state: WorkflowProgressStep['state'] = step.complete ? 'complete' : index === activeIndex ? 'current' : 'pending';
            return {
                id: step.key,
                label: this.onboardingStepLabel(step),
                state
            };
        });
    });
    protected readonly currentOnboardingStep = computed(() => this.onboardingSteps().find((step) => !step.complete) ?? null);
    protected readonly onboardingProgress = computed(() => {
        const steps = this.onboardingSteps();
        if (!steps.length) {
            return { complete: 0, total: 0 };
        }
        return {
            complete: steps.filter((step) => step.complete).length,
            total: steps.length
        };
    });
    protected exportUrl = computed(() => {
        const shareToken = this.shareService.token();
        const site = this.siteService.activeSite();
        const dates = this.getCurrentDateRange();
        if (!site || !dates) return '';

        const params = new URLSearchParams({
            from: dates.from,
            to: dates.to
        });
        for (const filter of this.activeFilters()) {
            params.append('filter', `${filter.type}:${filter.value}`);
        }
        if (this.isShareMode() && shareToken) {
            return `/api/share/${encodeURIComponent(shareToken)}/sites/${site.id}/hits/export?${params.toString()}`;
        }
        return `/api/sites/${site.id}/hits/export?${params.toString()}`;
    });
    protected readonly kpiCards = computed<KpiCardData[]>(() => {
        this.activeLanguage();
        const stats = this.stats();
        const loading = this.isStatsLoading();
        const updateKey = this.kpiUpdateKey();
        const cmp = stats?.comparison;
        const baseClass = 'text-2xl xl:text-3xl font-bold';
        const liveVisitors = stats?.live_visitors ?? 0;
        return [
            {
                id: 'live_visitors',
                label: this.transloco.translate('dashboard.kpis.liveVisitors'),
                value: liveVisitors,
                loading,
                updateKey,
                valueClass: liveVisitors > 0 ? `${baseClass} text-green-600 dark:text-green-400` : baseClass,
                delta: null
            },
            {
                id: 'total_pageviews',
                label: this.transloco.translate('dashboard.kpis.pageviews'),
                value: stats?.total_pageviews ?? 0,
                loading,
                updateKey,
                valueClass: baseClass,
                delta: cmp ? this.calcDelta(stats?.total_pageviews ?? 0, cmp.total_pageviews) : null
            },
            {
                id: 'unique_sessions',
                label: this.transloco.translate('dashboard.kpis.uniqueSessions'),
                value: stats?.unique_sessions ?? 0,
                loading,
                updateKey,
                valueClass: baseClass,
                delta: cmp ? this.calcDelta(stats?.unique_sessions ?? 0, cmp.unique_sessions) : null
            },
            {
                id: 'bounce_rate',
                label: this.transloco.translate('dashboard.kpis.bounceRate'),
                value: stats?.bounce_rate ?? 0,
                loading,
                updateKey,
                valueClass: baseClass,
                format: KPI_PERCENT_FORMAT,
                suffix: '%',
                delta: cmp ? this.calcDelta(stats?.bounce_rate ?? 0, cmp.bounce_rate) : null,
                invertDelta: true
            },
            {
                id: 'avg_session_duration',
                label: this.transloco.translate('dashboard.kpis.avgDuration'),
                value: stats?.avg_session_duration ?? 0,
                loading,
                updateKey,
                valueClass: baseClass,
                duration: true,
                delta: cmp ? this.calcDelta(stats?.avg_session_duration ?? 0, cmp.avg_session_duration) : null
            },
            {
                id: 'pages_per_session',
                label: this.transloco.translate('dashboard.kpis.pagesPerSession'),
                value: stats?.pages_per_session ?? 0,
                loading,
                updateKey,
                valueClass: baseClass,
                format: KPI_SHORT_DECIMAL_FORMAT,
                delta: cmp ? this.calcDelta(stats?.pages_per_session ?? 0, cmp.pages_per_session) : null
            }
        ];
    });

    protected openTrackingSettings() {
        const site = this.siteService.activeSite();
        if (!site) return;
        void this.router.navigate(['/sites', site.id, 'settings', 'tracking']);
    }
    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        const site = this.siteService.activeSite();
        if (!site) {
            return [
                {
                    label: this.transloco.translate('dashboard.breadcrumbOverview'),
                    isCurrent: true
                }
            ];
        }
        return [{ label: site.domain, favicon: site, isCurrent: true }];
    });

    private searchSubject = new Subject<string>();
    protected searchQuery = signal('');
    private lastTableEvent: TableLazyLoadEvent | null = null;
    protected readonly isShortRange = this.reportRange.isShortRange;
    protected chartTitle = computed(() => {
        this.activeLanguage();
        const range = this.reportRange.selectedRange();

        if (range.value !== 'custom') {
            return this.transloco.translate('dashboard.chartTitleWithRange', {
                range: range.label ?? translateRangeLabel(this.transloco, range.value)
            });
        }

        const dates = this.reportRange.customRangeDates();
        if (dates && dates.length === 2 && dates[0] && dates[1]) {
            const start = this.localeService.localizeDate(dates[0], undefined, {
                month: 'short',
                day: 'numeric'
            });
            const end = this.localeService.localizeDate(dates[1], undefined, {
                month: 'short',
                day: 'numeric',
                year: 'numeric'
            });
            return this.transloco.translate('dashboard.chartTitleCustomRange', {
                start,
                end
            });
        }

        return this.transloco.translate('dashboard.chartTitleOverview');
    });
    constructor() {
        this.searchSubject.pipe(debounceTime(400), distinctUntilChanged(), takeUntilDestroyed(this.destroyRef)).subscribe((q) => {
            this.searchQuery.set(q);
            this.refreshHits();
        });

        this.refreshOnboarding();

        effect(() => {
            if (this.reportLinkApplied) return;
            const siteID = this.route.snapshot.queryParamMap.get('site');
            const from = this.route.snapshot.queryParamMap.get('from');
            const to = this.route.snapshot.queryParamMap.get('to');
            const sites = this.siteService.sites();
            if (siteID && sites.length > 0) {
                const site = sites.find((candidate) => candidate.id === siteID);
                if (site) this.siteService.selectSite(site);
            }
            if (from && to) {
                const start = new Date(`${from}T00:00:00`);
                const end = new Date(`${to}T23:59:59.999`);
                if (!Number.isNaN(start.getTime()) && !Number.isNaN(end.getTime()) && start <= end) {
                    this.reportRange.selectRange({ value: { value: 'custom' }, customRange: [start, end] });
                }
            }
            if ((!siteID || sites.length > 0) && (!from || !to || this.reportRange.selectedRange().value === 'custom')) {
                this.reportLinkApplied = true;
            }
        });

        effect(() => {
            const site = this.siteService.activeSite();
            const dates = this.getCurrentDateRange();
            if (site && dates) {
                this.loadStatsForCurrentRange();
                this.refreshHits();
            }
        });

        this.realtimeRefresh.registerUntilDestroyed(this.destroyRef, {
            siteId: () => this.siteService.activeSite()?.id ?? null,
            kinds: REALTIME_ALL_ANALYTICS_KINDS,
            enabled: () => !!this.siteService.activeSite() && !!this.getCurrentDateRange(),
            refresh: () => this.refreshRealtimeData(),
            debounceMs: 600
        });
    }

    refreshAll() {
        this.loadStatsForCurrentRange();
        this.refreshHits();
        this.refreshOnboarding();
        this.searchConsoleRefreshKey.update((key) => key + 1);
    }

    private refreshOnboarding() {
        if (this.isShareMode()) {
            return;
        }
        this.onboarding
            .load()
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({ error: () => undefined });
    }

    protected dismissOnboarding() {
        this.onboarding
            .dismiss()
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({ error: () => undefined });
    }

    protected onboardingStepLabel(step: OnboardingStep): string {
        this.activeLanguage();
        return this.transloco.translate(`dashboard.onboarding.steps.${step.key}`);
    }

    protected onboardingStepAction(step: OnboardingStep): string {
        this.activeLanguage();
        const key = `dashboard.onboarding.actions.${step.key}`;
        const label = this.transloco.translate(key);
        return label === key ? this.transloco.translate('common.actions.open') : label;
    }

    protected runOnboardingAction(step: OnboardingStep) {
        switch (step.key) {
            case 'create_site':
                this.isAddSiteVisible.set(true);
                break;
            case 'verify_tracking':
            case 'automatic_events':
                if (step.site_id) {
                    const site = this.siteService.sites().find((candidate) => candidate.id === step.site_id);
                    if (site) {
                        this.siteService.selectSite(site);
                    }
                }
                this.openTrackingSettings();
                break;
            case 'invite_teammate':
                void this.router.navigate(['/admin/team/members/invite']);
                break;
            case 'schedule_report':
                void this.router.navigate(['/settings/reports']);
                break;
        }
    }

    onSearch(event: Event) {
        this.searchSubject.next((event.target as HTMLInputElement).value);
    }

    protected onMetricCardClick(event: MetricCardGroupRowClick): void {
        this.applyMetricFilter(event.filterType as MetricFilterType, event.metric);
    }

    loadHits(event: TableLazyLoadEvent) {
        this.lastTableEvent = event;
        const site = this.siteService.activeSite();
        const dates = this.getCurrentDateRange();
        if (!site || !dates) return;
        const filters = this.activeFilters();

        const rows = event.rows || 10;
        const first = event.first || 0;
        const page = first / rows + 1;

        this.hitService.loadHits(site.id, dates.from, dates.to, page, rows, event.sortField as string, event.sortOrder === 1 ? 'asc' : 'desc', this.searchQuery(), filters);
    }

    private refreshHits() {
        if (this.lastTableEvent) {
            this.lastTableEvent.first = 0;
            this.loadHits(this.lastTableEvent);
        }
    }

    private refreshStatsOnly(mode: StatsQueryMode = 'blocking') {
        this.loadStatsForCurrentRange(mode);
    }

    private refreshRealtimeData() {
        this.refreshStatsOnly('background');
        this.refreshHits();
        this.refreshOnboarding();
    }

    protected comparisonLabel = computed(() => {
        this.activeLanguage();
        const r = this.currentComparisonRange();
        if (!r) return '';
        const showYear = new Date(r.from).getFullYear() !== new Date().getFullYear();
        const opts = showYear ? ({ month: 'short', day: 'numeric', year: 'numeric' } as const) : ({ month: 'short', day: 'numeric' } as const);
        const fmt = (d: string) => this.localeService.localizeDate(new Date(d), undefined, opts);
        return `${fmt(r.from)} – ${fmt(r.to)}`;
    });

    private loadStatsForCurrentRange(mode: StatsQueryMode = 'blocking') {
        const site = this.siteService.activeSite();
        const dates = this.getCurrentDateRange();
        const filters = this.activeFilters();
        if (!site || !dates) return;
        const effectiveMode = mode === 'background' && this.stats() && !this.isStatsLoading() ? 'background' : 'blocking';
        this.statsQuery.load({
            siteId: site.id,
            from: dates.from,
            to: dates.to,
            filters,
            mode: effectiveMode,
            onSuccess: effectiveMode === 'background' ? () => this.kpiUpdateKey.update((key) => key + 1) : undefined
        });
    }

    protected readonly calcDelta = calcDelta;

    protected getCurrentDateRange() {
        return this.reportRange.currentDateRange();
    }

    protected openFunnelViewer(funnel: Funnel) {
        this.selectedFunnelId.set(funnel.id);
        this.showFunnelViewer.set(true);
    }

    protected openFunnelManager() {
        this.funnelEditRequestId.set(null);
        this.showFunnelManager.set(true);
    }

    protected setFunnelManagerVisible(visible: boolean) {
        this.showFunnelManager.set(visible);
        if (!visible) {
            this.funnelEditRequestId.set(null);
        }
    }

    protected editSelectedFunnel() {
        const funnelId = this.selectedFunnelId();
        if (!funnelId) return;
        this.funnelEditRequestId.set(funnelId);
        this.showFunnelViewer.set(false);
        this.showFunnelManager.set(true);
    }

    protected applyMetricFilter(type: MetricFilterType, metric: MetricStat) {
        if (!metric.name) return;
        this.activeFilters.update((filters) => {
            const existingIndex = filters.findIndex((filter) => filter.type === type);
            if (existingIndex >= 0) {
                const existing = filters[existingIndex];
                if (existing.value === metric.name) {
                    return filters.filter((_, idx) => idx !== existingIndex);
                }
                const next = [...filters];
                next[existingIndex] = { type, value: metric.name };
                return next;
            }
            return [...filters, { type, value: metric.name }];
        });
    }

    protected clearFilter() {
        this.activeFilters.set([]);
    }

    protected removeFilter(type: MetricFilterType, value: string) {
        this.activeFilters.update((filters) => filters.filter((filter) => !(filter.type === type && filter.value === value)));
    }

    protected activeFilterValue(type: MetricFilterType): string | null {
        return this.activeFilters().find((filter) => filter.type === type)?.value ?? null;
    }

    private filterLabel(filter: MetricFilter): string {
        switch (filter.type) {
            case 'path':
                return this.transloco.translate('common.filters.page', {
                    value: filter.value
                });
            case 'referrer':
                return this.transloco.translate('common.filters.source', {
                    value: filter.value
                });
            case 'device':
                return this.transloco.translate('common.filters.device', {
                    value: filter.value
                });
            case 'country':
                return this.transloco.translate('common.filters.country', {
                    value: filter.value
                });
            case 'city':
                return this.transloco.translate('common.filters.city', {
                    value: filter.value
                });
            case 'provider':
                return this.transloco.translate('common.filters.provider', {
                    value: filter.value
                });
            case 'asn':
                return this.transloco.translate('common.filters.asn', {
                    value: filter.value
                });
            case 'browser':
                return this.transloco.translate('common.filters.browser', {
                    value: filter.value
                });
            case 'language':
                return this.transloco.translate('common.filters.language', {
                    value: this.displayLanguageLabel(filter.value)
                });
            case 'ai_bot':
            case 'ai_bot_category':
            case 'ai_source':
                return aiFilterChipLabel(this.transloco, filter.type, filter.value);
            default:
                return `${filter.type}: ${filter.value}`;
        }
    }

    protected exportFiltered(format: TakeoutExportFormat = DEFAULT_HITS_EXPORT_FORMAT) {
        const url = withTakeoutExportFormat(this.exportUrl(), format);
        if (!url || this.isExportingFiltered()) return;

        this.isExportingFiltered.set(true);
        this.filteredExportState.set('idle');

        this.takeoutDownloadService
            .downloadFromUrl(url, buildTakeoutExportFilename(this.siteService.activeSite()?.domain, 'hits', format))
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isExportingFiltered.set(false))
            )
            .subscribe({
                next: () => this.filteredExportState.set('success'),
                error: () => this.filteredExportState.set('error')
            });
    }

    protected buildSiteUrl(path: string | null | undefined): string | null {
        const domain = this.siteDomain();
        if (!domain || !path) return null;
        const normalized = path.startsWith('/') ? path : `/${path}`;
        return `https://${domain}${normalized}`;
    }

    protected buildReferrerUrl(referrer: string | null | undefined): string | null {
        const url = this.normalizeUrl(referrer);
        return url ? url.href : null;
    }

    // TODO: Refactor global url vanity handling at some point
    protected displayReferrerUrl(url: string | null | undefined): string {
        if (!url) return '';

        return url.replace(/^https?:\/\//, '').replace(/^www\./, '');
    }

    protected referrerDomain(referrer: string | null | undefined): string | null {
        const url = this.normalizeUrl(referrer);
        return url ? url.hostname : null;
    }

    protected faviconUrlForDomain(domain: string | null | undefined): string | null {
        return domain ? browserAppUrl(this.document, `/api/favicon/${encodeURIComponent(domain)}`) : null;
    }

    private normalizeUrl(raw: string | null | undefined): URL | null {
        if (!raw) return null;
        const trimmed = raw.trim();
        if (!trimmed || trimmed.toLowerCase() === 'direct') return null;
        const normalized = /^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`;
        try {
            return new URL(normalized);
        } catch {
            return null;
        }
    }

    private displayLanguageLabel(value: string): string {
        const code = value.trim().toLowerCase();
        if (!/^[a-z]{2,3}$/.test(code)) {
            return value;
        }
        try {
            const displayNames = new Intl.DisplayNames([this.transloco.getActiveLang()], { type: 'language' });
            return displayNames.of(code) ?? value;
        } catch {
            return value;
        }
    }
}
