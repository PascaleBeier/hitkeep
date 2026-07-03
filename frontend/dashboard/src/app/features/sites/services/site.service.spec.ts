import { TestBed } from '@angular/core/testing';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideHttpClient } from '@angular/common/http';
import { SiteService } from '@features/sites/services/site.service';
import { Site } from '@models/analytics.types';

describe('SiteService', () => {
    let service: SiteService;
    let httpMock: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [SiteService, provideHttpClient(), provideHttpClientTesting()]
        });
        service = TestBed.inject(SiteService);
        httpMock = TestBed.inject(HttpTestingController);
        localStorage.clear();
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('should be created', () => {
        expect(service).toBeTruthy();
    });

    it('should clear active site when the active team has no sites', () => {
        service.activeSite.set(site('site-1', 'example.com'));

        service.loadSites();

        const req = httpMock.expectOne('/api/sites');
        expect(req.request.method).toBe('GET');
        req.flush([]);

        expect(service.activeSite()).toBeNull();
        expect(service.sites()).toEqual([]);
    });

    it('exposes sites alphabetically by domain', () => {
        service.applySites([site('site-zeta', 'zeta.example.com'), site('site-alpha', 'alpha.example.com'), site('site-2', 'site2.example.com'), site('site-10', 'site10.example.com')]);

        expect(service.sites().map((entry) => entry.domain)).toEqual(['alpha.example.com', 'site2.example.com', 'site10.example.com', 'zeta.example.com']);
    });

    it('preserves API-order fallback active site while exposing an alphabetized list', () => {
        service.applySites([site('site-zeta', 'zeta.example.com'), site('site-alpha', 'alpha.example.com')]);

        expect(service.sites().map((entry) => entry.id)).toEqual(['site-alpha', 'site-zeta']);
        expect(service.activeSite()?.id).toBe('site-zeta');
    });

    it('adds created sites into the alphabetized list and selects the new site', () => {
        service.applySites([site('site-zeta', 'zeta.example.com'), site('site-alpha', 'alpha.example.com')]);

        service.createSite('middle.example.com').subscribe();

        const req = httpMock.expectOne('/api/sites');
        expect(req.request.method).toBe('POST');
        req.flush(site('site-middle', 'middle.example.com'));

        expect(service.sites().map((entry) => entry.domain)).toEqual(['alpha.example.com', 'middle.example.com', 'zeta.example.com']);
        expect(service.activeSite()?.id).toBe('site-middle');
    });

    it('posts reset confirmation to the site stats reset endpoint', () => {
        service.resetSiteStats('site-1', 'example.com').subscribe((response) => {
            expect(response).toEqual({
                status: 'reset',
                rows_cleared: 12,
                imports_marked_deleted: 1,
                families_cleared: ['native']
            });
        });

        const req = httpMock.expectOne('/api/sites/site-1/stats/reset');
        expect(req.request.method).toBe('POST');
        expect(req.request.body).toEqual({ confirm_domain: 'example.com' });
        req.flush({
            status: 'reset',
            rows_cleared: 12,
            imports_marked_deleted: 1,
            families_cleared: ['native']
        });
    });
});

function site(id: string, domain: string): Site {
    return {
        id,
        user_id: 'user-1',
        domain,
        created_at: '2026-01-01T00:00:00Z'
    };
}
