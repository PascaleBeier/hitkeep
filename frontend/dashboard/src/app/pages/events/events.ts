import { ChangeDetectionStrategy, Component, computed, effect, inject, linkedSignal, signal, untracked } from '@angular/core';
import { FormsModule, ReactiveFormsModule } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { SelectModule } from 'primeng/select';
import { CardModule } from 'primeng/card';
import { SkeletonModule } from 'primeng/skeleton';
import { ButtonModule } from 'primeng/button';
import { MessageModule } from 'primeng/message';
import { SiteService } from '@features/sites/services/site.service';
import { AnalyticsService, EventDimensionFilter } from '@core/services/analytics.service';
import { MetricCardConfig, MetricCardGroup, MetricCardGroupRowClick, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import { DEFAULT_RANGE_OPTIONS, RangeToolbar, RangeOption } from '@components/range-toolbar/range-toolbar';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { SeriesChart, SeriesDefinition, SeriesChartPoint } from '@features/analytics/components/series-chart';
import { MetricStat, EventSeriesPoint, EventAudience } from '@models/analytics.types';
import { finalize } from 'rxjs';
import { injectActiveLang } from '@core/i18n/active-lang';

interface EventFilterChip {
    key: string;
    label: string;
    remove: () => void;
}

type EventDimensionFilterType = 'path' | 'referrer' | 'device' | 'country' | 'city' | 'provider' | 'asn';
type EventMetricFilterType = 'propertyValue' | EventDimensionFilterType;

interface EventOption {
    label: string;
    value: string;
    isAutomatic: boolean;
    icon: string;
}

const AUTOMATIC_EVENT_META: Record<string, { icon: string }> = {
    outbound_click: { icon: 'pi pi-external-link' },
    file_download: { icon: 'pi pi-download' },
    form_submit: { icon: 'pi pi-send' }
};
const AUTOMATIC_EVENT_NAMES = Object.keys(AUTOMATIC_EVENT_META);

@Component({
    selector: 'app-events',
    imports: [FormsModule, ReactiveFormsModule, TranslocoPipe, SelectModule, CardModule, SkeletonModule, ButtonModule, MessageModule, MetricCardGroup, RangeToolbar, PageHeader, PageHeaderLeft, PageBreadcrumb, SeriesChart],
    templateUrl: './events.html',
    styleUrl: './events.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class Events {
    protected readonly addEventsDocsUrl = 'https://hitkeep.com/guides/tracking/automatic-events/';
    private siteService = inject(SiteService);
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
    protected readonly customRangeDates = signal<Date[] | null>(null);
    protected isShortRange = computed(() => {
        if (this.selectedRange().value === '24h') return true;
        const d = this.customRangeDates();
        if (this.selectedRange().value === 'custom' && d && d.length === 2 && d[0] && d[1]) {
            return d[1].getTime() - d[0].getTime() < 48 * 60 * 60 * 1000;
        }
        return false;
    });

    protected eventNames = signal<string[]>([]);
    protected selectedEvent = signal<string | null>(null);
    protected propertyKeys = signal<string[]>([]);
    protected selectedPropertyKey = signal<string | null>(null);
    protected selectedPropertyValue = signal<string | null>(null);
    protected breakdown = signal<MetricStat[]>([]);
    protected eventSeries = signal<EventSeriesPoint[]>([]);
    protected comparisonEventSeries = signal<EventSeriesPoint[]>([]);
    protected audience = signal<EventAudience | null>(null);
    protected audienceDimFilters = signal<EventDimensionFilter[]>([]);

    protected isLoadingNames = signal(false);
    protected isLoadingKeys = signal(false);
    protected isLoadingBreakdown = signal(false);
    protected isLoadingEventSeries = signal(false);
    protected isLoadingComparisonSeries = signal(false);
    protected isLoadingAudience = signal(false);

    private comparisonRange = signal<{ from: string; to: string } | null>(null);

    protected readonly eventSeriesChart = computed<SeriesChartPoint[]>(() => this.eventSeries().map((p) => ({ time: p.time, count: p.count })));
    protected readonly comparisonEventSeriesChart = computed<SeriesChartPoint[]>(() => this.comparisonEventSeries().map((p) => ({ time: p.time, count: p.count })));
    protected readonly totalEventCount = computed(() => this.eventSeries().reduce((sum, p) => sum + p.count, 0));
    protected readonly comparisonTotalCount = computed(() => this.comparisonEventSeries().reduce((sum, p) => sum + p.count, 0));
    protected readonly totalEventDelta = computed(() => this.calcDelta(this.totalEventCount(), this.comparisonTotalCount()));
    protected readonly isLoading = computed(() => this.isLoadingNames() || this.isLoadingEventSeries() || this.isLoadingComparisonSeries());

    protected readonly activeSite = computed(() => this.siteService.activeSite());
    protected readonly noSite = computed(() => !this.activeSite());
    protected eventOptions = computed<EventOption[]>(() =>
        this.eventNames().map((name) => {
            const meta = AUTOMATIC_EVENT_META[name];
            return {
                label: name,
                value: name,
                isAutomatic: !!meta,
                icon: meta?.icon ?? ''
            };
        })
    );
    protected propertyOptions = computed(() => this.propertyKeys().map((key) => ({ label: key, value: key })));

    protected comparisonLabel = computed(() => {
        this.activeLanguage();
        const r = this.comparisonRange();
        if (!r) return '';
        const showYear = new Date(r.from).getFullYear() !== new Date().getFullYear();
        const opts = showYear ? ({ month: 'short', day: 'numeric', year: 'numeric' } as const) : ({ month: 'short', day: 'numeric' } as const);
        const fmt = (d: string) => this.localeService.localizeDate(new Date(d), undefined, opts);
        return `${fmt(r.from)} – ${fmt(r.to)}`;
    });

    protected readonly eventSeriesConfig = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        return [
            {
                key: 'count',
                label: this.transloco.translate('events.kpis.totalEvents'),
                color: '#6366f1',
                gradientFrom: 'rgba(99, 102, 241, 0.5)',
                gradientTo: 'rgba(99, 102, 241, 0.0)'
            }
        ];
    });

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        const site = this.siteService.activeSite();
        if (!site) return [{ label: this.transloco.translate('events.title'), isCurrent: true }];
        return [
            { label: site.domain, favicon: site, routerLink: '/dashboard' },
            { label: this.transloco.translate('events.title'), isCurrent: true }
        ];
    });

    // Unified filter chips — property value + audience dimension filter
    protected readonly filterChips = computed<EventFilterChip[]>(() => {
        this.activeLanguage();
        const chips: EventFilterChip[] = [];
        const propKey = this.selectedPropertyKey();
        const propValue = this.selectedPropertyValue();
        if (propKey && propValue) {
            chips.push({ key: `property:${propKey}:${propValue}`, label: `${propKey}: ${propValue}`, remove: () => this.selectedPropertyValue.set(null) });
        }
        for (const dim of this.audienceDimFilters()) {
            chips.push({
                key: `dimension:${dim.type}:${dim.value}`,
                label: this.dimFilterLabel(dim.type, dim.value),
                remove: () => this.removeDimensionFilter(dim.type, dim.value)
            });
        }
        return chips;
    });

    protected readonly hasActiveFilters = computed(() => this.filterChips().length > 0);

    protected readonly totalEventCountDisplay = computed(() => this.totalEventCount().toLocaleString());

    protected readonly totalEventDeltaClass = computed(() => {
        const d = this.totalEventDelta();
        if (d === null) return '';
        return d >= 0
            ? 'text-xs font-medium px-1.5 py-0.5 rounded-full bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
            : 'text-xs font-medium px-1.5 py-0.5 rounded-full bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400';
    });

    protected readonly totalEventDeltaLabel = computed(() => {
        const d = this.totalEventDelta();
        if (d === null) return '';
        return `${d >= 0 ? '+' : ''}${d.toFixed(1)}%`;
    });
    protected readonly metricCardTabs = computed<MetricCardGroupTab<EventMetricFilterType>[]>(() => {
        this.activeLanguage();
        const audience = this.audience();
        const loading = this.isLoadingAudience();
        const contentCards: MetricCardConfig<EventMetricFilterType>[] = [];
        if (this.selectedEvent() && this.selectedPropertyKey()) {
            contentCards.push({
                id: 'property-breakdown',
                title: this.transloco.translate('events.breakdown.title'),
                icon: 'pi-list',
                data: this.breakdown(),
                isLoading: this.isLoadingBreakdown(),
                isRowClickable: true,
                activeValue: this.selectedPropertyValue(),
                filterType: 'propertyValue'
            });
        }
        if (this.selectedEvent()) {
            contentCards.push({
                id: 'top-pages',
                title: this.transloco.translate('common.metrics.topPages'),
                icon: 'pi-file',
                data: audience?.top_pages ?? [],
                linkMode: 'path',
                siteDomain: this.activeSite()?.domain ?? null,
                isLoading: loading,
                isRowClickable: true,
                activeValue: this.activeDimensionFilterValue('path'),
                filterType: 'path'
            });
        }
        return [
            {
                id: 'content',
                label: this.transloco.translate('common.metricGroups.content'),
                icon: 'pi-file',
                cards: contentCards
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
                        data: audience?.top_referrers ?? [],
                        linkMode: 'url',
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeDimensionFilterValue('referrer'),
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
                        data: audience?.top_devices ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeDimensionFilterValue('device'),
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
                        data: audience?.top_countries ?? [],
                        isLoading: loading,
                        showCountryFlags: true,
                        showCountryNames: true,
                        isRowClickable: true,
                        activeValue: this.activeDimensionFilterValue('country'),
                        filterType: 'country'
                    },
                    {
                        id: 'cities',
                        title: this.transloco.translate('common.metrics.cities'),
                        icon: 'pi-map-marker',
                        data: audience?.top_cities ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeDimensionFilterValue('city'),
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
                        data: audience?.top_providers ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeDimensionFilterValue('provider'),
                        filterType: 'provider'
                    },
                    {
                        id: 'asns',
                        title: this.transloco.translate('common.metrics.asns'),
                        icon: 'pi-sitemap',
                        data: audience?.top_asns ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeDimensionFilterValue('asn'),
                        filterType: 'asn'
                    }
                ]
            }
        ];
    });

    constructor() {
        // Load event names when site/range changes; reset selection
        effect(() => {
            const site = this.activeSite();
            this.selectedRange();
            if (!site) return;
            const dates = this.getCurrentDateRange();
            if (!dates) return;
            this.loadEventNames(site.id, dates.from, dates.to);
        });

        // Load property keys when selected event or range changes; reset property selection
        effect(() => {
            const site = this.activeSite();
            const eventName = this.selectedEvent();
            this.selectedRange();
            if (!site || !eventName) {
                this.propertyKeys.set([]);
                untracked(() => {
                    this.selectedPropertyKey.set(null);
                    this.selectedPropertyValue.set(null);
                });
                return;
            }
            const dates = this.getCurrentDateRange();
            if (!dates) return;
            this.loadPropertyKeys(site.id, dates.from, dates.to, eventName);
        });

        // Reset property value filter when property key changes
        effect(() => {
            this.selectedPropertyKey();
            untracked(() => this.selectedPropertyValue.set(null));
        });

        // Reset audience dimension filter when event changes
        effect(() => {
            this.selectedEvent();
            untracked(() => this.audienceDimFilters.set([]));
        });

        // Load event timeseries (primary + comparison) when event/range/propertyFilter/dimFilter changes
        effect(() => {
            const site = this.activeSite();
            const eventName = this.selectedEvent();
            const propKey = this.selectedPropertyKey();
            const propValue = this.selectedPropertyValue();
            const dimFilters = this.audienceDimFilters();
            this.selectedRange();

            if (!site || !eventName) {
                this.eventSeries.set([]);
                this.comparisonEventSeries.set([]);
                this.comparisonRange.set(null);
                return;
            }
            const dates = this.getCurrentDateRange();
            if (!dates) return;

            const cmpRange = this.computeComparisonPeriod(dates.from, dates.to);
            this.comparisonRange.set(cmpRange);

            const filterKey = propKey && propValue ? propKey : undefined;
            const filterVal = propKey && propValue ? propValue : undefined;
            this.loadEventTimeseries(site.id, dates.from, dates.to, eventName, filterKey, filterVal, dimFilters);
            this.loadComparisonEventTimeseries(site.id, cmpRange.from, cmpRange.to, eventName, filterKey, filterVal, dimFilters);
        });

        // Load breakdown when event/propertyKey/range changes
        effect(() => {
            const site = this.activeSite();
            const eventName = this.selectedEvent();
            const propertyKey = this.selectedPropertyKey();
            this.selectedRange();
            if (!site || !eventName || !propertyKey) {
                this.breakdown.set([]);
                return;
            }
            const dates = this.getCurrentDateRange();
            if (!dates) return;
            this.loadBreakdown(site.id, dates.from, dates.to, eventName, propertyKey);
        });

        // Load audience panels when event/property filter/dimension filter/range changes
        effect(() => {
            const site = this.activeSite();
            const eventName = this.selectedEvent();
            const propKey = this.selectedPropertyKey();
            const propValue = this.selectedPropertyValue();
            const dimFilters = this.audienceDimFilters();
            this.selectedRange();

            if (!site || !eventName) {
                this.audience.set(null);
                return;
            }
            const dates = this.getCurrentDateRange();
            if (!dates) return;

            const filterKey = propKey && propValue ? propKey : undefined;
            const filterVal = propKey && propValue ? propValue : undefined;
            this.loadAudience(site.id, dates.from, dates.to, eventName, filterKey, filterVal, dimFilters);
        });
    }

    protected calcDelta(current: number, previous: number): number | null {
        if (previous === 0) return null;
        return ((current - previous) / previous) * 100;
    }

    protected clearAllFilters() {
        this.selectedPropertyValue.set(null);
        this.audienceDimFilters.set([]);
    }

    private computeComparisonPeriod(from: string, to: string): { from: string; to: string } {
        const start = new Date(from);
        const end = new Date(to);
        const duration = end.getTime() - start.getTime();
        const cmpEnd = new Date(start.getTime() - 1);
        return {
            from: new Date(cmpEnd.getTime() - duration).toISOString(),
            to: cmpEnd.toISOString()
        };
    }

    protected togglePropertyValueFilter(value: string) {
        const current = this.selectedPropertyValue();
        this.selectedPropertyValue.set(current === value ? null : value);
    }

    protected toggleAudienceDimFilter(type: EventDimensionFilterType, item: MetricStat) {
        if (!item.name) return;
        this.audienceDimFilters.update((filters) => {
            const existingIndex = filters.findIndex((filter) => filter.type === type);
            if (existingIndex >= 0) {
                const existing = filters[existingIndex];
                if (existing.value === item.name) {
                    return filters.filter((_, idx) => idx !== existingIndex);
                }
                const next = [...filters];
                next[existingIndex] = { type, value: item.name };
                return next;
            }
            return [...filters, { type, value: item.name }];
        });
    }

    protected onMetricCardClick(event: MetricCardGroupRowClick): void {
        if (event.filterType === 'propertyValue') {
            this.togglePropertyValueFilter(event.metric.name);
            return;
        }
        if (!this.isEventDimensionFilterType(event.filterType)) return;
        this.toggleAudienceDimFilter(event.filterType, event.metric);
    }

    protected activeDimensionFilterValue(type: EventDimensionFilterType): string | null {
        return this.audienceDimFilters().find((filter) => filter.type === type)?.value ?? null;
    }

    private removeDimensionFilter(type: string, value: string) {
        this.audienceDimFilters.update((filters) => filters.filter((filter) => !(filter.type === type && filter.value === value)));
    }

    private dimFilterLabel(dim: string, value: string): string {
        switch (dim) {
            case 'path':
                return this.transloco.translate('common.filters.page', { value });
            case 'referrer':
                return this.transloco.translate('common.filters.source', { value });
            case 'device':
                return this.transloco.translate('common.filters.device', { value });
            case 'country':
                return this.transloco.translate('common.filters.country', { value });
            case 'city':
                return this.transloco.translate('common.filters.city', { value });
            case 'provider':
                return this.transloco.translate('common.filters.provider', { value });
            case 'asn':
                return this.transloco.translate('common.filters.asn', { value });
            default:
                return `${dim}: ${value}`;
        }
    }

    private isEventDimensionFilterType(value: string): value is EventDimensionFilterType {
        return value === 'path' || value === 'referrer' || value === 'device' || value === 'country' || value === 'city' || value === 'provider' || value === 'asn';
    }

    protected getCurrentDateRange(): { from: string; to: string } | null {
        const range = this.selectedRange();
        const end = new Date();
        const start = new Date();

        if (range.value === 'custom') {
            const d = this.customRangeDates();
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
            case '1y':
                start.setFullYear(end.getFullYear() - 1);
                break;
            default:
                start.setDate(end.getDate() - 30);
        }
        return { from: start.toISOString(), to: end.toISOString() };
    }

    private loadEventNames(siteId: string, from: string, to: string) {
        this.isLoadingNames.set(true);
        const currentSelection = this.selectedEvent();
        this.analyticsService.getEventNames(siteId, from, to).subscribe({
            next: (names) => {
                const mergedNames = this.mergeAutomaticEventNames(names);
                this.eventNames.set(mergedNames);
                this.selectedEvent.set(currentSelection && mergedNames.includes(currentSelection) ? currentSelection : null);
                this.isLoadingNames.set(false);
            },
            error: () => this.isLoadingNames.set(false)
        });
    }

    private mergeAutomaticEventNames(names: string[]): string[] {
        const automaticNames = AUTOMATIC_EVENT_NAMES.filter((name) => !names.includes(name));
        return [...automaticNames, ...names];
    }

    private loadPropertyKeys(siteId: string, from: string, to: string, eventName: string) {
        this.isLoadingKeys.set(true);
        const currentPropertyKey = this.selectedPropertyKey();
        this.analyticsService.getEventPropertyKeys(siteId, from, to, eventName).subscribe({
            next: (keys) => {
                this.propertyKeys.set(keys);
                this.selectedPropertyKey.set(currentPropertyKey && keys.includes(currentPropertyKey) ? currentPropertyKey : null);
                this.isLoadingKeys.set(false);
            },
            error: () => this.isLoadingKeys.set(false)
        });
    }

    private loadBreakdown(siteId: string, from: string, to: string, eventName: string, propertyKey: string) {
        this.isLoadingBreakdown.set(true);
        this.analyticsService.getEventPropertyBreakdown(siteId, from, to, eventName, propertyKey).subscribe({
            next: (data) => {
                this.breakdown.set(data);
                this.isLoadingBreakdown.set(false);
            },
            error: () => this.isLoadingBreakdown.set(false)
        });
    }

    private loadEventTimeseries(siteId: string, from: string, to: string, eventName: string, propertyKey?: string, propertyValue?: string, filters: EventDimensionFilter[] = []) {
        this.isLoadingEventSeries.set(true);
        this.analyticsService
            .getEventTimeseries(siteId, from, to, eventName, propertyKey, propertyValue, filters)
            .pipe(finalize(() => this.isLoadingEventSeries.set(false)))
            .subscribe({
                next: (data) => this.eventSeries.set(data ?? []),
                error: () => this.eventSeries.set([])
            });
    }

    private loadComparisonEventTimeseries(siteId: string, from: string, to: string, eventName: string, propertyKey?: string, propertyValue?: string, filters: EventDimensionFilter[] = []) {
        this.isLoadingComparisonSeries.set(true);
        this.analyticsService
            .getEventTimeseries(siteId, from, to, eventName, propertyKey, propertyValue, filters)
            .pipe(finalize(() => this.isLoadingComparisonSeries.set(false)))
            .subscribe({
                next: (data) => this.comparisonEventSeries.set(data ?? []),
                error: () => this.comparisonEventSeries.set([])
            });
    }

    private loadAudience(siteId: string, from: string, to: string, eventName: string, propertyKey?: string, propertyValue?: string, filters: EventDimensionFilter[] = []) {
        this.isLoadingAudience.set(true);
        this.analyticsService
            .getEventAudience(siteId, from, to, eventName, propertyKey, propertyValue, filters)
            .pipe(finalize(() => this.isLoadingAudience.set(false)))
            .subscribe({
                next: (data) => this.audience.set(data),
                error: () => this.audience.set(null)
            });
    }
}
