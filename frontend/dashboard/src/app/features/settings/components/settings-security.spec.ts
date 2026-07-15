import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { SettingsSecurity } from './settings-security';
import { AuthService } from '@services/auth.service';
import { UserSecurityService } from '@services/user-security.service';

describe('SettingsSecurity', () => {
    let fixture: ComponentFixture<SettingsSecurity>;
    let component: SettingsSecurity;
    let originalClipboard: Clipboard | undefined;
    let createObjectURLSpy: ReturnType<typeof vi.fn>;
    let revokeObjectURLSpy: ReturnType<typeof vi.fn>;
    let clickSpy: ReturnType<typeof vi.fn>;

    const authServiceMock = {
        // eslint-disable-next-line @typescript-eslint/no-unused-vars
        changePassword: vi.fn((current: string, newPass: string) => of(void 0)),
        getSocialProviders: vi.fn(() => of({ providers: [], signup_enabled: false })),
        requestPasswordReset: vi.fn(() => of(void 0))
    };

    const userSecurityServiceMock = {
        loadStatus: vi.fn(() =>
            of({
                totp_enabled: false,
                totp_pending: false,
                passkeys: [{ id: 'pk-1', name: 'Laptop', created_at: '2026-03-10T00:00:00Z', updated_at: '2026-03-10T00:00:00Z' }],
                recovery_codes_generated: true,
                recovery_codes_remaining: 4,
                password_login_enabled: true,
                social_identities: []
            })
        ),
        startTotpSetup: vi.fn(() => of({ secret: '', otpauth_url: '', expires_at: '2026-03-10T00:00:00Z' })),
        verifyTotpSetup: vi.fn(() => of(null)),
        disableTotp: vi.fn(() => of(null)),
        startPasskeyRegistration: vi.fn(() => of({ publicKey: null })),
        finishPasskeyRegistration: vi.fn(() => of(null)),
        deletePasskey: vi.fn(() => of(void 0)),
        regenerateRecoveryCodes: vi.fn(() =>
            of({
                codes: ['ABCD-EFGH', 'JKLM-NPQR'],
                remaining: 2
            })
        ),
        startSocialLink: vi.fn<UserSecurityService['startSocialLink']>(() => of({ auth_url: 'https://identity.example.com/authorize' })),
        unlinkSocial: vi.fn<UserSecurityService['unlinkSocial']>(() => of(void 0))
    };

    beforeEach(async () => {
        vi.clearAllMocks();
        originalClipboard = navigator.clipboard;
        createObjectURLSpy = vi.fn(() => 'blob:recovery-codes');
        revokeObjectURLSpy = vi.fn();
        clickSpy = vi.fn();

        Object.defineProperty(navigator, 'clipboard', {
            configurable: true,
            value: {
                writeText: vi.fn(() => Promise.resolve())
            }
        });
        Object.defineProperty(window.URL, 'createObjectURL', {
            configurable: true,
            value: createObjectURLSpy
        });
        Object.defineProperty(window.URL, 'revokeObjectURL', {
            configurable: true,
            value: revokeObjectURLSpy
        });
        vi.spyOn(document, 'createElement').mockImplementation(((tagName: string) => {
            const element = document.createElementNS('http://www.w3.org/1999/xhtml', tagName) as HTMLAnchorElement;
            if (tagName.toLowerCase() === 'a') {
                Object.defineProperty(element, 'click', {
                    configurable: true,
                    value: clickSpy
                });
            }
            return element;
        }) as typeof document.createElement);

        await TestBed.configureTestingModule({
            imports: [
                SettingsSecurity,
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
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: {
                        en: 'en-US',
                        'en-US': 'en-US'
                    }
                }),
                { provide: AuthService, useValue: authServiceMock },
                { provide: UserSecurityService, useValue: userSecurityServiceMock }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(SettingsSecurity);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    afterEach(() => {
        vi.restoreAllMocks();
        Object.defineProperty(navigator, 'clipboard', {
            configurable: true,
            value: originalClipboard
        });
    });

    it('loads the current recovery code status', () => {
        expect(component['recoveryCodesGenerated']()).toBe(true);
        expect(component['recoveryCodesRemaining']()).toBe(4);
        expect(component['hasMfaProtection']()).toBe(true);
    });

    it('removes a linked provider summary after a guarded unlink succeeds', () => {
        component['securityStatus'].update((status) =>
            status
                ? {
                      ...status,
                      social_identities: [{ provider: 'github', observed_email: 'user@example.com', linked_at: '2026-03-10T00:00:00Z' }]
                  }
                : status
        );
        component['form'].currentPassword().control().setValue('password123');

        component['unlinkSocial']('github');

        expect(userSecurityServiceMock.unlinkSocial).toHaveBeenCalledWith('github', 'password123');
        expect(component['socialIdentities']()).toEqual([]);
        expect(component['socialSuccess']()).toBe('settings.security.social.unlinked');
    });

    it('regenerates recovery codes and updates the visible state', () => {
        component['regenerateRecoveryCodes']();

        expect(userSecurityServiceMock.regenerateRecoveryCodes).toHaveBeenCalled();
        expect(component['recoveryCodes']()).toEqual(['ABCD-EFGH', 'JKLM-NPQR']);
        expect(component['recoveryCodesRemaining']()).toBe(2);
    });

    it('copies the visible recovery codes', async () => {
        component['regenerateRecoveryCodes']();

        await component['copyRecoveryCodes']();

        expect(navigator.clipboard.writeText).toHaveBeenCalledWith('ABCD-EFGH\nJKLM-NPQR');
        expect(component['recoveryCodeSuccess']()).toBe('settings.security.recoveryCodes.copied');
    });

    it('downloads the visible recovery codes', () => {
        component['regenerateRecoveryCodes']();

        component['downloadRecoveryCodes']();

        expect(createObjectURLSpy).toHaveBeenCalledTimes(1);
        expect(clickSpy).toHaveBeenCalledTimes(1);
        expect(revokeObjectURLSpy).toHaveBeenCalledWith('blob:recovery-codes');
        expect(component['recoveryCodeSuccess']()).toBe('settings.security.recoveryCodes.downloaded');
    });

    it('does not submit if password form is invalid', () => {
        component['form'].currentPassword().control().setValue('');
        component['form'].newPassword().control().setValue('');
        component['onSubmit']();
        expect(authServiceMock.changePassword).not.toHaveBeenCalled();
    });

    it('submits if password form is valid even if TOTP form is invalid', () => {
        component['form'].currentPassword().control().setValue('old-password');
        component['form'].newPassword().control().setValue('new-password-123');
        // TOTP code is empty and thus invalid, but we only care about password form
        component['onSubmit']();
        expect(authServiceMock.changePassword).toHaveBeenCalledWith('old-password', 'new-password-123');
    });
});
