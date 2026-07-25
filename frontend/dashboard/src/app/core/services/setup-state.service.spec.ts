import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';

import { SetupStateService, SiteSetupState } from '@services/setup-state.service';

const setupState = (overrides: Partial<SiteSetupState> = {}): SiteSetupState => ({
    has_ai_fetches: false,
    has_chatbot_events: false,
    has_custom_events: false,
    has_ecommerce_events: false,
    has_web_vitals: false,
    ...overrides
});

describe('SetupStateService', () => {
    let service: SetupStateService;
    let httpMock: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [provideHttpClient(), provideHttpClientTesting()]
        });
        service = TestBed.inject(SetupStateService);
        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('returns no state while the shared lookup is unknown and caches every flag once it answers', () => {
        expect(service.state('site-1')).toBeNull();
        expect(service.state('site-1')).toBeNull();

        const req = httpMock.expectOne('/api/sites/site-1/setup-state');
        expect(req.request.method).toBe('GET');
        req.flush(setupState({ has_ai_fetches: true }));

        expect(service.state('site-1')).toEqual(setupState({ has_ai_fetches: true }));
        httpMock.expectNone('/api/sites/site-1/setup-state');
    });

    it('reports a fully configured site when the generic lookup fails', () => {
        expect(service.state('site-1')).toBeNull();
        httpMock.expectOne('/api/sites/site-1/setup-state').flush(null, { status: 503, statusText: 'Service Unavailable' });

        expect(service.state('site-1')).toEqual(
            setupState({
                has_ai_fetches: true,
                has_chatbot_events: true,
                has_custom_events: true,
                has_ecommerce_events: true,
                has_web_vitals: true
            })
        );
    });

    it('stays quiet without a site', () => {
        expect(service.state(null)).toBeNull();
        expect(service.state(undefined)).toBeNull();

        httpMock.expectNone('/api/sites/site-1/setup-state');
    });

    it('shares one lookup between the generic state and the needsSetup shorthand', () => {
        expect(service.state('site-1')).toBeNull();
        expect(service.needsSetup('site-1', 'has_ai_fetches', 0, false)).toBe(false);

        httpMock.expectOne('/api/sites/site-1/setup-state').flush(setupState());

        expect(service.needsSetup('site-1', 'has_ai_fetches', 0, false)).toBe(true);
        expect(service.state('site-1')?.has_ai_fetches).toBe(false);
    });

    it('asks the site once and reports a missing setup for the feature that never had data', () => {
        expect(service.needsSetup('site-1', 'has_ai_fetches', 0, false)).toBe(false);

        const req = httpMock.expectOne('/api/sites/site-1/setup-state');
        expect(req.request.method).toBe('GET');
        req.flush(setupState({ has_chatbot_events: true }));

        expect(service.needsSetup('site-1', 'has_ai_fetches', 0, false)).toBe(true);
        expect(service.needsSetup('site-1', 'has_chatbot_events', 0, false)).toBe(false);
        httpMock.expectNone('/api/sites/site-1/setup-state');
    });

    it('never asks while the range still has data', () => {
        expect(service.needsSetup('site-1', 'has_web_vitals', 7, false)).toBe(false);

        httpMock.expectNone('/api/sites/site-1/setup-state');
    });

    it('stays quiet without a site, while loading and before any numbers arrived', () => {
        expect(service.needsSetup(null, 'has_custom_events', 0, false)).toBe(false);
        expect(service.needsSetup('site-1', 'has_custom_events', 0, true)).toBe(false);
        expect(service.needsSetup('site-1', 'has_custom_events', null, false)).toBe(false);

        httpMock.expectNone('/api/sites/site-1/setup-state');
    });

    it('deduplicates concurrent lookups for the same site', () => {
        service.needsSetup('site-1', 'has_ecommerce_events', 0, false);
        service.needsSetup('site-1', 'has_web_vitals', 0, false);

        const req = httpMock.expectOne('/api/sites/site-1/setup-state');
        req.flush(setupState());

        expect(service.needsSetup('site-1', 'has_ecommerce_events', 0, false)).toBe(true);
        expect(service.needsSetup('site-1', 'has_web_vitals', 0, false)).toBe(true);
    });

    it('caches per site and looks up every site separately', () => {
        service.needsSetup('site-1', 'has_ai_fetches', 0, false);
        httpMock.expectOne('/api/sites/site-1/setup-state').flush(setupState());

        service.needsSetup('site-2', 'has_ai_fetches', 0, false);
        httpMock.expectOne('/api/sites/site-2/setup-state').flush(setupState({ has_ai_fetches: true }));

        expect(service.needsSetup('site-1', 'has_ai_fetches', 0, false)).toBe(true);
        expect(service.needsSetup('site-2', 'has_ai_fetches', 0, false)).toBe(false);
    });

    it('treats a failed lookup as a fully configured site and never retries it', () => {
        service.needsSetup('site-1', 'has_ai_fetches', 0, false);
        httpMock.expectOne('/api/sites/site-1/setup-state').flush(null, { status: 503, statusText: 'Service Unavailable' });

        expect(service.needsSetup('site-1', 'has_ai_fetches', 0, false)).toBe(false);
        expect(service.needsSetup('site-1', 'has_web_vitals', 0, false)).toBe(false);
        httpMock.expectNone('/api/sites/site-1/setup-state');
    });

    it('treats a malformed payload as a fully configured site', () => {
        service.needsSetup('site-1', 'has_custom_events', 0, false);
        httpMock.expectOne('/api/sites/site-1/setup-state').flush(null);

        expect(service.needsSetup('site-1', 'has_custom_events', 0, false)).toBe(false);
    });
});
