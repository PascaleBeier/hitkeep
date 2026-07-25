import type { SiteStats } from '@models/analytics.types';

/**
 * All-zero `SiteStats` payload for specs. Every list is empty and every counter
 * is zero, so a test only has to spell out the fields it actually asserts on.
 */
export function emptySiteStats(overrides: Partial<SiteStats> = {}): SiteStats {
    return {
        live_visitors: 0,
        total_pageviews: 0,
        unique_sessions: 0,
        bounce_rate: 0,
        avg_session_duration: 0,
        pages_per_session: 0,
        chart_data: [],
        top_pages: [],
        top_landing_pages: [],
        top_exit_pages: [],
        top_referrers: [],
        top_devices: [],
        top_countries: [],
        top_cities: [],
        top_providers: [],
        top_asns: [],
        top_browsers: [],
        top_ai_bots: [],
        top_ai_bot_categories: [],
        top_ai_sources: [],
        top_languages: [],
        top_utm_campaigns: [],
        top_utm_contents: [],
        top_utm_mediums: [],
        top_utm_sources: [],
        top_utm_terms: [],
        ai_bot_hits: 0,
        ai_source_visits: 0,
        utm_campaign_hits: 0,
        utm_content_hits: 0,
        utm_medium_hits: 0,
        utm_source_hits: 0,
        utm_term_hits: 0,
        goals: [],
        funnels: [],
        ...overrides
    };
}
