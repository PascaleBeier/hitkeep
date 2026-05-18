import { ChangeDetectionStrategy, Component, computed, effect, inject, linkedSignal, signal } from '@angular/core';
import { injectActiveLang } from '@core/i18n/active-lang';
import { ReactiveFormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { ButtonModule } from 'primeng/button';
import { CardModule } from 'primeng/card';
import { SiteService } from '@features/sites/services/site.service';
import { StatsService } from '@features/analytics/services/stats.service';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { PageState } from '@components/page-state/page-state';
import { KpiCard } from '@features/analytics/components/kpi-card';
import { DEFAULT_RANGE_OPTIONS, RangeOption, RangeToolbar } from '@components/range-toolbar/range-toolbar';
import { MetricCardGroup, MetricCardGroupRowClick, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import { SeriesChart, SeriesChartPoint, SeriesDefinition } from '@features/analytics/components/series-chart';

type MetricFilterType = 'utm_campaign' | 'utm_content' | 'utm_medium' | 'utm_source' | 'utm_term';
interface MetricFilter {
    type: MetricFilterType;
    value: string;
}

@Component({
    selector: 'app-utm-dashboard',
    standalone: true,
    imports: [ReactiveFormsModule, RouterLink, TranslocoPipe, ButtonModule, CardModule, PageHeader, PageHeaderLeft, PageBreadcrumb, PageState, RangeToolbar, KpiCard, MetricCardGroup, SeriesChart],
    templateUrl: './utm.html',
    styleUrl: './utm.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class UtmDashboard {
    protected siteService = inject(SiteService);
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
    protected readonly customRangeDates = signal<Date[] | null>(null);
    protected isRefreshing = computed(() => this.statsService.isLoading());
    protected isShortRange = computed(() => {
        if (this.selectedRange().value === '24h') return true;
        const customRangeDates = this.customRangeDates();
        if (this.selectedRange().value === 'custom' && customRangeDates) {
            const d = customRangeDates;
            if (d.length === 2 && d[0] && d[1]) {
                const diff = d[1].getTime() - d[0].getTime();
                return diff < 48 * 60 * 60 * 1000;
            }
        }
        return false;
    });
    protected activeFilters = signal<MetricFilter[]>([]);
    protected hasFilters = computed(() => this.activeFilters().length > 0);
    protected filterChips = computed(() =>
        this.activeFilters().map((filter) => ({
            ...filter,
            label: this.filterLabel(filter)
        }))
    );
    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        const site = this.siteService.activeSite();
        if (!site) {
            return [{ label: this.transloco.translate('nav.utm'), isCurrent: true }];
        }
        return [
            { label: site.domain, favicon: site, routerLink: '/dashboard' },
            { label: this.transloco.translate('nav.utm'), isCurrent: true }
        ];
    });

    protected comparisonLabel = computed(() => {
        this.activeLanguage();
        const r = this.statsService.currentComparisonRange();
        if (!r) return '';
        const showYear = new Date(r.from).getFullYear() !== new Date().getFullYear();
        const opts = showYear ? ({ month: 'short', day: 'numeric', year: 'numeric' } as const) : ({ month: 'short', day: 'numeric' } as const);
        const fmt = (d: string) => this.localeService.localizeDate(new Date(d), undefined, opts);
        return `${fmt(r.from)} – ${fmt(r.to)}`;
    });

    protected readonly utmSeriesData = computed<SeriesChartPoint[]>(() => {
        const chartData = this.statsService.stats()?.chart_data ?? [];
        return chartData.map((point) => ({
            time: point.time,
            pageviews: point.pageviews,
            visitors: point.visitors
        }));
    });
    protected readonly utmComparisonSeriesData = computed<SeriesChartPoint[]>(() => {
        const chartData = this.statsService.stats()?.comparison?.chart_data ?? [];
        return chartData.map((point) => ({
            time: point.time,
            pageviews: point.pageviews,
            visitors: point.visitors
        }));
    });
    protected readonly utmSeriesConfig = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        return [
            {
                key: 'pageviews',
                label: this.transloco.translate('dashboard.kpis.pageviews'),
                color: '#6366f1',
                gradientFrom: 'rgba(99, 102, 241, 0.5)',
                gradientTo: 'rgba(99, 102, 241, 0.0)'
            },
            {
                key: 'visitors',
                label: this.transloco.translate('dashboard.traffic.visitors'),
                color: '#14b8a6',
                gradientFrom: 'rgba(20, 184, 166, 0.5)',
                gradientTo: 'rgba(20, 184, 166, 0.0)'
            }
        ];
    });
    protected readonly utmKpis = computed(() => {
        this.activeLanguage();
        const stats = this.statsService.stats();
        const cmp = stats?.comparison;
        const loading = this.statsService.isLoading();

        return [
            {
                label: this.transloco.translate('utm.kpis.campaign'),
                value: stats?.utm_campaign_hits ?? 0,
                loading,
                valueClass: 'text-2xl xl:text-3xl font-bold',
                delta: cmp ? this.calcDelta(stats?.utm_campaign_hits ?? 0, cmp.utm_campaign_hits) : null
            },
            {
                label: this.transloco.translate('utm.kpis.content'),
                value: stats?.utm_content_hits ?? 0,
                loading,
                valueClass: 'text-2xl xl:text-3xl font-bold',
                delta: cmp ? this.calcDelta(stats?.utm_content_hits ?? 0, cmp.utm_content_hits) : null
            },
            {
                label: this.transloco.translate('utm.kpis.medium'),
                value: stats?.utm_medium_hits ?? 0,
                loading,
                valueClass: 'text-2xl xl:text-3xl font-bold',
                delta: cmp ? this.calcDelta(stats?.utm_medium_hits ?? 0, cmp.utm_medium_hits) : null
            },
            {
                label: this.transloco.translate('utm.kpis.source'),
                value: stats?.utm_source_hits ?? 0,
                loading,
                valueClass: 'text-2xl xl:text-3xl font-bold',
                delta: cmp ? this.calcDelta(stats?.utm_source_hits ?? 0, cmp.utm_source_hits) : null
            },
            {
                label: this.transloco.translate('utm.kpis.term'),
                value: stats?.utm_term_hits ?? 0,
                loading,
                valueClass: 'text-2xl xl:text-3xl font-bold',
                delta: cmp ? this.calcDelta(stats?.utm_term_hits ?? 0, cmp.utm_term_hits) : null
            }
        ];
    });
    protected readonly metricCardTabs = computed<MetricCardGroupTab<MetricFilterType>[]>(() => {
        this.activeLanguage();
        const stats = this.statsService.stats();
        const loading = this.statsService.isLoading();
        return [
            {
                id: 'acquisition',
                label: this.transloco.translate('common.metricGroups.acquisition'),
                icon: 'pi-link',
                cards: [
                    {
                        id: 'campaigns',
                        title: this.transloco.translate('utm.metrics.topCampaigns'),
                        icon: 'pi-tag',
                        data: stats?.top_utm_campaigns ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('utm_campaign'),
                        filterType: 'utm_campaign'
                    },
                    {
                        id: 'sources',
                        title: this.transloco.translate('utm.metrics.topSources'),
                        icon: 'pi-link',
                        data: stats?.top_utm_sources ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('utm_source'),
                        filterType: 'utm_source'
                    },
                    {
                        id: 'mediums',
                        title: this.transloco.translate('utm.metrics.topMediums'),
                        icon: 'pi-send',
                        data: stats?.top_utm_mediums ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('utm_medium'),
                        filterType: 'utm_medium'
                    },
                    {
                        id: 'contents',
                        title: this.transloco.translate('utm.metrics.topContents'),
                        icon: 'pi-file',
                        data: stats?.top_utm_contents ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('utm_content'),
                        filterType: 'utm_content'
                    },
                    {
                        id: 'terms',
                        title: this.transloco.translate('utm.metrics.topTerms'),
                        icon: 'pi-search',
                        data: stats?.top_utm_terms ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('utm_term'),
                        filterType: 'utm_term'
                    }
                ]
            }
        ];
    });

    constructor() {
        effect(() => {
            const site = this.siteService.activeSite();
            const dates = this.getCurrentDateRange();
            const filters = this.activeFilters();
            if (!site || !dates) {
                return;
            }
            this.statsService.loadStats(site.id, dates.from, dates.to, filters);
        });
    }

    protected refreshStats() {
        const site = this.siteService.activeSite();
        const dates = this.getCurrentDateRange();
        if (!site || !dates) {
            return;
        }
        this.statsService.loadStats(site.id, dates.from, dates.to, this.activeFilters());
    }

    protected calcDelta(current: number, previous: number): number | null {
        if (previous === 0) return null;
        return ((current - previous) / previous) * 100;
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
            case 'utm_campaign':
                return this.transloco.translate('utm.filters.campaign', { value: filter.value });
            case 'utm_content':
                return this.transloco.translate('utm.filters.content', { value: filter.value });
            case 'utm_medium':
                return this.transloco.translate('utm.filters.medium', { value: filter.value });
            case 'utm_source':
                return this.transloco.translate('utm.filters.source', { value: filter.value });
            case 'utm_term':
                return this.transloco.translate('utm.filters.term', { value: filter.value });
            default:
                return `${filter.type}: ${filter.value}`;
        }
    }

    private getCurrentDateRange() {
        const range = this.selectedRange();
        const end = new Date();
        const start = new Date();

        if (range.value === 'custom') {
            const dates = this.customRangeDates();
            if (dates && dates.length === 2 && dates[0] && dates[1]) {
                return { from: dates[0].toISOString(), to: dates[1].toISOString() };
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
