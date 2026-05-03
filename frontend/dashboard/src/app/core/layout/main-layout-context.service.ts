import { Injectable, TemplateRef, computed, effect, inject, signal } from '@angular/core';
import { Router } from '@angular/router';
import { TranslocoService } from '@jsverse/transloco';
import { Team } from '@models/analytics.types';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { PermissionService } from '@services/permission.service';
import { ShareService } from '@services/share.service';
import { SiteSettingsService } from '@services/site-settings.service';
import { TeamService } from '@services/team.service';
import { SiteService } from '@features/sites/services/site.service';

@Injectable()
export class MainLayoutContextService {
    private readonly router = inject(Router);
    readonly siteService = inject(SiteService);
    readonly shareService = inject(ShareService);
    private readonly siteSettings = inject(SiteSettingsService);
    private readonly bootstrap = inject(DashboardBootstrapService);
    readonly teamService = inject(TeamService);
    readonly perms = inject(PermissionService);
    private readonly transloco = inject(TranslocoService);

    readonly cloudHosted = this.bootstrap.cloudHosted;
    readonly cloudSupportUrl = this.bootstrap.cloudSupportUrl;
    readonly canCreateTeams = computed(() => !this.cloudHosted());
    readonly isTeamAdmin = computed(() => {
        const role = this.teamService.activeTeam()?.role;
        return role === 'owner' || role === 'admin';
    });

    readonly isMobileDrawerOpen = signal(false);
    readonly isAddSiteVisible = signal(false);
    readonly isCreateTeamVisible = signal(false);
    readonly isSiteSettingsVisible = signal(false);
    readonly siteSettingsTab = signal('0');
    readonly pageHeaderLeft = signal<TemplateRef<unknown> | null>(null);
    readonly pageHeaderRight = signal<TemplateRef<unknown> | null>(null);
    readonly hasPageHeader = computed(() => this.pageHeaderLeft() !== null);

    readonly beforeTeamSwitch = () => {
        if (!this.isSiteSettingsVisible()) {
            return true;
        }
        const proceed = window.confirm(this.transloco.translate('sites.settings.unsavedChangesConfirm'));
        if (!proceed) {
            return false;
        }
        this.isSiteSettingsVisible.set(false);
        return true;
    };

    private initialized = false;
    private pageHeaderOwner: symbol | null = null;

    init() {
        if (this.initialized) {
            return;
        }
        this.initialized = true;

        effect(() => {
            const tab = this.siteSettings.request();
            if (!tab) {
                return;
            }
            this.openSiteSettings(tab);
            this.siteSettings.clear();
        });
    }

    openSiteSettings(tab = '0') {
        if (this.siteService.activeSite()) {
            this.siteSettingsTab.set(tab);
            this.isSiteSettingsVisible.set(true);
        }
    }

    onTeamSelected(team: Team) {
        this.teamService.setActiveTeam(team.id).subscribe({
            next: () => {
                this.siteService.sites.set([]);
                this.siteService.activeSite.set(null);
                this.teamService.loadTeams().subscribe({
                    next: () => {
                        this.siteService.loadSites();
                        this.perms.loadPermissions().subscribe({
                            next: () => this.redirectIfTeamAdminAccessWasLost(),
                            error: () => this.redirectIfTeamAdminAccessWasLost()
                        });
                    },
                    error: () => {
                        this.siteService.loadSites();
                        this.perms.loadPermissions().subscribe({
                            next: () => this.redirectIfTeamAdminAccessWasLost(),
                            error: () => this.redirectIfTeamAdminAccessWasLost()
                        });
                    }
                });
            },
            error: () => undefined
        });
    }

    registerPageHeader(owner: symbol, left: TemplateRef<unknown> | null, right: TemplateRef<unknown> | null) {
        this.pageHeaderOwner = owner;
        this.pageHeaderLeft.set(left);
        this.pageHeaderRight.set(right);
    }

    clearPageHeader(owner: symbol) {
        if (this.pageHeaderOwner !== owner) {
            return;
        }

        this.pageHeaderOwner = null;
        this.pageHeaderLeft.set(null);
        this.pageHeaderRight.set(null);
    }

    private redirectIfTeamAdminAccessWasLost() {
        if (this.router.routerState.snapshot.url.startsWith('/admin/team') && !this.isTeamAdmin()) {
            this.router.navigateByUrl('/dashboard');
        }
    }
}
