import { DestroyRef } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Subject } from 'rxjs';
import { vi } from 'vitest';

import type { SiteStats } from '@models/analytics.types';
import { StatsQuery } from '@features/analytics/services/stats-query';
import { StatsService } from '@features/analytics/services/stats.service';

describe('StatsQuery', () => {
    const statsFixture = (overrides: Partial<SiteStats> = {}): SiteStats => ({
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
        top_browsers: [],
        top_ai_bots: [],
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
    });

    function createQuery(response: Subject<SiteStats>) {
        TestBed.configureTestingModule({});
        const statsService = {
            comparisonRange: vi.fn(() => ({
                from: '2026-05-31T00:00:00.000Z',
                to: '2026-06-01T00:00:00.000Z'
            })),
            fetchStats: vi.fn(() => response.asObservable())
        } as unknown as StatsService;

        return new StatsQuery(statsService, TestBed.inject(DestroyRef));
    }

    it('keeps visible loading off for background refreshes', () => {
        const response = new Subject<SiteStats>();
        const query = createQuery(response);
        const onSuccess = vi.fn((stats: SiteStats, result: unknown) => {
            expect(query.stats()).toBe(stats);
            expect(query.lastResult()).toEqual({ mode: 'background', sequence: 1 });
            expect(result).toEqual({ mode: 'background', sequence: 1 });
        });

        query.load({
            siteId: 'site-1',
            from: '2026-06-01T00:00:00.000Z',
            to: '2026-06-02T00:00:00.000Z',
            mode: 'background',
            onSuccess
        });

        expect(query.isLoading()).toBe(false);
        expect(query.isBackgroundRefreshing()).toBe(true);

        const stats = statsFixture({ total_pageviews: 12 });
        response.next(stats);
        response.complete();

        expect(query.stats()).toBe(stats);
        expect(query.lastResult()).toEqual({ mode: 'background', sequence: 1 });
        expect(onSuccess).toHaveBeenCalledWith(stats, {
            mode: 'background',
            sequence: 1
        });
        expect(query.isLoading()).toBe(false);
        expect(query.isBackgroundRefreshing()).toBe(false);
    });

    it('does not commit a result or run the success hook when a background request fails', () => {
        const response = new Subject<SiteStats>();
        const query = createQuery(response);
        const onSuccess = vi.fn();
        vi.spyOn(console, 'error').mockImplementation(() => undefined);

        query.load({
            siteId: 'site-1',
            from: '2026-06-01T00:00:00.000Z',
            to: '2026-06-02T00:00:00.000Z',
            mode: 'background',
            onSuccess
        });
        response.error(new Error('unavailable'));

        expect(query.stats()).toBeNull();
        expect(query.lastResult()).toBeNull();
        expect(onSuccess).not.toHaveBeenCalled();
        expect(query.isBackgroundRefreshing()).toBe(false);
    });
});
