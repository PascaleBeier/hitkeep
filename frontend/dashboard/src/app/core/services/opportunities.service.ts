import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable, map } from 'rxjs';
import { ShareService } from './share.service';

export type OpportunityType = 'conversion' | 'traffic' | 'ai' | 'search' | 'setup';
export type OpportunityStatus = 'new' | 'saved' | 'done' | 'dismissed';
export type OpportunityConfidence = 'high' | 'medium';
export type OpportunityDigestFrequency = 'daily' | 'weekly';

export interface OpportunityEvidence {
    id: string;
    label_key: string;
    value: string;
    detail_key?: string;
    detail_params?: Record<string, unknown>;
}

export interface OpportunityScoreBreakdown {
    sample: number;
    impact: number;
    urgency: number;
    effort: number;
    actionability: number;
    evidence_fit: number;
    freshness: number;
    total: number;
}

export interface Opportunity {
    id: string;
    site_id: string;
    kind: OpportunityType;
    type_key: string;
    title_key: string;
    summary_key: string;
    action_key: string;
    digest_key: string;
    copy_params: Record<string, unknown>;
    impact_value: string;
    impact_label_key: string;
    confidence: OpportunityConfidence;
    score: number;
    score_breakdown: OpportunityScoreBreakdown;
    status: OpportunityStatus;
    route_label_key: string;
    route_params: Record<string, unknown>;
    route_icon: string;
    detector_version: string;
    evidence: OpportunityEvidence[];
    cited_evidence_ids: string[];
    generated_at: string;
    created_at: string;
    updated_at: string;
}

export interface OpportunityListResponse {
    opportunities: Opportunity[];
}

export interface OpportunityGenerateResponse {
    opportunities: Opportunity[];
    ai_status: string;
}

export interface OpportunityDigestPreviewItem {
    id: string;
    site_id: string;
    kind: OpportunityType;
    type_key: string;
    category: string;
    title_key: string;
    action_key: string;
    digest_key: string;
    copy_params: Record<string, unknown>;
    impact_value: string;
    impact_label_key: string;
    confidence: OpportunityConfidence;
    score: number;
    score_breakdown: OpportunityScoreBreakdown;
    status: Extract<OpportunityStatus, 'new' | 'saved'>;
    route_label_key: string;
    route_params: Record<string, unknown>;
    route_icon: string;
    evidence: OpportunityEvidence[];
    cited_evidence_ids: string[];
}

export interface OpportunityDigestPreviewResponse {
    frequency: OpportunityDigestFrequency;
    should_send: boolean;
    reason: 'ready' | 'no_opportunities' | 'unsupported_frequency';
    items: OpportunityDigestPreviewItem[];
}

type OpportunityPayload = Omit<Opportunity, 'copy_params' | 'score_breakdown' | 'route_params' | 'evidence' | 'cited_evidence_ids'> & {
    copy_params?: Record<string, unknown> | null;
    score_breakdown?: OpportunityScoreBreakdown | null;
    route_params?: Record<string, unknown> | null;
    evidence?: OpportunityEvidence[] | null;
    cited_evidence_ids?: string[] | null;
};

type OpportunityDigestPreviewItemPayload = Omit<OpportunityDigestPreviewItem, 'copy_params' | 'score_breakdown' | 'route_params' | 'evidence' | 'cited_evidence_ids'> & {
    copy_params?: Record<string, unknown> | null;
    score_breakdown?: OpportunityScoreBreakdown | null;
    route_params?: Record<string, unknown> | null;
    evidence?: OpportunityEvidence[] | null;
    cited_evidence_ids?: string[] | null;
};

type OpportunityListPayload = Omit<OpportunityListResponse, 'opportunities'> & {
    opportunities?: (OpportunityPayload | null)[] | null;
};

type OpportunityGeneratePayload = Omit<OpportunityGenerateResponse, 'opportunities'> & {
    opportunities?: (OpportunityPayload | null)[] | null;
};

type OpportunityDigestPreviewPayload = Omit<OpportunityDigestPreviewResponse, 'items'> & {
    items?: (OpportunityDigestPreviewItemPayload | null)[] | null;
};

@Injectable({ providedIn: 'root' })
export class OpportunitiesService {
    private readonly http = inject(HttpClient);
    private readonly shareService = inject(ShareService);

