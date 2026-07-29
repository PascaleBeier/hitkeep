import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, input, signal, untracked } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TranslocoPipe } from '@jsverse/transloco';
import { CardModule } from '@openng/optimus-ui/card';
import { IconFieldModule } from '@openng/optimus-ui/iconfield';
import { InputIconModule } from '@openng/optimus-ui/inputicon';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { TableLazyLoadEvent, TableModule } from '@openng/optimus-ui/table';
import { debounceTime, distinctUntilChanged, finalize, Subject } from 'rxjs';
import { buildTakeoutExportFilename, DEFAULT_HITS_EXPORT_FORMAT, TakeoutExportFormat, withTakeoutExportFormat } from '@core/export/export-formats';
import { ExportSplitButton, ExportStatusBanner } from '@components/export-split-button/export-split-button';
import { PageState } from '@components/page-state/page-state';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { HitService } from '@features/hits/services/hit.service';
import { SiteFavicon } from '@features/sites/components/site-favicon';
import { TakeoutDownloadService } from '@services/takeout-download.service';

export interface TrafficRecordFilter {
    type: string;
    value: string;
}

/**
 * Shared raw-traffic report surface. Each mounted card owns its request state so
 * pagination, cancellation, errors, and retries never leak into another report.
 */
@Component({
    selector: 'app-traffic-records-card',
    standalone: true,
    imports: [CardModule, ExportSplitButton, ExportStatusBanner, IconFieldModule, InputIconModule, InputTextModule, PageState, RelativeDateTime, SiteFavicon, TableModule, TranslocoPipe],
    providers: [HitService],
    templateUrl: './traffic-records-card.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TrafficRecordsCard {
    siteId = input<string | null>(null);
    siteDomain = input<string | null>(null);
    from = input<string | null>(null);
    to = input<string | null>(null);
    filters = input<TrafficRecordFilter[]>([]);
    goalIds = input<string[]>([]);
    funnelIds = input<string[]>([]);
    enabled = input(true);
    refreshKey = input(0);
    shareToken = input<string | null>(null);
    showExport = input(false);
    titleKey = input('dashboard.rawHitsTitle');
    descriptionKey = input<string | null>(null);
    unavailableTitleKey = input('trafficRecords.unavailableTitle');
    unavailableDescriptionKey = input('trafficRecords.unavailableDescription');

    protected readonly hitService = inject(HitService);
    private readonly takeoutDownload = inject(TakeoutDownloadService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly searchSubject = new Subject<string>();
    private lastTableEvent: TableLazyLoadEvent | null = null;
    private lastLoadedScope = '';

    protected readonly searchQuery = signal('');
    protected readonly isExporting = signal(false);
    protected readonly exportState = signal<'idle' | 'success' | 'error'>('idle');
    private readonly requestScope = computed(() =>
        JSON.stringify({
            siteId: this.siteId(),
            from: this.from(),
            to: this.to(),
            filters: this.filters(),
            goalIds: this.goalIds(),
            funnelIds: this.funnelIds(),
            enabled: this.enabled(),
            refreshKey: this.refreshKey(),
            shareToken: this.shareToken()
        })
    );
    protected readonly exportUrl = computed(() => {
        const siteId = this.siteId();
        const from = this.from();
        const to = this.to();
        if (!this.enabled() || !siteId || !from || !to) return '';

        const params = new URLSearchParams({ from, to });
        const query = this.searchQuery();
        if (query) params.set('q', query);
        for (const filter of this.filters()) params.append('filter', `${filter.type}:${filter.value}`);
        for (const id of this.goalIds()) params.append('goal_id', id);
        for (const id of this.funnelIds()) params.append('funnel_id', id);

        const token = this.shareToken();
        return token ? `/api/share/${encodeURIComponent(token)}/sites/${siteId}/hits/export?${params.toString()}` : `/api/sites/${siteId}/hits/export?${params.toString()}`;
    });

    constructor() {
        this.searchSubject.pipe(debounceTime(400), distinctUntilChanged(), takeUntilDestroyed(this.destroyRef)).subscribe((query) => {
            this.searchQuery.set(query);
            this.reloadFromFirstPage();
        });

        effect(() => {
            const scope = this.requestScope();
            if (!this.enabled()) {
                untracked(() => this.hitService.reset());
                this.lastLoadedScope = scope;
                return;
            }
            if (!this.lastTableEvent || scope === this.lastLoadedScope) return;
            untracked(() => this.reloadFromFirstPage());
        });
    }

    protected onSearch(event: Event) {
        this.searchSubject.next((event.target as HTMLInputElement).value);
    }

    protected loadHits(event: TableLazyLoadEvent) {
        this.lastTableEvent = { ...event };
        const siteId = this.siteId();
        const from = this.from();
        const to = this.to();
        if (!this.enabled() || !siteId || !from || !to) {
            this.hitService.reset();
            return;
        }

        const rows = event.rows || 10;
        const first = event.first || 0;
        this.lastLoadedScope = this.requestScope();
        this.hitService.loadHits(siteId, from, to, first / rows + 1, rows, event.sortField as string, event.sortOrder === 1 ? 'asc' : 'desc', this.searchQuery(), this.filters(), this.goalIds(), this.funnelIds(), this.shareToken());
    }

    protected retry() {
        if (this.lastTableEvent) this.loadHits(this.lastTableEvent);
    }

    protected exportTraffic(format: TakeoutExportFormat = DEFAULT_HITS_EXPORT_FORMAT) {
        const url = withTakeoutExportFormat(this.exportUrl(), format);
        if (!url || this.isExporting()) return;
        this.isExporting.set(true);
        this.exportState.set('idle');
        this.takeoutDownload
            .downloadFromUrl(url, buildTakeoutExportFilename(this.siteDomain(), 'hits', format))
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isExporting.set(false))
            )
            .subscribe({
                next: () => this.exportState.set('success'),
                error: () => this.exportState.set('error')
            });
    }

    protected buildSiteUrl(path: string | null | undefined): string | null {
        const domain = this.siteDomain();
        if (!domain || !path) return null;
        return `https://${domain}${path.startsWith('/') ? path : `/${path}`}`;
    }

    protected buildReferrerUrl(referrer: string | null | undefined): string | null {
        return this.normalizeUrl(referrer)?.href ?? null;
    }

    protected displayReferrerUrl(url: string | null | undefined): string {
        return url?.replace(/^https?:\/\//, '').replace(/^www\./, '') ?? '';
    }

    protected referrerDomain(referrer: string | null | undefined): string | null {
        return this.normalizeUrl(referrer)?.hostname ?? null;
    }

    private reloadFromFirstPage() {
        if (!this.lastTableEvent) return;
        this.loadHits({ ...this.lastTableEvent, first: 0 });
    }

    private normalizeUrl(raw: string | null | undefined): URL | null {
        if (!raw) return null;
        const trimmed = raw.trim();
        if (!trimmed || trimmed.toLowerCase() === 'direct') return null;
        try {
            return new URL(/^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`);
        } catch {
            return null;
        }
    }
}
