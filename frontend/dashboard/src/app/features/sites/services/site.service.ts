import { Injectable, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { tap } from 'rxjs';
import { Site } from '@models/analytics.types';

const LAST_SITE_KEY = 'hk_last_site_id';

export type TrackingStatusState = 'waiting' | 'live' | 'dormant' | 'domain_mismatch';

export interface SiteTrackingStatus {
    site_id: string;
    tenant_id: string;
    status: TrackingStatusState;
    first_hit_at?: string;
    last_hit_at?: string;
    last_event_at?: string;
    last_hostname?: string;
    last_event_name?: string;
    last_automatic_event_at?: string;
    last_automatic_event_name?: string;
    tracker_source?: string;
    tracker_version?: string;
    configured_domain: string;
    updated_at?: string;
}

@Injectable({ providedIn: 'root' })
export class SiteService {
    private http = inject(HttpClient);

    // Global State for Sites
    readonly sites = signal<Site[]>([]);
    readonly activeSite = signal<Site | null>(null);
    readonly isLoading = signal<boolean>(false);

    loadSites() {
        this.isLoading.set(true);
        this.http.get<Site[]>('/api/sites').subscribe({
            next: (data) => {
                this.applySites(data);
                this.isLoading.set(false);
            },
            error: () => this.isLoading.set(false)
        });
    }

    applySites(data: Site[]) {
        this.sites.set(data);

        if (data.length === 0) {
            this.activeSite.set(null);
            localStorage.removeItem(LAST_SITE_KEY);
        } else if (!this.activeSite() || !data.some((site) => site.id === this.activeSite()?.id)) {
            const lastId = localStorage.getItem(LAST_SITE_KEY);
            const matchedSite = lastId ? data.find((s) => s.id === lastId) : null;
            this.activeSite.set(matchedSite || data[0]);
        }
    }

    selectSite(site: Site) {
        this.activeSite.set(site);
        localStorage.setItem(LAST_SITE_KEY, site.id);
    }

    createSite(domain: string) {
        return this.http.post<Site>('/api/sites', { domain }).pipe(
            tap((newSite) => {
                this.sites.update((list) => [newSite, ...list]);
                this.selectSite(newSite);
            })
        );
    }

    deleteSite(siteId: string) {
        return this.http.delete<void>(`/api/sites/${siteId}`).pipe(
            tap(() => {
                const updatedSites = this.sites().filter((site) => site.id !== siteId);
                this.sites.set(updatedSites);

                if (this.activeSite()?.id === siteId) {
                    const nextSite = updatedSites[0] ?? null;
                    this.activeSite.set(nextSite);
                    if (nextSite) {
                        localStorage.setItem(LAST_SITE_KEY, nextSite.id);
                    } else {
                        localStorage.removeItem(LAST_SITE_KEY);
                    }
                }
            })
        );
    }

    getTrackingStatus(siteId: string) {
        return this.http.get<SiteTrackingStatus>(`/api/sites/${siteId}/tracking/status`);
    }
}
