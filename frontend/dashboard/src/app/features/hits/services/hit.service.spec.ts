import { TestBed } from '@angular/core/testing';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideHttpClient } from '@angular/common/http';
import { HitService } from '@features/hits/services/hit.service';

describe('HitService', () => {
    let service: HitService;
    let httpMock: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [HitService, provideHttpClient(), provideHttpClientTesting()]
        });
        service = TestBed.inject(HitService);
        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('should be created', () => {
        expect(service).toBeTruthy();
    });

    it('sends dimension and conversion cohort filters together', () => {
        service.loadHits('site-1', 'from', 'to', 2, 25, 'path', 'asc', 'checkout', [{ type: 'country', value: 'DE' }], ['goal-1'], ['funnel-1']);

        const req = httpMock.expectOne((request) => request.url === '/api/sites/site-1/hits');
        expect(req.request.params.get('offset')).toBe('25');
        expect(req.request.params.getAll('filter')).toEqual(['country:DE']);
        expect(req.request.params.getAll('goal_id')).toEqual(['goal-1']);
        expect(req.request.params.getAll('funnel_id')).toEqual(['funnel-1']);
        req.flush({ data: [], total: 0 });
    });

    it('cancels a superseded request and only exposes the latest result', () => {
        service.loadHits('site-1', 'from', 'to', 1, 10, undefined, undefined, 'first');
        const first = httpMock.expectOne((request) => request.params.get('q') === 'first');

        service.loadHits('site-1', 'from', 'to', 1, 10, undefined, undefined, 'second');
        const second = httpMock.expectOne((request) => request.params.get('q') === 'second');

        expect(first.cancelled).toBe(true);
        const latest = { id: 'latest', site_id: 'site-1', session_id: 'session-1', page_id: 'page-1', timestamp: '2026-07-29T00:00:00Z', path: '/latest' };
        second.flush({ data: [latest], total: 1 });
        expect(service.hits()).toEqual([latest]);
        expect(service.total()).toBe(1);
        expect(service.isLoading()).toBe(false);
    });

    it('uses the public-share endpoint and exposes a local retryable error state', () => {
        service.loadHits('site-1', 'from', 'to', 1, 10, undefined, undefined, undefined, [], ['goal-1'], [], 'share token');

        const req = httpMock.expectOne((request) => request.url === '/api/share/share%20token/sites/site-1/hits');
        req.flush('failed', { status: 500, statusText: 'Server Error' });

        expect(service.hasError()).toBe(true);
        expect(service.hits()).toEqual([]);
        expect(service.total()).toBe(0);
        expect(service.isLoading()).toBe(false);
    });
});
