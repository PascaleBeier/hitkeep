import { TestBed } from '@angular/core/testing';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideHttpClient } from '@angular/common/http';
import { StatsService } from '@features/analytics/services/stats.service';

describe('StatsService', () => {
    let service: StatsService;
    let httpMock: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [StatsService, provideHttpClient(), provideHttpClientTesting()]
        });
        service = TestBed.inject(StatsService);
        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('should be created', () => {
        expect(service).toBeTruthy();
    });

    it('requests lightweight overview stats for all accessible sites', () => {
        service.fetchSitesOverviewStats('2026-07-01T00:00:00.000Z', '2026-07-02T00:00:00.000Z').subscribe((response) => {
            expect(response.sites).toEqual([]);
        });

        const req = httpMock.expectOne((request) => request.method === 'GET' && request.url === '/api/sites/overview');
        expect(req.request.params.get('from')).toBe('2026-07-01T00:00:00.000Z');
        expect(req.request.params.get('to')).toBe('2026-07-02T00:00:00.000Z');
        expect(req.request.params.has('compare_from')).toBe(false);
        req.flush({ sites: [] });
    });
});
