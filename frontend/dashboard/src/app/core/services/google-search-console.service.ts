import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';

export interface GoogleSearchConsoleStatus {
    status: 'connected' | 'disconnected' | 'credentials_missing';
    configured: boolean;
    connected: boolean;
    credential_status: 'configured' | 'missing';
    connected_account_label?: string;
    last_connected_at?: string;
    last_disconnected_at?: string;
    needs_admin_action: boolean;
    can_manage: boolean;
    managed_credentials_mode: 'managed' | 'self_hosted';
}

export interface GoogleSearchConsoleConnectResponse {
    auth_url: string;
}

export interface GoogleSearchConsoleActionResponse {
    status: string;
}

export interface GoogleSearchConsoleProperty {
    uri: string;
    permission_level: string;
}

export interface GoogleSearchConsolePropertiesResponse {
    properties: GoogleSearchConsoleProperty[];
}

export interface GoogleSearchConsoleSyncStatus {
    state: 'pending' | 'running' | 'succeeded' | 'failed' | 'needs_attention';
    imported_start_date?: string;
    imported_end_date?: string;
    last_success_at?: string;
    last_attempt_at?: string;
    last_error_category?: string;
    next_retry_at?: string;
    manual: boolean;
}

export interface GoogleSearchConsoleSiteMapping {
    site_id: string;
    team_id: string;
    mapped: boolean;
    property_uri?: string;
    property_permission_level?: string;
    mapped_at?: string;
    can_manage: boolean;
    sync_status?: GoogleSearchConsoleSyncStatus;
}

export interface SearchConsoleReportFilters {
    from?: string;
    to?: string;
    page?: string | null;
    path?: string | null;
    country?: string | null;
    device?: string | null;
    limit?: number;
}

export interface SearchConsoleOverview {
    data_source: 'google_search_console';
    clicks: number;
    impressions: number;
    ctr: number;
    average_position: number;
}

export interface SearchConsoleMetricPoint {
    date: string;
    clicks: number;
    impressions: number;
    ctr: number;
    average_position: number;
}

export interface SearchConsoleSeriesResponse {
    data_source: 'google_search_console';
    series: SearchConsoleMetricPoint[];
}

export interface SearchConsoleDimensionRow {
    value: string;
    clicks: number;
    impressions: number;
    ctr: number;
    average_position: number;
}

export interface SearchConsoleDimensionResponse {
    data_source: 'google_search_console';
    rows: SearchConsoleDimensionRow[];
}

@Injectable({ providedIn: 'root' })
export class GoogleSearchConsoleService {
    private http = inject(HttpClient);

    getStatus(teamID: string) {
        return this.http.get<GoogleSearchConsoleStatus>(`/api/user/teams/${encodeURIComponent(teamID)}/integrations/google-search-console/status`);
    }

    connect(teamID: string, returnPath: string) {
        return this.http.post<GoogleSearchConsoleConnectResponse>(`/api/user/teams/${encodeURIComponent(teamID)}/integrations/google-search-console/connect`, {
            return_path: returnPath
        });
    }

    disconnect(teamID: string) {
        return this.http.delete<GoogleSearchConsoleActionResponse>(`/api/user/teams/${encodeURIComponent(teamID)}/integrations/google-search-console`);
    }

    listProperties(teamID: string) {
        return this.http.get<GoogleSearchConsolePropertiesResponse>(`/api/user/teams/${encodeURIComponent(teamID)}/integrations/google-search-console/properties`);
    }

    getSiteMapping(siteID: string) {
        return this.http.get<GoogleSearchConsoleSiteMapping>(`/api/sites/${encodeURIComponent(siteID)}/integrations/google-search-console`);
    }

    mapSiteProperty(siteID: string, propertyURI: string) {
        return this.http.put<GoogleSearchConsoleSiteMapping>(`/api/sites/${encodeURIComponent(siteID)}/integrations/google-search-console/property`, {
            property_uri: propertyURI
        });
    }

    unmapSiteProperty(siteID: string) {
        return this.http.delete<GoogleSearchConsoleSiteMapping>(`/api/sites/${encodeURIComponent(siteID)}/integrations/google-search-console/property`);
    }

    requestSync(siteID: string) {
        return this.http.post<GoogleSearchConsoleSiteMapping>(`/api/sites/${encodeURIComponent(siteID)}/integrations/google-search-console/sync`, null);
    }

    getOverview(siteID: string, filters: SearchConsoleReportFilters = {}) {
        return this.http.get<SearchConsoleOverview>(`/api/sites/${encodeURIComponent(siteID)}/search-console/overview`, { params: this.reportParams(filters) });
    }

    getSeries(siteID: string, filters: SearchConsoleReportFilters = {}) {
        return this.http.get<SearchConsoleSeriesResponse>(`/api/sites/${encodeURIComponent(siteID)}/search-console/series`, { params: this.reportParams(filters) });
    }

    getQueries(siteID: string, filters: SearchConsoleReportFilters = {}) {
        return this.http.get<SearchConsoleDimensionResponse>(`/api/sites/${encodeURIComponent(siteID)}/search-console/queries`, { params: this.reportParams(filters) });
    }

    getPages(siteID: string, filters: SearchConsoleReportFilters = {}) {
        return this.http.get<SearchConsoleDimensionResponse>(`/api/sites/${encodeURIComponent(siteID)}/search-console/pages`, { params: this.reportParams(filters) });
    }

    getBreakdown(siteID: string, dimension: 'country' | 'device', filters: SearchConsoleReportFilters = {}) {
        return this.http.get<SearchConsoleDimensionResponse>(`/api/sites/${encodeURIComponent(siteID)}/search-console/breakdowns`, { params: this.reportParams(filters).set('dimension', dimension) });
    }

    private reportParams(filters: SearchConsoleReportFilters): HttpParams {
        let params = new HttpParams();
        if (filters.from) params = params.set('from', filters.from);
        if (filters.to) params = params.set('to', filters.to);
        if (filters.page) params = params.set('page', filters.page);
        if (filters.path) params = params.set('path', filters.path);
        if (filters.country) params = params.set('country', filters.country);
        if (filters.device) params = params.set('device', filters.device);
        if (filters.limit) params = params.set('limit', String(filters.limit));
        return params;
    }
}
