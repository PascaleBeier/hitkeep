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
import { MessageModule } from '@openng/optimus-ui/message';
import { TableModule } from '@openng/optimus-ui/table';
import { finalize, Subscription } from 'rxjs';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { calcDelta } from '@core/analytics/delta-utils';
import { HITKEEP_CHART_PALETTE } from '@core/charts/hitkeep-chart-options';
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
import { StatsService } from '@features/analytics/services/stats.service';
import { SiteService } from '@features/sites/services/site.service';
import { GoalManager } from '@features/goals/components/goal-manager';
import { Goal, GoalSeriesPoint, GoalStats, SiteStats } from '@models/analytics.types';
import { AccessService } from '@services/access.service';
import { AnalyticsService } from '@services/analytics.service';
import { NavigationNoticeService } from '@services/navigation-notice.service';
import { REALTIME_GOAL_KINDS } from '@services/realtime.service';
import { RealtimeRefreshCoordinator } from '@services/realtime-refresh-coordinator.service';
import { injectReportRange } from '@services/report-range-preferences.service';
import { ShareService } from '@services/share.service';
import { injectActiveLang } from '@core/i18n/active-lang';

@Component({
    selector: 'app-goals',
    standalone: true,
    imports: [
        ButtonModule,
        CardModule,
        ConfirmDialogModule,
        ConversionSubjectCard,
        GoalManager,
        InputTextModule,
        KpiCard,
        MessageModule,
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
    templateUrl: './goals.html',
    styleUrl: './goals.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class Goals {
    protected siteService = inject(SiteService);
    private analytics = inject(AnalyticsService);
    private statsService = inject(StatsService);
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
    private reportingSequence = 0;

    protected goals = signal<Goal[]>([]);
    protected goalsLoading = signal(false);
    protected goalsError = signal(false);
    protected selectedGoalId = signal<string | null>(null);
    protected selectedGoal = computed(() => this.goals().find((goal) => goal.id === this.selectedGoalId()) ?? null);
    protected subjectControl = new FormControl<string | null>(null);
    protected editorVisible = signal(false);
    protected editingGoal = signal<Goal | null>(null);
    protected deletingGoalId = signal<string | null>(null);
    protected stats = this.statsQuery.stats;
    protected isStatsLoading = this.statsQuery.isLoading;
    protected baselineStats = signal<SiteStats | null>(null);
    protected baselineLoading = signal(false);
    protected goalSeries = signal<GoalSeriesPoint[]>([]);
    protected comparisonGoalSeries = signal<GoalSeriesPoint[]>([]);
    protected seriesLoading = signal(false);
    protected trafficRefreshKey = signal(0);
    protected isRefreshing = computed(() => this.goalsLoading() || this.isStatsLoading() || this.baselineLoading() || this.seriesLoading());
    protected isShortRange = this.reportRange.isShortRange;
    protected canManage = computed(() => {
        const site = this.siteService.activeSite();
        return !!site && this.access.canSite(site.id, SITE_CAPABILITIES.manageGoals);
    });
    protected trafficDateRange = computed(() => this.reportRange.currentDateRange());
    protected trafficGoalIds = computed(() => {
        const selected = this.selectedGoalId();
        return selected ? [selected] : this.goals().map((goal) => goal.id);
    });
    protected trafficEnabled = computed(() => this.trafficGoalIds().length > 0);
    protected trafficDescriptionKey = computed(() => (this.selectedGoalId() ? 'goals.traffic.selectedDescription' : 'goals.traffic.allDescription'));
    protected trafficShareToken = computed(() => this.share.token());
    protected subjectOptions = computed(() => {
        this.activeLanguage();
        return [{ label: this.transloco.translate('goals.subject.all'), value: null }, ...this.goals().map((goal) => ({ label: goal.name, value: goal.id }))];
    });
    protected subjectDescription = computed(() => {
        this.activeLanguage();
        const goal = this.selectedGoal();
        if (!goal)
            return this.transloco.translate('goals.definitions.count', {
                count: this.goals().length
            });
        return `${this.transloco.translate(`goals.types.${goal.type}`)} · ${goal.value}`;
    });
    protected goalSeriesChart = computed<SeriesChartPoint[]>(() =>
        this.goalSeries().map((point) => ({
            time: point.time,
            conversions: point.conversions
        }))
    );
    protected comparisonGoalSeriesChart = computed<SeriesChartPoint[]>(() =>
        this.comparisonGoalSeries().map((point) => ({
            time: point.time,
            conversions: point.conversions
        }))
    );
    protected seriesConfig = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        return [
            {
                key: 'conversions',
                label: this.transloco.translate('goals.kpis.conversions'),
                color: HITKEEP_CHART_PALETTE.primary
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
        const conversions = this.goalSeries().reduce((sum, point) => sum + point.conversions, 0);
        const comparisonConversions = this.comparisonGoalSeries().reduce((sum, point) => sum + point.conversions, 0);
        const convertingSessions = this.stats()?.unique_sessions ?? 0;
        const comparisonConvertingSessions = this.stats()?.comparison?.unique_sessions ?? 0;
        const allSessions = this.baselineStats()?.unique_sessions ?? 0;
        const comparisonAllSessions = this.baselineStats()?.comparison?.unique_sessions ?? 0;
        const rate = allSessions ? (convertingSessions / allSessions) * 100 : 0;
        const comparisonRate = comparisonAllSessions ? (comparisonConvertingSessions / comparisonAllSessions) * 100 : 0;
        const loading = this.isStatsLoading() || this.baselineLoading() || this.seriesLoading();
        return [
            {
                label: this.transloco.translate('goals.kpis.conversions'),
                value: conversions,
                loading,
                delta: calcDelta(conversions, comparisonConversions)
            },
            {
                label: this.transloco.translate('goals.kpis.convertingSessions'),
                value: convertingSessions,
                loading,
                delta: calcDelta(convertingSessions, comparisonConvertingSessions)
            },
            {
                label: this.transloco.translate('common.kpis.conversionRate'),
                value: rate,
                loading,
                format: KPI_PERCENT_FORMAT,
                suffix: '%',
                delta: calcDelta(rate, comparisonRate)
            }
        ];
    });
    protected tableStats = computed(() => new Map((this.baselineStats()?.goals ?? []).map((item) => [item.goal_id, item])));
    protected pathSuggestions = computed(() => (this.baselineStats()?.top_pages ?? []).map((item) => item.name).filter(Boolean));
    protected eventSuggestions = computed(() =>
        this.goals()
            .filter((goal) => goal.type === 'event')
            .map((goal) => goal.value)
    );
    protected breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        const site = this.siteService.activeSite();
        return site
            ? [
                  { label: site.domain, favicon: site, routerLink: '/dashboard' },
                  { label: this.transloco.translate('nav.goals'), isCurrent: true }
              ]
            : [{ label: this.transloco.translate('nav.goals'), isCurrent: true }];
    });

    constructor() {
        this.destroyRef.onDestroy(() => this.reportingRequest?.unsubscribe());
        this.route.queryParamMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
            this.selectedGoalId.set(params.get('goal'));
            this.createRequested = params.get('create') === '1';
        });
        effect(() => {
            const site = this.siteService.activeSite();
            if (this.lastSiteId && site?.id !== this.lastSiteId) {
                this.selectedGoalId.set(null);
                this.subjectControl.setValue(null, { emitEvent: false });
                void this.router.navigate([], {
                    relativeTo: this.route,
                    queryParams: { goal: null },
                    queryParamsHandling: 'merge',
                    replaceUrl: true
                });
            }
            this.lastSiteId = site?.id ?? null;
            if (site) this.loadGoals(site.id);
            else this.goals.set([]);
        });
        effect(() => {
            const site = this.siteService.activeSite();
            const dates = this.reportRange.currentDateRange();
            const goals = this.goals();
            const selected = this.selectedGoalId();
            if (!site || !dates || this.goalsLoading()) return;
            if (this.createRequested && this.canManage()) {
                this.createRequested = false;
                this.openCreate();
            }
            if (selected && !goals.some((goal) => goal.id === selected)) {
                this.notice.show('goals.notices.unavailable', {
                    preserveNextNavigation: true
                });
                this.selectGoal(null);
                return;
            }
            const cohortIds = selected ? [selected] : goals.map((goal) => goal.id);
            const seriesIds = selected ? [selected] : [];
            this.loadReporting(site.id, dates.from, dates.to, cohortIds, seriesIds);
        });
        this.realtime.registerUntilDestroyed(this.destroyRef, {
            siteId: () => this.siteService.activeSite()?.id ?? null,
            kinds: REALTIME_GOAL_KINDS,
            enabled: () => !!this.siteService.activeSite(),
            refresh: () => this.refresh('background'),
            debounceMs: 700
        });
    }

    protected selectGoal(id: string | null) {
        this.selectedGoalId.set(id);
        this.subjectControl.setValue(id, { emitEvent: false });
        void this.router.navigate([], {
            relativeTo: this.route,
            queryParams: { goal: id, create: null },
            queryParamsHandling: 'merge',
            replaceUrl: true
        });
    }
    protected openCreate() {
        if (!this.canManage()) return;
        this.editingGoal.set(null);
        this.editorVisible.set(true);
    }
    protected openEdit(goal: Goal) {
        this.editingGoal.set(goal);
        this.editorVisible.set(true);
    }
    protected goalActions(goal: Goal): TableRowActionItem[] {
        return [
            {
                label: this.transloco.translate('common.actions.edit'),
                icon: 'pi pi-pencil',
                command: () => this.openEdit(goal)
            },
            {
                label: this.transloco.translate('common.actions.delete'),
                icon: 'pi pi-trash',
                danger: true,
                command: () => this.confirmDelete(goal)
            }
        ];
    }
    private confirmDelete(goal: Goal) {
        this.confirmation.confirm({
            message: this.transloco.translate('goals.manager.confirmDelete', {
                name: goal.name
            }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('common.actions.delete')),
            accept: () => this.deleteGoal(goal)
        });
    }
    private deleteGoal(goal: Goal) {
        const site = this.siteService.activeSite();
        if (!site) return;
        this.deletingGoalId.set(goal.id);
        this.analytics
            .deleteGoal(site.id, goal.id)
            .pipe(finalize(() => this.deletingGoalId.set(null)))
            .subscribe({
                next: () => {
                    if (this.selectedGoalId() === goal.id) this.selectGoal(null);
                    this.loadGoals(site.id);
                }
            });
    }
    protected onDefinitionsChanged() {
        const site = this.siteService.activeSite();
        if (site) this.loadGoals(site.id);
    }
    protected refresh(mode: StatsQueryMode = 'blocking') {
        const site = this.siteService.activeSite();
        const dates = this.reportRange.currentDateRange();
        if (!site || !dates) return;
        this.trafficRefreshKey.update((key) => key + 1);
        const selected = this.selectedGoalId();
        this.loadReporting(site.id, dates.from, dates.to, selected ? [selected] : this.goals().map((goal) => goal.id), selected ? [selected] : [], mode);
    }
    private loadGoals(siteId: string) {
        this.goalsLoading.set(true);
        this.goalsError.set(false);
        this.analytics
            .getGoals(siteId)
            .pipe(finalize(() => this.goalsLoading.set(false)))
            .subscribe({
                next: (goals) => {
                    this.goals.set(goals ?? []);
                    this.subjectControl.setValue(this.selectedGoalId(), {
                        emitEvent: false
                    });
                },
                error: () => {
                    this.goals.set([]);
                    this.goalsError.set(true);
                }
            });
    }
    private loadReporting(siteId: string, from: string, to: string, cohortIds: string[], seriesIds: string[], mode: StatsQueryMode = 'blocking') {
        const sequence = ++this.reportingSequence;
        this.reportingRequest?.unsubscribe();
        const request = new Subscription();
        this.reportingRequest = request;
        this.statsQuery.load({ siteId, from, to, goalIds: cohortIds, mode });
        const blocking = mode === 'blocking' || !this.baselineStats();
        if (blocking) this.baselineLoading.set(true);
        request.add(
            this.statsService
                .fetchStats(siteId, from, to)
                .pipe(finalize(() => sequence === this.reportingSequence && blocking && this.baselineLoading.set(false)))
                .subscribe({ next: (stats) => sequence === this.reportingSequence && this.baselineStats.set(stats) })
        );
        this.seriesLoading.set(blocking);
        request.add(
            this.analytics
                .getGoalTimeseries(siteId, from, to, seriesIds)
                .pipe(finalize(() => sequence === this.reportingSequence && blocking && this.seriesLoading.set(false)))
                .subscribe({ next: (series) => sequence === this.reportingSequence && this.goalSeries.set(series ?? []) })
        );
        const comparison = untracked(() => this.statsQuery.comparisonRange());
        if (comparison) {
            request.add(
                this.analytics.getGoalTimeseries(siteId, comparison.from, comparison.to, seriesIds).subscribe({
                    next: (series) => sequence === this.reportingSequence && this.comparisonGoalSeries.set(series ?? [])
                })
            );
        }
    }
    protected conversionStats(goal: Goal): GoalStats | undefined {
        return this.tableStats().get(goal.id);
    }
}
