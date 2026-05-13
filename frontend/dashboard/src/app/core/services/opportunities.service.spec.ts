import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';

import { OpportunitiesService, type Opportunity, type OpportunityDigestPreviewItem } from './opportunities.service';
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

    it('normalizes nullable list opportunities to an empty array', () => {
        let count = -1;
        service.list('site-1').subscribe((response) => {
            count = response.opportunities.length;
        });

        const req = httpMock.expectOne('/api/sites/site-1/opportunities');
        req.flush({ opportunities: null });

        expect(count).toBe(0);
    });

    it('normalizes nullable nested opportunity collections in list responses', () => {
        let opportunity: Opportunity | undefined;
        service.list('site-1').subscribe((response) => {
            opportunity = response.opportunities[0];
        });

        const req = httpMock.expectOne('/api/sites/site-1/opportunities');
        req.flush({
            opportunities: [
                opportunityFixture({
                    copy_params: null,
                    route_params: null,
                    evidence: null,
                    cited_evidence_ids: null
                })
            ]
        });

        expect(opportunity?.copy_params).toEqual({});
        expect(opportunity?.route_params).toEqual({});
        expect(opportunity?.evidence).toEqual([]);
        expect(opportunity?.cited_evidence_ids).toEqual([]);
    });

    it('normalizes nullable generated opportunities to an empty array', () => {
        let count = -1;
        service.generate('site-1', '2026-05-01T00:00:00Z', '2026-05-12T00:00:00Z').subscribe((response) => {
            count = response.opportunities.length;
        });

        const req = httpMock.expectOne('/api/sites/site-1/opportunities/generate?from=2026-05-01T00:00:00Z&to=2026-05-12T00:00:00Z');
        expect(req.request.method).toBe('POST');
        req.flush({ opportunities: null, ai_status: 'success' });

        expect(count).toBe(0);
    });

    it('normalizes nullable nested opportunity collections in generate responses', () => {
        let opportunity: Opportunity | undefined;
        service.generate('site-1', '2026-05-01T00:00:00Z', '2026-05-12T00:00:00Z').subscribe((response) => {
            opportunity = response.opportunities[0];
        });

        const req = httpMock.expectOne('/api/sites/site-1/opportunities/generate?from=2026-05-01T00:00:00Z&to=2026-05-12T00:00:00Z');
        req.flush({
            opportunities: [
                opportunityFixture({
                    copy_params: null,
                    route_params: null,
                    evidence: null,
                    cited_evidence_ids: null
                })
            ],
            ai_status: 'success'
        });

        expect(opportunity?.copy_params).toEqual({});
        expect(opportunity?.route_params).toEqual({});
        expect(opportunity?.evidence).toEqual([]);
        expect(opportunity?.cited_evidence_ids).toEqual([]);
    });

    it('previews the weekly opportunity digest through the authenticated site endpoint', () => {
        service.previewDigest('site-1', 'weekly').subscribe();

        const req = httpMock.expectOne('/api/sites/site-1/opportunities/digest-preview?frequency=weekly');
        expect(req.request.method).toBe('GET');
        req.flush({ frequency: 'weekly', should_send: false, reason: 'no_opportunities', items: [] });
    });

    it('normalizes nullable digest preview items to an empty array', () => {
        let count = -1;
        service.previewDigest('site-1', 'weekly').subscribe((response) => {
            count = response.items.length;
        });

        const req = httpMock.expectOne('/api/sites/site-1/opportunities/digest-preview?frequency=weekly');
        req.flush({ frequency: 'weekly', should_send: false, reason: 'no_opportunities', items: null });

        expect(count).toBe(0);
    });

    it('normalizes nullable nested opportunity collections in digest preview items', () => {
        let item: OpportunityDigestPreviewItem | undefined;
        service.previewDigest('site-1', 'weekly').subscribe((response) => {
            item = response.items[0];
        });

        const req = httpMock.expectOne('/api/sites/site-1/opportunities/digest-preview?frequency=weekly');
        req.flush({
            frequency: 'weekly',
            should_send: true,
            reason: 'ready',
            items: [
                digestItemFixture({
                    copy_params: null,
                    route_params: null,
                    evidence: null,
                    cited_evidence_ids: null
                })
            ]
        });

        expect(item?.copy_params).toEqual({});
        expect(item?.route_params).toEqual({});
        expect(item?.evidence).toEqual([]);
        expect(item?.cited_evidence_ids).toEqual([]);
    });

    it('normalizes nullable nested opportunity collections in status update responses', () => {
        let opportunity: Opportunity | undefined;
        service.updateStatus('site-1', 'op-1', 'done').subscribe((response) => {
            opportunity = response;
        });

        const req = httpMock.expectOne('/api/sites/site-1/opportunities/op-1');
        expect(req.request.method).toBe('PATCH');
        req.flush(
            opportunityFixture({
                status: 'done',
                copy_params: null,
                route_params: null,
                evidence: null,
                cited_evidence_ids: null
            })
        );

        expect(opportunity?.copy_params).toEqual({});
        expect(opportunity?.route_params).toEqual({});
        expect(opportunity?.evidence).toEqual([]);
        expect(opportunity?.cited_evidence_ids).toEqual([]);
    });
});

