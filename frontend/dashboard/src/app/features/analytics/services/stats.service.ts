import { inject, signal, Service } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { finalize, Observable } from 'rxjs';
import { SiteStats, SitesOverviewStatsResponse } from '@models/analytics.types';

@Service()
export class StatsService {
    private http = inject(HttpClient);

    readonly stats = signal<SiteStats | null>(null);
    readonly isLoading = signal<boolean>(false);
    readonly currentComparisonRange = signal<{ from: string; to: string } | null>(null);

    loadStats(siteId: string, from: string, to: string, filters: { type: string; value: string }[] = [], goalIds: string[] = [], funnelIds: string[] = []) {
        const cmp = this.computePreviousPeriod(from, to);
        this.currentComparisonRange.set(cmp);
        this.isLoading.set(true);
        this.fetchStats(siteId, from, to, filters, goalIds, funnelIds)
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: (data) => this.stats.set(data),
                error: (e) => console.error(e)
            });
    }

    fetchStats(siteId: string, from: string, to: string, filters: { type: string; value: string }[] = [], goalIds: string[] = [], funnelIds: string[] = []): Observable<SiteStats> {
        const cmp = this.computePreviousPeriod(from, to);
        let params = new HttpParams().set('from', from).set('to', to).set('compare_from', cmp.from).set('compare_to', cmp.to);
        for (const filter of filters) {
            params = params.append('filter', `${filter.type}:${filter.value}`);
        }
        for (const id of goalIds) {
            params = params.append('goal_id', id);
        }
        for (const id of funnelIds) {
            params = params.append('funnel_id', id);
        }

        return this.http.get<SiteStats>(`/api/sites/${siteId}/stats`, { params });
    }

    fetchSitesOverviewStats(from: string, to: string): Observable<SitesOverviewStatsResponse> {
        const params = new HttpParams().set('from', from).set('to', to);
        return this.http.get<SitesOverviewStatsResponse>('/api/sites/overview', { params });
    }

    comparisonRange(from: string, to: string): { from: string; to: string } {
        return this.computePreviousPeriod(from, to);
    }

    private computePreviousPeriod(from: string, to: string): { from: string; to: string } {
        const start = new Date(from);
        const end = new Date(to);
        const duration = end.getTime() - start.getTime();
        const cmpEnd = new Date(start.getTime() - 1);
        return {
            from: new Date(cmpEnd.getTime() - duration).toISOString(),
            to: cmpEnd.toISOString()
        };
    }
}
