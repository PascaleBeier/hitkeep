import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';

export interface SystemFeatureStatus {
    key: string;
    enabled: boolean;
    detail?: string;
}

export interface SystemInfo {
    version: string;
    runtime_mode: string;
    uptime: string;
    public_url: string;
    enabled_features: SystemFeatureStatus[];
    config_flags: Record<string, unknown>;
}

export interface SystemHealth {
    status: string;
    database: string;
    workers: string;
    is_leader: boolean;
}

export interface TenantDBInfo {
    tenant_id: string;
    name: string;
    bytes: number;
    path: string;
}

export interface SystemStorage {
    shared_db_path: string;
    shared_db_bytes: number;
    data_path: string;
    tenant_db_count: number;
    tenant_dbs?: TenantDBInfo[];
    spam_cache_path: string;
    backup_path: string;
    disk_available_bytes: number;
    disk_total_bytes: number;
}

export interface SystemIngestStats {
    recent_hits: number;
    recent_events: number;
    recent_rejections: number;
    recent_spam: number;
    hits_per_second: number;
}

export interface SystemBackupStatus {
    enabled: boolean;
    config_path: string;
    interval_min: number;
    retention: number;
    last_backup?: string;
    next_backup?: string;
    last_failed_at?: string;
    last_error?: string;
    recent_failures: number;
}

export interface SystemSpamStatus {
    db_path: string;
    last_refresh?: string;
    rule_count: number;
    auto_update: boolean;
    last_error?: string;
}

export interface ImportStageCleanupRunResult {
    imports_cleaned: number;
    files_cleaned: number;
    bytes_cleaned: number;
    imports_marked_failed: number;
    errors?: string[];
}

export interface SystemImportStageCleanupStatus {
    enabled: boolean;
    retention_days: number;
    stale_imports: number;
    stale_files: number;
    stale_bytes: number;
    last_run?: string;
    last_failed_at?: string;
    last_error?: string;
    recent_failures: number;
    last_cleaned_imports: number;
    last_cleaned_files: number;
    last_cleaned_bytes: number;
    last_marked_failed: number;
}

export interface SystemImportStageCleanupRunResponse {
    status: string;
    message?: string;
    result: ImportStageCleanupRunResult;
}

export interface SystemCacheEntry {
    size: number;
    max_size: number;
    ttl: string;
}

export interface SystemCacheStatus {
    permissions_cache: SystemCacheEntry;
    api_client_cache: SystemCacheEntry;
    rate_limiter_cache: SystemCacheEntry;
    status: string;
}

export interface SystemMailStatus {
    configured: boolean;
    driver: string;
    host: string;
    port: number;
    encryption: string;
    from_address: string;
    from_name: string;
    username: string;
    password_set: boolean;
    last_test_at?: string;
    last_test_ok?: boolean;
}

export interface InstanceAuditEntry {
    id: string;
    created_at: string;
    actor_id?: string;
    actor_email_snapshot: string;
    actor_role_snapshot: string;
    team_id?: string;
    action: string;
    target_type: string;
    target_id: string;
    target_user_id?: string;
    target_label: string;
    outcome: string;
    ip_address: string;
    ip_country_code?: string;
    user_agent: string;
    request_id: string;
    details: string;
}

export interface InstanceAuditListResponse {
    entries: InstanceAuditEntry[];
    total: number;
    limit: number;
    offset: number;
    has_more: boolean;
}

export type ActivationStatus = 'waiting' | 'live' | 'dormant' | 'domain_mismatch';

export interface SystemActivationRow {
    team_id: string;
    team_name: string;
    owner_email: string;
    plan_code?: string;
    plan_name?: string;
    cloud_region?: string;
    site_id: string;
    site_domain: string;
    sites_count: number;
    active_sites: number;
    status: ActivationStatus;
    first_hit_at?: string;
    last_hit_at?: string;
    last_event_at?: string;
    last_event_name?: string;
    hits_last_24h: number;
    hits_last_7d: number;
    events_last_7d: number;
    tracker_source?: string;
    tracker_version?: string;
}

export interface SystemActivationResponse {
    rows: SystemActivationRow[];
    total: number;
    limit: number;
    offset: number;
    has_more: boolean;
}

export interface ActivationFilterParams {
    status?: string;
    team?: string;
    domain?: string;
    last_seen_from?: string;
    last_seen_to?: string;
    limit?: number;
    offset?: number;
}

