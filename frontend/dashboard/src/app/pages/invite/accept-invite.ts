import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { ActivatedRoute, Router } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';
import { MessageModule } from 'primeng/message';
import { PasswordModule } from 'primeng/password';
import { finalize } from 'rxjs';

import { Brand } from '@components/brand/brand';
import { AuthCard } from '@core/components/auth-card/auth-card';
import { AuthDivider } from '@core/components/auth-divider/auth-divider';
import { AuthMethodOption, AuthMethods } from '@core/components/auth-methods/auth-methods';
import { AuthService, SocialProvider, SocialProviderID } from '@services/auth.service';

@Component({
    selector: 'app-accept-invite',
    standalone: true,
    imports: [ReactiveFormsModule, AuthCard, AuthDivider, AuthMethods, Brand, ButtonModule, MessageModule, PasswordModule, TranslocoPipe],
    templateUrl: './accept-invite.html',
    styleUrl: './accept-invite.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AcceptInvite implements OnInit {
    private readonly route = inject(ActivatedRoute);
    private readonly router = inject(Router);
    private readonly authService = inject(AuthService);

    protected token: string | null = null;
    protected readonly isCheckingSession = signal(false);
    protected readonly isLoading = signal(false);
    protected readonly isSSOLoading = signal(false);
    protected readonly socialLoading = signal<SocialProviderID | null>(null);
    protected readonly ssoAvailable = signal(false);
    protected readonly socialProviders = signal<readonly SocialProvider[]>([]);
    protected readonly isAutoAccepting = signal(false);
    protected readonly errorMessage = signal<string | null>(null);
    protected readonly loginRequired = signal(false);
    protected readonly authMethods = computed<readonly AuthMethodOption[]>(() => {
        const disabled = this.isLoading() || this.isSSOLoading() || this.socialLoading() !== null;
        const methods: AuthMethodOption[] = this.socialProviders().map((provider) => ({
            id: provider.id,
            labelKey: `social.continueWith.${provider.id}`,
            icon: provider.id === 'github' ? 'pi pi-github' : provider.id === 'google' ? 'pi pi-google' : 'pi pi-microsoft',
            wide: true,
            loading: this.socialLoading() === provider.id,
            disabled
        }));
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

    private readonly formModel = signal({
        password: new FormControl('', {
            nonNullable: true,
            validators: [Validators.required, Validators.minLength(8)]
        })
    });
    protected readonly form = compatForm(this.formModel);

    ngOnInit(): void {
        this.token = this.route.snapshot.queryParamMap.get('token');
        if (!this.token) {
            this.errorMessage.set('invite.accept.errors.tokenMissing');
            return;
        }
        if (this.route.snapshot.queryParamMap.get('error')?.trim().startsWith('sso_')) {
            this.errorMessage.set('invite.accept.errors.ssoFailed');
        }
        if (this.route.snapshot.queryParamMap.get('error')?.trim().startsWith('social_')) {
            this.errorMessage.set('social.errors.failed');
        }
        this.authService.getSocialProviders().subscribe({
            next: (response) => this.socialProviders.set(response.providers),
            error: () => this.socialProviders.set([])
        });
        const completionToken = this.socialCompletionToken();
        if (completionToken) {
            this.completeSocialInvite(completionToken);
            return;
        }
        if (this.authService.isAuthenticated()) {
            this.acceptAuthenticatedInvite();
            return;
        }
        this.isCheckingSession.set(true);
        this.authService
            .loadSession()
            .pipe(finalize(() => this.isCheckingSession.set(false)))
            .subscribe((session) => {
                if (session || this.authService.isAuthenticated()) {
                    this.acceptAuthenticatedInvite();
                    return;
                }
                this.authService.markUnauthenticated();
                this.loadInviteSSOAvailability();
            });
    }

    protected selectAuthMethod(method: string): void {
        if (method === 'sso') {
            this.onSSOLogin();
            return;
        }
        if (this.socialProviders().some((provider) => provider.id === method)) {
            this.onSocialLogin(method as SocialProviderID);
        }
    }

    protected onSocialLogin(provider: SocialProviderID): void {
        if (!this.token || this.socialLoading() !== null) return;
        this.socialLoading.set(provider);
        this.errorMessage.set(null);
        this.authService
            .startSocial(provider, {
                flow: 'invite',
                invite_token: this.token,
                return_url: '/dashboard'
            })
            .pipe(finalize(() => this.socialLoading.set(null)))
            .subscribe({
                next: (response) => this.navigateToSSOProvider(response.auth_url),
                error: () => this.errorMessage.set('social.errors.failed')
            });
    }

    protected onSSOLogin(): void {
        if (!this.token || !this.ssoAvailable() || this.isSSOLoading()) return;
        this.isSSOLoading.set(true);
        this.errorMessage.set(null);
        this.authService
            .startSSOLogin({
                invite_token: this.token,
                return_url: '/dashboard'
            })
            .pipe(finalize(() => this.isSSOLoading.set(false)))
            .subscribe({
                next: (response) => this.navigateToSSOProvider(response.auth_url),
                error: () => this.errorMessage.set('invite.accept.errors.ssoFailed')
            });
    }

    protected onSubmit(event?: Event): void {
        event?.preventDefault();
        if (!this.token) return;
        if (this.form().invalid()) {
            this.form.password().markAsTouched();
            return;
        }

        this.isLoading.set(true);
        this.errorMessage.set(null);
        this.loginRequired.set(false);

        this.authService
            .acceptInvite(this.token, this.form.password().value())
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: () => {
                    void this.router.navigateByUrl('/dashboard');
                },
                error: (error: unknown) => this.handleAcceptError(error)
            });
    }

    protected goToLogin(): void {
        if (!this.token) return;
        void this.router.navigate(['/login'], {
            queryParams: { returnUrl: `/accept-invite?token=${this.token}` }
        });
    }

    private acceptAuthenticatedInvite(): void {
        if (!this.token) return;
        this.isLoading.set(true);
        this.isAutoAccepting.set(true);
        this.errorMessage.set(null);
        this.loginRequired.set(false);
        this.authService
            .acceptInvite(this.token)
            .pipe(
                finalize(() => {
                    this.isLoading.set(false);
                    this.isAutoAccepting.set(false);
                })
            )
            .subscribe({
                next: () => {
                    void this.router.navigateByUrl('/dashboard');
                },
                error: (error: unknown) => this.handleAcceptError(error)
            });
    }

    private loadInviteSSOAvailability(): void {
        if (!this.token) return;
        this.authService.getInviteSSOAvailability(this.token).subscribe({
            next: (availability) => this.ssoAvailable.set(availability.enabled),
            error: () => this.ssoAvailable.set(false)
        });
    }

    private navigateToSSOProvider(authURL: string): void {
        window.location.assign(authURL);
    }

    private completeSocialInvite(completionToken: string): void {
        this.isLoading.set(true);
        this.authService
            .completeSocial(completionToken)
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: (response) => {
                    if (response.status === 'ok') {
                        void this.router.navigateByUrl(response.redirect_url || '/dashboard');
                        return;
                    }
                    if (response.status === 'mfa_required') {
                        this.authService.setSocialMfaHandoff(response);
                        void this.router.navigate(['/login'], {
                            queryParams: { returnUrl: response.redirect_url || '/dashboard' }
                        });
                        return;
                    }
                    this.errorMessage.set('social.errors.failed');
                },
                error: () => this.errorMessage.set('social.errors.failed')
            });
    }

    private socialCompletionToken(): string | null {
        if (typeof window === 'undefined') return null;
        return new URLSearchParams(window.location.hash.replace(/^#/, '')).get('social_token')?.trim() || null;
    }

    private handleAcceptError(error: unknown): void {
        if (error instanceof HttpErrorResponse && error.status === 400) {
            this.errorMessage.set('invite.accept.errors.expiredOrInvalid');
            return;
        }
        if (error instanceof HttpErrorResponse && error.status === 401) {
            this.loginRequired.set(true);
            return;
        }
        if (error instanceof HttpErrorResponse && error.status === 403) {
            this.errorMessage.set('invite.accept.errors.teamLimit');
            return;
        }
        this.errorMessage.set('invite.accept.errors.acceptFailed');
    }
}
