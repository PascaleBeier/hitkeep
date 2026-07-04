import { DOCUMENT } from '@angular/common';
import { HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';

const APP_OWNED_ROOT_PATHS = ['/api/', '/i18n/', '/scalar/', '/hk.js', '/hk-vitals.js', '/brand-icon.svg', '/icon.png', '/favicon.svg', '/favicon.ico', '/apple-touch-icon.png'];

export function browserBasePath(document: Document): string {
    const href = document.querySelector('base')?.getAttribute('href')?.trim() || '/';
    try {
        const path = new URL(href, 'https://hitkeep.local/').pathname;
        return normalizeBasePath(path);
    } catch {
        return '/';
    }
}

export function prefixBrowserBasePath(url: string, basePath: string): string {
    if (basePath === '/' || !APP_OWNED_ROOT_PATHS.some((path) => url.startsWith(path))) {
        return url;
    }
    if (url.startsWith(basePath)) {
        return url;
    }
    return `${basePath.slice(0, -1)}${url}`;
}

export function browserAppUrl(document: Document, url: string): string {
    return prefixBrowserBasePath(url, browserBasePath(document));
}

export function browserAbsoluteAppUrl(document: Document, url: string, origin = browserOrigin()): string {
    const appUrl = browserAppUrl(document, url);
    if (/^(?:[a-z][a-z\d+\-.]*:)?\/\//i.test(appUrl)) {
        return appUrl;
    }
    return `${origin}${appUrl.startsWith('/') ? appUrl : `/${appUrl}`}`;
}

export const basePathInterceptor: HttpInterceptorFn = (req, next) => {
    const document = inject(DOCUMENT);
    const url = prefixBrowserBasePath(req.url, browserBasePath(document));
    return next(url === req.url ? req : req.clone({ url }));
};

function normalizeBasePath(path: string): string {
    const cleanPath = path.trim();
    if (cleanPath === '' || cleanPath === '/') {
        return '/';
    }
    const withLeadingSlash = cleanPath.startsWith('/') ? cleanPath : `/${cleanPath}`;
    return withLeadingSlash.endsWith('/') ? withLeadingSlash : `${withLeadingSlash}/`;
}

function browserOrigin(): string {
    return typeof window !== 'undefined' && typeof window.location !== 'undefined' ? window.location.origin : '';
}
