import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { SiteService } from '@features/sites/services/site.service';
import { InstallMethod, SiteTrackingSettings } from './site-tracking-settings';

interface Toggle {
    control: () => { setValue: (value: boolean) => void };
}

type Internals = SiteTrackingSettings & {
    snippetCode: () => string;
    trackerHostURL: () => string;
    npmInitSnippet: () => string;
    serverIngestSnippet: () => string;
    changedOptionCount: () => number;
    selectInstallMethod: (method: InstallMethod) => void;
    trackingDomainControl: { setValue: (value: string | null) => void };
    trackingForm: {
        collectDnt: () => Toggle;
        disableBeacon: () => Toggle;
        enableWebVitals: () => Toggle;
        trackOutbound: () => Toggle;
        trackDownloads: () => Toggle;
        trackForms: () => Toggle;
    };
};

const customDomainOptions = {
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
};

const site = {
    id: 'site-1',
    user_id: 'user-1',
    domain: 'example.com',
    created_at: '2026-01-01T00:00:00Z'
};

describe('SiteTrackingSettings', () => {
    let component: SiteTrackingSettings;
    let internals: Internals;
    let fixture: ComponentFixture<SiteTrackingSettings>;
    let base: HTMLBaseElement;
    let previousBases: HTMLBaseElement[];
    let siteServiceMock: {
        getTrackingStatus: ReturnType<typeof vi.fn>;
        getTrackingDomainOptions: ReturnType<typeof vi.fn>;
    };

    const testId = (id: string) => fixture.nativeElement.querySelector(`[data-testid="${id}"]`) as HTMLElement | null;

    /** Mount the site with one active custom tracking domain and select it. */
    const selectCustomDomain = () => {
        siteServiceMock.getTrackingDomainOptions.mockReturnValue(of(customDomainOptions));
        fixture.componentRef.setInput('site', site);
        fixture.detectChanges();
        internals.trackingDomainControl.setValue('domain-1');
        fixture.detectChanges();
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
        internals = component as Internals;
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
        selectCustomDomain();
        expect(internals.snippetCode()).toContain('src="https://analytics.example.com/hk.js"');
    });

    it('derives the instance host by stripping the script filename, keeping the base path prefix', () => {
        expect(internals.trackerHostURL()).toMatch(/\/hitkeep$/);
        expect(internals.trackerHostURL()).not.toContain('hk.js');

        selectCustomDomain();

        expect(internals.trackerHostURL()).toBe('https://analytics.example.com');
        expect(internals.npmInitSnippet()).toContain("init({ host: 'https://analytics.example.com' })");
        expect(internals.serverIngestSnippet()).toContain('https://analytics.example.com/api/ingest/server/pageview');
    });

    it('puts the site domain in the server-side ingest sample', () => {
        fixture.componentRef.setInput('site', site);
        fixture.detectChanges();

        expect(internals.serverIngestSnippet()).toContain('"url": "https://example.com/pricing"');
        expect(internals.serverIngestSnippet()).toContain('Authorization: Bearer $HITKEEP_API_TOKEN');
    });

    it('renders the selected install method and only shows advanced options for the script tag', () => {
        expect(testId('tracking-snippet')).toBeTruthy();
        expect(testId('tracking-advanced-options')).toBeTruthy();

        internals.selectInstallMethod('npm');
        fixture.detectChanges();
        expect(testId('tracking-snippet')).toBeNull();
        expect(testId('tracking-advanced-options')).toBeNull();
        expect(testId('tracking-npm-install')).toBeTruthy();
        expect(testId('tracking-npm-registry-link')?.getAttribute('href')).toBe('https://www.npmjs.com/package/@hitkeep/tracker');

        internals.selectInstallMethod('wordpress');
        fixture.detectChanges();
        expect(testId('tracking-npm-install')).toBeNull();
        expect(testId('tracking-wordpress-url')?.textContent).toContain('/hitkeep');
        expect(testId('tracking-wordpress-directory-link')?.getAttribute('href')).toBe('https://wordpress.org/plugins/hitkeep/');

        internals.selectInstallMethod('server');
        fixture.detectChanges();
        expect(testId('tracking-wordpress-url')).toBeNull();
        expect(testId('tracking-server-snippet')).toBeTruthy();
        expect(testId('tracking-server-docs-link')?.getAttribute('href')).toBe('https://hitkeep.com/guides/tracking/server-side-tracking/');

        internals.selectInstallMethod('script');
        fixture.detectChanges();
        expect(testId('tracking-snippet')).toBeTruthy();
    });

    it('counts only non-default snippet options', () => {
        expect(internals.changedOptionCount()).toBe(0);
        expect(testId('tracking-options-badge')).toBeNull();

        internals.trackingForm.collectDnt().control().setValue(true);
        internals.trackingForm.trackOutbound().control().setValue(false);
        fixture.detectChanges();
        expect(internals.changedOptionCount()).toBe(2);
        expect(testId('tracking-options-badge')).toBeTruthy();

        internals.trackingForm.collectDnt().control().setValue(false);
        internals.trackingForm.trackOutbound().control().setValue(true);
        fixture.detectChanges();
        expect(internals.changedOptionCount()).toBe(0);
    });

    it('counts a selected custom tracking domain as a changed option', () => {
        expect(internals.changedOptionCount()).toBe(0);

        selectCustomDomain();

        expect(internals.changedOptionCount()).toBe(1);
    });

    it('reveals the verifier field grid on demand', () => {
        fixture.componentRef.setInput('site', site);
        fixture.detectChanges();

        expect(testId('tracking-verifier-details')).toBeNull();

        const toggle = testId('tracking-verifier-toggle');
        (toggle?.querySelector('button') ?? toggle)?.click();
        fixture.detectChanges();

        expect(testId('tracking-verifier-details')).toBeTruthy();
    });
});
