import { HttpErrorResponse } from '@angular/common/http';
import { inject, Service } from '@angular/core';
import { NavigationError, RedirectCommand, Router } from '@angular/router';
import { TimeoutError } from 'rxjs';

export const APPLICATION_ERROR_STATE_KEY = 'applicationError';
export const ROUTE_CRITICAL_REQUEST_TIMEOUT_MS = 15_000;

const APPLICATION_ERROR_KINDS = ['offline', 'client', 'server', 'navigation', 'generic'] as const;
const APPLICATION_ERROR_CONTEXTS = ['setup-status', 'dashboard-bootstrap', 'cloud-signup-status', 'cloud-signup-profile', 'navigation'] as const;

export type ApplicationErrorKind = (typeof APPLICATION_ERROR_KINDS)[number];
export type ApplicationErrorContext = (typeof APPLICATION_ERROR_CONTEXTS)[number];

export interface ApplicationErrorState {
    version: 1;
    kind: ApplicationErrorKind;
    context: ApplicationErrorContext;
    returnUrl: string;
    status?: number;
}

@Service()
export class ApplicationErrorNavigationService {
    private readonly router = inject(Router);

    fromHttp(error: unknown, context: Exclude<ApplicationErrorContext, 'navigation'>, returnUrl: string): RedirectCommand {
        const status = error instanceof HttpErrorResponse ? error.status : undefined;
        const kind = httpErrorKind(error, status);
        const state: ApplicationErrorState = {
            version: 1,
            kind,
            context,
            returnUrl: sanitizeApplicationReturnUrl(returnUrl)
        };

        if (isApplicationErrorStatus(status)) {
            state.status = status;
        }

        console.error('Route-critical request failed.', { context, status: status ?? null, returnUrl: state.returnUrl });
        return this.redirect(state);
    }

    fromNavigation(error: NavigationError): RedirectCommand | undefined {
        if (isApplicationErrorUrl(error.url)) {
            console.error('The application error page failed to load.', { url: sanitizeApplicationReturnUrl(error.url) });
            return undefined;
        }

        const state: ApplicationErrorState = {
            version: 1,
            kind: 'navigation',
            context: 'navigation',
            returnUrl: sanitizeApplicationReturnUrl(error.url)
        };

        console.error('Navigation failed.', { url: state.returnUrl });
        return this.redirect(state);
    }

    private redirect(state: ApplicationErrorState): RedirectCommand {
        return new RedirectCommand(this.router.parseUrl('/error'), {
            replaceUrl: true,
            state: { [APPLICATION_ERROR_STATE_KEY]: state }
        });
    }
}

export function readApplicationErrorState(value: unknown): ApplicationErrorState | null {
    if (!isRecord(value)) return null;
    const candidate = value[APPLICATION_ERROR_STATE_KEY];
    if (!isRecord(candidate) || candidate['version'] !== 1) return null;

    const kind = candidate['kind'];
    const context = candidate['context'];
    if (!isApplicationErrorKind(kind) || !isApplicationErrorContext(context)) return null;

    const returnUrl = sanitizeApplicationReturnUrl(candidate['returnUrl']);
    const status = candidate['status'];
    if (status !== undefined && !isApplicationErrorStatus(status)) return null;

    return {
        version: 1,
        kind,
        context,
        returnUrl,
        ...(typeof status === 'number' ? { status } : {})
    };
}

export function sanitizeApplicationReturnUrl(value: unknown): string {
    if (typeof value !== 'string') return '/dashboard';
    const candidate = value.trim();
    if (candidate.length === 0 || candidate.length > 2048 || !candidate.startsWith('/') || candidate.startsWith('//') || containsControlCharacter(candidate) || isApplicationErrorUrl(candidate)) {
        return '/dashboard';
    }

    try {
        const parsed = new URL(candidate, 'https://hitkeep.invalid');
        const path = decodeURIComponent(parsed.pathname);
        if (parsed.origin !== 'https://hitkeep.invalid' || !path.startsWith('/') || path.startsWith('//') || path.includes('\\') || isApplicationErrorUrl(path) || isSensitiveApplicationUrl(path)) {
            return '/dashboard';
        }
        return parsed.pathname;
    } catch {
        return '/dashboard';
    }
}

function containsControlCharacter(value: string): boolean {
    for (const character of value) {
        if (character.charCodeAt(0) < 32) return true;
    }
    return false;
}

function httpErrorKind(error: unknown, status: number | undefined): ApplicationErrorKind {
    if (status === 0 || error instanceof TimeoutError) return 'offline';
    if (isApplicationErrorStatus(status)) return status < 500 ? 'client' : 'server';
    return 'generic';
}

function isApplicationErrorStatus(value: unknown): value is number {
    return typeof value === 'number' && Number.isInteger(value) && value >= 400 && value <= 599;
}

export function isApplicationErrorUrl(value: string): boolean {
    const path = value.split(/[?#]/, 1)[0];
    return path === '/error' || path.startsWith('/error/');
}

function isSensitiveApplicationUrl(path: string): boolean {
    return path === '/share' || path.startsWith('/share/') || path === '/qr-share' || path.startsWith('/qr-share/');
}

function isApplicationErrorKind(value: unknown): value is ApplicationErrorKind {
    return APPLICATION_ERROR_KINDS.some((kind) => kind === value);
}

function isApplicationErrorContext(value: unknown): value is ApplicationErrorContext {
    return APPLICATION_ERROR_CONTEXTS.some((context) => context === value);
}

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === 'object' && value !== null;
}
