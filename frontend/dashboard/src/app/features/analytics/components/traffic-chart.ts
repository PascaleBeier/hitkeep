import { Component, input, output, computed, inject, ChangeDetectionStrategy } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';

import { ButtonModule } from '@openng/optimus-ui/button';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import type { HitkeepChartDesign } from '@core/charts/hitkeep-chart-options';
import { ChartDataPoint } from '@models/analytics.types';
import { SeriesChart, SeriesChartPoint, SeriesDefinition } from './series-chart';

/** The dashboard traffic chart: a SeriesChart preconfigured with pageviews, visitors, and a visitor trend line. */
@Component({
    selector: 'app-traffic-chart',
    standalone: true,
    imports: [ButtonModule, SeriesChart, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <app-series-chart
            [data]="chartPoints()"
            [series]="seriesDefinitions()"
            [comparisonData]="comparisonPoints()"
            [isLoading]="isLoading()"
            [isShortRange]="isShortRange()"
            [comparisonLabel]="comparisonLabel()"
            [design]="design()"
            [showDesignSelector]="showDesignSelector()"
            ariaLabelKey="dashboard.trafficChartAria"
            [emptyTitle]="'dashboard.empty.noTrafficTitle' | transloco"
            [emptyDescription]="'dashboard.empty.noTrafficDescription' | transloco"
        >
            <p-button chart-empty-actions [label]="'dashboard.empty.getTrackingCode' | transloco" icon="pi pi-code" size="small" (onClick)="snippetClicked.emit()" />
        </app-series-chart>
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

    private transloco = inject(TranslocoService);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    protected readonly chartPoints = computed<SeriesChartPoint[]>(() => {
        const raw = this.data() || [];
        const trend = linearTrendLine(raw.map((d) => d.visitors));
        return raw.map((d, index) => ({ time: d.time, pageviews: d.pageviews, visitors: d.visitors, 'visitors-trend': trend[index] ?? 0 }));
    });

    protected readonly comparisonPoints = computed<SeriesChartPoint[]>(() => (this.comparisonData() || []).map((d) => ({ time: d.time, pageviews: d.pageviews, visitors: d.visitors })));

    protected readonly seriesDefinitions = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        return [
            {
                key: 'pageviews',
                label: this.transloco.translate('dashboard.kpis.pageviews'),
                color: '#6366f1',
                gradientFrom: 'rgba(99, 102, 241, 0.5)',
                gradientTo: 'rgba(99, 102, 241, 0)',
                comparisonLabel: this.transloco.translate('comparison.pageviewsLabel')
            },
            {
                key: 'visitors',
                label: this.transloco.translate('dashboard.traffic.visitors'),
                color: '#14b8a6',
                gradientFrom: 'rgba(20, 184, 166, 0.5)',
                gradientTo: 'rgba(20, 184, 166, 0)',
                comparisonLabel: this.transloco.translate('comparison.visitorsLabel')
            },
            {
                key: 'visitors-trend',
                label: this.transloco.translate('dashboard.traffic.trendLine'),
                color: '#0ea5b7',
                dashed: true,
                smooth: false,
                includeInComparison: false
            }
        ];
    });
}

function linearTrendLine(values: number[]): number[] {
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
