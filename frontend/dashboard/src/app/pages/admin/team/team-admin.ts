import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, NavigationEnd, Router, RouterLink, RouterOutlet } from '@angular/router';
import { TranslocoService } from '@jsverse/transloco';
import { filter, map } from 'rxjs';
import { TabsModule } from 'primeng/tabs';
import { PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { TEAM_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';
import { TeamService } from '@services/team.service';
import { AdminPageFrame } from '../components/admin-page-frame';

interface TeamAdminTab {
    label: string;
    route: string;
    visible: boolean;
    icon?: string;
    danger?: boolean;
}

@Component({
    selector: 'app-team-admin',
    imports: [TabsModule, RouterLink, RouterOutlet, AdminPageFrame],
    templateUrl: './team-admin.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TeamAdminPage {
    private readonly transloco = inject(TranslocoService);
    private readonly teamService = inject(TeamService);
    private readonly access = inject(AccessService);
    private readonly route = inject(ActivatedRoute);
    private readonly router = inject(Router);
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    protected readonly activeTeam = this.teamService.activeTeam;
    protected readonly activeTab = toSignal(
        this.router.events.pipe(
            filter((event): event is NavigationEnd => event instanceof NavigationEnd),
            map(() => this.activeChildSegment())
        ),
        { initialValue: this.activeChildSegment() }
    );

    protected readonly canManageSettings = computed(() => this.access.canActiveTeam(TEAM_CAPABILITIES.manageSettings));
    protected readonly canViewAudit = computed(() => this.access.canActiveTeam(TEAM_CAPABILITIES.viewAudit));
    protected readonly tabs = computed<TeamAdminTab[]>(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('admin.team.tabs.overview'), route: 'overview', visible: true },
            { label: this.transloco.translate('admin.team.tabs.members'), route: 'members', visible: true },
            { label: this.transloco.translate('admin.team.tabs.apiClients'), route: 'api-clients', visible: this.canManageSettings() },
            { label: this.transloco.translate('admin.team.tabs.customDomains'), route: 'custom-domains', visible: this.canManageSettings() },
            { label: this.transloco.translate('admin.team.tabs.branding'), route: 'branding', visible: this.canManageSettings() },
            { label: this.transloco.translate('admin.team.tabs.activity'), route: 'activity', visible: this.canViewAudit() },
            { label: this.transloco.translate('admin.team.tabs.dangerZone'), route: 'danger-zone', visible: this.canManageSettings(), icon: 'pi pi-exclamation-triangle', danger: true }
        ].filter((tab) => tab.visible);
    });

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        return [{ label: this.transloco.translate('nav.administration') }, { label: this.transloco.translate('nav.team'), isCurrent: true }];
    });

    private activeChildSegment(): string {
        return this.route.snapshot.firstChild?.routeConfig?.path ?? 'overview';
    }
}
