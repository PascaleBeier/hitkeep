import { TranslocoService } from '@jsverse/transloco';
import { vi } from 'vitest';

import { buildTakeoutExportFilename, buildTakeoutExportMenuItems, DEFAULT_HITS_EXPORT_FORMAT, DEFAULT_TAKEOUT_EXPORT_FORMAT, TAKEOUT_EXPORT_FORMATS, withTakeoutExportFormat } from './export-formats';

describe('export-formats', () => {
    it('should expose all supported export formats in one place', () => {
        expect(TAKEOUT_EXPORT_FORMATS).toEqual(['csv', 'xlsx', 'parquet', 'json', 'ndjson']);
        expect(DEFAULT_TAKEOUT_EXPORT_FORMAT).toBe('xlsx');
        expect(DEFAULT_HITS_EXPORT_FORMAT).toBe('csv');
    });

    it('should build translated menu items that call onSelect with matching format', () => {
        const selected: string[] = [];
        const transloco = {
            translate: (key: string) => `tr:${key}`
        } as unknown as TranslocoService;

        const menuItems = buildTakeoutExportMenuItems(transloco, (format) => selected.push(format));

        expect(menuItems.length).toBe(TAKEOUT_EXPORT_FORMATS.length);
        expect(menuItems.map((item) => item.label)).toEqual(['tr:common.exportFormats.csv', 'tr:common.exportFormats.xlsx', 'tr:common.exportFormats.parquet', 'tr:common.exportFormats.json', 'tr:common.exportFormats.ndjson']);

        for (const item of menuItems) {
            item.command?.({} as never);
        }
        expect(selected).toEqual([...TAKEOUT_EXPORT_FORMATS]);
    });

    describe('buildTakeoutExportFilename', () => {
        it('should slugify the domain and stamp the current date', () => {
            vi.useFakeTimers();
            vi.setSystemTime(new Date('2026-05-18T10:20:30Z'));

            expect(buildTakeoutExportFilename('Example.COM', 'ai-fetches', 'xlsx')).toBe('example-com-ai-fetches-2026-05-18.xlsx');
            expect(buildTakeoutExportFilename('sub.example.com', 'ai-chatbots', 'csv')).toBe('sub-example-com-ai-chatbots-2026-05-18.csv');

            vi.useRealTimers();
        });

        it('should trim leading and trailing separators from the slug', () => {
            vi.useFakeTimers();
            vi.setSystemTime(new Date('2026-05-18T10:20:30Z'));

            expect(buildTakeoutExportFilename('--example--', 'hits', 'json')).toBe('example-hits-2026-05-18.json');

            vi.useRealTimers();
        });

        it('should fall back to "site" for missing or unusable domains', () => {
            vi.useFakeTimers();
            vi.setSystemTime(new Date('2026-05-18T10:20:30Z'));

            expect(buildTakeoutExportFilename(null, 'hits', 'csv')).toBe('site-hits-2026-05-18.csv');
            expect(buildTakeoutExportFilename(undefined, 'hits', 'csv')).toBe('site-hits-2026-05-18.csv');
            expect(buildTakeoutExportFilename('', 'hits', 'csv')).toBe('site-hits-2026-05-18.csv');
            expect(buildTakeoutExportFilename('///', 'hits', 'csv')).toBe('site-hits-2026-05-18.csv');

            vi.useRealTimers();
        });
    });

    describe('withTakeoutExportFormat', () => {
        it('should append the format to an existing query string', () => {
            expect(withTakeoutExportFormat('/api/sites/site-1/ai-fetch/export?from=a&to=b', 'parquet')).toBe('/api/sites/site-1/ai-fetch/export?from=a&to=b&format=parquet');
        });

        it('should replace an existing format parameter', () => {
            expect(withTakeoutExportFormat('/api/sites/site-1/hits/export?format=csv', 'ndjson')).toBe('/api/sites/site-1/hits/export?format=ndjson');
        });

        it('should keep repeated filter parameters', () => {
            expect(withTakeoutExportFormat('/api/sites/site-1/ai-chatbots/export?filter=path:/a&filter=device:Desktop', 'csv')).toBe('/api/sites/site-1/ai-chatbots/export?filter=path%3A%2Fa&filter=device%3ADesktop&format=csv');
        });

        it('should return an empty string for an empty base url', () => {
            expect(withTakeoutExportFormat('', 'csv')).toBe('');
        });
    });
});