    list(siteId: string): Observable<OpportunityListResponse> {
        const shareToken = this.shareService.token();
        if (shareToken) {
            return this.http.get<OpportunityListPayload>(`/api/share/${shareToken}/sites/${siteId}/opportunities`).pipe(map((response) => this.normalizeOpportunityList(response)));
        }
        return this.http.get<OpportunityListPayload>(`/api/sites/${siteId}/opportunities`).pipe(map((response) => this.normalizeOpportunityList(response)));
    }

    generate(siteId: string, from: string, to: string): Observable<OpportunityGenerateResponse> {
        const params = new HttpParams().set('from', from).set('to', to);
        return this.http.post<OpportunityGeneratePayload>(`/api/sites/${siteId}/opportunities/generate`, {}, { params }).pipe(map((response) => this.normalizeOpportunityGenerate(response)));
    }

    previewDigest(siteId: string, frequency: OpportunityDigestFrequency): Observable<OpportunityDigestPreviewResponse> {
        const params = new HttpParams().set('frequency', frequency);
        return this.http.get<OpportunityDigestPreviewPayload>(`/api/sites/${siteId}/opportunities/digest-preview`, { params }).pipe(map((response) => this.normalizeDigestPreview(response)));
    }

    updateStatus(siteId: string, opportunityId: string, status: OpportunityStatus): Observable<Opportunity> {
        return this.http.patch<OpportunityPayload>(`/api/sites/${siteId}/opportunities/${opportunityId}`, { status }).pipe(map((response) => this.normalizeOpportunity(response)));
    }

    private normalizeOpportunityList(response: OpportunityListPayload): OpportunityListResponse {
        return {
            ...response,
            opportunities: this.normalizeOpportunities(response.opportunities)
        };
    }

    private normalizeOpportunityGenerate(response: OpportunityGeneratePayload): OpportunityGenerateResponse {
        return {
            ...response,
            opportunities: this.normalizeOpportunities(response.opportunities)
        };
    }

    private normalizeDigestPreview(response: OpportunityDigestPreviewPayload): OpportunityDigestPreviewResponse {
        return {
            ...response,
            items: this.normalizeDigestPreviewItems(response.items)
        };
    }

    private normalizeOpportunities(opportunities: (OpportunityPayload | null)[] | null | undefined): Opportunity[] {
        if (!Array.isArray(opportunities)) {
            return [];
        }
        return opportunities.filter(isPayloadObject).map((opportunity) => this.normalizeOpportunity(opportunity));
    }

    private normalizeOpportunity(opportunity: OpportunityPayload): Opportunity {
        return {
            ...opportunity,
            copy_params: normalizeRecord(opportunity.copy_params),
            score_breakdown: opportunity.score_breakdown ?? emptyScoreBreakdown(),
            route_params: normalizeRecord(opportunity.route_params),
            evidence: Array.isArray(opportunity.evidence) ? opportunity.evidence : [],
            cited_evidence_ids: Array.isArray(opportunity.cited_evidence_ids) ? opportunity.cited_evidence_ids : []
        };
    }

    private normalizeDigestPreviewItems(items: (OpportunityDigestPreviewItemPayload | null)[] | null | undefined): OpportunityDigestPreviewItem[] {
        if (!Array.isArray(items)) {
            return [];
        }
        return items.filter(isPayloadObject).map((item) => ({
            ...item,
            copy_params: normalizeRecord(item.copy_params),
            score_breakdown: item.score_breakdown ?? emptyScoreBreakdown(),
            route_params: normalizeRecord(item.route_params),
            evidence: Array.isArray(item.evidence) ? item.evidence : [],
            cited_evidence_ids: Array.isArray(item.cited_evidence_ids) ? item.cited_evidence_ids : []
        }));
    }
}

function isPayloadObject<T extends object>(value: T | null | undefined): value is T {
    return typeof value === 'object' && value !== null;
}

function normalizeRecord(value: Record<string, unknown> | null | undefined): Record<string, unknown> {
    if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
        return value;
    }
    return {};
}

function emptyScoreBreakdown(): OpportunityScoreBreakdown {
    return {
        sample: 0,
        impact: 0,
        urgency: 0,
        effort: 0,
        actionability: 0,
        evidence_fit: 0,
        freshness: 0,
        total: 0
    };
}