export interface AuditFilterParams {
    action?: string;
    target_type?: string;
    outcome?: string;
    actor_id?: string;
    from?: string;
    to?: string;
    query?: string;
    limit?: number;
    offset?: number;
}

const AUDIT_EXPORT_LIMIT = 50000;

@Injectable({ providedIn: 'root' })
export class AdminSystemService {
    private http = inject(HttpClient);

    getSystem() {
        return this.http.get<SystemInfo>('/api/admin/system');
    }

    getHealth() {
        return this.http.get<SystemHealth>('/api/admin/system/health');
    }

    getStorage() {
        return this.http.get<SystemStorage>('/api/admin/system/storage');
    }

    getIngestStats() {
        return this.http.get<SystemIngestStats>('/api/admin/system/ingest');
    }

    getBackups() {
        return this.http.get<SystemBackupStatus>('/api/admin/system/backups');
    }

    getSpamFilter() {
        return this.http.get<SystemSpamStatus>('/api/admin/system/spam-filter');
    }

    refreshSpamFilter() {
        return this.http.post<{ status: string; message: string }>('/api/admin/system/spam-filter/refresh', {});
    }

    getImportStageCleanup() {
        return this.http.get<SystemImportStageCleanupStatus>('/api/admin/system/import-stage-cleanup');
    }

    runImportStageCleanup() {
        return this.http.post<SystemImportStageCleanupRunResponse>('/api/admin/system/import-stage-cleanup/run', {});
    }

    getCaches() {
        return this.http.get<SystemCacheStatus>('/api/admin/system/caches');
    }

    getMail() {
        return this.http.get<SystemMailStatus>('/api/admin/system/mail');
    }

    testMail(email: string) {
        return this.http.post<{ status: string; message: string }>('/api/admin/system/mail/test', { email });
    }

    getActivation(params?: ActivationFilterParams) {
        let httpParams = new HttpParams();
        if (params) {
            if (params.status) httpParams = httpParams.set('status', params.status);
            if (params.team) httpParams = httpParams.set('team', params.team);
            if (params.domain) httpParams = httpParams.set('domain', params.domain);
            if (params.last_seen_from) httpParams = httpParams.set('last_seen_from', params.last_seen_from);
            if (params.last_seen_to) httpParams = httpParams.set('last_seen_to', params.last_seen_to);
            if (params.limit !== undefined) httpParams = httpParams.set('limit', params.limit);
            if (params.offset !== undefined) httpParams = httpParams.set('offset', params.offset);
        }
        return this.http.get<SystemActivationResponse>('/api/admin/system/activation', { params: httpParams });
    }

    listAudit(params?: AuditFilterParams) {
        const httpParams = this.auditParams(params, { includePagination: true });
        return this.http.get<InstanceAuditListResponse>('/api/admin/system/audit', { params: httpParams });
    }

    exportAudit(params?: AuditFilterParams) {
        const httpParams = this.auditParams(params, {
            exportLimit: AUDIT_EXPORT_LIMIT,
            format: 'json'
        });
        return this.http.get<Blob>('/api/admin/system/audit/export', {
            params: httpParams,
            responseType: 'blob' as 'json'
        });
    }

    private auditParams(
        params?: AuditFilterParams,
        options: {
            includePagination?: boolean;
            exportLimit?: number;
            format?: 'json' | 'csv';
        } = {}
    ): HttpParams {
        let httpParams = new HttpParams();
        if (params) {
            if (params.action) httpParams = httpParams.set('action', params.action);
            if (params.target_type) httpParams = httpParams.set('target_type', params.target_type);
            if (params.outcome) httpParams = httpParams.set('outcome', params.outcome);
            if (params.actor_id) httpParams = httpParams.set('actor_id', params.actor_id);
            if (params.from) httpParams = httpParams.set('from', params.from);
            if (params.to) httpParams = httpParams.set('to', params.to);
            if (params.query) httpParams = httpParams.set('query', params.query);
            if (options.includePagination && params.limit !== undefined) httpParams = httpParams.set('limit', params.limit);
            if (options.includePagination && params.offset !== undefined) httpParams = httpParams.set('offset', params.offset);
        }
        if (options.exportLimit !== undefined) {
            httpParams = httpParams.set('limit', options.exportLimit);
        }
        if (options.format) {
            httpParams = httpParams.set('format', options.format);
        }
        return httpParams;
    }
}
