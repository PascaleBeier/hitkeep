import { ChangeDetectionStrategy, Component, inject } from '@angular/core';

import { SiteDangerZone } from '@features/sites/components/site-danger-zone';
import { SiteService } from '@features/sites/services/site.service';

@Component({
    selector: 'app-site-danger-zone-page',
    imports: [SiteDangerZone],
    template: '<app-site-danger-zone [site]="site()" />',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteDangerZonePage {
    protected readonly site = inject(SiteService).activeSite;
}
