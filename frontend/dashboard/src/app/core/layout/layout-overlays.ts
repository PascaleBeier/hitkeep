import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { SessionExpiryIndicator } from '@components/session-expiry-indicator/session-expiry-indicator';
import { CreateTeamDialog } from '@components/create-team-dialog/create-team-dialog';
import { AddSiteDialog } from '@features/sites/components/add-site-dialog';
import { MainLayoutContextService } from '@layout/main-layout-context.service';

@Component({
    selector: 'app-layout-overlays',
    imports: [SessionExpiryIndicator, AddSiteDialog, CreateTeamDialog],
    templateUrl: './layout-overlays.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class LayoutOverlays {
    protected readonly context = inject(MainLayoutContextService);
    protected readonly siteService = this.context.siteService;
    protected readonly shareService = this.context.shareService;
    protected readonly isAddSiteVisible = this.context.isAddSiteVisible;
    protected readonly isCreateTeamVisible = this.context.isCreateTeamVisible;
}
