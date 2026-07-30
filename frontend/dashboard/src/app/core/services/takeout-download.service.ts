import { HttpClient, HttpResponse } from '@angular/common/http';
import { inject, Service } from '@angular/core';
import { buildTakeoutExportFilename, TakeoutExportFormat } from '@core/export/export-formats';
import { map, Observable } from 'rxjs';

@Service()
export class TakeoutDownloadService {
    private http = inject(HttpClient);

    downloadUserTakeout(format: TakeoutExportFormat): Observable<string> {
        // No domain to slugify here, so `user` takes the slot instead.
        const fallbackFilename = buildTakeoutExportFilename('user', 'takeout', format);
        return this.downloadFromUrl(`/api/user/takeout?format=${format}`, fallbackFilename);
    }

    downloadSiteTakeout(siteID: string, domain: string | undefined, format: TakeoutExportFormat): Observable<string> {
        const fallbackFilename = buildTakeoutExportFilename(domain, 'takeout', format);
        return this.downloadFromUrl(`/api/sites/${siteID}/takeout?format=${format}`, fallbackFilename);
    }

    downloadFromUrl(url: string, fallbackFilename: string): Observable<string> {
        return this.http.get(url, { responseType: 'blob', observe: 'response' }).pipe(map((response) => this.persistDownload(response, fallbackFilename)));
    }

    private persistDownload(response: HttpResponse<Blob>, fallbackFilename: string): string {
        const blob = response.body;
        if (!blob) {
            throw new Error('missing_takeout_download');
        }

        if (this.isHTMLResponse(response, blob)) {
            throw new Error('unexpected_html_download_response');
        }

        const filename = this.ensureFilenameExtension(this.extractFilename(response.headers.get('content-disposition')) ?? fallbackFilename, fallbackFilename);
        this.saveBlob(blob, filename);
        return filename;
    }

    private isHTMLResponse(response: HttpResponse<Blob>, blob: Blob): boolean {
        const contentType = (response.headers.get('content-type') ?? blob.type ?? '').toLowerCase();
        return contentType.includes('text/html') || contentType.includes('application/xhtml+xml');
    }

    private saveBlob(blob: Blob, filename: string): void {
        const objectURL = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = objectURL;
        link.download = filename;
        link.style.display = 'none';
        document.body.appendChild(link);
        link.click();
        link.remove();
        URL.revokeObjectURL(objectURL);
    }

    private extractFilename(header: string | null): string | null {
        if (!header) return null;

        const encodedMatch = header.match(/filename\*=UTF-8''([^;]+)/i);
        if (encodedMatch?.[1]) {
            try {
                return decodeURIComponent(encodedMatch[1]);
            } catch {
                return encodedMatch[1];
            }
        }

        const match = header.match(/filename="?([^";]+)"?/i);
        return match?.[1] ?? null;
    }

    private ensureFilenameExtension(filename: string, fallbackFilename: string): string {
        if (this.fileExtension(filename)) {
            return filename;
        }

        const fallbackExtension = this.fileExtension(fallbackFilename);
        if (!fallbackExtension) {
            return filename;
        }

        return `${filename}.${fallbackExtension}`;
    }

    private fileExtension(filename: string): string {
        const trimmed = filename.trim();
        const lastDot = trimmed.lastIndexOf('.');
        if (lastDot <= 0 || lastDot === trimmed.length - 1) {
            return '';
        }
        return trimmed.slice(lastDot + 1);
    }
}
