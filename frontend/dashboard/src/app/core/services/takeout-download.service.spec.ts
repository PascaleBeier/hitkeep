import type { Mock } from 'vitest';
import { HttpHeaders, provideHttpClient } from '@angular/common/http';
import { TestBed } from '@angular/core/testing';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { vi } from 'vitest';

import { TakeoutDownloadService } from './takeout-download.service';

describe('TakeoutDownloadService', () => {
    let service: TakeoutDownloadService;
    let httpMock: HttpTestingController;
    let createObjectURLSpy: Mock;
    let revokeObjectURLSpy: Mock;
    let clickSpy: Mock;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [TakeoutDownloadService, provideHttpClient(), provideHttpClientTesting()]
        });

        service = TestBed.inject(TakeoutDownloadService);
        httpMock = TestBed.inject(HttpTestingController);

        createObjectURLSpy = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:test-url');
        revokeObjectURLSpy = vi.spyOn(URL, 'revokeObjectURL');
        clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click');
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('downloads user takeout and uses response filename when provided', () => {
        let downloadedFilename = '';

        service.downloadUserTakeout('json').subscribe({
            next: (filename) => {
                downloadedFilename = filename;
            },
            error: (error: unknown) => expect.fail(`unexpected error: ${String(error)}`)
        });

        const req = httpMock.expectOne('/api/user/takeout?format=json');
        expect(req.request.method).toBe('GET');
        expect(req.request.responseType).toBe('blob');
        req.flush(new Blob(['{"ok":true}'], { type: 'application/json' }), {
            headers: new HttpHeaders({
                'content-disposition': 'attachment; filename="user-export.json"'
            })
        });

        expect(downloadedFilename).toBe('user-export.json');
        expect(createObjectURLSpy).toHaveBeenCalled();
        expect(clickSpy).toHaveBeenCalled();
        expect(revokeObjectURLSpy).toHaveBeenCalledWith('blob:test-url');
    });

    it('downloads site takeout and falls back to sanitized filename when header is missing', () => {
        let downloadedFilename = '';

        service.downloadSiteTakeout('site-123', 'My Site.test', 'csv').subscribe({
            next: (filename) => {
                downloadedFilename = filename;
            },
            error: (error: unknown) => expect.fail(`unexpected error: ${String(error)}`)
        });

        const req = httpMock.expectOne('/api/sites/site-123/takeout?format=csv');
        req.flush(new Blob(['a,b\n1,2'], { type: 'text/csv' }));

        expect(downloadedFilename).toMatch(/^my-site-test-takeout-\d{4}-\d{2}-\d{2}\.csv$/);
        expect(createObjectURLSpy).toHaveBeenCalled();
        expect(clickSpy).toHaveBeenCalled();
        expect(revokeObjectURLSpy).toHaveBeenCalledWith('blob:test-url');
    });

    it('allows empty ndjson response as a valid download', () => {
        let downloadedFilename = '';

        service.downloadUserTakeout('ndjson').subscribe({
            next: (filename) => {
                downloadedFilename = filename;
            },
            error: (error: unknown) => expect.fail(`unexpected error: ${String(error)}`)
        });

        const req = httpMock.expectOne('/api/user/takeout?format=ndjson');
        req.flush(new Blob([]), {
            headers: new HttpHeaders({
                'content-disposition': 'attachment; filename="user-export.ndjson"',
                'content-type': 'application/x-ndjson'
            })
        });

        expect(downloadedFilename).toBe('user-export.ndjson');
        expect(createObjectURLSpy).toHaveBeenCalled();
        expect(clickSpy).toHaveBeenCalled();
    });

    it('supports generic export URLs used by filtered exports', () => {
        let downloadedFilename = '';

        service.downloadFromUrl('/api/sites/site-1/hits/export?format=csv', 'fallback.csv').subscribe({
            next: (filename) => {
                downloadedFilename = filename;
            },
            error: (error: unknown) => expect.fail(`unexpected error: ${String(error)}`)
        });

        const req = httpMock.expectOne('/api/sites/site-1/hits/export?format=csv');
        req.flush(new Blob(['id,path\n1,/'], { type: 'text/csv' }), {
            headers: new HttpHeaders({
                'content-disposition': 'attachment; filename="hits.csv"'
            })
        });

        expect(downloadedFilename).toBe('hits.csv');
        expect(createObjectURLSpy).toHaveBeenCalled();
        expect(clickSpy).toHaveBeenCalled();
    });

    it('decodes RFC5987 filename values from content-disposition', () => {
        let downloadedFilename = '';

        service.downloadFromUrl('/api/user/takeout?format=ndjson', 'fallback.ndjson').subscribe({
            next: (filename) => {
                downloadedFilename = filename;
            },
            error: (error: unknown) => expect.fail(`unexpected error: ${String(error)}`)
        });

        const req = httpMock.expectOne('/api/user/takeout?format=ndjson');
        req.flush(new Blob(['{"n":1}\n'], { type: 'application/x-ndjson' }), {
            headers: new HttpHeaders({
                'content-disposition': "attachment; filename*=UTF-8''takeout%20data.ndjson"
            })
        });

        expect(downloadedFilename).toBe('takeout data.ndjson');
    });

    it('restores the fallback file extension when the header filename omits it', () => {
        let downloadedFilename = '';

        service.downloadFromUrl('/api/sites/site-1/ai-chatbots/export?format=csv', 'site-ai-chatbots.csv').subscribe({
            next: (filename) => {
                downloadedFilename = filename;
            },
            error: (error: unknown) => expect.fail(`unexpected error: ${String(error)}`)
        });

        const req = httpMock.expectOne('/api/sites/site-1/ai-chatbots/export?format=csv');
        req.flush(new Blob(['id,name\n1,test'], { type: 'text/csv' }), {
            headers: new HttpHeaders({
                'content-disposition': 'attachment; filename="ai-chatbots-export"'
            })
        });

        expect(downloadedFilename).toBe('ai-chatbots-export.csv');
    });

    it('rejects unexpected html responses instead of downloading them', () => {
        let downloadError: unknown;
        const objectURLCallsBefore = createObjectURLSpy.mock.calls.length;
        const clickCallsBefore = clickSpy.mock.calls.length;

        service.downloadFromUrl('/api/sites/site-1/ai-fetch/export?format=csv', 'fallback.csv').subscribe({
            next: () => expect.fail('expected html response to be rejected'),
            error: (error: unknown) => {
                downloadError = error;
            }
        });

        const req = httpMock.expectOne('/api/sites/site-1/ai-fetch/export?format=csv');
        req.flush(new Blob(['<html><body>login</body></html>'], { type: 'text/html' }), {
            headers: new HttpHeaders({
                'content-type': 'text/html; charset=utf-8'
            })
        });

        expect(downloadError).toBeInstanceOf(Error);
        expect((downloadError as Error).message).toBe('unexpected_html_download_response');
        expect(createObjectURLSpy.mock.calls.length).toBe(objectURLCallsBefore);
        expect(clickSpy.mock.calls.length).toBe(clickCallsBefore);
    });
});
