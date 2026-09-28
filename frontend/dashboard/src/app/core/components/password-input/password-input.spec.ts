import { ComponentFixture, TestBed } from '@angular/core/testing';
import { FormControl } from '@angular/forms';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { PasswordInput } from './password-input';

describe('PasswordInput', () => {
    let fixture: ComponentFixture<PasswordInput>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                PasswordInput,
                TranslocoTestingModule.forRoot({
                    langs: { en: { common: { showPassword: 'Show password', hidePassword: 'Hide password' } } },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(PasswordInput);
    });

    it('binds the real input and exposes an accessible visibility button', async () => {
        const control = new FormControl('', { nonNullable: true });
        fixture.componentRef.setInput('control', control);
        fixture.componentRef.setInput('inputId', 'password');
        fixture.componentRef.setInput('autocomplete', 'new-password');
        fixture.componentRef.setInput('invalid', true);
        fixture.componentRef.setInput('describedBy', 'password-error');
        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        const input = element.querySelector<HTMLInputElement>('input')!;
        const button = element.querySelector<HTMLButtonElement>('button')!;
        expect(input.id).toBe('password');
        expect(input.autocomplete).toBe('new-password');
        expect(input.getAttribute('aria-invalid')).toBe('true');
        expect(input.getAttribute('aria-describedby')).toBe('password-error');
        expect(input.type).toBe('password');
        expect(button.type).toBe('button');
        expect(button.getAttribute('aria-label')).toBe('Show password');

        input.value = 'secret123';
        input.dispatchEvent(new Event('input', { bubbles: true }));
        expect(control.value).toBe('secret123');

        button.click();
        await fixture.whenStable();
        expect(input.type).toBe('text');
        expect(button.getAttribute('aria-label')).toBe('Hide password');
        expect(button.getAttribute('aria-pressed')).toBe('true');
    });
});
