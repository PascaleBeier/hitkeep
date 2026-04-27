import { describe, expect, it, beforeEach } from 'vitest';
import { resolveCurrentReturnUrl, shouldRedirectAfterUnauthorized } from './auth.interceptor';

describe('resolveCurrentReturnUrl', () => {
    beforeEach(() => {
        history.pushState({}, '', '/');
    });

    it('prefers browser location for deep links', () => {
        history.pushState({}, '', '/admin/system?tab=users#top');

        const router = {
            url: '/',
            routerState: {
                snapshot: {
                    url: '/'
                }
            }
        } as const;

        expect(resolveCurrentReturnUrl(router)).toBe('/admin/system?tab=users#top');
    });

    it('falls back to router url when browser path is root', () => {
        const router = {
            url: '/events',
            routerState: {
                snapshot: {
                    url: '/dashboard'
                }
            }
        } as const;

        expect(resolveCurrentReturnUrl(router)).toBe('/events');
    });

    it('guards against protocol-relative paths', () => {
        const router = {
            url: '//evil.example',
            routerState: {
                snapshot: {
                    url: '//evil.example'
                }
            }
        } as const;

        expect(resolveCurrentReturnUrl(router)).toBe('/dashboard');
    });
});

describe('shouldRedirectAfterUnauthorized', () => {
    it('redirects expired dashboard API traffic back to login', () => {
        expect(shouldRedirectAfterUnauthorized('/dashboard?range=7d')).toBe(true);
    });

    it('avoids login and setup redirect loops', () => {
        expect(shouldRedirectAfterUnauthorized('/login?returnUrl=/dashboard')).toBe(false);
        expect(shouldRedirectAfterUnauthorized('/setup')).toBe(false);
    });
});
