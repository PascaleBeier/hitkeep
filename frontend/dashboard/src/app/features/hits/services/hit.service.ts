import { Injectable, inject, signal } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { finalize } from 'rxjs';
import type { Subscription } from 'rxjs';
import { Hit, PaginatedHits } from '@models/analytics.types';

@Injectable({ providedIn: 'root' })
export class HitService {
    private http = inject(HttpClient);

    readonly hits = signal<Hit[]>([]);
    readonly total = signal<number>(0);
    readonly isLoading = signal<boolean>(false);
    readonly hasError = signal<boolean>(false);

    private requestSequence = 0;
    private activeRequest: Subscription | null = null;

    loadHits(
        siteId: string,
        from: string,
        to: string,
        page = 1,
        pageSize = 10,
        sortField?: string,
        sortOrder?: string,
        query?: string,
        filters: { type: string; value: string }[] = [],
        goalIds: string[] = [],
        funnelIds: string[] = [],
        shareToken?: string | null
    ) {
        const sequence = ++this.requestSequence;
        this.activeRequest?.unsubscribe();
        this.isLoading.set(true);
        this.hasError.set(false);

        let params = new HttpParams()
            .set('from', from)
            .set('to', to)
            .set('limit', pageSize)
            .set('offset', (page - 1) * pageSize);

        if (sortField) params = params.set('sort', sortField);
        if (sortOrder) params = params.set('order', sortOrder);
        if (query) params = params.set('q', query);
        for (const filter of filters) {
            params = params.append('filter', `${filter.type}:${filter.value}`);
        }
        for (const id of goalIds) params = params.append('goal_id', id);
        for (const id of funnelIds) params = params.append('funnel_id', id);

        const endpoint = shareToken ? `/api/share/${encodeURIComponent(shareToken)}/sites/${siteId}/hits` : `/api/sites/${siteId}/hits`;
        this.activeRequest = this.http
            .get<PaginatedHits>(endpoint, { params })
            .pipe(
                finalize(() => {
                    if (sequence === this.requestSequence) this.isLoading.set(false);
                })
            )
            .subscribe({
                next: (res) => {
                    if (sequence !== this.requestSequence) return;
                    this.hits.set(res.data);
                    this.total.set(res.total);
                },
                error: () => {
                    if (sequence !== this.requestSequence) return;
                    this.hits.set([]);
                    this.total.set(0);
                    this.hasError.set(true);
                }
            });
    }

    reset() {
        this.requestSequence += 1;
        this.activeRequest?.unsubscribe();
        this.activeRequest = null;
        this.hits.set([]);
        this.total.set(0);
        this.isLoading.set(false);
        this.hasError.set(false);
    }
}
