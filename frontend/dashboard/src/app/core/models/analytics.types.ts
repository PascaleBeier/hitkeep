import type { TeamRole } from '@core/access/capabilities';
export type { TeamRole } from '@core/access/capabilities';

export interface Site {
    id: string;
    user_id: string;
    domain: string;
    created_at: string;
    data_retention_days?: number;
}

export interface Team {
    id: string;
    name: string;
    logo_url: string;
    role: TeamRole;
    created_at: string;
    usage?: TeamUsageSummary;
    entitlements?: TeamEntitlements;
    plan?: TeamPlan;
}

export interface TeamUsageSummary {
    current_sites: number;
    current_members: number;
    current_pending_invites: number;
}

export interface TeamEntitlements {
    max_sites_per_team: number;
    max_team_members: number;
    max_retention_days: number;
    allow_sso: boolean;
    allow_custom_branding: boolean;
}

export interface TeamPlan {
    code: string;
    name: string;
    upgrade_url?: string;
    support_url?: string;
}

export interface CloudPlanTier {
    code: string;
    name: string;
    entitlements: TeamEntitlements;
}

export interface UserTeamsResponse {
    active_team_id: string;
    recent_team_ids?: string[];
    teams: Team[];
}

export interface TeamMember {
    id: string;
    user_id: string;
    email: string;
    role: TeamRole;
    added_at: string;
}

export interface TeamInvite {
    id: string;
    team_id: string;
    email: string;
    role: TeamRole;
    invited_user_id?: string;
    status: 'pending' | 'accepted' | 'revoked';
    created_by?: string;
    created_at: string;
    expires_at: string;
    accepted_at?: string;
    revoked_at?: string;
}

export interface TeamAuditEntry {
    id: string;
    team_id: string;
    action: string;
    details: string;
    actor_user_id?: string;
    actor_email?: string;
    actor_email_snapshot?: string;
    actor_role_snapshot?: string;
    target_type?: string;
    target_id?: string;
    target_label?: string;
    outcome?: string;
    ip_address?: string;
    ip_country_code?: string;
    user_agent?: string;
    request_id?: string;
    target_user_id?: string;
    target_email?: string;
    created_at: string;
}

export interface TeamAuditListResponse {
    entries: TeamAuditEntry[];
    total: number;
    limit: number;
    offset: number;
    has_more: boolean;
    action?: string;
}

export interface IPExclusion {
    id: string;
    site_id?: string;
    cidr: string;
    description?: string;
    created_at: string;
    created_by?: string;
}

