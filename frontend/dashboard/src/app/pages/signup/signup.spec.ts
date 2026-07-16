import { DOCUMENT } from '@angular/common';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { Router } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { NEVER, of } from 'rxjs';
import { vi } from 'vitest';

import { Signup } from '@pages/signup/signup';
import { AnalyticsService } from '@services/analytics.service';
import { AuthService } from '@services/auth.service';
import { CloudSignupTrackingService } from '@services/cloud-signup-tracking.service';
import { CloudService, CloudSignupResponse } from '@services/cloud.service';

describe('Signup', () => {
    let component: Signup;
    let fixture: ComponentFixture<Signup>;
    let queryParams: Record<string, string>;
    let locationAssignMock: ReturnType<typeof vi.fn>;
    let documentMock: Document;

    const cloudServiceMock: { signup: ReturnType<typeof vi.fn> } = {
        signup: vi.fn(() => NEVER)
    };

    const analyticsServiceMock = {
        getSystemStatus: vi.fn(() =>
            of({
                needs_setup: false,
                version: 'v2.0.0',
                cloud: {
                    hosted: true,
                    signup_enabled: true,
                    jurisdiction: 'EU'
                }
            })
        )
    };

    const routerMock = {
        navigate: vi.fn<(commands: readonly unknown[], extras: Record<string, unknown>) => Promise<boolean>>(() => Promise.resolve(true)),
        navigateByUrl: vi.fn(() => Promise.resolve(true))
    };
    const authServiceMock = {
        getSSOAvailability: vi.fn(() => of({ enabled: false })),
        getSocialProviders: vi.fn(() => of({ providers: [], signup_enabled: false })),
        startSocial: vi.fn<AuthService['startSocial']>(() => NEVER)
    };
    const signupTrackingMock = {
        install: vi.fn(),
        trackEvent: vi.fn()
    };

    beforeEach(async () => {
        vi.clearAllMocks();
        routerMock.navigateByUrl.mockClear();
        routerMock.navigate.mockClear();
        authServiceMock.getSSOAvailability.mockReturnValue(of({ enabled: false }));
        authServiceMock.getSocialProviders.mockReturnValue(of({ providers: [], signup_enabled: false }));
        signupTrackingMock.install.mockClear();
        signupTrackingMock.trackEvent.mockClear();
        locationAssignMock = vi.fn();
        const locationMock = {
            hostname: 'cloud.hitkeep.eu',
            assign: locationAssignMock
        } as unknown as Location;
        const defaultViewMock = new Proxy(window, {
            get(target, prop) {
                if (prop === 'location') {
                    return locationMock;
                }
                const value = Reflect.get(target, prop, target);
                return typeof value === 'function' ? value.bind(target) : value;
            }
        }) as Window;
        documentMock = new Proxy(document, {
            get(target, prop) {
                if (prop === 'defaultView') {
                    return defaultViewMock;
                }
                const value = Reflect.get(target, prop, target);
                return typeof value === 'function' ? value.bind(target) : value;
            }
        }) as Document;
        queryParams = {};

        await TestBed.configureTestingModule({
            imports: [
                Signup,
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
                { provide: Router, useValue: routerMock },
                { provide: AuthService, useValue: authServiceMock },
                { provide: CloudService, useValue: cloudServiceMock },
                { provide: AnalyticsService, useValue: analyticsServiceMock },
                { provide: CloudSignupTrackingService, useValue: signupTrackingMock },
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
            .overrideComponent(Signup, {
                set: {
                    imports: [],
                    template: '<div></div>'
                }
            })
            .compileComponents();

        fixture = TestBed.createComponent(Signup);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('shows the hosted jurisdiction from system status', () => {
        expect(component['currentJurisdiction']()).toBe('EU');
    });

    it('installs signup-only tracking for hosted cloud signup', () => {
        expect(signupTrackingMock.install).toHaveBeenCalledTimes(1);
    });

    it('tracks the signup page view when hosted signup is available', () => {
        expect(signupTrackingMock.trackEvent).toHaveBeenCalledWith('signup_page_view', {
            jurisdiction: 'EU',
            plan: 'free',
            interval: 'monthly',
            source_path: '/signup'
        });
    });

    it('submits a cloud signup request with free plan', () => {
        cloudServiceMock.signup.mockReturnValue(
            of({
                status: 'verification_sent',
                plan_code: 'free',
                billing: 'monthly'
            } as CloudSignupResponse)
        );

        component['signupForm'].teamName().control().setValue('Cloud Team');
        component['signupForm'].email().control().setValue('user@example.com');
        component['signupForm'].password().control().setValue('password123');
        component['signupForm'].acceptedTos().control().setValue(true);

        component['onSubmit']();

        const payload = (cloudServiceMock.signup.mock.calls[0]?.[0] ?? null) as Record<string, unknown> | null;
        expect(payload?.['team_name']).toBe('Cloud Team');
        expect(payload?.['email']).toBe('user@example.com');
        expect(payload?.['plan_code']).toBe('free');
        expect(payload?.['billing']).toBe('monthly');
        expect(payload?.['jurisdiction']).toBe('EU');
        expect(payload?.['locale']).toBe('en');
        expect(payload?.['accepted_tos']).toBe(true);
        expect(signupTrackingMock.trackEvent).toHaveBeenCalledWith('signup_started', {
            jurisdiction: 'EU',
            plan: 'free',
            interval: 'monthly',
            source_path: '/signup',
            auth_method: 'password'
        });
        expect(signupTrackingMock.trackEvent).toHaveBeenCalledWith('signup_completed_candidate', {
            jurisdiction: 'EU',
            plan: 'free',
            interval: 'monthly',
            source_path: '/signup',
            auth_method: 'password',
            response_status: 'verification_sent'
        });
    });

    it('hydrates and submits paid annual intent from query params', () => {
        queryParams = {
            plan: 'business',
            billing: 'annual'
        };
        cloudServiceMock.signup.mockReturnValue(
            of({
                status: 'verification_sent',
                plan_code: 'business',
                billing: 'annual'
            } as CloudSignupResponse)
        );

        fixture = TestBed.createComponent(Signup);
        component = fixture.componentInstance;
        fixture.detectChanges();
        component['signupForm'].teamName().control().setValue('Agency Team');
        component['signupForm'].email().control().setValue('agency@example.com');
        component['signupForm'].password().control().setValue('password123');
        component['signupForm'].acceptedTos().control().setValue(true);

        component['onSubmit']();

        const payload = cloudServiceMock.signup.mock.calls.at(-1)?.[0] as Record<string, unknown> | undefined;
        expect(payload?.['plan_code']).toBe('business');
        expect(payload?.['billing']).toBe('annual');
        expect(signupTrackingMock.trackEvent.mock.calls.some(([name, properties]) => name === 'signup_started' && properties?.plan === 'business' && properties?.interval === 'annual')).toBe(true);
    });

    it('starts social signup after region selection and records provider-only growth metadata', () => {
        authServiceMock.startSocial.mockReturnValue(of({ auth_url: 'https://accounts.example.com/authorize' }));
        component['socialProviders'].set([{ id: 'github', display_name: 'GitHub' }]);
        component['selectedPlan'].set('pro');
        component['selectedBilling'].set('annual');

        component['selectSocialMethod']('github');

        expect(authServiceMock.startSocial).toHaveBeenCalledWith('github', {
            flow: 'signup',
            return_url: '/signup/social/complete?plan=pro&billing=annual&region=EU'
        });
        expect(signupTrackingMock.trackEvent).toHaveBeenCalledWith('signup_started', {
            jurisdiction: 'EU',
            plan: 'pro',
            interval: 'annual',
            source_path: '/signup',
            auth_method: 'social',
            provider: 'github'
        });
        expect(locationAssignMock).toHaveBeenCalledWith('https://accounts.example.com/authorize');
    });

    it('maps configured social signup providers to branded marks', () => {
        component['socialProviders'].set([
            { id: 'google', display_name: 'Google' },
            { id: 'github', display_name: 'GitHub' },
            { id: 'microsoft', display_name: 'Microsoft' }
        ]);

        expect(component['socialMethods']().map((method) => method.providerIcon)).toEqual(['google', 'github', 'microsoft']);
    });

    it('does not discover or offer enterprise SSO from public signup', () => {
        expect(authServiceMock.getSSOAvailability).not.toHaveBeenCalled();
    });

    it('uses the scoped PrimeNG select-button design tokens', () => {
        expect(component['jurisdictionFieldsetDesignTokens'].root?.background).toBe('{content.hover.background}');
        expect(component['jurisdictionDesignTokens'].root?.borderRadius).toBe('{border.radius.xl}');
    });

    it('hydrates team name and email from query params', async () => {
        queryParams = {
            team_name: 'Analytical Engine',
            email: 'ada@example.com'
        };

        fixture = TestBed.createComponent(Signup);
        component = fixture.componentInstance;
        fixture.detectChanges();

        expect(component['signupForm'].teamName().value()).toBe('Analytical Engine');
        expect(component['signupForm'].email().value()).toBe('ada@example.com');
    });

    it('shows stable provider cancellation feedback after returning to signup', () => {
        queryParams = { error: 'social_provider_cancelled' };

        fixture = TestBed.createComponent(Signup);
        component = fixture.componentInstance;
        fixture.detectChanges();

        expect(component['errorMessage']()).toBe('social.errors.cancelled');
    });

    it('builds a jurisdiction URL for the alternate region with safe funnel context', () => {
        queryParams = {
            region: 'EU',
            utm_source: 'hitkeep_docs',
            utm_campaign: 'agency_pilot',
            plan: 'business',
            billing: 'annual',
            error: 'exists'
        };
        fixture = TestBed.createComponent(Signup);
        component = fixture.componentInstance;
        fixture.detectChanges();

        component['signupForm'].teamName().control().setValue('Cloud Team');
        component['signupForm'].email().control().setValue('user@example.com');
        component['signupForm'].password().control().setValue('password123');
        component['signupForm'].acceptedTos().control().setValue(true);

        const href = component['signupUrlForJurisdiction']('US');
        expect(href).toContain('https://cloud.hitkeep.com/signup');
        expect(href).toContain('team_name=Cloud+Team');
        expect(href).toContain('email=user%40example.com');
        expect(href).toContain('utm_source=hitkeep_docs');
        expect(href).toContain('utm_campaign=agency_pilot');
        expect(href).toContain('plan=business');
        expect(href).toContain('billing=annual');
        expect(href).not.toContain('region=');
        expect(href).not.toContain('error=');
        expect(href).not.toContain('password123');
    });

    it('does nothing when the selected jurisdiction is already active', () => {
        const redirectSpy = vi.spyOn(
            component as unknown as {
                redirectToJurisdiction: (jurisdiction: 'EU' | 'US') => void;
            },
            'redirectToJurisdiction'
        );

        component['selectJurisdiction']('EU');

        expect(redirectSpy).not.toHaveBeenCalled();
    });

    it('tracks and redirects when selecting the alternate region', () => {
        const redirectSpy = vi
            .spyOn(
                component as unknown as {
                    redirectToJurisdiction: (jurisdiction: 'EU' | 'US') => void;
                },
                'redirectToJurisdiction'
            )
            .mockImplementation(() => undefined);

        component['selectJurisdiction']('US');

        expect(signupTrackingMock.trackEvent).toHaveBeenCalledWith('cloud_region_switch_click', {
            jurisdiction: 'EU',
            plan: 'free',
            interval: 'monthly',
            source_path: '/signup',
            target_jurisdiction: 'US'
        });
        expect(redirectSpy).toHaveBeenCalledWith('US');
    });

    it('redirects a region query parameter to the matching regional signup host', () => {
        queryParams = {
            region: 'US',
            utm_source: 'hitkeep_docs',
            email: 'ada@example.com',
            team_name: 'Analytical Engine'
        };
        locationAssignMock.mockClear();

        fixture = TestBed.createComponent(Signup);
        component = fixture.componentInstance;
        fixture.detectChanges();

        expect(locationAssignMock).toHaveBeenCalledTimes(1);
        const target = locationAssignMock.mock.calls[0]?.[0] as string;
        expect(target).toContain('https://cloud.hitkeep.com/signup');
        expect(target).toContain('utm_source=hitkeep_docs');
        expect(target).toContain('email=ada%40example.com');
        expect(target).toContain('team_name=Analytical+Engine');
        expect(target).not.toContain('region=');
    });

    it('ignores invalid region query parameters', () => {
        queryParams = {
            region: 'APAC',
            utm_source: 'hitkeep_docs'
        };
        locationAssignMock.mockClear();

        fixture = TestBed.createComponent(Signup);
        component = fixture.componentInstance;
        fixture.detectChanges();

        expect(locationAssignMock).not.toHaveBeenCalled();
    });
});
