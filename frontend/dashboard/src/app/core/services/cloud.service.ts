import { inject, Service } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';

import { CloudPlanTier } from '@models/analytics.types';

export type CloudPlanCode = 'free' | 'pro' | 'business';
export type BillingInterval = 'monthly' | 'annual';

export interface CloudSignupRequest {
    email: string;
    password: string;
    team_name: string;
    plan_code: CloudPlanCode;
    billing: BillingInterval;
    jurisdiction?: string;
    locale?: string;
    given_name?: string;
    last_name?: string;
    accepted_tos: boolean;
}

export interface CloudSignupResponse {
    status: string;
    plan_code: string;
    billing: BillingInterval;
    redirect_url?: string;
    checkout_url?: string;
}

export interface BillingPortalSessionResponse {
    url: string;
}

export interface BillingPortalSessionRequest {
    locale?: string;
}

export interface BillingCheckoutSessionRequest {
    plan_code: 'pro' | 'business';
    billing: BillingInterval;
    locale?: string;
}

@Service()
export class CloudService {
    private readonly http = inject(HttpClient);

    signup(payload: CloudSignupRequest): Observable<CloudSignupResponse> {
        return this.http.post<CloudSignupResponse>('/api/cloud/signup', payload);
    }

    createBillingPortalSession(payload: BillingPortalSessionRequest = {}): Observable<BillingPortalSessionResponse> {
        return this.http.post<BillingPortalSessionResponse>('/api/cloud/billing/portal', payload);
    }

    createBillingCheckoutSession(payload: BillingCheckoutSessionRequest): Observable<BillingPortalSessionResponse> {
        return this.http.post<BillingPortalSessionResponse>('/api/cloud/billing/checkout', payload);
    }

    getPlans(): Observable<CloudPlanTier[]> {
        return this.http.get<CloudPlanTier[]>('/api/cloud/plans');
    }
}
