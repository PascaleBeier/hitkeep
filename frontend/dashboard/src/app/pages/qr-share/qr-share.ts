import { ChangeDetectionStrategy, Component, computed, DestroyRef, effect, inject, linkedSignal, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute } from '@angular/router';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { TagModule } from '@openng/optimus-ui/tag';
import { MenuItem } from '@openng/optimus-ui/api';
import { buildTakeoutExportMenuItems, TakeoutExportFormat } from '@core/export/export-formats';
import { CopyControl } from '@components/copy-control/copy-control';
import { DEFAULT_RANGE_OPTIONS, RangeOption, RangeToolbar, resolveDateRange, selectDefaultRange } from '@components/range-toolbar/range-toolbar';
import { KpiCard } from '@features/analytics/components/kpi-card';
import { MetricCardGroup, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import { SeriesChart, SeriesChartPoint, SeriesDefinition } from '@features/analytics/components/series-chart';
import { QRCodePreview } from '@features/qr/qr-code-preview';
import { QRCode, QRCodeSummary } from '@models/analytics.types';
import { QRCodesService, buildQRCodeDestination } from '@services/qr-codes.service';

@Component({
    selector: 'app-qr-share-page',
    standalone: true,
    imports: [TranslocoPipe, ButtonModule, SplitButtonModule, TagModule, CopyControl, RangeToolbar, KpiCard, MetricCardGroup, SeriesChart, QRCodePreview],
    templateUrl: './qr-share.html',
    styleUrl: './qr-share.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class QRSharePage {
    private readonly route = inject(ActivatedRoute);
    private readonly service = inject(QRCodesService);
    private readonly transloco = inject(TranslocoService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly token = this.route.snapshot.paramMap.get('token') ?? '';

    protected readonly qr = signal<QRCode | null>(null);
    protected readonly summary = signal<QRCodeSummary | null>(null);
    protected readonly series = signal<SeriesChartPoint[]>([]);
    protected readonly loading = signal(true);
    protected readonly statsLoading = signal(false);
    protected readonly errorKey = signal<string | null>(null);

    protected readonly timeRanges = signal<RangeOption[]>(DEFAULT_RANGE_OPTIONS);
    protected readonly selectedRange = linkedSignal<RangeOption[], RangeOption>({
        source: this.timeRanges,
        computation: (ranges, previous) => selectDefaultRange(ranges, previous?.value)
    });
    protected readonly customRangeDates = signal<Date[] | null>(null);
    protected readonly assetURL = computed(() => (this.qr()?.has_asset ? this.service.qrShareAssetURL(this.token) : null));
    protected readonly finalDestination = computed(() => {
        const qr = this.qr();
        if (!qr) return '';
        return buildQRCodeDestination(
            {
                destination_url: qr.destination_url,
                utm_source: qr.utm_source ?? '',
                utm_medium: qr.utm_medium ?? '',
                utm_campaign: qr.utm_campaign ?? '',
                utm_term: qr.utm_term ?? '',
                utm_content: qr.utm_content ?? '',
                custom_params: qr.custom_params ?? {}
            },
            qr.id
        );
    });

    protected readonly kpis = computed(() => {
        const summary = this.summary();
        const loading = this.statsLoading();
        return [
            { label: this.transloco.translate('qrCodes.kpis.opens'), value: summary?.open_count ?? 0, loading },
            { label: this.transloco.translate('dashboard.kpis.pageviews'), value: summary?.pageviews ?? 0, loading },
            { label: this.transloco.translate('dashboard.traffic.visitors'), value: summary?.visitors ?? 0, loading }
        ];
    });
    protected readonly seriesConfig = computed<SeriesDefinition[]>(() => [
        {
            key: 'opens',
            label: this.transloco.translate('qrCodes.kpis.opens'),
            color: '#2563eb',
            gradientFrom: 'rgba(37, 99, 235, 0.22)',
            gradientTo: 'rgba(37, 99, 235, 0.02)'
        }
    ]);
    protected readonly metricTabs = computed<MetricCardGroupTab[]>(() => {
        const summary = this.summary();
        const loading = this.statsLoading();
        return [
            {
                id: 'qr',
                label: this.transloco.translate('qrCodes.analytics.fullAnalytics'),
                icon: 'pi-chart-line',
                cards: [
                    { id: 'pages', title: this.transloco.translate('common.metrics.topPages'), icon: 'pi-file', data: summary?.top_pages ?? [], isLoading: loading },
                    { id: 'referrers', title: this.transloco.translate('common.metrics.topReferrers'), icon: 'pi-link', data: summary?.top_referrers ?? [], isLoading: loading },
                    { id: 'devices', title: this.transloco.translate('common.metrics.devices'), icon: 'pi-desktop', data: summary?.top_devices ?? [], isLoading: loading },
                    { id: 'countries', title: this.transloco.translate('common.metrics.countries'), icon: 'pi-globe', data: summary?.top_countries ?? [], isLoading: loading, showCountryFlags: true, showCountryNames: true }
                ]
            }
        ];
    });

    constructor() {
        if (!this.token) {
            this.errorKey.set('qrCodes.share.invalid');
            this.loading.set(false);
            return;
        }

        this.service
            .getQRShare(this.token)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (qr) => this.qr.set(qr),
                error: () => this.errorKey.set('qrCodes.share.invalid'),
                complete: () => this.loading.set(false)
            });

        effect(() => {
            const qr = this.qr();
            const range = this.currentDateRange();
            if (!qr || !range) return;
            this.loadStats(range.from, range.to);
        });
    }

    protected refresh(): void {
        const range = this.currentDateRange();
        if (range) this.loadStats(range.from, range.to);
    }

    protected exportTakeout(format: TakeoutExportFormat): void {
        const qr = this.qr();
        if (!qr) return;
        this.service
            .downloadQRShareTakeout(this.token, qr, format)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({ error: () => this.errorKey.set('qrCodes.errors.takeout') });
    }

    protected takeoutMenuItems(): MenuItem[] {
        return buildTakeoutExportMenuItems(this.transloco, (format) => this.exportTakeout(format));
    }

    private loadStats(from: string, to: string): void {
        this.statsLoading.set(true);
        this.service
            .qrShareSummary(this.token, from, to)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (summary) => this.summary.set(summary),
                error: () => this.errorKey.set('qrCodes.errors.stats'),
                complete: () => this.statsLoading.set(false)
            });
        this.service
            .qrShareOpenSeries(this.token, from, to)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (points) => this.series.set(points.map((point) => ({ time: point.time, opens: point.opens }))),
                error: () => this.errorKey.set('qrCodes.errors.stats')
            });
    }

    private currentDateRange(): { from: string; to: string } | null {
        return resolveDateRange(this.selectedRange(), this.customRangeDates());
    }
}
