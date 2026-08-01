import { Component, effect, inject, signal, computed, ChangeDetectionStrategy, DestroyRef, untracked } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router } from '@angular/router';
import { injectActiveLang } from '@core/i18n/active-lang';
import { finalize } from 'rxjs';
import { TranslocoService } from '@jsverse/transloco';
import { TranslocoPipe } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
// OptimusUI
import { CardModule } from '@openng/optimus-ui/card';
import { ButtonModule } from '@openng/optimus-ui/button';
import { TooltipModule } from '@openng/optimus-ui/tooltip';
// Features
import { SiteService } from '@features/sites/services/site.service';
import { injectStatsQuery, type StatsQueryMode } from '@features/analytics/services/stats-query';
import { injectAIActivityQuery, type AIActivityQueryMode } from '@features/analytics/services/ai-activity-query';
import { TrafficRecordsCard } from '@features/hits/components/traffic-records-card';
import { RealtimeRefreshCoordinator } from '@services/realtime-refresh-coordinator.service';
import { REALTIME_ALL_ANALYTICS_KINDS } from '@services/realtime.service';
import { TrafficChart } from '@features/analytics/components/traffic-chart';
import { MetricCardGroup, MetricCardGroupAction, MetricCardGroupRowClick, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import { SearchConsoleDrilldown } from '@features/analytics/components/search-console-drilldown';
import type { Funnel, GoalStats, MetricStat } from '@models/analytics.types';
import type { ConversionSubject } from '@features/analytics/models/conversion-subject';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { WorkflowProgress, type WorkflowProgressStep } from '@components/workflow-progress/workflow-progress';
import { ExportSplitButton, ExportStatusBanner } from '@components/export-split-button/export-split-button';
import { FilterChipItem, FilterChipRow } from '@components/filter-chip-row/filter-chip-row';
import { KPI_PERCENT_FORMAT, KPI_SHORT_DECIMAL_FORMAT, KpiCard } from '@features/analytics/components/kpi-card';
import { ShareService } from '@services/share.service';
import { translateRangeLabel } from '@components/range-toolbar/range-toolbar';
import { ReportRangeToolbar } from '@components/report-range-toolbar/report-range-toolbar';
import { buildTakeoutExportFilename, DEFAULT_HITS_EXPORT_FORMAT, TakeoutExportFormat, withTakeoutExportFormat } from '@core/export/export-formats';
import { calcDelta } from '@core/analytics/delta-utils';
import { aiFilterChipLabel } from '@features/analytics/ai-category-labels';
import { buildAIMetricCardTabs } from '@features/analytics/ai-metric-cards';
import { TakeoutDownloadService } from '@services/takeout-download.service';
import { AddSiteDialog } from '@features/sites/components/add-site-dialog';
import { TeamService } from '@services/team.service';
import { OnboardingService, OnboardingStep } from '@services/onboarding.service';
import { injectReportRange } from '@services/report-range-preferences.service';
import { AccessService } from '@services/access.service';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { NavigationNoticeService } from '@services/navigation-notice.service';

type MetricFilterType = 'path' | 'referrer' | 'device' | 'country' | 'city' | 'provider' | 'asn' | 'browser' | 'language' | 'ai_bot' | 'ai_bot_category' | 'ai_source';
type DashboardMetricAction = MetricFilterType | ConversionSubject['kind'];
interface MetricFilter {
    type: MetricFilterType;
    value: string;
}
interface DashboardReportRequest {
    siteId: string;
    from: string;
    to: string;
    filters: MetricFilter[];
    subject: { kind: ConversionSubject['kind']; id: string } | null;
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
        TranslocoPipe,
        CardModule,
        ButtonModule,
        TooltipModule,
        PageHeader,
        PageHeaderLeft,
        PageBreadcrumb,
        WorkflowProgress,
        ReportRangeToolbar,
        ExportSplitButton,
        ExportStatusBanner,
        FilterChipRow,
        KpiCard,
        TrafficChart,
        MetricCardGroup,
        SearchConsoleDrilldown,
        AddSiteDialog,
        TrafficRecordsCard
    ],
    templateUrl: './dashboard.html',
    styleUrl: './dashboard.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class Dashboard {
    protected siteService = inject(SiteService);
    private shareService = inject(ShareService);
    private teamService = inject(TeamService);
    private takeoutDownloadService = inject(TakeoutDownloadService);
    private localeService = inject(TranslocoLocaleService);
    private transloco = inject(TranslocoService);
    private destroyRef = inject(DestroyRef);
    private router = inject(Router);
    private route = inject(ActivatedRoute);
    private onboarding = inject(OnboardingService);
    private realtimeRefresh = inject(RealtimeRefreshCoordinator);
    private access = inject(AccessService);
    private navigationNotice = inject(NavigationNoticeService);
    protected readonly reportRange = injectReportRange();
    private reportLinkApplied = false;
    private readonly activeLanguage = injectActiveLang();
    private statsQuery = injectStatsQuery();
    private aiActivityQuery = injectAIActivityQuery();
    private validationRequestKey: string | null = null;
    private validationAfterResultSequence = 0;
    protected isShareMode = computed(() => this.shareService.isShareMode());
    protected trafficShareToken = computed(() => this.shareService.token());
    protected stats = this.statsQuery.stats;
    protected isStatsLoading = this.statsQuery.isLoading;
    protected aiActivity = this.aiActivityQuery.report;
    protected isAIActivityLoading = this.aiActivityQuery.isLoading;
    protected currentComparisonRange = this.statsQuery.comparisonRange;
    protected readonly kpiUpdateKey = signal(0);
    protected selectedConversion = signal<ConversionSubject | null>(null, {
        equal: (a, b) => a?.kind === b?.kind && a?.id === b?.id && a?.name === b?.name
    });
    private requestedConversion = signal<{ kind: ConversionSubject['kind']; id: string } | null>(null, {
        equal: (a, b) => a?.kind === b?.kind && a?.id === b?.id
    });
    private conversionSiteId: string | null = null;
    protected searchConsoleRefreshKey = signal(0);
    protected isAddSiteVisible = signal(false);
    protected searchConsoleDateRange = computed(() => this.getCurrentDateRange());
    protected searchConsoleFilters = computed(() => ({
        path: this.activeFilterValue('path'),
        country: this.activeFilterValue('country'),
        device: this.activeFilterValue('device')
    }));
    protected siteDomain = computed(() => this.siteService.activeSite()?.domain ?? null);
    protected emptyTeamName = computed(() => this.teamService.activeTeam()?.name ?? null);
    protected activeFilters = signal<MetricFilter[]>([]);
    protected trafficRefreshKey = signal(0);
    protected trafficDateRange = computed(() => this.getCurrentDateRange());
    protected trafficGoalIds = computed(() => {
        const subject = this.requestedConversion();
        return subject?.kind === 'goal' ? [subject.id] : [];
    });
    protected trafficFunnelIds = computed(() => {
        const subject = this.requestedConversion();
        return subject?.kind === 'funnel' ? [subject.id] : [];
    });
    private reportRequest = computed<DashboardReportRequest | null>(
        () => {
            const site = this.siteService.activeSite();
            const dates = this.getCurrentDateRange();
            if (!site || !dates) return null;
            return {
                siteId: site.id,
                from: dates.from,
                to: dates.to,
                filters: this.activeFilters(),
                subject: this.requestedConversion()
            };
        },
        { equal: (a, b) => JSON.stringify(a) === JSON.stringify(b) }
    );
    protected hasFilters = computed(() => this.activeFilters().length > 0 || this.selectedConversion() !== null);
    protected canManageConversions = computed(() => {
        const site = this.siteService.activeSite();
        return !this.isShareMode() && !!site && this.access.canSite(site.id, SITE_CAPABILITIES.manageGoals);
    });
    protected isExportingFiltered = signal(false);
    protected filteredExportState = signal<'idle' | 'success' | 'error'>('idle');
    protected filterChips = computed<FilterChipItem[]>(() => {
        this.activeLanguage();
        const chips = this.activeFilters().map((filter) => ({
            key: `${filter.type}:${filter.value}`,
            label: this.filterLabel(filter),
            remove: () => this.removeFilter(filter.type, filter.value)
        }));
        const subject = this.selectedConversion();
        if (subject) {
            chips.unshift({
                key: `${subject.kind}:${subject.id}`,
                label: this.scopedAIHitLabel(this.transloco.translate(`dashboard.conversions.filters.${subject.kind}`, { name: subject.name }), true),
                remove: () => this.clearConversionSubject()
            });
        }
        return chips;
    });
    protected readonly metricCardTabs = computed<MetricCardGroupTab<DashboardMetricAction>[]>(() => {
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
            ...buildAIMetricCardTabs(this.transloco, stats, this.aiActivity(), loading, this.isAIActivityLoading(), (type) => this.activeFilterValue(type)),
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
            },
            {
                id: 'conversions',
                label: this.transloco.translate('dashboard.conversions.title'),
                icon: 'pi-chart-line',
                cards: [
                    {
                        id: 'goals',
                        title: this.transloco.translate('goals.list.title'),
                        data:
                            stats?.goals.map((goal) => ({
                                key: goal.goal_id,
                                name: goal.name,
                                value: goal.conversions,
                                shareLabel: `${this.localeService.localizeNumber(goal.conversion_rate, 'decimal', undefined, { maximumFractionDigits: 1 })}%`,
                                detailsHref: `/goals?goal=${encodeURIComponent(goal.goal_id)}`,
                                detailsAriaLabel: this.transloco.translate('goals.list.detailsAria', { name: goal.name })
                            })) ?? [],
                        linkMode: 'details',
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.selectedConversion()?.kind === 'goal' ? this.selectedConversion()?.id : null,
                        filterType: 'goal',
                        actionId: this.canManageConversions() ? 'goal' : undefined,
                        actionLabel: this.canManageConversions() ? this.transloco.translate('goals.list.createAction') : undefined
                    },
                    {
                        id: 'funnels',
                        title: this.transloco.translate('funnels.list.title'),
                        data:
                            stats?.funnels.map((funnel) => ({
                                key: funnel.id,
                                name: funnel.name,
                                value: funnel.steps.length,
                                valueLabel: this.transloco.translate('funnels.list.stepsCount', { count: funnel.steps.length }),
                                detailsHref: `/funnels?funnel=${encodeURIComponent(funnel.id)}`,
                                detailsAriaLabel: this.transloco.translate('funnels.list.detailsAria', { name: funnel.name })
                            })) ?? [],
                        linkMode: 'details',
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.selectedConversion()?.kind === 'funnel' ? this.selectedConversion()?.id : null,
                        showShare: false,
                        filterType: 'funnel',
                        actionId: this.canManageConversions() ? 'funnel' : undefined,
                        actionLabel: this.canManageConversions() ? this.transloco.translate('funnels.list.createAction') : undefined
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
        const subject = this.requestedConversion();
        if (subject) params.append(`${subject.kind}_id`, subject.id);
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
        this.route.queryParamMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
            const goalId = params.get('goal');
            const funnelId = params.get('funnel');
            this.requestedConversion.set(goalId ? { kind: 'goal', id: goalId } : funnelId ? { kind: 'funnel', id: funnelId } : null);
            if (goalId && funnelId) {
                void this.router.navigate([], { relativeTo: this.route, queryParams: { funnel: null }, queryParamsHandling: 'merge', replaceUrl: true });
            }
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
            const request = this.reportRequest();
            const siteId = site?.id ?? null;
            if (this.conversionSiteId && siteId !== this.conversionSiteId) {
                const hadConversion = this.requestedConversion() !== null;
                untracked(() => {
                    this.aiActivityQuery.clear();
                    this.clearConversionSubject();
                    // If there is no conversion chip to clear, no signal would
                    // cause this effect to run again for the new site. Start
                    // the activity request here so a site switch cannot leave
                    // the old report visible or skip the new site's cards.
                    if (!hadConversion) this.loadAIActivityForCurrentRange();
                });
                this.conversionSiteId = siteId;
                return;
            }
            this.conversionSiteId = siteId;
            if (request) {
                untracked(() => {
                    this.loadStats(request);
                    this.loadAIActivity(request);
                });
            } else {
                untracked(() => this.aiActivityQuery.clear());
            }
        });

        effect(() => {
            const request = this.requestedConversion();
            const stats = this.stats();
            const result = this.statsQuery.lastResult();
            const requestKey = request ? `${request.kind}:${request.id}` : null;
            if (!request || !stats || this.isStatsLoading() || this.validationRequestKey !== requestKey || !result || result.sequence <= this.validationAfterResultSequence) return;
            const subject = request.kind === 'goal' ? stats.goals.find((goal) => goal.goal_id === request.id) : stats.funnels.find((funnel) => funnel.id === request.id);
            if (!subject) {
                this.selectedConversion.set(null);
                this.requestedConversion.set(null);
                this.navigationNotice.show('dashboard.conversions.invalidSubject', { preserveNextNavigation: true });
                void this.router.navigate([], { relativeTo: this.route, queryParams: { goal: null, funnel: null }, queryParamsHandling: 'merge', replaceUrl: true });
                return;
            }
            const selected = this.selectedConversion();
            if (selected?.kind !== request.kind || selected.id !== request.id || selected.name !== subject.name) {
                this.selectedConversion.set({ kind: request.kind, id: request.id, name: subject.name });
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
        this.loadAIActivityForCurrentRange();
        this.trafficRefreshKey.update((key) => key + 1);
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

    protected onMetricCardClick(event: MetricCardGroupRowClick): void {
        if (event.filterType === 'goal') {
            const goal = this.stats()?.goals.find((candidate) => candidate.goal_id === event.metric.key);
            if (goal) this.toggleGoal(goal);
            return;
        }
        if (event.filterType === 'funnel') {
            const funnel = this.stats()?.funnels.find((candidate) => candidate.id === event.metric.key);
            if (funnel) this.toggleFunnel(funnel);
            return;
        }
        this.applyMetricFilter(event.filterType as MetricFilterType, event.metric);
    }

    protected onMetricCardAction(event: MetricCardGroupAction): void {
        if (event.actionId === 'goal' || event.actionId === 'funnel') {
            this.openConversionCreate(event.actionId);
        }
    }

    private refreshStatsOnly(mode: StatsQueryMode = 'blocking') {
        this.loadStatsForCurrentRange(mode);
        this.loadAIActivityForCurrentRange(mode);
    }

    private refreshRealtimeData() {
        this.refreshStatsOnly('background');
        this.trafficRefreshKey.update((key) => key + 1);
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
        const request = this.reportRequest();
        if (!request) return;
        this.loadStats(request, mode);
    }

    private loadAIActivityForCurrentRange(mode: AIActivityQueryMode = 'blocking') {
        const request = this.reportRequest();
        if (!request) {
            this.aiActivityQuery.clear();
            return;
        }
        this.loadAIActivity(request, mode);
    }

    private loadStats(request: DashboardReportRequest, mode: StatsQueryMode = 'blocking') {
        this.validationRequestKey = request.subject ? `${request.subject.kind}:${request.subject.id}` : null;
        this.validationAfterResultSequence = this.statsQuery.lastResult()?.sequence ?? 0;
        const effectiveMode = mode === 'background' && this.stats() && !this.isStatsLoading() ? 'background' : 'blocking';
        this.statsQuery.load({
            siteId: request.siteId,
            from: request.from,
            to: request.to,
            filters: request.filters,
            goalIds: request.subject?.kind === 'goal' ? [request.subject.id] : [],
            funnelIds: request.subject?.kind === 'funnel' ? [request.subject.id] : [],
            mode: effectiveMode,
            onSuccess: effectiveMode === 'background' ? () => this.kpiUpdateKey.update((key) => key + 1) : undefined
        });
    }

    private loadAIActivity(request: DashboardReportRequest, mode: AIActivityQueryMode = 'blocking') {
        const effectiveMode: AIActivityQueryMode = mode === 'background' && this.aiActivity() && !this.isAIActivityLoading() ? 'background' : 'blocking';
        this.aiActivityQuery.load(
            {
                siteId: request.siteId,
                from: request.from,
                to: request.to,
                filters: request.filters,
                goalIds: request.subject?.kind === 'goal' ? [request.subject.id] : [],
                funnelIds: request.subject?.kind === 'funnel' ? [request.subject.id] : []
            },
            effectiveMode
        );
    }

    protected readonly calcDelta = calcDelta;

    protected getCurrentDateRange() {
        return this.reportRange.currentDateRange();
    }

    protected toggleGoal(goal: GoalStats) {
        this.setConversionSubject({ kind: 'goal', id: goal.goal_id, name: goal.name });
    }

    protected toggleFunnel(funnel: Funnel) {
        this.setConversionSubject({ kind: 'funnel', id: funnel.id, name: funnel.name });
    }

    protected openConversionCreate(kind: ConversionSubject['kind']) {
        void this.router.navigate([kind === 'goal' ? '/goals' : '/funnels'], { queryParams: { create: '1' } });
    }

    private setConversionSubject(subject: ConversionSubject) {
        const current = this.requestedConversion();
        if (current?.kind === subject.kind && current.id === subject.id) {
            this.clearConversionSubject();
            return;
        }
        this.selectedConversion.set(subject);
        this.requestedConversion.set({ kind: subject.kind, id: subject.id });
        void this.router.navigate([], {
            relativeTo: this.route,
            queryParams: { goal: subject.kind === 'goal' ? subject.id : null, funnel: subject.kind === 'funnel' ? subject.id : null },
            queryParamsHandling: 'merge',
            replaceUrl: true
        });
    }

    protected clearConversionSubject() {
        if (this.selectedConversion()) this.selectedConversion.set(null);
        if (this.requestedConversion()) this.requestedConversion.set(null);
        void this.router.navigate([], { relativeTo: this.route, queryParams: { goal: null, funnel: null }, queryParamsHandling: 'merge', replaceUrl: true });
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
        this.clearConversionSubject();
    }

    protected removeFilter(type: MetricFilterType, value: string) {
        this.activeFilters.update((filters) => filters.filter((filter) => !(filter.type === type && filter.value === value)));
    }

    protected activeFilterValue(type: MetricFilterType): string | null {
        return this.activeFilters().find((filter) => filter.type === type)?.value ?? null;
    }

    private filterLabel(filter: MetricFilter): string {
        let label: string;
        switch (filter.type) {
            case 'path':
                label = this.transloco.translate('common.filters.page', {
                    value: filter.value
                });
                break;
            case 'referrer':
                label = this.transloco.translate('common.filters.source', {
                    value: filter.value
                });
                break;
            case 'device':
                label = this.transloco.translate('common.filters.device', {
                    value: filter.value
                });
                break;
            case 'country':
                label = this.transloco.translate('common.filters.country', {
                    value: filter.value
                });
                break;
            case 'city':
                label = this.transloco.translate('common.filters.city', {
                    value: filter.value
                });
                break;
            case 'provider':
                label = this.transloco.translate('common.filters.provider', {
                    value: filter.value
                });
                break;
            case 'asn':
                label = this.transloco.translate('common.filters.asn', {
                    value: filter.value
                });
                break;
            case 'browser':
                label = this.transloco.translate('common.filters.browser', {
                    value: filter.value
                });
                break;
            case 'language':
                label = this.transloco.translate('common.filters.language', {
                    value: this.displayLanguageLabel(filter.value)
                });
                break;
            case 'ai_bot':
            case 'ai_bot_category':
            case 'ai_source':
                label = aiFilterChipLabel(this.transloco, filter.type, filter.value);
                break;
            default:
                label = `${filter.type}: ${filter.value}`;
                break;
        }
        return this.scopedAIHitLabel(label, !['path', 'ai_bot', 'ai_bot_category'].includes(filter.type));
    }

    private scopedAIHitLabel(label: string, hitOnly: boolean): string {
        if (!hitOnly) return label;
        return `${label} · ${this.transloco.translate('aiAgents.filters.scopeHits')}`;
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
