import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';

import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';
import { Site } from '@models/analytics.types';
import { SiteDangerZone } from './site-danger-zone';
import { SiteService } from '@features/sites/services/site.service';
import { NavigationNoticeService } from '@services/navigation-notice.service';

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
        deleteSite: vi.fn((siteID: string) => {
            void siteID;
            return of(undefined);
        }),
        resetSiteStats: vi.fn((siteID: string, domain: string) => {
            void siteID;
            void domain;
            return of({ status: 'reset' as const, rows_cleared: 12, imports_marked_deleted: 1, families_cleared: ['native'] });
        }),
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
                                    resetConfirmTitle: 'Reset stats',
                                    resetConfirmMessage: 'Type {{domain}} to reset stats for this site.',
                                    resetConfirmHint: 'Tracking stays active.',
                                    resetAction: 'Reset stats',
                                    resetSuccess: 'Cleared {{rows}} rows and {{imports}} imports.',
                                    resetFailed: 'Reset failed.',
                                    deleteTitle: 'Delete site',
                                    deleteDescription: 'Delete everything.',
                                    deleteConfirmTitle: 'Delete site',
                                    deleteConfirmMessage: 'Type {{domain}} to delete this site.',
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
            providers: [provideRouter([]), { provide: AccessService, useValue: accessService }, { provide: SiteService, useValue: siteService }]
        }).compileComponents();

        fixture = TestBed.createComponent(SiteDangerZone);
        fixture.componentRef.setInput('site', site);
        fixture.detectChanges();
    });

    afterEach(() => {
        document.querySelectorAll('.p-dialog-mask, .p-dialog').forEach((element) => element.remove());
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

    it('renders danger actions without inline confirmation inputs', () => {
        expect(fixture.nativeElement.textContent).toContain('Reset stats');
        expect(fixture.nativeElement.textContent).toContain('Delete site');
        expect(fixture.nativeElement.querySelector('#site-danger-confirm')).toBeNull();
    });

    it('requires the site domain in the confirmation dialog before resetting stats', async () => {
        findButton('Reset stats').click();
        fixture.detectChanges();
        await fixture.whenStable();

        const input = document.body.querySelector('#site-danger-confirm') as HTMLInputElement | null;
        expect(input).toBeTruthy();
        expect(dialogPrimaryButton('Reset stats').disabled).toBe(true);

        input!.value = 'wrong.example.com';
        input!.dispatchEvent(new Event('input'));
        fixture.detectChanges();
        expect(dialogPrimaryButton('Reset stats').disabled).toBe(true);

        input!.value = 'example.com';
        input!.dispatchEvent(new Event('input'));
        fixture.detectChanges();
        expect(dialogPrimaryButton('Reset stats').disabled).toBe(false);

        dialogPrimaryButton('Reset stats').click();
        fixture.detectChanges();

        const resetCalls = siteService.resetSiteStats.mock.calls as unknown as unknown[][];
        expect(resetCalls[0]).toEqual(['site-1', 'example.com']);
        expect(siteService.loadSites).toHaveBeenCalled();
        expect(fixture.nativeElement.textContent).toContain('Cleared 12 rows and 1 imports.');
    });

    it('requires the site domain in the confirmation dialog before deleting the site', async () => {
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
        findButton('Delete site').click();
        fixture.detectChanges();
        await fixture.whenStable();

        const input = document.body.querySelector('#site-danger-confirm') as HTMLInputElement | null;
        expect(input).toBeTruthy();
        expect(document.body.textContent).toContain('Type example.com to delete this site.');
        expect(document.body.textContent).not.toContain('Type example.com to confirm');
        expect(dialogPrimaryButton('Delete site').disabled).toBe(true);

        input!.value = 'example.com';
        input!.dispatchEvent(new Event('input'));
        fixture.detectChanges();
        expect(dialogPrimaryButton('Delete site').disabled).toBe(false);

        dialogPrimaryButton('Delete site').click();
        await fixture.whenStable();

        expect(siteService.deleteSite).toHaveBeenCalledWith('site-1');
        expect(navigate).toHaveBeenCalledWith(['/overview']);
        expect(TestBed.inject(NavigationNoticeService).key()).toBe('sites.settings.notices.siteDeleted');
    });

    it('renders reset errors inline', () => {
        siteService.resetSiteStats.mockReturnValueOnce(throwError(() => new Error('boom')));
        const component = fixture.componentInstance as unknown as { pendingAction: { set(value: string): void }; confirmValue: { set(value: string): void }; resetStats(): void };
        component.pendingAction.set('reset');
        component.confirmValue.set('example.com');

        component.resetStats();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Reset failed.');
    });

    it('renders delete errors inline', () => {
        const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => undefined);
        siteService.deleteSite.mockReturnValueOnce(throwError(() => new Error('boom')));
        const component = fixture.componentInstance as unknown as { pendingAction: { set(value: string): void }; confirmValue: { set(value: string): void }; deleteSite(): void };
        component.pendingAction.set('delete');
        component.confirmValue.set('example.com');

        component.deleteSite();
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

    function dialogPrimaryButton(label: string): HTMLButtonElement {
        const buttons = Array.from(document.body.querySelectorAll('.dialog-shell-footer button')) as HTMLButtonElement[];
        const button = buttons.find((candidate) => candidate.textContent?.includes(label));
        if (!button) {
            throw new Error(`Dialog button ${label} not found`);
        }
        return button;
    }
});
