import { DestroyRef } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Subject } from 'rxjs';
import { vi } from 'vitest';

import type { SiteStats } from '@models/analytics.types';
import { StatsQuery } from '@features/analytics/services/stats-query';
import { StatsService } from '@features/analytics/services/stats.service';
import { emptySiteStats } from '@testing/empty-site-stats';

describe('StatsQuery', () => {
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

        const stats = emptySiteStats({ total_pageviews: 12 });
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
