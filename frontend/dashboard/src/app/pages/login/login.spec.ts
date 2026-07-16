import { Component } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of, Subject } from 'rxjs';
import { vi } from 'vitest';

import { Login } from '@pages/login/login';
import { AnalyticsService } from '@services/analytics.service';
import { AuthService } from '@services/auth.service';
import { UserPreferencesService } from '@services/user-preferences.service';

@Component({
    standalone: true,
    template: ''
})
class DummyRouteComponent {}

describe('Login', () => {
    let component: Login;
    let fixture: ComponentFixture<Login>;
    let returnUrl: string | null;
    let authError: string | null;
    let authMethod: string | null;
    let email: string | null;
    const authMock: {
        status: () => string;
        login: ReturnType<typeof vi.fn>;
        getSSOAvailability: ReturnType<typeof vi.fn>;
        getSocialProviders: ReturnType<typeof vi.fn>;
        startSocial: ReturnType<typeof vi.fn>;
        previewSocial: ReturnType<typeof vi.fn>;
        completeSocial: ReturnType<typeof vi.fn>;
        consumeSocialMfaHandoff: ReturnType<typeof vi.fn>;
        startSSOLogin: ReturnType<typeof vi.fn>;
        startPasskeyLogin: ReturnType<typeof vi.fn>;
        finishPasskeyLogin: ReturnType<typeof vi.fn>;
        verifyMfaTotp: ReturnType<typeof vi.fn>;
        verifyMfaRecoveryCode: ReturnType<typeof vi.fn>;
        requestMfaEmailLink: ReturnType<typeof vi.fn>;
    } = {
        status: () => 'unknown',
        login: vi.fn(() => of({ status: 'ok' as const })),
        getSSOAvailability: vi.fn(() => of({ enabled: false })),
        getSocialProviders: vi.fn(() => of({ providers: [], signup_enabled: false })),
        startSocial: vi.fn(() => of({ auth_url: 'https://identity.example.com/authorize' })),
        previewSocial: vi.fn(() => of({ provider: 'google', display_name: 'Google', observed_email: 'user@example.com', email_verified: true, email_confirmation_required: false, flow: 'login' })),
        completeSocial: vi.fn(() => of({ status: 'ok' as const })),
        consumeSocialMfaHandoff: vi.fn(() => null),
        startSSOLogin: vi.fn(() => of({ auth_url: 'https://identity.example.com/authorize' })),
        startPasskeyLogin: vi.fn(() =>
            of({
                challenge_token: '',
                publicKey: {
                    challenge: '',
                    rpId: '',
                    timeout: 0,
                    userVerification: 'preferred' as UserVerificationRequirement
                }
            })
        ),
        finishPasskeyLogin: vi.fn(() => of(void 0)),
        verifyMfaTotp: vi.fn(() => of(void 0)),
        verifyMfaRecoveryCode: vi.fn(() => of(void 0)),
        requestMfaEmailLink: vi.fn(() => of(void 0))
    };

    beforeEach(async () => {
        returnUrl = null;
        authError = null;
        authMethod = null;
        email = null;
        vi.clearAllMocks();
        authMock.getSocialProviders.mockReturnValue(of({ providers: [], signup_enabled: false }));
        authMock.startSocial.mockReturnValue(of({ auth_url: 'https://accounts.example.com/authorize' }));
        authMock.consumeSocialMfaHandoff.mockReturnValue(null);

        const preferencesMock = {
            load: () => of(void 0)
        } as unknown as UserPreferencesService;
        const analyticsMock = {
            getSystemStatus: () =>
                of({
                    needs_setup: false,
                    version: 'v2.0.0',
                    cloud: {
                        hosted: true,
                        signup_enabled: true,
                        jurisdiction: 'EU'
                    }
                })
        } as unknown as AnalyticsService;

        await TestBed.configureTestingModule({
            imports: [
                Login,
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
                provideRouter([
                    { path: 'dashboard', component: DummyRouteComponent },
                    { path: 'events', component: DummyRouteComponent }
                ]),
                { provide: AuthService, useValue: authMock as unknown as AuthService },
                { provide: AnalyticsService, useValue: analyticsMock },
                { provide: UserPreferencesService, useValue: preferencesMock },
                {
                    provide: ActivatedRoute,
                    useValue: {
                        snapshot: {
                            get queryParamMap() {
                                return convertToParamMap({
                                    ...(returnUrl ? { returnUrl } : {}),
                                    ...(authError ? { error: authError } : {}),
                                    ...(authMethod ? { method: authMethod } : {}),
                                    ...(email ? { email } : {})
                                });
                            }
                        }
                    }
                }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(Login);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('resolves valid in-app returnUrl', () => {
        returnUrl = '/events?range=30d';
        expect(component['resolveReturnUrl']()).toBe('/events?range=30d');
    });

    it('falls back for unsafe returnUrl', () => {
        for (const unsafe of ['https://evil.example/phish', '//evil.example/phish', '/\\evil.example/phish', '/%2f%2fevil.example/phish', '/%5cevil.example/phish', '/login?next=/dashboard']) {
            returnUrl = unsafe;
            expect(component['resolveReturnUrl']()).toBe('/');
        }
    });

    it('maps a revoked membership or invitation to the SSO access guidance', () => {
        expect(component['authErrorKey']('sso_access_denied')).toBe('login.errors.ssoAccessDenied');
    });

    it('routes successful logins without returnUrl through the authenticated start page', () => {
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        component['loginForm'].email().control().setValue('user@example.com');
        component['loginForm'].password().control().setValue('password123');

        component.onSubmit();

        expect(authMock.login).toHaveBeenCalledWith({
            email: 'user@example.com',
            password: 'password123',
            remember_me: false
        });
        expect(navigate).toHaveBeenCalledWith('/');
    });

    it('shows SSO only when the instance has an enabled team configuration', async () => {
        component['isPasskeySupported'].set(false);
        component['ssoAvailable'].set(false);
        await fixture.whenStable();

        expect(fixture.nativeElement.textContent).not.toContain('login.signInWithSSO');

        component['ssoAvailable'].set(true);
        await fixture.whenStable();

        expect(fixture.nativeElement.textContent).toContain('login.continueWithSSO');
    });

    it('puts available authentication methods before the password form', async () => {
        component['isPasskeySupported'].set(false);
        component['ssoAvailable'].set(true);

        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        const methods = element.querySelector<HTMLElement>('app-auth-methods');
        const emailField = element.querySelector<HTMLElement>('#email');
        if (!methods || !emailField) {
            throw new Error('expected authentication methods before the email field');
        }
        expect(methods.compareDocumentPosition(emailField) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });

    it('starts configured social login with the safe return URL and remember-me choice', () => {
        const navigate = vi.spyOn(component as unknown as { navigateToSSOProvider: (url: string) => void }, 'navigateToSSOProvider').mockImplementation(() => undefined);
        component['socialProviders'].set([{ id: 'google', display_name: 'Google' }]);
        component['loginForm'].rememberMe().control().setValue(true);
        returnUrl = '/events?range=30d';

        component['selectAuthMethod']('google');

        expect(authMock.startSocial).toHaveBeenCalledWith('google', {
            flow: 'login',
            return_url: '/events?range=30d',
            remember_me: true
        });
        expect(navigate).toHaveBeenCalledWith('https://accounts.example.com/authorize');
    });

    it('maps social providers to branded marks while retaining generic authentication icons', () => {
        component['isPasskeySupported'].set(false);
        component['socialProviders'].set([{ id: 'google', display_name: 'Google' }]);
        component['ssoAvailable'].set(true);

        const methods = component['authMethods']();
        const googleMethod = methods.find((method) => method.id === 'google');
        const ssoMethod = methods.find((method) => method.id === 'sso');

        expect(googleMethod?.providerIcon).toBe('google');
        expect(ssoMethod?.icon).toBe('pi pi-building');
    });

    it('requires an explicit HitKeep email before completing a first Microsoft login', () => {
        authMock.previewSocial.mockReturnValueOnce(of({ provider: 'microsoft', display_name: 'Microsoft', observed_email: 'mutable@example.com', email_verified: false, email_confirmation_required: true, flow: 'login' }));

        component['completeSocialLogin']('completion-token');

        expect(component['authMode']()).toBe('social');
        expect(component['loginForm'].email().value()).toBe('mutable@example.com');
        expect(authMock.completeSocial).not.toHaveBeenCalled();

        component['loginForm'].email().control().setValue('owner@example.com');
        component.onSubmit();

        expect(authMock.completeSocial).toHaveBeenCalledWith('completion-token', 'owner@example.com');
    });

    it('completes an already-linked Microsoft identity without asking for mutable email', () => {
        authMock.previewSocial.mockReturnValueOnce(of({ provider: 'microsoft', display_name: 'Microsoft', observed_email: 'mutable@example.com', email_verified: false, email_confirmation_required: false, flow: 'login' }));

        component['completeSocialLogin']('linked-completion-token');

        expect(authMock.completeSocial).toHaveBeenCalledWith('linked-completion-token', undefined);
        expect(component['authMode']()).toBe('password');
    });

    it('honors the server-bound return path after social callback completion', () => {
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        authMock.completeSocial.mockReturnValueOnce(of({ status: 'ok' as const, redirect_url: '/events?range=30d' }));

        component['completeSocialLogin']('return-path-token');

        expect(navigate).toHaveBeenCalledWith('/events?range=30d');
    });

    it('stays loading while preview hands off to social completion', () => {
        const preview = new Subject<{ provider: 'google'; display_name: string; observed_email: string; email_verified: boolean; email_confirmation_required: boolean; flow: 'login' }>();
        const completion = new Subject<{ status: 'ok'; redirect_url: string }>();
        authMock.previewSocial.mockReturnValueOnce(preview);
        authMock.completeSocial.mockReturnValueOnce(completion);

        component['completeSocialLogin']('loading-token');
        expect(component['isLoading']()).toBe(true);

        preview.next({ provider: 'google', display_name: 'Google', observed_email: 'user@example.com', email_verified: true, email_confirmation_required: false, flow: 'login' });
        preview.complete();
        expect(component['isLoading']()).toBe(true);

        completion.next({ status: 'ok', redirect_url: '/dashboard' });
        completion.complete();
        expect(component['isLoading']()).toBe(false);
    });

    it('keeps the server-bound social return path through MFA email verification', () => {
        authMock.completeSocial.mockReturnValueOnce(of({ status: 'mfa_required' as const, redirect_url: '/events?range=30d', challenge_token: 'social-mfa', factors: ['email_link' as const] }));

        component['completeSocialLogin']('social-mfa-token');
        component['requestEmailLinkMfa']();

        expect(authMock.requestMfaEmailLink).toHaveBeenCalledWith('social-mfa', '/events?range=30d');
    });

    it('resumes invitation social MFA in the existing login verification UI', () => {
        authMock.consumeSocialMfaHandoff.mockReturnValueOnce({
            status: 'mfa_required',
            challenge_token: 'social-invite-challenge',
            factors: ['totp']
        });

        fixture = TestBed.createComponent(Login);
        component = fixture.componentInstance;
        fixture.detectChanges();

        expect(component['mfaChallengeToken']()).toBe('social-invite-challenge');
        expect(component['mfaHasTotp']()).toBe(true);
        expect(fixture.nativeElement.textContent).toContain('login.mfaTitle');
    });

    it('uses PrimeNG surfaces for the auth card, feedback, and method divider', async () => {
        component['isPasskeySupported'].set(false);
        component['ssoAvailable'].set(true);
        component['errorMessage'].set('login.errors.unexpected');

        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        expect(element.querySelector('app-auth-card p-card.p-card')).toBeTruthy();
        expect(element.querySelector('p-message.p-message')).toBeTruthy();
        expect(element.querySelector('app-auth-divider p-divider.p-divider')).toBeTruthy();
    });

    it('switches to an email-only SSO workflow with a password fallback', async () => {
        component['isPasskeySupported'].set(false);
        component['ssoAvailable'].set(true);
        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        element.querySelector<HTMLButtonElement>('p-button[data-auth-method="sso"] button')?.click();
        await fixture.whenStable();

        expect(component['authMode']()).toBe('sso');
        expect(element.querySelector('#email')).toBeTruthy();
        expect(element.querySelector('#password')).toBeNull();
        expect(element.querySelector('#rememberMe')).toBeNull();
        expect(element.querySelector('p-button[data-auth-method="password"]')).toBeTruthy();
        expect(element.querySelector<HTMLButtonElement>('button[type="submit"]')?.textContent).toContain('login.continueWithSSO');
    });

    it('hydrates a requested SSO workflow and email only when SSO is available', async () => {
        authMethod = 'sso';
        email = 'analyst@example.com';
        authMock.getSSOAvailability.mockReturnValueOnce(of({ enabled: true }));

        fixture = TestBed.createComponent(Login);
        component = fixture.componentInstance;
        await fixture.whenStable();

        expect(component['authMode']()).toBe('sso');
        expect(component['loginForm'].email().value()).toBe('analyst@example.com');
    });

    it('starts SSO with the entered email and preserves the safe return URL', () => {
        const navigate = vi
            .spyOn(
                component as unknown as {
                    navigateToSSOProvider: (authURL: string) => void;
                },
                'navigateToSSOProvider'
            )
            .mockImplementation(() => undefined);
        returnUrl = '/events?range=30d';
        component['ssoAvailable'].set(true);
        component['loginForm'].email().control().setValue('analyst@example.com');
        component['loginForm'].rememberMe().control().setValue(true);

        component['onSSOLogin']();

        expect(authMock.startSSOLogin).toHaveBeenCalledWith({
            email: 'analyst@example.com',
            return_url: '/events?range=30d',
            remember_me: true
        });
        expect(navigate).toHaveBeenCalledWith('https://identity.example.com/authorize');
    });

    it('submits the email-only form through SSO instead of password login', () => {
        vi.spyOn(
            component as unknown as {
                navigateToSSOProvider: (authURL: string) => void;
            },
            'navigateToSSOProvider'
        ).mockImplementation(() => undefined);
        component['ssoAvailable'].set(true);
        component['selectAuthMethod']('sso');
        component['loginForm'].email().control().setValue('analyst@example.com');

        component.onSubmit();

        expect(authMock.startSSOLogin).toHaveBeenCalledWith({
            email: 'analyst@example.com',
            return_url: '/',
            remember_me: false
        });
        expect(authMock.login).not.toHaveBeenCalled();
    });

    it('does not start conditional passkey mediation in SSO mode', () => {
        window.localStorage.setItem('hitkeep.passkey.used_on_device', '1');
        component['isPasskeySupported'].set(true);
        component['authMode'].set('sso');

        expect(component['shouldAttemptConditionalPasskey']()).toBe(false);

        window.localStorage.removeItem('hitkeep.passkey.used_on_device');
    });

    it('requires a valid email before starting SSO', () => {
        component['loginForm'].email().control().setValue('not-an-email');

        component['onSSOLogin']();

        expect(authMock.startSSOLogin).not.toHaveBeenCalled();
        expect(component['loginForm'].email().touched()).toBe(true);
    });

    it('stores recovery-code MFA state from the login response', () => {
        authMock.login.mockReturnValueOnce(
            of({
                status: 'mfa_required' as const,
                challenge_token: 'challenge-123',
                factors: ['recovery_code' as const]
            })
        );

        component['loginForm'].email().control().setValue('user@example.com');
        component['loginForm'].password().control().setValue('password123');
        component.onSubmit();

        expect(component['mfaChallengeToken']()).toBe('challenge-123');
        expect(component['mfaHasRecoveryCode']()).toBe(true);
        expect(component['mfaHasTotp']()).toBe(false);
    });

    it('stores email-link MFA state from the login response', () => {
        authMock.login.mockReturnValueOnce(
            of({
                status: 'mfa_required' as const,
                challenge_token: 'challenge-email',
                factors: ['email_link' as const]
            })
        );

        component['loginForm'].email().control().setValue('user@example.com');
        component['loginForm'].password().control().setValue('password123');
        component.onSubmit();

        expect(component['mfaChallengeToken']()).toBe('challenge-email');
        expect(component['mfaHasEmailLink']()).toBe(true);
    });

    it('renders one MFA fallback divider for passkey and email-link alternatives', () => {
        component['isPasskeySupported'].set(true);
        component['mfaChallengeToken'].set('challenge-fallback');
        component['mfaFactors'].set(['totp', 'passkey', 'email_link']);

        fixture.detectChanges();

        expect(fixture.nativeElement.querySelectorAll('app-auth-divider p-divider').length).toBe(1);
        expect(fixture.nativeElement.querySelectorAll('.hk-auth-actions-stack p-button').length).toBe(2);
    });

    it('verifies recovery code MFA with the current challenge token', () => {
        component['mfaChallengeToken'].set('challenge-456');
        component['mfaFactors'].set(['recovery_code']);
        component['loginForm'].recoveryCode().control().setValue('ABCD-EFGH');

        component['verifyRecoveryCodeMfa']();

        expect(authMock.verifyMfaRecoveryCode).toHaveBeenCalledWith('challenge-456', 'ABCD-EFGH');
    });

    it('requests an MFA email link with the current challenge token and return url', () => {
        returnUrl = '/events?range=30d';
        component['mfaChallengeToken'].set('challenge-789');
        component['mfaFactors'].set(['email_link']);

        component['requestEmailLinkMfa']();

        expect(authMock.requestMfaEmailLink).toHaveBeenCalledWith('challenge-789', '/events?range=30d');
        expect(component['infoMessage']()).toBe('login.emailLinkSent');
    });

    it('links hosted signup to the local signup route without regional cloud links', () => {
        const footer = (fixture.nativeElement as HTMLElement).querySelector('.hk-auth-footer');
        const signupLinks = Array.from(footer?.querySelectorAll<HTMLAnchorElement>('a[href$="/signup"]') ?? []);

        expect(signupLinks.length).toBe(1);
        expect(signupLinks[0]?.getAttribute('href')).toBe('/signup');
        expect(footer?.querySelector('a[href^="https://cloud.hitkeep"]')).toBeNull();
    });

    it('hides the signup link when hosted signup is unavailable', async () => {
        component['cloudStatus'].set({
            hosted: false,
            signup_enabled: true,
            jurisdiction: 'EU'
        });
        await fixture.whenStable();
        expect((fixture.nativeElement as HTMLElement).querySelector('.hk-auth-footer a[href$="/signup"]')).toBeNull();

        component['cloudStatus'].set({
            hosted: true,
            signup_enabled: false,
            jurisdiction: 'EU'
        });
        await fixture.whenStable();
        expect((fixture.nativeElement as HTMLElement).querySelector('.hk-auth-footer a[href$="/signup"]')).toBeNull();
    });

    it('reuses a single passkey start request for concurrent standalone login attempts', async () => {
        authMock.startPasskeyLogin.mockReturnValueOnce(
            of({
                challenge_token: 'challenge-789',
                publicKey: {
                    challenge: 'challenge-bytes',
                    rpId: 'analytics.example.com',
                    timeout: 300000,
                    userVerification: 'required' as UserVerificationRequirement
                }
            })
        );

        const [first, second] = await Promise.all([component['getStandalonePasskeyStart'](), component['getStandalonePasskeyStart']()]);

        expect(authMock.startPasskeyLogin).toHaveBeenCalledTimes(1);
        expect(first.challenge_token).toBe('challenge-789');
        expect(second.challenge_token).toBe('challenge-789');
    });
});
