import { ChangeDetectionStrategy, Component, inject } from '@angular/core';

import { SettingsAPIClients } from '@features/settings/components/settings-api-clients';
import { TeamService } from '@services/team.service';

@Component({
    selector: 'app-team-api-clients',
    imports: [SettingsAPIClients],
    template: `
        @if (team(); as t) {
            <app-settings-api-clients scope="team" [teamId]="t.id" />
        }
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TeamAPIClientsPage {
    private readonly teamService = inject(TeamService);

    protected readonly team = this.teamService.activeTeam;
}
