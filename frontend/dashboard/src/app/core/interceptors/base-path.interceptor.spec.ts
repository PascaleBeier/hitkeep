import { HttpClient, provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { DOCUMENT } from '@angular/common';
import { afterEach, describe, expect, it } from 'vitest';

import { basePathInterceptor, browserAppUrl, browserBasePath, prefixBrowserBasePath } from './base-path.interceptor';

describe('browserBasePath', () => {
    it('normalizes root and subdirectory base hrefs', () => {
        expect(browserBasePath(baseDocument('/'))).toBe('/');
        expect(browserBasePath(baseDocument('/hitkeep/'))).toBe('/hitkeep/');
        expect(browserBasePath(baseDocument('/tools/hitkeep'))).toBe('/tools/hitkeep/');
    });
});

describe('prefixBrowserBasePath', () => {
    it('prefixes app-owned absolute paths when a subdirectory base is active', () => {
        expect(prefixBrowserBasePath('/api/status', '/hitkeep/')).toBe('/hitkeep/api/status');
        expect(prefixBrowserBasePath('/i18n/en.json', '/hitkeep/')).toBe('/hitkeep/i18n/en.json');
        expect(prefixBrowserBasePath('/scalar/index.html', '/hitkeep/')).toBe('/hitkeep/scalar/index.html');
        expect(prefixBrowserBasePath('/hk.js', '/hitkeep/')).toBe('/hitkeep/hk.js');
        expect(prefixBrowserBasePath('/brand-icon.svg', '/hitkeep/')).toBe('/hitkeep/brand-icon.svg');
        expect(prefixBrowserBasePath('/icon.png', '/hitkeep/')).toBe('/hitkeep/icon.png');
    });

    it('preserves root deployments, already-prefixed paths, and external URLs', () => {
        expect(prefixBrowserBasePath('/api/status', '/')).toBe('/api/status');
        expect(prefixBrowserBasePath('/hitkeep/api/status', '/hitkeep/')).toBe('/hitkeep/api/status');
        expect(prefixBrowserBasePath('https://api.example.com/status', '/hitkeep/')).toBe('https://api.example.com/status');
    });
});

describe('browserAppUrl', () => {
    it('prefixes app-owned resource URLs that do not use HttpClient', () => {
        expect(browserAppUrl(baseDocument('/hitkeep/'), '/api/user/avatar?s=96')).toBe('/hitkeep/api/user/avatar?s=96');
        expect(browserAppUrl(baseDocument('/hitkeep/'), '/api/favicon/acme.test')).toBe('/hitkeep/api/favicon/acme.test');
    });
});

describe('browserAbsoluteAppUrl', () => {
    it('builds absolute app-owned URLs from the browser origin and base path', async () => {
        const { browserAbsoluteAppUrl } = await import('./base-path.interceptor');

        expect(browserAbsoluteAppUrl(baseDocument('/hitkeep/'), '/hk.js', 'https://www.example.net')).toBe('https://www.example.net/hitkeep/hk.js');
    });
});

describe('basePathInterceptor', () => {
    afterEach(() => {
        TestBed.inject(HttpTestingController).verify();
    });

    it('prefixes API requests with the document base path', () => {
        TestBed.configureTestingModule({
            providers: [provideHttpClient(withInterceptors([basePathInterceptor])), provideHttpClientTesting(), { provide: DOCUMENT, useValue: baseDocument('/hitkeep/') }]
        });

        const http = TestBed.inject(HttpClient);
        const httpMock = TestBed.inject(HttpTestingController);

        http.get('/api/status').subscribe();

        httpMock.expectOne('/hitkeep/api/status').flush({});
    });
});

function baseDocument(baseHref: string): Document {
    const document = window.document.implementation.createHTMLDocument('hitkeep test');
    const base = document.createElement('base');
    base.href = baseHref;
    document.head.append(base);
    return document;
}
