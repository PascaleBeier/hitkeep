import { ChangeDetectionStrategy, Component, inject, signal, effect, computed, linkedSignal, untracked } from '@angular/core';
import { injectActiveLang } from '@core/i18n/active-lang';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { ButtonModule } from 'primeng/button';
import { CardModule } from 'primeng/card';
import { SelectModule } from 'primeng/select';
import { SiteService } from '@features/sites/services/site.service';
import { StatsService } from '@features/analytics/services/stats.service';
import { AnalyticsService } from '@services/analytics.service';
import { GoalList } from '@features/analytics/components/goal-list';
import { MetricCardGroup, MetricCardGroupRowClick, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import { GoalManager } from '@features/goals/components/goal-manager';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { PageState } from '@components/page-state/page-state';
import { SeriesChart, SeriesDefinition, SeriesChartPoint } from '@features/analytics/components/series-chart';
import { Goal, GoalSeriesPoint, SiteStats } from '@models/analytics.types';
import { KpiCard } from '@features/analytics/components/kpi-card';
import { DEFAULT_RANGE_OPTIONS, RangeOption, RangeToolbar } from '@components/range-toolbar/range-toolbar';
import { finalize } from 'rxjs';

type MetricFilterType = 'path' | 'referrer' | 'device' | 'country' | 'city' | 'provider' | 'asn';
interface MetricFilter {
    type: MetricFilterType;
    value: string;
}

@Component({
    selector: 'app-goals',
    standalone: true,
    imports: [ReactiveFormsModule, ButtonModule, CardModule, SelectModule, PageHeader, PageHeaderLeft, PageBreadcrumb, PageState, RangeToolbar, SeriesChart, KpiCard, MetricCardGroup, GoalList, GoalManager, TranslocoPipe],
    templateUrl: './goals.html',
    styleUrl: './goals.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class Goals {
    protected siteService = inject(SiteService);
    protected statsService = inject(StatsService);
    private analyticsService = inject(AnalyticsService);
    private localeService = inject(TranslocoLocaleService);
    private transloco = inject(TranslocoService);
    private readonly activeLanguage = injectActiveLang();

    protected timeRanges = signal<RangeOption[]>(DEFAULT_RANGE_OPTIONS);
    protected selectedRange = linkedSignal<RangeOption[], RangeOption>({
        source: this.timeRanges,
        computation: (ranges, previous) => {
            const value = previous?.value.value ?? '30d';
            return ranges.find((r) => r.value === value) ?? ranges[2]!;
        }
    });
    private readonly goalFilterFormModel = signal({
        goalFilter: new FormControl<{ id: string; name: string } | null>(null),
        customRangeDates: new FormControl<Date[] | null>(null)
    });
    protected readonly goalFilterForm = compatForm(this.goalFilterFormModel);
    protected isShortRange = computed(() => {
        if (this.selectedRange().value === '24h') return true;
        const customRangeDates = this.goalFilterForm.customRangeDates().value();
        if (this.selectedRange().value === 'custom' && customRangeDates) {
            const d = customRangeDates;
            if (d.length === 2 && d[0] && d[1]) {
                const diff = d[1].getTime() - d[0].getTime();
                return diff < 48 * 60 * 60 * 1000;
            }
        }
        return false;
    });
    protected isGoalManagerVisible = signal(false);
    protected goals = signal<Goal[]>([]);
    protected goalsLoading = signal(false);
    protected baselineStats = signal<SiteStats | null>(null);
    protected baselineLoading = signal(false);
    protected isRefreshing = computed(() => this.statsService.isLoading() || this.isGoalSeriesLoading() || this.baselineLoading());
    protected goalSeries = signal<GoalSeriesPoint[]>([]);
    protected goalSeriesChart = computed<SeriesChartPoint[]>(() =>
        this.goalSeries().map((point) => ({
            time: point.time,
            conversions: point.conversions
        }))
    );
    protected comparisonGoalSeries = signal<GoalSeriesPoint[]>([]);
    protected comparisonGoalSeriesChart = computed<SeriesChartPoint[]>(() =>
        this.comparisonGoalSeries().map((point) => ({
            time: point.time,
            conversions: point.conversions
        }))
    );
    protected isGoalSeriesLoading = signal(false);
    protected isComparisonGoalSeriesLoading = signal(false);
    protected activeGoalFilters = signal<{ id: string; name: string }[]>([]);
    protected activeFilters = signal<{ type: MetricFilterType; value: string }[]>([]);
    protected hasFilters = computed(() => this.activeFilters().length > 0);
    protected filterChips = computed(() =>
        this.activeFilters().map((filter) => ({
            ...filter,
            label: this.filterLabel(filter)
        }))
    );

    protected comparisonLabel = computed(() => {
        this.activeLanguage();
        const r = this.statsService.currentComparisonRange();
        if (!r) return '';
        const showYear = new Date(r.from).getFullYear() !== new Date().getFullYear();
        const opts = showYear ? ({ month: 'short', day: 'numeric', year: 'numeric' } as const) : ({ month: 'short', day: 'numeric' } as const);
        const fmt = (d: string) => this.localeService.localizeDate(new Date(d), undefined, opts);
        return `${fmt(r.from)} – ${fmt(r.to)}`;
    });

    protected readonly goalKpis = computed(() => {
        this.activeLanguage();
        const activeIds = new Set(this.activeGoalFilters().map((filter) => filter.id));
        const goals = this.goals();
        const totalGoals = activeIds.size > 0 ? goals.filter((goal) => activeIds.has(goal.id)).length : goals.length;
        const totalConversions = this.goalSeries().reduce((sum, point) => sum + point.conversions, 0);
        const cmpTotalConversions = this.comparisonGoalSeries().reduce((sum, point) => sum + point.conversions, 0);
        const sessionsWithGoals = this.statsService.stats()?.unique_sessions ?? 0;
        const totalSessions = this.baselineStats()?.unique_sessions ?? 0;
        const conversionRate = totalSessions > 0 ? (sessionsWithGoals / totalSessions) * 100 : 0;
        const cmpTotalSessions = this.baselineStats()?.comparison?.unique_sessions ?? 0;
        const cmpConversionRate = cmpTotalSessions > 0 ? (cmpTotalConversions / cmpTotalSessions) * 100 : 0;
        const isLoading = this.statsService.isLoading() || this.baselineLoading();

        return [
            {
                label: this.transloco.translate('goals.kpis.totalGoals'),
                value: totalGoals,
                loading: isLoading,
                valueClass: 'text-2xl xl:text-3xl font-bold',
                delta: null as number | null
            },
            {
                label: this.transloco.translate('goals.kpis.conversions'),
                value: totalConversions,
                loading: isLoading,
                valueClass: 'text-2xl xl:text-3xl font-bold',
                delta: this.calcDelta(totalConversions, cmpTotalConversions)
            },
            {
                label: this.transloco.translate('common.kpis.conversionRate'),
                value: `${this.localeService.localizeNumber(conversionRate, 'decimal', undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 })}%`,
                loading: isLoading,
                valueClass: 'text-2xl xl:text-3xl font-bold',
                delta: this.calcDelta(conversionRate, cmpConversionRate)
            },
            {
                label: this.transloco.translate('dashboard.kpis.uniqueSessions'),
                value: totalSessions,
                loading: isLoading,
                valueClass: 'text-2xl xl:text-3xl font-bold',
                delta: this.calcDelta(totalSessions, cmpTotalSessions)
            }
        ];
    });
    protected readonly goalSeriesConfig = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        return [
            {
                key: 'conversions',
                label: this.transloco.translate('goals.kpis.conversions'),
                color: '#6366f1',
                gradientFrom: 'rgba(99, 102, 241, 0.5)',
                gradientTo: 'rgba(99, 102, 241, 0.0)'
            }
        ];
    });
    protected readonly metricCardTabs = computed<MetricCardGroupTab<MetricFilterType>[]>(() => {
        this.activeLanguage();
        const stats = this.statsService.stats();
        const loading = this.statsService.isLoading();
        const siteDomain = this.siteService.activeSite()?.domain ?? null;
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
                        icon: 'pi-globe',
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
                    { id: 'asns', title: this.transloco.translate('common.metrics.asns'), icon: 'pi-sitemap', data: stats?.top_asns ?? [], isLoading: loading, isRowClickable: true, activeValue: this.activeFilterValue('asn'), filterType: 'asn' }
                ]
            }
        ];
    });
    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        const site = this.siteService.activeSite();
        if (!site) {
            return [{ label: this.transloco.translate('nav.goals'), isCurrent: true }];
        }
        return [
            { label: site.domain, favicon: site, routerLink: '/dashboard' },
            { label: this.transloco.translate('nav.goals'), isCurrent: true }
        ];
    });

    constructor() {
        effect(() => {
            const site = this.siteService.activeSite();
            if (site) {
                this.loadGoals(site.id);
            } else {
                this.goals.set([]);
            }
        });

        effect(() => {
            const site = this.siteService.activeSite();
            const metricFilters = this.activeFilters();
            const dates = this.getCurrentDateRange();
            if (site && dates) {
                const goalIds = this.getGoalIdsForFilters();
                this.statsService.loadStats(site.id, dates.from, dates.to, metricFilters, goalIds, []);
                this.loadBaselineStats(site.id, dates.from, dates.to, metricFilters);
                this.loadGoalSeries(site.id, dates.from, dates.to, goalIds);
                const cmpRange = untracked(() => this.statsService.currentComparisonRange());
                if (cmpRange) {
                    this.loadComparisonGoalSeries(site.id, cmpRange.from, cmpRange.to, goalIds);
                }
            }
        });
    }

    openGoalManager() {
        this.isGoalManagerVisible.set(true);
    }

    refreshStats() {
        const site = this.siteService.activeSite();
        const dates = this.getCurrentDateRange();
        if (site && dates) {
            const goalIds = this.getGoalIdsForFilters();
            this.statsService.loadStats(site.id, dates.from, dates.to, this.activeFilters(), goalIds, []);
            this.loadBaselineStats(site.id, dates.from, dates.to, this.activeFilters());
            this.loadGoalSeries(site.id, dates.from, dates.to, goalIds);
            const cmpRange = this.statsService.currentComparisonRange();
            if (cmpRange) {
                this.loadComparisonGoalSeries(site.id, cmpRange.from, cmpRange.to, goalIds);
            }
        }
    }

    protected availableGoalFilters = computed(() => {
        const selected = new Set(this.activeGoalFilters().map((filter) => filter.id));
        return this.goals()
            .filter((goal) => !selected.has(goal.id))
            .map((goal) => ({ label: goal.name, value: { id: goal.id, name: goal.name } }));
    });

    protected addGoalFilter(filter: { id: string; name: string } | null) {
        if (!filter) return;
        const active = this.activeGoalFilters();
        if (active.some((existing) => existing.id === filter.id)) return;
        this.activeGoalFilters.set([...active, filter]);
    }

    protected onGoalFilterSelect(filter: { id: string; name: string } | null): void {
        this.addGoalFilter(filter);
        this.goalFilterForm.goalFilter().control().setValue(null, { emitEvent: false });
    }

    protected removeGoalFilter(id: string) {
        this.activeGoalFilters.update((list) => list.filter((item) => item.id !== id));
    }

    protected clearGoalFilters() {
        this.activeGoalFilters.set([]);
    }

    private getGoalIdsForFilters(): string[] {
        const active = this.activeGoalFilters();
        if (active.length > 0) {
            return active.map((filter) => filter.id);
        }
        return this.goals().map((goal) => goal.id);
    }

    private loadGoals(siteId: string) {
        this.goalsLoading.set(true);
        this.analyticsService
            .getGoals(siteId)
            .pipe(finalize(() => this.goalsLoading.set(false)))
            .subscribe({
                next: (data) => this.goals.set(data ?? []),
                error: () => this.goals.set([])
            });
    }

    private loadBaselineStats(siteId: string, from: string, to: string, filters: { type: MetricFilterType; value: string }[]) {
        this.baselineLoading.set(true);
        this.statsService
            .fetchStats(siteId, from, to, filters, [], [])
            .pipe(finalize(() => this.baselineLoading.set(false)))
            .subscribe({
                next: (data) => this.baselineStats.set(data),
                error: () => this.baselineStats.set(null)
            });
    }

    protected applyMetricFilter(type: MetricFilterType, metric: { name: string }) {
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

    protected onMetricCardClick(event: MetricCardGroupRowClick): void {
        this.applyMetricFilter(event.filterType as MetricFilterType, event.metric);
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
                return this.transloco.translate('common.filters.page', { value: filter.value });
            case 'referrer':
                return this.transloco.translate('common.filters.source', { value: filter.value });
            case 'device':
                return this.transloco.translate('common.filters.device', { value: filter.value });
            case 'country':
                return this.transloco.translate('common.filters.country', { value: filter.value });
            case 'city':
                return this.transloco.translate('common.filters.city', { value: filter.value });
            case 'provider':
                return this.transloco.translate('common.filters.provider', { value: filter.value });
            case 'asn':
                return this.transloco.translate('common.filters.asn', { value: filter.value });
            default:
                return `${filter.type}: ${filter.value}`;
        }
    }

    private loadGoalSeries(siteId: string, from: string, to: string, goalIds: string[]) {
        this.isGoalSeriesLoading.set(true);
        this.analyticsService
            .getGoalTimeseries(siteId, from, to, goalIds)
            .pipe(finalize(() => this.isGoalSeriesLoading.set(false)))
            .subscribe({
                next: (data) => this.goalSeries.set(data ?? []),
                error: () => this.goalSeries.set([])
            });
    }

    private loadComparisonGoalSeries(siteId: string, from: string, to: string, goalIds: string[]) {
        this.isComparisonGoalSeriesLoading.set(true);
        this.analyticsService
            .getGoalTimeseries(siteId, from, to, goalIds)
            .pipe(finalize(() => this.isComparisonGoalSeriesLoading.set(false)))
            .subscribe({
                next: (data) => this.comparisonGoalSeries.set(data ?? []),
                error: () => this.comparisonGoalSeries.set([])
            });
    }

    protected calcDelta(current: number, previous: number): number | null {
        if (previous === 0) return null;
        return ((current - previous) / previous) * 100;
    }

    protected getCurrentDateRange() {
        const range = this.selectedRange();
        const end = new Date();
        const start = new Date();

        if (range.value === 'custom') {
            const d = this.goalFilterForm.customRangeDates().value();
            if (d && d.length === 2 && d[0] && d[1]) {
                return { from: d[0].toISOString(), to: d[1].toISOString() };
            }
            return null;
        }

        switch (range.value) {
            case '24h':
                start.setHours(end.getHours() - 24);
                break;
            case '7d':
                start.setDate(end.getDate() - 7);
                break;
            case '30d':
                start.setDate(end.getDate() - 30);
                break;
            case '1y':
                start.setFullYear(end.getFullYear() - 1);
                break;
        }
        return { from: start.toISOString(), to: end.toISOString() };
    }
}
