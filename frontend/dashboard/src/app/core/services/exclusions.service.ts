import { inject, Service } from '@angular/core';
import { HttpClient } from '@angular/common/http';

import { CurrentIP, IPExclusion } from '@models/analytics.types';

export interface CreateExclusionPayload {
    type?: IPExclusion['type'];
    cidr?: string;
    country_code?: string;
    user_agent?: string;
    path?: string;
    description?: string;
}

@Service()
export class ExclusionsService {
    private http = inject(HttpClient);

    listSiteExclusions(siteID: string, effective = false) {
        return this.http.get<IPExclusion[]>(`/api/sites/${siteID}/exclusions`, { params: effective ? { effective: true } : {} });
    }

    createSiteExclusion(siteID: string, payload: CreateExclusionPayload) {
        return this.http.post<IPExclusion>(`/api/sites/${siteID}/exclusions`, payload);
    }

    deleteSiteExclusion(siteID: string, ruleID: string) {
        return this.http.delete<void>(`/api/sites/${siteID}/exclusions/${ruleID}`);
    }

    listTeamExclusions(teamID: string, effective = false) {
        return this.http.get<IPExclusion[]>(`/api/user/teams/${teamID}/exclusions`, { params: effective ? { effective: true } : {} });
    }

    createTeamExclusion(teamID: string, payload: CreateExclusionPayload) {
        return this.http.post<IPExclusion>(`/api/user/teams/${teamID}/exclusions`, payload);
    }

    deleteTeamExclusion(teamID: string, ruleID: string) {
        return this.http.delete<void>(`/api/user/teams/${teamID}/exclusions/${ruleID}`);
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
