import { inject, Injector, signal } from '@angular/core';
import { rxResource } from '@angular/core/rxjs-interop';
import { finalize, Subject, takeUntil, tap } from 'rxjs';

import { AnalyticsService } from '@core/services/analytics.service';
import type { AIActivityReport } from '@models/analytics.types';

export interface AIActivityQueryRequest {
    siteId: string;
    from: string;
    to: string;
    filters?: { type: string; value: string }[];
    /** Previous-period window; omitted requests come back without a baseline. */
    comparison?: { from: string; to: string } | null;
    goalIds?: string[];
    funnelIds?: string[];
    onSuccess?: (report: AIActivityReport) => void;
}

export type AIActivityQueryMode = 'blocking' | 'background';

/**
 * Single-flight loader for the unified AI activity report. Every `load` cancels
 * the request still in flight, and `clear` explicitly tears down the active
 * stream, so a burst of filter clicks or a site switch can only ever commit
 * the last response — the page never has to sequence results itself.
 */
export class AIActivityQuery {
    readonly report = signal<AIActivityReport | null>(null);
    readonly isLoading = signal(false);
    readonly error = signal<unknown | null>(null);

    private readonly request = signal<(AIActivityQueryRequest & { mode: AIActivityQueryMode }) | undefined>(undefined);
    private readonly cancel$ = new Subject<void>();
    private readonly query;

    constructor(
        private readonly analyticsService: AnalyticsService,
        injector: Injector
    ) {
        this.query = rxResource({
            params: () => this.request(),
            stream: ({ params }) =>
                this.analyticsService.getAIActivity(params.siteId, params.from, params.to, params.filters ?? [], params.comparison ?? undefined, params.goalIds ?? [], params.funnelIds ?? []).pipe(
                    takeUntil(this.cancel$),
                    tap({
                        next: (report) => {
                            this.report.set(report);
                            this.error.set(null);
                            params.onSuccess?.(report);
                        },
                        error: (error) => {
                            // A failed activity request must not leave the
                            // previous site's/range's report visible. The
                            // dashboard falls back to legacy stats cards.
                            this.report.set(null);
                            this.error.set(error);
                        }
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
        this.error.set(null);
        if (mode !== 'background') {
            this.report.set(null);
        }
        this.isLoading.set(mode !== 'background');
        this.request.set({ ...request, mode });
    }

    clear(): void {
        this.cancel$.next();
        this.request.set(undefined);
        this.report.set(null);
        this.error.set(null);
        this.isLoading.set(false);
    }
}

export function injectAIActivityQuery(): AIActivityQuery {
    return new AIActivityQuery(inject(AnalyticsService), inject(Injector));
}