export interface CurrentIP {
    ip: string;
    cidr: string;
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

export interface GoalSeriesPoint {
    time: string;
    conversions: number;
}

export interface FunnelSeriesPoint {
    time: string;
    entries: number;
    completions: number;
}

export interface EventSeriesPoint {
    time: string;
    count: number;
}

export type WebVitalMetric = 'LCP' | 'INP' | 'CLS' | 'FCP' | 'TTFB';
export type WebVitalRating = 'good' | 'needs_improvement' | 'poor';

export interface WebVitalSummaryMetric {
    metric: WebVitalMetric;
    p75: number;
    samples: number;
    good: number;
    needs_improvement: number;
    poor: number;
    rating: WebVitalRating;
}

export interface WebVitalSeriesPoint {
    time: string;
    p75: number;
    samples: number;
    good: number;
    needs_improvement: number;
    poor: number;
}

export interface WebVitalPageRow {
    path: string;
    p75: number;
    samples: number;
    good: number;
    needs_improvement: number;
    poor: number;
    rating: WebVitalRating;
    metrics: Partial<Record<WebVitalMetric, WebVitalMetricBreakdown>>;
}

export interface WebVitalMetricBreakdown {
    p75: number;
    samples: number;
    good: number;
    needs_improvement: number;
    poor: number;
    rating: WebVitalRating;
}

export type WebVitalDimension = 'browser' | 'country' | 'language' | 'device' | 'city' | 'provider' | 'asn';

export interface WebVitalDimensionRow {
    name: string;
    p75: number;
    samples: number;
    good: number;
    needs_improvement: number;
    poor: number;
    rating: WebVitalRating;
}

export interface AIFetch {
    id: string;
    site_id: string;
    timestamp: string;
    assistant_name: string;
    assistant_family: string;
    path: string;
    hostname?: string;
    status_code: number;
    content_type?: string;
    resource_type: string;
    response_ms?: number;
    bytes_served?: number;
    user_agent?: string;
}

export interface AIFetchOverview {
    total_requests: number;
    unique_paths: number;
    unique_assistants: number;
    error_rate_4xx: number;
    error_rate_5xx: number;
    median_response_ms: number;
    total_bytes: number;
    top_assistants: MetricStat[];
    top_families: MetricStat[];
    top_paths: MetricStat[];
    top_error_paths: MetricStat[];
    resource_type_split: MetricStat[];
}

export interface AIFetchSeriesPoint {
    time: string;
    count: number;
}

export interface AIFetchCorrelationSummary {
    total_fetches: number;
    fetched_paths: number;
    correlated_paths: number;
    ai_referred_visits: number;
    uncorrelated_fetches: number;
}

export interface AIFetchCitationYieldRow {
    path: string;
    assistant_name: string;
    fetch_count: number;
    ai_referred_visits: number;
    citation_yield_pct: number;
}

export interface AIFetchOpportunityRow {
    path: string;
    fetch_count: number;
    ai_referred_visits: number;
    error_requests: number;
    error_rate_pct: number;
}

export interface AIFetchFailureHotspot {
    assistant_name: string;
    path_prefix: string;
    total_requests: number;
    error_requests: number;
    error_rate_pct: number;
}

export interface AIFetchCorrelationReport {
    summary: AIFetchCorrelationSummary;
    citation_yield: AIFetchCitationYieldRow[];
    opportunity_pages: AIFetchOpportunityRow[];
    failure_hotspots: AIFetchFailureHotspot[];
}

export interface EventAudience {
    top_pages: MetricStat[];
    top_referrers: MetricStat[];
    top_devices: MetricStat[];
    top_countries: MetricStat[];
    top_cities?: MetricStat[];
    top_providers?: MetricStat[];
    top_asns?: MetricStat[];
    imported_excluded?: ImportExclusionReason[];
}

export interface EcommerceSummary {
    revenue: number;
    orders: number;
    average_order_value: number;
    checkout_starts: number;
    checkout_conversion_rate: number;
    currency: string;
    top_cities?: MetricStat[];
    top_providers?: MetricStat[];
    top_asns?: MetricStat[];
}

export interface EcommerceSeriesPoint {
    time: string;
    revenue: number;
    orders: number;
}

export interface EcommerceProductStat {
    item_id: string;
    item_name: string;
    revenue: number;
    orders: number;
    quantity: number;
}

export interface EcommerceSourceStat {
    utm_source: string;
    utm_medium: string;
    utm_campaign: string;
    referrer: string;
    revenue: number;
    orders: number;
}

export interface MetricStat {
    name: string;
    value: number;
}

export interface ImportExclusionReason {
    reason: string;
    detail?: string;
}

export interface ComparisonStats {
    total_pageviews: number;
    unique_sessions: number;
    bounce_rate: number;
    avg_session_duration: number;
    pages_per_session: number;
    chart_data: ChartDataPoint[];
    utm_campaign_hits: number;
    utm_content_hits: number;
    utm_medium_hits: number;
    utm_source_hits: number;
    utm_term_hits: number;
    goals: GoalStats[];
    total_conversions: number;
}

export interface SiteStats {
    live_visitors: number;
    total_pageviews: number;
    unique_sessions: number;
    bounce_rate: number;
    avg_session_duration: number;
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
    top_browsers: MetricStat[];
    top_ai_bots: MetricStat[];
    top_ai_sources: MetricStat[];
    top_languages: MetricStat[];
    top_utm_campaigns: MetricStat[];
    top_utm_contents: MetricStat[];
    top_utm_mediums: MetricStat[];
    top_utm_sources: MetricStat[];
    top_utm_terms: MetricStat[];
    ai_bot_hits: number;
    ai_source_visits: number;
    utm_campaign_hits: number;
    utm_content_hits: number;
    utm_medium_hits: number;
    utm_source_hits: number;
    utm_term_hits: number;
    goals: GoalStats[];
    funnels: Funnel[];
    comparison?: ComparisonStats;
    imported_excluded?: ImportExclusionReason[];
}

export interface GoalStats {
    goal_id: string;
    name: string;
    conversions: number;
    conversion_rate: number;
}

export interface Goal {
    id: string;
    site_id: string;
    name: string;
    type: 'event' | 'path';
    value: string;
    created_at: string;
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

export interface SystemStatus {
    needs_setup: boolean;
    version: string;
    cloud?: CloudStatus;
}

export interface CloudStatus {
    hosted: boolean;
    signup_enabled: boolean;
    jurisdiction?: string;
    region?: string;
    upgrade_url?: string;
    support_url?: string;
}

export type ReportFrequency = 'daily' | 'weekly' | 'monthly';

export interface FrequencyPrefs {
    daily: boolean;
    weekly: boolean;
    monthly: boolean;
}

export interface SiteReportSubscription {
    site_id: string;
    domain: string;
    daily: boolean;
    weekly: boolean;
    monthly: boolean;
}

export interface ReportSubscriptions {
    sites: SiteReportSubscription[];
    digest: FrequencyPrefs;
}
