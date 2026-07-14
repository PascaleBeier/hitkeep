import { ChangeDetectionStrategy, Component, inject } from '@angular/core';

import { SiteRetentionSettings } from '@features/sites/components/site-retention-settings';
import { SiteService } from '@features/sites/services/site.service';

@Component({
    selector: 'app-site-retention-settings-page',
    imports: [SiteRetentionSettings],
    template: '<app-site-retention-settings [site]="site()" />',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteRetentionSettingsPage {
    protected readonly site = inject(SiteService).activeSite;
}
