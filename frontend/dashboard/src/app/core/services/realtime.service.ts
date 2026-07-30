import { DOCUMENT } from '@angular/common';
import { DestroyRef, computed, effect, inject, signal, Service } from '@angular/core';
import { Subject } from 'rxjs';
import { SiteService } from '@features/sites/services/site.service';
import { ShareService } from '@services/share.service';
import { browserAbsoluteAppUrl } from '@core/interceptors/base-path.interceptor';

export const REALTIME_KINDS = {
    hits: 'hits',
    events: 'events',
    ecommerce: 'ecommerce',
    webVitals: 'web_vitals',
    aiFetch: 'ai_fetch',
    imports: 'imports',
    goals: 'goals',
    funnels: 'funnels',
    opportunities: 'opportunities'
} as const;

export type RealtimeKind = (typeof REALTIME_KINDS)[keyof typeof REALTIME_KINDS];
export type RealtimeStatus = 'idle' | 'connecting' | 'open' | 'error';

export const REALTIME_TRAFFIC_KINDS = [REALTIME_KINDS.hits] as const;
export const REALTIME_EVENT_KINDS = [REALTIME_KINDS.events, REALTIME_KINDS.ecommerce] as const;
export const REALTIME_GOAL_KINDS = [REALTIME_KINDS.hits, REALTIME_KINDS.events, REALTIME_KINDS.goals] as const;
export const REALTIME_FUNNEL_KINDS = [REALTIME_KINDS.hits, REALTIME_KINDS.events, REALTIME_KINDS.funnels] as const;
export const REALTIME_OPPORTUNITY_KINDS = [REALTIME_KINDS.opportunities, REALTIME_KINDS.hits, REALTIME_KINDS.events, REALTIME_KINDS.ecommerce, REALTIME_KINDS.webVitals, REALTIME_KINDS.aiFetch] as const;
export const REALTIME_ALL_ANALYTICS_KINDS = [
    REALTIME_KINDS.hits,
    REALTIME_KINDS.events,
    REALTIME_KINDS.ecommerce,
    REALTIME_KINDS.webVitals,
    REALTIME_KINDS.aiFetch,
    REALTIME_KINDS.imports,
    REALTIME_KINDS.goals,
    REALTIME_KINDS.funnels,
    REALTIME_KINDS.opportunities
] as const;

export interface RealtimeEvent {
    type: 'analytics.changed' | 'analytics.resync';
    site_id: string;
    kinds: RealtimeKind[];
    changed_at: string;
    bucket_start: string;
    counts: Partial<Record<RealtimeKind, number>>;
}

@Service()
export class RealtimeService {
    private readonly siteService = inject(SiteService);
    private readonly shareService = inject(ShareService);
    private readonly document = inject(DOCUMENT);
    private readonly destroyRef = inject(DestroyRef);
    private readonly eventsSubject = new Subject<RealtimeEvent>();

    readonly events$ = this.eventsSubject.asObservable();
    readonly status = signal<RealtimeStatus>('idle');
    readonly activeSiteId = signal<string | null>(null);
    readonly isOpen = computed(() => this.status() === 'open');

    private source: EventSource | null = null;
    private sourceKey = '';

    constructor() {
        effect(() => {
            const shareToken = this.shareService.token();
            const shareSite = this.shareService.site();
            const activeSite = this.siteService.activeSite();
            const siteID = shareToken ? (shareSite?.id ?? null) : (activeSite?.id ?? null);
            const key = siteID ? `${shareToken ?? 'site'}:${siteID}` : '';

            if (!siteID || key === this.sourceKey) {
                if (!siteID) {
                    this.closeSource();
                    this.activeSiteId.set(null);
                    this.status.set('idle');
                }
                return;
            }

            this.openSource(siteID, shareToken);
        });
        this.destroyRef.onDestroy(() => this.closeSource());
    }

    private openSource(siteID: string, shareToken: string | null): void {
        this.closeSource();

        if (typeof EventSource === 'undefined') {
            this.status.set('error');
            this.activeSiteId.set(siteID);
            return;
        }

        const path = shareToken ? `/api/share/${encodeURIComponent(shareToken)}/sites/${encodeURIComponent(siteID)}/realtime` : `/api/sites/${encodeURIComponent(siteID)}/realtime`;
        const source = new EventSource(browserAbsoluteAppUrl(this.document, path), { withCredentials: true });
        this.source = source;
        this.sourceKey = `${shareToken ?? 'site'}:${siteID}`;
        this.activeSiteId.set(siteID);
        this.status.set('connecting');

        source.onopen = () => this.status.set('open');
        source.onerror = () => this.status.set('error');
        source.addEventListener('analytics.changed', (event) => this.emitEvent('analytics.changed', event));
        source.addEventListener('analytics.resync', (event) => this.emitEvent('analytics.resync', event));
    }

    private emitEvent(type: RealtimeEvent['type'], event: Event): void {
        if (!(event instanceof MessageEvent) || typeof event.data !== 'string') {
            return;
        }
        try {
            const payload = JSON.parse(event.data) as Omit<RealtimeEvent, 'type'>;
            if (!payload.site_id || !Array.isArray(payload.kinds)) return;
            this.eventsSubject.next({ ...payload, type });
        } catch {
            this.status.set('error');
        }
    }

    private closeSource(): void {
        if (this.source) {
            this.source.close();
            this.source = null;
        }
        this.sourceKey = '';
    }
}
