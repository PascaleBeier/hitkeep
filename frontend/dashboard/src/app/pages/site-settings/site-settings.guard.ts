import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';

import { INSTANCE_CAPABILITIES, SITE_CAPABILITIES } from '@core/access/capabilities';
import type { SiteSettingsSection } from '@features/sites/site-settings-section';
import { SiteService } from '@features/sites/services/site.service';
import { AccessService } from '@services/access.service';
import { NavigationNoticeService } from '@services/navigation-notice.service';

export const siteSettingsSiteGuard: CanActivateFn = (route) => {
    const sites = inject(SiteService);
    const router = inject(Router);
    const notice = inject(NavigationNoticeService);
    const siteID = route.paramMap.get('siteId');
    const site = sites.sites().find((candidate) => candidate.id === siteID);

    if (!site) {
        notice.show('sites.settings.notices.siteUnavailable', { preserveNextNavigation: true });
        return router.createUrlTree(['/overview']);
    }

    sites.selectSite(site);
    return true;
};

export const siteSettingsSectionGuard: CanActivateFn = (route) => {
    const sites = inject(SiteService);
    const access = inject(AccessService);
    const router = inject(Router);
    const notice = inject(NavigationNoticeService);
    const site = sites.activeSite();
    const section = route.data['siteSettingsSection'] as SiteSettingsSection | undefined;

    if (!site) {
        notice.show('sites.settings.notices.siteUnavailable', { preserveNextNavigation: true });
        return router.createUrlTree(['/overview']);
    }

    const allowed = (() => {
        switch (section) {
            case 'filtering':
                return access.canSite(site.id, SITE_CAPABILITIES.manageData) || access.hasInstance(INSTANCE_CAPABILITIES.manageSiteExclusions);
            case 'retention':
                return access.canSite(site.id, SITE_CAPABILITIES.manageData);
            case 'access':
                return access.canSite(site.id, SITE_CAPABILITIES.manageTeam);
            case 'danger-zone':
                return access.canSite(site.id, SITE_CAPABILITIES.delete);
            default:
                return true;
        }
    })();

    if (allowed) return true;

    notice.show('sites.settings.notices.sectionUnavailable', { preserveNextNavigation: true });
    return router.createUrlTree(['/sites', site.id, 'settings', 'general']);
};
