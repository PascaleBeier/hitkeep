import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Router } from '@angular/router';
import { FormControl, Validators, ReactiveFormsModule } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { finalize } from 'rxjs/operators';
import { TranslocoPipe } from '@jsverse/transloco';

// PrimeNG Imports
import { PasswordModule } from 'primeng/password';
import { ButtonModule } from 'primeng/button';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';

// Corrected path to Core
import { AuthCard } from '@core/components/auth-card/auth-card';
import { Brand } from '@components/brand/brand';

@Component({
    selector: 'app-setup',
    standalone: true,
    imports: [AuthCard, Brand, ReactiveFormsModule, PasswordModule, ButtonModule, InputTextModule, MessageModule, TranslocoPipe],
    templateUrl: './setup.html',
    styleUrl: './setup.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class Setup {
    private http = inject(HttpClient);
    private router = inject(Router);

    protected isLoading = signal(false);
    protected errorMessage = signal<string | null>(null);

    private readonly setupModel = signal({
        givenName: new FormControl('', { nonNullable: true }),
        lastName: new FormControl('', { nonNullable: true }),
        email: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.email] }),
        password: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.minLength(8)] })
    });
    protected readonly setupForm = compatForm(this.setupModel);

    onSubmit(event?: Event): void {
        event?.preventDefault();
        if (this.setupForm().invalid()) {
            this.setupForm.email().markAsTouched();
            this.setupForm.password().markAsTouched();
            return;
        }

        this.isLoading.set(true);
        this.errorMessage.set(null);

        const email = this.setupForm.email().value();
        const password = this.setupForm.password().value();
        const givenName = this.setupForm.givenName().value().trim();
        const lastName = this.setupForm.lastName().value().trim();

        const payload: { email: string; password: string; given_name?: string; last_name?: string } = { email, password };
        if (givenName !== '') {
            payload.given_name = givenName;
        }
        if (lastName !== '') {
            payload.last_name = lastName;
        }

        this.http
            .post('/api/initial-user', payload)
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: () => {
                    this.router.navigate(['/dashboard']);
                },
                error: (err) => {
                    console.error('Failed to create initial user:', err);
                    this.errorMessage.set('setup.errors.unexpected');
                }
            });
    }
}
