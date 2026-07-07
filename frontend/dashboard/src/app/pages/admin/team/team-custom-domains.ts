import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { ConfirmationService } from 'primeng/api';
import { ConfirmDialogModule } from 'primeng/confirmdialog';

import { TeamService } from '@services/team.service';
import { TeamTrackingDomains } from './team-tracking-domains';

@Component({
    selector: 'app-team-custom-domains',
    imports: [ConfirmDialogModule, TeamTrackingDomains],
    template: `
        <p-confirmdialog />

        @if (team(); as t) {
            <app-team-tracking-domains [teamId]="t.id" />
        }
    `,
    changeDetection: ChangeDetectionStrategy.OnPush,
    providers: [ConfirmationService]
})
export class TeamCustomDomainsPage {
    private readonly teamService = inject(TeamService);

    protected readonly team = this.teamService.activeTeam;
}
