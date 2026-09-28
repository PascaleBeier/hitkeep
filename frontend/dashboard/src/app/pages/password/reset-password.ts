import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, OnInit, inject, signal } from '@angular/core';

import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { ActivatedRoute, Router } from '@angular/router';
import { finalize } from 'rxjs';
import { TranslocoPipe } from '@jsverse/transloco';

// OptimusUI
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageModule } from '@openng/optimus-ui/message';

// Core
import { AuthCard } from '@core/components/auth-card/auth-card';
import { PasswordInput } from '@core/components/password-input/password-input';
import { Brand } from '@components/brand/brand';
import { AuthService } from '@services/auth.service';
import { cloudBillingReviewUrl, cloudPurchaseIntent, cloudPurchaseQuery } from '@core/utils/cloud-purchase-intent';

@Component({
    selector: 'app-reset-password',
    standalone: true,
    imports: [AuthCard, PasswordInput, ReactiveFormsModule, Brand, ButtonModule, MessageModule, TranslocoPipe],
    templateUrl: './reset-password.html',
    styleUrl: './reset-password.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ResetPassword implements OnInit {
    private readonly document = inject(DOCUMENT);
    private route = inject(ActivatedRoute);
    private router = inject(Router);
    private authService = inject(AuthService);
    private readonly purchaseIntent = cloudPurchaseIntent(this.route.snapshot.queryParamMap.get('plan'), this.route.snapshot.queryParamMap.get('billing'));

    protected token: string | null = null;
    protected isLoading = signal(false);
    protected errorMessage = signal<string | null>(null);
    protected successMessage = signal(false);

    private readonly formModel = signal({
        password: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.minLength(8)] })
    });
    protected readonly form = compatForm(this.formModel);

    ngOnInit() {
        this.token = this.route.snapshot.queryParamMap.get('token');
        if (!this.token) {
            this.errorMessage.set('password.reset.errors.invalidLink');
        }
    }

    onSubmit(event?: Event) {
        event?.preventDefault();
        if (!this.token) return;
        if (this.form().invalid()) {
            this.form.password().markAsTouched();
            this.document.getElementById('password')?.focus();
            return;
        }

        this.isLoading.set(true);
        this.errorMessage.set(null);

        this.authService
            .resetPassword(this.token, this.form.password().value())
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: () => this.successMessage.set(true),
                error: (err) => {
                    if (err.status === 400) {
                        this.errorMessage.set('password.reset.errors.expiredOrInvalid');
                    } else {
                        this.errorMessage.set('password.reset.errors.resetFailed');
                    }
                }
            });
    }

    goToLogin() {
        const intent = this.purchaseIntent;
        void this.router.navigate(['/login'], {
            queryParams: intent.plan === 'free' ? {} : { ...cloudPurchaseQuery(intent), returnUrl: cloudBillingReviewUrl(intent) }
        });
    }
}
