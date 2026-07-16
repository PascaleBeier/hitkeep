import { DestroyRef, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { finalize, Subscription } from 'rxjs';
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

    private request: Subscription | null = null;
    private resultSequence = 0;

    constructor(
        private readonly statsService: StatsService,
        private readonly destroyRef: DestroyRef
    ) {
        this.destroyRef.onDestroy(() => this.request?.unsubscribe());
    }

    load(request: StatsQueryRequest): void {
        const mode = request.mode ?? 'blocking';
        this.comparisonRange.set(this.statsService.comparisonRange(request.from, request.to));
        this.request?.unsubscribe();
        if (mode === 'background') {
            this.isBackgroundRefreshing.set(true);
        } else {
            this.isLoading.set(true);
        }
        this.request = this.statsService
            .fetchStats(request.siteId, request.from, request.to, request.filters ?? [], request.goalIds ?? [], request.funnelIds ?? [])
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => {
                    if (mode === 'background') {
                        this.isBackgroundRefreshing.set(false);
                    } else {
                        this.isLoading.set(false);
                    }
                })
            )
            .subscribe({
                next: (stats) => {
                    const result = { mode, sequence: ++this.resultSequence };
                    this.stats.set(stats);
                    this.lastResult.set(result);
                    request.onSuccess?.(stats, result);
                },
                error: (e) => console.error(e)
            });
    }
}

export function injectStatsQuery(): StatsQuery {
    return new StatsQuery(inject(StatsService), inject(DestroyRef));
}
