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
                            },
                            social: {
                                continueWith: {
                                    google: 'Continue with Google',
                                    github: 'Continue with GitHub',
                                    microsoft: 'Continue with Microsoft'
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

    it('renders official provider marks without monochrome PrimeIcons', async () => {
        fixture.componentRef.setInput('methods', [
            { id: 'google', labelKey: 'social.continueWith.google', providerIcon: 'google' },
            { id: 'github', labelKey: 'social.continueWith.github', providerIcon: 'github' },
            { id: 'microsoft', labelKey: 'social.continueWith.microsoft', providerIcon: 'microsoft' }
        ]);

        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        const providerIcons = Array.from(element.querySelectorAll<HTMLElement>('app-social-provider-icon'));
        expect(providerIcons.map((icon) => icon.dataset['providerIcon'])).toEqual(['google', 'github', 'microsoft']);
        expect(providerIcons.every((icon) => icon.getAttribute('aria-hidden') === 'true')).toBe(true);
        expect(element.querySelector('.pi-google, .pi-github, .pi-microsoft')).toBeNull();
        expect(element.querySelector('.hk-social-provider-mark--google')).toBeTruthy();
        expect(element.querySelector('.hk-social-provider-mark--github-light')).toBeTruthy();
        expect(element.querySelector('.hk-social-provider-mark--github-dark')).toBeTruthy();
        expect(element.querySelector('.hk-social-provider-mark--microsoft')).toBeTruthy();
    });

    it('replaces a loading provider mark with the PrimeNG spinner and exposes the group label', async () => {
        fixture.componentRef.setInput('methods', [{ id: 'google', labelKey: 'social.continueWith.google', providerIcon: 'google', loading: true }]);

        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        const group = element.querySelector<HTMLElement>('[role="group"]');
        const button = element.querySelector<HTMLButtonElement>('button');
        expect(group?.getAttribute('aria-label')).toBe('Authentication methods');
        expect(button?.disabled).toBe(true);
        expect(element.querySelector('p-button')?.getAttribute('aria-busy')).toBe('true');
        expect(element.querySelector('app-social-provider-icon')).toBeNull();
        expect(element.querySelector('svg[data-p-icon="spinner"]')).toBeTruthy();
    });
});
