import { DOCUMENT } from '@angular/common';
import { inject, Service } from '@angular/core';
import { NavigationExtras, RedirectCommand, Router } from '@angular/router';

import { browserBasePath } from '@core/interceptors/base-path.interceptor';

export type SessionEndReason = 'signed-out' | 'session-ended';

export const SESSION_END_STATE_KEY = 'sessionEnd';
export const SESSION_END_STATE_VERSION = 1;

const SESSION_END_REASONS = ['signed-out', 'session-ended'] as const;

export interface SessionEndNavigationState {
    version: typeof SESSION_END_STATE_VERSION;
    reason: SessionEndReason;
}

interface ReturnUrlRouterContext {
    url: string;
    routerState: {
        snapshot: {
            url: string;
        };
    };
}

@Service()
export class SessionEndNavigationService {
    private readonly router = inject(Router);
    private readonly document = inject(DOCUMENT);
    private navigationPending = false;

    navigateToLogin(reason: SessionEndReason, returnUrl?: string): void {
        if (this.navigationPending) return;

        const target = reason === 'session-ended' ? (returnUrl ?? resolveCurrentReturnUrl(this.router, browserBasePath(this.document))) : undefined;
        if (target && !shouldRedirectAfterUnauthorized(target)) return;

        this.navigationPending = true;
        void this.router.navigate(['/login'], sessionEndNavigationExtras(reason, target)).finally(() => {
            this.navigationPending = false;
        });
    }

    redirectToLogin(reason: SessionEndReason, returnUrl: string): RedirectCommand {
        const extras = sessionEndNavigationExtras(reason, returnUrl);
        return new RedirectCommand(this.router.createUrlTree(['/login'], extras), extras);
    }
}

export function createSessionEndState(reason: SessionEndReason): SessionEndNavigationState {
    return {
        version: SESSION_END_STATE_VERSION,
        reason
    };
}

export function readSessionEndState(value: unknown): SessionEndNavigationState | null {
    if (!isRecord(value)) return null;
    const candidate = value[SESSION_END_STATE_KEY];
    if (!hasOnlyKeys(candidate, ['version', 'reason']) || candidate['version'] !== SESSION_END_STATE_VERSION || !isSessionEndReason(candidate['reason'])) {
        return null;
    }
    return createSessionEndState(candidate['reason']);
}

export function resolveCurrentReturnUrl(router: ReturnUrlRouterContext, basePath = '/'): string {
    const browserPath = typeof window !== 'undefined' && typeof window.location !== 'undefined' ? `${window.location.pathname || ''}${window.location.search || ''}${window.location.hash || ''}` : '';
    const normalizedBrowserPath = stripBrowserBasePath(browserPath, basePath);
    const candidate = normalizedBrowserPath && normalizedBrowserPath !== '/' ? normalizedBrowserPath : router.url || router.routerState.snapshot.url || '/dashboard';
    if (!candidate.startsWith('/') || candidate.startsWith('//')) return '/dashboard';
    return candidate;
}

export function shouldRedirectAfterUnauthorized(currentUrl: string): boolean {
    return !isAuthenticationEntryPath(currentUrl);
}

export function sanitizeSessionEndReturnUrl(value: string): string {
    const candidate = value.trim();
    if (candidate.length === 0 || candidate.length > 2048 || !candidate.startsWith('/') || candidate.startsWith('//') || containsControlCharacter(candidate)) {
        return '/dashboard';
    }

    try {
        const parsed = new URL(candidate, 'https://hitkeep.invalid');
        const decodedPath = decodeURIComponent(parsed.pathname);
        if (parsed.origin !== 'https://hitkeep.invalid' || !decodedPath.startsWith('/') || decodedPath.startsWith('//') || decodedPath.includes('\\') || containsControlCharacter(decodedPath) || isAuthenticationEntryPath(decodedPath)) {
            return '/dashboard';
        }
    } catch {
        return '/dashboard';
    }
    return candidate;
}

function isSessionEndReason(value: unknown): value is SessionEndReason {
    return SESSION_END_REASONS.some((reason) => reason === value);
}

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === 'object' && value !== null;
}

function hasOnlyKeys(value: unknown, expectedKeys: readonly string[]): value is Record<string, unknown> {
    return isRecord(value) && Object.keys(value).length === expectedKeys.length && expectedKeys.every((key) => Object.hasOwn(value, key));
}

function sessionEndNavigationExtras(reason: SessionEndReason, returnUrl?: string): NavigationExtras {
    return {
        ...(reason === 'session-ended' && { queryParams: { returnUrl: sanitizeSessionEndReturnUrl(returnUrl ?? '/dashboard') } }),
        replaceUrl: true,
        state: { [SESSION_END_STATE_KEY]: createSessionEndState(reason) }
    };
}

function isAuthenticationEntryPath(value: string): boolean {
    const path = value.split(/[?#]/, 1)[0] ?? value;
    return path === '/login' || path.startsWith('/login/') || path === '/setup' || path.startsWith('/setup/');
}

function containsControlCharacter(value: string): boolean {
    for (const character of value) {
        if (character.charCodeAt(0) < 32) return true;
    }
    return false;
}

function stripBrowserBasePath(browserPath: string, basePath: string): string {
    if (!browserPath || basePath === '/') return browserPath;
    const prefix = basePath.endsWith('/') ? basePath.slice(0, -1) : basePath;
    if (browserPath === prefix) return '/';
    if (browserPath.startsWith(`${prefix}/`)) return browserPath.slice(prefix.length) || '/';
    return browserPath;
}
