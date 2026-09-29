import { computed, inject, signal, Service } from '@angular/core';
import { HttpClient, httpResource } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import type { Annotation, AnnotationInput } from '@models/analytics.types';
import { AccessService } from '@services/access.service';
import { ShareService } from '@services/share.service';
import { SiteService } from '@features/sites/services/site.service';

/** Day charts pin notes to UTC bucket starts; hour charts keep the local clock time. */
export type AnnotationGranularity = 'day' | 'hour';

/** What the note dialog opens on: a new note (no id) or an existing one. */
export interface AnnotationDraft {
    id?: string;
    startsAt: Date;
    endsAt: Date | null;
    body: string;
    granularity: AnnotationGranularity;
}

const TIMELINE_START = '2000-01-01T00:00:00Z';
const TIMELINE_LOOKAHEAD_MS = 400 * 86_400_000;

/**
 * The active site's notes for every chart on the page. One request covers the
 * site's whole timeline so charts with different windows share it and filter
 * locally. Share links are for outside readers, so notes stay off there.
 */
@Service()
export class SiteAnnotationsService {
    private readonly http = inject(HttpClient);
    private readonly sites = inject(SiteService);
    private readonly share = inject(ShareService);
    private readonly access = inject(AccessService);

    readonly siteId = computed(() => (this.share.isShareMode() ? null : (this.sites.activeSite()?.id ?? null)));

    /** Set by the first chart, so pages without charts never ask for notes. */
    private readonly wanted = signal(false);

    // ponytail: whole-timeline list per site; page by the chart window if a site collects thousands of notes.
    private readonly list = httpResource<Annotation[]>(() => {
        const siteId = this.siteId();
        if (!siteId || !this.wanted()) {
            return undefined;
        }
        return {
            url: `/api/sites/${siteId}/annotations`,
            params: { from: TIMELINE_START, to: new Date(Date.now() + TIMELINE_LOOKAHEAD_MS).toISOString() }
        };
    });

    readonly annotations = computed<Annotation[]>(() => (this.siteId() && this.list.hasValue() ? this.list.value() : []));

    readonly canWrite = computed(() => {
        const siteId = this.siteId();
        return !!siteId && this.access.canSite(siteId, SITE_CAPABILITIES.manageAnnotations);
    });

    /** Charts call this so notes load once the first one is on screen. */
    load(): void {
        this.wanted.set(true);
    }

    /** The note the dialog shows; null while it is closed. */
    readonly draft = signal<AnnotationDraft | null>(null);

    open(draft: AnnotationDraft): void {
        this.draft.set(draft);
    }

    openExisting(annotation: Annotation, granularity: AnnotationGranularity): void {
        this.open({
            id: annotation.id,
            startsAt: new Date(annotation.starts_at),
            endsAt: annotation.ends_at ? new Date(annotation.ends_at) : null,
            body: annotation.body,
            granularity
        });
    }

    close(): void {
        this.draft.set(null);
    }

    async save(id: string | undefined, input: AnnotationInput): Promise<void> {
        const url = this.siteUrl();
        await firstValueFrom(id ? this.http.put<Annotation>(`${url}/${id}`, input) : this.http.post<Annotation>(url, input));
        this.list.reload();
    }

    async remove(id: string): Promise<void> {
        await firstValueFrom(this.http.delete<void>(`${this.siteUrl()}/${id}`));
        this.list.reload();
    }

    private siteUrl(): string {
        const siteId = this.siteId();
        if (!siteId) {
            throw new Error('No active site for annotations');
        }
        return `/api/sites/${siteId}/annotations`;
    }
}
