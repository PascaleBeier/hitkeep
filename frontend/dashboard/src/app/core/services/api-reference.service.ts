import { inject, Service } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';

export interface APIDocVersionInfo {
    version: string;
    openapi_url: string;
    latest: boolean;
}

export interface APIDocVersionsResponse {
    latest: string;
    versions: APIDocVersionInfo[];
}

export interface OpenAPISpec {
    openapi: string;
    info: {
        title: string;
        version: string;
        description?: string;
    };
    servers?: { url: string }[];
    tags?: { name: string; description?: string }[];
    security?: Record<string, string[]>[];
    paths: Record<string, Record<string, OpenAPIOperation>>;
    components?: {
        securitySchemes?: Record<string, OpenAPISecurityScheme>;
    };
}

export interface OpenAPIOperation {
    summary?: string;
    description?: string;
    tags?: string[];
    security?: Record<string, string[]>[];
}

export interface OpenAPISecurityScheme {
    type: string;
    description?: string;
    name?: string;
    in?: string;
    scheme?: string;
    bearerFormat?: string;
}

@Service()
export class APIReferenceService {
    private http = inject(HttpClient);

    getVersions(): Observable<APIDocVersionsResponse> {
        return this.http.get<APIDocVersionsResponse>('/api/docs/versions');
    }

    getSpec(version: string): Observable<OpenAPISpec> {
        return this.http.get<OpenAPISpec>(`/api/docs/${encodeURIComponent(version)}/openapi.json`);
    }
}
