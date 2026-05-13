import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, linkedSignal, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { finalize } from 'rxjs';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';
import { MessageModule } from 'primeng/message';
import { TagModule } from 'primeng/tag';
import { SiteService } from '@features/sites/services/site.service';
import { PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { EmptyState } from '@components/molecules/empty-state';
import { PageFrame } from '@components/page-frame/page-frame';
import { DEFAULT_RANGE_OPTIONS, RangeOption, RangeToolbar } from '@components/range-toolbar/range-toolbar';
import { KpiCard } from '@features/analytics/components/kpi-card';
import { injectActiveLang } from '@core/i18n/active-lang';
import { AdminSystemService, SystemAIStatus } from '@services/admin-system.service';
import { Opportunity, OpportunityStatus, OpportunitiesService } from '@services/opportunities.service';
import { PermissionService } from '@services/permission.service';
import { OpportunityCard } from './opportunity-card';
import { OpportunityDetailDrawer } from './opportunity-detail-drawer';
import { OpportunityFilterRail } from './opportunity-filter-rail';
import { OpportunityEvidenceView, OpportunityFilter, OpportunityFilterItem, OpportunityView, StatusFilter, TagSeverity } from './opportunity-view';

@Component({
    selector: 'app-opportunities',
    imports: [TranslocoPipe, ButtonModule, MessageModule, TagModule, PageFrame, EmptyState, RangeToolbar, KpiCard, OpportunityFilterRail, OpportunityCard, OpportunityDetailDrawer],
    templateUrl: './opportunities.html',
    styleUrl: './opportunities.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class OpportunitiesPage {
    protected readonly siteService = inject(SiteService);
    private readonly opportunitiesService = inject(OpportunitiesService);
    private readonly adminSystemService = inject(AdminSystemService);
    private readonly permissionService = inject(PermissionService);
    private readonly transloco = inject(TranslocoService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly activeLanguage = injectActiveLang();

    protected readonly timeRanges = signal<RangeOption[]>(DEFAULT_RANGE_OPTIONS);
    protected readonly selectedRange = linkedSignal<RangeOption[], RangeOption>({
        source: this.timeRanges,
        computation: (ranges, previous) => {
            const value = previous?.value.value ?? '30d';
            return ranges.find((range) => range.value === value) ?? ranges[2]!;
        }
    });
    protected readonly customRangeDates = signal<Date[] | null>(null);
    protected readonly typeFilter = signal<OpportunityFilter>('all');
    protected readonly statusFilter = signal<StatusFilter>('all');
    protected readonly selectedOpportunityId = signal<string | null>(null);
    protected readonly isDetailOpen = signal(false);
    protected readonly isLoading = signal(false);
    protected readonly isGenerating = signal(false);
    protected readonly errorState = signal<'idle' | 'load' | 'generate' | 'status'>('idle');
    protected readonly aiStatus = signal<SystemAIStatus | null>(null);
    protected readonly pendingStatusId = signal<string | null>(null);

    protected readonly typeFilters = ['all', 'conversion', 'traffic', 'ai', 'search', 'setup'] as const;
    protected readonly statusFilters = ['all', 'new', 'saved', 'done'] as const;
    protected readonly opportunities = signal<Opportunity[]>([]);

    protected readonly canManageActiveSite = computed(() => {
        const site = this.siteService.activeSite();
        return site ? this.permissionService.canManageSite(site.id) : false;
    });

    protected readonly aiStatusLabel = computed(() => {
        this.activeLanguage();
        const status = this.aiStatus();
        if (!status?.enabled) {
            return this.transloco.translate('opportunities.aiStatus.disabled');
        }
        if (!status.configured) {
            return this.transloco.translate('opportunities.aiStatus.notConfigured');
        }
        if (status.config_mode === 'cloud_managed') {
            return this.transloco.translate('opportunities.aiStatus.cloudManaged');
        }
        return this.transloco.translate('opportunities.aiStatus.selfHosted', {
            provider: status.provider || this.transloco.translate('opportunities.aiStatus.providerUnknown'),
            model: status.model || this.transloco.translate('opportunities.aiStatus.modelUnknown')
        });
    });

    protected readonly aiStatusSeverity = computed<TagSeverity>(() => {
        const status = this.aiStatus();
        if (!status?.enabled) return 'secondary';
        if (!status.configured || status.budget_exhausted) return 'warn';
        return status.config_mode === 'cloud_managed' ? 'info' : 'success';
    });

    protected readonly aiStatusIcon = computed(() => {
        const status = this.aiStatus();
        if (!status?.enabled) return 'pi pi-power-off';
        if (status.config_mode === 'cloud_managed') return 'pi pi-cloud';
        return 'pi pi-server';
    });

    protected readonly aiStatusMessageKey = computed(() => {
        const status = this.aiStatus();
        if (!status) return null;
        if (!status.enabled) return 'opportunities.aiStatus.disabledHint';
        if (!status.configured) return 'opportunities.aiStatus.notConfiguredHint';
        if (status.budget_exhausted) return 'opportunities.aiStatus.budgetExhaustedHint';
        return null;
    });

    protected readonly aiStatusMessageSeverity = computed<'info' | 'warn'>(() => {
        const status = this.aiStatus();
        if (!status?.enabled) return 'info';
        return status.budget_exhausted || !status.configured ? 'warn' : 'info';
    });

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        const site = this.siteService.activeSite();
        if (!site) {
            return [{ label: this.transloco.translate('nav.opportunities'), isCurrent: true }];
        }
        return [
            { label: site.domain, favicon: site, routerLink: '/dashboard' },
            { label: this.transloco.translate('nav.opportunities'), isCurrent: true }
        ];
    });

    protected readonly visibleOpportunities = computed(() =>
        this.opportunities().filter((opportunity) => {
            if (opportunity.status === 'dismissed') {
                return false;
            }

            const matchesType = this.typeFilter() === 'all' || opportunity.kind === this.typeFilter();
            const matchesStatus = this.statusFilter() === 'all' || opportunity.status === this.statusFilter();
            return matchesType && matchesStatus;
        })
    );

    protected readonly visibleOpportunityViews = computed(() => {
        this.activeLanguage();
        return this.visibleOpportunities().map((opportunity) => this.toOpportunityView(opportunity));
    });

    protected readonly topActionOpportunityViews = computed(() => {
        this.activeLanguage();
        return this.visibleOpportunities()
            .filter((opportunity) => this.isActionableStatus(opportunity.status))
            .slice(0, 3)
            .map((opportunity) => this.toOpportunityView(opportunity));
    });

    protected readonly selectedOpportunityView = computed(() => {
        this.activeLanguage();
        const id = this.selectedOpportunityId();
        const opportunity = this.opportunities().find((item) => item.id === id) ?? this.visibleOpportunities()[0] ?? null;
        return opportunity ? this.toOpportunityView(opportunity) : null;
    });

    protected readonly kpiCards = computed(() => {
        this.activeLanguage();
        const open = this.opportunities().filter((opportunity) => opportunity.status !== 'dismissed' && opportunity.status !== 'done');
        const highConfidence = open.filter((opportunity) => opportunity.confidence === 'high').length;
        const aiWins = open.filter((opportunity) => opportunity.kind === 'ai' || opportunity.kind === 'search').length;
        const averageScore = open.length ? Math.round(open.reduce((sum, opportunity) => sum + opportunity.score, 0) / open.length) : 0;

        return [
            {
                label: this.transloco.translate('opportunities.kpis.open'),
                value: open.length,
                loading: this.isLoading(),
                valueClass: 'text-2xl xl:text-3xl font-bold'
            },
            {
                label: this.transloco.translate('opportunities.kpis.score'),
                value: averageScore,
                loading: this.isLoading(),
                valueClass: 'text-2xl xl:text-3xl font-bold'
            },
            {
                label: this.transloco.translate('opportunities.kpis.highConfidence'),
                value: highConfidence,
                loading: this.isLoading(),
                valueClass: 'text-2xl xl:text-3xl font-bold'
            },
            {
                label: this.transloco.translate('opportunities.kpis.aiSearchWins'),
                value: aiWins,
                loading: this.isLoading(),
                valueClass: 'text-2xl xl:text-3xl font-bold'
            }
        ];
    });

    protected readonly typeCounts = computed(() => {
        const counts = new Map<OpportunityFilter, number>([['all', 0]]);
        for (const opportunity of this.opportunities()) {
            if (opportunity.status === 'dismissed') {
                continue;
            }
            counts.set('all', (counts.get('all') ?? 0) + 1);
            counts.set(opportunity.kind, (counts.get(opportunity.kind) ?? 0) + 1);
        }
        return counts;
    });

    protected readonly statusCounts = computed(() => {
        const counts = new Map<StatusFilter, number>([['all', 0]]);
        for (const opportunity of this.opportunities()) {
            const status = opportunity.status;
            if (status === 'dismissed') {
                continue;
            }
            counts.set('all', (counts.get('all') ?? 0) + 1);
            counts.set(status, (counts.get(status) ?? 0) + 1);
        }
        return counts;
    });

    protected readonly typeFilterItems = computed<OpportunityFilterItem<OpportunityFilter>[]>(() => {
        this.activeLanguage();
        const counts = this.typeCounts();
        const active = this.typeFilter();
        return this.typeFilters.map((filter) => ({
            value: filter,
            label: this.filterLabel(filter),
            count: counts.get(filter) ?? 0,
            active: active === filter
        }));
    });

    protected readonly statusFilterItems = computed<OpportunityFilterItem<StatusFilter>[]>(() => {
        this.activeLanguage();
        const counts = this.statusCounts();
        const active = this.statusFilter();
        return this.statusFilters.map((filter) => ({
            value: filter,
            label: this.statusLabel(filter),
            count: counts.get(filter) ?? 0,
            active: active === filter
        }));
    });

    protected readonly score = computed(() => {
        const visible = this.visibleOpportunities();
        if (!visible.length) return 0;
        return Math.round(visible.reduce((sum, opportunity) => sum + opportunity.score, 0) / visible.length);
    });

    constructor() {
        effect(() => {
            const site = this.siteService.activeSite();
            if (!site) {
                this.opportunities.set([]);
                return;
            }
            this.loadOpportunities(site.id);
        });

        effect(() => {
            if (!this.permissionService.isInstanceAdmin()) {
                this.aiStatus.set(null);
                return;
            }
            this.loadAIStatus();
        });
    }

    protected setTypeFilter(filter: OpportunityFilter) {
        this.typeFilter.set(filter);
    }

    protected setStatusFilter(filter: StatusFilter) {
        this.statusFilter.set(filter);
    }

    protected openOpportunity(opportunity: OpportunityView) {
        this.selectedOpportunityId.set(opportunity.id);
        this.isDetailOpen.set(true);
    }

    protected refreshOpportunities() {
        const site = this.siteService.activeSite();
        const range = this.getCurrentDateRange();
        if (!site || !range || this.isGenerating() || !this.canManageActiveSite()) return;

        this.isGenerating.set(true);
        this.errorState.set('idle');
        this.opportunitiesService
            .generate(site.id, range.from, range.to)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isGenerating.set(false))
            )
            .subscribe({
                next: (response) => {
                    this.opportunities.set(response.opportunities);
                    this.selectedOpportunityId.set(response.opportunities[0]?.id ?? null);
                },
                error: () => this.errorState.set('generate')
            });
    }

    protected setStatus(opportunity: OpportunityView, status: OpportunityStatus, event?: Event) {
        event?.stopPropagation();
        const site = this.siteService.activeSite();
        if (!site || this.pendingStatusId() || !this.canManageActiveSite() || opportunity.status === status) return;

        this.pendingStatusId.set(opportunity.id);
        this.errorState.set('idle');
        this.opportunitiesService
            .updateStatus(site.id, opportunity.id, status)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.pendingStatusId.set(null))
            )
            .subscribe({
                next: (updated) => {
                    this.opportunities.update((items) => items.map((item) => (item.id === updated.id ? updated : item)));
                    if (updated.status === 'dismissed' && this.selectedOpportunityId() === updated.id) {
                        this.isDetailOpen.set(false);
                        this.selectedOpportunityId.set(this.visibleOpportunities()[0]?.id ?? null);
                    }
                },
                error: () => this.errorState.set('status')
            });
    }

    private statusFor(opportunity: Opportunity): OpportunityStatus {
        return opportunity.status;
    }

    private isActionableStatus(status: OpportunityStatus): boolean {
        return status === 'new' || status === 'saved';
    }

    private severityFor(opportunity: Opportunity): TagSeverity {
        if (opportunity.score >= 88) return 'danger';
        if (opportunity.kind === 'ai') return 'success';
        if (opportunity.kind === 'search') return 'warn';
        if (opportunity.kind === 'setup') return 'secondary';
        return 'info';
    }

    private iconFor(opportunity: Opportunity): string {
        switch (opportunity.kind) {
            case 'conversion':
                return 'pi pi-filter';
            case 'traffic':
                return 'pi pi-chart-line';
            case 'ai':
                return 'pi pi-sparkles';
            case 'search':
                return 'pi pi-search';
            case 'setup':
                return 'pi pi-cog';
        }
    }

    private routeIconFor(opportunity: Opportunity): string {
        return opportunity.route_icon || 'pi pi-arrow-right';
    }

    private titleFor(opportunity: Opportunity): string {
        return this.transloco.translate(opportunity.title_key, opportunity.copy_params);
    }

    private summaryFor(opportunity: Opportunity): string {
        return this.transloco.translate(opportunity.summary_key, opportunity.copy_params);
    }

    private actionFor(opportunity: Opportunity): string {
        return this.transloco.translate(opportunity.action_key, opportunity.copy_params);
    }

    private impactLabelFor(opportunity: Opportunity): string {
        return this.transloco.translate(opportunity.impact_label_key, opportunity.copy_params);
    }

    private routeLabelFor(opportunity: Opportunity): string {
        return this.transloco.translate(opportunity.route_label_key, opportunity.route_params);
    }

    private evidenceLabelFor(evidence: Opportunity['evidence'][number]): string {
        return this.transloco.translate(evidence.label_key, evidence.detail_params);
    }

    private evidenceDetailFor(evidence: Opportunity['evidence'][number]): string | null {
        if (!evidence.detail_key) return null;
        return this.transloco.translate(evidence.detail_key, evidence.detail_params ?? {});
    }

    private filterLabel(filter: OpportunityFilter): string {
        return this.transloco.translate(`opportunities.filters.types.${filter}`);
    }

    private statusLabel(status: OpportunityStatus | StatusFilter): string {
        return this.transloco.translate(`opportunities.filters.statuses.${status}`);
    }

    private confidenceLabel(opportunity: Opportunity): string {
        return this.transloco.translate(`opportunities.confidence.${opportunity.confidence}`);
    }

    private typeLabel(opportunity: Opportunity): string {
        return this.transloco.translate(`opportunities.filters.types.${opportunity.kind}`);
    }

    private scoreWidth(opportunity: Opportunity): string {
        return `${opportunity.score}%`;
    }

    private toOpportunityView(opportunity: Opportunity): OpportunityView {
        return {
            source: opportunity,
            id: opportunity.id,
            kind: opportunity.kind,
            status: opportunity.status,
            title: this.titleFor(opportunity),
            summary: this.summaryFor(opportunity),
            action: this.actionFor(opportunity),
            typeLabel: this.typeLabel(opportunity),
            confidenceLabel: this.confidenceLabel(opportunity),
            statusLabel: this.statusLabel(this.statusFor(opportunity)),
            impactValue: opportunity.impact_value,
            impactLabel: this.impactLabelFor(opportunity),
            routeLabel: this.routeLabelFor(opportunity),
            routeIcon: this.routeIconFor(opportunity),
            icon: this.iconFor(opportunity),
            severity: this.severityFor(opportunity),
            score: opportunity.score,
            scoreWidth: this.scoreWidth(opportunity),
            evidence: opportunity.evidence.map(
                (evidence): OpportunityEvidenceView => ({
                    id: evidence.id,
                    label: this.evidenceLabelFor(evidence),
                    value: evidence.value,
                    detail: this.evidenceDetailFor(evidence)
                })
            )
        };
    }

    private loadOpportunities(siteId: string) {
        this.isLoading.set(true);
        this.errorState.set('idle');
        this.opportunitiesService
            .list(siteId)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isLoading.set(false))
            )
            .subscribe({
                next: (response) => {
                    this.opportunities.set(response.opportunities);
                    this.selectedOpportunityId.set(response.opportunities[0]?.id ?? null);
                },
                error: () => this.errorState.set('load')
            });
    }

    private loadAIStatus() {
        this.adminSystemService
            .getAI()
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (status) => this.aiStatus.set(status),
                error: () => this.aiStatus.set(null)
            });
    }

    private getCurrentDateRange() {
        const range = this.selectedRange();
        const end = new Date();
        const start = new Date();

        if (range.value === 'custom') {
            const dates = this.customRangeDates();
            if (dates && dates.length === 2 && dates[0] && dates[1]) {
                return { from: dates[0].toISOString(), to: dates[1].toISOString() };
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
            case '30d':
                start.setDate(end.getDate() - 30);
                break;
            case '1y':
                start.setFullYear(end.getFullYear() - 1);
                break;
        }
        return { from: start.toISOString(), to: end.toISOString() };
    }
}
