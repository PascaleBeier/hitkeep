import { TranslocoService } from '@jsverse/transloco';
import { MenuItem } from '@openng/optimus-ui/api';

export const TAKEOUT_EXPORT_FORMATS = ['csv', 'xlsx', 'parquet', 'json', 'ndjson'] as const;
export type TakeoutExportFormat = (typeof TAKEOUT_EXPORT_FORMATS)[number];

export const DEFAULT_TAKEOUT_EXPORT_FORMAT: TakeoutExportFormat = 'xlsx';
export const DEFAULT_HITS_EXPORT_FORMAT: TakeoutExportFormat = 'csv';

interface TakeoutExportMenuOption {
    format: TakeoutExportFormat;
    icon: string;
    labelKey: string;
}

const TAKEOUT_EXPORT_MENU_OPTIONS: readonly TakeoutExportMenuOption[] = [
    { format: 'csv', icon: 'pi pi-file', labelKey: 'common.exportFormats.csv' },
    { format: 'xlsx', icon: 'pi pi-file-excel', labelKey: 'common.exportFormats.xlsx' },
    { format: 'parquet', icon: 'pi pi-database', labelKey: 'common.exportFormats.parquet' },
    { format: 'json', icon: 'pi pi-file', labelKey: 'common.exportFormats.json' },
    { format: 'ndjson', icon: 'pi pi-file', labelKey: 'common.exportFormats.ndjson' }
];

export function buildTakeoutExportMenuItems(transloco: TranslocoService, onSelect: (format: TakeoutExportFormat) => void): MenuItem[] {
    return TAKEOUT_EXPORT_MENU_OPTIONS.map((option) => ({
        label: transloco.translate(option.labelKey),
        icon: option.icon,
        command: () => onSelect(option.format)
    }));
}

/**
 * Builds a download filename such as `example-com-ai-fetches-2026-05-18.csv`.
 * The domain is slugified; unusable domains fall back to `site`.
 */
export function buildTakeoutExportFilename(domain: string | null | undefined, slug: string, format: TakeoutExportFormat): string {
    const safeDomain = (domain || 'site')
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/(^-|-$)/g, '');
    const dateStamp = new Date().toISOString().slice(0, 10);
    return `${safeDomain || 'site'}-${slug}-${dateStamp}.${format}`;
}

/** Appends (or replaces) the `format` query parameter of an export URL. */
export function withTakeoutExportFormat(baseUrl: string, format: TakeoutExportFormat): string {
    if (!baseUrl) return '';
    const url = new URL(baseUrl, window.location.origin);
    url.searchParams.set('format', format);
    return url.pathname + `?${url.searchParams.toString()}`;
}
