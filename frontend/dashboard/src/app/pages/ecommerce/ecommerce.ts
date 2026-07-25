import { ChangeDetectionStrategy, Component, computed, DestroyRef, effect, inject, signal } from '@angular/core';
import { DOCUMENT, NgOptimizedImage } from '@angular/common';
import { ReactiveFormsModule } from '@angular/forms';
import { finalize, forkJoin } from 'rxjs';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { injectActiveLang } from '@core/i18n/active-lang';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { ButtonModule } from '@openng/optimus-ui/button';
import { CardModule } from '@openng/optimus-ui/card';
import { IconFieldModule } from '@openng/optimus-ui/iconfield';
import { InputIconModule } from '@openng/optimus-ui/inputicon';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { TableModule } from '@openng/optimus-ui/table';
import { TabsModule } from '@openng/optimus-ui/tabs';
import { SiteService } from '@features/sites/services/site.service';
import { AnalyticsService } from '@core/services/analytics.service';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { NoSiteSelected } from '@components/no-site-selected/no-site-selected';
import { SetupCallout } from '@components/setup-callout/setup-callout';
import { KPI_MONEY_FALLBACK_FORMAT, KPI_PERCENT_FORMAT, KpiCard, KpiCardModel } from '@features/analytics/components/kpi-card';
import { ReportRangeToolbar } from '@components/report-range-toolbar/report-range-toolbar';
import { MetricCardGroup, MetricCardGroupRowClick, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import { SeriesChart, SeriesChartPoint, SeriesDefinition } from '@features/analytics/components/series-chart';
import { EcommerceProductStat, EcommerceSeriesPoint, EcommerceSourceStat, EcommerceSummary, MetricStat, SiteStats } from '@models/analytics.types';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';
import { RealtimeRefreshCoordinator } from '@services/realtime-refresh-coordinator.service';
import { REALTIME_EVENT_KINDS } from '@services/realtime.service';
import { injectReportRange } from '@services/report-range-preferences.service';
import { SetupStateService } from '@services/setup-state.service';

type MetricFilterType = 'referrer' | 'device' | 'country' | 'city' | 'provider' | 'asn' | 'utm_source';

interface MetricFilter {
    type: MetricFilterType;
    value: string;
}

interface ProductFilter {
    itemId: string;
    itemName: string;
}

type DataLoadMode = 'blocking' | 'background';

@Component({
    selector: 'app-ecommerce',
    imports: [
        NgOptimizedImage,
        ReactiveFormsModule,
        TranslocoPipe,
        ButtonModule,
        CardModule,
        IconFieldModule,
        InputIconModule,
        InputTextModule,
        TableModule,
        TabsModule,
        PageHeader,
        PageHeaderLeft,
        PageBreadcrumb,
        ReportRangeToolbar,
        KpiCard,
        MetricCardGroup,
        SeriesChart,
        NoSiteSelected,
        SetupCallout
    ],
    templateUrl: './ecommerce.html',
    styleUrl: './ecommerce.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class EcommercePage {
    protected readonly ecommerceDocsUrl = 'https://hitkeep.com/guides/analytics/ecommerce/';
    protected siteService = inject(SiteService);
    private analyticsService = inject(AnalyticsService);
    private localeService = inject(TranslocoLocaleService);
    private transloco = inject(TranslocoService);
    private document = inject(DOCUMENT);
    private destroyRef = inject(DestroyRef);
    private realtimeRefresh = inject(RealtimeRefreshCoordinator);
    private readonly setupState = inject(SetupStateService);
    private readonly reportRange = injectReportRange();
    private readonly activeLanguage = injectActiveLang();

    protected readonly summary = signal<EcommerceSummary | null>(null);
    protected readonly series = signal<EcommerceSeriesPoint[]>([]);
    protected readonly products = signal<EcommerceProductStat[]>([]);
    protected readonly sources = signal<EcommerceSourceStat[]>([]);
    protected readonly filterStats = signal<SiteStats | null>(null);
    protected readonly isLoading = signal(false);
    protected readonly kpiUpdateKey = signal(0);

    protected readonly isShortRange = this.reportRange.isShortRange;

    /**
     * True only once the shared setup state confirms the site never sent an
     * ecommerce event. Empty ranges on an instrumented shop keep the regular
     * empty states.
     */
    protected readonly needsSetup = computed(() => {
        const summary = this.summary();
        return this.setupState.needsSetup(this.siteService.activeSite()?.id, 'has_ecommerce_events', summary ? summary.orders + summary.checkout_starts : null, this.isLoading());
    });

    protected readonly activeFilters = signal<MetricFilter[]>([]);
    protected readonly selectedProduct = signal<ProductFilter | null>(null);
    protected readonly hasFilters = computed(() => this.activeFilters().length > 0 || this.selectedProduct() !== null);
    protected readonly summaryCurrency = computed(() => this.resolveCurrency(this.summary()?.currency));
    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        const site = this.siteService.activeSite();
        if (!site) {
            return [{ label: this.transloco.translate('nav.ecommerce'), isCurrent: true }];
        }
        return [
            { label: site.domain, favicon: site, routerLink: '/dashboard' },
            { label: this.transloco.translate('nav.ecommerce'), isCurrent: true }
        ];
    });

    protected readonly kpiCards = computed<KpiCardModel[]>(() => {
        this.activeLanguage();
        const summary = this.summary();
        const loading = this.isLoading();
        const updateKey = this.kpiUpdateKey();
        const currency = this.resolveCurrency(summary?.currency);
        const moneyFormat: Intl.NumberFormatOptions = currency
            ? {
                  style: 'currency',
                  currency,
                  maximumFractionDigits: 2
              }
            : KPI_MONEY_FALLBACK_FORMAT;

        return [
            {
                label: this.transloco.translate('ecommerce.kpis.revenue'),
                value: summary?.revenue ?? 0,
                format: moneyFormat,
                loading,
                updateKey,
                valueClass: 'text-2xl xl:text-3xl font-bold'
            },
            {
                label: this.transloco.translate('ecommerce.kpis.orders'),
                value: summary?.orders ?? 0,
                loading,
                updateKey,
                valueClass: 'text-2xl xl:text-3xl font-bold'
            },
            {
                label: this.transloco.translate('ecommerce.kpis.averageOrderValue'),
                value: summary?.average_order_value ?? 0,
                format: moneyFormat,
                loading,
                updateKey,
                valueClass: 'text-2xl xl:text-3xl font-bold'
            },
            {
                label: this.transloco.translate('ecommerce.kpis.checkoutConversion'),
                value: summary?.checkout_conversion_rate ?? 0,
                format: KPI_PERCENT_FORMAT,
                suffix: '%',
                loading,
                updateKey,
                valueClass: 'text-2xl xl:text-3xl font-bold'
            }
        ];
    });
    protected readonly chartData = computed<SeriesChartPoint[]>(() =>
        this.series().map((point) => ({
            time: point.time,
            revenue: point.revenue,
            orders: point.orders
        }))
    );
    protected readonly chartConfig = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        return [
            {
                key: 'revenue',
                label: this.transloco.translate('ecommerce.chart.revenue'),
                color: '#0f9d58',
                gradientFrom: 'rgba(15, 157, 88, 0.45)',
                gradientTo: 'rgba(15, 157, 88, 0.0)'
            },
            {
                key: 'orders',
                label: this.transloco.translate('ecommerce.chart.orders'),
                color: '#2563eb',
                gradientFrom: 'rgba(37, 99, 235, 0.35)',
                gradientTo: 'rgba(37, 99, 235, 0.0)'
            }
        ];
    });
    protected readonly filterChips = computed(() => {
        this.activeLanguage();
        const chips = this.activeFilters().map((filter) => ({
            key: `${filter.type}:${filter.value}`,
            label: this.filterLabel(filter),
            remove: () => this.removeFilter(filter.type, filter.value)
        }));
        const product = this.selectedProduct();
        if (product) {
            chips.push({
                key: `item:${product.itemId || product.itemName}`,
                label: this.transloco.translate('ecommerce.filters.product', {
                    value: product.itemName || product.itemId
                }),
                remove: () => this.selectedProduct.set(null)
            });
        }
        return chips;
    });
    protected readonly metricCardTabs = computed<MetricCardGroupTab<MetricFilterType>[]>(() => {
        this.activeLanguage();
        const stats = this.filterStats();
        const summary = this.summary();
        const loading = this.isLoading();
        return [
            {
                id: 'acquisition',
                label: this.transloco.translate('common.metricGroups.acquisition'),
                icon: 'pi-link',
                cards: [
                    {
                        id: 'utm-sources',
                        title: this.transloco.translate('ecommerce.filtersPanels.sources'),
                        icon: 'pi-link',
                        data: stats?.top_utm_sources ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('utm_source'),
                        filterType: 'utm_source'
                    },
                    {
                        id: 'referrers',
                        title: this.transloco.translate('ecommerce.filtersPanels.referrers'),
                        icon: 'pi-share-alt',
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
                        title: this.transloco.translate('ecommerce.filtersPanels.devices'),
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
                        title: this.transloco.translate('ecommerce.filtersPanels.countries'),
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
                        data: summary?.top_cities ?? [],
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
                        data: summary?.top_providers ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('provider'),
                        filterType: 'provider'
                    },
                    {
                        id: 'asns',
                        title: this.transloco.translate('common.metrics.asns'),
                        icon: 'pi-sitemap',
                        data: summary?.top_asns ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeFilterValue('asn'),
                        filterType: 'asn'
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
            const product = this.selectedProduct();
            if (!site || !dates) {
                this.summary.set(null);
                this.series.set([]);
                this.products.set([]);
                this.sources.set([]);
                this.filterStats.set(null);
                return;
            }
            this.loadData(site.id, dates.from, dates.to, filters, product);
        });
        this.realtimeRefresh.registerUntilDestroyed(this.destroyRef, {
            siteId: () => this.siteService.activeSite()?.id ?? null,
            kinds: REALTIME_EVENT_KINDS,
            enabled: () => !!this.siteService.activeSite() && !!this.getCurrentDateRange(),
            refresh: () => this.refreshData('background'),
            debounceMs: 700
        });
    }

    protected refreshData(mode: DataLoadMode = 'blocking') {
        const site = this.siteService.activeSite();
        const dates = this.getCurrentDateRange();
        if (!site || !dates) {
            return;
        }
        this.loadData(site.id, dates.from, dates.to, this.activeFilters(), this.selectedProduct(), mode);
    }

    protected applyMetricFilter(type: MetricFilterType, metric: MetricStat) {
        if (!metric.name) {
            return;
        }
        this.activeFilters.update((filters) => {
            const existingIndex = filters.findIndex((filter) => filter.type === type);
            if (existingIndex >= 0) {
                const existing = filters[existingIndex];
                if (existing.value === metric.name) {
                    return filters.filter((_, index) => index !== existingIndex);
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

    protected activeFilterValue(type: MetricFilterType): string | null {
        return this.activeFilters().find((filter) => filter.type === type)?.value ?? null;
    }

    protected removeFilter(type: MetricFilterType, value: string) {
        this.activeFilters.update((filters) => filters.filter((filter) => !(filter.type === type && filter.value === value)));
    }

    protected clearAllFilters() {
        this.activeFilters.set([]);
        this.selectedProduct.set(null);
    }

    protected toggleProductFilter(product: EcommerceProductStat) {
        const current = this.selectedProduct();
        if (current && current.itemId === product.item_id && current.itemName === product.item_name) {
            this.selectedProduct.set(null);
            return;
        }
        this.selectedProduct.set({
            itemId: product.item_id,
            itemName: product.item_name
        });
    }

    protected isProductFilterActive(product: EcommerceProductStat): boolean {
        const current = this.selectedProduct();
        return current?.itemId === product.item_id && current?.itemName === product.item_name;
    }

    protected toggleSourceFilter(source: EcommerceSourceStat): void {
        const sourceValue = source.utm_source?.trim();
        if (!sourceValue) {
            return;
        }
        this.applyMetricFilter('utm_source', {
            name: sourceValue,
            value: source.orders
        });
    }

    protected isSourceFilterActive(source: EcommerceSourceStat): boolean {
        const sourceValue = source.utm_source?.trim();
        return !!sourceValue && this.activeFilterValue('utm_source') === sourceValue;
    }

    protected sourceLinkUrl(value: string | null | undefined): string | null {
        const url = this.normalizeUrl(value);
        return url ? url.href : null;
    }

    protected sourceDomain(value: string | null | undefined): string | null {
        const url = this.normalizeUrl(value);
        return url ? url.hostname : null;
    }

    protected sourceDisplayUrl(value: string | null | undefined): string {
        if (!value) return '';
        return value.replace(/^https?:\/\//, '').replace(/^www\./, '');
    }

    protected faviconUrlForDomain(domain: string | null | undefined): string | null {
        return domain ? browserAppUrl(this.document, `/api/favicon/${encodeURIComponent(domain)}`) : null;
    }

    protected formatCurrency(value: number, currency: string): string {
        return new Intl.NumberFormat(this.activeLanguage(), {
            style: 'currency',
            currency,
            maximumFractionDigits: 2
        }).format(value);
    }

    protected formatMoney(value: number, currency: string | null): string {
        if (currency) {
            return this.formatCurrency(value, currency);
        }

        return this.localeService.localizeNumber(value, 'decimal', undefined, {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2
        });
    }

    protected formatNumber(value: number): string {
        return this.localeService.localizeNumber(value, 'decimal');
    }

    protected formatPercent(value: number): string {
        return this.localeService.localizeNumber(value, 'decimal', undefined, {
            minimumFractionDigits: 1,
            maximumFractionDigits: 1
        });
    }

    protected resolveCurrency(currency: string | null | undefined): string | null {
        const normalized = (currency ?? '').trim().toUpperCase();
        if (/^[A-Z]{3}$/.test(normalized)) {
            return normalized;
        }
        return null;
    }

    private loadData(siteId: string, from: string, to: string, filters: MetricFilter[], product: ProductFilter | null, mode: DataLoadMode = 'blocking') {
        const blocking = mode === 'blocking' || this.isLoading() || !this.summary();
        if (blocking) this.isLoading.set(true);
        forkJoin({
            summary: this.analyticsService.getEcommerceSummary(siteId, from, to, filters, product?.itemId, product?.itemName),
            series: this.analyticsService.getEcommerceTimeseries(siteId, from, to, filters, product?.itemId, product?.itemName),
            products: this.analyticsService.getEcommerceProducts(siteId, from, to, filters, product?.itemId, product?.itemName),
            sources: this.analyticsService.getEcommerceSources(siteId, from, to, filters, product?.itemId, product?.itemName),
            stats: this.analyticsService.getSiteStats(siteId, from, to, undefined, undefined, filters)
        })
            .pipe(
                finalize(() => {
                    if (blocking) this.isLoading.set(false);
                })
            )
            .subscribe({
                next: ({ summary, series, products, sources, stats }) => {
                    this.summary.set(summary);
                    this.series.set(series);
                    this.products.set(products);
                    this.sources.set(sources);
                    this.filterStats.set(stats);
                    if (!blocking) this.kpiUpdateKey.update((key) => key + 1);
                },
                error: (error) => console.error(error)
            });
    }

    private filterLabel(filter: MetricFilter): string {
        switch (filter.type) {
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
            case 'utm_source':
                return this.transloco.translate('ecommerce.filters.utmSource', {
                    value: filter.value
                });
            default:
                return `${filter.type}: ${filter.value}`;
        }
    }

    private getCurrentDateRange() {
        return this.reportRange.currentDateRange();
    }

    private normalizeUrl(raw: string | null | undefined): URL | null {
        if (!raw) return null;
        const trimmed = raw.trim();
        if (!trimmed || trimmed.toLowerCase() === 'direct' || trimmed.startsWith('(')) return null;
        const normalized = /^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`;
        try {
            return new URL(normalized);
        } catch {
            return null;
        }
    }
}
