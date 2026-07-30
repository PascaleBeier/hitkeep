import { computed, inject, signal, Service } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { tap } from 'rxjs';
import { Site } from '@models/analytics.types';

export interface ShareLink {
    id: string;
    site_id: string;
    token_hint: string;
    created_at: string;
    url?: string;
}

export interface ShareLinkResponse extends ShareLink {
    url: string;
    token: string;
}

@Service()
export class ShareService {
    private http = inject(HttpClient);

    readonly token = signal<string | null>(null);
    readonly site = signal<Site | null>(null);
    readonly isShareMode = computed(() => !!this.token());

    setToken(token: string | null) {
        this.token.set(token);
    }

    clear() {
        this.token.set(null);
        this.site.set(null);
    }

    loadShareSite(token: string) {
        this.setToken(token);
        return this.http.get<Site>(`/api/share/${token}/site`).pipe(tap((site) => this.site.set(site)));
    }

    listShareLinks(siteId: string) {
        return this.http.get<ShareLink[]>(`/api/sites/${siteId}/share`);
    }

    createShareLink(siteId: string) {
        return this.http.post<ShareLinkResponse>(`/api/sites/${siteId}/share`, {});
    }

    deleteShareLink(siteId: string, shareId: string) {
        return this.http.delete<void>(`/api/sites/${siteId}/share/${shareId}`);
    }
}
