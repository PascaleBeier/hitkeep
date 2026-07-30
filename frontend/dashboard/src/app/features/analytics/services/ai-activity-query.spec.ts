import { Injector } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Subject } from 'rxjs';
import { vi } from 'vitest';

import { AnalyticsService } from '@core/services/analytics.service';
import { AIActivityQuery, type AIActivityQueryRequest } from '@features/analytics/services/ai-activity-query';
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

        return { query: new AIActivityQuery(analyticsService, TestBed.inject(Injector)), responses, calls };
    }

    const request = { siteId: 'site-1', from: '2026-07-01T00:00:00Z', to: '2026-07-08T00:00:00Z' };

    function load(query: AIActivityQuery, value: AIActivityQueryRequest = request, mode: 'blocking' | 'background' = 'blocking'): void {
        query.load(value, mode);
        TestBed.tick();
    }

    it('forwards site, range, filters and comparison window to the analytics service', () => {
        const { query, responses, calls } = createQuery();

        load(query, {
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

    it('keeps the visible loading flag off for background refreshes and runs the success hook', () => {
        const { query, responses } = createQuery();
        const onSuccess = vi.fn();

        load(query, { ...request, onSuccess }, 'background');
        expect(query.isLoading()).toBe(false);

        const report = emptyAIActivityReport({ ai_requests: 7 });
        responses[0].next(report);
        responses[0].complete();

        expect(query.report()).toBe(report);
        expect(query.isLoading()).toBe(false);
        expect(onSuccess).toHaveBeenCalledWith(report);
    });

    it('cancels the first of two rapid loads so only the latest can commit', () => {
        const { query, responses } = createQuery();

        load(query, { ...request, filters: [{ type: 'ai_bot', value: 'GPTBot' }] });
        load(query, { ...request, filters: [{ type: 'ai_bot', value: 'ClaudeBot' }] });

        expect(responses[0].observed).toBe(false);
        expect(responses[1].observed).toBe(true);
        responses[0].next(emptyAIActivityReport({ ai_requests: 111 }));
        const fresh = emptyAIActivityReport({ ai_requests: 222 });
        responses[1].next(fresh);

        expect(query.report()).toBe(fresh);
        expect(query.report()?.ai_requests).toBe(222);
    });

    it('preserves the last successful report when a later request fails', () => {
        const { query, responses } = createQuery();
        vi.spyOn(console, 'error').mockImplementation(() => undefined);

        load(query);
        const successful = emptyAIActivityReport({ ai_requests: 5 });
        responses[0].next(successful);
        responses[0].complete();

        const onSuccess = vi.fn();
        load(query, { ...request, onSuccess });
        responses[1].error(new Error('unavailable'));

        expect(query.report()).toBe(successful);
        expect(query.isLoading()).toBe(false);
        expect(onSuccess).not.toHaveBeenCalled();
    });

    it('tears down the active observable with its injector', () => {
        const { query, responses } = createQuery();
        load(query);
        expect(responses[0].observed).toBe(true);

        TestBed.resetTestingModule();
        expect(responses[0].observed).toBe(false);
    });
});
