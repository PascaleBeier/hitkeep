import { Location } from '@angular/common';
import { Component } from '@angular/core';
import { CanActivateFn, Router, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { TestBed } from '@angular/core/testing';

import { SESSION_END_STATE_KEY, SessionEndNavigationService } from './session-end-navigation.service';

@Component({ standalone: true, selector: 'app-session-end-login-probe', template: 'login' })
class SessionEndLoginProbe {}

@Component({ standalone: true, selector: 'app-session-end-dashboard-probe', template: 'dashboard' })
class SessionEndDashboardProbe {}

describe('SessionEndNavigationService router integration', () => {
    it('delivers the validated session-ended state through a real guard redirect', async () => {
        const redirectGuard: CanActivateFn = (_route, state) => TestBed.inject(SessionEndNavigationService).redirectToLogin('session-ended', state.url);

        TestBed.configureTestingModule({
            providers: [
                provideRouter([
                    { path: 'login', component: SessionEndLoginProbe },
                    { path: 'dashboard', canActivate: [redirectGuard], component: SessionEndDashboardProbe }
                ])
            ]
        });

        await RouterTestingHarness.create('/dashboard');
        const state = TestBed.inject(Location).getState() as Record<string, unknown>;

        expect(TestBed.inject(Router).url).toBe('/login?returnUrl=%2Fdashboard');
        expect(state[SESSION_END_STATE_KEY]).toEqual({ version: 1, reason: 'session-ended' });
    });
});
