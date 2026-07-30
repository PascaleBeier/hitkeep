import { inject, Injector, signal } from '@angular/core';
import { rxResource } from '@angular/core/rxjs-interop';
import { finalize, tap } from 'rxjs';
import type { SiteStats } from '@models/analytics.types';
import { StatsService } from '@features/analytics/services/stats.service';

export interface StatsQueryRequest {
    siteId: string;
    from: string;
    to: string;
    filters?: { type: string; value: string }[];
    goalIds?: string[];
    funnelIds?: string[];
    mode?: StatsQueryMode;
    onSuccess?: (stats: SiteStats, result: StatsQueryResult) => void;
}

export type StatsQueryMode = 'blocking' | 'background';

export interface StatsQueryResult {
    mode: StatsQueryMode;
    sequence: number;
}

export class StatsQuery {
    readonly stats = signal<SiteStats | null>(null);
    readonly isLoading = signal(false);
    readonly isBackgroundRefreshing = signal(false);
    readonly comparisonRange = signal<{ from: string; to: string } | null>(null);
    readonly lastResult = signal<StatsQueryResult | null>(null);

    private readonly request = signal<StatsQueryRequest | undefined>(undefined);
    private readonly query;
    private resultSequence = 0;

    constructor(
        private readonly statsService: StatsService,
        injector: Injector
    ) {
        this.query = rxResource({
            params: () => this.request(),
            stream: ({ params }) =>
                this.statsService.fetchStats(params.siteId, params.from, params.to, params.filters ?? [], params.goalIds ?? [], params.funnelIds ?? []).pipe(
                    tap({
                        next: (stats) => {
                            const result = { mode: params.mode ?? 'blocking', sequence: ++this.resultSequence };
                            this.stats.set(stats);
                            this.lastResult.set(result);
                            params.onSuccess?.(stats, result);
                        },
                        error: (error) => console.error(error)
                    }),
                    finalize(() => {
                        if (this.request() !== params) return;
                        if ((params.mode ?? 'blocking') === 'background') {
                            this.isBackgroundRefreshing.set(false);
                        } else {
                            this.isLoading.set(false);
                        }
                    })
                ),
            injector
        });
    }

    load(request: StatsQueryRequest): void {
        const mode = request.mode ?? 'blocking';
        this.comparisonRange.set(this.statsService.comparisonRange(request.from, request.to));
        if (mode === 'background') {
            this.isLoading.set(false);
            this.isBackgroundRefreshing.set(true);
        } else {
            this.isBackgroundRefreshing.set(false);
            this.isLoading.set(true);
        }
        this.request.set({ ...request, mode });
    }
}

export function injectStatsQuery(): StatsQuery {
    return new StatsQuery(inject(StatsService), inject(Injector));
}
