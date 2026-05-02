export type AuditTableSeverity = 'success' | 'info' | 'warn' | 'danger' | 'secondary' | 'contrast';

export interface AuditTableOption {
    label: string;
    value: string;
}

export interface AuditTableQuery {
    action?: string;
    outcome?: string;
    target_type?: string;
    query?: string;
    from?: string;
    to?: string;
    limit: number;
    offset: number;
}

export interface AuditTableFilterConfig {
    action: boolean;
    outcome: boolean;
    targetType: boolean;
    dateRange: boolean;
    search: boolean;
}

export interface AuditTableExportStatus {
    severity: 'success' | 'error' | 'info' | 'warn';
    key: string;
}

export interface AuditTableRow {
    id: string;
    created_at: string;
    action: string;
    details?: string;
    actor_id?: string;
    actor_user_id?: string;
    actor_email?: string;
    actor_email_snapshot?: string;
    actor_role_snapshot?: string;
    team_id?: string;
    target_type?: string;
    target_id?: string;
    target_label?: string;
    target_user_id?: string;
    target_email?: string;
    outcome?: string;
    ip_address?: string;
    ip_country_code?: string;
    user_agent?: string;
    request_id?: string;
}
