import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { FormControl, FormsModule, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { finalize } from 'rxjs/operators';
import { TranslocoPipe } from '@jsverse/transloco';

import { PasswordModule } from 'primeng/password';
import { ButtonModule } from 'primeng/button';
import { InputTextModule } from 'primeng/inputtext';
import { CheckboxModule } from 'primeng/checkbox';
import { FieldsetModule } from 'primeng/fieldset';
import { MessageModule } from 'primeng/message';
import { SelectButtonModule } from 'primeng/selectbutton';

import { AuthCard } from '@core/components/auth-card/auth-card';
import { AuthDivider } from '@core/components/auth-divider/auth-divider';
import { AuthMethodOption, AuthMethods } from '@core/components/auth-methods/auth-methods';
import { Brand } from '@components/brand/brand';
import { injectActiveLang } from '@core/i18n/active-lang';
import { CloudStatus } from '@models/analytics.types';
import { AnalyticsService } from '@services/analytics.service';
import { CloudSignupTrackingService } from '@services/cloud-signup-tracking.service';
import { BillingInterval, CloudPlanCode, CloudService, CloudSignupRequest } from '@services/cloud.service';
import { AUTH_FIELDSET_DESIGN_TOKENS, AUTH_SELECT_BUTTON_DESIGN_TOKENS } from '@core/theme/hitkeep-preset';
import { AuthService, SocialProvider, SocialProviderID } from '@services/auth.service';

type Jurisdiction = 'EU' | 'US';

@Component({
    selector: 'app-signup',
    imports: [AuthCard, AuthDivider, AuthMethods, Brand, FormsModule, ReactiveFormsModule, PasswordModule, ButtonModule, InputTextModule, CheckboxModule, FieldsetModule, MessageModule, SelectButtonModule, RouterLink, TranslocoPipe],
    templateUrl: './signup.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class Signup {
    private static readonly REGION_BASE_URLS: Record<Jurisdiction, string> = {
        EU: 'https://cloud.hitkeep.eu',
        US: 'https://cloud.hitkeep.com'
    };
    private static readonly PRESERVED_QUERY_PARAMS = new Set(['ref', 'source', 'plan', 'billing', 'utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content', 'utm_id', 'utm_source_platform', 'utm_creative_format', 'utm_marketing_tactic']);

    private readonly document = inject(DOCUMENT);
    private readonly destroyRef = inject(DestroyRef);
    private readonly router = inject(Router);
    private readonly route = inject(ActivatedRoute);
    private readonly analytics = inject(AnalyticsService);
    private readonly auth = inject(AuthService);
    private readonly cloud = inject(CloudService);
    private readonly signupTracking = inject(CloudSignupTrackingService);

    protected readonly isLoading = signal(false);
    protected readonly socialLoading = signal<SocialProviderID | null>(null);
    protected readonly socialProviders = signal<readonly SocialProvider[]>([]);
    protected readonly errorMessage = signal<string | null>(null);
    protected readonly cloudStatus = signal<CloudStatus | null>(null);
    protected readonly verificationSent = signal(false);
    protected readonly submittedEmail = signal('');
    protected readonly selectedPlan = signal<CloudPlanCode>('free');
    protected readonly selectedBilling = signal<BillingInterval>('monthly');
    private readonly activeLanguage = injectActiveLang();
    protected readonly currentYear = new Date().getFullYear();
    protected readonly jurisdictionOptions: Jurisdiction[] = ['EU', 'US'];
    protected readonly jurisdictionFieldsetDesignTokens = AUTH_FIELDSET_DESIGN_TOKENS;
    protected readonly jurisdictionDesignTokens = AUTH_SELECT_BUTTON_DESIGN_TOKENS;
    protected readonly currentJurisdiction = computed<Jurisdiction>(() => this.normalizeJurisdiction(this.cloudStatus()?.jurisdiction) ?? this.inferJurisdictionFromHost());
    protected readonly socialMethods = computed<readonly AuthMethodOption[]>(() =>
        this.socialProviders().map((provider) => ({
            id: provider.id,
            labelKey: `social.continueWith.${provider.id}`,
            icon: provider.id === 'github' ? 'pi pi-github' : provider.id === 'google' ? 'pi pi-google' : 'pi pi-microsoft',
            wide: true,
            loading: this.socialLoading() === provider.id,
            disabled: this.isLoading() || this.socialLoading() !== null
        }))
    );
    private trackedSignupPageView = false;
    private trackedInitialSignupError = false;

    private readonly signupModel = signal({
        email: new FormControl('', {
            nonNullable: true,
            validators: [Validators.required, Validators.email]
        }),
        password: new FormControl('', {
            nonNullable: true,
            validators: [Validators.required, Validators.minLength(8)]
        }),
        teamName: new FormControl('', {
            nonNullable: true,
            validators: [Validators.required, Validators.maxLength(120)]
        }),
        acceptedTos: new FormControl(false, {
            nonNullable: true,
            validators: [Validators.requiredTrue]
        })
    });
    protected readonly signupForm = compatForm(this.signupModel);

    constructor() {
        this.hydrateFromQuery();
        this.redirectToRequestedJurisdiction();
        this.analytics
            .getSystemStatus()
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (status) => {
                    this.cloudStatus.set(status.cloud ?? null);
                    if (status.cloud?.hosted && status.cloud.signup_enabled) {
                        this.signupTracking.install();
                        this.trackSignupPageView();
                        this.trackInitialSignupError();
                    }
                },
                error: (err) => {
                    console.error('Failed to load cloud status for signup', err);
                }
            });
        this.auth
            .getSocialProviders()
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (response) => this.socialProviders.set(response.signup_enabled ? response.providers : []),
                error: () => this.socialProviders.set([])
            });
    }

    protected selectJurisdiction(jurisdiction: Jurisdiction): void {
        if (jurisdiction === this.currentJurisdiction()) {
            return;
        }

        this.trackRegionSwitchClick(jurisdiction);
        this.redirectToJurisdiction(jurisdiction);
    }

    protected onSubmit(event?: Event): void {
        event?.preventDefault();
        if (this.signupForm().invalid()) {
            this.signupForm.email().markAsTouched();
            this.signupForm.password().markAsTouched();
            this.signupForm.teamName().markAsTouched();
            this.signupForm.acceptedTos().markAsTouched();
            return;
        }

        const payload: CloudSignupRequest = {
            email: this.signupForm.email().value().trim().toLowerCase(),
            password: this.signupForm.password().value(),
            team_name: this.signupForm.teamName().value().trim(),
            plan_code: this.selectedPlan(),
            billing: this.selectedBilling(),
            jurisdiction: this.currentJurisdiction(),
            locale: this.activeLanguage(),
            accepted_tos: true
        };

        this.isLoading.set(true);
        this.errorMessage.set(null);
        this.trackSignupEvent('signup_started', { auth_method: 'password' });
        this.cloud
            .signup(payload)
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: (response) => {
                    this.trackSignupEvent('signup_completed_candidate', {
                        auth_method: 'password',
                        response_status: response.status
                    });
                    if (response.status === 'verification_sent') {
                        this.submittedEmail.set(payload.email);
                        this.verificationSent.set(true);
                        return;
                    }
                    const target = response.redirect_url?.trim() || '/dashboard';
                    void this.router.navigateByUrl(target);
                },
                error: (err) => {
                    console.error('Cloud signup failed', err);
                    if (err.status === 409) {
                        this.errorMessage.set('signup.errors.emailExists');
                        this.trackSignupEvent('signup_error_view', {
                            auth_method: 'password',
                            error_status: 409,
                            error_code: 'email_exists'
                        });
                        return;
                    }
                    this.errorMessage.set('signup.errors.unexpected');
                    this.trackSignupEvent('signup_error_view', {
                        auth_method: 'password',
                        error_status: err.status ?? 0,
                        error_code: 'unexpected'
                    });
                }
            });
    }

    protected selectSocialMethod(method: string): void {
        if (!this.socialProviders().some((provider) => provider.id === method)) return;
        const provider = method as SocialProviderID;
        this.socialLoading.set(provider);
        this.errorMessage.set(null);
        this.trackSignupEvent('signup_started', { auth_method: 'social', provider });
        const query = new URLSearchParams({
            plan: this.selectedPlan(),
            billing: this.selectedBilling(),
            region: this.currentJurisdiction()
        });
        this.auth
            .startSocial(provider, {
                flow: 'signup',
                return_url: `/signup/social/complete?${query.toString()}`
            })
            .pipe(finalize(() => this.socialLoading.set(null)))
            .subscribe({
                next: (response) => this.document.defaultView?.location.assign(response.auth_url),
                error: (err) => {
                    this.errorMessage.set(this.socialErrorKey(err?.error?.code));
                    this.trackSignupEvent('signup_error_view', {
                        auth_method: 'social',
                        provider,
                        error_code: err?.error?.code ?? 'social_start_failed'
                    });
                }
            });
    }

    private signupUrlForJurisdiction(jurisdiction: Jurisdiction): string {
        const url = new URL('/signup', Signup.REGION_BASE_URLS[jurisdiction]);

        this.appendPreservedQueryParams(url);
        const email = this.signupForm.email().value().trim().toLowerCase();
        const teamName = this.signupForm.teamName().value().trim();
        if (email !== '') {
            url.searchParams.set('email', email);
        }
        if (teamName !== '') {
            url.searchParams.set('team_name', teamName);
        }

        return url.toString();
    }

    protected trackRegionSwitchClick(jurisdiction: Jurisdiction): void {
        this.trackSignupEvent('cloud_region_switch_click', {
            target_jurisdiction: jurisdiction
        });
    }

    private hydrateFromQuery(): void {
        const params = this.route.snapshot.queryParamMap;

        this.selectedPlan.set(this.normalizePlan(params.get('plan')));
        this.selectedBilling.set(this.selectedPlan() === 'free' ? 'monthly' : this.normalizeBilling(params.get('billing')));

        const teamName = params.get('team_name')?.trim();
        if (teamName) {
            this.signupForm.teamName().control().setValue(teamName);
        }

        const email = params.get('email')?.trim().toLowerCase();
        if (email) {
            this.signupForm.email().control().setValue(email);
        }

        const errorParam = params.get('error');
        if (errorParam === 'expired') {
            this.errorMessage.set('signup.errors.verificationExpired');
        } else if (errorParam === 'exists') {
            this.errorMessage.set('signup.errors.emailExists');
        } else if (errorParam?.startsWith('social_')) {
            this.errorMessage.set(this.socialErrorKey(errorParam));
        }
    }

    private redirectToRequestedJurisdiction(): void {
        const requested = this.normalizeJurisdiction(this.route.snapshot.queryParamMap.get('region'));
        if (!requested || requested === this.inferJurisdictionFromHost()) {
            return;
        }

        this.redirectToJurisdiction(requested);
    }

    private redirectToJurisdiction(jurisdiction: Jurisdiction): void {
        this.document.defaultView?.location.assign(this.signupUrlForJurisdiction(jurisdiction));
    }

    private appendPreservedQueryParams(url: URL): void {
        const params = this.route.snapshot.queryParamMap;
        for (const key of params.keys) {
            if (!this.shouldPreserveQueryParam(key)) {
                continue;
            }

            for (const value of params.getAll(key)) {
                if (value.trim() !== '') {
                    url.searchParams.append(key, value);
                }
            }
        }
    }

    private shouldPreserveQueryParam(key: string): boolean {
        const normalized = key.trim().toLowerCase();
        return normalized.startsWith('utm_') || Signup.PRESERVED_QUERY_PARAMS.has(normalized);
    }

    private trackSignupPageView(): void {
        if (this.trackedSignupPageView) {
            return;
        }
        this.trackedSignupPageView = true;
        this.trackSignupEvent('signup_page_view');
    }

    private trackInitialSignupError(): void {
        if (this.trackedInitialSignupError || !this.errorMessage()) {
            return;
        }
        this.trackedInitialSignupError = true;
        this.trackSignupEvent('signup_error_view', {
            error_code: 'verification_redirect'
        });
    }

    private trackSignupEvent(name: string, properties: Record<string, unknown> = {}): void {
        this.signupTracking.trackEvent(name, {
            jurisdiction: this.currentJurisdiction(),
            plan: this.selectedPlan(),
            interval: this.selectedBilling(),
            source_path: '/signup',
            ...properties
        });
    }

    private inferJurisdictionFromHost(hostname?: string): Jurisdiction {
        const value = (hostname ?? this.document.defaultView?.location.hostname ?? '').trim().toLowerCase();
        if (value === 'cloud.hitkeep.com' || value.endsWith('.hitkeep.com')) {
            return 'US';
        }
        return 'EU';
    }

    private normalizeJurisdiction(value: string | null | undefined): Jurisdiction | null {
        const normalized = value?.trim().toUpperCase();
        if (normalized === 'EU' || normalized === 'US') {
            return normalized;
        }
        return null;
    }

    private normalizePlan(value: string | null | undefined): CloudPlanCode {
        const normalized = value?.trim().toLowerCase();
        return normalized === 'pro' || normalized === 'business' ? normalized : 'free';
    }

    private normalizeBilling(value: string | null | undefined): BillingInterval {
        return value?.trim().toLowerCase() === 'annual' ? 'annual' : 'monthly';
    }

    private socialErrorKey(code?: string): string {
        if (code === 'social_provider_unavailable') return 'social.errors.unavailable';
        if (code === 'social_email_unverified') return 'social.errors.emailUnverified';
        if (code === 'social_provider_cancelled') return 'social.errors.cancelled';
        if (code === 'social_identity_conflict') return 'social.errors.identityConflict';
        return 'social.errors.failed';
    }
}
