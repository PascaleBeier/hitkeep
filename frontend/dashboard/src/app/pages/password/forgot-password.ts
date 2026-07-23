import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';

import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { RouterLink } from '@angular/router';
import { finalize } from 'rxjs';
import { TranslocoPipe } from '@jsverse/transloco';

// OptimusUI
import { ButtonModule } from '@openng/optimus-ui/button';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MessageModule } from '@openng/optimus-ui/message';

// Core
import { AuthCard } from '@core/components/auth-card/auth-card';
import { Brand } from '@components/brand/brand';
import { AuthService } from '@services/auth.service';

@Component({
    selector: 'app-forgot-password',
    standalone: true,
    imports: [AuthCard, ReactiveFormsModule, RouterLink, Brand, ButtonModule, InputTextModule, MessageModule, TranslocoPipe],
    templateUrl: './forgot-password.html',
    styleUrl: './forgot-password.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ForgotPassword {
    private authService = inject(AuthService);

    protected isLoading = signal(false);
    protected errorMessage = signal<string | null>(null);
    protected successMessage = signal(false);

    private readonly formModel = signal({
        email: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.email] })
    });
    protected readonly form = compatForm(this.formModel);

    onSubmit(event?: Event) {
        event?.preventDefault();
        if (this.form().invalid()) {
            this.form.email().markAsTouched();
            return;
        }

        this.isLoading.set(true);
        this.errorMessage.set(null);

        this.authService
            .requestPasswordReset(this.form.email().value())
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: () => this.successMessage.set(true),
                error: () => {
                    // Even on error, we usually don't want to block the UI for security enumeration reasons,
                    // but if the API fails hard (500), show a generic message.
                    this.errorMessage.set('password.forgot.errors.unexpected');
                }
            });
    }
}
