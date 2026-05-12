import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { ShareService } from './share.service';

export type OpportunityType = 'conversion' | 'revenue' | 'ai' | 'search' | 'setup';
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
    monthly_upside: number;
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

@Injectable({ providedIn: 'root' })
export class OpportunitiesService {
    private readonly http = inject(HttpClient);
    private readonly shareService = inject(ShareService);

    list(siteId: string): Observable<OpportunityListResponse> {
        const shareToken = this.shareService.token();
        if (shareToken) {
            return this.http.get<OpportunityListResponse>(`/api/share/${shareToken}/sites/${siteId}/opportunities`);
        }
        return this.http.get<OpportunityListResponse>(`/api/sites/${siteId}/opportunities`);
    }

    generate(siteId: string, from: string, to: string): Observable<OpportunityGenerateResponse> {
        const params = new HttpParams().set('from', from).set('to', to);
        return this.http.post<OpportunityGenerateResponse>(`/api/sites/${siteId}/opportunities/generate`, {}, { params });
    }

    previewDigest(siteId: string, frequency: OpportunityDigestFrequency): Observable<OpportunityDigestPreviewResponse> {
        const params = new HttpParams().set('frequency', frequency);
        return this.http.get<OpportunityDigestPreviewResponse>(`/api/sites/${siteId}/opportunities/digest-preview`, { params });
    }

    updateStatus(siteId: string, opportunityId: string, status: OpportunityStatus): Observable<Opportunity> {
        return this.http.patch<Opportunity>(`/api/sites/${siteId}/opportunities/${opportunityId}`, { status });
    }
}
