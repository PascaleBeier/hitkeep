import { ChangeDetectionStrategy, Component, OnInit, inject, signal } from '@angular/core';

import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { ActivatedRoute, Router } from '@angular/router';
import { finalize } from 'rxjs';
import { TranslocoPipe } from '@jsverse/transloco';

// OptimusUI
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageModule } from '@openng/optimus-ui/message';
import { PasswordModule } from '@openng/optimus-ui/password';

// Core
import { AuthCard } from '@core/components/auth-card/auth-card';
import { Brand } from '@components/brand/brand';
import { AuthService } from '@services/auth.service';

@Component({
    selector: 'app-reset-password',
    standalone: true,
    imports: [AuthCard, ReactiveFormsModule, Brand, ButtonModule, MessageModule, PasswordModule, TranslocoPipe],
    templateUrl: './reset-password.html',
    styleUrl: './reset-password.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ResetPassword implements OnInit {
    private route = inject(ActivatedRoute);
    private router = inject(Router);
    private authService = inject(AuthService);

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
        this.router.navigate(['/login']);
    }
}
