import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';

import { WebhooksService } from './webhooks.service';

describe('WebhooksService', () => {
    let service: WebhooksService;
    let http: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
        service = TestBed.inject(WebhooksService);
        http = TestBed.inject(HttpTestingController);
    });

    afterEach(() => http.verify());

    it('uses site-scoped management and delivery endpoints', () => {
        service.list('site', 'site-1').subscribe();
        http.expectOne('/api/sites/site-1/webhooks').flush([]);

        service.test('webhook-1', 'site', 'site-1').subscribe();
        const testReq = http.expectOne('/api/sites/site-1/webhooks/webhook-1/test');
        expect(testReq.request.method).toBe('POST');
        testReq.flush({ event_id: 'event-1', delivery_ids: ['delivery-1'] });

        service.deliveries('webhook-1', 'site', 'site-1').subscribe();
        http.expectOne('/api/sites/site-1/webhooks/webhook-1/deliveries').flush([]);
    });

    it('uses instance-scoped management endpoints', () => {
        service.catalog('instance').subscribe();
        http.expectOne('/api/admin/webhooks/events').flush([]);

        service.rotate('webhook-1', 'instance').subscribe();
        const rotateReq = http.expectOne('/api/admin/webhooks/webhook-1/rotate');
        expect(rotateReq.request.method).toBe('POST');
        rotateReq.flush({ webhook: null, secret: 'whsec_new' });
    });
});
