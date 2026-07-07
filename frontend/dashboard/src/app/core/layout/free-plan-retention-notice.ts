import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';

import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { ShareService } from '@services/share.service';
import { TeamService } from '@services/team.service';

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
    protected readonly visible = computed(() => {
        this.dismissalRevision();

        const team = this.team();
        return Boolean(this.bootstrap.cloudHosted() && !this.share.isShareMode() && team?.plan?.code === 'free' && !this.isDismissed(this.dismissalKey()));
    });

    protected dismiss(): void {
        const key = this.dismissalKey();
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
