import { TemplateRef, computed, DestroyRef, inject, signal, Service } from '@angular/core';
import { takeUntilDestroyed, toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router } from '@angular/router';
import { catchError, filter, finalize, forkJoin, map, of, startWith, switchMap, tap } from 'rxjs';
import { NavigationNoticeService } from '@services/navigation-notice.service';
import { Site, Team } from '@models/analytics.types';
import { MenuItem } from '@openng/optimus-ui/api';
import { TranslocoService } from '@jsverse/transloco';
import { injectActiveLang } from '@core/i18n/active-lang';
import { SITE_CAPABILITIES, TEAM_CAPABILITIES } from '@core/access/capabilities';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { AccessService } from '@services/access.service';
import { PermissionService } from '@services/permission.service';
import { ShareService } from '@services/share.service';
import { TeamService } from '@services/team.service';
import type { SiteSettingsSection } from '@features/sites/site-settings-section';
import { SiteService } from '@features/sites/services/site.service';

@Service({ autoProvided: false })
export class MainLayoutContextService {
    private readonly router = inject(Router);
    private readonly destroyRef = inject(DestroyRef);
    readonly siteService = inject(SiteService);
    readonly shareService = inject(ShareService);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = injectActiveLang();
    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly access = inject(AccessService);
    readonly teamService = inject(TeamService);
    readonly perms = inject(PermissionService);

    readonly cloudHosted = this.bootstrap.cloudHosted;
    readonly cloudSupportUrl = this.bootstrap.cloudSupportUrl;
    // Self-hosted deployments have no plan limits; managed cloud trusts the
    // server-derived flag instead of re-deriving policy in the frontend.
    readonly canCreateTeams = computed(() => !this.cloudHosted() || this.perms.canCreateTeams());
    readonly isTeamAdmin = computed(() => this.access.canActiveTeam(TEAM_CAPABILITIES.manageSettings));

