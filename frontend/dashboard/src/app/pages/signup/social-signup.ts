import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { finalize } from 'rxjs';

import { ButtonModule } from 'primeng/button';
import { CheckboxModule } from 'primeng/checkbox';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';

import { Brand } from '@components/brand/brand';
import { AuthCard } from '@core/components/auth-card/auth-card';
import { injectActiveLang } from '@core/i18n/active-lang';
import { AuthService, SocialPreviewResponse } from '@services/auth.service';
import { BillingInterval, CloudPlanCode } from '@services/cloud.service';
import { CloudSignupTrackingService } from '@services/cloud-signup-tracking.service';

type Jurisdiction = 'EU' | 'US';

@Component({
    selector: 'app-social-signup',
    imports: [AuthCard, Brand, ButtonModule, CheckboxModule, InputTextModule, MessageModule, ReactiveFormsModule, RouterLink, TranslocoPipe],
    templateUrl: './social-signup.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SocialSignup {
    private readonly document = inject(DOCUMENT);
    private readonly route = inject(ActivatedRoute);
    private readonly router = inject(Router);
    private readonly auth = inject(AuthService);
    private readonly tracking = inject(CloudSignupTrackingService);
    private readonly activeLanguage = injectActiveLang();

    protected readonly isLoading = signal(true);
    protected readonly errorMessage = signal<string | null>(null);
    protected readonly verificationSent = signal(false);
    protected readonly preview = signal<SocialPreviewResponse | null>(null);
    protected readonly currentYear = new Date().getFullYear();
    protected readonly selectedPlan = signal<CloudPlanCode>(this.normalizePlan(this.route.snapshot.queryParamMap.get('plan')));
    protected readonly selectedBilling = signal<BillingInterval>(this.normalizeBilling(this.route.snapshot.queryParamMap.get('billing')));
    protected readonly jurisdiction = signal<Jurisdiction>(this.normalizeJurisdiction(this.route.snapshot.queryParamMap.get('region')) ?? this.inferJurisdiction());
    private readonly completionToken = this.readCompletionToken();

    private readonly formModel = signal({
        email: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.email] }),
        teamName: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.maxLength(120)] }),
        acceptedTos: new FormControl(false, { nonNullable: true, validators: [Validators.requiredTrue] })
    });
    protected readonly form = compatForm(this.formModel);

    constructor() {
        this.tracking.install();
        if (!this.completionToken) {
            this.isLoading.set(false);
            this.errorMessage.set('social.errors.completionExpired');
            return;
        }
        this.auth
            .previewSocial(this.completionToken)
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: (preview) => {
                    this.preview.set(preview);
                    if (preview.observed_email) this.form.email().control().setValue(preview.observed_email);
                    this.track('signup_page_view');
                },
                error: () => this.errorMessage.set('social.errors.completionExpired')
            });
    }

    protected onSubmit(event?: Event): void {
        event?.preventDefault();
        if (!this.completionToken || !this.preview() || this.form().invalid()) {
            this.form.email().markAsTouched();
            this.form.teamName().markAsTouched();
            this.form.acceptedTos().markAsTouched();
            return;
        }
        this.isLoading.set(true);
        this.errorMessage.set(null);
        this.track('signup_started');
        this.auth
            .completeSocialSignup({
                completion_token: this.completionToken,
                email: this.preview()!.provider === 'microsoft' ? this.form.email().value().trim().toLowerCase() : undefined,
                team_name: this.form.teamName().value().trim(),
                plan_code: this.selectedPlan(),
                billing: this.selectedBilling(),
                jurisdiction: this.jurisdiction(),
                locale: this.activeLanguage(),
                accepted_tos: true
            })
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: (response) => {
                    this.track('signup_completed_candidate', { response_status: response.status });
                    if (response.status === 'verification_sent') {
                        this.verificationSent.set(true);
                        return;
                    }
                    void this.router.navigateByUrl(response.redirect_url?.trim() || '/dashboard');
                },
                error: (err) => {
                    const code = err?.error?.code;
                    this.errorMessage.set(code === 'social_account_exists' ? 'signup.errors.emailExists' : code === 'jurisdiction_mismatch' ? 'social.errors.regionMismatch' : 'social.errors.signupFailed');
                    this.track('signup_error_view', { error_status: err?.status ?? 0, error_code: code ?? 'social_signup_failed' });
                }
            });
    }

    private track(name: string, properties: Record<string, unknown> = {}): void {
        this.tracking.trackEvent(name, {
            jurisdiction: this.jurisdiction(),
            plan: this.selectedPlan(),
            interval: this.selectedBilling(),
            source_path: '/signup/social/complete',
            auth_method: 'social',
            provider: this.preview()?.provider,
            ...properties
        });
    }

    private readCompletionToken(): string {
        const hash = this.document.defaultView?.location.hash.replace(/^#/, '') ?? '';
        return new URLSearchParams(hash).get('social_token')?.trim() ?? '';
    }

    private inferJurisdiction(): Jurisdiction {
        const hostname = this.document.defaultView?.location.hostname.toLowerCase() ?? '';
        return hostname === 'cloud.hitkeep.com' || hostname.endsWith('.hitkeep.com') ? 'US' : 'EU';
    }

    private normalizeJurisdiction(value: string | null): Jurisdiction | null {
        const normalized = value?.trim().toUpperCase();
        return normalized === 'EU' || normalized === 'US' ? normalized : null;
    }

    private normalizePlan(value: string | null): CloudPlanCode {
        const normalized = value?.trim().toLowerCase();
        return normalized === 'pro' || normalized === 'business' ? normalized : 'free';
    }

    private normalizeBilling(value: string | null): BillingInterval {
        return value?.trim().toLowerCase() === 'annual' ? 'annual' : 'monthly';
    }
}
