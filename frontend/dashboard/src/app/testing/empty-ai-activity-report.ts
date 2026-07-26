import type { AIActivityComparison, AIActivityReport, AIActivityStat, AIFetchCorrelationReport } from '@models/analytics.types';

/**
 * All-zero `AIActivityReport` for specs, so a test only spells out the fields it
 * asserts on. Mirrors `emptySiteStats`.
 */
export function emptyAIActivityReport(overrides: Partial<AIActivityReport> = {}): AIActivityReport {
    return {
        ai_requests: 0,
        tracked_hits: 0,
        fetch_count: 0,
        referral_visits: 0,
        paths_crawled: 0,
        unique_agents: 0,
        pageviews: 0,
        error_rate_4xx: 0,
        error_rate_5xx: 0,
        median_response_ms: 0,
        total_bytes: 0,
        top_agents: [],
        top_categories: [],
        top_paths: [],
        top_sources: [],
        top_families: [],
        top_resource_types: [],
        top_error_paths: [],
        top_agents_by_category: {},
        series: [],
        ...overrides
    };
}

/** All-zero previous-period baseline for delta assertions. */
export function emptyAIActivityComparison(overrides: Partial<AIActivityComparison> = {}): AIActivityComparison {
    return {
        ai_requests: 0,
        tracked_hits: 0,
        fetch_count: 0,
        referral_visits: 0,
        paths_crawled: 0,
        unique_agents: 0,
        pageviews: 0,
        ...overrides
    };
}

/** All-zero fetch-only correlation report, the twin of `emptyAIActivityReport`. */
export function emptyAIFetchCorrelation(overrides: Partial<AIFetchCorrelationReport> = {}): AIFetchCorrelationReport {
    return {
        summary: {
            total_fetches: 0,
            fetched_paths: 0,
            correlated_paths: 0,
            ai_referred_visits: 0,
            uncorrelated_fetches: 0
        },
        citation_yield: [],
        citation_paths: [],
        opportunity_pages: [],
        failure_hotspots: [],
        ...overrides
    };
}

/** One breakdown row; `value` defaults to the sum of both provenance counters. */
export function aiActivityStat(name: string, trackedHits: number, fetchCount: number, value = trackedHits + fetchCount): AIActivityStat {
    return { name, value, tracked_hits: trackedHits, fetch_count: fetchCount };
}