    private readonly navigationNotice = inject(NavigationNoticeService);
    readonly isTeamSwitchPending = signal(false);
    readonly isContextPending = computed(() => this.isTeamSwitchPending() || this.siteService.isLoading());
    readonly canOpenSiteSettings = computed(() => !this.shareService.isShareMode() && !!this.siteService.activeSite() && this.access.canActiveSite(SITE_CAPABILITIES.view));
    readonly siteActions = computed<MenuItem[]>(() => {
        this.activeLanguage();
        if (this.shareService.isShareMode() || this.isContextPending()) return [];
        const items: MenuItem[] = [];
        if (this.canOpenSiteSettings()) items.push({ label: this.transloco.translate('sites.settings.title'), icon: 'pi pi-cog', command: () => this.openSiteSettings() });
        if (this.access.canActiveSite(SITE_CAPABILITIES.manageTeam)) items.push({ label: this.transloco.translate('share.dialog.title'), icon: 'pi pi-share-alt', command: () => this.openSiteSettings('share') });
        items.push({ label: this.transloco.translate('overview.actions.addSite'), icon: 'pi pi-plus', command: () => this.openAddSite() });
        return items;
    });
    readonly isMobileDrawerOpen = signal(false);
    private readonly sidebarPreference = this.readSidebarPreference();
    readonly isSidebarCollapsed = signal(this.sidebarPreference.collapsed);
    readonly sidebarWidthPx = signal(this.sidebarPreference.widthPx);
    readonly isAddSiteVisible = signal(false);
    readonly isCreateTeamVisible = signal(false);
    readonly pageHeaderLeft = signal<TemplateRef<unknown> | null>(null);
    readonly pageHeaderRight = signal<TemplateRef<unknown> | null>(null);
    readonly hasPageHeader = computed(() => this.pageHeaderLeft() !== null);
    readonly isOverviewPage = computed(() => {
        const url = this.activeUrl().split(/[?#]/, 1)[0] || '/';
        return url === '/overview' || url.startsWith('/overview/');
    });

    readonly beforeTeamSwitch = () => !this.isContextPending();
    private pageHeaderOwner: symbol | null = null;
    private readonly activeUrl = toSignal(
        this.router.events.pipe(
            filter((event): event is NavigationEnd => event instanceof NavigationEnd),
            map((event) => event.urlAfterRedirects),
            startWith(this.router.url)
        ),
        { initialValue: this.router.url }
    );

    toggleSidebar(): void {
        this.isSidebarCollapsed.update((collapsed) => !collapsed);
        this.storeSidebarPreference();
    }

    setSidebarWidthPx(widthPx: number): void {
        this.sidebarWidthPx.set(Number.isFinite(widthPx) ? Math.min(320, Math.max(220, Math.round(widthPx))) : 240);
        this.storeSidebarPreference();
    }

    private readSidebarPreference(): { collapsed: boolean; widthPx: number } {
        try {
            const value = JSON.parse(localStorage.getItem('hk_sidebar_layout') ?? 'null');
            if (value?.version === 1 && typeof value.widthPx === 'number' && Number.isFinite(value.widthPx) && typeof value.collapsed === 'boolean') {
                return { collapsed: value.collapsed, widthPx: Math.min(320, Math.max(220, Math.round(value.widthPx))) };
            }
        } catch {
            // Navigation remains usable when browser storage is unavailable.
        }
        return { collapsed: false, widthPx: 240 };
    }

    private storeSidebarPreference(): void {
        try {
            localStorage.setItem('hk_sidebar_layout', JSON.stringify({ version: 1, collapsed: this.isSidebarCollapsed(), widthPx: this.sidebarWidthPx() }));
        } catch {
            // Keep the in-memory preference when storage is blocked or full.
        }
    }

    openSiteSettings(section: SiteSettingsSection = 'general') {
        const site = this.siteService.activeSite();
        if (!site || !this.canOpenSiteSettings() || this.isContextPending()) return;
        if (section === 'share' && !this.access.canActiveSite(SITE_CAPABILITIES.manageTeam)) return;
        void this.router.navigate(['/sites', site.id, 'settings', section]);
    }

    openAddSite(): void {
        if (!this.shareService.isShareMode() && !this.isContextPending()) this.isAddSiteVisible.set(true);
    }

    onSiteSelected(site: Site) {
        if (this.shareService.isShareMode() || this.isContextPending()) return;
        const selected = this.siteService.sites().find((candidate) => candidate.id === site.id);
        if (!selected) return;
        this.siteService.selectSite(selected);
        const section = this.currentSiteSettingsSection();
        if (section) {
            void this.router.navigate(['/sites', site.id, 'settings', section]);
        } else if (this.router.url.split(/[?#]/, 1)[0] === '/overview') {
            void this.router.navigate(['/dashboard']);
        }
    }

    onTeamSelected(team: Team): void {
        if (this.shareService.isShareMode() || this.isContextPending() || team.id === this.teamService.activeTeamId()) return;
        this.isTeamSwitchPending.set(true);
        void this.router.navigate(['/overview']);
        this.teamService
            .setActiveTeam(team.id)
            .pipe(
                tap(() => {
                    this.siteService.sites.set([]);
                    this.siteService.activeSite.set(null);
                    this.perms.permissions.set(null);
                }),
                switchMap(() =>
                    forkJoin({
                        teams: this.teamService.loadTeams().pipe(catchError(() => of(null))),
                        permissions: this.perms.loadPermissions().pipe(catchError(() => of(null)))
                    })
                ),
                tap(() => {
                    this.siteService.loadSites();
                    this.redirectIfTeamAdminAccessWasLost();
                }),
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isTeamSwitchPending.set(false))
            )
            .subscribe({ error: () => this.navigationNotice.show('teams.switcher.switchError', { preserveNextNavigation: true }) });
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
        const currentURL = this.router.routerState.snapshot.url;
        if ((currentURL.startsWith('/admin/team') || currentURL.startsWith('/integration/google-search-console')) && !this.isTeamAdmin()) {
            this.router.navigateByUrl('/dashboard');
        }
    }

    private currentSiteSettingsSection(): SiteSettingsSection | null {
        const match = this.router.url.match(/^\/sites\/[^/]+\/settings\/(general|tracking|filtering|retention|access|share|danger-zone)(?:[/?#]|$)/);
        return (match?.[1] as SiteSettingsSection | undefined) ?? null;
    }
}
