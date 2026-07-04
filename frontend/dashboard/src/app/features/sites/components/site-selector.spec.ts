import { ComponentFixture, TestBed } from '@angular/core/testing';
import { WritableSignal, signal } from '@angular/core';
import { SiteSelector } from '@features/sites/components/site-selector';
import { By } from '@angular/platform-browser';
import { provideHttpClient } from '@angular/common/http';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';

describe('SiteSelector', () => {
    let component: SiteSelector;
    let fixture: ComponentFixture<SiteSelector>;
    let canSiteMock: ReturnType<typeof vi.fn>;
    let allowedSiteCapabilities: WritableSignal<string[] | null>;

    beforeEach(async () => {
        allowedSiteCapabilities = signal<string[] | null>(null);
        canSiteMock = vi.fn((_siteId: string, capability: string) => allowedSiteCapabilities()?.includes(capability) ?? true);

        await TestBed.configureTestingModule({
            imports: [
                SiteSelector,
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
                provideHttpClient(),
                provideRouter([]),
                {
                    provide: AccessService,
                    useValue: {
                        canSite: canSiteMock
                    }
                }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(SiteSelector);
        component = fixture.componentInstance;
        fixture.componentRef.setInput('sites', [{ id: '1', domain: 'test.com' }]);
        fixture.componentRef.setInput('current', { id: '1', domain: 'test.com' });
        fixture.detectChanges();
    });

    afterEach(() => {
        TestBed.resetTestingModule();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('A11Y: should have a label associated with the dropdown', () => {
        const label = fixture.debugElement.query(By.css('label'));
        const select = fixture.debugElement.query(By.css('p-select'));
        expect(label.nativeElement.getAttribute('for')).toBe('site-dropdown');
        expect(select.attributes['inputId']).toBe('site-dropdown');
    });

    it('keeps long selected domains and options constrained to the sidebar width', async () => {
        const longDomain = 'a-very-long-customer-subdomain-with-campaign-context-and-region.example-analytics.test';
        const host = fixture.nativeElement as HTMLElement;
        host.style.width = '16rem';
        host.style.maxWidth = '16rem';
        host.style.minWidth = '0';

        fixture.componentRef.setInput('sites', [{ id: '1', user_id: 'user-1', domain: longDomain, created_at: '2026-01-01T00:00:00Z' }]);
        fixture.componentRef.setInput('current', { id: '1', user_id: 'user-1', domain: longDomain, created_at: '2026-01-01T00:00:00Z' });
        fixture.detectChanges();
        await fixture.whenStable();

        const select = fixture.debugElement.query(By.css('p-select.site-selector__select'));
        const selectBox = select?.nativeElement as HTMLElement | undefined;

        expect(select).toBeTruthy();
        expect(selectBox?.classList.contains('w-full')).toBe(true);
        expect(selectBox).toBeTruthy();
        expect(selectBox!.getBoundingClientRect().width).toBeLessThanOrEqual(host.getBoundingClientRect().width);

        selectBox!.click();
        fixture.detectChanges();
        await fixture.whenStable();
        await new Promise((resolve) => requestAnimationFrame(resolve));

        const panel = selectBox!.querySelector('.p-select-overlay') as HTMLElement | null;
        const selectedOption = selectBox!.querySelector('.p-select-label app-site-select-option') as HTMLElement | null;
        const selectedDomain = selectBox!.querySelector('.p-select-label .site-select-option__domain') as HTMLElement | null;
        const optionDomain = panel?.querySelector('.site-select-option__domain') as HTMLElement | null;

        expect(panel).toBeTruthy();
        expect(selectedOption).toBeTruthy();
        expect(selectedDomain).toBeTruthy();
        expect(optionDomain).toBeTruthy();
        expect(getComputedStyle(panel!).maxWidth).toBe('100%');
        expect(getComputedStyle(selectedOption!).display).toBe('block');
        expect(panel!.getBoundingClientRect().width).toBeLessThanOrEqual(selectBox!.getBoundingClientRect().width);
        expect(selectedDomain!.getBoundingClientRect().right).toBeLessThanOrEqual(selectBox!.getBoundingClientRect().right);
        expect(optionDomain!.getBoundingClientRect().right).toBeLessThanOrEqual(panel!.getBoundingClientRect().right);
    });

    it('A11Y: Add Site button should have aria-label', () => {
        const btn = fixture.debugElement.query(By.css('button[aria-label]'));
        expect(btn).toBeTruthy();
    });

    it('disables dashboard sharing when the active site cannot manage team access', () => {
        allowedSiteCapabilities.set([SITE_CAPABILITIES.view]);
        fixture.detectChanges();

        const shareButton = fixture.debugElement.query(By.css('button[title="sites.selector.shareDashboardAria"]'));

        expect(shareButton.nativeElement.disabled).toBe(true);
    });
});
