import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';

import { OpportunitiesService } from './opportunities.service';
import { ShareService } from './share.service';

describe('OpportunitiesService', () => {
    let service: OpportunitiesService;
    let shareService: ShareService;
    let httpMock: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [provideHttpClient(), provideHttpClientTesting()]
        });
        service = TestBed.inject(OpportunitiesService);
        shareService = TestBed.inject(ShareService);
        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => {
        shareService.clear();
        httpMock.verify();
    });

    it('lists opportunities through the authenticated site endpoint by default', () => {
        service.list('site-1').subscribe();

        const req = httpMock.expectOne('/api/sites/site-1/opportunities');
        expect(req.request.method).toBe('GET');
        req.flush({ opportunities: [] });
    });

    it('lists opportunities through the read-only share endpoint in share mode', () => {
        shareService.setToken('share-token');

        service.list('site-1').subscribe();

        const req = httpMock.expectOne('/api/share/share-token/sites/site-1/opportunities');
        expect(req.request.method).toBe('GET');
        req.flush({ opportunities: [] });
    });

    it('previews the weekly opportunity digest through the authenticated site endpoint', () => {
        service.previewDigest('site-1', 'weekly').subscribe();

        const req = httpMock.expectOne('/api/sites/site-1/opportunities/digest-preview?frequency=weekly');
        expect(req.request.method).toBe('GET');
        req.flush({ frequency: 'weekly', should_send: false, reason: 'no_opportunities', items: [] });
    });
});
