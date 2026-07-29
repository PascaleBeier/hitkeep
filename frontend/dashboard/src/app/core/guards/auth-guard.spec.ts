import { HttpErrorResponse } from '@angular/common/http';
import { TestBed } from '@angular/core/testing';
import { CanActivateFn, RedirectCommand, Router, provideRouter } from '@angular/router';
import { Observable, firstValueFrom, of, throwError } from 'rxjs';
import { vi } from 'vitest';

import { authGuard } from '@guards/auth-guard';
import { APPLICATION_ERROR_STATE_KEY } from '@services/application-error-navigation.service';
import { AuthService } from '@services/auth.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';

describe('authGuard', () => {
    const executeGuard: CanActivateFn = (...guardParameters) => TestBed.runInInjectionContext(() => authGuard(...guardParameters));
    const auth = { markUnauthenticated: vi.fn() };
    const bootstrap = { load: vi.fn<() => Observable<unknown>>() };

    beforeEach(() => {
        auth.markUnauthenticated.mockReset();
        bootstrap.load.mockReset();
        TestBed.configureTestingModule({
            providers: [provideRouter([]), { provide: AuthService, useValue: auth }, { provide: DashboardBootstrapService, useValue: bootstrap }]
        });
        vi.spyOn(console, 'error').mockImplementation(() => undefined);
    });

    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('allows shared dashboard routes without probing the user session', () => {
        expect(executeGuard({} as never, { url: '/share/public-token/dashboard' } as never)).toBe(true);
        expect(bootstrap.load).not.toHaveBeenCalled();
    });

    it('waits for dashboard bootstrap before activating protected routes', async () => {
        bootstrap.load.mockReturnValue(of({}));

        expect(await firstValueFrom(executeGuard({} as never, { url: '/dashboard' } as never) as Observable<boolean>)).toBe(true);
        expect(auth.markUnauthenticated).not.toHaveBeenCalled();
    });

    it('redirects an unauthenticated bootstrap to login with a return URL', async () => {
        bootstrap.load.mockReturnValue(throwError(() => new HttpErrorResponse({ status: 401 })));

        const result = await firstValueFrom(executeGuard({} as never, { url: '/dashboard?range=7d' } as never) as Observable<unknown>);

        expect(result).toBeInstanceOf(RedirectCommand);
        expect(TestBed.inject(Router).serializeUrl((result as RedirectCommand).redirectTo)).toBe('/login?returnUrl=%2Fdashboard%3Frange%3D7d');
        expect(auth.markUnauthenticated).toHaveBeenCalledTimes(1);
    });

    it('keeps a server outage distinct from an authentication failure', async () => {
        bootstrap.load.mockReturnValue(throwError(() => new HttpErrorResponse({ status: 502, error: 'private response' })));

        const command = (await firstValueFrom(executeGuard({} as never, { url: '/dashboard' } as never) as Observable<unknown>)) as RedirectCommand;

        expect(command).toBeInstanceOf(RedirectCommand);
        expect(command.navigationBehaviorOptions?.state?.[APPLICATION_ERROR_STATE_KEY]).toEqual({
            version: 1,
            kind: 'server',
            context: 'dashboard-bootstrap',
            returnUrl: '/dashboard',
            status: 502
        });
        expect(auth.markUnauthenticated).not.toHaveBeenCalled();
    });
});
