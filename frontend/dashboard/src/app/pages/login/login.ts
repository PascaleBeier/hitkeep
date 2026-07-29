import { ChangeDetectionStrategy, Component, DestroyRef, computed, inject, signal } from '@angular/core';

import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { firstValueFrom } from 'rxjs';
import { finalize } from 'rxjs/operators';
import { TranslocoPipe } from '@jsverse/transloco';

import { PasswordModule } from '@openng/optimus-ui/password';
import { ButtonModule } from '@openng/optimus-ui/button';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { CheckboxModule } from '@openng/optimus-ui/checkbox';
import { InputOtpModule } from '@openng/optimus-ui/inputotp';
import { MessageModule } from '@openng/optimus-ui/message';
import { TooltipModule } from '@openng/optimus-ui/tooltip';

import { AuthCard } from '@core/components/auth-card/auth-card';
import { AuthDivider } from '@core/components/auth-divider/auth-divider';
import { Brand } from '@components/brand/brand';
import { AuthMethodOption, AuthMethods } from '@core/components/auth-methods/auth-methods';
import { CloudStatus } from '@models/analytics.types';
import { AuthService, LoginResponse, PasskeyLoginFinishRequest, PasskeyLoginStartResponse, SocialProvider, SocialProviderID } from '@services/auth.service';
import { AnalyticsService } from '@services/analytics.service';
import { UserPreferencesService } from '@services/user-preferences.service';
import { readSessionEndState, SessionEndReason } from '@services/session-end-navigation.service';
import { toAssertionResponseJson, toPublicKeyRequestOptions } from '@core/utils/webauthn';

type MfaFactor = 'totp' | 'passkey' | 'recovery_code' | 'email_link';
type AuthMode = 'password' | 'sso' | 'social';

const SESSION_END_NOTICES = {
    'signed-out': { key: 'login.sessionEnded.signedOut', severity: 'success' },
    'session-ended': { key: 'login.sessionEnded.expired', severity: 'warn' }
} as const;

