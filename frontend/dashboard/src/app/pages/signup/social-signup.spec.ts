import { DOCUMENT } from '@angular/common';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { SocialSignup } from '@pages/signup/social-signup';
import { AuthService } from '@services/auth.service';
import { CloudSignupTrackingService } from '@services/cloud-signup-tracking.service';

describe('SocialSignup', () => {
    let fixture: ComponentFixture<SocialSignup>;
    let component: SocialSignup;

    const authMock = {
        previewSocial: vi.fn<AuthService['previewSocial']>(() =>
            of({
                provider: 'google' as const,
                display_name: 'Google',
                observed_email: 'user@example.com',
                email_verified: true,
                email_confirmation_required: false,
                flow: 'signup' as const
            })
        ),
        completeSocialSignup: vi.fn<AuthService['completeSocialSignup']>(() => of({ status: 'ok' as const, plan_code: 'pro', billing: 'annual', redirect_url: '/signup/verified?plan=pro&billing=annual' }))
    };
    const trackingMock = { install: vi.fn(), trackEvent: vi.fn() };
    const routerMock = { navigateByUrl: vi.fn<(url: string) => Promise<boolean>>(() => Promise.resolve(true)) };

    beforeEach(async () => {
        vi.clearAllMocks();
        const locationMock = {
            hash: '#social_token=completion-token',
            hostname: 'cloud.hitkeep.eu'
        } as Location;
        const documentMock = new Proxy(document, {
            get(target, prop) {
                if (prop === 'defaultView') {
                    return { location: locationMock } as Window;
                }
                const value = Reflect.get(target, prop, target);
                return typeof value === 'function' ? value.bind(target) : value;
            }
        }) as Document;

        await TestBed.configureTestingModule({
            imports: [
                SocialSignup,
                TranslocoTestingModule.forRoot({
                    langs: { en: {} },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ],
            providers: [
                { provide: AuthService, useValue: authMock },
                { provide: CloudSignupTrackingService, useValue: trackingMock },
                { provide: Router, useValue: routerMock },
                { provide: DOCUMENT, useValue: documentMock },
                {
                    provide: ActivatedRoute,
                    useValue: {
                        snapshot: {
                            queryParamMap: convertToParamMap({ plan: 'pro', billing: 'annual', region: 'EU' })
                        }
                    }
                }
            ]
        })
            .overrideComponent(SocialSignup, { set: { imports: [], template: '<div></div>' } })
            .compileComponents();

        fixture = TestBed.createComponent(SocialSignup);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('previews the one-time completion and keeps provider email read-only in the payload', () => {
        expect(authMock.previewSocial).toHaveBeenCalledWith('completion-token');
        expect(component['preview']()?.provider).toBe('google');
        expect(component['jurisdiction']()).toBe('EU');

        component['form'].teamName().control().setValue('Social Team');
        component['form'].acceptedTos().control().setValue(true);
        component['onSubmit']();

        expect(authMock.completeSocialSignup).toHaveBeenCalledWith({
            completion_token: 'completion-token',
            email: undefined,
            team_name: 'Social Team',
            plan_code: 'pro',
            billing: 'annual',
            jurisdiction: 'EU',
            locale: 'en',
            accepted_tos: true
        });
        const signupStarted = trackingMock.trackEvent.mock.calls.find(([name]) => name === 'signup_started');
        const signupProperties = signupStarted?.[1] as Record<string, unknown> | undefined;
        expect(signupProperties?.['auth_method']).toBe('social');
        expect(signupProperties?.['provider']).toBe('google');
        expect(routerMock.navigateByUrl).toHaveBeenCalledWith('/signup/verified?plan=pro&billing=annual');
    });
});
