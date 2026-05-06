import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { ImportExportPage } from './import-export';
import { vi } from 'vitest';

interface ImportExportPageAccess {
    activeTab(): string;
    onTabChange(value: string | number | undefined): void;
}

describe('ImportExportPage', () => {
    let fixture: ComponentFixture<ImportExportPage>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                ImportExportPage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            importExport: {
                                title: 'Import & Export',
                                tabs: {
                                    import: 'Import',
                                    export: 'Export'
                                }
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [provideRouter([])]
        }).compileComponents();

        fixture = TestBed.createComponent(ImportExportPage);
        fixture.detectChanges();
    });

    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('should render Import and Export tabs', () => {
        const text = fixture.nativeElement.textContent;

        expect(text).toContain('Import');
        expect(text).toContain('Export');
    });

    it('should select the Export tab from the export route URL', () => {
        const router = TestBed.inject(Router);
        vi.spyOn(router, 'url', 'get').mockReturnValue('/import-export/export');
        const exportFixture = TestBed.createComponent(ImportExportPage);
        exportFixture.detectChanges();

        expect((exportFixture.componentInstance as unknown as ImportExportPageAccess).activeTab()).toBe('export');
    });

    it('should navigate when a tab is selected', () => {
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);

        (fixture.componentInstance as unknown as ImportExportPageAccess).onTabChange('export');

        expect(navigateSpy).toHaveBeenCalledWith(['/import-export', 'export']);
    });
});
