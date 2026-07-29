import { TestBed } from '@angular/core/testing';
import { RedirectCommand, Router, provideRouter } from '@angular/router';
import { vi } from 'vitest';

import { SESSION_END_STATE_KEY, SessionEndNavigationService, createSessionEndState, readSessionEndState, sanitizeSessionEndReturnUrl } from './session-end-navigation.service';

describe('SessionEndNavigationService', () => {
    beforeEach(() => {
        TestBed.configureTestingModule({ providers: [provideRouter([])] });
    });

    it('accepts only the versioned internal reasons', () => {
        expect(readSessionEndState({ [SESSION_END_STATE_KEY]: createSessionEndState('signed-out') })).toEqual(createSessionEndState('signed-out'));
        expect(readSessionEndState({ [SESSION_END_STATE_KEY]: { version: 1, reason: 'server-error' } })).toBeNull();
        expect(readSessionEndState({ [SESSION_END_STATE_KEY]: { version: 2, reason: 'session-ended' } })).toBeNull();
        expect(readSessionEndState({ [SESSION_END_STATE_KEY]: { version: 1, reason: 'session-ended', response: 'private backend body' } })).toBeNull();
    });

    it('sanitizes unsafe or authentication return URLs', () => {
        for (const target of ['https://example.com', '//example.com', '/\\evil.example', '/%2f%2fevil.example', '/login', '/setup', '/dashboard\nsecret', '']) {
            expect(sanitizeSessionEndReturnUrl(target)).toBe('/dashboard');
        }
        expect(sanitizeSessionEndReturnUrl('/events?range=30d#top')).toBe('/events?range=30d#top');
    });

    it('creates a replaceable redirect carrying only the safe reason', () => {
        const service = TestBed.inject(SessionEndNavigationService);
        const command = service.redirectToLogin('session-ended', '/dashboard?range=7d');

        expect(command).toBeInstanceOf(RedirectCommand);
        expect(TestBed.inject(Router).serializeUrl(command.redirectTo)).toBe('/login?returnUrl=%2Fdashboard%3Frange%3D7d');
        expect(command.navigationBehaviorOptions?.replaceUrl).toBe(true);
        expect(command.navigationBehaviorOptions?.state).toEqual({ [SESSION_END_STATE_KEY]: createSessionEndState('session-ended') });
        expect(JSON.stringify(command.navigationBehaviorOptions?.state)).not.toContain('private');
    });

    it('deduplicates concurrent imperative expiry redirects', async () => {
        const service = TestBed.inject(SessionEndNavigationService);
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);

        service.navigateToLogin('session-ended', '/dashboard');
        service.navigateToLogin('session-ended', '/events');
        await Promise.resolve();

        expect(navigate).toHaveBeenCalledTimes(1);
        expect(navigate.mock.calls[0]?.[0]).toEqual(['/login']);
        expect(navigate.mock.calls[0]?.[1]?.replaceUrl).toBe(true);
        expect(navigate.mock.calls[0]?.[1]?.queryParams).toEqual({ returnUrl: '/dashboard' });
    });
});
