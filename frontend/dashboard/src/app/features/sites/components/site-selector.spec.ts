import { ComponentFixture, TestBed } from '@angular/core/testing';
import { SiteSelector } from '@features/sites/components/site-selector';
import { By } from '@angular/platform-browser';
import { provideHttpClient } from '@angular/common/http';
import { provideRouter } from '@angular/router';
import { userEvent } from 'vitest/browser';
import { TranslocoTestingModule } from '@jsverse/transloco';

describe('SiteSelector', () => {
    let component: SiteSelector;
    let fixture: ComponentFixture<SiteSelector>;

    beforeEach(async () => {
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
            providers: [provideHttpClient(), provideRouter([])]
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

    it('keeps contextual Add site available when the team has no sites and closes the selector', async () => {
        let opened = 0;
        fixture.componentRef.setInput('sites', []);
        fixture.componentRef.setInput('current', null);
        fixture.componentRef.setInput('actions', [{ label: 'Add site', icon: 'pi pi-plus', command: () => opened++ }]);
        await fixture.whenStable();
        const select = fixture.nativeElement.querySelector('p-select') as HTMLElement;
        expect(select).toBeTruthy();
        select.click();
        await fixture.whenStable();
        const action = fixture.nativeElement.querySelector('button[aria-label="Add site"]') as HTMLButtonElement;
        expect(action).toBeTruthy();
        await userEvent.click(action);
        await fixture.whenStable();
        expect(opened).toBe(1);
        await expect.poll(() => fixture.nativeElement.querySelector('.p-select-overlay')).toBeNull();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('A11Y: should expose an accessible label on the dropdown', () => {
        const select = fixture.debugElement.query(By.css('p-select'));
        expect(select.attributes['inputId']).toBe('site-dropdown');
        expect(select.attributes['aria-label']).toBeTruthy();
    });

    it('keeps long selected domains and options constrained to the selector width', async () => {
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
});
