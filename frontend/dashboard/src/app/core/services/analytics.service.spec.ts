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

    it('requests the unified AI activity report with repeatable filters and comparison window', () => {
        service
            .getAIActivity(
                'site-1',
                '2026-07-01T00:00:00Z',
                '2026-07-08T00:00:00Z',
                [
                    { type: 'ai_bot', value: 'GPTBot' },
                    { type: 'path', value: '/docs' }
                ],
                { from: '2026-06-24T00:00:00Z', to: '2026-07-01T00:00:00Z' }
            )
            .subscribe();

        const req = httpMock.expectOne((request) => request.url === '/api/sites/site-1/ai-activity');
        expect(req.request.method).toBe('GET');
        expect(req.request.params.get('from')).toBe('2026-07-01T00:00:00Z');
        expect(req.request.params.get('to')).toBe('2026-07-08T00:00:00Z');
        expect(req.request.params.getAll('filter')).toEqual(['ai_bot:GPTBot', 'path:/docs']);
        expect(req.request.params.get('compare_from')).toBe('2026-06-24T00:00:00Z');
        expect(req.request.params.get('compare_to')).toBe('2026-07-01T00:00:00Z');
        req.flush({});
    });

    it('omits filter and comparison params from an unfiltered AI activity request', () => {
        service.getAIActivity('site-1', 'from', 'to').subscribe();

        const req = httpMock.expectOne((request) => request.url === '/api/sites/site-1/ai-activity');
        expect(req.request.params.has('filter')).toBe(false);
        expect(req.request.params.has('compare_from')).toBe(false);
        expect(req.request.params.has('compare_to')).toBe(false);
        req.flush({});
    });

    it('updates goals and funnels in place and sends cohort ids to hit queries', () => {
        service.updateGoal('site-1', 'goal-1', { name: 'Signup', type: 'event', value: 'signup' }).subscribe();
        const goalReq = httpMock.expectOne('/api/sites/site-1/goals/goal-1');
        expect(goalReq.request.method).toBe('PUT');
        goalReq.flush({ id: 'goal-1' });

        service.updateFunnel('site-1', 'funnel-1', { name: 'Checkout', steps: [] }).subscribe();
        const funnelReq = httpMock.expectOne('/api/sites/site-1/funnels/funnel-1');
        expect(funnelReq.request.method).toBe('PUT');
        funnelReq.flush({ id: 'funnel-1' });

        service.getHits('site-1', 'from', 'to', 1, 10, undefined, undefined, undefined, ['goal-1'], ['funnel-1']).subscribe();
        const hitsReq = httpMock.expectOne((request) => request.url === '/api/sites/site-1/hits');
        expect(hitsReq.request.params.getAll('goal_id')).toEqual(['goal-1']);
        expect(hitsReq.request.params.getAll('funnel_id')).toEqual(['funnel-1']);
        hitsReq.flush({ data: [], total: 0 });
    });
});
