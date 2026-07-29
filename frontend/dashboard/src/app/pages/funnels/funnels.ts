import { ChangeDetectionStrategy, Component, computed, DestroyRef, effect, inject, signal, untracked } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormControl } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoDecimalPipe, TranslocoLocaleService } from '@jsverse/transloco-locale';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { ButtonModule } from '@openng/optimus-ui/button';
import { CardModule } from '@openng/optimus-ui/card';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { TableModule } from '@openng/optimus-ui/table';
import { finalize, Subscription } from 'rxjs';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { calcDelta } from '@core/analytics/delta-utils';
import { HITKEEP_CHART_PALETTE } from '@core/charts/hitkeep-chart-options';
import { injectActiveLang } from '@core/i18n/active-lang';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageState } from '@components/page-state/page-state';
import { ReportRangeToolbar } from '@components/report-range-toolbar/report-range-toolbar';
import { TableRowActionItem, TableRowActions } from '@components/table-row-actions/table-row-actions';
import { KpiCard, KpiCardModel, KPI_PERCENT_FORMAT } from '@features/analytics/components/kpi-card';
import { ConversionSubjectCard } from '@features/analytics/components/conversion-subject-card';
import { SeriesChart, SeriesChartPoint, SeriesDefinition } from '@features/analytics/components/series-chart';
import { TrafficRecordsCard } from '@features/hits/components/traffic-records-card';
import { injectStatsQuery, StatsQueryMode } from '@features/analytics/services/stats-query';
import { FunnelManager } from '@features/funnels/components/funnel-manager';
import { SiteService } from '@features/sites/services/site.service';
import { Funnel, FunnelSeriesPoint, FunnelStats } from '@models/analytics.types';
import { AccessService } from '@services/access.service';
import { AnalyticsService } from '@services/analytics.service';
import { NavigationNoticeService } from '@services/navigation-notice.service';
import { REALTIME_FUNNEL_KINDS } from '@services/realtime.service';
import { RealtimeRefreshCoordinator } from '@services/realtime-refresh-coordinator.service';
import { injectReportRange } from '@services/report-range-preferences.service';
import { ShareService } from '@services/share.service';