function opportunityFixture(overrides: Partial<Record<string, unknown>> = {}) {
    return {
        id: 'op-1',
        site_id: 'site-1',
        kind: 'conversion',
        type_key: 'opportunities.types.checkout_conversion',
        title_key: 'opportunities.catalog.checkout_conversion.title',
        summary_key: 'opportunities.catalog.checkout_conversion.summary',
        action_key: 'opportunities.catalog.checkout_conversion.action',
        digest_key: 'opportunities.catalog.checkout_conversion.digest',
        copy_params: { conversion_rate: '42%' },
        impact_value: '120',
        impact_label_key: 'opportunities.impact.checkout_starts',
        confidence: 'medium',
        score: 82,
        score_breakdown: scoreBreakdownFixture(),
        status: 'new',
        route_label_key: 'opportunities.routes.checkout',
        route_params: { path: '/checkout' },
        route_icon: 'pi pi-arrow-right',
        detector_version: 'opportunities-detectors-v1',
        evidence: [{ id: 'conversion_rate', label_key: 'opportunities.evidence.checkout_conversion_rate', value: '42%' }],
        cited_evidence_ids: ['conversion_rate'],
        generated_at: '2026-05-09T10:00:00Z',
        created_at: '2026-05-09T10:00:00Z',
        updated_at: '2026-05-09T10:00:00Z',
        ...overrides
    };
}

function digestItemFixture(overrides: Partial<Record<string, unknown>> = {}) {
    return {
        id: 'digest-op-1',
        site_id: 'site-1',
        kind: 'conversion',
        type_key: 'opportunities.types.checkout_conversion',
        category: 'conversion',
        title_key: 'opportunities.catalog.checkout_conversion.title',
        action_key: 'opportunities.catalog.checkout_conversion.action',
        digest_key: 'opportunities.catalog.checkout_conversion.digest',
        copy_params: { conversion_rate: '42%' },
        impact_value: '120',
        impact_label_key: 'opportunities.impact.checkout_starts',
        confidence: 'medium',
        score: 82,
        score_breakdown: scoreBreakdownFixture(),
        status: 'new',
        route_label_key: 'opportunities.routes.checkout',
        route_params: { path: '/checkout' },
        route_icon: 'pi pi-arrow-right',
        evidence: [{ id: 'conversion_rate', label_key: 'opportunities.evidence.checkout_conversion_rate', value: '42%' }],
        cited_evidence_ids: ['conversion_rate'],
        ...overrides
    };
}

function scoreBreakdownFixture() {
    return {
        sample: 82,
        impact: 70,
        urgency: 55,
        effort: 70,
        actionability: 85,
        evidence_fit: 99,
        freshness: 50,
        total: 82
    };
}
