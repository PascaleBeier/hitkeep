import { ChangeDetectionStrategy, Component, inject } from '@angular/core';

import { SiteTrackingSettings } from '@features/sites/components/site-tracking-settings';
import { SiteService } from '@features/sites/services/site.service';

@Component({
    selector: 'app-site-tracking-settings-page',
    imports: [SiteTrackingSettings],
    template: '<app-site-tracking-settings [site]="site()" />',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteTrackingSettingsPage {
    protected readonly site = inject(SiteService).activeSite;
}
