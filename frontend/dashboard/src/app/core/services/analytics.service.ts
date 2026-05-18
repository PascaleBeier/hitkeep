import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import {
    AIFetchCorrelationReport,
    AIFetchOverview,
    AIFetchSeriesPoint,
    EventAudience,
    EventSeriesPoint,
    EcommerceProductStat,
    EcommerceSeriesPoint,
    EcommerceSourceStat,
    EcommerceSummary,
    FunnelSeriesPoint,
    GoalSeriesPoint,
    SiteStats as SiteStatsModel,
    SystemStatus,
    WebVitalDimension,
    WebVitalDimensionRow,
    WebVitalMetric,
    WebVitalPageRow,
    WebVitalRating,
    WebVitalSeriesPoint,
    WebVitalSummaryMetric
} from '@models/analytics.types';

export interface Site {
    id: string;
    user_id: string;
    domain: string;
    created_at: string;
}

export interface Hit {
    id: string;
    site_id: string;
    session_id: string;
    page_id: string;
    timestamp: string;
    path: string;
    referrer?: string;
    user_agent?: string;
    viewport_width?: number;
    viewport_height?: number;
    language?: string;
    country_code?: string;
    region?: string;
    city?: string;
    provider?: string;
    asn?: number;
    asn_org?: string;
    utm_source?: string;
    utm_medium?: string;
    utm_campaign?: string;
    utm_term?: string;
    utm_content?: string;
    is_unique?: boolean;
}

export interface PaginatedHits {
    data: Hit[];
    total: number;
}

export interface ChartDataPoint {
    time: string;
    pageviews: number;
    visitors: number;
}

export interface ImportExclusionReason {
    reason: string;
    detail?: string;
}

export interface SiteStats {
    live_visitors: number;
    total_pageviews: number;
    unique_sessions: number;
    bounce_rate: number;
    avg_session_duration: number; // Seconds
    pages_per_session: number;
    chart_data: ChartDataPoint[];
    top_pages: MetricStat[];
    top_landing_pages: MetricStat[];
    top_exit_pages: MetricStat[];
    top_referrers: MetricStat[];
    top_devices: MetricStat[];
    top_countries: MetricStat[];
    top_cities?: MetricStat[];
    top_providers?: MetricStat[];
    top_asns?: MetricStat[];
    top_languages: MetricStat[];
    utm_campaign_hits: number;
    utm_content_hits: number;
    utm_medium_hits: number;
    utm_source_hits: number;
    utm_term_hits: number;
    goals: GoalStats[];
    imported_excluded?: ImportExclusionReason[];
}

export interface MetricStat {
    name: string;
    value: number;
}

export interface Goal {
    id: string;
    site_id: string;
    name: string;
    type: 'event' | 'path';
    value: string;
    created_at: string;
}

export interface GoalStats {
    goal_id: string;
    name: string;
    conversions: number;
    conversion_rate: number;
}

export interface FunnelStep {
    type: 'event' | 'path';
    value: string;
}

export interface Funnel {
    id: string;
    site_id: string;
    name: string;
    steps: FunnelStep[];
    created_at: string;
}

export interface FunnelStepStats {
    step_index: number;
    name: string;
    visitors: number;
    dropoff: number;
    conversion_rate: number;
}

export interface FunnelStats {
    funnel_id: string;
    name: string;
    steps: FunnelStepStats[];
    total_entries: number;
    total_completions: number;
    overall_conversion_rate: number;
}

export interface AIFetchFilters {
    assistantName?: string | null;
    assistantFamily?: string | null;
    resourceType?: string | null;
    path?: string | null;
}

export interface EventDimensionFilter {
    type: string;
    value: string;
}

@Injectable({
    providedIn: 'root'
})
export class AnalyticsService {
    private http = inject(HttpClient);

    getSystemStatus(): Observable<SystemStatus> {
        return this.http.get<SystemStatus>('/api/status');
    }

    /**
     * Fetches the list of websites tracked by the user.
     */
    getSites(): Observable<Site[]> {
        return this.http.get<Site[]>('/api/sites');
    }

