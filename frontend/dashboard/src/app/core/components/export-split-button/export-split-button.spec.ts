import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { MenuItem } from '@openng/optimus-ui/api';

import { ExportSplitButton, ExportStatusBanner } from '@components/export-split-button/export-split-button';
import { TAKEOUT_EXPORT_FORMATS, TakeoutExportFormat } from '@core/export/export-formats';

const translations = {
    en: {
        common: {
            preparing: 'Preparing…',
            actions: { exportCsv: 'Export' },
            exportFormats: {
                csv: 'CSV',
                xlsx: 'Excel',
                parquet: 'Parquet',
                json: 'JSON',
                ndjson: 'NDJSON'
            }
        },
        aiAgents: {
            fetchDepth: {
                exportStatus: {
                    success: 'Export ready.',
                    error: 'Export failed.'
                }
            }
        }
    }
};

describe('ExportSplitButton', () => {
    let fixture: ComponentFixture<ExportSplitButton>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                ExportSplitButton,
                TranslocoTestingModule.forRoot({
                    langs: translations,
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(ExportSplitButton);
    });

    const menuItemsOf = (instance: ExportSplitButton): MenuItem[] => (instance as unknown as { menuItems: () => MenuItem[] }).menuItems();

    it('renders the export label and emits the default format on the primary action', () => {
        const requested: TakeoutExportFormat[] = [];
        fixture.componentInstance.export.subscribe((format) => requested.push(format));
        fixture.detectChanges();

        const primaryButton = fixture.nativeElement.querySelector('button') as HTMLButtonElement;
        expect(primaryButton.textContent).toContain('Export');
        expect(primaryButton.querySelector('.pi-download')).not.toBeNull();

        primaryButton.click();
        expect(requested).toEqual(['csv']);
    });

    it('swaps in the preparing label while an export is running', () => {
        fixture.componentRef.setInput('isExporting', true);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Preparing…');
        expect(fixture.nativeElement.textContent).not.toContain('Export ready.');
    });

    it('disables both actions when disabled is set', () => {
        fixture.componentRef.setInput('disabled', true);
        fixture.detectChanges();

        const buttons = [...fixture.nativeElement.querySelectorAll('button')] as HTMLButtonElement[];
        expect(buttons.length).toBeGreaterThan(0);
        expect(buttons.every((button) => button.disabled)).toBe(true);
    });

    it('offers every takeout format and emits the chosen one', () => {
        const requested: TakeoutExportFormat[] = [];
        fixture.componentInstance.export.subscribe((format) => requested.push(format));
        fixture.detectChanges();

        const items = menuItemsOf(fixture.componentInstance);
        expect(items.map((item) => item.label)).toEqual(['CSV', 'Excel', 'Parquet', 'JSON', 'NDJSON']);

        for (const item of items) {
            item.command?.({} as never);
        }
        expect(requested).toEqual([...TAKEOUT_EXPORT_FORMATS]);
    });
});

describe('ExportStatusBanner', () => {
    let fixture: ComponentFixture<ExportStatusBanner>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                ExportStatusBanner,
                TranslocoTestingModule.forRoot({
                    langs: translations,
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(ExportStatusBanner);
        fixture.componentRef.setInput('successKey', 'aiAgents.fetchDepth.exportStatus.success');
        fixture.componentRef.setInput('errorKey', 'aiAgents.fetchDepth.exportStatus.error');
    });

    it('stays silent while idle', () => {
        fixture.componentRef.setInput('state', 'idle');
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('div')).toBeNull();
    });

    it('renders the success banner', () => {
        fixture.componentRef.setInput('state', 'success');
        fixture.detectChanges();

        const banner = fixture.nativeElement.querySelector('div') as HTMLElement;
        expect(banner.textContent).toContain('Export ready.');
        expect(banner.className).toBe('mb-6 rounded-md border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700');
    });

    it('renders the error banner', () => {
        fixture.componentRef.setInput('state', 'error');
        fixture.detectChanges();

        const banner = fixture.nativeElement.querySelector('div') as HTMLElement;
        expect(banner.textContent).toContain('Export failed.');
        expect(banner.className).toBe('mb-6 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700');
    });
});
