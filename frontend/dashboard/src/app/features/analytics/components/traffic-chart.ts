import { Component, input, output, computed, inject, ChangeDetectionStrategy, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { toSignal } from '@angular/core/rxjs-interop';

import { ButtonModule } from 'primeng/button';
import { SelectModule } from 'primeng/select';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { NgxEchartsDirective } from 'ngx-echarts';
import type { EChartsCoreOption, EChartsInitOpts } from 'echarts/core';
import { buildHitkeepChartMergeOptions, buildHitkeepChartOptions, hitkeepChartTheme, withChartAlpha, type HitkeepChartDesign, type HitkeepChartSeries } from '@core/charts/hitkeep-chart-options';
import { provideHitkeepEcharts } from '@core/charts/hitkeep-echarts.provider';
import { ChartDataPoint } from '@models/analytics.types';
import { PreferencesService } from '@services/preferences.service';

interface ChartDesignOption {
    label: string;
    value: HitkeepChartDesign;
}

let trafficChartId = 0;

@Component({
    selector: 'app-traffic-chart',
    standalone: true,
    imports: [ButtonModule, FormsModule, NgxEchartsDirective, SelectModule, TranslocoPipe],
    providers: [provideHitkeepEcharts()],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <div class="h-80 w-full relative" role="img" [attr.aria-label]="accessibilityLabel()">
            @if (isLoading()) {
                <div class="flex items-center justify-center h-full" aria-live="polite">
                    <i class="pi pi-spin pi-spinner text-4xl text-[var(--p-primary-color)]" aria-hidden="true"></i>
                </div>
            } @else if (hasTraffic()) {
                @if (showDesignSelector()) {
                    <div class="absolute right-2 top-2 z-10">
                        <label class="sr-only" [for]="chartDesignSelectId">{{ 'common.chartDesign.label' | transloco }}</label>
                        <p-select
                            [inputId]="chartDesignSelectId"
                            [options]="chartDesignOptions()"
                            [ngModel]="effectiveDesign()"
                            (ngModelChange)="setSelectedDesign($event)"
                            optionLabel="label"
                            optionValue="value"
                            size="small"
                            appendTo="body"
                            styleClass="chart-design-select"
                        />
                    </div>
                }
                <div echarts class="h-full w-full" [options]="chartFrameOptions()" [merge]="chartMergeOptions()" [initOpts]="chartInitOptions" [autoResize]="true"></div>
            } @else {
                <div class="absolute inset-0 flex flex-col items-center justify-center text-[var(--p-text-muted-color)] bg-[var(--p-surface-ground)]/50 rounded-lg border-2 border-dashed border-[var(--p-surface-border)] p-6 text-center">
                    <h3 class="font-semibold text-[var(--p-text-color)] text-lg mb-1">{{ 'dashboard.empty.noTrafficTitle' | transloco }}</h3>
                    <p class="text-sm mb-4 max-w-xs">{{ 'dashboard.empty.noTrafficDescription' | transloco }}</p>
                    <p-button [label]="'dashboard.empty.getTrackingCode' | transloco" icon="pi pi-code" size="small" (onClick)="snippetClicked.emit()"></p-button>
                </div>
            }
        </div>
        @if (comparisonLabel()) {
            <p class="text-xs text-[var(--p-text-muted-color)] text-right mt-2">{{ 'comparison.vsLabel' | transloco }} {{ comparisonLabel() }}</p>
        }
    `
})
export class TrafficChart {
    data = input.required<ChartDataPoint[]>();
    comparisonData = input<ChartDataPoint[]>([]);
    isLoading = input<boolean>(false);
    isShortRange = input<boolean>(false);
    comparisonLabel = input<string>('');
    design = input<HitkeepChartDesign>('area');
    showDesignSelector = input<boolean>(true);

    snippetClicked = output<void>();
    protected readonly chartInitOptions: EChartsInitOpts = { renderer: 'canvas' };
    protected readonly chartDesignSelectId = `traffic-chart-design-${++trafficChartId}`;

    private prefs = inject(PreferencesService);
    private localeService = inject(TranslocoLocaleService);
    private transloco = inject(TranslocoService);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });
    protected readonly selectedDesign = signal<HitkeepChartDesign | null>(null);
    protected readonly effectiveDesign = computed(() => this.selectedDesign() ?? this.design());

    protected readonly chartDesignOptions = computed<ChartDesignOption[]>(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('common.chartDesign.area'), value: 'area' },
            { label: this.transloco.translate('common.chartDesign.line'), value: 'line' },
            { label: this.transloco.translate('common.chartDesign.bar'), value: 'bar' }
        ];
    });

    protected hasTraffic = computed(() => {
        const d = this.data();
        // It has traffic if data exists AND at least one bucket has > 0 pageviews
        return d && d.length > 0 && d.some((p) => p.pageviews > 0);
    });

    protected accessibilityLabel = computed(() => {
        this.activeLanguage();
        const count = this.data()?.length || 0;
        return this.transloco.translate('dashboard.trafficChartAria', { count });
    });

    protected chartOptions = computed(() => {
        this.activeLanguage();
        const raw = this.data() || [];
        const cmp = this.comparisonData() || [];

        return buildHitkeepChartOptions({
            ariaLabel: this.accessibilityLabel(),
            design: this.effectiveDesign(),
            labels: raw.map((d) => this.formatBucketLabel(d.time)),
            locale: this.transloco.getActiveLang(),
            series: this.chartSeries(raw, cmp),
            theme: hitkeepChartTheme(this.prefs.isDarkMode()),
            yAxisTicks: 6
        });
    });

    protected chartFrameOptions = computed((): EChartsCoreOption => {
        this.activeLanguage();
        return buildHitkeepChartOptions({
            ariaLabel: this.transloco.translate('dashboard.trafficChartAria', { count: 0 }),
            design: this.effectiveDesign(),
            labels: [],
            locale: this.transloco.getActiveLang(),
            series: this.chartSeries([], [], this.comparisonLabel().trim().length > 0),
            theme: hitkeepChartTheme(this.prefs.isDarkMode()),
            yAxisTicks: 6
        });
    });

    protected chartMergeOptions = computed((): EChartsCoreOption => {
        this.activeLanguage();
        const raw = this.data() || [];
        const cmp = this.comparisonData() || [];
        return buildHitkeepChartMergeOptions({
            ariaLabel: this.accessibilityLabel(),
            design: this.effectiveDesign(),
            labels: raw.map((d) => this.formatBucketLabel(d.time)),
            locale: this.transloco.getActiveLang(),
            series: this.chartSeries(raw, cmp),
            theme: hitkeepChartTheme(this.prefs.isDarkMode()),
            yAxisTicks: 6
        });
    });

    protected setSelectedDesign(value: HitkeepChartDesign | null): void {
        if (value) {
            this.selectedDesign.set(value);
        }
    }

    private chartSeries(raw: ChartDataPoint[], cmp: ChartDataPoint[], includeComparison = cmp.length > 0): HitkeepChartSeries[] {
        const visitors = raw.map((d) => d.visitors);
        const series: HitkeepChartSeries[] = [
            {
                id: 'pageviews',
                label: this.transloco.translate('dashboard.kpis.pageviews'),
                data: raw.map((d) => d.pageviews),
                color: '#6366f1',
                gradientFrom: 'rgba(99, 102, 241, 0.5)',
                gradientTo: 'rgba(99, 102, 241, 0)'
            },
            {
                id: 'visitors',
                label: this.transloco.translate('dashboard.traffic.visitors'),
                data: visitors,
                color: '#14b8a6',
                gradientFrom: 'rgba(20, 184, 166, 0.5)',
                gradientTo: 'rgba(20, 184, 166, 0)'
            },
            {
                id: 'visitors-trend',
                label: this.transloco.translate('dashboard.traffic.trendLine'),
                data: this.linearTrendLine(visitors),
                color: '#0ea5b7',
                dashed: true,
                smooth: false
            }
        ];

        if (includeComparison) {
            series.push(
                {
                    id: 'pageviews-comparison',
                    label: this.transloco.translate('comparison.pageviewsLabel'),
                    data: cmp.map((d) => d.pageviews),
                    color: withChartAlpha('#6366f1', 0.4),
                    muted: true,
                    dashed: true
                },
                {
                    id: 'visitors-comparison',
                    label: this.transloco.translate('comparison.visitorsLabel'),
                    data: cmp.map((d) => d.visitors),
                    color: withChartAlpha('#14b8a6', 0.4),
                    muted: true,
                    dashed: true
                }
            );
        }

        return series;
    }

    private formatBucketLabel(time: string): string {
        const date = new Date(time);
        if (this.isShortRange()) return this.localeService.localizeDate(date, undefined, { hour: 'numeric', minute: '2-digit' });
        return this.localeService.localizeDate(date, undefined, { month: 'short', day: 'numeric' });
    }

    private linearTrendLine(values: number[]): number[] {
        const n = values.length;
        if (n === 0) return [];
        if (n === 1) return [values[0] ?? 0];

        let sumX = 0;
        let sumY = 0;
        let sumXY = 0;
        let sumXX = 0;
        for (let i = 0; i < n; i++) {
            const x = i + 1;
            const y = values[i] ?? 0;
            sumX += x;
            sumY += y;
            sumXY += x * y;
            sumXX += x * x;
        }

        const denominator = n * sumXX - sumX * sumX;
        const slope = denominator === 0 ? 0 : (n * sumXY - sumX * sumY) / denominator;
        const intercept = (sumY - slope * sumX) / n;

        return values.map((_, index) => Number((intercept + slope * (index + 1)).toFixed(2)));
    }
}