    /**
     * Creates a new website for the user.
     */
    createSite(domain: string): Observable<Site> {
        return this.http.post<Site>('/api/sites', { domain });
    }

    updateSiteRetention(siteId: string, days: number): Observable<void> {
        return this.http.put<void>(`/api/sites/${siteId}/retention`, { days });
    }

    /**
     * Fetches aggregated statistics and chart data.
     */
    getSiteStats(siteId: string, from?: string, to?: string, compareFrom?: string, compareTo?: string, filters: { type: string; value: string }[] = []): Observable<SiteStatsModel> {
        let params = new HttpParams();
        if (from) params = params.set('from', from);
        if (to) params = params.set('to', to);
        if (compareFrom) params = params.set('compare_from', compareFrom);
        if (compareTo) params = params.set('compare_to', compareTo);
        for (const filter of filters) {
            params = params.append('filter', `${filter.type}:${filter.value}`);
        }

        return this.http.get<SiteStatsModel>(`/api/sites/${siteId}/stats`, { params });
    }

    // Events
    getEventNames(siteId: string, from: string, to: string): Observable<string[]> {
        const params = new HttpParams().set('from', from).set('to', to);
        return this.http.get<string[]>(`/api/sites/${siteId}/events/names`, { params });
    }

    getEventPropertyKeys(siteId: string, from: string, to: string, eventName: string): Observable<string[]> {
        const params = new HttpParams().set('from', from).set('to', to).set('event_name', eventName);
        return this.http.get<string[]>(`/api/sites/${siteId}/events/properties`, { params });
    }

    getEventPropertyBreakdown(siteId: string, from: string, to: string, eventName: string, propertyKey: string): Observable<MetricStat[]> {
        const params = new HttpParams().set('from', from).set('to', to).set('event_name', eventName).set('property_key', propertyKey);
        return this.http.get<MetricStat[]>(`/api/sites/${siteId}/events/breakdown`, { params });
    }

    getEventTimeseries(siteId: string, from: string, to: string, eventName: string, propertyKey?: string, propertyValue?: string, filters: EventDimensionFilter[] = []): Observable<EventSeriesPoint[]> {
        let params = new HttpParams().set('from', from).set('to', to).set('event_name', eventName);
        if (propertyKey) params = params.set('property_key', propertyKey);
        if (propertyValue) params = params.set('property_value', propertyValue);
        for (const filter of filters) {
            params = params.append('filter', `${filter.type}:${filter.value}`);
        }
        return this.http.get<EventSeriesPoint[]>(`/api/sites/${siteId}/events/timeseries`, { params });
    }

    getEventAudience(siteId: string, from: string, to: string, eventName: string, propertyKey?: string, propertyValue?: string, filters: EventDimensionFilter[] = []): Observable<EventAudience> {
        let params = new HttpParams().set('from', from).set('to', to).set('event_name', eventName);
        if (propertyKey) params = params.set('property_key', propertyKey);
        if (propertyValue) params = params.set('property_value', propertyValue);
        for (const filter of filters) {
            params = params.append('filter', `${filter.type}:${filter.value}`);
        }
        return this.http.get<EventAudience>(`/api/sites/${siteId}/events/audience`, { params });
    }

    getEcommerceSummary(siteId: string, from: string, to: string, filters: { type: string; value: string }[] = [], itemId?: string | null, itemName?: string | null): Observable<EcommerceSummary> {
        const params = this.buildEcommerceParams(from, to, filters, itemId, itemName);
        return this.http.get<EcommerceSummary>(`/api/sites/${siteId}/ecommerce`, { params });
    }

    getEcommerceTimeseries(siteId: string, from: string, to: string, filters: { type: string; value: string }[] = [], itemId?: string | null, itemName?: string | null): Observable<EcommerceSeriesPoint[]> {
        const params = this.buildEcommerceParams(from, to, filters, itemId, itemName);
        return this.http.get<EcommerceSeriesPoint[]>(`/api/sites/${siteId}/ecommerce/timeseries`, { params });
    }

