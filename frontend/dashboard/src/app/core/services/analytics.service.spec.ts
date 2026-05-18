import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { AnalyticsService } from './analytics.service';

describe('AnalyticsService Web Vitals', () => {
    let service: AnalyticsService;
    let httpMock: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [provideHttpClient(), provideHttpClientTesting()]
        });
        service = TestBed.inject(AnalyticsService);
        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('requests Web Vitals summary with optional filters', () => {
        service.getWebVitalsSummary('site-1', '2026-05-01T00:00:00Z', '2026-05-13T00:00:00Z', null, '/pricing', 'poor').subscribe();

        const req = httpMock.expectOne((request) => request.url === '/api/sites/site-1/web-vitals/summary');
        expect(req.request.method).toBe('GET');
        expect(req.request.params.get('from')).toBe('2026-05-01T00:00:00Z');
        expect(req.request.params.get('to')).toBe('2026-05-13T00:00:00Z');
        expect(req.request.params.has('metric')).toBe(false);
        expect(req.request.params.get('path')).toBe('/pricing');
        expect(req.request.params.get('rating')).toBe('poor');
        req.flush([]);
    });

    it('requests Web Vitals timeseries and page rows with metric and limit', () => {
        service.getWebVitalsTimeseries('site-1', 'from', 'to', 'LCP').subscribe();
        const seriesReq = httpMock.expectOne((request) => request.url === '/api/sites/site-1/web-vitals/timeseries');
        expect(seriesReq.request.params.get('metric')).toBe('LCP');
        seriesReq.flush([]);

        service.getWebVitalsPages('site-1', 'from', 'to', 'INP', '/checkout', 'needs_improvement', 12).subscribe();
        const pagesReq = httpMock.expectOne((request) => request.url === '/api/sites/site-1/web-vitals/pages');
        expect(pagesReq.request.params.get('metric')).toBe('INP');
        expect(pagesReq.request.params.get('path')).toBe('/checkout');
        expect(pagesReq.request.params.get('rating')).toBe('needs_improvement');
        expect(pagesReq.request.params.get('limit')).toBe('12');
        pagesReq.flush([]);

        service.getWebVitalsBreakdown('site-1', 'from', 'to', 'LCP', 'browser', '/pricing', 'poor', 10).subscribe();
        const breakdownReq = httpMock.expectOne((request) => request.url === '/api/sites/site-1/web-vitals/breakdown');
        expect(breakdownReq.request.params.get('metric')).toBe('LCP');
        expect(breakdownReq.request.params.get('dimension')).toBe('browser');
        expect(breakdownReq.request.params.get('path')).toBe('/pricing');
        expect(breakdownReq.request.params.get('rating')).toBe('poor');
        expect(breakdownReq.request.params.get('limit')).toBe('10');
        breakdownReq.flush([]);
    });

    it('requests AI fetch reports with path filters', () => {
        const filters = { assistantName: 'GPTBot', assistantFamily: 'OpenAI', resourceType: 'html', path: '/docs' };

        service.getAIFetchOverview('site-1', 'from', 'to', filters).subscribe();
        const overviewReq = httpMock.expectOne((request) => request.url === '/api/sites/site-1/ai-fetch/overview');
        expect(overviewReq.request.params.get('assistant_name')).toBe('GPTBot');
        expect(overviewReq.request.params.get('assistant_family')).toBe('OpenAI');
        expect(overviewReq.request.params.get('resource_type')).toBe('html');
        expect(overviewReq.request.params.get('path')).toBe('/docs');
        overviewReq.flush({});

        service.getAIFetchCorrelation('site-1', 'from', 'to', filters).subscribe();
        const correlationReq = httpMock.expectOne((request) => request.url === '/api/sites/site-1/ai-fetch/correlation');
        expect(correlationReq.request.params.get('path')).toBe('/docs');
        correlationReq.flush({ summary: {}, citation_yield: [], opportunity_pages: [], failure_hotspots: [] });
    });
});
