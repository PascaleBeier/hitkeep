import { DOCUMENT } from '@angular/common';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { VerifiedSignup } from '@pages/signup/verified-signup';
import { AnalyticsService } from '@services/analytics.service';
import { CloudSignupTrackingService } from '@services/cloud-signup-tracking.service';

describe('VerifiedSignup', () => {
    let queryParams: Record<string, string>;
    const analytics = { getSystemStatus: vi.fn(() => of({ cloud: { hosted: true } })) };
    const tracking = { install: vi.fn(), trackEvent: vi.fn() };
    const router = { navigateByUrl: vi.fn<(url: string) => Promise<boolean>>(() => Promise.resolve(true)) };

    beforeEach(async () => {
        vi.clearAllMocks();
        queryParams = {};
        await TestBed.configureTestingModule({
            imports: [VerifiedSignup, TranslocoTestingModule.forRoot({ langs: { en: {} }, translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }, preloadLangs: true })],
            providers: [
                { provide: AnalyticsService, useValue: analytics },
                { provide: CloudSignupTrackingService, useValue: tracking },
                { provide: Router, useValue: router },
                { provide: DOCUMENT, useValue: document },
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
            .overrideComponent(VerifiedSignup, { set: { imports: [], template: '<div></div>' } })
            .compileComponents();
    });

    function create(): ComponentFixture<VerifiedSignup> {
        const fixture = TestBed.createComponent(VerifiedSignup);
        fixture.detectChanges();
        return fixture;
    }

    it('keeps verified Business annual intent for explicit payment review', () => {
        queryParams = { plan: 'business', billing: 'annual' };
        const component = create().componentInstance;
        expect(router.navigateByUrl).not.toHaveBeenCalled();
        component['reviewPayment']();
        expect(router.navigateByUrl).toHaveBeenCalledWith('/admin/team?purchase=review&plan=business&billing=annual');
        expect(tracking.trackEvent).toHaveBeenCalledWith('signup_verified', expect.objectContaining({ plan: 'business', interval: 'annual' }));
    });

    it('does not offer Cloud payment on a self-hosted instance', () => {
        analytics.getSystemStatus.mockReturnValueOnce(of({ cloud: { hosted: false } }));
        const component = create().componentInstance;
        component['reviewPayment']();
        expect(router.navigateByUrl).toHaveBeenCalledWith('/dashboard');
        expect(tracking.install).not.toHaveBeenCalled();
    });

    it('lets a verified user continue on Free', () => {
        queryParams = { plan: 'pro', billing: 'monthly' };
        create().componentInstance['continueFree']();
        expect(router.navigateByUrl).toHaveBeenCalledWith('/dashboard');
    });
});
