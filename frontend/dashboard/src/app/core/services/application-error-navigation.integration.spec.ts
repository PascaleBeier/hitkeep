import { Location } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { Component, inject } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { CanActivateFn, Router, withNavigationErrorHandler, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { vi } from 'vitest';

import { APPLICATION_ERROR_STATE_KEY, ApplicationErrorNavigationService } from './application-error-navigation.service';

@Component({ standalone: true, template: 'error probe' })
class ErrorProbe {
    readonly historyState = inject(Location).getState();
}

@Component({ standalone: true, template: 'ok' })
class StableProbe {}

describe('ApplicationErrorNavigationService routing integration', () => {
    beforeEach(() => {
        vi.spyOn(console, 'error').mockImplementation(() => undefined);
    });

    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('carries a route-critical redirect state through the real router', async () => {
        const failingGuard: CanActivateFn = (_route, state) => TestBed.inject(ApplicationErrorNavigationService).fromHttp(new HttpErrorResponse({ status: 503, error: 'private response' }), 'setup-status', state.url);

        TestBed.configureTestingModule({
            providers: [
                provideRouter(
                    [
                        { path: 'error', component: ErrorProbe },
                        { path: 'login', component: StableProbe },
                        { path: 'guarded', canActivate: [failingGuard], component: StableProbe }
                    ],
                    withNavigationErrorHandler((error) => TestBed.inject(ApplicationErrorNavigationService).fromNavigation(error))
                )
            ]
        });

        const harness = await RouterTestingHarness.create('/guarded');
        const probe = harness.routeDebugElement?.componentInstance as ErrorProbe;
        const state = (probe.historyState as Record<string, unknown>)[APPLICATION_ERROR_STATE_KEY] as Record<string, unknown>;

        expect(TestBed.inject(Router).url).toBe('/error');
        expect(state).toEqual({
            version: 1,
            kind: 'server',
            context: 'setup-status',
            returnUrl: '/guarded',
            status: 503
        });
    });

    it('redirects navigation errors to the error route with a safe return URL', async () => {
        TestBed.configureTestingModule({
            providers: [
                provideRouter(
                    [
                        { path: 'error', component: ErrorProbe },
                        { path: 'broken', loadComponent: () => Promise.reject(new Error('private navigation failure')) }
                    ],
                    withNavigationErrorHandler((error) => TestBed.inject(ApplicationErrorNavigationService).fromNavigation(error))
                )
            ]
        });

        const harness = await RouterTestingHarness.create('/broken');
        const probe = harness.routeDebugElement?.componentInstance as ErrorProbe;
        const state = (probe.historyState as Record<string, unknown>)[APPLICATION_ERROR_STATE_KEY] as Record<string, unknown>;

        expect(TestBed.inject(Router).url).toBe('/error');
        expect(state).toEqual({ version: 1, kind: 'navigation', context: 'navigation', returnUrl: '/broken' });
    });
});
