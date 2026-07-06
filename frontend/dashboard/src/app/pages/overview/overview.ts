import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { finalize, Subscription } from 'rxjs';
import { ButtonModule } from 'primeng/button';
import { CardModule } from 'primeng/card';
import { IconFieldModule } from 'primeng/iconfield';
import { InputIconModule } from 'primeng/inputicon';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';
import { SelectModule } from 'primeng/select';
import { SkeletonModule } from 'primeng/skeleton';
import { injectActiveLang } from '@core/i18n/active-lang';
import { DateRange } from '@components/range-toolbar/range-toolbar';
import { ReportRangeToolbar } from '@components/report-range-toolbar/report-range-toolbar';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { SiteFavicon } from '@features/sites/components/site-favicon';
import { SiteService } from '@features/sites/services/site.service';
import { StatsService } from '@features/analytics/services/stats.service';
import { ChartDataPoint, Site, SiteOverviewStats } from '@models/analytics.types';
import { MainLayoutContextService } from '@layout/main-layout-context.service';
import { injectReportRange } from '@services/report-range-preferences.service';

type OverviewSortKey = 'domain' | 'pageviews' | 'visitors';
type OverviewSiteStatus = 'loading' | 'ready' | 'error';

interface SiteStatsState {
    status: OverviewSiteStatus;
    stats: SiteOverviewStats | null;
}

interface OverviewSortOption {
    label: string;
    value: OverviewSortKey;
}

interface OverviewSiteRow {
    site: Site;
    status: OverviewSiteStatus;
    pageviews: number;
    pageviewsLabel: string;
    visitors: number;
    visitorsLabel: string;
    bounceRateLabel: string;
    sparklinePoints: string;
    sparklineAreaPoints: string;
    sparklineAria: string;
    hasTraffic: boolean;
    hasSparkline: boolean;
    searchText: string;
}

const SPARKLINE_WIDTH = 160;
const SPARKLINE_HEIGHT = 56;

