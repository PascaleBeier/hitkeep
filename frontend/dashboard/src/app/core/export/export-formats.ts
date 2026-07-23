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
