import { Injector } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Subject } from 'rxjs';
import { vi } from 'vitest';

import type { SiteStats } from '@models/analytics.types';
import { StatsQuery, type StatsQueryRequest } from '@features/analytics/services/stats-query';
import { StatsService } from '@features/analytics/services/stats.service';
import { emptySiteStats } from '@testing/empty-site-stats';

describe('StatsQuery', () => {
    interface Harness {
        query: StatsQuery;
        responses: Subject<SiteStats>[];
        fetchStats: ReturnType<typeof vi.fn>;
    }

    const request: StatsQueryRequest = {
        siteId: 'site-1',
        from: '2026-06-01T00:00:00.000Z',
        to: '2026-06-02T00:00:00.000Z'
    };

    function createQuery(): Harness {
        TestBed.configureTestingModule({});
        const responses: Subject<SiteStats>[] = [];
        const fetchStats = vi.fn(() => {
            const response = new Subject<SiteStats>();
            responses.push(response);
            return response.asObservable();
        });
        const statsService = {
            comparisonRange: vi.fn(() => ({
                from: '2026-05-31T00:00:00.000Z',
                to: '2026-06-01T00:00:00.000Z'
            })),
            fetchStats
        } as unknown as StatsService;

        return { query: new StatsQuery(statsService, TestBed.inject(Injector)), responses, fetchStats };
    }

    function load(query: StatsQuery, value: StatsQueryRequest): void {
        query.load(value);
        TestBed.tick();
    }

    it('keeps visible loading off for background refreshes and sequences successful results', () => {
        const { query, responses } = createQuery();
        const onSuccess = vi.fn();

        load(query, { ...request, mode: 'background', onSuccess });

        expect(query.isLoading()).toBe(false);
        expect(query.isBackgroundRefreshing()).toBe(true);

        const stats = emptySiteStats({ total_pageviews: 12 });
        responses[0].next(stats);
        responses[0].complete();

        expect(query.stats()).toBe(stats);
        expect(query.lastResult()).toEqual({ mode: 'background', sequence: 1 });
        expect(onSuccess).toHaveBeenCalledWith(stats, { mode: 'background', sequence: 1 });
        expect(query.isLoading()).toBe(false);
        expect(query.isBackgroundRefreshing()).toBe(false);
    });

    it('cancels a stale request before starting the next one', () => {
        const { query, responses } = createQuery();

        load(query, { ...request, siteId: 'site-1' });
        expect(responses[0].observed).toBe(true);

        load(query, { ...request, siteId: 'site-2' });
        expect(responses[0].observed).toBe(false);
        expect(responses[1].observed).toBe(true);

        const fresh = emptySiteStats({ total_pageviews: 22 });
        responses[0].next(emptySiteStats({ total_pageviews: 11 }));
        responses[1].next(fresh);

        expect(query.stats()).toBe(fresh);
        expect(query.lastResult()).toEqual({ mode: 'blocking', sequence: 1 });
    });

    it('preserves the last successful value and skips the success hook after an error', () => {
        const { query, responses } = createQuery();
        vi.spyOn(console, 'error').mockImplementation(() => undefined);

        load(query, request);
        const successful = emptySiteStats({ total_pageviews: 8 });
        responses[0].next(successful);
        responses[0].complete();

        const onSuccess = vi.fn();
        load(query, { ...request, mode: 'background', onSuccess });
        responses[1].error(new Error('unavailable'));

        expect(query.stats()).toBe(successful);
        expect(query.lastResult()).toEqual({ mode: 'blocking', sequence: 1 });
        expect(onSuccess).not.toHaveBeenCalled();
        expect(query.isBackgroundRefreshing()).toBe(false);
    });

    it('keeps loading flags aligned when the refresh mode changes mid-flight', () => {
        const { query } = createQuery();

        load(query, request);
        expect(query.isLoading()).toBe(true);

        load(query, { ...request, mode: 'background' });
        expect(query.isLoading()).toBe(false);
        expect(query.isBackgroundRefreshing()).toBe(true);

        load(query, request);
        expect(query.isLoading()).toBe(true);
        expect(query.isBackgroundRefreshing()).toBe(false);
    });

    it('forwards filters and tears down the active observable with its injector', () => {
        const { query, responses, fetchStats } = createQuery();
        load(query, {
            ...request,
            filters: [{ type: 'country', value: 'DE' }],
            goalIds: ['goal-1'],
            funnelIds: ['funnel-1']
        });

        expect(fetchStats).toHaveBeenCalledWith(request.siteId, request.from, request.to, [{ type: 'country', value: 'DE' }], ['goal-1'], ['funnel-1']);
        expect(responses[0].observed).toBe(true);

        TestBed.resetTestingModule();
        expect(responses[0].observed).toBe(false);
    });
});
