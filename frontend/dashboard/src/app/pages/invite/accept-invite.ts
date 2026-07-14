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
import { AuthService } from '@services/auth.service';

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
    protected readonly ssoAvailable = signal(false);
    protected readonly isAutoAccepting = signal(false);
    protected readonly errorMessage = signal<string | null>(null);
    protected readonly loginRequired = signal(false);
    protected readonly authMethods = computed<readonly AuthMethodOption[]>(() => [
        {
            id: 'sso',
            labelKey: 'login.continueWithSSO',
            icon: 'pi pi-building',
            wide: true,
            loading: this.isSSOLoading(),
            disabled: this.isLoading() || this.isSSOLoading()
        }
    ]);

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
        }
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
