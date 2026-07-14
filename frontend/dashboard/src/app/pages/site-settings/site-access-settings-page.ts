import { ChangeDetectionStrategy, Component, inject } from '@angular/core';

import { SiteTeamSettings } from '@features/sites/components/site-team-settings';
import { SiteService } from '@features/sites/services/site.service';

@Component({
    selector: 'app-site-access-settings-page',
    imports: [SiteTeamSettings],
    template: '<app-site-team-settings [site]="site()" />',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteAccessSettingsPage {
    protected readonly site = inject(SiteService).activeSite;
}
