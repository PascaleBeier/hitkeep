import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';

export type WebhookScope = 'instance' | 'site';

export interface WebhookEventDescriptor {
    type: string;
    site_scoped: boolean;
    scopes: WebhookScope[];
}

export interface Webhook {
    id: string;
    site_id?: string;
    scope: WebhookScope;
    name: string;
    description: string;
    url: string;
    enabled: boolean;
    events: string[];
    created_at: string;
    updated_at: string;
}

export interface WebhookInput {
    name: string;
    description: string;
    url: string;
    enabled: boolean;
    events: string[];
}

export interface WebhookSecretResponse {
    webhook: Webhook;
    secret: string;
}

export interface WebhookTestResponse {
    event_id: string;
    delivery_ids: string[];
}

export interface WebhookDeliveryAttempt {
    id: string;
    attempt_number: number;
    status: string;
    response_status?: number;
    error_code?: string;
    error_message?: string;
    started_at: string;
    completed_at: string;
    next_attempt_at?: string;
}

export interface WebhookDelivery {
    id: string;
    event_id: string;
    webhook_id: string;
    site_id?: string;
    event_type: string;
    status: string;
    attempt_count: number;
    next_attempt_at?: string;
    last_attempt_at?: string;
    completed_at?: string;
    response_status?: number;
    last_error_code?: string;
    last_error_message?: string;
    created_at: string;
    attempts: WebhookDeliveryAttempt[];
}

@Injectable({ providedIn: 'root' })
export class WebhooksService {
    private readonly http = inject(HttpClient);

    catalog(scope: WebhookScope, siteID?: string): Observable<WebhookEventDescriptor[]> {
        return this.http.get<WebhookEventDescriptor[]>(`${this.basePath(scope, siteID)}/events`);
    }

    list(scope: WebhookScope, siteID?: string): Observable<Webhook[]> {
        return this.http.get<Webhook[]>(this.basePath(scope, siteID));
    }

    create(input: WebhookInput, scope: WebhookScope, siteID?: string): Observable<WebhookSecretResponse> {
        return this.http.post<WebhookSecretResponse>(this.basePath(scope, siteID), input);
    }

    update(webhookID: string, input: WebhookInput, scope: WebhookScope, siteID?: string): Observable<Webhook> {
        return this.http.put<Webhook>(this.webhookPath(webhookID, scope, siteID), input);
    }

    rotate(webhookID: string, scope: WebhookScope, siteID?: string): Observable<WebhookSecretResponse> {
        return this.http.post<WebhookSecretResponse>(`${this.webhookPath(webhookID, scope, siteID)}/rotate`, {});
    }

    test(webhookID: string, scope: WebhookScope, siteID?: string): Observable<WebhookTestResponse> {
        return this.http.post<WebhookTestResponse>(`${this.webhookPath(webhookID, scope, siteID)}/test`, {});
    }

    deliveries(webhookID: string, scope: WebhookScope, siteID?: string): Observable<WebhookDelivery[]> {
        return this.http.get<WebhookDelivery[]>(`${this.webhookPath(webhookID, scope, siteID)}/deliveries`);
    }

    delete(webhookID: string, scope: WebhookScope, siteID?: string): Observable<void> {
        return this.http.delete<void>(this.webhookPath(webhookID, scope, siteID));
    }

    private webhookPath(webhookID: string, scope: WebhookScope, siteID?: string): string {
        return `${this.basePath(scope, siteID)}/${encodeURIComponent(webhookID)}`;
    }

    private basePath(scope: WebhookScope, siteID?: string): string {
        if (scope === 'instance') return '/api/admin/webhooks';
        if (!siteID) throw new Error('siteID is required for site webhooks');
        return `/api/sites/${encodeURIComponent(siteID)}/webhooks`;
    }
}
