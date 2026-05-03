import { TestBed } from '@angular/core/testing';
import { CanActivateFn, Router } from '@angular/router';
import { AuthService } from '@services/auth.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { Observable, firstValueFrom, of, throwError } from 'rxjs';
import { vi } from 'vitest';

import { authGuard } from '@guards/auth-guard';

describe('authGuard', () => {
    const executeGuard: CanActivateFn = (...guardParameters) => TestBed.runInInjectionContext(() => authGuard(...guardParameters));
    const auth = {
        markUnauthenticated: vi.fn()
    };
    const bootstrap = {
        load: vi.fn<() => Observable<unknown>>()
    };
    const router = {
        createUrlTree: vi.fn((commands: string[], options?: unknown) => ({ commands, options }))
    };

    beforeEach(() => {
        auth.markUnauthenticated.mockReset();
        bootstrap.load.mockReset();
        router.createUrlTree.mockClear();

        TestBed.configureTestingModule({
            providers: [
                { provide: AuthService, useValue: auth },
                { provide: DashboardBootstrapService, useValue: bootstrap },
                { provide: Router, useValue: router }
            ]
        });
    });

    it('allows shared dashboard routes without probing the user session', () => {
        const result = executeGuard({} as never, { url: '/share/public-token/dashboard' } as never);

        expect(result).toBe(true);
        expect(bootstrap.load).not.toHaveBeenCalled();
    });

    it('waits for dashboard bootstrap before activating protected routes', async () => {
        bootstrap.load.mockReturnValue(of({}));

        const result = executeGuard({} as never, { url: '/dashboard' } as never) as Observable<boolean>;
        expect(await firstValueFrom(result)).toBe(true);
        expect(auth.markUnauthenticated).not.toHaveBeenCalled();
    });

    it('redirects failed protected bootstraps to login with a return URL', async () => {
        bootstrap.load.mockReturnValue(throwError(() => new Error('unauthenticated')));

        const result = executeGuard({} as never, { url: '/dashboard?range=7d' } as never) as Observable<unknown>;
        expect(await firstValueFrom(result)).toEqual({
            commands: ['/login'],
            options: { queryParams: { returnUrl: '/dashboard?range=7d' } }
        });
        expect(auth.markUnauthenticated).toHaveBeenCalled();
        expect(router.createUrlTree).toHaveBeenCalledWith(['/login'], {
            queryParams: { returnUrl: '/dashboard?range=7d' }
        });
    });
});
