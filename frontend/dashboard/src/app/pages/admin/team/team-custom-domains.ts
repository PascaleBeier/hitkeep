import { ChangeDetectionStrategy, Component, computed, effect, inject } from '@angular/core';
import { Router } from '@angular/router';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';

import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { TeamService } from '@services/team.service';
import { TeamTrackingDomains } from './team-tracking-domains';

@Component({
    selector: 'app-team-custom-domains',
    imports: [ConfirmDialogModule, TeamTrackingDomains],
    template: `
        <p-confirmdialog />

        @if (!locked()) {
            @if (team(); as t) {
                <app-team-tracking-domains [teamId]="t.id" />
            }
        }
    `,
    changeDetection: ChangeDetectionStrategy.OnPush,
    providers: [ConfirmationService]
})
export class TeamCustomDomainsPage {
    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly router = inject(Router);
    private readonly teamService = inject(TeamService);

    protected readonly team = this.teamService.activeTeam;
    /** Custom domains require Pro or higher on managed cloud; free teams are sent to the plan comparison. */
    protected readonly locked = computed(() => this.bootstrap.cloudHosted() && this.team()?.plan?.code === 'free');

    constructor() {
        effect(() => {
            if (this.locked()) {
                void this.router.navigate(['/admin/team/overview']);
            }
        });
    }
}
