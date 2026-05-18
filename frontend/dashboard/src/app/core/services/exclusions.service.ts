import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';

import { CurrentIP, IPExclusion } from '@models/analytics.types';

interface CreateExclusionPayload {
    type?: 'cidr' | 'country';
    cidr?: string;
    country_code?: string;
    description?: string;
}

@Injectable({ providedIn: 'root' })
export class ExclusionsService {
    private http = inject(HttpClient);

    listSiteExclusions(siteID: string) {
        return this.http.get<IPExclusion[]>(`/api/sites/${siteID}/exclusions`);
    }

    createSiteExclusion(siteID: string, payload: CreateExclusionPayload) {
        return this.http.post<IPExclusion>(`/api/sites/${siteID}/exclusions`, payload);
    }

    deleteSiteExclusion(siteID: string, ruleID: string) {
        return this.http.delete<void>(`/api/sites/${siteID}/exclusions/${ruleID}`);
    }

    listInstanceExclusions() {
        return this.http.get<IPExclusion[]>('/api/admin/exclusions');
    }

    createInstanceExclusion(payload: CreateExclusionPayload) {
        return this.http.post<IPExclusion>('/api/admin/exclusions', payload);
    }

    deleteInstanceExclusion(ruleID: string) {
        return this.http.delete<void>(`/api/admin/exclusions/${ruleID}`);
    }

    getCurrentIP() {
        return this.http.get<CurrentIP>('/api/user/current-ip');
    }
}
