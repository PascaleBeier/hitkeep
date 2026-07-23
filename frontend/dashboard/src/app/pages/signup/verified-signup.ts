import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, DestroyRef, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageModule } from '@openng/optimus-ui/message';

import { Brand } from '@components/brand/brand';
import { AuthCard } from '@core/components/auth-card/auth-card';
import { injectActiveLang } from '@core/i18n/active-lang';
import { BillingInterval, CloudService } from '@services/cloud.service';
import { CloudSignupTrackingService } from '@services/cloud-signup-tracking.service';

type PaidPlan = 'pro' | 'business';

@Component({
    selector: 'app-verified-signup',
    imports: [AuthCard, Brand, ButtonModule, MessageModule, TranslocoPipe],
    template: `
        <main class="hk-auth-page">
            <section class="hk-auth-shell" aria-labelledby="verified-signup-title">
                <app-auth-card>
                    <header class="hk-auth-header">
                        <app-brand size="large" />
                        <h1 id="verified-signup-title" class="hk-auth-title">
                            {{ 'signup.verified.title' | transloco }}
                        </h1>
                        <p class="hk-auth-subtitle">
                            {{
                                'signup.verified.subtitle'
                                    | transloco
                                        : {
                                              plan: 'signup.plans.' + plan() + '.name' | transloco,
                                              billing: 'signup.billing.' + billing() | transloco
                                          }
                            }}
                        </p>
                    </header>

                    <div class="hk-auth-form" aria-live="polite">
                        @if (checkoutFailed()) {
                            <p-message severity="error">{{ 'signup.verified.checkoutFailed' | transloco }}</p-message>
                            <p-button [label]="'signup.verified.retryCheckout' | transloco" [loading]="checkoutPending()" [fluid]="true" (onClick)="startCheckout()" />
                        } @else {
                            <div class="flex items-center justify-center gap-3 rounded-lg border border-primary/20 bg-primary/5 px-4 py-4 text-sm">
                                <i class="pi pi-spin pi-spinner text-primary" aria-hidden="true"></i>
                                <span>{{ 'signup.verified.openingCheckout' | transloco }}</span>
                            </div>
                        }

                        <p-button [label]="'signup.verified.continueFree' | transloco" severity="secondary" [text]="true" [fluid]="true" (onClick)="continueFree()" />
                        <p class="m-0 text-center text-xs text-[var(--p-text-muted-color)]">
                            {{ 'signup.verified.freeNote' | transloco }}
                        </p>
                    </div>
                </app-auth-card>
            </section>
        </main>
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class VerifiedSignup {
    private readonly document = inject(DOCUMENT);
    private readonly route = inject(ActivatedRoute);
    private readonly router = inject(Router);
    private readonly destroyRef = inject(DestroyRef);
    private readonly cloud = inject(CloudService);
    private readonly tracking = inject(CloudSignupTrackingService);
    private readonly activeLanguage = injectActiveLang();

    protected readonly plan = signal<PaidPlan>(this.normalizePlan(this.route.snapshot.queryParamMap.get('plan')));
    protected readonly billing = signal<BillingInterval>(this.normalizeBilling(this.route.snapshot.queryParamMap.get('billing')));
    protected readonly checkoutPending = signal(false);
    protected readonly checkoutFailed = signal(false);

    constructor() {
        this.tracking.install();
        this.track('signup_verified');
        this.startCheckout();
    }

    protected startCheckout(): void {
        if (this.checkoutPending()) {
            return;
        }
        this.checkoutPending.set(true);
        this.checkoutFailed.set(false);
        this.cloud
            .createBillingCheckoutSession({
                plan_code: this.plan(),
                billing: this.billing(),
                locale: this.activeLanguage()
            })
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: ({ url }) => {
                    this.track('checkout_started');
                    this.document.defaultView?.location.assign(url);
                },
                error: () => {
                    this.checkoutPending.set(false);
                    this.checkoutFailed.set(true);
                }
            });
    }

    protected continueFree(): void {
        this.track('continue_on_free');
        void this.router.navigateByUrl('/dashboard');
    }

    private track(name: string): void {
        this.tracking.trackEvent(name, {
            plan: this.plan(),
            interval: this.billing(),
            jurisdiction: this.inferJurisdiction(),
            source_path: '/signup/verified'
        });
    }

    private normalizePlan(value: string | null): PaidPlan {
        return value?.trim().toLowerCase() === 'business' ? 'business' : 'pro';
    }

    private normalizeBilling(value: string | null): BillingInterval {
        return value?.trim().toLowerCase() === 'annual' ? 'annual' : 'monthly';
    }

    private inferJurisdiction(): 'EU' | 'US' {
        const hostname = this.document.defaultView?.location.hostname.toLowerCase() ?? '';
        return hostname === 'cloud.hitkeep.com' || hostname.endsWith('.hitkeep.com') ? 'US' : 'EU';
    }
}
