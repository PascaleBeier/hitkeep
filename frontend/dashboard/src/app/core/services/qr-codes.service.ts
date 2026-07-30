import { HttpClient, HttpParams } from '@angular/common/http';
import { inject, Service } from '@angular/core';
import { Observable } from 'rxjs';
import { TakeoutExportFormat } from '@core/export/export-formats';
import { QRCode, QRCodeAsset, QRCodeOpenSeriesPoint, QRCodeRequest, QRCodeShareLink, QRCodeSummary } from '@models/analytics.types';
import { TakeoutDownloadService } from '@services/takeout-download.service';

export interface QRCodeShareLinkResponse extends QRCodeShareLink {
    token: string;
    url: string;
}

@Service()
export class QRCodesService {
    private readonly http = inject(HttpClient);
    private readonly takeout = inject(TakeoutDownloadService);

    list(siteID: string, includeArchived = false): Observable<QRCode[]> {
        let params = new HttpParams();
        if (includeArchived) params = params.set('include_archived', 'true');
        return this.http.get<QRCode[]>(`/api/sites/${siteID}/qr-codes`, { params });
    }

    get(siteID: string, qrID: string): Observable<QRCode> {
        return this.http.get<QRCode>(`/api/sites/${siteID}/qr-codes/${qrID}`);
    }

    create(siteID: string, request: QRCodeRequest): Observable<QRCode> {
        return this.http.post<QRCode>(`/api/sites/${siteID}/qr-codes`, request);
    }

    update(siteID: string, qrID: string, request: QRCodeRequest): Observable<QRCode> {
        return this.http.patch<QRCode>(`/api/sites/${siteID}/qr-codes/${qrID}`, request);
    }

    archive(siteID: string, qrID: string): Observable<void> {
        return this.http.delete<void>(`/api/sites/${siteID}/qr-codes/${qrID}`);
    }

    uploadAsset(siteID: string, qrID: string, file: File): Observable<QRCodeAsset> {
        const form = new FormData();
        form.append('asset', file, file.name);
        return this.http.put<QRCodeAsset>(`/api/sites/${siteID}/qr-codes/${qrID}/asset`, form);
    }

    deleteAsset(siteID: string, qrID: string): Observable<void> {
        return this.http.delete<void>(`/api/sites/${siteID}/qr-codes/${qrID}/asset`);
    }

    assetURL(siteID: string, qrID: string): string {
        return `/api/sites/${siteID}/qr-codes/${qrID}/asset`;
    }

    summary(siteID: string, qrID: string, from?: string, to?: string): Observable<QRCodeSummary> {
        return this.http.get<QRCodeSummary>(`/api/sites/${siteID}/qr-codes/${qrID}/summary`, { params: this.rangeParams(from, to) });
    }

    openSeries(siteID: string, qrID: string, from?: string, to?: string): Observable<QRCodeOpenSeriesPoint[]> {
        return this.http.get<QRCodeOpenSeriesPoint[]>(`/api/sites/${siteID}/qr-codes/${qrID}/opens/timeseries`, { params: this.rangeParams(from, to) });
    }

    listShares(siteID: string, qrID: string): Observable<QRCodeShareLink[]> {
        return this.http.get<QRCodeShareLink[]>(`/api/sites/${siteID}/qr-codes/${qrID}/share`);
    }

    createShare(siteID: string, qrID: string): Observable<QRCodeShareLinkResponse> {
        return this.http.post<QRCodeShareLinkResponse>(`/api/sites/${siteID}/qr-codes/${qrID}/share`, {});
    }

    deleteShare(siteID: string, qrID: string, shareID: string): Observable<void> {
        return this.http.delete<void>(`/api/sites/${siteID}/qr-codes/${qrID}/share/${shareID}`);
    }

    getQRShare(token: string): Observable<QRCode> {
        return this.http.get<QRCode>(`/api/qr-share/${token}/qr-code`);
    }

    qrShareAssetURL(token: string): string {
        return `/api/qr-share/${token}/qr-code/asset`;
    }

    qrShareSummary(token: string, from?: string, to?: string): Observable<QRCodeSummary> {
        return this.http.get<QRCodeSummary>(`/api/qr-share/${token}/qr-code/summary`, { params: this.rangeParams(from, to) });
    }

    qrShareOpenSeries(token: string, from?: string, to?: string): Observable<QRCodeOpenSeriesPoint[]> {
        return this.http.get<QRCodeOpenSeriesPoint[]>(`/api/qr-share/${token}/qr-code/opens/timeseries`, { params: this.rangeParams(from, to) });
    }

    downloadTakeout(siteID: string, qr: QRCode, format: TakeoutExportFormat, siteDomain?: string): Observable<string> {
        return this.takeout.downloadFromUrl(`/api/sites/${siteID}/qr-codes/${qr.id}/takeout?format=${format}`, `${qrExportFilename(siteDomain, qr.name, 'analytics')}.${format}`);
    }

    downloadQRShareTakeout(token: string, qr: QRCode, format: TakeoutExportFormat): Observable<string> {
        return this.takeout.downloadFromUrl(`/api/qr-share/${token}/qr-code/takeout?format=${format}`, `${qrExportFilename(undefined, qr.name, 'analytics')}.${format}`);
    }

    private rangeParams(from?: string, to?: string): HttpParams {
        let params = new HttpParams();
        if (from) params = params.set('from', from);
        if (to) params = params.set('to', to);
        return params;
    }
}

export function qrExportFilename(siteDomain: string | undefined, name: string | undefined, suffix: string, extension?: string): string {
    const safeDomain = slug(siteDomain || '');
    const safeName = slug(name || 'qr-code').slice(0, 72);
    return [safeDomain, safeName || 'qr-code', suffix].filter(Boolean).join('-') + (extension ? `.${extension}` : '');
}

function slug(value: string): string {
    return value
        .toLowerCase()
        .replace(/^https?:\/\//, '')
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/(^-|-$)/g, '');
}

export function buildQRCodeDestination(qr: Pick<QRCodeRequest, 'destination_url' | 'utm_source' | 'utm_medium' | 'utm_campaign' | 'utm_term' | 'utm_content' | 'custom_params'>, qrID?: string): string {
    const rawURL = qr.destination_url.trim();
    if (!rawURL) return '';
    try {
        const url = new URL(rawURL);
        setParam(url, 'utm_source', qr.utm_source);
        setParam(url, 'utm_medium', qr.utm_medium);
        setParam(url, 'utm_campaign', qr.utm_campaign);
        setParam(url, 'utm_term', qr.utm_term);
        setParam(url, 'utm_content', qr.utm_content);
        for (const [key, value] of Object.entries(qr.custom_params ?? {})) {
            const cleanKey = key.trim();
            if (!cleanKey) continue;
            setParam(url, cleanKey, value);
        }
        if (qrID) url.searchParams.set('hk_qr', qrID);
        return url.toString();
    } catch {
        return '';
    }
}

function setParam(url: URL, key: string, value: string | undefined): void {
    const trimmed = value?.trim();
    if (trimmed) url.searchParams.set(key, trimmed);
}
