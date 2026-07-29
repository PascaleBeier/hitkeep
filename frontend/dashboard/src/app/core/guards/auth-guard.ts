import { HttpErrorResponse } from '@angular/common/http';
import { inject } from '@angular/core';
import { CanActivateFn } from '@angular/router';
import { catchError, map, of } from 'rxjs';

import { AuthService } from '@services/auth.service';
import { ApplicationErrorNavigationService } from '@services/application-error-navigation.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { SessionEndNavigationService } from '@services/session-end-navigation.service';

export const authGuard: CanActivateFn = (_route, state) => {
    if (state.url.startsWith('/share/')) {
        return true;
    }

    const auth = inject(AuthService);
    const bootstrap = inject(DashboardBootstrapService);
    const applicationErrors = inject(ApplicationErrorNavigationService);
    const sessionEndNavigation = inject(SessionEndNavigationService);

    return bootstrap.load().pipe(
        map(() => true),
        catchError((error: unknown) => {
            if (error instanceof HttpErrorResponse && error.status === 401) {
                auth.markUnauthenticated();
                return of(sessionEndNavigation.redirectToLogin('session-ended', state.url));
            }
            return of(applicationErrors.fromHttp(error, 'dashboard-bootstrap', state.url));
        })
    );
};
