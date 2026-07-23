import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';
import { PageState } from './page-state';

describe('PageState', () => {
    let fixture: ComponentFixture<PageState>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                PageState,
                TranslocoTestingModule.forRoot({
                    langs: { en: { state: { title: 'Nothing here', message: 'Choose a site first.', action: 'Try again' } } },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ]
        }).compileComponents();
    });

    it('renders translated title, message, icon, and accessible label wiring', () => {
        fixture = TestBed.createComponent(PageState);
        fixture.componentRef.setInput('titleKey', 'state.title');
        fixture.componentRef.setInput('messageKey', 'state.message');
        fixture.componentRef.setInput('icon', 'pi pi-lock');
        fixture.componentRef.setInput('titleId', 'custom-state-title');
        fixture.detectChanges();

        const root = fixture.nativeElement.querySelector('.page-state') as HTMLElement;
        expect(root.getAttribute('aria-labelledby')).toBe('custom-state-title');
        expect(fixture.nativeElement.querySelector('h2')?.id).toBe('custom-state-title');
        expect(fixture.nativeElement.querySelector('h2')?.textContent).toContain('Nothing here');
        expect(fixture.nativeElement.querySelector('p')?.textContent).toContain('Choose a site first.');
        expect(fixture.nativeElement.querySelector('i')?.className).toContain('pi-lock');
    });

    it('generates a stable default title id and icon', () => {
        fixture = TestBed.createComponent(PageState);
        fixture.componentRef.setInput('titleKey', 'state.title');
        fixture.componentRef.setInput('messageKey', 'state.message');
        fixture.detectChanges();

        const root = fixture.nativeElement.querySelector('.page-state') as HTMLElement;
        const heading = fixture.nativeElement.querySelector('h2') as HTMLElement;
        expect(root.getAttribute('aria-labelledby')).toBe(heading.id);
        expect(heading.id).toContain('page-state-');
        expect(fixture.nativeElement.querySelector('i')?.className).toContain('pi-info-circle');
        expect(fixture.nativeElement.querySelector('button')).toBeNull();
    });

    it('renders and emits the optional action', () => {
        fixture = TestBed.createComponent(PageState);
        fixture.componentRef.setInput('titleKey', 'state.title');
        fixture.componentRef.setInput('messageKey', 'state.message');
        fixture.componentRef.setInput('actionLabelKey', 'state.action');
        fixture.componentRef.setInput('actionIcon', 'pi pi-refresh');
        const clicked = vi.fn();
        fixture.componentInstance.actionClicked.subscribe(clicked);
        fixture.detectChanges();

        const button = fixture.nativeElement.querySelector('button') as HTMLButtonElement;
        expect(button.textContent).toContain('Try again');
        expect(button.querySelector('.pi-refresh')).not.toBeNull();
        button.click();
        expect(clicked).toHaveBeenCalledTimes(1);
    });
});
