import { ChangeDetectionStrategy, Component, OnInit, inject, signal } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { ActivatedRoute, Router } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';
import { PasswordModule } from 'primeng/password';
import { finalize } from 'rxjs';

import { Brand } from '@components/brand/brand';
import { AuthService } from '@services/auth.service';

@Component({
    selector: 'app-accept-invite',
    standalone: true,
    imports: [ReactiveFormsModule, Brand, ButtonModule, PasswordModule, TranslocoPipe],
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
    protected readonly isAutoAccepting = signal(false);
    protected readonly errorMessage = signal<string | null>(null);
    protected readonly loginRequired = signal(false);

    private readonly formModel = signal({
        password: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.minLength(8)] })
    });
    protected readonly form = compatForm(this.formModel);

    ngOnInit(): void {
        this.token = this.route.snapshot.queryParamMap.get('token');
        if (!this.token) {
            this.errorMessage.set('invite.accept.errors.tokenMissing');
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
