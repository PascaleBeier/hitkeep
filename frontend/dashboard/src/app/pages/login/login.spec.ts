import { Component } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
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
    const authMock: {
        status: () => string;
        login: ReturnType<typeof vi.fn>;
        startPasskeyLogin: ReturnType<typeof vi.fn>;
        finishPasskeyLogin: ReturnType<typeof vi.fn>;
        verifyMfaTotp: ReturnType<typeof vi.fn>;
        verifyMfaRecoveryCode: ReturnType<typeof vi.fn>;
        requestMfaEmailLink: ReturnType<typeof vi.fn>;
    } = {
        status: () => 'unknown',
        login: vi.fn(() => of({ status: 'ok' as const })),
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
        vi.clearAllMocks();

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
                                return convertToParamMap(returnUrl ? { returnUrl } : {});
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
        returnUrl = 'https://evil.example/phish';
        expect(component['resolveReturnUrl']()).toBe('/');
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

        expect(fixture.nativeElement.querySelectorAll('.hk-auth-divider').length).toBe(1);
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

    it('builds region-aware signup URLs for hosted cloud', () => {
        expect(component['currentJurisdiction']()).toBe('EU');
        expect(component['primarySignupUrl']()).toBe('/signup');
        expect(component['alternateJurisdiction']()).toBe('US');
        expect(component['alternateSignupUrl']()).toBe('https://cloud.hitkeep.com/signup');
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
