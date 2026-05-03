import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { catchError, map, of } from 'rxjs';

import { AuthService } from '@services/auth.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';

export const authGuard: CanActivateFn = (_route, state) => {
    if (state.url.startsWith('/share/')) {
        return true;
    }

    const auth = inject(AuthService);
    const bootstrap = inject(DashboardBootstrapService);
    const router = inject(Router);

    return bootstrap.load().pipe(
        map(() => true),
        catchError(() => {
            auth.markUnauthenticated();
            return of(
                router.createUrlTree(['/login'], {
                    queryParams: { returnUrl: state.url }
                })
            );
        })
    );
};
