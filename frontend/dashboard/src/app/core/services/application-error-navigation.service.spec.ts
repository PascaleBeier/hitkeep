import { HttpErrorResponse } from '@angular/common/http';
import { TestBed } from '@angular/core/testing';
import { RedirectCommand, Router, provideRouter } from '@angular/router';
import { TimeoutError } from 'rxjs';
import { vi } from 'vitest';

import { APPLICATION_ERROR_STATE_KEY, ApplicationErrorNavigationService, readApplicationErrorState, sanitizeApplicationReturnUrl } from '@services/application-error-navigation.service';

describe('ApplicationErrorNavigationService', () => {
    beforeEach(() => {
        TestBed.configureTestingModule({ providers: [provideRouter([])] });
        vi.spyOn(console, 'error').mockImplementation(() => undefined);
    });

    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('creates a safe redirect for a route-critical server response', () => {
        const service = TestBed.inject(ApplicationErrorNavigationService);
        const router = TestBed.inject(Router);
        const command = service.fromHttp(new HttpErrorResponse({ status: 503, error: { private: 'do not expose' } }), 'dashboard-bootstrap', '/dashboard?range=7d');

        expect(command).toBeInstanceOf(RedirectCommand);
        expect(router.serializeUrl(command.redirectTo)).toBe('/error');
        expect(command.navigationBehaviorOptions?.replaceUrl).toBe(true);
        expect(command.navigationBehaviorOptions?.state?.[APPLICATION_ERROR_STATE_KEY]).toEqual({
            version: 1,
            kind: 'server',
            context: 'dashboard-bootstrap',
            returnUrl: '/dashboard',
            status: 503
        });
        expect(JSON.stringify(command.navigationBehaviorOptions?.state)).not.toContain('do not expose');
    });

    it('distinguishes offline failures without inventing an HTTP status', () => {
        const command = TestBed.inject(ApplicationErrorNavigationService).fromHttp(new HttpErrorResponse({ status: 0 }), 'setup-status', '/login');
        const state = command.navigationBehaviorOptions?.state?.[APPLICATION_ERROR_STATE_KEY];

        expect(state).toEqual({ version: 1, kind: 'offline', context: 'setup-status', returnUrl: '/login' });
    });

    it('classifies request timeouts as offline without inventing an HTTP status', () => {
        const command = TestBed.inject(ApplicationErrorNavigationService).fromHttp(new TimeoutError(), 'dashboard-bootstrap', '/dashboard');
        const state = command.navigationBehaviorOptions?.state?.[APPLICATION_ERROR_STATE_KEY];

        expect(state).toEqual({ version: 1, kind: 'offline', context: 'dashboard-bootstrap', returnUrl: '/dashboard' });
    });

    it('accepts only versioned, safe navigation state', () => {
        expect(
            readApplicationErrorState({
                [APPLICATION_ERROR_STATE_KEY]: {
                    version: 1,
                    kind: 'client',
                    context: 'cloud-signup-profile',
                    returnUrl: '/signup',
                    status: 429
                }
            })
        ).toEqual({ version: 1, kind: 'client', context: 'cloud-signup-profile', returnUrl: '/signup', status: 429 });
        expect(readApplicationErrorState({ [APPLICATION_ERROR_STATE_KEY]: { version: 1, kind: 'server', context: 'setup-status', returnUrl: '/login', status: '503' } })).toBeNull();
        expect(readApplicationErrorState({ [APPLICATION_ERROR_STATE_KEY]: { version: 2, kind: 'server', context: 'setup-status', returnUrl: '/login' } })).toBeNull();
    });

    it('prevents external, recursive, and malformed retry targets', () => {
        for (const target of ['https://example.com', '//example.com', '/error', '/error?status=500', '', '/dashboard\nmalformed']) {
            expect(sanitizeApplicationReturnUrl(target)).toBe('/dashboard');
        }
        expect(sanitizeApplicationReturnUrl('/dashboard?range=30d')).toBe('/dashboard');
        expect(sanitizeApplicationReturnUrl('/dashboard#overview')).toBe('/dashboard');
        expect(sanitizeApplicationReturnUrl('/share/private-token/dashboard')).toBe('/dashboard');
    });
});
