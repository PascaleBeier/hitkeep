import { ImportManifest, ImportProviderDescriptor } from '@services/imports.service';
import { acceptsImportFileExtension, importGuideUrl, importManifestHasDatasetEvents, importManifestHasEventCoverage, importManifestHasEventDimensions, importManifestHasEventProperties } from './imports';

describe('ImportsPage file acceptance', () => {
    const plausible: ImportProviderDescriptor = {
        key: 'plausible',
        name: 'Plausible',
        accepted_extensions: ['.zip', '.csv'],
        capabilities: []
    };

    const simpleAnalytics: ImportProviderDescriptor = {
        key: 'simpleanalytics',
        name: 'Simple Analytics',
        accepted_extensions: ['.csv'],
        capabilities: []
    };

    it('accepts Plausible files by extension only', () => {
        expect(acceptsImportFileExtension(plausible, 'plausible-export.zip')).toBe(true);
        expect(acceptsImportFileExtension(plausible, 'traffic.csv')).toBe(true);
        expect(acceptsImportFileExtension(plausible, 'events.txt')).toBe(false);
    });

    it('accepts Simple Analytics files by extension only', () => {
        expect(acceptsImportFileExtension(simpleAnalytics, 'datapoints.csv')).toBe(true);
        expect(acceptsImportFileExtension(simpleAnalytics, 'traffic.csv')).toBe(true);
        expect(acceptsImportFileExtension(simpleAnalytics, 'traffic.zip')).toBe(false);
        expect(acceptsImportFileExtension(simpleAnalytics, 'traffic')).toBe(false);
    });

    it('links providers to their import guides', () => {
        expect(importGuideUrl('plausible')).toBe('https://hitkeep.com/guides/data/import-plausible/');
        expect(importGuideUrl('simpleanalytics')).toBe('https://hitkeep.com/guides/data/import-simple-analytics/');
        expect(importGuideUrl('unknown')).toBe('');
    });

    it('hides event validation boxes for traffic-only Simple Analytics manifests', () => {
        const manifest = testManifest({
            provider: 'simpleanalytics',
            datasets: [{ key: 'datapoints', name: 'Datapoints', files: ['datapoints.csv'], rows_scanned: 2, rows_accepted: 2, rows_skipped: 0, pageviews: 2 }],
            event_coverage: { rows_scanned: 0, rows_accepted: 0, events: 0, visitors: 0, event_names: null as unknown as string[], property_keys: null as unknown as string[] },
            event_property_coverage: {
                attributed_rows: 0,
                attributed_events: 0,
                attributed_visitors: 0,
                attributed_property_keys: null as unknown as string[],
                unattributed_rows: 0,
                unattributed_events: 0,
                unattributed_visitors: 0,
                unattributed_property_keys: null as unknown as string[]
            },
            event_dimension_coverage: { available: null as unknown as string[], unavailable: null as unknown as string[], reason: 'Simple Analytics datapoints exports contain pageviews, not custom event rows.' }
        });

        expect(importManifestHasDatasetEvents(manifest)).toBe(false);
        expect(importManifestHasEventCoverage(manifest)).toBe(false);
        expect(importManifestHasEventProperties(manifest)).toBe(false);
        expect(importManifestHasEventDimensions(manifest)).toBe(false);
    });

    it('keeps event validation boxes when the manifest contains custom events', () => {
        const manifest = testManifest({
            datasets: [{ key: 'custom_events', name: 'Custom events', files: ['imported_custom_events.csv'], rows_scanned: 1, rows_accepted: 1, rows_skipped: 0, events: 3 }],
            event_coverage: { rows_scanned: 1, rows_accepted: 1, events: 3, visitors: 2, event_names: ['Signup'], property_keys: ['path'] },
            event_property_coverage: {
                attributed_rows: 1,
                attributed_events: 3,
                attributed_visitors: 2,
                attributed_property_keys: ['path'],
                unattributed_rows: 0,
                unattributed_events: 0,
                unattributed_visitors: 0,
                unattributed_property_keys: []
            },
            event_dimension_coverage: { available: ['date', 'event_name', 'path'], unavailable: ['browser'], reason: 'Aggregate export' }
        });

        expect(importManifestHasDatasetEvents(manifest)).toBe(true);
        expect(importManifestHasEventCoverage(manifest)).toBe(true);
        expect(importManifestHasEventProperties(manifest)).toBe(true);
        expect(importManifestHasEventDimensions(manifest)).toBe(true);
    });
});

function testManifest(overrides: Partial<ImportManifest>): ImportManifest {
    return {
        provider: 'plausible',
        source_hash: 'test',
        files: [],
        ignored_files: [],
        missing_files: [],
        datasets: [],
        event_coverage: { rows_scanned: 0, rows_accepted: 0, events: 0, visitors: 0, event_names: [], property_keys: [] },
        event_property_coverage: {
            attributed_rows: 0,
            attributed_events: 0,
            attributed_visitors: 0,
            attributed_property_keys: [],
            unattributed_rows: 0,
            unattributed_events: 0,
            unattributed_visitors: 0,
            unattributed_property_keys: []
        },
        event_dimension_coverage: { available: [], unavailable: [] },
        overlap: {
            policy: 'skip_native_day',
            native_traffic_days: 0,
            native_event_days: 0,
            native_event_keys: 0,
            estimated_skipped_rows: 0,
            estimated_skipped_pageviews: 0,
            estimated_skipped_events: 0
        },
        warnings: [],
        rows_scanned: 0,
        rows_accepted: 0,
        rows_skipped: 0,
        ...overrides
    };
}
