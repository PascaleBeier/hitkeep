import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';

import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';
import { Site } from '@models/analytics.types';
import { SiteDangerZone } from './site-danger-zone';
import { SiteService } from '@features/sites/services/site.service';

describe('SiteDangerZone', () => {
    let fixture: ComponentFixture<SiteDangerZone>;

    const site: Site = {
        id: 'site-1',
        user_id: 'user-1',
        domain: 'example.com',
        created_at: '2026-05-01T00:00:00Z'
    };

    const accessService = {
        canSite: vi.fn(() => true)
    };

    const siteService = {
        deleteSite: vi.fn(() => of(undefined)),
        resetSiteStats: vi.fn(() => of({ status: 'reset' as const, rows_cleared: 12, imports_marked_deleted: 1, families_cleared: ['native'] })),
        loadSites: vi.fn()
    };

    beforeEach(async () => {
        vi.clearAllMocks();
        accessService.canSite.mockReturnValue(true);
        siteService.resetSiteStats.mockReturnValue(of({ status: 'reset' as const, rows_cleared: 12, imports_marked_deleted: 1, families_cleared: ['native'] }));

        await TestBed.configureTestingModule({
            imports: [
                SiteDangerZone,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            sites: {
                                danger: {
                                    resetTitle: 'Reset stats',
                                    resetDescription: 'Clear stats.',
                                    resetConfirmLabel: 'Type {{domain}} to reset stats',
                                    resetConfirmHint: 'Tracking stays active.',
                                    resetAction: 'Reset stats',
                                    resetSuccess: 'Cleared {{rows}} rows and {{imports}} imports.',
                                    resetFailed: 'Reset failed.',
                                    deleteTitle: 'Delete site',
                                    deleteDescription: 'Delete everything.',
                                    confirmLabel: 'Type {{domain}} to confirm',
                                    confirmPlaceholder: 'example.com',
                                    deleteAction: 'Delete site',
                                    deleteFailed: 'Delete failed.'
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
            providers: [
                { provide: AccessService, useValue: accessService },
                { provide: SiteService, useValue: siteService }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(SiteDangerZone);
        fixture.componentRef.setInput('site', site);
        fixture.detectChanges();
    });

    it('hides the danger zone without delete capability', () => {
        accessService.canSite.mockReturnValue(false);
        fixture = TestBed.createComponent(SiteDangerZone);
        fixture.componentRef.setInput('site', site);
        fixture.detectChanges();

        const canSiteCalls = accessService.canSite.mock.calls as unknown as unknown[][];
        expect(canSiteCalls.some((call) => call[0] === 'site-1' && call[1] === SITE_CAPABILITIES.delete)).toBe(true);
        expect(fixture.nativeElement.textContent).not.toContain('Reset stats');
        expect(fixture.nativeElement.textContent).not.toContain('Delete site');
    });

    it('keeps reset disabled until the domain matches', () => {
        const input = fixture.nativeElement.querySelector('#reset-site-confirm') as HTMLInputElement;
        const resetButton = findButton('Reset stats');

        expect(resetButton.disabled).toBe(true);

        input.value = 'wrong.example.com';
        input.dispatchEvent(new Event('input'));
        fixture.detectChanges();
        expect(resetButton.disabled).toBe(true);

        input.value = 'example.com';
        input.dispatchEvent(new Event('input'));
        fixture.detectChanges();
        expect(findButton('Reset stats').disabled).toBe(false);
    });

    it('resets stats and renders success feedback inline', () => {
        fixture.componentInstance['resetConfirmValue'].set('example.com');

        fixture.componentInstance['resetStats']();
        fixture.detectChanges();

        const resetCalls = siteService.resetSiteStats.mock.calls as unknown as unknown[][];
        expect(resetCalls[0]).toEqual(['site-1', 'example.com']);
        expect(siteService.loadSites).toHaveBeenCalled();
        expect(fixture.nativeElement.textContent).toContain('Cleared 12 rows and 1 imports.');
    });

    it('renders reset errors inline', () => {
        siteService.resetSiteStats.mockReturnValueOnce(throwError(() => new Error('boom')));
        fixture.componentInstance['resetConfirmValue'].set('example.com');

        fixture.componentInstance['resetStats']();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Reset failed.');
    });

    it('renders delete errors inline', () => {
        const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => undefined);
        siteService.deleteSite.mockReturnValueOnce(throwError(() => new Error('boom')));
        fixture.componentInstance['confirmValue'].set('example.com');

        fixture.componentInstance['deleteSite']();
        fixture.detectChanges();

        expect(alertSpy).not.toHaveBeenCalled();
        expect(fixture.nativeElement.textContent).toContain('Delete failed.');

        alertSpy.mockRestore();
    });

    function findButton(label: string): HTMLButtonElement {
        const buttons = Array.from(fixture.nativeElement.querySelectorAll('button')) as HTMLButtonElement[];
        const button = buttons.find((candidate) => candidate.textContent?.includes(label));
        if (!button) {
            throw new Error(`Button ${label} not found`);
        }
        return button;
    }
});
