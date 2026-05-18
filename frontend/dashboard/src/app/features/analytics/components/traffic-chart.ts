import { Component, input, output, computed, inject, ChangeDetectionStrategy } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';

import { ChartModule } from 'primeng/chart';
import { ButtonModule } from 'primeng/button';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { ChartDataPoint } from '@models/analytics.types';
import { PreferencesService } from '@services/preferences.service';

interface ChartContext {
    chart: {
        ctx: CanvasRenderingContext2D;
    };
}

@Component({
    selector: 'app-traffic-chart',
    standalone: true,
    imports: [ChartModule, ButtonModule, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <div class="h-80 w-full relative" role="img" [attr.aria-label]="accessibilityLabel()">
            @if (isLoading()) {
                <div class="flex items-center justify-center h-full" aria-live="polite">
                    <i class="pi pi-spin pi-spinner text-4xl text-[var(--p-primary-color)]" aria-hidden="true"></i>
                </div>
            } @else if (hasTraffic()) {
                <p-chart type="line" [data]="chartPayload()" [options]="chartOptions()" height="100%" />
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

    snippetClicked = output<void>();

    private prefs = inject(PreferencesService);
    private localeService = inject(TranslocoLocaleService);
    private transloco = inject(TranslocoService);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

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

    protected chartPayload = computed(() => {
        this.activeLanguage();
        const raw = this.data() || [];
        const cmp = this.comparisonData() || [];
        const pageviews = raw.map((d) => d.pageviews);
        const visitors = raw.map((d) => d.visitors);
        const trend = this.linearTrendLine(visitors);

        const labels = raw.map((d) => {
            const date = new Date(d.time);
            if (this.isShortRange()) return this.localeService.localizeDate(date, undefined, { hour: 'numeric', minute: '2-digit' });
            return this.localeService.localizeDate(date, undefined, { month: 'short', day: 'numeric' });
        });

        const datasets: object[] = [
            {
                label: this.transloco.translate('dashboard.kpis.pageviews'),
                data: pageviews,
                fill: true,
                backgroundColor: (ctx: ChartContext) => this.getGradient(ctx, 'rgba(99, 102, 241, 0.5)', 'rgba(99, 102, 241, 0.0)'),
                borderColor: '#6366f1',
                pointBackgroundColor: '#6366f1',
                tension: 0.4,
                borderWidth: 2,
                pointRadius: 0,
                pointHoverRadius: 4
            },
            {
                label: this.transloco.translate('dashboard.traffic.visitors'),
                data: visitors,
                fill: true,
                backgroundColor: (ctx: ChartContext) => this.getGradient(ctx, 'rgba(20, 184, 166, 0.5)', 'rgba(20, 184, 166, 0.0)'),
                borderColor: '#14b8a6',
                pointBackgroundColor: '#14b8a6',
                tension: 0.4,
                borderWidth: 2,
                pointRadius: 0,
                pointHoverRadius: 4
            },
            {
                label: this.transloco.translate('dashboard.traffic.trendLine'),
                data: trend,
                fill: false,
                borderColor: '#0ea5b7',
                pointBackgroundColor: '#0ea5b7',
                tension: 0,
                borderWidth: 2,
                pointRadius: 0,
                pointHoverRadius: 4,
                borderDash: [6, 4]
            }
        ];

        if (cmp.length > 0) {
            datasets.push(
                {
                    label: this.transloco.translate('comparison.pageviewsLabel'),
                    data: cmp.map((d) => d.pageviews),
                    fill: false,
                    borderColor: 'rgba(99, 102, 241, 0.4)',
                    pointBackgroundColor: 'rgba(99, 102, 241, 0.4)',
                    tension: 0.4,
                    borderWidth: 1.5,
                    pointRadius: 0,
                    pointHoverRadius: 3,
                    borderDash: [5, 5]
                },
                {
                    label: this.transloco.translate('comparison.visitorsLabel'),
                    data: cmp.map((d) => d.visitors),
                    fill: false,
                    borderColor: 'rgba(20, 184, 166, 0.4)',
                    pointBackgroundColor: 'rgba(20, 184, 166, 0.4)',
                    tension: 0.4,
                    borderWidth: 1.5,
                    pointRadius: 0,
                    pointHoverRadius: 3,
                    borderDash: [5, 5]
                }
            );
        }

        return { labels, datasets };
    });

    protected chartOptions = computed(() => {
        const isDark = this.prefs.isDarkMode();
        const textColor = isDark ? '#94a3b8' : '#64748b';
        const gridColor = isDark ? 'rgba(255, 255, 255, 0.05)' : 'rgba(0, 0, 0, 0.05)';
        const tooltipBg = isDark ? 'rgba(15, 23, 42, 0.9)' : 'rgba(255, 255, 255, 0.9)';
        const tooltipText = isDark ? '#f8fafc' : '#0f172a';
        const tooltipBorder = isDark ? '#334155' : '#e2e8f0';

        return {
            maintainAspectRatio: false,
            aspectRatio: 0.5,
            responsive: true,
            interaction: { mode: 'index', intersect: false },
            plugins: {
                legend: { labels: { color: textColor, usePointStyle: true, boxWidth: 8 }, position: 'bottom' },
                tooltip: {
                    mode: 'index',
                    intersect: false,
                    backgroundColor: tooltipBg,
                    titleColor: tooltipText,
                    bodyColor: tooltipText,
                    borderColor: tooltipBorder,
                    borderWidth: 1,
                    padding: 10,
                    cornerRadius: 8,
                    displayColors: true
                }
            },
            scales: {
                x: {
                    ticks: { color: textColor, maxTicksLimit: 8 },
                    grid: { color: gridColor, drawBorder: false, tickLength: 0 },
                    border: { display: false }
                },
                y: {
                    ticks: { color: textColor, maxTicksLimit: 6, precision: 0 },
                    grid: { color: gridColor, drawBorder: false, tickLength: 0 },
                    border: { display: false },
                    beginAtZero: true
                }
            }
        };
    });

    private getGradient(context: ChartContext, c1: string, c2: string) {
        const ctx = context.chart.ctx;
        if (!ctx) return c1;
        const gradient = ctx.createLinearGradient(0, 0, 0, 300);
        gradient.addColorStop(0, c1);
        gradient.addColorStop(1, c2);
        return gradient;
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
