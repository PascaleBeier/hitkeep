import { DOCUMENT } from '@angular/common';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { Observable, of, throwError } from 'rxjs';
import { vi } from 'vitest';

import { VerifiedSignup } from '@pages/signup/verified-signup';
import { CloudSignupTrackingService } from '@services/cloud-signup-tracking.service';
import { BillingPortalSessionResponse, CloudService } from '@services/cloud.service';

describe('VerifiedSignup', () => {
    let queryParams: Record<string, string>;
    let locationAssign: ReturnType<typeof vi.fn>;

    const cloud = {
        createBillingCheckoutSession: vi.fn<(request: unknown) => Observable<BillingPortalSessionResponse>>()
    };
    const tracking = {
        install: vi.fn(),
        trackEvent: vi.fn()
    };
    const router = {
        navigateByUrl: vi.fn<(url: string) => Promise<boolean>>(() => Promise.resolve(true))
    };

    beforeEach(async () => {
        vi.clearAllMocks();
        queryParams = {};
        locationAssign = vi.fn();
        const location = {
            hostname: 'cloud.hitkeep.eu',
            assign: locationAssign
        } as unknown as Location;
        const defaultView = new Proxy(window, {
            get(target, property) {
                if (property === 'location') return location;
                const value = Reflect.get(target, property, target);
                return typeof value === 'function' ? value.bind(target) : value;
            }
        }) as Window;
        const documentMock = new Proxy(document, {
            get(target, property) {
                if (property === 'defaultView') return defaultView;
                const value = Reflect.get(target, property, target);
                return typeof value === 'function' ? value.bind(target) : value;
            }
        }) as Document;

        await TestBed.configureTestingModule({
            imports: [
                VerifiedSignup,
                TranslocoTestingModule.forRoot({
                    langs: { en: {} },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                { provide: CloudService, useValue: cloud },
                { provide: CloudSignupTrackingService, useValue: tracking },
                { provide: Router, useValue: router },
                { provide: DOCUMENT, useValue: documentMock },
                {
                    provide: ActivatedRoute,
                    useValue: {
                        snapshot: {
                            get queryParamMap() {
                                return convertToParamMap(queryParams);
                            }
                        }
                    }
                }
            ]
        })
            .overrideComponent(VerifiedSignup, {
                set: { imports: [], template: '<div></div>' }
            })
            .compileComponents();
    });

    function create(): ComponentFixture<VerifiedSignup> {
        const fixture = TestBed.createComponent(VerifiedSignup);
        fixture.detectChanges();
        return fixture;
    }

    it('takes verified Business annual intent directly to checkout', () => {
        queryParams = { plan: 'business', billing: 'annual' };
        cloud.createBillingCheckoutSession.mockReturnValue(of({ url: 'https://checkout.stripe.com/c/pay/test' }));

        create();

        expect(cloud.createBillingCheckoutSession).toHaveBeenCalledWith({
            plan_code: 'business',
            billing: 'annual',
            locale: 'en'
        });
        expect(tracking.trackEvent).toHaveBeenCalledWith('signup_verified', {
            plan: 'business',
            interval: 'annual',
            jurisdiction: 'EU',
            source_path: '/signup/verified'
        });
        expect(tracking.trackEvent).toHaveBeenCalledWith('checkout_started', {
            plan: 'business',
            interval: 'annual',
            jurisdiction: 'EU',
            source_path: '/signup/verified'
        });
        expect(locationAssign).toHaveBeenCalledWith('https://checkout.stripe.com/c/pay/test');
    });

    it('keeps a continue-on-Free escape hatch when checkout fails', () => {
        queryParams = { plan: 'pro', billing: 'monthly' };
        cloud.createBillingCheckoutSession.mockReturnValue(throwError(() => new Error('checkout unavailable')));

        const fixture = create();
        const component = fixture.componentInstance;
        expect(component['checkoutFailed']()).toBe(true);

        component['continueFree']();

        expect(tracking.trackEvent).toHaveBeenCalledWith('continue_on_free', {
            plan: 'pro',
            interval: 'monthly',
            jurisdiction: 'EU',
            source_path: '/signup/verified'
        });
        expect(router.navigateByUrl).toHaveBeenCalledWith('/dashboard');
    });
});
