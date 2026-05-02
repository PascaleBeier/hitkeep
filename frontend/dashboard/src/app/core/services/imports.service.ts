import { HttpClient, HttpParams } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';

export interface ImportProviderDescriptor {
    key: string;
    name: string;
    accepted_extensions: string[];
    capabilities: string[];
}

export interface ImportUploadFileInput {
    filename: string;
    size_bytes: number;
    sha256?: string;
}

export interface ImportUploadFile {
    id: string;
    filename: string;
    size_bytes: number;
    bytes_received: number;
    sha256?: string;
    status: string;
}

export interface ImportUploadCreateResponse {
    import_id: string;
    provider: string;
    status: string;
    chunk_size: number;
    files: ImportUploadFile[];
}

export interface ImportChunkResponse {
    import_id: string;
    file_id: string;
    bytes_received: number;
    complete: boolean;
}

export interface ImportWarning {
    code: string;
    message: string;
    file?: string;
}

export interface ImportDatasetSummary {
    key: string;
    name: string;
    files: string[];
    rows_scanned: number;
    rows_accepted: number;
    rows_skipped: number;
    visitors?: number;
    visits?: number;
    pageviews?: number;
    events?: number;
}

export interface ImportEventCoverage {
    rows_scanned: number;
    rows_accepted: number;
    events: number;
    visitors: number;
    event_names: string[];
    property_keys: string[];
}

export interface ImportEventPropertyCoverage {
    attributed_rows: number;
    attributed_events: number;
    attributed_visitors: number;
    attributed_property_keys: string[];
    unattributed_rows: number;
    unattributed_events: number;
    unattributed_visitors: number;
    unattributed_property_keys: string[];
    unattributed_relationship?: string;
    unavailable_relationship_message?: string;
}

export interface ImportEventDimensionCoverage {
    available: string[];
    unavailable: string[];
    reason?: string;
}

export interface ImportOverlapSummary {
    policy: string;
    native_traffic_days: number;
    native_event_days: number;
    native_event_keys: number;
    estimated_skipped_rows: number;
    estimated_skipped_pageviews: number;
    estimated_skipped_events: number;
}

export interface ImportManifest {
    provider: string;
    source_hash: string;
    date_start?: string;
    date_end?: string;
    files: string[];
    ignored_files: string[];
    missing_files: string[];
    datasets: ImportDatasetSummary[];
    event_coverage: ImportEventCoverage;
    event_property_coverage: ImportEventPropertyCoverage;
    event_dimension_coverage: ImportEventDimensionCoverage;
    overlap: ImportOverlapSummary;
    warnings: ImportWarning[];
    rows_scanned: number;
    rows_accepted: number;
    rows_skipped: number;
}

export interface ImportJob {
    id: string;
    site_id: string;
    provider: string;
    status: string;
    source_hash?: string;
    bytes_total: number;
    bytes_received: number;
    rows_scanned: number;
    rows_imported: number;
    error?: string;
    manifest?: ImportManifest;
    files?: ImportUploadFile[];
    created_at: string;
    updated_at: string;
    validated_at?: string;
    started_at?: string;
    finished_at?: string;
}

export interface ImportListResponse {
    imports: ImportJob[];
}

@Injectable({ providedIn: 'root' })
export class ImportsService {
    private readonly http = inject(HttpClient);

    listImporters(siteId: string) {
        return this.http.get<ImportProviderDescriptor[]>(`/api/sites/${siteId}/importers`);
    }

    createUpload(siteId: string, provider: string, files: ImportUploadFileInput[]) {
        return this.http.post<ImportUploadCreateResponse>(`/api/sites/${siteId}/imports/${provider}/uploads`, { files });
    }

    uploadChunk(siteId: string, importId: string, fileId: string, offset: number, chunk: Blob) {
        const params = new HttpParams().set('offset', offset);
        return this.http.put<ImportChunkResponse>(`/api/sites/${siteId}/imports/uploads/${importId}/files/${fileId}/chunks`, chunk, { params });
    }

    validate(siteId: string, importId: string) {
        return this.http.post<ImportJob>(`/api/sites/${siteId}/imports/uploads/${importId}/validate`, {});
    }

    start(siteId: string, importId: string) {
        return this.http.post<ImportJob>(`/api/sites/${siteId}/imports/${importId}/start`, {});
    }

    get(siteId: string, importId: string) {
        return this.http.get<ImportJob>(`/api/sites/${siteId}/imports/${importId}`);
    }

    list(siteId: string) {
        return this.http.get<ImportListResponse>(`/api/sites/${siteId}/imports`);
    }

    delete(siteId: string, importId: string) {
        return this.http.delete<{ status: string }>(`/api/sites/${siteId}/imports/${importId}`);
    }
}
