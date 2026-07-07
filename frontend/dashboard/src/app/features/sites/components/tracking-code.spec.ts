import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { SiteService } from '@features/sites/services/site.service';
import { SiteTrackingSettings } from './site-tracking-settings';

describe('SiteTrackingSettings', () => {
    let component: SiteTrackingSettings;
    let fixture: ComponentFixture<SiteTrackingSettings>;
    let base: HTMLBaseElement;
    let previousBases: HTMLBaseElement[];
    let siteServiceMock: {
        getTrackingStatus: ReturnType<typeof vi.fn>;
        getTrackingDomainOptions: ReturnType<typeof vi.fn>;
    };

    beforeEach(async () => {
        previousBases = Array.from(window.document.head.querySelectorAll('base'));
        previousBases.forEach((entry) => entry.remove());
        base = window.document.createElement('base');
        base.href = '/hitkeep/';
        window.document.head.append(base);
        siteServiceMock = {
            getTrackingStatus: vi.fn(() =>
                of({
                    site_id: 'site-1',
                    tenant_id: 'team-1',
                    status: 'waiting',
                    configured_domain: 'example.com'
                })
            ),
            getTrackingDomainOptions: vi.fn(() =>
                of({
                    site_id: 'site-1',
                    team_id: 'team-1',
                    default_url: 'https://hitkeep.test/hk.js',
                    domains: []
                })
            )
        };

        await TestBed.configureTestingModule({
            imports: [
                SiteTrackingSettings,
                TranslocoTestingModule.forRoot({
                    langs: { en: {} },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [{ provide: SiteService, useValue: siteServiceMock }]
        }).compileComponents();

        fixture = TestBed.createComponent(SiteTrackingSettings);
        fixture.componentRef.setInput('site', null);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });
    afterEach(() => {
        base.remove();
        previousBases.forEach((entry) => window.document.head.append(entry));
    });
    it('should create', () => {
        expect(component).toBeTruthy();
    });
    it('should update snippet when toggles change', () => {
        const internals = component as SiteTrackingSettings & {
            snippetCode: () => string;
            trackingForm: {
                collectDnt: () => { control: () => { setValue: (value: boolean) => void } };
                disableBeacon: () => { control: () => { setValue: (value: boolean) => void } };
                enableWebVitals: () => { control: () => { setValue: (value: boolean) => void } };
                trackOutbound: () => { control: () => { setValue: (value: boolean) => void } };
                trackDownloads: () => { control: () => { setValue: (value: boolean) => void } };
                trackForms: () => { control: () => { setValue: (value: boolean) => void } };
            };
        };
        const getSnippet = () => internals.snippetCode();

        expect(getSnippet()).toContain('/hitkeep/hk.js');
        expect(getSnippet()).not.toContain('data-collect-dnt');
        expect(getSnippet()).not.toContain('data-disable-beacon');
        expect(getSnippet()).not.toContain('data-enable-web-vitals');
        expect(getSnippet()).not.toContain('data-disable-outbound-tracking');
        expect(getSnippet()).not.toContain('data-disable-download-tracking');
        expect(getSnippet()).not.toContain('data-disable-form-tracking');

        internals.trackingForm.collectDnt().control().setValue(true);
        internals.trackingForm.enableWebVitals().control().setValue(true);
        fixture.detectChanges();
        expect(getSnippet()).toContain('data-collect-dnt="true"');
        expect(getSnippet()).toContain('data-enable-web-vitals="true"');

        internals.trackingForm.disableBeacon().control().setValue(true);
        internals.trackingForm.trackOutbound().control().setValue(false);
        internals.trackingForm.trackDownloads().control().setValue(false);
        internals.trackingForm.trackForms().control().setValue(false);
        fixture.detectChanges();
        expect(getSnippet()).toContain('hk.js');
        expect(getSnippet()).toContain('data-disable-beacon="true"');
        expect(getSnippet()).toContain('data-disable-outbound-tracking="true"');
        expect(getSnippet()).toContain('data-disable-download-tracking="true"');
        expect(getSnippet()).toContain('data-disable-form-tracking="true"');
    });

    it('should use a selected custom tracking domain in the snippet', () => {
        siteServiceMock.getTrackingDomainOptions.mockReturnValue(
            of({
                site_id: 'site-1',
                team_id: 'team-1',
                default_url: 'https://hitkeep.test/hk.js',
                domains: [
                    {
                        id: 'domain-1',
                        team_id: 'team-1',
                        hostname: 'analytics.example.com',
                        verification_status: 'verified',
                        target_status: 'verified',
                        tls_mode: 'external',
                        tls_status: 'verified',
                        enabled: true,
                        active: true,
                        dns_txt_name: '_hitkeep-tracking.analytics.example.com',
                        dns_txt_value: 'hitkeep-domain-verification=token',
                        dns_target: 'hitkeep.test',
                        created_at: '2026-01-01T00:00:00Z',
                        updated_at: '2026-01-01T00:00:00Z'
                    }
                ]
            })
        );
        fixture.componentRef.setInput('site', {
            id: 'site-1',
            user_id: 'user-1',
            domain: 'example.com',
            created_at: '2026-01-01T00:00:00Z'
        });
        fixture.detectChanges();

        const internals = component as SiteTrackingSettings & {
            snippetCode: () => string;
            trackingDomainControl: { setValue: (value: string | null) => void };
        };
        internals.trackingDomainControl.setValue('domain-1');
        fixture.detectChanges();
        expect(internals.snippetCode()).toContain('src="https://analytics.example.com/hk.js"');
    });
});
