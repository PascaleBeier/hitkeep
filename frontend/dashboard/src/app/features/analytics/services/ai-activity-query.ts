import { DestroyRef, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { finalize, Subscription } from 'rxjs';

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

    private request: Subscription | null = null;

    constructor(
        private readonly analyticsService: AnalyticsService,
        private readonly destroyRef: DestroyRef
    ) {
        this.destroyRef.onDestroy(() => this.request?.unsubscribe());
    }

    load(request: AIActivityQueryRequest, mode: AIActivityQueryMode = 'blocking'): void {
        this.request?.unsubscribe();
        // A background refresh replaces the report in place: no skeleton, so the
        // visible loading flag stays untouched for the whole round trip.
        if (mode !== 'background') {
            this.isLoading.set(true);
        }

        this.request = this.analyticsService
            .getAIActivity(request.siteId, request.from, request.to, request.filters ?? [], request.comparison ?? undefined)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => {
                    if (mode !== 'background') {
                        this.isLoading.set(false);
                    }
                })
            )
            .subscribe({
                next: (report) => {
                    this.report.set(report);
                    request.onSuccess?.(report);
                },
                error: (e) => console.error(e)
            });
    }
}

export function injectAIActivityQuery(): AIActivityQuery {
    return new AIActivityQuery(inject(AnalyticsService), inject(DestroyRef));
}
