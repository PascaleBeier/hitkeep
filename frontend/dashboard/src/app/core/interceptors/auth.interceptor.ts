import { HttpContextToken, HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { EMPTY, catchError, throwError } from 'rxjs';
import { AuthService } from '@services/auth.service';
import { SessionEndNavigationService } from '@services/session-end-navigation.service';
import { ShareService } from '@services/share.service';

export { resolveCurrentReturnUrl, shouldRedirectAfterUnauthorized } from '@services/session-end-navigation.service';

export const SKIP_AUTH_REDIRECT = new HttpContextToken<boolean>(() => false);

export const authInterceptor: HttpInterceptorFn = (req, next) => {
    const auth = inject(AuthService);
    const sessionEndNavigation = inject(SessionEndNavigationService);
    const share = inject(ShareService);
    const isAuthRequest =
        req.url.startsWith('/api/login') || req.url.startsWith('/api/logout') || req.url.startsWith('/api/initial-user') || req.url.startsWith('/api/auth/') || req.url.startsWith('/api/cloud/') || req.url.startsWith('/api/user/password');

    // We clone the request to ensure credentials (cookies) are included.
    // This ensures the http-only cookie is sent to the backend.
    const authReq = req.clone({
        withCredentials: true
    });

    return next(authReq).pipe(
        catchError((error: HttpErrorResponse) => {
            if (req.context.get(SKIP_AUTH_REDIRECT)) {
                return throwError(() => error);
            }

            // If we receive a 401 Unauthorized, it means the cookie is missing or invalid.
            if (error.status === 401 && !share.isShareMode() && !isAuthRequest) {
                auth.markUnauthenticated();
                sessionEndNavigation.navigateToLogin('session-ended');

                // Complete the request stream so late 401s do not crash screens with
                // subscriptions that omit explicit error handlers.
                return EMPTY;
            }
            return throwError(() => error);
        })
    );
};
