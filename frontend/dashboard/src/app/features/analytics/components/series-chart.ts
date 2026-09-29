import { ChangeDetectionStrategy, Component, DestroyRef, afterRenderEffect, computed, input, inject, signal } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { NgxEchartsDirective } from 'ngx-echarts';
import type { ECharts, EChartsCoreOption, EChartsInitOpts } from 'echarts/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ChartDesignToggle } from '@components/chart-design-toggle/chart-design-toggle';
import { bindAnnotationInteractions, type AnnotationMarkerKind } from '@core/charts/annotation-interactions';
import { annotationMarkers } from '@core/charts/annotation-markers';
import {
    annotationAccentColor,
    buildHitkeepChartMergeOptions,
    buildHitkeepChartOptions,
    hitkeepChartTheme,
    resolveChartColor,
    withChartAlpha,
    type HitkeepChartAnnotations,
    type HitkeepChartDesign,
    type HitkeepChartSeries
} from '@core/charts/hitkeep-chart-options';
import { provideHitkeepEcharts } from '@core/charts/hitkeep-echarts.provider';
import { AnnotationStrip } from '@features/annotations/annotation-strip';
import { SiteAnnotationsService } from '@features/annotations/site-annotations.service';
import { ChartDesignPreferencesService } from '@services/chart-design-preferences.service';
import { PreferencesService } from '@services/preferences.service';
import { ThemeManagerService } from '@services/theme-manager.service';
import { injectSkeletonGate } from '@services/report-subject.service';

export type SeriesChartPoint = Record<string, number | string> & { time: string };

export interface SeriesDefinition {
    key: string;
    label: string;
    color: string;
    gradientFrom?: string;
    gradientTo?: string;
    design?: HitkeepChartDesign;
    dashed?: boolean;
    smooth?: boolean;
    /** Translated label for the comparison twin; falls back to "<label> (prev.)". */
    comparisonLabel?: string;
    /** Set false for derived series (e.g. trend lines) that must not get a comparison twin. */
    includeInComparison?: boolean;
}

