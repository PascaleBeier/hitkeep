import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';

import { AuthMethodOption, AuthMethods } from './auth-methods';

describe('AuthMethods', () => {
    let fixture: ComponentFixture<AuthMethods>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                AuthMethods,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            login: {
                                authenticationMethods: 'Authentication methods',
                                continueWithPassword: 'Continue with password',
                                continueWithSSO: 'Continue with SSO'
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(AuthMethods);
    });

    it('renders configured authentication methods and emits the selected method', async () => {
        const selected = vi.fn();
        const methods: readonly AuthMethodOption[] = [
            { id: 'password', labelKey: 'login.continueWithPassword', icon: 'pi pi-lock' },
            { id: 'sso', labelKey: 'login.continueWithSSO', icon: 'pi pi-building', wide: true }
        ];
        fixture.componentRef.setInput('methods', methods);
        fixture.componentInstance.methodSelected.subscribe(selected);

        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        const buttons = element.querySelectorAll<HTMLButtonElement>('button');
        expect(buttons.length).toBe(2);
        expect(buttons[0].textContent).toContain('Continue with password');
        expect(buttons[1].textContent).toContain('Continue with SSO');

        buttons[1].click();
        await fixture.whenStable();

        expect(selected).toHaveBeenCalledWith('sso');
    });

    it('keeps loading methods disabled and exposes the group label', async () => {
        fixture.componentRef.setInput('methods', [{ id: 'sso', labelKey: 'login.continueWithSSO', icon: 'pi pi-building', loading: true }]);

        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        const group = element.querySelector<HTMLElement>('[role="group"]');
        const button = element.querySelector<HTMLButtonElement>('button');
        expect(group?.getAttribute('aria-label')).toBe('Authentication methods');
        expect(button?.disabled).toBe(true);
        expect(element.querySelector('p-button')?.getAttribute('aria-busy')).toBe('true');
    });
});
