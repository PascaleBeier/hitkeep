import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { CanActivateFn, RedirectCommand, Router, UrlTree, provideRouter } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { vi } from 'vitest';

import { cloudSignupGuard } from '@guards/cloud-signup-guard';
import { SKIP_AUTH_REDIRECT } from '@core/interceptors/auth.interceptor';
import { APPLICATION_ERROR_STATE_KEY } from '@services/application-error-navigation.service';
import { AuthService } from '@services/auth.service';

describe('cloudSignupGuard', () => {
    const executeGuard: CanActivateFn = (...guardParameters) => TestBed.runInInjectionContext(() => cloudSignupGuard(...guardParameters));
    const auth = { markAuthenticated: vi.fn(), markUnauthenticated: vi.fn() };

    beforeEach(() => {
        vi.clearAllMocks();
        TestBed.configureTestingModule({
            providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([]), { provide: AuthService, useValue: auth }]
        });
        vi.spyOn(console, 'error').mockImplementation(() => undefined);
    });

    afterEach(() => {
        TestBed.inject(HttpTestingController).verify();
        vi.restoreAllMocks();
    });

    it('routes an unconfigured instance to setup', async () => {
        const result = firstValueFrom(executeGuard({} as never, { url: '/signup' } as never) as never);
        const request = TestBed.inject(HttpTestingController).expectOne('/api/status');

        expect(request.request.context.get(SKIP_AUTH_REDIRECT)).toBe(true);
        request.flush({ needs_setup: true, version: 'test' });

        const target = await result;
        expect(target).toBeInstanceOf(UrlTree);
        expect(TestBed.inject(Router).serializeUrl(target as UrlTree)).toBe('/setup');
    });

    it('shows the application error page when signup status cannot be determined', async () => {
        const result = firstValueFrom(executeGuard({} as never, { url: '/signup?plan=pro' } as never) as never);
        TestBed.inject(HttpTestingController).expectOne('/api/status').flush('unavailable', { status: 504, statusText: 'Gateway Timeout' });

        const command = (await result) as RedirectCommand;
        expect(command).toBeInstanceOf(RedirectCommand);
        expect(command.navigationBehaviorOptions?.state?.[APPLICATION_ERROR_STATE_KEY]).toEqual({
            version: 1,
            kind: 'server',
            context: 'cloud-signup-status',
            returnUrl: '/signup',
            status: 504
        });
    });

    it('allows signup when the hosted profile probe returns 401', async () => {
        const result = firstValueFrom(executeGuard({} as never, { url: '/signup' } as never) as never);
        TestBed.inject(HttpTestingController)
            .expectOne('/api/status')
            .flush({ cloud: { hosted: true, signup_enabled: true } });
        TestBed.inject(HttpTestingController).expectOne('/api/user/profile').flush('unauthorized', { status: 401, statusText: 'Unauthorized' });

        expect(await result).toBe(true);
        expect(auth.markUnauthenticated).toHaveBeenCalledTimes(1);
    });

    it('shows the application error page for a hosted profile outage', async () => {
        const result = firstValueFrom(executeGuard({} as never, { url: '/signup' } as never) as never);
        TestBed.inject(HttpTestingController)
            .expectOne('/api/status')
            .flush({ cloud: { hosted: true, signup_enabled: true } });
        TestBed.inject(HttpTestingController).expectOne('/api/user/profile').flush('unavailable', { status: 503, statusText: 'Service Unavailable' });

        const command = (await result) as RedirectCommand;
        expect(command).toBeInstanceOf(RedirectCommand);
        expect(command.navigationBehaviorOptions?.state?.[APPLICATION_ERROR_STATE_KEY]).toEqual({
            version: 1,
            kind: 'server',
            context: 'cloud-signup-profile',
            returnUrl: '/signup',
            status: 503
        });
        expect(auth.markUnauthenticated).not.toHaveBeenCalled();
    });

    it('classifies a hosted profile network failure as offline without a status', async () => {
        const result = firstValueFrom(executeGuard({} as never, { url: '/signup' } as never) as never);
        TestBed.inject(HttpTestingController)
            .expectOne('/api/status')
            .flush({ cloud: { hosted: true, signup_enabled: true } });
        TestBed.inject(HttpTestingController).expectOne('/api/user/profile').error(new ProgressEvent('network'));

        const command = (await result) as RedirectCommand;
        const state = command.navigationBehaviorOptions?.state?.[APPLICATION_ERROR_STATE_KEY];
        expect(state).toEqual({ version: 1, kind: 'offline', context: 'cloud-signup-profile', returnUrl: '/signup' });
        expect(auth.markUnauthenticated).not.toHaveBeenCalled();
    });

    for (const testCase of [
        { kind: 'client', label: 'forbidden profile responses', status: 403, statusText: 'Forbidden' },
        { kind: 'client', label: 'rate-limited profile responses', status: 429, statusText: 'Too Many Requests' },
        { kind: 'server', label: 'server profile responses', status: 500, statusText: 'Internal Server Error' }
    ] as const) {
        it(`routes ${testCase.label} to the profile error page`, async () => {
            const result = firstValueFrom(executeGuard({} as never, { url: '/signup' } as never) as never);
            TestBed.inject(HttpTestingController)
                .expectOne('/api/status')
                .flush({ cloud: { hosted: true, signup_enabled: true } });
            TestBed.inject(HttpTestingController).expectOne('/api/user/profile').flush('unavailable', { status: testCase.status, statusText: testCase.statusText });

            const command = (await result) as RedirectCommand;
            const state = command.navigationBehaviorOptions?.state?.[APPLICATION_ERROR_STATE_KEY] as Record<string, unknown>;
            expect(state).toEqual({ version: 1, kind: testCase.kind, context: 'cloud-signup-profile', returnUrl: '/signup', status: testCase.status });
            expect(Object.keys(state)).not.toContain('error');
            expect(auth.markUnauthenticated).not.toHaveBeenCalled();
        });
    }
});
