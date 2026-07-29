import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { CanActivateFn, RedirectCommand, Router, UrlTree, provideRouter } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { vi } from 'vitest';

import { SKIP_AUTH_REDIRECT } from '@core/interceptors/auth.interceptor';
import { setupGuard } from '@guards/setup-guard';
import { APPLICATION_ERROR_STATE_KEY, ROUTE_CRITICAL_REQUEST_TIMEOUT_MS } from '@services/application-error-navigation.service';

describe('setupGuard', () => {
    const executeGuard: CanActivateFn = (...guardParameters) => TestBed.runInInjectionContext(() => setupGuard(...guardParameters));

    beforeEach(() => {
        TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])] });
        vi.spyOn(console, 'error').mockImplementation(() => undefined);
    });

    afterEach(() => {
        TestBed.inject(HttpTestingController).verify();
        vi.restoreAllMocks();
    });

    it('uses the status response to route incomplete setup', async () => {
        const result = firstValueFrom(executeGuard({} as never, { url: '/dashboard' } as never) as never);
        const request = TestBed.inject(HttpTestingController).expectOne('/api/status');

        expect(request.request.context.get(SKIP_AUTH_REDIRECT)).toBe(true);
        request.flush({ needs_setup: true });

        expect(serialize(await result)).toBe('/setup');
    });

    it('turns a route-critical status failure into an application error redirect', async () => {
        const result = firstValueFrom(executeGuard({} as never, { url: '/login?returnUrl=%2Fdashboard' } as never) as never);
        TestBed.inject(HttpTestingController).expectOne('/api/status').flush('unavailable', { status: 503, statusText: 'Service Unavailable' });

        const command = (await result) as RedirectCommand;
        expect(command).toBeInstanceOf(RedirectCommand);
        expect(TestBed.inject(Router).serializeUrl(command.redirectTo)).toBe('/error');
        expect(command.navigationBehaviorOptions?.state?.[APPLICATION_ERROR_STATE_KEY]).toEqual({
            version: 1,
            kind: 'server',
            context: 'setup-status',
            returnUrl: '/login',
            status: 503
        });
    });

    it('turns a status request timeout into an offline application error redirect', async () => {
        vi.useFakeTimers();
        try {
            const result = firstValueFrom(executeGuard({} as never, { url: '/login' } as never) as never);
            TestBed.inject(HttpTestingController).expectOne('/api/status');

            await vi.advanceTimersByTimeAsync(ROUTE_CRITICAL_REQUEST_TIMEOUT_MS);

            const command = (await result) as RedirectCommand;
            expect(command.navigationBehaviorOptions?.state?.[APPLICATION_ERROR_STATE_KEY]).toEqual({
                version: 1,
                kind: 'offline',
                context: 'setup-status',
                returnUrl: '/login'
            });
        } finally {
            vi.useRealTimers();
        }
    });
});

function serialize(target: unknown): string {
    expect(target).toBeInstanceOf(UrlTree);
    return TestBed.inject(Router).serializeUrl(target as UrlTree);
}
