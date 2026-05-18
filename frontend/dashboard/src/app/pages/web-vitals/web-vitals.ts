import { ChangeDetectionStrategy, Component, computed, effect, inject, linkedSignal, signal } from '@angular/core';
import { DOCUMENT, NgOptimizedImage } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { finalize, forkJoin } from 'rxjs';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { ButtonModule } from 'primeng/button';
import { CardModule } from 'primeng/card';
import { IconFieldModule } from 'primeng/iconfield';
import { InputIconModule } from 'primeng/inputicon';
import { InputTextModule } from 'primeng/inputtext';
import { SelectModule } from 'primeng/select';
import { TabsModule } from 'primeng/tabs';
import { TableModule } from 'primeng/table';
import { TagModule } from 'primeng/tag';
import { SiteService } from '@features/sites/services/site.service';
import { AnalyticsService } from '@core/services/analytics.service';
import { injectActiveLang } from '@core/i18n/active-lang';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { DEFAULT_RANGE_OPTIONS, RangeOption, RangeToolbar } from '@components/range-toolbar/range-toolbar';
import { SeriesChart, SeriesChartPoint, SeriesDefinition } from '@features/analytics/components/series-chart';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';
import { WebVitalDimension, WebVitalDimensionRow, WebVitalMetric, WebVitalMetricBreakdown, WebVitalPageRow, WebVitalRating, WebVitalSeriesPoint, WebVitalSummaryMetric } from '@models/analytics.types';

interface SelectOption<T> {
    label: string;
    value: T;
}

interface FilterChip {
    key: 'rating' | 'path';
    label: string;
    removable: boolean;
}

interface PathOption {
    label: string;
    value: string;
}

interface BreakdownTab {
    label: string;
    value: 'pages' | WebVitalDimension;
}

interface WebVitalPageTableRow {
    path: string;
    lcp: WebVitalMetricBreakdown | null;
    inp: WebVitalMetricBreakdown | null;
    cls: WebVitalMetricBreakdown | null;
    fcp: WebVitalMetricBreakdown | null;
    ttfb: WebVitalMetricBreakdown | null;
    samples: number;
    selectedMetricCell: WebVitalMetricBreakdown | null;
    ratingSamples: number;
}

