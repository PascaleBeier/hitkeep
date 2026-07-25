import { DestroyRef } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Subject } from 'rxjs';
import { vi } from 'vitest';

import { AnalyticsService } from '@core/services/analytics.service';
import { AIActivityQuery } from '@features/analytics/services/ai-activity-query';
import type { AIActivityReport } from '@models/analytics.types';
import { emptyAIActivityReport } from '@testing/empty-ai-activity-report';

describe('AIActivityQuery', () => {
    interface Harness {
        query: AIActivityQuery;
        responses: Subject<AIActivityReport>[];
        calls: unknown[][];
    }

    function createQuery(): Harness {
        TestBed.configureTestingModule({});
        const responses: Subject<AIActivityReport>[] = [];
        const calls: unknown[][] = [];
        const analyticsService = {
            getAIActivity: vi.fn((...args: unknown[]) => {
                calls.push(args);
                const response = new Subject<AIActivityReport>();
                responses.push(response);
                return response.asObservable();
            })
        } as unknown as AnalyticsService;

        return { query: new AIActivityQuery(analyticsService, TestBed.inject(DestroyRef)), responses, calls };
    }

    const request = { siteId: 'site-1', from: '2026-07-01T00:00:00Z', to: '2026-07-08T00:00:00Z' };

    it('forwards site, range, filters and comparison window to the analytics service', () => {
        const { query, responses, calls } = createQuery();

        query.load({
            ...request,
            filters: [{ type: 'ai_bot', value: 'GPTBot' }],
            comparison: { from: '2026-06-24T00:00:00Z', to: '2026-07-01T00:00:00Z' }
        });

        expect(calls).toEqual([['site-1', '2026-07-01T00:00:00Z', '2026-07-08T00:00:00Z', [{ type: 'ai_bot', value: 'GPTBot' }], { from: '2026-06-24T00:00:00Z', to: '2026-07-01T00:00:00Z' }]]);
        expect(query.isLoading()).toBe(true);

        const report = emptyAIActivityReport({ ai_requests: 12 });
        responses[0].next(report);
        responses[0].complete();

        expect(query.report()).toBe(report);
        expect(query.isLoading()).toBe(false);
    });

    it('keeps the visible loading flag off for background refreshes', () => {
        const { query, responses } = createQuery();
        const onSuccess = vi.fn();

        query.load({ ...request, onSuccess }, 'background');

        expect(query.isLoading()).toBe(false);

        const report = emptyAIActivityReport({ ai_requests: 7 });
        responses[0].next(report);
        responses[0].complete();

        expect(query.report()).toBe(report);
        expect(query.isLoading()).toBe(false);
        expect(onSuccess).toHaveBeenCalledWith(report);
    });

    it('applies only the last of two rapid loads', () => {
        const { query, responses } = createQuery();

        query.load({ ...request, filters: [{ type: 'ai_bot', value: 'GPTBot' }] });
        query.load({ ...request, filters: [{ type: 'ai_bot', value: 'ClaudeBot' }] });

        const stale = emptyAIActivityReport({ ai_requests: 111 });
        const fresh = emptyAIActivityReport({ ai_requests: 222 });
        responses[0].next(stale);
        responses[1].next(fresh);

        expect(query.report()).toBe(fresh);
        expect(query.report()?.ai_requests).toBe(222);
    });

    it('leaves the report untouched when a request fails', () => {
        const { query, responses } = createQuery();
        vi.spyOn(console, 'error').mockImplementation(() => undefined);
        const onSuccess = vi.fn();

        query.load({ ...request, onSuccess });
        responses[0].error(new Error('unavailable'));

        expect(query.report()).toBeNull();
        expect(query.isLoading()).toBe(false);
        expect(onSuccess).not.toHaveBeenCalled();
    });
});
