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
import { AnalyticsService } from '@services/analytics.service';
import { StatsService } from '@features/analytics/services/stats.service';
import { FunnelList } from '@features/analytics/components/funnel-list';
import { MetricCardGroup, MetricCardGroupRowClick, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import { FunnelManager } from '@features/funnels/components/funnel-manager';
import { FunnelViewer } from '@features/funnels/components/funnel-viewer';
import { Funnel } from '@models/analytics.types';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { SeriesChart, SeriesDefinition, SeriesChartPoint } from '@features/analytics/components/series-chart';
import { FunnelSeriesPoint } from '@models/analytics.types';
import { KpiCard } from '@features/analytics/components/kpi-card';
import { DEFAULT_RANGE_OPTIONS, RangeOption, RangeToolbar } from '@components/range-toolbar/range-toolbar';
import { finalize } from 'rxjs';

type MetricFilterType = 'path' | 'referrer' | 'device' | 'country' | 'city' | 'provider' | 'asn';
interface MetricFilter {
    type: MetricFilterType;
    value: string;
}

@Component({
    selector: 'app-funnels',
    standalone: true,
    imports: [ReactiveFormsModule, ButtonModule, CardModule, SelectModule, PageHeader, PageHeaderLeft, PageBreadcrumb, RangeToolbar, SeriesChart, KpiCard, MetricCardGroup, FunnelList, FunnelManager, FunnelViewer, TranslocoPipe],
    templateUrl: './funnels.html',
    styleUrl: './funnels.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class Funnels {
    protected siteService = inject(SiteService);
    protected analyticsService = inject(AnalyticsService);
    protected statsService = inject(StatsService);
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
    private readonly funnelFilterFormModel = signal({
        funnelFilter: new FormControl<{ id: string; name: string } | null>(null),
        customRangeDates: new FormControl<Date[] | null>(null)
    });
    protected readonly funnelFilterForm = compatForm(this.funnelFilterFormModel);
    protected isShortRange = computed(() => {
        if (this.selectedRange().value === '24h') return true;
        const customRangeDates = this.funnelFilterForm.customRangeDates().value();
        if (this.selectedRange().value === 'custom' && customRangeDates) {
            const d = customRangeDates;
            if (d.length === 2 && d[0] && d[1]) {
                const diff = d[1].getTime() - d[0].getTime();
                return diff < 48 * 60 * 60 * 1000;
            }
        }
        return false;
    });
    protected isFunnelManagerVisible = signal(false);
    protected isFunnelViewerVisible = signal(false);
    protected selectedFunnel = signal<Funnel | null>(null);
    protected funnelEditRequestId = signal<string | null>(null);

    protected funnels = signal<Funnel[]>([]);
    protected loading = signal(false);
    protected funnelsLoaded = signal(false);
    protected isRefreshing = computed(() => this.statsService.isLoading() || this.isFunnelSeriesLoading() || this.loading());
    protected funnelSeries = signal<FunnelSeriesPoint[]>([]);
    protected funnelSeriesChart = computed<SeriesChartPoint[]>(() =>
        this.funnelSeries().map((point) => ({
            time: point.time,
            entries: point.entries,
            completions: point.completions
        }))
    );
    protected comparisonFunnelSeries = signal<FunnelSeriesPoint[]>([]);
    protected comparisonFunnelSeriesChart = computed<SeriesChartPoint[]>(() =>
        this.comparisonFunnelSeries().map((point) => ({
            time: point.time,
            entries: point.entries,
            completions: point.completions
        }))
    );
    protected isFunnelSeriesLoading = signal(false);
    protected isComparisonFunnelSeriesLoading = signal(false);
    protected activeFunnelFilters = signal<{ id: string; name: string }[]>([]);
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

    protected readonly funnelKpis = computed(() => {
        this.activeLanguage();
        const activeIds = new Set(this.activeFunnelFilters().map((filter) => filter.id));
        const funnelsCount = activeIds.size > 0 ? this.funnels().filter((funnel) => activeIds.has(funnel.id)).length : this.funnels().length;
        const entries = this.funnelSeries().reduce((sum, point) => sum + point.entries, 0);
        const completions = this.funnelSeries().reduce((sum, point) => sum + point.completions, 0);
        const completionRate = entries > 0 ? (completions / entries) * 100 : 0;
        const cmpEntries = this.comparisonFunnelSeries().reduce((sum, point) => sum + point.entries, 0);
        const cmpCompletions = this.comparisonFunnelSeries().reduce((sum, point) => sum + point.completions, 0);
        const cmpCompletionRate = cmpEntries > 0 ? (cmpCompletions / cmpEntries) * 100 : 0;

        return [
            {
                label: this.transloco.translate('funnels.kpis.funnels'),
                value: funnelsCount,
                loading: this.loading(),
                valueClass: 'text-2xl xl:text-3xl font-bold',
                delta: null as number | null
            },
            {
                label: this.transloco.translate('funnels.kpis.entries'),
                value: entries,
                loading: this.isFunnelSeriesLoading(),
                valueClass: 'text-2xl xl:text-3xl font-bold',
                delta: this.calcDelta(entries, cmpEntries)
            },
            {
                label: this.transloco.translate('funnels.kpis.completions'),
                value: completions,
                loading: this.isFunnelSeriesLoading(),
                valueClass: 'text-2xl xl:text-3xl font-bold',
                delta: this.calcDelta(completions, cmpCompletions)
            },
            {
                label: this.transloco.translate('funnels.kpis.completionRate'),
                value: `${this.localeService.localizeNumber(completionRate, 'decimal', undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 })}%`,
                loading: this.isFunnelSeriesLoading(),
                valueClass: 'text-2xl xl:text-3xl font-bold',
                delta: this.calcDelta(completionRate, cmpCompletionRate)
            }
        ];
    });
    protected readonly funnelSeriesConfig = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        return [
            {
                key: 'entries',
                label: this.transloco.translate('funnels.kpis.entries'),
                color: '#6366f1',
                gradientFrom: 'rgba(99, 102, 241, 0.5)',
                gradientTo: 'rgba(99, 102, 241, 0.0)'
            },
            {
                key: 'completions',
                label: this.transloco.translate('funnels.kpis.completions'),
                color: '#14b8a6',
                gradientFrom: 'rgba(20, 184, 166, 0.5)',
                gradientTo: 'rgba(20, 184, 166, 0.0)'
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
            return [{ label: this.transloco.translate('nav.funnels'), isCurrent: true }];
        }
        return [
            { label: site.domain, favicon: site, routerLink: '/dashboard' },
            { label: this.transloco.translate('nav.funnels'), isCurrent: true }
        ];
    });
    protected readonly viewerDateRange = computed(() => this.getDateRange());

    constructor() {
        effect(() => {
            const site = this.siteService.activeSite();
            if (site) {
                this.loadFunnels();
            } else {
                this.funnels.set([]);
                this.funnelsLoaded.set(false);
            }
        });

        effect(() => {
            const site = this.siteService.activeSite();
            const filters = this.activeFunnelFilters();
            const metricFilters = this.activeFilters();
            const dates = this.getCurrentDateRange();
            if (site && dates && this.funnelsLoaded()) {
                const funnelIds = this.getFunnelIdsForFilters();
                if (funnelIds.length === 0 && filters.length === 0) {
                    this.statsService.stats.set(null);
                    return;
                }
                this.loadFunnelSeries(site.id, dates.from, dates.to, funnelIds);
                this.statsService.loadStats(site.id, dates.from, dates.to, metricFilters, [], funnelIds);
                const cmpRange = untracked(() => this.statsService.currentComparisonRange());
                if (cmpRange) {
                    this.loadComparisonFunnelSeries(site.id, cmpRange.from, cmpRange.to, funnelIds);
                }
            }
        });
    }

    loadFunnels() {
        const site = this.siteService.activeSite();
        if (!site) return;
        this.loading.set(true);
        this.funnelsLoaded.set(false);
        this.analyticsService.getFunnels(site.id).subscribe({
            next: (data) => {
                this.funnels.set(data);
                this.loading.set(false);
                this.funnelsLoaded.set(true);
            },
            error: () => {
                this.loading.set(false);
                this.funnelsLoaded.set(true);
            }
        });
    }

    protected availableFunnelFilters = computed(() => {
        const selected = new Set(this.activeFunnelFilters().map((filter) => filter.id));
        return this.funnels()
            .filter((funnel) => !selected.has(funnel.id))
            .map((funnel) => ({ label: funnel.name, value: { id: funnel.id, name: funnel.name } }));
    });

    protected addFunnelFilter(filter: { id: string; name: string } | null) {
        if (!filter) return;
        const active = this.activeFunnelFilters();
        if (active.some((existing) => existing.id === filter.id)) return;
        this.activeFunnelFilters.set([...active, filter]);
    }

    protected onFunnelFilterSelect(filter: { id: string; name: string } | null): void {
        this.addFunnelFilter(filter);
        this.funnelFilterForm.funnelFilter().control().setValue(null, { emitEvent: false });
    }

    protected removeFunnelFilter(id: string) {
        this.activeFunnelFilters.update((list) => list.filter((item) => item.id !== id));
    }

    protected clearFunnelFilters() {
        this.activeFunnelFilters.set([]);
    }

    private getFunnelIdsForFilters(): string[] {
        const active = this.activeFunnelFilters();
        if (active.length > 0) {
            return active.map((filter) => filter.id);
        }
        return this.funnels().map((funnel) => funnel.id);
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

    private loadFunnelSeries(siteId: string, from: string, to: string, funnelIds: string[]) {
        this.isFunnelSeriesLoading.set(true);
        this.analyticsService
            .getFunnelTimeseries(siteId, from, to, funnelIds)
            .pipe(finalize(() => this.isFunnelSeriesLoading.set(false)))
            .subscribe({
                next: (data) => this.funnelSeries.set(data ?? []),
                error: () => this.funnelSeries.set([])
            });
    }

    private loadComparisonFunnelSeries(siteId: string, from: string, to: string, funnelIds: string[]) {
        this.isComparisonFunnelSeriesLoading.set(true);
        this.analyticsService
            .getFunnelTimeseries(siteId, from, to, funnelIds)
            .pipe(finalize(() => this.isComparisonFunnelSeriesLoading.set(false)))
            .subscribe({
                next: (data) => this.comparisonFunnelSeries.set(data ?? []),
                error: () => this.comparisonFunnelSeries.set([])
            });
    }

    protected calcDelta(current: number, previous: number): number | null {
        if (previous === 0) return null;
        return ((current - previous) / previous) * 100;
    }

    protected refreshStats() {
        const site = this.siteService.activeSite();
        const dates = this.getCurrentDateRange();
        if (!site || !dates) return;

        const filters = this.activeFunnelFilters();
        const metricFilters = this.activeFilters();
        const funnelIds = this.getFunnelIdsForFilters();

        if (funnelIds.length === 0 && filters.length === 0) {
            this.statsService.stats.set(null);
            return;
        }

        this.loadFunnelSeries(site.id, dates.from, dates.to, funnelIds);
        this.statsService.loadStats(site.id, dates.from, dates.to, metricFilters, [], funnelIds);
        const cmpRange = this.statsService.currentComparisonRange();
        if (cmpRange) {
            this.loadComparisonFunnelSeries(site.id, cmpRange.from, cmpRange.to, funnelIds);
        }
    }

    protected getCurrentDateRange() {
        const range = this.selectedRange();
        const end = new Date();
        const start = new Date();

        if (range.value === 'custom') {
            const d = this.funnelFilterForm.customRangeDates().value();
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

    openFunnelManager() {
        this.funnelEditRequestId.set(null);
        this.isFunnelManagerVisible.set(true);
    }

    setFunnelManagerVisible(visible: boolean) {
        this.isFunnelManagerVisible.set(visible);
        if (!visible) {
            this.funnelEditRequestId.set(null);
        }
    }

    viewFunnel(funnel: Funnel) {
        this.selectedFunnel.set(funnel);
        this.isFunnelViewerVisible.set(true);
    }

    editSelectedFunnel() {
        const funnel = this.selectedFunnel();
        if (!funnel) return;
        this.funnelEditRequestId.set(funnel.id);
        this.isFunnelViewerVisible.set(false);
        this.isFunnelManagerVisible.set(true);
    }

    getDateRange() {
        const range = this.getCurrentDateRange();
        if (range) return range;
        const end = new Date();
        const start = new Date();
        start.setDate(end.getDate() - 30);
        return { from: start.toISOString(), to: end.toISOString() };
    }
}
