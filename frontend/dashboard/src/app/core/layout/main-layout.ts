import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { DrawerModule } from 'primeng/drawer';
import { Brand } from '@components/brand/brand';
import { TeamSwitcher } from '@components/team-switcher/team-switcher';
import { UserControls } from '@components/user-controls/user-controls';
import { SessionExpiryIndicator } from '@components/session-expiry-indicator/session-expiry-indicator';
import { AddSiteDialog } from '@features/sites/components/add-site-dialog';
import { SiteSettingsDrawer } from '@features/sites/components/site-settings-drawer';
import { SiteSelector } from '@features/sites/components/site-selector';
import { CreateTeamDialog } from '@components/create-team-dialog/create-team-dialog';
import { MainLayoutContextService } from '@layout/main-layout-context.service';

@Component({
    selector: 'app-main-layout',
    changeDetection: ChangeDetectionStrategy.OnPush,
    host: {
        '(document:keydown)': 'handleKeyboard($event)'
    },
    providers: [MainLayoutContextService],
    imports: [RouterOutlet, RouterLink, RouterLinkActive, NgTemplateOutlet, Brand, SiteSelector, TeamSwitcher, SessionExpiryIndicator, AddSiteDialog, CreateTeamDialog, SiteSettingsDrawer, UserControls, DrawerModule, TranslocoPipe],
    templateUrl: './main-layout.html',
    styleUrl: './main-layout.css'
})
export class MainLayout {
    private static readonly docsURL = 'https://hitkeep.com/guides/introduction/';
    private static readonly supportFallbackURL = 'https://hitkeep.com/support/help/';
    protected readonly context = inject(MainLayoutContextService);
    protected readonly siteService = this.context.siteService;
    protected readonly shareService = this.context.shareService;
    protected readonly teamService = this.context.teamService;
    protected readonly perms = this.context.perms;
    protected readonly cloudHosted = this.context.cloudHosted;
    protected readonly cloudSupportUrl = this.context.cloudSupportUrl;
    protected readonly canCreateTeams = computed(() => !this.cloudHosted());
    protected readonly navBase = computed(() => {
        const token = this.shareService.token();
        return token ? `/share/${token}` : '';
    });
    protected readonly docsUrl = MainLayout.docsURL;
    protected readonly supportUrl = computed(() => {
        if (!this.cloudHosted()) {
            return '';
        }

        return this.cloudSupportUrl() || MainLayout.supportFallbackURL;
    });
    protected readonly isTeamAdmin = this.context.isTeamAdmin;
    protected readonly isMobileDrawerOpen = this.context.isMobileDrawerOpen;
    protected readonly isAddSiteVisible = this.context.isAddSiteVisible;
    protected readonly isCreateTeamVisible = this.context.isCreateTeamVisible;
    protected readonly isSiteSettingsVisible = this.context.isSiteSettingsVisible;
    protected readonly siteSettingsTab = this.context.siteSettingsTab;
    protected readonly beforeTeamSwitch = this.context.beforeTeamSwitch;
    protected readonly canViewSystemStatus = this.perms.isInstanceAdmin;
    protected readonly canImportActiveSite = computed(() => {
        const site = this.siteService.activeSite();
        return !!site && this.perms.canManageSite(site.id);
    });
    protected readonly isAdministrationVisible = computed(() => this.canViewSystemStatus() || this.isTeamAdmin());

    handleKeyboard(event: KeyboardEvent) {
        if ((event.metaKey || event.ctrlKey) && event.key === 'k') {
            event.preventDefault();
            this.openSiteSettings();
        }
    }

    openSiteSettings(tab = '0') {
        this.context.openSiteSettings(tab);
    }

    constructor() {
        this.context.init();
    }
}
