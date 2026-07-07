import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, resource, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { ButtonModule } from 'primeng/button';
import { CardModule } from 'primeng/card';
import { ProgressBarModule } from 'primeng/progressbar';
import { TagModule } from 'primeng/tag';
import { CopyControl } from '@components/copy-control/copy-control';
import { TeamService } from '@services/team.service';
import { injectActiveLang } from '@core/i18n/active-lang';
import { AnalyticsService } from '@services/analytics.service';
import { CloudService } from '@services/cloud.service';
import { firstValueFrom } from 'rxjs';

import { CloudPlanTier, TeamPlan, TeamRole } from '@models/analytics.types';

/** Monthly EUR list prices per cloud plan; formatted locale-aware at render time. */
const PLAN_MONTHLY_PRICES_EUR: Record<string, number> = { free: 0, pro: 15, business: 39 };

@Component({
    selector: 'app-team-overview',
    imports: [ButtonModule, CardModule, ProgressBarModule, TagModule, CopyControl, TranslocoPipe],
    templateUrl: './team-overview.html',
    styleUrl: './team-overview.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TeamOverviewPage {
    private readonly destroyRef = inject(DestroyRef);
    private readonly transloco = inject(TranslocoService);
    private readonly localeService = inject(TranslocoLocaleService);
    private readonly activeLanguage = injectActiveLang();
    private readonly analyticsService = inject(AnalyticsService);
    private readonly cloudService = inject(CloudService);
    protected readonly teamService = inject(TeamService);

    protected readonly team = this.teamService.activeTeam;
    protected readonly systemStatusResource = resource({
        loader: () => firstValueFrom(this.analyticsService.getSystemStatus())
    });
    protected readonly systemStatus = computed(() => this.systemStatusResource.value() ?? null);
    protected readonly planTiers = signal<CloudPlanTier[]>([]);
    protected readonly portalPending = signal(false);
    protected readonly checkoutPending = signal(false);
    protected readonly usageCards = computed(() => {
        const team = this.team();
        const cloud = this.systemStatus()?.cloud;
        if (!cloud?.hosted || !team?.usage || !team.entitlements) {
            return [];
        }

        return [this.buildUsageCard('sites', team.usage.current_sites, team.entitlements.max_sites_per_team), this.buildUsageCard('members', team.usage.current_members, team.entitlements.max_team_members)];
    });
    protected readonly cloudPlan = computed(() => {
        const team = this.team();
        const cloud = this.systemStatus()?.cloud;
        if (!cloud?.hosted || !team?.plan || !team.entitlements) {
            return null;
        }

        return {
            plan: team.plan,
            cloud,
            retentionDays: team.entitlements.max_retention_days
        };
    });
    protected readonly showUsageSection = computed(() => this.usageCards().length > 0);
    protected readonly canManageBilling = computed(() => {
        const plan = this.cloudPlan()?.plan;
        return Boolean(plan && plan.code !== 'free' && !this.isOperatorPlan(plan) && !this.portalPending());
    });
    protected readonly canStartUpgrade = computed(() => this.cloudPlan()?.plan.code === 'free' && !this.checkoutPending());
    protected readonly showPlanComparison = computed(() => this.canStartUpgrade() && this.planTiers().length > 1);
    protected readonly currentTier = computed(() => {
        const code = this.cloudPlan()?.plan.code;
        return this.planTiers().find((t) => t.code === code) ?? null;
    });
    protected readonly upgradeTiers = computed(() => this.planTiers().filter((t) => t.code !== 'free'));
    /** Locale-aware plan prices, e.g. "€15" in English and "15 €" in German. */
    protected readonly planPriceLabels = computed<Record<string, string>>(() => {
        this.activeLanguage();
        const labels: Record<string, string> = {};
        for (const [code, amount] of Object.entries(PLAN_MONTHLY_PRICES_EUR)) {
            labels[code] = this.localeService.localizeNumber(amount, 'currency', undefined, { currency: 'EUR', minimumFractionDigits: 0, maximumFractionDigits: 0 });
        }
        return labels;
    });
    private planTiersLoaded = false;

    constructor() {
        effect(() => {
            if (this.planTiersLoaded || !this.systemStatus()?.cloud?.hosted) {
                return;
            }
            this.planTiersLoaded = true;
            this.cloudService
                .getPlans()
                .pipe(takeUntilDestroyed(this.destroyRef))
                .subscribe((tiers) => this.planTiers.set(tiers));
        });
    }

    protected roleSeverity(role: TeamRole): 'danger' | 'info' | 'secondary' {
        switch (role) {
            case 'owner':
                return 'danger';
            case 'admin':
                return 'info';
            case 'member':
            default:
                return 'secondary';
        }
    }

    protected roleLabel(role: TeamRole): string {
        return this.transloco.translate(`teams.roles.${role}`);
    }

    protected usageDescription(current: number): string {
        return this.transloco.translate('admin.team.overview.usage.currentUsage', { count: current });
    }

    protected usageLimitLabel(limit: number): string {
        if (limit <= 0) {
            return this.transloco.translate('admin.team.overview.usage.unlimited');
        }
        return this.transloco.translate('admin.team.overview.usage.limitValue', { count: limit });
    }

    protected usageStateClass(percentage: number, limit: number): string {
        if (limit <= 0) {
            return 'team-overview__usage-card--unlimited';
        }
        if (percentage >= 95) {
            return 'team-overview__usage-card--critical';
        }
        if (percentage >= 80) {
            return 'team-overview__usage-card--warning';
        }
        return 'team-overview__usage-card--healthy';
    }

    protected pendingInviteLabel(count: number): string {
        if (count === 1) {
            return this.transloco.translate('admin.team.overview.usage.pendingInviteOne');
        }
        return this.transloco.translate('admin.team.overview.usage.pendingInviteMany', { count });
    }

    protected retentionLabel(days: number): string {
        if (days <= 0) {
            return this.transloco.translate('admin.team.overview.cloud.unlimitedRetention');
        }
        return this.transloco.translate('admin.team.overview.cloud.retentionDays', { count: days });
    }

    protected planName(plan: TeamPlan): string {
        this.activeLanguage();
        if (this.isOperatorPlan(plan)) {
            return this.transloco.translate('admin.team.overview.cloud.operatorPlan');
        }
        return plan.name;
    }

    protected cloudPlanDescription(plan: TeamPlan): string {
        this.activeLanguage();
        if (this.isOperatorPlan(plan)) {
            return this.transloco.translate('admin.team.overview.cloud.operatorDescription');
        }
        return this.transloco.translate('admin.team.overview.cloud.description');
    }

    protected openBillingPortal(): void {
        if (this.portalPending()) {
            return;
        }

        this.portalPending.set(true);
        this.cloudService
            .createBillingPortalSession({ locale: this.activeLanguage() })
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: ({ url }) => {
                    this.portalPending.set(false);
                    this.redirectTo(url);
                },
                error: () => {
                    this.portalPending.set(false);
                }
            });
    }

    protected startCheckoutForPlan(planCode: 'pro' | 'business'): void {
        if (this.checkoutPending()) {
            return;
        }

        this.checkoutPending.set(true);
        this.cloudService
            .createBillingCheckoutSession({ plan_code: planCode, locale: this.activeLanguage() })
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: ({ url }) => {
                    this.checkoutPending.set(false);
                    this.redirectTo(url);
                },
                error: () => {
                    this.checkoutPending.set(false);
                }
            });
    }

    protected startUpgradeCheckout(): void {
        if (this.checkoutPending()) {
            return;
        }

        this.checkoutPending.set(true);
        this.cloudService
            .createBillingCheckoutSession({ plan_code: 'pro', locale: this.activeLanguage() })
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: ({ url }) => {
                    this.checkoutPending.set(false);
                    this.redirectTo(url);
                },
                error: () => {
                    this.checkoutPending.set(false);
                }
            });
    }

    private isOperatorPlan(plan: TeamPlan): boolean {
        return plan.code === 'operator';
    }

    private buildUsageCard(key: string, current: number, limit: number) {
        const percentage = limit > 0 ? Math.min(100, Math.round((current / limit) * 100)) : 0;

        return {
            key,
            current,
            limit,
            displayValue: current.toLocaleString(),
            percentage,
            className: this.usageStateClass(percentage, limit),
            hasFiniteLimit: limit > 0,
            limitLabel: this.usageLimitLabel(limit),
            description: this.usageDescription(current)
        };
    }

    protected retentionYears(days: number): string {
        if (days <= 0) {
            return this.transloco.translate('admin.team.overview.usage.unlimited');
        }
        if (days < 365) {
            return this.transloco.translate('admin.team.overview.plans.features.retentionDays', { count: days });
        }
        return this.transloco.translate('admin.team.overview.plans.features.retention', { count: Math.round(days / 365) });
    }

    protected featureValue(value: number | boolean, suffix?: string): string {
        if (typeof value === 'boolean') {
            return '';
        }
        if (value <= 0) {
            return this.transloco.translate('admin.team.overview.usage.unlimited');
        }
        return suffix ? `${value} ${suffix}` : `${value}`;
    }

    protected redirectTo(url: string): void {
        window.location.assign(url);
    }
}
