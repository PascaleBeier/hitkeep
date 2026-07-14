import { ChangeDetectionStrategy, Component, inject } from '@angular/core';

import { SiteGeneralSettings } from '@features/sites/components/site-general-settings';
import { SiteService } from '@features/sites/services/site.service';

@Component({
    selector: 'app-site-general-settings-page',
    imports: [SiteGeneralSettings],
    template: '<app-site-general-settings [site]="site()" />',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteGeneralSettingsPage {
    protected readonly site = inject(SiteService).activeSite;
}
