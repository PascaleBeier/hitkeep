import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { MessageModule } from '@openng/optimus-ui/message';
import { FreePlanRetentionNotice } from '@layout/free-plan-retention-notice';
import { LayoutMobileHeader } from '@layout/layout-mobile-header';
import { LayoutOverlays } from '@layout/layout-overlays';
import { LayoutPageBar } from '@layout/layout-page-bar';
import { LayoutSidebar } from '@layout/layout-sidebar';
import { MainLayoutContextService } from '@layout/main-layout-context.service';
import { SidebarMenuService } from '@layout/sidebar-menu.service';
import type { SiteSettingsSection } from '@features/sites/site-settings-section';
import { NavigationNoticeService } from '@services/navigation-notice.service';

@Component({
    selector: 'app-main-layout',
    changeDetection: ChangeDetectionStrategy.OnPush,
    host: {
        '(document:keydown)': 'handleKeyboard($event)'
    },
    providers: [MainLayoutContextService, SidebarMenuService],
    imports: [RouterOutlet, LayoutSidebar, LayoutMobileHeader, LayoutPageBar, LayoutOverlays, FreePlanRetentionNotice, MessageModule, TranslocoPipe],
    templateUrl: './main-layout.html',
    styleUrl: './main-layout.css'
})
export class MainLayout {
    protected readonly context = inject(MainLayoutContextService);
    protected readonly navigationNotice = inject(NavigationNoticeService);

    handleKeyboard(event: KeyboardEvent) {
        if ((event.metaKey || event.ctrlKey) && event.key === 'k') {
            event.preventDefault();
            this.openSiteSettings();
        }
    }

    openSiteSettings(section: SiteSettingsSection = 'general') {
        this.context.openSiteSettings(section);
    }
}
