import { ChangeDetectionStrategy, Component, computed, input, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { toSignal } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { NgxEchartsDirective } from 'ngx-echarts';
import { SelectModule } from 'primeng/select';
import type { EChartsCoreOption, EChartsInitOpts } from 'echarts/core';
import { buildHitkeepChartMergeOptions, buildHitkeepChartOptions, hitkeepChartTheme, withChartAlpha, type HitkeepChartDesign, type HitkeepChartSeries } from '@core/charts/hitkeep-chart-options';
import { provideHitkeepEcharts } from '@core/charts/hitkeep-echarts.provider';
import { PreferencesService } from '@services/preferences.service';

export type SeriesChartPoint = Record<string, number | string> & { time: string };

export interface SeriesDefinition {
    key: string;
    label: string;
    color: string;
    gradientFrom: string;
    gradientTo: string;
    design?: HitkeepChartDesign;
}

interface ChartDesignOption {
    label: string;
    value: HitkeepChartDesign;
}

let seriesChartId = 0;

@Component({
    selector: 'app-series-chart',
    changeDetection: ChangeDetectionStrategy.OnPush,
    imports: [FormsModule, NgxEchartsDirective, SelectModule, TranslocoPipe],
    providers: [provideHitkeepEcharts()],
    template: `
        <div class="h-80 w-full relative" role="img" [attr.aria-label]="accessibilityLabel()">
            @if (isLoading()) {
                <div class="flex items-center justify-center h-full" aria-live="polite">
                    <i class="pi pi-spin pi-spinner text-4xl text-[var(--p-primary-color)]" aria-hidden="true"></i>
                </div>
            } @else if (hasData()) {
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
                    <h3 class="font-semibold text-[var(--p-text-color)] text-lg mb-1">{{ emptyTitle() || ('common.empty.noDataTitle' | transloco) }}</h3>
                    <p class="text-sm max-w-xs">{{ emptyDescription() || ('common.empty.noDataDescription' | transloco) }}</p>
                </div>
            }
        </div>
        @if (comparisonLabel()) {
            <p class="text-xs text-[var(--p-text-muted-color)] text-right mt-2">{{ 'comparison.vsLabel' | transloco }} {{ comparisonLabel() }}</p>
        }
    `
})
export class SeriesChart {
    data = input.required<SeriesChartPoint[]>();
    series = input.required<SeriesDefinition[]>();
    comparisonData = input<SeriesChartPoint[]>([]);
    isLoading = input<boolean>(false);
    isShortRange = input<boolean>(false);
    emptyTitle = input<string>('');
    emptyDescription = input<string>('');
    comparisonLabel = input<string>('');
    design = input<HitkeepChartDesign>('area');
    showDesignSelector = input<boolean>(true);

    protected readonly chartInitOptions: EChartsInitOpts = { renderer: 'canvas' };
    protected readonly chartDesignSelectId = `series-chart-design-${++seriesChartId}`;
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

    protected hasData = computed(() => {
        const data = this.data() || [];
        const series = this.series() || [];
        if (data.length === 0 || series.length === 0) return false;
        return data.some((point) => series.some((s) => Number(point[s.key] ?? 0) > 0));
    });

    protected accessibilityLabel = computed(() => {
        this.activeLanguage();
        const count = this.data()?.length || 0;
        return this.transloco.translate('common.seriesChartAria', { count });
    });

    protected chartOptions = computed(() => {
        this.activeLanguage();
        const raw = this.data() || [];
        const cmp = this.comparisonData() || [];

        return buildHitkeepChartOptions({
            ariaLabel: this.accessibilityLabel(),
            design: this.effectiveDesign(),
            labels: raw.map((point) => this.formatBucketLabel(point.time)),
            locale: this.transloco.getActiveLang(),
            series: this.chartSeries(raw, cmp),
            theme: hitkeepChartTheme(this.prefs.isDarkMode())
        });
    });

    protected chartFrameOptions = computed((): EChartsCoreOption => {
        this.activeLanguage();
        return buildHitkeepChartOptions({
            ariaLabel: this.transloco.translate('common.seriesChartAria', { count: 0 }),
            design: this.effectiveDesign(),
            labels: [],
            locale: this.transloco.getActiveLang(),
            series: this.chartSeries([], [], this.comparisonLabel().trim().length > 0),
            theme: hitkeepChartTheme(this.prefs.isDarkMode())
        });
    });

    protected chartMergeOptions = computed((): EChartsCoreOption => {
        this.activeLanguage();
        const raw = this.data() || [];
        const cmp = this.comparisonData() || [];
        return buildHitkeepChartMergeOptions({
            ariaLabel: this.accessibilityLabel(),
            design: this.effectiveDesign(),
            labels: raw.map((point) => this.formatBucketLabel(point.time)),
            locale: this.transloco.getActiveLang(),
            series: this.chartSeries(raw, cmp),
            theme: hitkeepChartTheme(this.prefs.isDarkMode())
        });
    });

    protected setSelectedDesign(value: HitkeepChartDesign | null): void {
        if (value) {
            this.selectedDesign.set(value);
        }
    }

    private chartSeries(raw: SeriesChartPoint[], cmp: SeriesChartPoint[], includeComparison = cmp.length > 0): HitkeepChartSeries[] {
        const series = this.series() || [];
        const chartSeries: HitkeepChartSeries[] = series.map((s) => ({
            id: s.key,
            label: s.label,
            data: raw.map((d) => Number(d[s.key] ?? 0)),
            color: s.color,
            gradientFrom: s.gradientFrom,
            gradientTo: s.gradientTo,
            design: s.design
        }));

        if (includeComparison) {
            for (const s of series) {
                chartSeries.push({
                    id: `${s.key}-comparison`,
                    label: `${s.label} (prev.)`,
                    data: cmp.map((d) => Number(d[s.key] ?? 0)),
                    color: withChartAlpha(s.color, 0.4),
                    muted: true,
                    dashed: true
                });
            }
        }

        return chartSeries;
    }

    private formatBucketLabel(time: string): string {
        const date = new Date(time);
        if (this.isShortRange()) return this.localeService.localizeDate(date, undefined, { hour: 'numeric', minute: '2-digit' });
        return this.localeService.localizeDate(date, undefined, { month: 'short', day: 'numeric' });
    }
}
