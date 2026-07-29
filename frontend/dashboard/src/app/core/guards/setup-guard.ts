import { HttpClient, HttpContext } from '@angular/common/http';
import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { catchError, map, of, timeout } from 'rxjs';

import { SKIP_AUTH_REDIRECT } from '@core/interceptors/auth.interceptor';
import { SystemStatus } from '@models/analytics.types';
import { ApplicationErrorNavigationService, ROUTE_CRITICAL_REQUEST_TIMEOUT_MS } from '@services/application-error-navigation.service';

export const setupGuard: CanActivateFn = (_route, state) => {
    const http = inject(HttpClient);
    const router = inject(Router);
    const applicationErrors = inject(ApplicationErrorNavigationService);

    // Check if we are already on the setup page to avoid redirect loops
    const isSetupRoute = state.url.startsWith('/setup');

    const context = new HttpContext().set(SKIP_AUTH_REDIRECT, true);
    return http.get<SystemStatus>('/api/status', { context }).pipe(
        timeout({ first: ROUTE_CRITICAL_REQUEST_TIMEOUT_MS }),
        map((status) => {
            if (status.needs_setup) {
                return isSetupRoute ? true : router.createUrlTree(['/setup']);
            }
            return isSetupRoute ? router.createUrlTree(['/login']) : true;
        }),
        catchError((error: unknown) => of(applicationErrors.fromHttp(error, 'setup-status', state.url)))
    );
};
