import { inject, Service } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import type { InstanceRole, SiteRole } from '@core/access/capabilities';

export type { InstanceRole, SiteRole } from '@core/access/capabilities';

export interface APIClientSiteRole {
    site_id: string;
    role: SiteRole;
}

export interface APIClient {
    id: string;
    user_id?: string | null;
    tenant_id?: string | null;
    owner_type: 'personal' | 'team';
    name: string;
    description: string;
    instance_role: InstanceRole;
    expires_at?: string | null;
    last_used_at?: string | null;
    revoked_at?: string | null;
    created_at: string;
    updated_at: string;
    site_roles: APIClientSiteRole[];
}

export interface CreateAPIClientRequest {
    name: string;
    description: string;
    instance_role: InstanceRole;
    expires_at?: string | null;
    site_roles: APIClientSiteRole[];
}

export interface UpdateAPIClientRequest {
    name: string;
    description: string;
    instance_role: InstanceRole;
    expires_at?: string | null;
    revoked?: boolean;
    site_roles: APIClientSiteRole[];
}

export interface CreateAPIClientResponse {
    client: APIClient;
    token: string;
}

@Service()
export class APIClientsService {
    private http = inject(HttpClient);

    listClients(teamID?: string | null): Observable<APIClient[]> {
        return this.http.get<APIClient[]>(this.basePath(teamID));
    }

    createClient(payload: CreateAPIClientRequest, teamID?: string | null): Observable<CreateAPIClientResponse> {
        return this.http.post<CreateAPIClientResponse>(this.basePath(teamID), payload);
    }

    updateClient(clientID: string, payload: UpdateAPIClientRequest, teamID?: string | null): Observable<APIClient> {
        return this.http.put<APIClient>(`${this.basePath(teamID)}/${encodeURIComponent(clientID)}`, payload);
    }

    rotateClient(clientID: string, teamID?: string | null): Observable<CreateAPIClientResponse> {
        return this.http.post<CreateAPIClientResponse>(`${this.basePath(teamID)}/${encodeURIComponent(clientID)}/rotate`, {});
    }

    deleteClient(clientID: string, teamID?: string | null): Observable<void> {
        return this.http.delete<void>(`${this.basePath(teamID)}/${encodeURIComponent(clientID)}`);
    }

    private basePath(teamID?: string | null): string {
        if (teamID) {
            return `/api/user/teams/${encodeURIComponent(teamID)}/api-clients`;
        }
        return '/api/user/api-clients';
    }
}