    getEcommerceProducts(siteId: string, from: string, to: string, filters: { type: string; value: string }[] = [], itemId?: string | null, itemName?: string | null, limit = 10): Observable<EcommerceProductStat[]> {
        const params = this.buildEcommerceParams(from, to, filters, itemId, itemName).set('limit', limit);
        return this.http.get<EcommerceProductStat[]>(`/api/sites/${siteId}/ecommerce/products`, { params });
    }

    getEcommerceSources(siteId: string, from: string, to: string, filters: { type: string; value: string }[] = [], itemId?: string | null, itemName?: string | null, limit = 10): Observable<EcommerceSourceStat[]> {
        const params = this.buildEcommerceParams(from, to, filters, itemId, itemName).set('limit', limit);
        return this.http.get<EcommerceSourceStat[]>(`/api/sites/${siteId}/ecommerce/sources`, { params });
    }

    getWebVitalsSummary(siteId: string, from: string, to: string, metric?: WebVitalMetric | null, path?: string | null, rating?: WebVitalRating | null): Observable<WebVitalSummaryMetric[]> {
        const params = this.buildWebVitalsParams(from, to, metric, path, rating);
        return this.http.get<WebVitalSummaryMetric[]>(`/api/sites/${siteId}/web-vitals/summary`, { params });
    }

    getWebVitalsTimeseries(siteId: string, from: string, to: string, metric: WebVitalMetric, path?: string | null, rating?: WebVitalRating | null): Observable<WebVitalSeriesPoint[]> {
        const params = this.buildWebVitalsParams(from, to, metric, path, rating);
        return this.http.get<WebVitalSeriesPoint[]>(`/api/sites/${siteId}/web-vitals/timeseries`, { params });
    }

    getWebVitalsPages(siteId: string, from: string, to: string, metric: WebVitalMetric, path?: string | null, rating?: WebVitalRating | null, limit = 25): Observable<WebVitalPageRow[]> {
        const params = this.buildWebVitalsParams(from, to, metric, path, rating).set('limit', limit);
        return this.http.get<WebVitalPageRow[]>(`/api/sites/${siteId}/web-vitals/pages`, { params });
    }

    getWebVitalsBreakdown(siteId: string, from: string, to: string, metric: WebVitalMetric, dimension: WebVitalDimension, path?: string | null, rating?: WebVitalRating | null, limit = 25): Observable<WebVitalDimensionRow[]> {
        const params = this.buildWebVitalsParams(from, to, metric, path, rating).set('dimension', dimension).set('limit', limit);
        return this.http.get<WebVitalDimensionRow[]>(`/api/sites/${siteId}/web-vitals/breakdown`, { params });
    }

    getAIFetchOverview(siteId: string, from: string, to: string, filters: AIFetchFilters = {}): Observable<AIFetchOverview> {
        return this.http.get<AIFetchOverview>(`/api/sites/${siteId}/ai-fetch/overview`, {
            params: this.buildAIFetchParams(from, to, filters)
        });
    }

    getAIFetchTimeseries(siteId: string, from: string, to: string, filters: AIFetchFilters = {}): Observable<AIFetchSeriesPoint[]> {
        return this.http.get<AIFetchSeriesPoint[]>(`/api/sites/${siteId}/ai-fetch/timeseries`, {
            params: this.buildAIFetchParams(from, to, filters)
        });
    }

    getAIFetchCorrelation(siteId: string, from: string, to: string, filters: AIFetchFilters = {}, windowDays?: number): Observable<AIFetchCorrelationReport> {
        let params = this.buildAIFetchParams(from, to, filters);
        if (windowDays) {
            params = params.set('window_days', windowDays);
        }
        return this.http.get<AIFetchCorrelationReport>(`/api/sites/${siteId}/ai-fetch/correlation`, { params });
    }

    /**
     * Fetches raw hits (RESTful nested resource).
     * GET /api/sites/{id}/hits
     */
    getHits(siteId: string, from: string, to: string, page = 1, pageSize = 10, sortField?: string, sortOrder?: string, query?: string): Observable<PaginatedHits> {
        let params = new HttpParams()
            .set('from', from)
            .set('to', to)
            .set('limit', pageSize)
            .set('offset', (page - 1) * pageSize);

        if (sortField) params = params.set('sort', sortField);
        if (sortOrder) params = params.set('order', sortOrder);
        if (query) params = params.set('q', query);

        return this.http.get<PaginatedHits>(`/api/sites/${siteId}/hits`, { params });
    }