@Component({
    selector: 'app-login',
    standalone: true,
    imports: [AuthCard, AuthDivider, AuthMethods, Brand, ReactiveFormsModule, PasswordModule, ButtonModule, InputTextModule, CheckboxModule, InputOtpModule, MessageModule, TooltipModule, RouterLink, TranslocoPipe],
    templateUrl: './login.html',
    styleUrl: './login.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class Login {
    private static readonly PASSKEY_DEVICE_HISTORY_KEY = 'hitkeep.passkey.used_on_device';
    private destroyRef = inject(DestroyRef);
    private router = inject(Router);
    private route = inject(ActivatedRoute);
    private auth = inject(AuthService);
    private analytics = inject(AnalyticsService);
    private preferences = inject(UserPreferencesService);
    private conditionalPasskeyAbortController: AbortController | null = null;
    private standalonePasskeyStartRequest: Promise<PasskeyLoginStartResponse> | null = null;
    private standalonePasskeyStartResponse: PasskeyLoginStartResponse | null = null;
    private hasAttemptedConditionalPasskey = false;
    protected isLoading = signal(false);
    protected isPasskeyLoading = signal(false);
    protected isSSOLoading = signal(false);
    protected socialLoading = signal<SocialProviderID | null>(null);
    protected ssoAvailable = signal(false);
    protected socialProviders = signal<readonly SocialProvider[]>([]);
    protected pendingSocialCompletion = signal<string | null>(null);
    private readonly pendingSocialReturnUrl = signal<string | null>(null);
    protected readonly authMode = signal<AuthMode>('password');
    protected errorMessage = signal<string | null>(null);
    protected infoMessage = signal<string | null>(null);
    protected readonly sessionEndReason = signal<SessionEndReason | null>(null);
    protected readonly sessionEndNotice = computed(() => {
        const reason = this.sessionEndReason();
        return reason ? SESSION_END_NOTICES[reason] : null;
    });
    protected currentYear = new Date().getFullYear();
    protected readonly isPasskeySupported = signal(typeof window !== 'undefined' && typeof navigator !== 'undefined' && Boolean(window.PublicKeyCredential) && Boolean(navigator.credentials));
    protected readonly mfaChallengeToken = signal<string | null>(null);
    protected readonly mfaFactors = signal<MfaFactor[]>([]);
    protected readonly mfaPasskeyOptions = signal<PasskeyLoginStartResponse['publicKey'] | null>(null);
    protected readonly cloudStatus = signal<CloudStatus | null>(null);
    protected readonly isMfaRequired = computed(() => this.mfaChallengeToken() !== null);
    protected readonly mfaHasTotp = computed(() => this.mfaFactors().includes('totp'));
    protected readonly mfaHasRecoveryCode = computed(() => this.mfaFactors().includes('recovery_code'));
    protected readonly mfaHasPasskey = computed(() => this.mfaFactors().includes('passkey'));
    protected readonly mfaHasEmailLink = computed(() => this.mfaFactors().includes('email_link'));
    protected readonly mfaHasVisiblePasskey = computed(() => this.isPasskeySupported() && this.mfaHasPasskey());
    protected readonly mfaHasActionFallback = computed(() => this.mfaHasVisiblePasskey() || this.mfaHasEmailLink());
    protected readonly mfaHasFallback = computed(() => this.mfaHasRecoveryCode() || this.mfaHasActionFallback());
    protected readonly mfaShowsFallbackDivider = computed(() => this.mfaHasFallback() && (this.mfaHasTotp() || (this.mfaHasRecoveryCode() && this.mfaHasActionFallback())));
    protected readonly showSignupLink = computed(() => Boolean(this.cloudStatus()?.hosted && this.cloudStatus()?.signup_enabled));
    protected readonly authMethods = computed<readonly AuthMethodOption[]>(() => {
        const disabled = this.isLoading() || this.isPasskeyLoading() || this.isSSOLoading() || this.socialLoading() !== null;
        if (this.authMode() === 'sso') {
            return [
                {
                    id: 'password',
                    labelKey: 'login.continueWithPassword',
                    icon: 'pi pi-lock',
                    wide: true,
                    disabled
                }
            ];
        }
        if (this.authMode() === 'social') {
            return [
                {
                    id: 'password',
                    labelKey: 'login.continueWithPassword',
                    icon: 'pi pi-lock',
                    wide: true,
                    disabled
                }
            ];
        }

        const methods: AuthMethodOption[] = [];
        for (const provider of this.socialProviders()) {
            methods.push({
                id: provider.id,
                labelKey: `social.continueWith.${provider.id}`,
                providerIcon: provider.id,
                wide: true,
                loading: this.socialLoading() === provider.id,
                disabled
            });
        }
        if (this.isPasskeySupported()) {
            methods.push({
                id: 'passkey',
                labelKey: 'login.signInWithPasskey',
                icon: 'pi pi-key',
                wide: true,
                loading: this.isPasskeyLoading(),
                disabled
            });
        }
        if (this.ssoAvailable()) {
            methods.push({
                id: 'sso',
                labelKey: 'login.continueWithSSO',
                icon: 'pi pi-building',
                wide: true,
                loading: this.isSSOLoading(),
                disabled
            });
        }
        return methods;
    });
    protected readonly authSubtitleKey = computed(() => (this.authMode() === 'sso' ? 'login.ssoSubtitle' : this.authMode() === 'social' ? 'social.confirmEmailSubtitle' : 'login.subtitle'));
    protected readonly primaryActionKey = computed(() => (this.authMode() === 'sso' ? 'login.continueWithSSO' : this.authMode() === 'social' ? 'social.confirmEmailAction' : 'login.signIn'));

    private readonly loginModel = signal({
        email: new FormControl('', {
            nonNullable: true,
            validators: [Validators.required, Validators.email]
        }),
        password: new FormControl('', {
            nonNullable: true,
            validators: [Validators.required]
        }),
        rememberMe: new FormControl(false, { nonNullable: true }),
        mfaCode: new FormControl('', {
            nonNullable: true,
            validators: [Validators.required, Validators.pattern(/^[0-9]{6}$/)]
        }),
        recoveryCode: new FormControl('', {
            nonNullable: true,
            validators: [Validators.required]
        })
    });
    protected readonly loginForm = compatForm(this.loginModel);

    constructor() {
        // Browser history state can outlive the navigation that created it. Only
        // honour state carried by this router navigation, so a direct or reloaded
        // /login URL never inherits an old session-ended notice.
        const sessionEndState = readSessionEndState(this.router.getCurrentNavigation()?.extras.state);
        if (sessionEndState) {
            this.sessionEndReason.set(sessionEndState.reason);
        }

        const requestedEmail = this.route.snapshot.queryParamMap.get('email')?.trim();
        const requestedSSO = this.route.snapshot.queryParamMap.get('method')?.trim().toLowerCase() === 'sso';
        if (requestedEmail) {
            this.loginForm.email().control().setValue(requestedEmail);
        }

        this.analytics
            .getSystemStatus()
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (status) => this.cloudStatus.set(status.cloud ?? null),
                error: (err) => {
                    console.error('Failed to load cloud status for login', err);
                }
            });

        this.auth
            .getSSOAvailability()
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (availability) => {
                    this.ssoAvailable.set(availability.enabled);
                    if (availability.enabled && requestedSSO) {
                        this.abortConditionalPasskeyPrompt();
                        this.authMode.set('sso');
                    }
                },
                error: () => {
                    this.ssoAvailable.set(false);
                    this.authMode.set('password');
                }
            });

        this.auth
            .getSocialProviders()
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (response) => this.socialProviders.set(response.providers),
                error: () => this.socialProviders.set([])
            });

        const authError = this.route.snapshot.queryParamMap.get('error')?.trim();
        const authErrorKey = this.authErrorKey(authError);
        if (authErrorKey) {
            this.errorMessage.set(authErrorKey);
        }
        if (this.route.snapshot.queryParamMap.get('info') === 'social_confirmed') {
            this.infoMessage.set('social.confirmedSignIn');
        }

        const socialMfaHandoff = this.auth.consumeSocialMfaHandoff();
        const completionToken = this.socialCompletionToken();
        if (socialMfaHandoff?.status === 'mfa_required') {
            this.pendingSocialReturnUrl.set(this.safeReturnUrl(socialMfaHandoff.redirect_url));
            this.enterMfaState({
                status: 'mfa_required',
                challenge_token: socialMfaHandoff.challenge_token,
                factors: socialMfaHandoff.factors,
                passkey: socialMfaHandoff.passkey
            });
        } else if (completionToken) {
            this.completeSocialLogin(completionToken);
        }

        if (!completionToken && this.shouldAttemptConditionalPasskey()) {
            void this.startConditionalPasskeyLogin();
        }
    }

    protected onSSOLogin(): void {
        if (this.loginForm.email().invalid()) {
            this.loginForm.email().markAsTouched();
            return;
        }

        this.abortConditionalPasskeyPrompt();
        this.isSSOLoading.set(true);
        this.errorMessage.set(null);
        this.infoMessage.set(null);

        this.auth
            .startSSOLogin({
                email: this.loginForm.email().value(),
                return_url: this.resolveReturnUrl(),
                remember_me: this.loginForm.rememberMe().value()
            })
            .pipe(finalize(() => this.isSSOLoading.set(false)))
            .subscribe({
                next: (response) => this.navigateToSSOProvider(response.auth_url),
                error: (err) => {
                    if (err?.status === 403 || err?.error?.code === 'sso_access_denied') {
                        this.errorMessage.set('login.errors.ssoAccessDenied');
                    } else {
                        this.errorMessage.set(err?.status === 400 || err?.status === 503 ? 'login.errors.ssoUnavailable' : 'login.errors.ssoFailed');
                    }
                }
            });
    }

    protected selectAuthMethod(method: string): void {
        switch (method) {
            case 'password':
                this.authMode.set('password');
                this.pendingSocialCompletion.set(null);
                this.pendingSocialReturnUrl.set(null);
                this.errorMessage.set(null);
                this.infoMessage.set(null);
                break;
            case 'sso':
                if (!this.ssoAvailable()) {
                    return;
                }
                this.abortConditionalPasskeyPrompt();
                this.authMode.set('sso');
                this.errorMessage.set(null);
                this.infoMessage.set(null);
                break;
            case 'passkey':
                void this.onPasskeyLogin();
                break;
            default:
                if (this.socialProviders().some((provider) => provider.id === method)) {
                    this.startSocialLogin(method as SocialProviderID);
                }
        }
    }

    private startSocialLogin(provider: SocialProviderID): void {
        if (this.socialLoading() !== null) return;
        this.abortConditionalPasskeyPrompt();
        this.socialLoading.set(provider);
        this.errorMessage.set(null);
        this.infoMessage.set(null);
        this.auth
            .startSocial(provider, {
                flow: 'login',
                return_url: this.resolveReturnUrl(),
                remember_me: this.loginForm.rememberMe().value()
            })
            .pipe(finalize(() => this.socialLoading.set(null)))
            .subscribe({
                next: (response) => this.navigateToSSOProvider(response.auth_url),
                error: (err) => this.errorMessage.set(this.socialErrorKey(err?.error?.code))
            });
    }

    private completeSocialLogin(completionToken: string): void {
        this.isLoading.set(true);
        this.errorMessage.set(null);
        this.auth.previewSocial(completionToken).subscribe({
            next: (preview) => {
                const requestedEmail = this.loginForm.email().value().trim();
                if (preview.email_confirmation_required && this.route.snapshot.queryParamMap.get('info') !== 'social_confirmed') {
                    this.isLoading.set(false);
                    this.pendingSocialCompletion.set(completionToken);
                    this.authMode.set('social');
                    if (!requestedEmail && preview.observed_email) {
                        this.loginForm.email().control().setValue(preview.observed_email);
                    }
                    this.infoMessage.set('social.confirmEmailPrompt');
                    return;
                }
                this.submitSocialCompletion(completionToken);
            },
            error: (err) => {
                this.isLoading.set(false);
                this.errorMessage.set(this.socialErrorKey(err?.error?.code));
            }
        });
    }

    private submitSocialCompletion(completionToken: string, email?: string): void {
        this.pendingSocialCompletion.set(null);
        this.isLoading.set(true);
        this.auth
            .completeSocial(completionToken, email)
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: (response) => {
                    const socialReturnUrl = this.safeReturnUrl(response.redirect_url);
                    if (response.status === 'mfa_required') {
                        this.pendingSocialReturnUrl.set(socialReturnUrl);
                        this.enterMfaState({
                            status: 'mfa_required',
                            challenge_token: response.challenge_token,
                            factors: response.factors,
                            passkey: response.passkey
                        });
                        return;
                    }
                    if (response.status === 'signup_required') {
                        this.pendingSocialReturnUrl.set(null);
                        const signupToken = response.completion_token?.trim();
                        if (!signupToken) {
                            this.authMode.set('password');
                            this.errorMessage.set('social.errors.failed');
                            return;
                        }
                        void this.router.navigate(['/signup/social/complete'], {
                            fragment: `social_token=${encodeURIComponent(signupToken)}`
                        });
                        return;
                    }
                    if (response.status === 'verification_sent') {
                        this.pendingSocialReturnUrl.set(null);
                        this.authMode.set('password');
                        this.infoMessage.set('social.confirmationSent');
                        return;
                    }
                    this.clearMfaState();
                    this.redirectAfterLogin(socialReturnUrl);
                },
                error: (err) => {
                    this.pendingSocialReturnUrl.set(null);
                    this.authMode.set('password');
                    this.errorMessage.set(this.socialErrorKey(err?.error?.code));
                }
            });
    }

    onSubmit(event?: Event): void {
        event?.preventDefault();
        if (this.isMfaRequired()) {
            if (this.mfaHasTotp()) {
                this.verifyTotpMfa();
                return;
            }
            if (this.mfaHasRecoveryCode()) {
                this.verifyRecoveryCodeMfa();
                return;
            }
            if (this.mfaHasPasskey()) {
                void this.onPasskeyLogin();
                return;
            }
            this.errorMessage.set('login.errors.unexpected');
            return;
        }
        if (this.authMode() === 'sso') {
            this.onSSOLogin();
            return;
        }
        const socialCompletion = this.pendingSocialCompletion();
        if (this.authMode() === 'social' && socialCompletion) {
            if (this.loginForm.email().invalid()) {
                this.loginForm.email().markAsTouched();
                return;
            }
            this.submitSocialCompletion(socialCompletion, this.loginForm.email().value().trim());
            return;
        }
        if (this.loginForm.email().invalid() || this.loginForm.password().invalid()) {
            this.loginForm.email().markAsTouched();
            this.loginForm.password().markAsTouched();
            return;
        }
        this.startPasswordLogin();
    }

    private startPasswordLogin(): void {
        this.abortConditionalPasskeyPrompt();
        this.isLoading.set(true);
        this.errorMessage.set(null);
        this.infoMessage.set(null);

        const email = this.loginForm.email().value();
        const password = this.loginForm.password().value();
        const rememberMe = this.loginForm.rememberMe().value();

        this.auth
            .login({ email, password, remember_me: rememberMe })
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: (resp) => {
                    if (resp.status === 'mfa_required') {
                        this.enterMfaState(resp);
                        return;
                    }
                    this.clearMfaState();
                    this.redirectAfterLogin();
                },
                error: (err) => {
                    console.error('Login failed:', err);
                    if (err.status === 401) {
                        this.errorMessage.set('login.errors.invalidCredentials');
                    } else {
                        this.errorMessage.set('login.errors.unexpected');
                    }
                }
            });
    }

    protected async onPasskeyLogin(): Promise<void> {
        if (!this.isPasskeySupported()) {
            this.errorMessage.set('login.errors.passkeyNotSupported');
            return;
        }

        this.abortConditionalPasskeyPrompt(false);
        this.isPasskeyLoading.set(true);
        this.errorMessage.set(null);

        try {
            const isMfaPasskey = this.isMfaRequired() && this.mfaFactors().includes('passkey');
            let challengeToken = this.mfaChallengeToken() ?? '';
            let options: PasskeyLoginStartResponse['publicKey'] | null = this.mfaPasskeyOptions();

            if (!isMfaPasskey) {
                const start = await this.getStandalonePasskeyStart();
                challengeToken = start.challenge_token;
                options = start.publicKey;
            }
            if (!challengeToken || !options) {
                this.errorMessage.set('login.errors.passkeyFailed');
                return;
            }

            const credential = await navigator.credentials.get({
                publicKey: this.toPasskeyRequestOptions(options)
            });

            if (!(credential instanceof PublicKeyCredential) || !(credential.response instanceof AuthenticatorAssertionResponse)) {
                this.errorMessage.set('login.errors.passkeyFailed');
                return;
            }

            const payload = this.toPasskeyFinishPayload(credential, challengeToken, this.loginForm.rememberMe().value());
            await firstValueFrom(this.auth.finishPasskeyLogin(payload));
            if (!isMfaPasskey) {
                this.clearStandalonePasskeyStart(challengeToken);
            }
            this.markPasskeyUsedOnDevice();
            this.clearMfaState();
            this.redirectAfterLogin();
        } catch (err) {
            if (!this.isMfaRequired()) {
                this.clearStandalonePasskeyStart();
            }
            if ((err as { status?: number })?.status === 401 || (err as { status?: number })?.status === 403) {
                this.errorMessage.set('login.errors.passkeyFailed');
            } else {
                this.errorMessage.set('login.errors.passkeyFailed');
            }
        } finally {
            this.isPasskeyLoading.set(false);
        }
    }

    protected clearMfaState(): void {
        this.mfaChallengeToken.set(null);
        this.mfaFactors.set([]);
        this.mfaPasskeyOptions.set(null);
        this.loginForm.mfaCode().control().reset('');
        this.loginForm.recoveryCode().control().reset('');
        this.infoMessage.set(null);
    }

    private redirectAfterLogin(preferredReturnUrl?: string | null) {
        const targetUrl = preferredReturnUrl ?? this.pendingSocialReturnUrl() ?? this.resolveReturnUrl();
        this.pendingSocialReturnUrl.set(null);
        this.preferences.load().subscribe({
            next: () => {
                void this.router.navigateByUrl(targetUrl);
            },
            error: () => {
                void this.router.navigateByUrl(targetUrl);
            }
        });
    }

    private resolveReturnUrl(): string {
        return this.safeReturnUrl(this.route.snapshot.queryParamMap.get('returnUrl')) ?? '/';
    }

    private safeReturnUrl(raw: string | null | undefined): string | null {
        const returnUrl = raw?.trim();
        if (!returnUrl) return null;
        const containsUnsafeCharacter = ['#', '\\', '\r', '\n', '\t', String.fromCharCode(0)].some((character) => returnUrl.includes(character));
        if (!returnUrl.startsWith('/') || returnUrl.startsWith('//') || containsUnsafeCharacter) return null;
        try {
            const parsed = new URL(returnUrl, 'https://hitkeep.invalid');
            const decodedPath = decodeURIComponent(parsed.pathname);
            const decodedPathContainsUnsafeCharacter = ['\\', '\r', '\n', '\t', String.fromCharCode(0)].some((character) => decodedPath.includes(character));
            if (parsed.origin !== 'https://hitkeep.invalid' || !decodedPath.startsWith('/') || decodedPath.startsWith('//') || decodedPathContainsUnsafeCharacter || decodedPath.startsWith('/login') || decodedPath.startsWith('/setup')) return null;
        } catch {
            return null;
        }
        return returnUrl;
    }

    private authErrorKey(error: string | undefined): string | null {
        switch (error) {
            case 'mfa_link_invalid':
                return 'login.errors.mfaEmailLinkInvalid';
            case 'sso_provider_error':
                return 'login.errors.ssoProviderError';
            case 'sso_email_unverified':
                return 'login.errors.ssoEmailUnverified';
            case 'sso_email_not_allowed':
                return 'login.errors.ssoEmailNotAllowed';
            case 'sso_unavailable':
                return 'login.errors.ssoUnavailable';
            case 'sso_access_denied':
                return 'login.errors.ssoAccessDenied';
            case 'sso_failed':
                return 'login.errors.ssoFailed';
            case 'social_provider_cancelled':
            case 'social_state_invalid':
            case 'social_completion_invalid':
            case 'social_email_unverified':
            case 'social_provider_unavailable':
            case 'social_identity_conflict':
            case 'social_confirmation_invalid':
            case 'social_login_failed':
                return this.socialErrorKey(error);
            default:
                return null;
        }
    }

    private socialCompletionToken(): string | null {
        if (typeof window === 'undefined') return null;
        const token = new URLSearchParams(window.location.hash.replace(/^#/, '')).get('social_token');
        return token?.trim() || null;
    }

    private socialErrorKey(code?: string): string {
        switch (code) {
            case 'social_email_unverified':
                return 'social.errors.emailUnverified';
            case 'social_identity_conflict':
                return 'social.errors.identityConflict';
            case 'social_provider_cancelled':
                return 'social.errors.cancelled';
            case 'social_email_required':
                return 'social.errors.emailRequired';
            case 'social_account_not_found':
                return 'social.errors.accountNotFound';
            case 'social_confirmation_failed':
                return 'social.errors.confirmationFailed';
            case 'social_provider_unavailable':
                return 'social.errors.unavailable';
            default:
                return 'social.errors.failed';
        }
    }

    private navigateToSSOProvider(authURL: string): void {
        window.location.assign(authURL);
    }

    private enterMfaState(resp: LoginResponse): void {
        if (!resp.challenge_token) {
            this.errorMessage.set('login.errors.unexpected');
            return;
        }
        const factors: MfaFactor[] = resp.factors && resp.factors.length > 0 ? resp.factors : resp.passkey ? ['passkey'] : ['totp'];
        this.mfaChallengeToken.set(resp.challenge_token);
        this.mfaFactors.set(factors);
        this.mfaPasskeyOptions.set(resp.passkey ?? null);
        this.loginForm.mfaCode().control().reset('');
        this.loginForm.recoveryCode().control().reset('');
        this.errorMessage.set(null);
        this.infoMessage.set(null);
    }

    private verifyTotpMfa(): void {
        this.abortConditionalPasskeyPrompt();
        const challengeToken = this.mfaChallengeToken();
        if (!challengeToken) {
            this.errorMessage.set('login.errors.unexpected');
            return;
        }
        if (this.loginForm.mfaCode().invalid()) {
            this.loginForm.mfaCode().markAsTouched();
            return;
        }

        this.isLoading.set(true);
        this.errorMessage.set(null);
        this.infoMessage.set(null);
        this.auth
            .verifyMfaTotp(challengeToken, this.loginForm.mfaCode().value())
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: () => {
                    this.clearMfaState();
                    this.redirectAfterLogin();
                },
                error: (err) => {
                    if (err.status === 401 || err.status === 403) {
                        this.errorMessage.set('login.errors.invalidTotp');
                    } else {
                        this.errorMessage.set('login.errors.unexpected');
                    }
                }
            });
    }

    protected verifyRecoveryCodeMfa(): void {
        this.abortConditionalPasskeyPrompt();
        const challengeToken = this.mfaChallengeToken();
        if (!challengeToken) {
            this.errorMessage.set('login.errors.unexpected');
            return;
        }
        if (this.loginForm.recoveryCode().invalid()) {
            this.loginForm.recoveryCode().markAsTouched();
            return;
        }

        this.isLoading.set(true);
        this.errorMessage.set(null);
        this.infoMessage.set(null);
        this.auth
            .verifyMfaRecoveryCode(challengeToken, this.loginForm.recoveryCode().value())
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: () => {
                    this.clearMfaState();
                    this.redirectAfterLogin();
                },
                error: (err) => {
                    if (err.status === 401 || err.status === 403) {
                        this.errorMessage.set('login.errors.invalidRecoveryCode');
                    } else {
                        this.errorMessage.set('login.errors.unexpected');
                    }
                }
            });
    }

    protected requestEmailLinkMfa(): void {
        this.abortConditionalPasskeyPrompt();
        const challengeToken = this.mfaChallengeToken();
        if (!challengeToken) {
            this.errorMessage.set('login.errors.unexpected');
            return;
        }

        this.isLoading.set(true);
        this.errorMessage.set(null);
        this.infoMessage.set(null);
        this.auth
            .requestMfaEmailLink(challengeToken, this.pendingSocialReturnUrl() ?? this.resolveReturnUrl())
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: () => {
                    this.infoMessage.set('login.emailLinkSent');
                },
                error: () => {
                    this.errorMessage.set('login.errors.emailLinkFailed');
                }
            });
    }

    private async startConditionalPasskeyLogin(): Promise<void> {
        this.hasAttemptedConditionalPasskey = true;
        this.conditionalPasskeyAbortController = new AbortController();
        const abortSignal = this.conditionalPasskeyAbortController.signal;

        try {
            const start = await this.getStandalonePasskeyStart();
            if (abortSignal.aborted) {
                return;
            }

            const request: CredentialRequestOptions = {
                publicKey: this.toPasskeyRequestOptions(start.publicKey),
                mediation: 'conditional' as CredentialMediationRequirement,
                signal: abortSignal
            };
            const credential = await navigator.credentials.get(request);
            if (abortSignal.aborted || !credential) {
                return;
            }

            if (!(credential instanceof PublicKeyCredential) || !(credential.response instanceof AuthenticatorAssertionResponse)) {
                return;
            }

            const payload = this.toPasskeyFinishPayload(credential, start.challenge_token, this.loginForm.rememberMe().value());
            await firstValueFrom(this.auth.finishPasskeyLogin(payload));
            this.clearStandalonePasskeyStart(start.challenge_token);
            this.markPasskeyUsedOnDevice();
            this.clearMfaState();
            this.redirectAfterLogin();
        } catch (err) {
            if (this.isAbortError(err) || this.isNotAllowedError(err)) {
                return;
            }
            this.clearStandalonePasskeyStart();
            // Keep conditional mediation silent on unexpected failures.
            console.warn('Conditional passkey mediation failed', err);
        } finally {
            this.conditionalPasskeyAbortController = null;
        }
    }

    private shouldAttemptConditionalPasskey(): boolean {
        if (this.hasAttemptedConditionalPasskey) {
            return false;
        }
        if (this.isMfaRequired()) {
            return false;
        }
        if (this.authMode() !== 'password') {
            return false;
        }
        if (!this.isPasskeySupported()) {
            return false;
        }
        return this.hasUsedPasskeyOnDevice();
    }

    private hasUsedPasskeyOnDevice(): boolean {
        if (typeof window === 'undefined') {
            return false;
        }
        try {
            return window.localStorage.getItem(Login.PASSKEY_DEVICE_HISTORY_KEY) === '1';
        } catch {
            return false;
        }
    }

    private markPasskeyUsedOnDevice(): void {
        if (typeof window === 'undefined') {
            return;
        }
        try {
            window.localStorage.setItem(Login.PASSKEY_DEVICE_HISTORY_KEY, '1');
        } catch {
            // best-effort only
        }
    }

    private abortConditionalPasskeyPrompt(clearStandaloneStart = true): void {
        if (this.conditionalPasskeyAbortController) {
            this.conditionalPasskeyAbortController.abort();
            this.conditionalPasskeyAbortController = null;
        }
        if (clearStandaloneStart) {
            this.clearStandalonePasskeyStart();
        }
    }

    private isAbortError(err: unknown): boolean {
        return err instanceof DOMException && err.name === 'AbortError';
    }

    private isNotAllowedError(err: unknown): boolean {
        return err instanceof DOMException && err.name === 'NotAllowedError';
    }

    private toPasskeyRequestOptions(options: PasskeyLoginStartResponse['publicKey']): PublicKeyCredentialRequestOptions {
        return toPublicKeyRequestOptions(options);
    }

    private toPasskeyFinishPayload(credential: PublicKeyCredential, challengeToken: string, rememberMe: boolean): PasskeyLoginFinishRequest {
        const serialized = toAssertionResponseJson(credential);
        if (!serialized) {
            throw new Error('Invalid passkey assertion response');
        }

        return {
            challenge_token: challengeToken,
            credential: serialized,
            remember_me: rememberMe
        };
    }

    private getStandalonePasskeyStart(): Promise<PasskeyLoginStartResponse> {
        if (this.standalonePasskeyStartResponse) {
            return Promise.resolve(this.standalonePasskeyStartResponse);
        }

        if (this.standalonePasskeyStartRequest) {
            return this.standalonePasskeyStartRequest;
        }

        this.standalonePasskeyStartRequest = firstValueFrom(this.auth.startPasskeyLogin())
            .then((start) => {
                this.standalonePasskeyStartResponse = start;
                return start;
            })
            .finally(() => {
                this.standalonePasskeyStartRequest = null;
            });

        return this.standalonePasskeyStartRequest;
    }

    private clearStandalonePasskeyStart(challengeToken?: string): void {
        if (!challengeToken || this.standalonePasskeyStartResponse?.challenge_token === challengeToken) {
            this.standalonePasskeyStartResponse = null;
        }
    }
}
