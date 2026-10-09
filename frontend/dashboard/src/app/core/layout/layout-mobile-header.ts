import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import { Brand } from '@components/brand/brand';
import { UserControls } from '@components/user-controls/user-controls';
import { SiteSelector } from '@features/sites/components/site-selector';
import { MainLayoutContextService } from '@layout/main-layout-context.service';

@Component({
    selector: 'app-layout-mobile-header',
    imports: [Brand, UserControls, SiteSelector, TranslocoPipe],
    templateUrl: './layout-mobile-header.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class LayoutMobileHeader {
    protected readonly context = inject(MainLayoutContextService);
    protected readonly shareService = this.context.shareService;
    protected readonly siteService = this.context.siteService;
    protected readonly isMobileDrawerOpen = this.context.isMobileDrawerOpen;
}
