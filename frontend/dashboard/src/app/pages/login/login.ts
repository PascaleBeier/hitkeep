import { ChangeDetectionStrategy, Component, DestroyRef, computed, inject, signal } from '@angular/core';

import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { firstValueFrom } from 'rxjs';
import { finalize } from 'rxjs/operators';
import { TranslocoPipe } from '@jsverse/transloco';

import { PasswordModule } from 'primeng/password';
import { ButtonModule } from 'primeng/button';
import { InputTextModule } from 'primeng/inputtext';
import { CheckboxModule } from 'primeng/checkbox';
import { InputOtpModule } from 'primeng/inputotp';
import { MessageModule } from 'primeng/message';
import { TooltipModule } from 'primeng/tooltip';

import { AuthCard } from '@core/components/auth-card/auth-card';
import { AuthDivider } from '@core/components/auth-divider/auth-divider';
import { Brand } from '@components/brand/brand';
import { AuthMethodOption, AuthMethods } from '@core/components/auth-methods/auth-methods';
import { CloudStatus } from '@models/analytics.types';
import { AuthService, LoginResponse, PasskeyLoginFinishRequest, PasskeyLoginStartResponse } from '@services/auth.service';
import { AnalyticsService } from '@services/analytics.service';
import { UserPreferencesService } from '@services/user-preferences.service';
import { toAssertionResponseJson, toPublicKeyRequestOptions } from '@core/utils/webauthn';

type MfaFactor = 'totp' | 'passkey' | 'recovery_code' | 'email_link';
type AuthMode = 'password' | 'sso';

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
    protected ssoAvailable = signal(false);
    protected readonly authMode = signal<AuthMode>('password');
    protected errorMessage = signal<string | null>(null);
    protected infoMessage = signal<string | null>(null);
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
        const disabled = this.isLoading() || this.isPasskeyLoading() || this.isSSOLoading();
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

        const methods: AuthMethodOption[] = [];
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
    protected readonly authSubtitleKey = computed(() => (this.authMode() === 'sso' ? 'login.ssoSubtitle' : 'login.subtitle'));
    protected readonly primaryActionKey = computed(() => (this.authMode() === 'sso' ? 'login.continueWithSSO' : 'login.signIn'));

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

        const authError = this.route.snapshot.queryParamMap.get('error')?.trim();
        const authErrorKey = this.authErrorKey(authError);
        if (authErrorKey) {
            this.errorMessage.set(authErrorKey);
        }

        if (this.shouldAttemptConditionalPasskey()) {
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
        }
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

    private redirectAfterLogin() {
        const targetUrl = this.resolveReturnUrl();
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
        const returnUrl = this.route.snapshot.queryParamMap.get('returnUrl')?.trim();
        if (!returnUrl) return '/';
        if (!returnUrl.startsWith('/') || returnUrl.startsWith('//')) return '/';
        if (returnUrl.startsWith('/login') || returnUrl.startsWith('/setup')) return '/';
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
            default:
                return null;
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
            .requestMfaEmailLink(challengeToken, this.resolveReturnUrl())
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
