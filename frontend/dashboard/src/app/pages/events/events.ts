import { ChangeDetectionStrategy, Component, computed, effect, inject, linkedSignal, signal, untracked } from '@angular/core';
import { FormsModule, ReactiveFormsModule } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { SelectModule } from 'primeng/select';
import { CardModule } from 'primeng/card';
import { SkeletonModule } from 'primeng/skeleton';
import { ButtonModule } from 'primeng/button';
import { SiteService } from '@features/sites/services/site.service';
import { AnalyticsService } from '@core/services/analytics.service';
import { MetricList } from '@features/analytics/components/metric-list';
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
    remove: 'property' | 'dimension';
}

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
    imports: [FormsModule, ReactiveFormsModule, TranslocoPipe, SelectModule, CardModule, SkeletonModule, ButtonModule, MetricList, RangeToolbar, PageHeader, PageHeaderLeft, PageBreadcrumb, SeriesChart],
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
    protected audienceDimFilter = signal<{ dim: string; value: string } | null>(null);

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
            chips.push({ key: 'property', label: `${propKey}: ${propValue}`, remove: 'property' });
        }
        const dim = this.audienceDimFilter();
        if (dim) {
            chips.push({ key: 'dimension', label: this.dimFilterLabel(dim.dim, dim.value), remove: 'dimension' });
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
            untracked(() => this.audienceDimFilter.set(null));
        });

        // Load event timeseries (primary + comparison) when event/range/propertyFilter/dimFilter changes
        effect(() => {
            const site = this.activeSite();
            const eventName = this.selectedEvent();
            const propKey = this.selectedPropertyKey();
            const propValue = this.selectedPropertyValue();
            const dimFilter = this.audienceDimFilter();
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
            const dimKey = dimFilter?.dim;
            const dimVal = dimFilter?.value;
            this.loadEventTimeseries(site.id, dates.from, dates.to, eventName, filterKey, filterVal, dimKey, dimVal);
            this.loadComparisonEventTimeseries(site.id, cmpRange.from, cmpRange.to, eventName, filterKey, filterVal, dimKey, dimVal);
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
            const dimFilter = this.audienceDimFilter();
            this.selectedRange();

            if (!site || !eventName) {
                this.audience.set(null);
                return;
            }
            const dates = this.getCurrentDateRange();
            if (!dates) return;

            const filterKey = propKey && propValue ? propKey : undefined;
            const filterVal = propKey && propValue ? propValue : undefined;
            this.loadAudience(site.id, dates.from, dates.to, eventName, filterKey, filterVal, dimFilter?.dim, dimFilter?.value);
        });
    }

    protected calcDelta(current: number, previous: number): number | null {
        if (previous === 0) return null;
        return ((current - previous) / previous) * 100;
    }

    protected removeFilter(type: 'property' | 'dimension') {
        if (type === 'property') this.selectedPropertyValue.set(null);
        if (type === 'dimension') this.audienceDimFilter.set(null);
    }

    protected clearAllFilters() {
        this.selectedPropertyValue.set(null);
        this.audienceDimFilter.set(null);
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

    protected toggleAudienceDimFilter(dim: string, item: MetricStat) {
        const current = this.audienceDimFilter();
        if (current?.dim === dim && current?.value === item.name) {
            this.audienceDimFilter.set(null);
        } else {
            this.audienceDimFilter.set({ dim, value: item.name });
        }
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
            default:
                return `${dim}: ${value}`;
        }
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

    private loadEventTimeseries(siteId: string, from: string, to: string, eventName: string, propertyKey?: string, propertyValue?: string, dimensionKey?: string, dimensionValue?: string) {
        this.isLoadingEventSeries.set(true);
        this.analyticsService
            .getEventTimeseries(siteId, from, to, eventName, propertyKey, propertyValue, dimensionKey, dimensionValue)
            .pipe(finalize(() => this.isLoadingEventSeries.set(false)))
            .subscribe({
                next: (data) => this.eventSeries.set(data ?? []),
                error: () => this.eventSeries.set([])
            });
    }

    private loadComparisonEventTimeseries(siteId: string, from: string, to: string, eventName: string, propertyKey?: string, propertyValue?: string, dimensionKey?: string, dimensionValue?: string) {
        this.isLoadingComparisonSeries.set(true);
        this.analyticsService
            .getEventTimeseries(siteId, from, to, eventName, propertyKey, propertyValue, dimensionKey, dimensionValue)
            .pipe(finalize(() => this.isLoadingComparisonSeries.set(false)))
            .subscribe({
                next: (data) => this.comparisonEventSeries.set(data ?? []),
                error: () => this.comparisonEventSeries.set([])
            });
    }

    private loadAudience(siteId: string, from: string, to: string, eventName: string, propertyKey?: string, propertyValue?: string, dimensionKey?: string, dimensionValue?: string) {
        this.isLoadingAudience.set(true);
        this.analyticsService
            .getEventAudience(siteId, from, to, eventName, propertyKey, propertyValue, dimensionKey, dimensionValue)
            .pipe(finalize(() => this.isLoadingAudience.set(false)))
            .subscribe({
                next: (data) => this.audience.set(data),
                error: () => this.audience.set(null)
            });
    }
}
