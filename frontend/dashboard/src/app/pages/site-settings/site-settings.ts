import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, NavigationEnd, Router, RouterLink, RouterOutlet } from '@angular/router';
import { TranslocoService } from '@jsverse/transloco';
import { BreadcrumbModule } from '@openng/optimus-ui/breadcrumb';
import { MenuItem } from '@openng/optimus-ui/api';
import { TabsModule } from '@openng/optimus-ui/tabs';
import { filter, map } from 'rxjs';

import { PageFrame } from '@components/page-frame/page-frame';
import { INSTANCE_CAPABILITIES, SITE_CAPABILITIES } from '@core/access/capabilities';
import { injectActiveLang } from '@core/i18n/active-lang';
import type { SiteSettingsSection } from '@features/sites/site-settings-section';
import { SiteService } from '@features/sites/services/site.service';
import { AccessService } from '@services/access.service';

interface SiteSettingsTab {
    label: string;
    route: SiteSettingsSection;
    icon: string;
    visible: boolean;
    danger?: boolean;
}

@Component({
    selector: 'app-site-settings-page',
    imports: [BreadcrumbModule, PageFrame, TabsModule, RouterLink, RouterOutlet],
    templateUrl: './site-settings.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteSettingsPage {
    private readonly route = inject(ActivatedRoute);
    private readonly router = inject(Router);
    private readonly transloco = inject(TranslocoService);
    private readonly access = inject(AccessService);
    private readonly activeLanguage = injectActiveLang();
    protected readonly site = inject(SiteService).activeSite;

    protected readonly activeTab = toSignal(
        this.router.events.pipe(
            filter((event): event is NavigationEnd => event instanceof NavigationEnd),
            map(() => this.activeChildSegment())
        ),
        { initialValue: this.activeChildSegment() }
    );

    protected readonly tabs = computed<SiteSettingsTab[]>(() => {
        this.activeLanguage();
        const site = this.site();
        if (!site) return [];
        const tabs: SiteSettingsTab[] = [
            { label: this.transloco.translate('sites.settings.tabs.general'), route: 'general', icon: 'pi pi-cog', visible: true },
            { label: this.transloco.translate('sites.settings.tabs.tracking'), route: 'tracking', icon: 'pi pi-code', visible: true },
            {
                label: this.transloco.translate('sites.settings.tabs.filtering'),
                route: 'filtering',
                icon: 'pi pi-filter-slash',
                visible: this.access.canSite(site.id, SITE_CAPABILITIES.manageData) || this.access.hasInstance(INSTANCE_CAPABILITIES.manageSiteExclusions)
            },
            { label: this.transloco.translate('sites.settings.tabs.retention'), route: 'retention', icon: 'pi pi-history', visible: this.access.canSite(site.id, SITE_CAPABILITIES.manageData) },
            { label: this.transloco.translate('sites.settings.tabs.access'), route: 'access', icon: 'pi pi-users', visible: this.access.canSite(site.id, SITE_CAPABILITIES.manageTeam) },
            { label: this.transloco.translate('sites.settings.tabs.share'), route: 'share', icon: 'pi pi-share-alt', visible: this.access.canSite(site.id, SITE_CAPABILITIES.manageTeam) },
            {
                label: this.transloco.translate('sites.settings.tabs.dangerZone'),
                route: 'danger-zone',
                icon: 'pi pi-exclamation-triangle',
                visible: this.access.canSite(site.id, SITE_CAPABILITIES.delete),
                danger: true
            }
        ];
        return tabs.filter((tab) => tab.visible);
    });

    protected readonly breadcrumbs = computed<MenuItem[]>(() => {
        this.activeLanguage();
        return [{ label: this.transloco.translate('sites.settings.title'), routerLink: ['/sites', this.site()?.id, 'settings', 'general'] }, { label: this.tabs().find((tab) => tab.route === this.activeTab())?.label ?? '' }];
    });

    private activeChildSegment(): SiteSettingsSection {
        return (this.route.snapshot.firstChild?.routeConfig?.path as SiteSettingsSection | undefined) ?? 'general';
    }
}
