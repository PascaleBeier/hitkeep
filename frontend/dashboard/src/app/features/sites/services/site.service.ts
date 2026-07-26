import { Injectable, effect, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { tap } from 'rxjs';
import { Site, SiteTrackingDomainOptions } from '@models/analytics.types';
import { sortSitesByDomain } from '@features/sites/utils/site-sort';
import { ReportSubjectService } from '@services/report-subject.service';

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

export interface SiteStatsResetResponse {
    status: 'reset';
    rows_cleared: number;
    imports_marked_deleted: number;
    families_cleared: string[];
}

@Injectable({ providedIn: 'root' })
export class SiteService {
    private http = inject(HttpClient);
    private reportSubject = inject(ReportSubjectService);

    // Global State for Sites
    readonly sites = signal<Site[]>([]);
    readonly activeSite = signal<Site | null>(null);
    readonly isLoading = signal<boolean>(false);

    constructor() {
        // Analytics surfaces hold their content across reloads. Publishing the
        // active site is what tells them the subject itself changed, so they
        // fall back to skeletons instead of animating another site's numbers.
        effect(() => this.reportSubject.set(this.activeSite()?.id ?? null));
    }

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
        this.sites.set(sortSitesByDomain(data));

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
                this.sites.update((list) => sortSitesByDomain([newSite, ...list]));
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

    renameSiteDomain(siteId: string, domain: string) {
        return this.http.put<Site>(`/api/sites/${siteId}/domain`, { domain }).pipe(
            tap((updated) => {
                this.sites.update((list) => sortSitesByDomain(list.map((site) => (site.id === updated.id ? { ...site, domain: updated.domain } : site))));
                const active = this.activeSite();
                if (active?.id === updated.id) {
                    this.activeSite.set({ ...active, domain: updated.domain });
                }
            })
        );
    }

    resetSiteStats(siteId: string, confirmDomain: string) {
        return this.http.post<SiteStatsResetResponse>(`/api/sites/${siteId}/stats/reset`, { confirm_domain: confirmDomain });
    }

    getTrackingStatus(siteId: string) {
        return this.http.get<SiteTrackingStatus>(`/api/sites/${siteId}/tracking/status`);
    }

    getTrackingDomainOptions(siteId: string) {
        return this.http.get<SiteTrackingDomainOptions>(`/api/sites/${encodeURIComponent(siteId)}/tracking-domain-options`);
    }
}
