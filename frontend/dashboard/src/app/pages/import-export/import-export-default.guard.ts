import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { PermissionService } from '@services/permission.service';
import { SiteService } from '@features/sites/services/site.service';

export const importExportDefaultGuard: CanActivateFn = () => {
    const router = inject(Router);
    const siteService = inject(SiteService);
    const permissions = inject(PermissionService);
    const activeSite = siteService.activeSite();
    const defaultTab = activeSite && permissions.canManageSite(activeSite.id) ? 'import' : 'export';

    return router.parseUrl(`/import-export/${defaultTab}`);
};
