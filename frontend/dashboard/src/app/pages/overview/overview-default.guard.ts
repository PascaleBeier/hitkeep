import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { SiteService } from '@features/sites/services/site.service';

export const overviewDefaultGuard: CanActivateFn = () => {
    const router = inject(Router);
    const sites = inject(SiteService).sites();
    return router.parseUrl(sites.length > 1 ? '/overview' : '/dashboard');
};
