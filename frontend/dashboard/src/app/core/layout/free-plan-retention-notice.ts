import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';

import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { ShareService } from '@services/share.service';
import { TeamService } from '@services/team.service';

type UsagePressureKind = 'sites' | 'members';

interface UsagePressure {
    kind: UsagePressureKind;
    current: number;
    limit: number;
}

const USAGE_PRESSURE_THRESHOLD = 0.8;

@Component({
    selector: 'app-free-plan-retention-notice',
    imports: [ButtonModule, RouterLink, TranslocoPipe],
    templateUrl: './free-plan-retention-notice.html',
    styleUrl: './free-plan-retention-notice.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class FreePlanRetentionNotice {
    private static readonly dismissedValue = 'dismissed';

    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly document = inject(DOCUMENT);
    private readonly share = inject(ShareService);
    private readonly teamService = inject(TeamService);

    private readonly dismissalRevision = signal(0);

    protected readonly team = this.teamService.activeTeam;
    protected readonly retentionDays = computed(() => this.team()?.entitlements?.max_retention_days || 60);
    protected readonly dismissalKey = computed(() => {
        const team = this.team();
        return team?.id ? `hitkeep.freeRetentionNotice.dismissed.${team.id}` : '';
    });

    /** The most exhausted plan limit at or above the pressure threshold, if any. */
    protected readonly usagePressure = computed<UsagePressure | null>(() => {
        const team = this.team();
        const usage = team?.usage;
        const entitlements = team?.entitlements;
        if (!usage || !entitlements) {
            return null;
        }

        const candidates: UsagePressure[] = [
            { kind: 'sites', current: usage.current_sites ?? 0, limit: entitlements.max_sites_per_team },
            { kind: 'members', current: usage.current_members ?? 0, limit: entitlements.max_team_members }
        ];
        return (
            candidates
                .filter((candidate) => candidate.limit > 0 && candidate.current / candidate.limit >= USAGE_PRESSURE_THRESHOLD)
                .sort((left, right) => right.current / right.limit - left.current / left.limit)[0] ?? null
        );
    });

    protected readonly usageDismissalKey = computed(() => {
        const team = this.team();
        const pressure = this.usagePressure();
        return team?.id && pressure ? `hitkeep.freeUsageNotice.dismissed.${team.id}.${pressure.kind}` : '';
    });

    /** Usage pressure has its own dismissal so it resurfaces even after the retention notice was dismissed. */
    protected readonly variant = computed<'usage' | 'retention' | null>(() => {
        this.dismissalRevision();

        const team = this.team();
        const eligible = Boolean(this.bootstrap.cloudHosted() && !this.share.isShareMode() && team?.plan?.code === 'free');
        if (!eligible) {
            return null;
        }
        if (this.usagePressure() && !this.isDismissed(this.usageDismissalKey())) {
            return 'usage';
        }
        if (!this.isDismissed(this.dismissalKey())) {
            return 'retention';
        }
        return null;
    });

    protected readonly visible = computed(() => this.variant() !== null);

    protected dismiss(): void {
        const key = this.variant() === 'usage' ? this.usageDismissalKey() : this.dismissalKey();
        if (!key) {
            return;
        }

        try {
            this.document.defaultView?.localStorage.setItem(key, FreePlanRetentionNotice.dismissedValue);
        } catch {
            // Browsers can deny localStorage in restricted contexts; dismissal is best-effort.
        }
        this.dismissalRevision.update((value) => value + 1);
    }

    private isDismissed(key: string): boolean {
        if (!key) {
            return false;
        }

        try {
            return this.document.defaultView?.localStorage.getItem(key) === FreePlanRetentionNotice.dismissedValue;
        } catch {
            return false;
        }
    }
}
