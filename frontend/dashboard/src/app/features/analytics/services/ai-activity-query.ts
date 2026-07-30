import { inject, Injector, signal } from '@angular/core';
import { rxResource } from '@angular/core/rxjs-interop';
import { finalize, tap } from 'rxjs';

import { AnalyticsService } from '@core/services/analytics.service';
import type { AIActivityReport } from '@models/analytics.types';

export interface AIActivityQueryRequest {
    siteId: string;
    from: string;
    to: string;
    filters?: { type: string; value: string }[];
    /** Previous-period window; omitted requests come back without a baseline. */
    comparison?: { from: string; to: string } | null;
    onSuccess?: (report: AIActivityReport) => void;
}

export type AIActivityQueryMode = 'blocking' | 'background';

/**
 * Single-flight loader for the unified AI activity report. Every `load` cancels
 * the request still in flight, so a burst of filter clicks can only ever commit
 * the last response — the page never has to sequence results itself.
 */
export class AIActivityQuery {
    readonly report = signal<AIActivityReport | null>(null);
    readonly isLoading = signal(false);

    private readonly request = signal<(AIActivityQueryRequest & { mode: AIActivityQueryMode }) | undefined>(undefined);
    private readonly query;

    constructor(
        private readonly analyticsService: AnalyticsService,
        injector: Injector
    ) {
        this.query = rxResource({
            params: () => this.request(),
            stream: ({ params }) =>
                this.analyticsService.getAIActivity(params.siteId, params.from, params.to, params.filters ?? [], params.comparison ?? undefined).pipe(
                    tap({
                        next: (report) => {
                            this.report.set(report);
                            params.onSuccess?.(report);
                        },
                        error: (error) => console.error(error)
                    }),
                    finalize(() => {
                        if (this.request() === params && params.mode !== 'background') {
                            this.isLoading.set(false);
                        }
                    })
                ),
            injector
        });
    }

    load(request: AIActivityQueryRequest, mode: AIActivityQueryMode = 'blocking'): void {
        // A background refresh replaces the report in place: no skeleton, so the
        // visible loading flag stays untouched for the whole round trip.
        this.isLoading.set(mode !== 'background');
        this.request.set({ ...request, mode });
    }
}

export function injectAIActivityQuery(): AIActivityQuery {
    return new AIActivityQuery(inject(AnalyticsService), inject(Injector));
}
