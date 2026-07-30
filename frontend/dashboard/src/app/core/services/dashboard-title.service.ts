import { effect, inject, Service } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { Title } from '@angular/platform-browser';
import { ActivatedRouteSnapshot, NavigationEnd, Router } from '@angular/router';
import { TranslocoService } from '@jsverse/transloco';
import { filter, map, startWith } from 'rxjs';

import { injectActiveLang } from '@core/i18n/active-lang';
import { ShareService } from '@services/share.service';
import { TeamService } from '@services/team.service';
import { SiteService } from '@features/sites/services/site.service';

export type DashboardTitleScope = 'none' | 'site' | 'team';

export interface DashboardTitleRouteData {
    titleKey?: string;
    titleScope?: DashboardTitleScope;
}

@Service()
export class DashboardTitleService {
    private readonly router = inject(Router);
    private readonly title = inject(Title);
    private readonly transloco = inject(TranslocoService);
    private readonly siteService = inject(SiteService);
    private readonly shareService = inject(ShareService);
    private readonly teamService = inject(TeamService);
    private readonly activeLanguage = injectActiveLang();
    private readonly activeUrl = toSignal(
        this.router.events.pipe(
            filter((event): event is NavigationEnd => event instanceof NavigationEnd),
            map((event) => event.urlAfterRedirects),
            startWith(this.router.url)
        ),
        { initialValue: this.router.url }
    );

    constructor() {
        effect(() => {
            this.activeUrl();
            this.activeLanguage();
            const routeTitle = this.currentRouteTitle();
            const pageTitle = routeTitle.titleKey ? this.transloco.translate(routeTitle.titleKey) : '';
            const contextTitle = this.contextTitle(routeTitle.titleScope ?? 'none');

            this.title.setTitle(this.composeTitle(contextTitle, pageTitle));
        });
    }

    private currentRouteTitle(): DashboardTitleRouteData {
        let route: ActivatedRouteSnapshot | null = this.router.routerState.snapshot.root;
        let titleData: DashboardTitleRouteData = {};

        while (route) {
            const data = route.data as DashboardTitleRouteData;
            if (data.titleKey) {
                titleData = data;
            }
            route = route.firstChild;
        }

        return titleData;
    }

    private contextTitle(scope: DashboardTitleScope): string {
        if (scope === 'site') {
            const site = this.shareService.isShareMode() ? this.shareService.site() : this.siteService.activeSite();
            return site?.domain?.trim() ?? '';
        }

        if (scope === 'team') {
            return this.teamService.activeTeam()?.name?.trim() ?? '';
        }

        return '';
    }

    private composeTitle(contextTitle: string, pageTitle: string): string {
        return [contextTitle, pageTitle, 'HitKeep']
            .map((part) => part.trim())
            .filter(Boolean)
            .join(' - ');
    }
}