@Component({
    selector: 'app-funnels',
    standalone: true,
    imports: [
        ButtonModule,
        CardModule,
        ConfirmDialogModule,
        ConversionSubjectCard,
        FunnelManager,
        InputTextModule,
        KpiCard,
        PageBreadcrumb,
        PageHeader,
        PageHeaderLeft,
        PageState,
        ReportRangeToolbar,
        SeriesChart,
        TableModule,
        TableRowActions,
        TrafficRecordsCard,
        TranslocoDecimalPipe,
        TranslocoPipe
    ],
    providers: [ConfirmationService],
    templateUrl: './funnels.html',
    styleUrl: './funnels.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class Funnels {
    protected siteService = inject(SiteService);
    private analytics = inject(AnalyticsService);
    private access = inject(AccessService);
    private route = inject(ActivatedRoute);
    private router = inject(Router);
    private transloco = inject(TranslocoService);
    private locale = inject(TranslocoLocaleService);
    private confirmation = inject(ConfirmationService);
    private destroyRef = inject(DestroyRef);
    private realtime = inject(RealtimeRefreshCoordinator);
    private notice = inject(NavigationNoticeService);
    private share = inject(ShareService);
    protected reportRange = injectReportRange();
    private activeLanguage = injectActiveLang();
    private statsQuery = injectStatsQuery();
    private lastSiteId: string | null = null;
    private createRequested = false;
    private reportingRequest: Subscription | null = null;
    private funnelStatsRequest: Subscription | null = null;
    private reportingSequence = 0;
    private funnelStatsSequence = 0;

    protected funnels = signal<Funnel[]>([]);
    protected loading = signal(false);
    protected loadError = signal(false);
    protected selectedFunnelId = signal<string | null>(null);
    protected selectedFunnel = computed(() => this.funnels().find((funnel) => funnel.id === this.selectedFunnelId()) ?? null);
    protected subjectControl = new FormControl<string | null>(null);
    protected editorVisible = signal(false);
    protected editingFunnel = signal<Funnel | null>(null);
    protected deletingFunnelId = signal<string | null>(null);
    protected funnelSeries = signal<FunnelSeriesPoint[]>([]);
    protected comparisonFunnelSeries = signal<FunnelSeriesPoint[]>([]);
    protected seriesLoading = signal(false);
    protected funnelStats = signal<FunnelStats | null>(null);
    protected funnelStatsLoading = signal(false);
    protected trafficRefreshKey = signal(0);
    protected isShortRange = this.reportRange.isShortRange;
    protected stats = this.statsQuery.stats;
    protected isStatsLoading = this.statsQuery.isLoading;
    protected isRefreshing = computed(() => this.loading() || this.seriesLoading() || this.funnelStatsLoading());
    protected canManage = computed(() => {
        const site = this.siteService.activeSite();
        return !!site && this.access.canSite(site.id, SITE_CAPABILITIES.manageGoals);
    });
    protected trafficDateRange = computed(() => this.reportRange.currentDateRange());
    protected trafficFunnelIds = computed(() => {
        const selected = this.selectedFunnelId();
        return selected ? [selected] : this.funnels().map((funnel) => funnel.id);
    });
    protected trafficEnabled = computed(() => this.trafficFunnelIds().length > 0);
    protected trafficDescriptionKey = computed(() => (this.selectedFunnelId() ? 'funnels.traffic.selectedDescription' : 'funnels.traffic.allDescription'));
    protected trafficShareToken = computed(() => this.share.token());
    protected subjectOptions = computed(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('funnels.subject.all'), value: null },
            ...this.funnels().map((funnel) => ({
                label: funnel.name,
                value: funnel.id
            }))
        ];
    });
    protected subjectDescription = computed(() => {
        this.activeLanguage();
        const funnel = this.selectedFunnel();
        if (!funnel)
            return this.transloco.translate('funnels.definitions.count', {
                count: this.funnels().length
            });
        const steps = funnel.steps.map((step) => step.value).join(' → ');
        return `${this.transloco.translate('funnels.list.stepsCount', { count: funnel.steps.length })} · ${steps}`;
    });
    protected seriesData = computed<SeriesChartPoint[]>(() =>
        this.funnelSeries().map((point) => ({
            time: point.time,
            entries: point.entries,
            completions: point.completions
        }))
    );
    protected comparisonSeriesData = computed<SeriesChartPoint[]>(() =>
        this.comparisonFunnelSeries().map((point) => ({
            time: point.time,
            entries: point.entries,
            completions: point.completions
        }))
    );
    protected seriesConfig = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        return [
            {
                key: 'entries',
                label: this.transloco.translate('funnels.kpis.entries'),
                color: HITKEEP_CHART_PALETTE.primary
            },
            {
                key: 'completions',
                label: this.transloco.translate('funnels.kpis.completions'),
                color: HITKEEP_CHART_PALETTE.secondary
            }
        ];
    });
    protected comparisonLabel = computed(() => {
        this.activeLanguage();
        const range = this.statsQuery.comparisonRange();
        if (!range) return '';
        const format = (value: string) =>
            this.locale.localizeDate(new Date(value), undefined, {
                month: 'short',
                day: 'numeric'
            });
        return `${format(range.from)} – ${format(range.to)}`;
    });
    protected kpis = computed<KpiCardModel[]>(() => {
        this.activeLanguage();
        const entries = this.funnelSeries().reduce((sum, point) => sum + point.entries, 0);
        const completions = this.funnelSeries().reduce((sum, point) => sum + point.completions, 0);
        const comparisonEntries = this.comparisonFunnelSeries().reduce((sum, point) => sum + point.entries, 0);
        const comparisonCompletions = this.comparisonFunnelSeries().reduce((sum, point) => sum + point.completions, 0);
        const rate = entries ? (completions / entries) * 100 : 0;
        const comparisonRate = comparisonEntries ? (comparisonCompletions / comparisonEntries) * 100 : 0;
        return [
            {
                label: this.transloco.translate('funnels.kpis.entries'),
                value: entries,
                loading: this.seriesLoading(),
                delta: calcDelta(entries, comparisonEntries)
            },
            {
                label: this.transloco.translate('funnels.kpis.completions'),
                value: completions,
                loading: this.seriesLoading(),
                delta: calcDelta(completions, comparisonCompletions)
            },
            {
                label: this.transloco.translate('funnels.kpis.completionRate'),
                value: rate,
                loading: this.seriesLoading(),
                format: KPI_PERCENT_FORMAT,
                suffix: '%',
                delta: calcDelta(rate, comparisonRate)
            }
        ];
    });
    protected pathSuggestions = computed(() => (this.stats()?.top_pages ?? []).map((item) => item.name).filter(Boolean));
    protected eventSuggestions = computed(() => [...new Set(this.funnels().flatMap((funnel) => funnel.steps.filter((step) => step.type === 'event').map((step) => step.value)))]);
    protected breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        const site = this.siteService.activeSite();
        return site
            ? [
                  { label: site.domain, favicon: site, routerLink: '/dashboard' },
                  { label: this.transloco.translate('nav.funnels'), isCurrent: true }
              ]
            : [{ label: this.transloco.translate('nav.funnels'), isCurrent: true }];
    });

    constructor() {
        this.destroyRef.onDestroy(() => {
            this.reportingRequest?.unsubscribe();
            this.funnelStatsRequest?.unsubscribe();
        });
        this.route.queryParamMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
            this.selectedFunnelId.set(params.get('funnel'));
            this.createRequested = params.get('create') === '1';
        });
        effect(() => {
            const site = this.siteService.activeSite();
            if (this.lastSiteId && site?.id !== this.lastSiteId) {
                this.selectedFunnelId.set(null);
                this.subjectControl.setValue(null, { emitEvent: false });
                void this.router.navigate([], {
                    relativeTo: this.route,
                    queryParams: { funnel: null },
                    queryParamsHandling: 'merge',
                    replaceUrl: true
                });
            }
            this.lastSiteId = site?.id ?? null;
            if (site) this.loadFunnels(site.id);
            else this.funnels.set([]);
        });
        effect(() => {
            const site = this.siteService.activeSite();
            const dates = this.reportRange.currentDateRange();
            const funnels = this.funnels();
            const selected = this.selectedFunnelId();
            if (!site || !dates || this.loading()) return;
            if (this.createRequested && this.canManage()) {
                this.createRequested = false;
                this.openCreate();
            }
            if (selected && !funnels.some((funnel) => funnel.id === selected)) {
                this.notice.show('funnels.notices.unavailable', {
                    preserveNextNavigation: true
                });
                this.selectFunnel(null);
                return;
            }
            this.loadReporting(site.id, dates.from, dates.to, selected ? [selected] : []);
            if (selected) this.loadFunnelStats(site.id, selected, dates.from, dates.to);
            else this.clearFunnelStats();
        });
        this.realtime.registerUntilDestroyed(this.destroyRef, {
            siteId: () => this.siteService.activeSite()?.id ?? null,
            kinds: REALTIME_FUNNEL_KINDS,
            enabled: () => !!this.siteService.activeSite(),
            refresh: () => this.refresh('background'),
            debounceMs: 700
        });
    }

    protected selectFunnel(id: string | null) {
        this.selectedFunnelId.set(id);
        this.subjectControl.setValue(id, { emitEvent: false });
        void this.router.navigate([], {
            relativeTo: this.route,
            queryParams: { funnel: id, create: null },
            queryParamsHandling: 'merge',
            replaceUrl: true
        });
    }
    protected openCreate() {
        if (!this.canManage()) return;
        this.editingFunnel.set(null);
        this.editorVisible.set(true);
    }
    protected openEdit(funnel: Funnel) {
        this.editingFunnel.set(funnel);
        this.editorVisible.set(true);
    }
    protected funnelActions(funnel: Funnel): TableRowActionItem[] {
        return [
            {
                label: this.transloco.translate('common.actions.edit'),
                icon: 'pi pi-pencil',
                command: () => this.openEdit(funnel)
            },
            {
                label: this.transloco.translate('common.actions.delete'),
                icon: 'pi pi-trash',
                danger: true,
                command: () => this.confirmDelete(funnel)
            }
        ];
    }
    private confirmDelete(funnel: Funnel) {
        this.confirmation.confirm({
            message: this.transloco.translate('funnels.manager.confirmDelete', {
                name: funnel.name
            }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('common.actions.delete')),
            accept: () => this.deleteFunnel(funnel)
        });
    }
    private deleteFunnel(funnel: Funnel) {
        const site = this.siteService.activeSite();
        if (!site) return;
        this.deletingFunnelId.set(funnel.id);
        this.analytics
            .deleteFunnel(site.id, funnel.id)
            .pipe(finalize(() => this.deletingFunnelId.set(null)))
            .subscribe({
                next: () => {
                    if (this.selectedFunnelId() === funnel.id) this.selectFunnel(null);
                    this.loadFunnels(site.id);
                }
            });
    }
    protected onDefinitionsChanged() {
        const site = this.siteService.activeSite();
        if (site) this.loadFunnels(site.id);
    }
    protected refresh(mode: StatsQueryMode = 'blocking') {
        const site = this.siteService.activeSite();
        const dates = this.reportRange.currentDateRange();
        if (!site || !dates) return;
        this.trafficRefreshKey.update((key) => key + 1);
        const selected = this.selectedFunnelId();
        this.loadReporting(site.id, dates.from, dates.to, selected ? [selected] : [], mode);
        if (selected) this.loadFunnelStats(site.id, selected, dates.from, dates.to);
    }
    private loadFunnels(siteId: string) {
        this.loading.set(true);
        this.loadError.set(false);
        this.analytics
            .getFunnels(siteId)
            .pipe(finalize(() => this.loading.set(false)))
            .subscribe({
                next: (funnels) => {
                    this.funnels.set(funnels ?? []);
                    this.subjectControl.setValue(this.selectedFunnelId(), {
                        emitEvent: false
                    });
                },
                error: () => {
                    this.funnels.set([]);
                    this.loadError.set(true);
                }
            });
    }
    private loadReporting(siteId: string, from: string, to: string, funnelIds: string[], mode: StatsQueryMode = 'blocking') {
        const sequence = ++this.reportingSequence;
        this.reportingRequest?.unsubscribe();
        const request = new Subscription();
        this.reportingRequest = request;
        this.statsQuery.load({ siteId, from, to, funnelIds, mode });
        const blocking = mode === 'blocking';
        if (blocking) this.seriesLoading.set(true);
        request.add(
            this.analytics
                .getFunnelTimeseries(siteId, from, to, funnelIds)
                .pipe(finalize(() => sequence === this.reportingSequence && blocking && this.seriesLoading.set(false)))
                .subscribe({ next: (series) => sequence === this.reportingSequence && this.funnelSeries.set(series ?? []) })
        );
        const comparison = untracked(() => this.statsQuery.comparisonRange());
        if (comparison) {
            request.add(
                this.analytics.getFunnelTimeseries(siteId, comparison.from, comparison.to, funnelIds).subscribe({
                    next: (series) => sequence === this.reportingSequence && this.comparisonFunnelSeries.set(series ?? [])
                })
            );
        }
    }
    private loadFunnelStats(siteId: string, funnelId: string, from: string, to: string) {
        const sequence = ++this.funnelStatsSequence;
        this.funnelStatsRequest?.unsubscribe();
        this.funnelStatsLoading.set(true);
        this.funnelStatsRequest = this.analytics
            .getFunnelStats(siteId, funnelId, from, to)
            .pipe(finalize(() => sequence === this.funnelStatsSequence && this.funnelStatsLoading.set(false)))
            .subscribe({
                next: (stats) => sequence === this.funnelStatsSequence && this.funnelStats.set(stats),
                error: () => sequence === this.funnelStatsSequence && this.funnelStats.set(null)
            });
    }

    private clearFunnelStats() {
        this.funnelStatsSequence += 1;
        this.funnelStatsRequest?.unsubscribe();
        this.funnelStatsRequest = null;
        this.funnelStatsLoading.set(false);
        this.funnelStats.set(null);
    }
}
