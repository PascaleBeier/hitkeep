import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter, Router } from '@angular/router';
import { vi } from 'vitest';

import { SESSION_END_STATE_KEY, createSessionEndState } from './session-end-navigation.service';
import { AuthService } from './auth.service';

describe('AuthService session termination', () => {
    let auth: AuthService;
    let http: HttpTestingController;
    let navigate: ReturnType<typeof vi.spyOn>;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])]
        });
        auth = TestBed.inject(AuthService);
        http = TestBed.inject(HttpTestingController);
        navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
    });

    afterEach(() => {
        vi.useRealTimers();
        auth.markUnauthenticated();
        http.verify({ ignoreCancelled: true });
        vi.restoreAllMocks();
    });

    it('navigates with a signed-out notice even when the logout request fails', () => {
        auth.logout().subscribe({ error: () => undefined });
        http.expectOne('/api/logout').flush('unavailable', { status: 503, statusText: 'Service Unavailable' });

        expect(navigate.mock.calls[0]?.[0]).toEqual(['/login']);
        expect(navigate.mock.calls[0]?.[1]?.replaceUrl).toBe(true);
        expect(navigate.mock.calls[0]?.[1]?.state).toEqual({ [SESSION_END_STATE_KEY]: createSessionEndState('signed-out') });
    });

    it('does not end the session when a logout request is cancelled', () => {
        const subscription = auth.logout().subscribe({ error: () => undefined });
        http.expectOne('/api/logout');

        subscription.unsubscribe();

        expect(navigate).not.toHaveBeenCalled();
    });

    it('navigates with a session-ended notice when the local timer expires', () => {
        vi.useFakeTimers();
        const issuedAt = Date.now();
        auth.applySession({
            expires_at: new Date(issuedAt + 1000).toISOString(),
            issued_at: new Date(issuedAt).toISOString(),
            duration_seconds: 1,
            warning_seconds: 0,
            extendable: false,
            timing_adjustable: false,
            remembered: false,
            remember_me_duration_days: 0
        });

        vi.advanceTimersByTime(1000);

        expect(navigate.mock.calls[0]?.[0]).toEqual(['/login']);
        expect(navigate.mock.calls[0]?.[1]?.replaceUrl).toBe(true);
        expect(navigate.mock.calls[0]?.[1]?.state).toEqual({ [SESSION_END_STATE_KEY]: createSessionEndState('session-ended') });
        vi.useRealTimers();
    });
});
