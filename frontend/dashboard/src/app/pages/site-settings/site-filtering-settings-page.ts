import { ChangeDetectionStrategy, Component, inject } from '@angular/core';

import { SiteExclusionSettings } from '@features/sites/components/site-exclusion-settings';
import { SiteService } from '@features/sites/services/site.service';

@Component({
    selector: 'app-site-filtering-settings-page',
    imports: [SiteExclusionSettings],
    template: '<app-site-exclusion-settings [site]="site()" />',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteFilteringSettingsPage {
    protected readonly site = inject(SiteService).activeSite;
}