    // Goals
    getGoals(siteId: string): Observable<Goal[]> {
        return this.http.get<Goal[]>(`/api/sites/${siteId}/goals`);
    }

    getGoalTimeseries(siteId: string, from?: string, to?: string, goalIds: string[] = []): Observable<GoalSeriesPoint[]> {
        let params = new HttpParams();
        if (from) params = params.set('from', from);
        if (to) params = params.set('to', to);
        for (const id of goalIds) {
            params = params.append('goal_id', id);
        }
        return this.http.get<GoalSeriesPoint[]>(`/api/sites/${siteId}/goals/timeseries`, { params });
    }

    createGoal(siteId: string, goal: Partial<Goal>): Observable<void> {
        return this.http.post<void>(`/api/sites/${siteId}/goals`, goal);
    }

    deleteGoal(siteId: string, goalId: string): Observable<void> {
        return this.http.delete<void>(`/api/sites/${siteId}/goals/${goalId}`);
    }

    // Funnels
    getFunnels(siteId: string): Observable<Funnel[]> {
        return this.http.get<Funnel[]>(`/api/sites/${siteId}/funnels`);
    }

    private buildAIFetchParams(from: string, to: string, filters: AIFetchFilters): HttpParams {
        let params = new HttpParams().set('from', from).set('to', to);
        if (filters.assistantName) {
            params = params.set('assistant_name', filters.assistantName);
        }
        if (filters.assistantFamily) {
            params = params.set('assistant_family', filters.assistantFamily);
        }
        if (filters.resourceType) {
            params = params.set('resource_type', filters.resourceType);
        }
        if (filters.path) {
            params = params.set('path', filters.path);
        }
        return params;
    }

    private buildEcommerceParams(from: string, to: string, filters: { type: string; value: string }[] = [], itemId?: string | null, itemName?: string | null): HttpParams {
        let params = new HttpParams().set('from', from).set('to', to);
        for (const filter of filters) {
            params = params.append('filter', `${filter.type}:${filter.value}`);
        }
        if (itemId) {
            params = params.set('item_id', itemId);
        }
        if (itemName) {
            params = params.set('item_name', itemName);
        }
        return params;
    }

    private buildWebVitalsParams(from: string, to: string, metric?: WebVitalMetric | null, path?: string | null, rating?: WebVitalRating | null): HttpParams {
        let params = new HttpParams().set('from', from).set('to', to);
        if (metric) params = params.set('metric', metric);
        if (path) params = params.set('path', path);
        if (rating) params = params.set('rating', rating);
        return params;
    }

    getFunnelTimeseries(siteId: string, from?: string, to?: string, funnelIds: string[] = []): Observable<FunnelSeriesPoint[]> {
        let params = new HttpParams();
        if (from) params = params.set('from', from);
        if (to) params = params.set('to', to);
        for (const id of funnelIds) {
            params = params.append('funnel_id', id);
        }
        return this.http.get<FunnelSeriesPoint[]>(`/api/sites/${siteId}/funnels/timeseries`, { params });
    }

    createFunnel(siteId: string, funnel: Partial<Funnel>): Observable<void> {
        return this.http.post<void>(`/api/sites/${siteId}/funnels`, funnel);
    }

    deleteFunnel(siteId: string, funnelId: string): Observable<void> {
        return this.http.delete<void>(`/api/sites/${siteId}/funnels/${funnelId}`);
    }

    getFunnelStats(siteId: string, funnelId: string, from?: string, to?: string): Observable<FunnelStats> {
        let params = new HttpParams();
        if (from) params = params.set('from', from);
        if (to) params = params.set('to', to);
        return this.http.get<FunnelStats>(`/api/sites/${siteId}/funnels/${funnelId}/stats`, { params });
    }
}