const METRICS: WebVitalMetric[] = ['LCP', 'INP', 'CLS', 'FCP', 'TTFB'];
const DEFAULT_WEB_VITAL_PATH = '/';
const WEB_VITAL_THRESHOLDS: Record<WebVitalMetric, { good: number; poor: number }> = {
    LCP: { good: 2500, poor: 4000 },
    INP: { good: 200, poor: 500 },
    CLS: { good: 0.1, poor: 0.25 },
    FCP: { good: 1800, poor: 3000 },
    TTFB: { good: 800, poor: 1800 }
};
@Component({
    selector: 'app-web-vitals',
    imports: [
        NgOptimizedImage,
        FormsModule,
        TranslocoPipe,
        ButtonModule,
        CardModule,
        IconFieldModule,
        InputIconModule,
        InputTextModule,
        SelectModule,
        TabsModule,
        TableModule,
        TagModule,
        PageHeader,
        PageHeaderLeft,
        PageBreadcrumb,
        RangeToolbar,
        SeriesChart
    ],
    templateUrl: './web-vitals.html',
    styleUrl: './web-vitals.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class WebVitalsPage {
    protected readonly docsUrl = 'https://hitkeep.com/guides/analytics/web-vitals/';
    protected readonly siteService = inject(SiteService);
    private readonly analyticsService = inject(AnalyticsService);
    private readonly localeService = inject(TranslocoLocaleService);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = injectActiveLang();
    private readonly document = inject(DOCUMENT);

    protected readonly summary = signal<WebVitalSummaryMetric[]>([]);
    protected readonly series = signal<WebVitalSeriesPoint[]>([]);
    protected readonly pages = signal<WebVitalPageRow[]>([]);
    protected readonly dimensionRows = signal<WebVitalDimensionRow[]>([]);
    protected readonly isLoading = signal(false);
    protected readonly errorKey = signal<string | null>(null);

    protected readonly selectedMetric = signal<WebVitalMetric>('LCP');
    protected readonly selectedRating = signal<WebVitalRating | null>(null);
    protected readonly pathFilter = signal(DEFAULT_WEB_VITAL_PATH);
    protected readonly activeBreakdownTab = signal<'pages' | WebVitalDimension>('pages');
    protected readonly siteDomain = computed(() => this.siteService.activeSite()?.domain ?? null);
    protected readonly siteFaviconUrl = computed(() => {
        const domain = this.siteDomain();
        return domain ? browserAppUrl(this.document, `/api/favicon/${encodeURIComponent(domain)}`) : '';
    });

    protected readonly timeRanges = signal<RangeOption[]>(DEFAULT_RANGE_OPTIONS);
    protected readonly selectedRange = linkedSignal<RangeOption[], RangeOption>({
        source: this.timeRanges,
        computation: (ranges, previous) => {
            const value = previous?.value.value ?? '30d';
            return ranges.find((range) => range.value === value) ?? ranges[2]!;
        }
    });
    protected readonly customRangeDates = signal<Date[] | null>(null);
    protected readonly isShortRange = computed(() => {
        if (this.selectedRange().value === '24h') return true;
        const dates = this.customRangeDates();
        return this.selectedRange().value === 'custom' && dates?.length === 2 && !!dates[0] && !!dates[1] && dates[1].getTime() - dates[0].getTime() < 48 * 60 * 60 * 1000;
    });

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        const site = this.siteService.activeSite();
        if (!site) {
            return [{ label: this.transloco.translate('nav.webVitals'), isCurrent: true }];
        }
        return [
            { label: site.domain, favicon: site, routerLink: '/dashboard' },
            { label: this.transloco.translate('nav.webVitals'), isCurrent: true }
        ];
    });
    protected readonly metricOptions = computed<SelectOption<WebVitalMetric>[]>(() => {
        this.activeLanguage();
        return METRICS.map((metric) => ({ label: this.metricLabel(metric), value: metric }));
    });
    protected readonly ratingOptions = computed<SelectOption<WebVitalRating | null>[]>(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('webVitals.filters.allRatings'), value: null },
            { label: this.transloco.translate('webVitals.ratings.good'), value: 'good' },
            { label: this.transloco.translate('webVitals.ratings.needs_improvement'), value: 'needs_improvement' },
            { label: this.transloco.translate('webVitals.ratings.poor'), value: 'poor' }
        ];
    });
    protected readonly pathOptions = computed<PathOption[]>(() => {
        const options = new Map<string, PathOption>();
        options.set(DEFAULT_WEB_VITAL_PATH, { label: DEFAULT_WEB_VITAL_PATH, value: DEFAULT_WEB_VITAL_PATH });
        for (const row of this.pages()) {
            options.set(row.path, { label: row.path, value: row.path });
        }
        return [...options.values()].sort((a, b) => (a.value === DEFAULT_WEB_VITAL_PATH ? -1 : b.value === DEFAULT_WEB_VITAL_PATH ? 1 : a.value.localeCompare(b.value)));
    });
    protected readonly breakdownTabs = computed<BreakdownTab[]>(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('webVitals.breakdown.tabs.pages'), value: 'pages' },
            { label: this.transloco.translate('webVitals.breakdown.tabs.devices'), value: 'device' },
            { label: this.transloco.translate('webVitals.breakdown.tabs.browsers'), value: 'browser' },
            { label: this.transloco.translate('webVitals.breakdown.tabs.languages'), value: 'language' },
            { label: this.transloco.translate('webVitals.breakdown.tabs.countries'), value: 'country' },
            { label: this.transloco.translate('webVitals.breakdown.tabs.cities'), value: 'city' },
            { label: this.transloco.translate('webVitals.breakdown.tabs.providers'), value: 'provider' },
            { label: this.transloco.translate('webVitals.breakdown.tabs.asns'), value: 'asn' }
        ];
    });
    protected readonly metricCards = computed(() => {
        this.activeLanguage();
        const rows = this.summary();
        return METRICS.map((metric) => {
            const row = rows.find((item) => item.metric === metric);
            return {
                metric,
                label: this.metricLabel(metric),
                value: row ? this.formatMetricValue(metric, row.p75) : '-',
                thresholdPosition: row ? this.thresholdPosition(metric, row.p75) : 0,
                goodThreshold: this.formatMetricValue(metric, WEB_VITAL_THRESHOLDS[metric].good),
                poorThreshold: this.formatMetricValue(metric, WEB_VITAL_THRESHOLDS[metric].poor),
                samples: row?.samples ?? 0,
                rating: row?.rating ?? null,
                good: row?.good ?? 0,
                needsImprovement: row?.needs_improvement ?? 0,
                poor: row?.poor ?? 0,
                selected: this.selectedMetric() === metric
            };
        });
    });
    protected readonly selectedSummary = computed(() => this.summary().find((item) => item.metric === this.selectedMetric()) ?? null);
    protected readonly pageRows = computed<WebVitalPageTableRow[]>(() => {
        const metric = this.selectedMetric();
        const rating = this.selectedRating();
        return this.pages().map((row) => {
            const selectedMetricCell = this.metricBreakdownForMetric(row, metric);
            return {
                path: row.path,
                lcp: row.metrics?.LCP ?? null,
                inp: row.metrics?.INP ?? null,
                cls: row.metrics?.CLS ?? null,
                fcp: row.metrics?.FCP ?? null,
                ttfb: row.metrics?.TTFB ?? null,
                samples: row.samples,
                selectedMetricCell,
                ratingSamples: rating ? this.ratingCountForBreakdown(selectedMetricCell, rating) : row.samples
            };
        });
    });
    protected readonly pageTableSortField = computed(() => (this.selectedRating() ? 'ratingSamples' : 'samples'));
    protected readonly selectedRatingLabel = computed(() => {
        this.activeLanguage();
        const rating = this.selectedRating();
        return rating ? this.transloco.translate(`webVitals.ratings.${rating}`) : '';
    });
    protected readonly pageBreakdownTitle = computed(() => {
        this.activeLanguage();
        const rating = this.selectedRating();
        if (!rating) return this.transloco.translate('webVitals.pages.title');
        return this.transloco.translate('webVitals.pages.filteredTitle', {
            rating: this.selectedRatingLabel(),
            metric: this.selectedMetric()
        });
    });
    protected readonly pageBreakdownDescription = computed(() => {
        this.activeLanguage();
        const rating = this.selectedRating();
        if (!rating) {
            return this.transloco.translate('webVitals.pages.description', { metric: this.selectedMetric() });
        }
        return this.transloco.translate('webVitals.pages.filteredDescription', {
            rating: this.selectedRatingLabel(),
            metric: this.selectedMetric()
        });
    });
    protected readonly ratingCountColumnLabel = computed(() => {
        this.activeLanguage();
        return this.transloco.translate('webVitals.columns.ratingSamples', {
            rating: this.selectedRatingLabel()
        });
    });
    protected readonly chartData = computed<SeriesChartPoint[]>(() =>
        this.series().map((point) => ({
            time: point.time,
            p75: point.p75,
            good: point.good,
            needs_improvement: point.needs_improvement,
            poor: point.poor
        }))
    );
    protected readonly chartConfig = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        return [
            {
                key: 'p75',
                label: this.transloco.translate('webVitals.chart.p75', { metric: this.selectedMetric() }),
                color: '#2563eb',
                gradientFrom: 'rgba(37, 99, 235, 0.3)',
                gradientTo: 'rgba(37, 99, 235, 0)'
            }
        ];
    });
    protected readonly filterChips = computed<FilterChip[]>(() => {
        this.activeLanguage();
        const chips: FilterChip[] = [
            {
                key: 'path',
                label: this.transloco.translate('webVitals.filters.pathChip', { path: this.pathFilter() }),
                removable: this.pathFilter() !== DEFAULT_WEB_VITAL_PATH
            }
        ];
        const rating = this.selectedRating();
        if (rating) {
            chips.push({
                key: 'rating',
                label: this.transloco.translate('webVitals.filters.ratingChip', { rating: this.transloco.translate(`webVitals.ratings.${rating}`) }),
                removable: true
            });
        }
        return chips;
    });
    protected readonly hasCustomFilters = computed(() => this.selectedRating() !== null || this.pathFilter() !== DEFAULT_WEB_VITAL_PATH);

    constructor() {
        effect(() => {
            const site = this.siteService.activeSite();
            const dates = this.getCurrentDateRange();
            const metric = this.selectedMetric();
            const rating = this.selectedRating();
            const path = this.pathFilter();
            this.activeBreakdownTab();
            if (!site || !dates) {
                this.summary.set([]);
                this.series.set([]);
                this.pages.set([]);
                this.dimensionRows.set([]);
                return;
            }
            this.loadData(site.id, dates.from, dates.to, metric, path, rating);
        });
    }

    protected refreshData() {
        const site = this.siteService.activeSite();
        const dates = this.getCurrentDateRange();
        if (!site || !dates) return;
        this.loadData(site.id, dates.from, dates.to, this.selectedMetric(), this.pathFilter(), this.selectedRating());
    }

    protected selectMetric(metric: WebVitalMetric) {
        this.selectedMetric.set(metric);
    }

    protected handleMetricCardKeydown(event: KeyboardEvent, metric: WebVitalMetric) {
        if (event.key !== 'Enter' && event.key !== ' ') return;
        event.preventDefault();
        this.selectMetric(metric);
    }

    protected clearPathFilter() {
        this.pathFilter.set(DEFAULT_WEB_VITAL_PATH);
    }

    protected clearRatingFilter() {
        this.selectedRating.set(null);
    }

    protected clearFilterChip(chip: FilterChip) {
        if (chip.key === 'rating') this.clearRatingFilter();
        if (chip.key === 'path') this.clearPathFilter();
    }

    protected clearAllFilters() {
        this.clearRatingFilter();
        this.clearPathFilter();
    }

    protected toggleRatingFilter(rating: WebVitalRating) {
        this.selectRatingFilter(this.selectedRating() === rating ? null : rating);
    }

    protected selectRatingFilter(rating: WebVitalRating | null) {
        this.selectedRating.set(rating);
        if (rating) this.activeBreakdownTab.set('pages');
    }

    protected setPathFromRow(path: string) {
        const value = this.normalizePathFilter(path);
        this.pathFilter.set(value);
    }

    protected selectPathFilter(path: string | null) {
        this.pathFilter.set(this.normalizePathFilter(path ?? DEFAULT_WEB_VITAL_PATH));
    }

    protected setBreakdownTab(value: string | number | undefined) {
        if (value === 'pages' || value === 'browser' || value === 'country' || value === 'language' || value === 'device' || value === 'city' || value === 'provider' || value === 'asn') {
            this.activeBreakdownTab.set(value);
        }
    }

    protected ratingSeverity(rating: WebVitalRating | null): 'success' | 'warn' | 'danger' | 'secondary' {
        switch (rating) {
            case 'good':
                return 'success';
            case 'needs_improvement':
                return 'warn';
            case 'poor':
                return 'danger';
            default:
                return 'secondary';
        }
    }

    protected ratingPercent(count: number, total: number): number {
        if (total <= 0) return 0;
        return (count / total) * 100;
    }

    protected formatMetricValue(metric: WebVitalMetric, value: number): string {
        if (metric === 'CLS') {
            return `${this.localeService.localizeNumber(value * 100, 'decimal', undefined, {
                minimumFractionDigits: 1,
                maximumFractionDigits: 1
            })}%`;
        }
        if ((metric === 'LCP' || metric === 'FCP') && value >= 1000) {
            return `${this.localeService.localizeNumber(value / 1000, 'decimal', undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 })} s`;
        }
        return `${this.localeService.localizeNumber(value, 'decimal', undefined, { maximumFractionDigits: 0 })} ms`;
    }

    protected metricCell(row: WebVitalPageTableRow, metric: WebVitalMetric): WebVitalMetricBreakdown | null {
        switch (metric) {
            case 'LCP':
                return row.lcp;
            case 'INP':
                return row.inp;
            case 'CLS':
                return row.cls;
            case 'FCP':
                return row.fcp;
            case 'TTFB':
                return row.ttfb;
        }
    }

    protected dimensionLabel(row: WebVitalDimensionRow): string {
        if (row.name === '(Unknown)' || row.name === '(Unspecified)') {
            return this.transloco.translate('webVitals.breakdown.unknown');
        }
        return row.name;
    }

    protected formatNumber(value: number): string {
        return this.localeService.localizeNumber(value, 'decimal');
    }

    protected metricLabel(metric: WebVitalMetric): string {
        return this.transloco.translate(`webVitals.metrics.${metric}`);
    }

    protected buildSiteUrl(path: string | null | undefined): string | null {
        const domain = this.siteDomain();
        if (!domain || !path) return null;
        const normalized = path.startsWith('/') ? path : `/${path}`;
        return `https://${domain}${normalized}`;
    }

    private thresholdPosition(metric: WebVitalMetric, value: number): number {
        const max = WEB_VITAL_THRESHOLDS[metric].poor;
        return Math.round(Math.min(100, Math.max(0, (value / max) * 100)));
    }

    private normalizePathFilter(value: string): string {
        const path = value.trim().split(/[?#]/, 1)[0]?.trim() || DEFAULT_WEB_VITAL_PATH;
        return path.startsWith('/') ? path : `/${path}`;
    }

    private metricBreakdownForMetric(row: WebVitalPageRow, metric: WebVitalMetric): WebVitalMetricBreakdown | null {
        return row.metrics?.[metric] ?? null;
    }

    private ratingCountForBreakdown(row: WebVitalMetricBreakdown | null, rating: WebVitalRating): number {
        if (!row) return 0;
        switch (rating) {
            case 'good':
                return row.good;
            case 'needs_improvement':
                return row.needs_improvement;
            case 'poor':
                return row.poor;
        }
    }

    private loadData(siteId: string, from: string, to: string, metric: WebVitalMetric, path: string | null, rating: WebVitalRating | null) {
        this.isLoading.set(true);
        this.errorKey.set(null);
        const activeTab = this.activeBreakdownTab();
        const dimension: WebVitalDimension = activeTab === 'pages' ? 'country' : activeTab;
        forkJoin({
            summary: this.analyticsService.getWebVitalsSummary(siteId, from, to, null, null, null),
            series: this.analyticsService.getWebVitalsTimeseries(siteId, from, to, metric, path, rating),
            pages: this.analyticsService.getWebVitalsPages(siteId, from, to, metric, null, rating, 100),
            dimensions: this.analyticsService.getWebVitalsBreakdown(siteId, from, to, metric, dimension, path, rating, 25)
        })
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: ({ summary, series, pages, dimensions }) => {
                    this.summary.set(summary);
                    this.series.set(series);
                    this.pages.set(pages);
                    this.dimensionRows.set(dimensions);
                },
                error: (error) => {
                    console.error(error);
                    this.errorKey.set('webVitals.error');
                    this.summary.set([]);
                    this.series.set([]);
                    this.pages.set([]);
                    this.dimensionRows.set([]);
                }
            });
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