@Component({
    selector: 'app-overview-page',
    imports: [FormsModule, ButtonModule, CardModule, IconFieldModule, InputIconModule, InputTextModule, MessageModule, PageBreadcrumb, PageHeader, PageHeaderLeft, ReportRangeToolbar, SelectModule, SiteFavicon, SkeletonModule, TranslocoPipe],
    templateUrl: './overview.html',
    styleUrl: './overview.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class OverviewPage {
    protected readonly siteService = inject(SiteService);
    private readonly transloco = inject(TranslocoService);
    private readonly localeService = inject(TranslocoLocaleService);
    private readonly statsService = inject(StatsService);
    private readonly router = inject(Router);
    private readonly destroyRef = inject(DestroyRef);
    private readonly layoutContext = inject(MainLayoutContextService, { optional: true });
    private readonly reportRange = injectReportRange();
    private readonly activeLanguage = injectActiveLang();
    private statsRequest: Subscription | null = null;
    private loadSequence = 0;

    protected readonly searchTerm = signal('');
    protected readonly sortValue = signal<OverviewSortKey>('domain');
    protected readonly siteStates = signal<Record<string, SiteStatsState>>({});
    protected readonly isStatsLoading = signal(false);

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        return [{ label: this.transloco.translate('overview.title'), isCurrent: true }];
    });

    protected readonly sortOptions = computed<OverviewSortOption[]>(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('overview.sort.domain'), value: 'domain' },
            { label: this.transloco.translate('overview.sort.pageviews'), value: 'pageviews' },
            { label: this.transloco.translate('overview.sort.visitors'), value: 'visitors' }
        ];
    });

    protected readonly siteRows = computed<OverviewSiteRow[]>(() => {
        this.activeLanguage();
        const term = this.searchTerm().trim().toLowerCase();
        const states = this.siteStates();
        const rows = this.siteService.sites().map((site) => this.buildSiteRow(site, states[site.id] ?? { status: 'loading', stats: null }));
        return rows.filter((row) => !term || row.searchText.includes(term)).sort((left, right) => this.compareRows(left, right));
    });

    constructor() {
        effect(() => {
            this.loadSiteStats(this.siteService.sites(), this.currentDateRange());
        });

        this.destroyRef.onDestroy(() => this.statsRequest?.unsubscribe());
    }

    protected onSearch(event: Event): void {
        this.searchTerm.set((event.target as HTMLInputElement | null)?.value ?? '');
    }

    protected setSort(value: OverviewSortKey | null): void {
        if (value === 'domain' || value === 'pageviews' || value === 'visitors') {
            this.sortValue.set(value);
        }
    }

    protected refreshOverview(): void {
        this.loadSiteStats(this.siteService.sites(), this.currentDateRange());
    }

    protected openAddSite(): void {
        this.layoutContext?.isAddSiteVisible.set(true);
    }

    protected openDashboard(site: Site): void {
        this.siteService.selectSite(site);
        void this.router.navigateByUrl('/dashboard');
    }

    protected openDashboardFromKeyboard(event: KeyboardEvent, site: Site): void {
        if (event.key !== 'Enter' && event.key !== ' ') {
            return;
        }

        event.preventDefault();
        this.openDashboard(site);
    }

    private loadSiteStats(sites: Site[], range: DateRange | null): void {
        const sequence = ++this.loadSequence;
        this.statsRequest?.unsubscribe();

        if (!range || sites.length === 0) {
            this.siteStates.set({});
            this.isStatsLoading.set(false);
            return;
        }

        this.isStatsLoading.set(true);
        this.siteStates.set(
            Object.fromEntries(
                sites.map((site) => [
                    site.id,
                    {
                        status: 'loading',
                        stats: null
                    } satisfies SiteStatsState
                ])
            )
        );

        this.statsRequest = this.statsService
            .fetchSitesOverviewStats(range.from, range.to)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => {
                    if (this.loadSequence === sequence) {
                        this.isStatsLoading.set(false);
                    }
                })
            )
            .subscribe({
                next: (response) => {
                    if (this.loadSequence !== sequence) {
                        return;
                    }

                    const statsBySite = new Map(response.sites.map((stats) => [stats.site_id, stats]));
                    this.siteStates.set(
                        Object.fromEntries(
                            sites.map((site) => {
                                const stats = statsBySite.get(site.id);
                                if (!stats || stats.status === 'error') {
                                    return [site.id, { status: 'error', stats: null } satisfies SiteStatsState];
                                }
                                return [site.id, { status: 'ready', stats } satisfies SiteStatsState];
                            })
                        )
                    );
                },
                error: () => {
                    if (this.loadSequence !== sequence) {
                        return;
                    }
                    this.siteStates.set(Object.fromEntries(sites.map((site) => [site.id, { status: 'error', stats: null } satisfies SiteStatsState])));
                }
            });
    }

    private buildSiteRow(site: Site, state: SiteStatsState): OverviewSiteRow {
        const stats = state.stats;
        const chartData = stats?.chart_data ?? [];
        const pageviews = stats?.total_pageviews ?? 0;
        const visitors = stats?.unique_sessions ?? 0;
        const hasTraffic = !!stats && (pageviews > 0 || chartData.some((point) => point.pageviews > 0 || point.visitors > 0));
        const sparklinePoints = stats ? this.sparklinePoints(chartData, pageviews) : '';
        const hasSparkline = hasTraffic && sparklinePoints.length > 0;
        return {
            site,
            status: state.status,
            pageviews,
            pageviewsLabel: state.status === 'loading' ? '' : this.localeService.localizeNumber(pageviews, 'decimal'),
            visitors,
            visitorsLabel: state.status === 'loading' ? '' : this.localeService.localizeNumber(visitors, 'decimal'),
            bounceRateLabel: state.status === 'ready' && stats ? `${this.localeService.localizeNumber(stats.bounce_rate, 'decimal', undefined, { maximumFractionDigits: 1 })}%` : '-',
            sparklinePoints,
            sparklineAreaPoints: sparklinePoints ? `0,${SPARKLINE_HEIGHT} ${sparklinePoints} ${SPARKLINE_WIDTH},${SPARKLINE_HEIGHT}` : '',
            sparklineAria: this.transloco.translate('overview.chartAria', { site: site.domain }),
            hasTraffic,
            hasSparkline,
            searchText: site.domain.toLowerCase()
        };
    }

    private compareRows(left: OverviewSiteRow, right: OverviewSiteRow): number {
        switch (this.sortValue()) {
            case 'pageviews':
                return right.pageviews - left.pageviews || left.site.domain.localeCompare(right.site.domain);
            case 'visitors':
                return right.visitors - left.visitors || left.site.domain.localeCompare(right.site.domain);
            default:
                return left.site.domain.localeCompare(right.site.domain);
        }
    }

    private sparklinePoints(data: ChartDataPoint[], fallbackValue = 0): string {
        const values = data.length > 0 ? data.map((point) => point.pageviews) : fallbackValue > 0 ? [fallbackValue, fallbackValue] : [];
        if (values.length === 0) {
            return '';
        }
        const max = Math.max(...values, 1);
        const denominator = Math.max(values.length - 1, 1);
        return values
            .map((value, index) => {
                const x = (index / denominator) * SPARKLINE_WIDTH;
                const y = SPARKLINE_HEIGHT - (value / max) * (SPARKLINE_HEIGHT - 4) - 2;
                return `${x.toFixed(1)},${y.toFixed(1)}`;
            })
            .join(' ');
    }

    private currentDateRange(): DateRange | null {
        return this.reportRange.currentDateRange();
    }
}
