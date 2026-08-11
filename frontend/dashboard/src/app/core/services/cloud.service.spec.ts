import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';

import { CloudService, CloudSignupVerificationResendResponse } from '@services/cloud.service';

describe('CloudService', () => {
    let service: CloudService;
    let httpMock: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [CloudService, provideHttpClient(), provideHttpClientTesting()]
        });
        service = TestBed.inject(CloudService);
        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('requests a privacy-neutral signup verification resend', () => {
        const response: CloudSignupVerificationResendResponse = {
            status: 'accepted',
            retry_after_seconds: 300
        };
        let actual: CloudSignupVerificationResendResponse | undefined;

        service.resendSignupVerification('user@example.com').subscribe((value) => (actual = value));

        const req = httpMock.expectOne('/api/cloud/signup/resend-verification');
        expect(req.request.method).toBe('POST');
        expect(req.request.body).toEqual({ email: 'user@example.com' });
        req.flush(response);
        expect(actual).toEqual(response);
    });
});
