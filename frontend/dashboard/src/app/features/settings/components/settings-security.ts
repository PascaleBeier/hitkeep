import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';

import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { firstValueFrom, finalize } from 'rxjs';
import { TranslocoPipe } from '@jsverse/transloco';

// PrimeNG
import { ButtonModule } from 'primeng/button';
import { PasswordModule } from 'primeng/password';
import { MessageModule } from 'primeng/message';
import { InputOtpModule } from 'primeng/inputotp';
import { SettingsCard } from '@features/settings/components/settings-card';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';

// Core
import { AuthService } from '@services/auth.service';
import { SocialProvider, SocialProviderID } from '@services/auth.service';
import { PasskeyRegistrationFinishRequest, PasskeyRegistrationStartResponse, UserRecoveryCodesResponse, UserSecurityService, UserSecurityStatus, UserTotpSetup } from '@services/user-security.service';
import { UserProfileService } from '@services/user-profile.service';
import { toCreationResponseJson, toPublicKeyCreationOptions } from '@core/utils/webauthn';
import { ActivatedRoute } from '@angular/router';

@Component({
    selector: 'app-settings-security',
    imports: [ReactiveFormsModule, ButtonModule, PasswordModule, MessageModule, InputOtpModule, SettingsCard, RelativeDateTime, TranslocoPipe],
    templateUrl: './settings-security.html',
    styleUrl: './settings-security.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SettingsSecurity {
    private authService = inject(AuthService);
    private securityService = inject(UserSecurityService);
    private profileService = inject(UserProfileService);
    private route = inject(ActivatedRoute, { optional: true });

    protected readonly isPasswordLoading = signal(false);
    protected readonly passwordError = signal<string | null>(null);
    protected readonly passwordSuccess = signal(false);
    protected readonly setPasswordSent = signal(false);
    protected readonly socialProviders = signal<readonly SocialProvider[]>([]);
    protected readonly socialLoading = signal<SocialProviderID | null>(null);
    protected readonly socialError = signal<string | null>(null);
    protected readonly socialSuccess = signal<string | null>(null);

    protected readonly isSecurityLoading = signal(false);
    protected readonly securityError = signal<string | null>(null);
    protected readonly securityStatus = signal<UserSecurityStatus | null>(null);
    protected readonly totpSetup = signal<UserTotpSetup | null>(null);
    protected readonly totpError = signal<string | null>(null);
    protected readonly totpSuccess = signal<string | null>(null);
    protected readonly passkeyError = signal<string | null>(null);
    protected readonly passkeySuccess = signal<string | null>(null);
    protected readonly isTotpActionLoading = signal(false);
    protected readonly isPasskeyActionLoading = signal(false);
    protected readonly isRecoveryCodesLoading = signal(false);
    protected readonly recoveryCodeError = signal<string | null>(null);
    protected readonly recoveryCodeSuccess = signal<string | null>(null);
    protected readonly recoveryCodes = signal<string[]>([]);

    private readonly formModel = signal({
        currentPassword: new FormControl('', { nonNullable: true, validators: [Validators.required] }),
        newPassword: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.minLength(8)] }),
        totpCode: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.pattern(/^[0-9]{6}$/)] }),
        disableTotpCode: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.pattern(/^[0-9]{6}$/)] }),
        passkeyName: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(64)] })
    });
    protected readonly form = compatForm(this.formModel);
    protected readonly totpEnabled = computed(() => this.securityStatus()?.totp_enabled ?? false);
    protected readonly passkeys = computed(() => this.securityStatus()?.passkeys ?? []);
    protected readonly hasMfaProtection = computed(() => this.totpEnabled() || this.passkeys().length > 0);
    protected readonly recoveryCodesGenerated = computed(() => this.securityStatus()?.recovery_codes_generated ?? false);
    protected readonly recoveryCodesRemaining = computed(() => this.securityStatus()?.recovery_codes_remaining ?? 0);
    protected readonly passwordLoginEnabled = computed(() => this.securityStatus()?.password_login_enabled ?? true);
    protected readonly socialIdentities = computed(() => this.securityStatus()?.social_identities ?? []);
    protected readonly availableSocialProviders = computed(() => {
        const linked = new Set(this.socialIdentities().map((identity) => identity.provider));
        return this.socialProviders().filter((provider) => !linked.has(provider.id));
    });

    constructor() {
        this.loadSecurityStatus();
        this.authService.getSocialProviders().subscribe({
            next: (response) => this.socialProviders.set(response.providers),
            error: () => this.socialProviders.set([])
        });
        const socialResult = this.route?.snapshot.queryParamMap.get('social');
        if (socialResult === 'linked') {
            this.socialSuccess.set('settings.security.social.linked');
        } else if (socialResult) {
            this.socialError.set('settings.security.social.errors.linkFailed');
        }
    }

    protected onSubmit(event?: Event): void {
        event?.preventDefault();
        if (this.form.currentPassword().invalid() || this.form.newPassword().invalid()) {
            this.form.currentPassword().markAsTouched();
            this.form.newPassword().markAsTouched();
            return;
        }

        this.isPasswordLoading.set(true);
        this.passwordError.set(null);
        this.passwordSuccess.set(false);

        const currentPassword = this.form.currentPassword().value();
        const newPassword = this.form.newPassword().value();

        this.authService
            .changePassword(currentPassword, newPassword)
            .pipe(finalize(() => this.isPasswordLoading.set(false)))
            .subscribe({
                next: () => {
                    this.passwordSuccess.set(true);
                    this.form.currentPassword().control().reset('');
                    this.form.newPassword().control().reset('');
                },
                error: (err) => {
                    // 403 is returned by backend for invalid current password
                    // TODO: adhere to RESTFUL
                    const msg = err.status === 403 ? 'settings.security.errors.invalidCurrentPassword' : 'settings.security.errors.updateFailed';
                    this.passwordError.set(msg);
                }
            });
    }

    protected requestSetPassword(): void {
        const email = this.profileService.profile()?.email;
        if (!email) {
            this.passwordError.set('settings.security.errors.updateFailed');
            return;
        }
        this.isPasswordLoading.set(true);
        this.passwordError.set(null);
        this.setPasswordSent.set(false);
        this.authService
            .requestPasswordReset(email)
            .pipe(finalize(() => this.isPasswordLoading.set(false)))
            .subscribe({
                next: () => this.setPasswordSent.set(true),
                error: () => this.passwordError.set('settings.security.errors.updateFailed')
            });
    }

    protected connectSocial(provider: SocialProviderID): void {
        if (this.socialLoading() !== null) return;
        this.socialLoading.set(provider);
        this.socialError.set(null);
        this.socialSuccess.set(null);
        this.securityService
            .startSocialLink(provider)
            .pipe(finalize(() => this.socialLoading.set(null)))
            .subscribe({
                next: (response) => window.location.assign(response.auth_url),
                error: () => this.socialError.set('settings.security.social.errors.linkFailed')
            });
    }

    protected unlinkSocial(provider: SocialProviderID): void {
        this.socialLoading.set(provider);
        this.socialError.set(null);
        this.socialSuccess.set(null);
        this.securityService
            .unlinkSocial(provider, this.form.currentPassword().value())
            .pipe(finalize(() => this.socialLoading.set(null)))
            .subscribe({
                next: () => {
                    this.securityStatus.update((current) =>
                        current
                            ? {
                                  ...current,
                                  social_identities: current.social_identities.filter((identity) => identity.provider !== provider)
                              }
                            : current
                    );
                    this.socialSuccess.set('settings.security.social.unlinked');
                },
                error: (err) => {
                    this.socialError.set(err?.error?.code === 'social_last_login_method' ? 'settings.security.social.errors.lastMethod' : 'settings.security.social.errors.unlinkFailed');
                }
            });
    }

    protected providerName(provider: SocialProviderID): string {
        return this.socialProviders().find((entry) => entry.id === provider)?.display_name ?? provider;
    }

    protected startTotpSetup(): void {
        this.isTotpActionLoading.set(true);
        this.totpError.set(null);
        this.totpSuccess.set(null);

        this.securityService
            .startTotpSetup()
            .pipe(finalize(() => this.isTotpActionLoading.set(false)))
            .subscribe({
                next: (setup) => {
                    this.totpSetup.set(setup);
                    this.form.totpCode().control().reset('');
                },
                error: () => {
                    this.totpError.set('settings.security.twoFactor.errors.startFailed');
                }
            });
    }

    protected verifyTotpSetup(): void {
        const control = this.form.totpCode().control();
        if (control.invalid || !this.totpSetup()) {
            control.markAsTouched();
            return;
        }

        this.isTotpActionLoading.set(true);
        this.totpError.set(null);
        this.totpSuccess.set(null);

        this.securityService
            .verifyTotpSetup(control.value)
            .pipe(finalize(() => this.isTotpActionLoading.set(false)))
            .subscribe({
                next: (status) => {
                    this.securityStatus.set(status);
                    this.totpSetup.set(null);
                    control.reset('');
                    this.form.disableTotpCode().control().reset('');
                    this.totpSuccess.set('settings.security.twoFactor.enabled');
                },
                error: () => {
                    this.totpError.set('settings.security.twoFactor.errors.verifyFailed');
                }
            });
    }

    protected disableTotp(): void {
        const control = this.form.disableTotpCode().control();
        if (control.invalid || !this.totpEnabled()) {
            control.markAsTouched();
            return;
        }

        this.isTotpActionLoading.set(true);
        this.totpError.set(null);
        this.totpSuccess.set(null);

        this.securityService
            .disableTotp(control.value)
            .pipe(finalize(() => this.isTotpActionLoading.set(false)))
            .subscribe({
                next: (status) => {
                    this.securityStatus.set(status);
                    this.totpSetup.set(null);
                    this.form.totpCode().control().reset('');
                    control.reset('');
                    this.totpSuccess.set('settings.security.twoFactor.disabled');
                },
                error: () => {
                    this.totpError.set('settings.security.twoFactor.errors.disableFailed');
                }
            });
    }

    protected async registerPasskey(): Promise<void> {
        if (typeof window === 'undefined' || typeof navigator === 'undefined' || !window.PublicKeyCredential || !navigator.credentials) {
            this.passkeyError.set('settings.security.passkeys.errors.notSupported');
            return;
        }

        if (this.form.passkeyName().invalid()) {
            this.form.passkeyName().markAsTouched();
            return;
        }

        const requestedName = this.form.passkeyName().value().trim();
        this.isPasskeyActionLoading.set(true);
        this.passkeyError.set(null);
        this.passkeySuccess.set(null);

        try {
            const begin = await firstValueFrom(this.securityService.startPasskeyRegistration(requestedName || undefined));
            const credential = await navigator.credentials.create({
                publicKey: this.toPublicKeyOptions(begin)
            });

            if (!(credential instanceof PublicKeyCredential) || !(credential.response instanceof AuthenticatorAttestationResponse)) {
                this.passkeyError.set('settings.security.passkeys.errors.registrationFailed');
                return;
            }

            const payload = this.toPasskeyFinishPayload(credential);
            if (!payload) {
                this.passkeyError.set('settings.security.passkeys.errors.notSupported');
                return;
            }
            const status = await firstValueFrom(this.securityService.finishPasskeyRegistration(payload));
            this.securityStatus.set(status);
            this.form.passkeyName().control().reset('');
            this.passkeySuccess.set('settings.security.passkeys.registered');
        } catch {
            this.passkeyError.set('settings.security.passkeys.errors.registrationFailed');
        } finally {
            this.isPasskeyActionLoading.set(false);
        }
    }

    protected deletePasskey(passkeyID: string): void {
        this.isPasskeyActionLoading.set(true);
        this.passkeyError.set(null);
        this.passkeySuccess.set(null);

        this.securityService
            .deletePasskey(passkeyID)
            .pipe(finalize(() => this.isPasskeyActionLoading.set(false)))
            .subscribe({
                next: () => {
                    this.securityStatus.update((current) => {
                        if (!current) return current;
                        return {
                            ...current,
                            passkeys: current.passkeys.filter((entry) => entry.id !== passkeyID)
                        };
                    });
                    this.passkeySuccess.set('settings.security.passkeys.deleted');
                },
                error: () => {
                    this.passkeyError.set('settings.security.passkeys.errors.deleteFailed');
                }
            });
    }

    protected regenerateRecoveryCodes(): void {
        if (!this.hasMfaProtection()) {
            this.recoveryCodeError.set('settings.security.recoveryCodes.errors.mfaRequired');
            return;
        }

        this.isRecoveryCodesLoading.set(true);
        this.recoveryCodeError.set(null);
        this.recoveryCodeSuccess.set(null);

        this.securityService
            .regenerateRecoveryCodes()
            .pipe(finalize(() => this.isRecoveryCodesLoading.set(false)))
            .subscribe({
                next: (response) => this.handleRecoveryCodeResponse(response),
                error: (err) => {
                    const message = err.status === 409 ? 'settings.security.recoveryCodes.errors.mfaRequired' : 'settings.security.recoveryCodes.errors.generateFailed';
                    this.recoveryCodeError.set(message);
                }
            });
    }

    protected async copyRecoveryCodes(): Promise<void> {
        if (this.recoveryCodes().length === 0) {
            return;
        }
        if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
            this.recoveryCodeError.set('settings.security.recoveryCodes.errors.copyFailed');
            this.recoveryCodeSuccess.set(null);
            return;
        }

        this.recoveryCodeError.set(null);
        this.recoveryCodeSuccess.set(null);

        try {
            await navigator.clipboard.writeText(this.buildRecoveryCodesText());
            this.recoveryCodeSuccess.set('settings.security.recoveryCodes.copied');
        } catch {
            this.recoveryCodeError.set('settings.security.recoveryCodes.errors.copyFailed');
        }
    }

    protected downloadRecoveryCodes(): void {
        if (this.recoveryCodes().length === 0) {
            return;
        }
        if (typeof window === 'undefined' || typeof document === 'undefined' || !window.URL?.createObjectURL) {
            this.recoveryCodeError.set('settings.security.recoveryCodes.errors.downloadFailed');
            this.recoveryCodeSuccess.set(null);
            return;
        }

        this.recoveryCodeError.set(null);
        this.recoveryCodeSuccess.set(null);

        const blob = new Blob([this.buildRecoveryCodesText()], { type: 'text/plain;charset=utf-8' });
        const url = window.URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = `hitkeep-recovery-codes-${new Date().toISOString().slice(0, 10)}.txt`;

        try {
            link.click();
            this.recoveryCodeSuccess.set('settings.security.recoveryCodes.downloaded');
        } catch {
            this.recoveryCodeError.set('settings.security.recoveryCodes.errors.downloadFailed');
        } finally {
            window.URL.revokeObjectURL(url);
        }
    }

    private loadSecurityStatus(): void {
        this.isSecurityLoading.set(true);
        this.securityError.set(null);

        this.securityService
            .loadStatus()
            .pipe(finalize(() => this.isSecurityLoading.set(false)))
            .subscribe({
                next: (status) => {
                    this.securityStatus.set(status);
                    this.recoveryCodes.set([]);
                },
                error: () => {
                    this.securityError.set('settings.security.errors.loadFailed');
                }
            });
    }

    private handleRecoveryCodeResponse(response: UserRecoveryCodesResponse): void {
        this.recoveryCodes.set(response.codes);
        this.securityStatus.update((current) => {
            if (!current) {
                return current;
            }

            return {
                ...current,
                recovery_codes_generated: true,
                recovery_codes_remaining: response.remaining
            };
        });
        this.recoveryCodeSuccess.set('settings.security.recoveryCodes.generated');
    }

    private buildRecoveryCodesText(): string {
        return this.recoveryCodes().join('\n');
    }

    private toPublicKeyOptions(response: PasskeyRegistrationStartResponse): PublicKeyCredentialCreationOptions {
        return toPublicKeyCreationOptions(response.publicKey);
    }

    private toPasskeyFinishPayload(credential: PublicKeyCredential): PasskeyRegistrationFinishRequest | null {
        return toCreationResponseJson(credential);
    }
}
