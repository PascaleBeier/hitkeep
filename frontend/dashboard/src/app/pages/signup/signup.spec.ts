import { DOCUMENT } from '@angular/common';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { Router } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { NEVER, of } from 'rxjs';
import { vi } from 'vitest';

import { Signup } from '@pages/signup/signup';
import { AnalyticsService } from '@services/analytics.service';
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
        navigateByUrl: vi.fn(() => Promise.resolve(true))
    };
    const signupTrackingMock = {
        install: vi.fn(),
        trackEvent: vi.fn()
    };

    beforeEach(async () => {
        vi.clearAllMocks();
        routerMock.navigateByUrl.mockClear();
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
            plan_code: 'free',
            source_path: '/signup'
        });
    });

    it('submits a cloud signup request with free plan', () => {
        cloudServiceMock.signup.mockReturnValue(of({ status: 'verification_sent', plan_code: 'free' } as CloudSignupResponse));

        component['signupForm'].teamName().control().setValue('Cloud Team');
        component['signupForm'].email().control().setValue('user@example.com');
        component['signupForm'].password().control().setValue('password123');
        component['signupForm'].acceptedTos().control().setValue(true);

        component['onSubmit']();

        const payload = (cloudServiceMock.signup.mock.calls[0]?.[0] ?? null) as Record<string, unknown> | null;
        expect(payload?.['team_name']).toBe('Cloud Team');
        expect(payload?.['email']).toBe('user@example.com');
        expect(payload?.['plan_code']).toBe('free');
        expect(payload?.['jurisdiction']).toBe('EU');
        expect(payload?.['locale']).toBe('en');
        expect(payload?.['accepted_tos']).toBe(true);
        expect(signupTrackingMock.trackEvent).toHaveBeenCalledWith('signup_started', {
            jurisdiction: 'EU',
            plan_code: 'free',
            source_path: '/signup'
        });
        expect(signupTrackingMock.trackEvent).toHaveBeenCalledWith('signup_completed_candidate', {
            jurisdiction: 'EU',
            plan_code: 'free',
            source_path: '/signup',
            response_status: 'verification_sent'
        });
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

    it('builds a jurisdiction URL for the alternate region with safe funnel context', () => {
        queryParams = {
            region: 'EU',
            utm_source: 'hitkeep_docs',
            utm_campaign: 'agency_pilot',
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
        expect(href).not.toContain('region=');
        expect(href).not.toContain('error=');
        expect(href).not.toContain('password123');
    });

    it('does nothing when the selected jurisdiction is already active', () => {
        const redirectSpy = vi.spyOn(component as unknown as { redirectToJurisdiction: (jurisdiction: 'EU' | 'US') => void }, 'redirectToJurisdiction');

        component['selectJurisdiction']('EU');

        expect(redirectSpy).not.toHaveBeenCalled();
    });

    it('tracks and redirects when selecting the alternate region', () => {
        const redirectSpy = vi.spyOn(component as unknown as { redirectToJurisdiction: (jurisdiction: 'EU' | 'US') => void }, 'redirectToJurisdiction').mockImplementation(() => undefined);

        component['selectJurisdiction']('US');

        expect(signupTrackingMock.trackEvent).toHaveBeenCalledWith('cloud_region_switch_click', {
            jurisdiction: 'EU',
            plan_code: 'free',
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