@Component({
    selector: 'app-series-chart',
    changeDetection: ChangeDetectionStrategy.OnPush,
    imports: [AnnotationStrip, ButtonModule, ChartDesignToggle, NgxEchartsDirective, TranslocoPipe],
    providers: [provideHitkeepEcharts()],
    template: `
        @if ((showDesignSelector() || notes.canWrite()) && !showSkeleton() && hasData()) {
            <div class="mb-2 flex items-center justify-end gap-2">
                @if (notes.canWrite()) {
                    <p-button type="button" size="small" severity="secondary" icon="pi pi-bookmark" [text]="true" [label]="'annotations.add' | transloco" [ariaLabel]="'annotations.addAria' | transloco" (onClick)="addNote()" />
                }
                @if (showDesignSelector()) {
                    <app-chart-design-toggle [value]="effectiveDesign()" (valueChange)="setSelectedDesign($event)" />
                }
            </div>
        }
        <div class="h-80 w-full relative" role="img" [attr.aria-label]="accessibilityLabel()">
            @if (showSkeleton()) {
                <div class="flex items-center justify-center h-full" aria-live="polite">
                    <i class="pi pi-spin pi-spinner text-4xl text-[var(--p-primary-color)]" aria-hidden="true"></i>
                </div>
            } @else if (hasData()) {
                <div echarts class="h-full w-full" [options]="chartFrameOptions()" [initOpts]="chartInitOptions" [autoResize]="true" (chartInit)="onChartInit($event)"></div>
            } @else {
                <div class="absolute inset-0 flex flex-col items-center justify-center text-[var(--p-text-muted-color)] bg-[var(--p-surface-ground)]/50 rounded-lg border-2 border-dashed border-[var(--p-surface-border)] p-6 text-center">
                    <h3 class="font-semibold text-[var(--p-text-color)] text-lg mb-1">{{ emptyTitle() || ('common.empty.noDataTitle' | transloco) }}</h3>
                    <p class="text-sm max-w-xs">{{ emptyDescription() || ('common.empty.noDataDescription' | transloco) }}</p>
                    <div class="mt-4 empty:hidden">
                        <ng-content select="[chart-empty-actions]" />
                    </div>
                </div>
            }
        </div>
        @if (comparisonLabel()) {
            <p class="text-xs text-[var(--p-text-muted-color)] text-right mt-2">{{ 'comparison.vsLabel' | transloco }} {{ comparisonLabel() }}</p>
        }
        @if (!showSkeleton() && hasData()) {
            <app-annotation-strip [annotations]="markers().visible" [granularity]="granularity()" (selected)="notes.openExisting($event, granularity())" />
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
    ariaLabelKey = input<string>('common.seriesChartAria');

    protected readonly chartInitOptions: EChartsInitOpts = { renderer: 'canvas' };
    private readonly chartInstance = signal<ECharts | null>(null);
    private prefs = inject(PreferencesService);
    private themeManager = inject(ThemeManagerService);
    private designPrefs = inject(ChartDesignPreferencesService);
    private localeService = inject(TranslocoLocaleService);
    private transloco = inject(TranslocoService);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });
    protected readonly notes = inject(SiteAnnotationsService);
    private unbindNotes: (() => void) | null = null;

    protected readonly effectiveDesign = computed(() => this.designPrefs.design() ?? this.design());
    protected readonly granularity = computed(() => (this.isShortRange() ? 'hour' : 'day'));
    protected readonly markers = computed(() => annotationMarkers(this.data() || [], this.notes.annotations()));

    private readonly chartAnnotations = computed((): HitkeepChartAnnotations => {
        this.activeLanguage();
        const { lines, areas } = this.markers();
        return {
            lines: lines.map(({ index, annotations }) => ({
                index,
                label: annotations.length > 1 ? this.transloco.translate('annotations.chart.grouped', { count: annotations.length, body: annotations[0].body }) : annotations[0].body
            })),
            areas: areas.map(({ start, end, annotation }) => ({ start, end, label: annotation.body }))
        };
    });

    protected hasData = computed(() => {
        const data = this.data() || [];
        const series = this.series() || [];
        if (data.length === 0 || series.length === 0) return false;
        return data.some((point) => series.some((s) => Number(point[s.key] ?? 0) > 0));
    });

    protected readonly showSkeleton = injectSkeletonGate(this.isLoading, this.hasData);

    protected accessibilityLabel = computed(() => {
        this.activeLanguage();
        const count = this.data()?.length || 0;
        const label = this.transloco.translate(this.ariaLabelKey(), { count });
        const notes = this.markers().visible.length;
        return notes > 0 ? `${label} ${this.transloco.translate('annotations.chart.aria', { count: notes })}` : label;
    });

    protected chartFrameOptions = computed((): EChartsCoreOption => {
        this.activeLanguage();
        this.themeManager.version();
        return buildHitkeepChartOptions({
            ariaLabel: this.transloco.translate(this.ariaLabelKey(), { count: 0 }),
            design: this.effectiveDesign(),
            labels: [],
            locale: this.transloco.getActiveLang(),
            series: this.chartSeries([], [], this.comparisonLabel().trim().length > 0),
            theme: hitkeepChartTheme(this.prefs.isDarkMode())
        });
    });

    protected chartMergeOptions = computed((): EChartsCoreOption => {
        this.activeLanguage();
        this.themeManager.version();
        const raw = this.data() || [];
        const cmp = this.comparisonData() || [];
        const bucketLabel = this.bucketLabelFormatter();
        return buildHitkeepChartMergeOptions({
            ariaLabel: this.accessibilityLabel(),
            design: this.effectiveDesign(),
            labels: raw.map((point) => bucketLabel.format(new Date(point.time))),
            locale: this.transloco.getActiveLang(),
            series: this.chartSeries(raw, cmp),
            theme: hitkeepChartTheme(this.prefs.isDarkMode())
        });
    });

    constructor() {
        this.notes.load();
        inject(DestroyRef).onDestroy(() => this.unbindNotes?.());

        // ngx-echarts merges without `replaceMerge`, which only stayed invisible
        // while every reload tore the chart down. Now that a range switch keeps
        // the instance alive, a series the new data dropped — a comparison twin,
        // a conditional metric — would otherwise linger on screen forever.
        //
        // This runs after render on purpose: a theme, language or design change
        // makes the directive re-apply the frame with `notMerge`, and the data
        // patch has to land after that reset rather than before it.
        afterRenderEffect(() => {
            // Bail before reading the options: with no live chart there is
            // nothing to patch, and building them would relabel every bucket
            // for a spinner or an empty state. A new instance re-runs this.
            const chart = this.chartInstance();
            if (!chart || chart.isDisposed()) return;
            chart.setOption(this.chartMergeOptions(), { replaceMerge: ['series'] });
        });
    }

    protected onChartInit(chart: ECharts): void {
        this.chartInstance.set(chart);
        this.unbindNotes?.();
        this.unbindNotes = bindAnnotationInteractions(chart, {
            canWrite: () => this.notes.canWrite(),
            bucketCount: () => this.data()?.length ?? 0,
            isBar: () => this.effectiveDesign() === 'bar',
            previewColor: annotationAccentColor,
            onAdd: (start, end) => this.openNewNote(start, end),
            onOpen: (kind, index) => this.openMarker(kind, index)
        });
    }

    /** The toolbar button: a point on the newest bucket, which the reader can move in the dialog. */
    protected addNote(): void {
        const last = (this.data()?.length ?? 0) - 1;
        if (last >= 0) {
            this.openNewNote(last, null);
        }
    }

    private openNewNote(start: number, end: number | null): void {
        const data = this.data() ?? [];
        const startPoint = data[start];
        if (!startPoint) {
            return;
        }
        const endPoint = end === null ? undefined : data[end];
        this.notes.open({ startsAt: new Date(startPoint.time), endsAt: endPoint ? new Date(endPoint.time) : null, body: '', granularity: this.granularity() });
    }

    private openMarker(kind: AnnotationMarkerKind, index: number): void {
        const markers = this.markers();
        const annotation = kind === 'line' ? markers.lines[index]?.annotations[0] : markers.areas[index]?.annotation;
        if (annotation) {
            this.notes.openExisting(annotation, this.granularity());
        }
    }

    protected setSelectedDesign(value: HitkeepChartDesign): void {
        this.designPrefs.setDesign(value);
    }

    private chartSeries(raw: SeriesChartPoint[], cmp: SeriesChartPoint[], includeComparison = cmp.length > 0): HitkeepChartSeries[] {
        const series = this.series() || [];
        const chartSeries: HitkeepChartSeries[] = series.map((s) => ({
            id: s.key,
            label: s.label,
            data: raw.map((d) => Number(d[s.key] ?? 0)),
            color: resolveChartColor(s.color),
            gradientFrom: s.gradientFrom,
            gradientTo: s.gradientTo,
            design: s.design,
            dashed: s.dashed,
            smooth: s.smooth
        }));
        // Notes explain the current period, so they ride on its first series only.
        if (raw.length > 0 && chartSeries[0]) {
            chartSeries[0].annotations = this.chartAnnotations();
        }

        if (includeComparison) {
            for (const s of series) {
                if (s.includeInComparison === false) {
                    continue;
                }
                chartSeries.push({
                    id: `${s.key}-comparison`,
                    label: s.comparisonLabel ?? `${s.label} (prev.)`,
                    data: cmp.map((d) => Number(d[s.key] ?? 0)),
                    color: withChartAlpha(s.color, 0.4),
                    muted: true,
                    dashed: true
                });
            }
        }

        return chartSeries;
    }

    /**
     * One formatter for the whole axis. A year of daily buckets would otherwise
     * construct a few hundred `Intl.DateTimeFormat`s per rebuild, which is the
     * most expensive part of the range switch this chart animates through.
     */
    private bucketLabelFormatter(): Intl.DateTimeFormat {
        const options: Intl.DateTimeFormatOptions = this.isShortRange() ? { hour: 'numeric', minute: '2-digit' } : { month: 'short', day: 'numeric' };
        return new Intl.DateTimeFormat(this.localeService.getLocale(), options);
    }
}
