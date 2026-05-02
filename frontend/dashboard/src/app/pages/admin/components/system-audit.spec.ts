import { TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';

import { AdminSystemService } from '@services/admin-system.service';
import { AuditPresentationService } from '@services/audit-presentation.service';
import { SystemAudit } from './system-audit';

interface SystemAuditTestAccess {
    query: { set(value: { target_type?: string; limit: number; offset: number }): void };
    exportAudit(): void;
    exportStatus(): { severity: 'success' | 'error'; key: string } | null;
    isExporting(): boolean;
}

describe('SystemAudit', () => {
    let component: SystemAuditTestAccess;
    let exportAuditMock: ReturnType<typeof vi.fn>;
    let createObjectURLMock: ReturnType<typeof vi.fn>;
    let revokeObjectURLMock: ReturnType<typeof vi.fn>;
    let clickSpy: ReturnType<typeof vi.spyOn>;

    beforeEach(() => {
        exportAuditMock = vi.fn();
        createObjectURLMock = vi.fn(() => 'blob:instance-audit');
        revokeObjectURLMock = vi.fn();
        clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined);

        Object.defineProperty(window.URL, 'createObjectURL', {
            configurable: true,
            value: createObjectURLMock
        });
        Object.defineProperty(window.URL, 'revokeObjectURL', {
            configurable: true,
            value: revokeObjectURLMock
        });

        TestBed.configureTestingModule({
            imports: [
                TranslocoTestingModule.forRoot({
                    langs: { en: {} },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                {
                    provide: AdminSystemService,
                    useValue: {
                        listAudit: vi.fn(),
                        exportAudit: exportAuditMock
                    }
                },
                {
                    provide: AuditPresentationService,
                    useValue: {
                        actionOptions: vi.fn(() => []),
                        outcomeOptions: vi.fn(() => []),
                        targetTypeOptions: vi.fn(() => [])
                    }
                }
            ]
        });

        component = TestBed.runInInjectionContext(() => new SystemAudit()) as unknown as SystemAuditTestAccess;
    });

    afterEach(() => {
        clickSpy.mockRestore();
    });

    it('exports the active audit filters and reports success in place', () => {
        exportAuditMock.mockReturnValue(of(new Blob(['[]'], { type: 'application/json' })));
        component.query.set({ target_type: 'mail', limit: 25, offset: 0 });

        component.exportAudit();

        const filter = exportAuditMock.mock.calls[0]?.[0] as Record<string, unknown>;
        expect(filter['target_type']).toBe('mail');
        expect(filter['limit']).toBe(25);
        expect(filter['offset']).toBe(0);
        expect(createObjectURLMock).toHaveBeenCalled();
        expect(revokeObjectURLMock).toHaveBeenCalledWith('blob:instance-audit');
        expect(component.isExporting()).toBe(false);
        expect(component.exportStatus()).toEqual({
            severity: 'success',
            key: 'admin.system.audit.exportSuccess'
        });
    });

    it('reports audit export failures in place', () => {
        exportAuditMock.mockReturnValue(throwError(() => new Error('export failed')));

        component.exportAudit();

        expect(component.isExporting()).toBe(false);
        expect(component.exportStatus()).toEqual({
            severity: 'error',
            key: 'admin.system.audit.exportFailed'
        });
    });
});
