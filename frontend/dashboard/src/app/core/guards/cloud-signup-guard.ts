import { HttpClient, HttpContext, HttpErrorResponse } from '@angular/common/http';
import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { catchError, map, of, switchMap, timeout } from 'rxjs';

import { SKIP_AUTH_REDIRECT } from '@core/interceptors/auth.interceptor';
import { SystemStatus } from '@models/analytics.types';
import { ApplicationErrorNavigationService, ROUTE_CRITICAL_REQUEST_TIMEOUT_MS } from '@services/application-error-navigation.service';
import { AuthService } from '@services/auth.service';

export const cloudSignupGuard: CanActivateFn = (_route, state) => {
    const http = inject(HttpClient);
    const router = inject(Router);
    const auth = inject(AuthService);
    const applicationErrors = inject(ApplicationErrorNavigationService);
    const context = new HttpContext().set(SKIP_AUTH_REDIRECT, true);

    return http.get<SystemStatus>('/api/status', { context }).pipe(
        timeout({ first: ROUTE_CRITICAL_REQUEST_TIMEOUT_MS }),
        switchMap((status) => {
            if (status.cloud?.hosted && status.cloud.signup_enabled) {
                return http.get('/api/user/profile', { context }).pipe(
                    timeout({ first: ROUTE_CRITICAL_REQUEST_TIMEOUT_MS }),
                    map(() => {
                        auth.markAuthenticated();
                        return router.createUrlTree(['/dashboard']);
                    }),
                    catchError((error: unknown) => {
                        if (error instanceof HttpErrorResponse && error.status === 401) {
                            auth.markUnauthenticated();
                            return of(true);
                        }
                        return of(applicationErrors.fromHttp(error, 'cloud-signup-profile', state.url));
                    })
                );
            }
            if (status.needs_setup) {
                return of(router.createUrlTree(['/setup']));
            }
            return of(router.createUrlTree(['/login']));
        }),
        catchError((error: unknown) => of(applicationErrors.fromHttp(error, 'cloud-signup-status', state.url)))
    );
};
