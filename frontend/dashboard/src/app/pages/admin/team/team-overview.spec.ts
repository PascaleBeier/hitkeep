import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { TeamOverviewPage } from './team-overview';
import { TeamService } from '@services/team.service';
import { AnalyticsService } from '@services/analytics.service';
import { CloudService } from '@services/cloud.service';
import { Team } from '@models/analytics.types';
import { of } from 'rxjs';
import { vi } from 'vitest';

describe('TeamOverviewPage', () => {
    let fixture: ComponentFixture<TeamOverviewPage>;
    type TeamOverviewTestAccess = TeamOverviewPage & {
        openBillingPortal(): void;
        startUpgradeCheckout(): void;
        redirectTo(url: string): void;
    };
    let systemStatusResponse: {
        needs_setup: boolean;
        version: string;
        cloud?: {
            hosted: boolean;
            signup_enabled: boolean;
            jurisdiction?: string;
            region?: string;
            upgrade_url?: string;
            support_url?: string;
        };
    };

    const activeTeam = signal<Team | null>({
        id: '00000000-0000-0000-0000-000000000001',
        name: 'Acme Analytics',
        logo_url: '',
        role: 'owner',
        created_at: '2026-03-01T00:00:00Z',
        usage: {
            current_sites: 3,
            current_members: 5,
            current_pending_invites: 2
        },
        entitlements: {
            max_sites_per_team: 10,
            max_team_members: 8,
            max_retention_days: 365,
            allow_sso: true,
            allow_custom_branding: true
        },
        plan: {
            code: 'free',
            name: 'Free',
            upgrade_url: 'https://hitkeep.com/cloud/upgrade',
            support_url: 'https://hitkeep.com/cloud/support'
        }
    });

    beforeEach(async () => {
        systemStatusResponse = {
            needs_setup: false,
            version: 'v2.0.0',
            cloud: {
                hosted: true,
                signup_enabled: false,
                jurisdiction: 'EU',
                region: 'eu-central-1',
                upgrade_url: 'https://hitkeep.com/cloud/upgrade',
                support_url: 'https://hitkeep.com/cloud/support'
            }
        };

        await TestBed.configureTestingModule({
            imports: [
                TeamOverviewPage,
                NoopAnimationsModule,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            admin: {
                                team: {
                                    overview: {
                                        title: 'Team',
                                        teamIdLabel: 'Team ID',
                                        memberCount: 'Members',
                                        usage: {
                                            title: 'Usage and limits',
                                            description: 'Track plan usage before it becomes a limit.',
                                            currentUsage: '{{count}} used',
                                            unlimited: 'Unlimited',
                                            unlimitedFootnote: 'Unlimited on the current plan',
                                            limitValue: 'Limit {{count}}',
                                            progressLabel: '{{count}}% of limit',
                                            pendingInviteOne: '1 pending invite',
                                            pendingInviteMany: '{{count}} pending invites',
                                            metrics: {
                                                sites: 'Sites',
                                                members: 'Members'
                                            }
                                        },
                                        cloud: {
                                            title: 'Cloud plan',
                                            managed: 'Managed cloud',
                                            description: 'Run this team in the hosted EU or US cell with hard limits, backups, and upgrade paths.',
                                            manageBillingAction: 'Manage billing',
                                            jurisdiction: 'Jurisdiction',
                                            region: 'Region',
                                            retention: 'Retention',
                                            retentionDays: '{{count}} days',
                                            unlimitedRetention: 'Unlimited retention',
                                            upgradeAction: 'Upgrade plan',
                                            supportAction: 'Contact support'
                                        },
                                        plans: {
                                            title: 'Upgrade your plan',
                                            description: 'Unlock longer retention, more sites, and team capacity.',
                                            currentPlan: 'Current plan',
                                            recommended: 'Recommended',
                                            upgradeAction: 'Upgrade to {{plan}}',
                                            features: {
                                                sites: 'Up to {{count}} sites',
                                                members: 'Up to {{count}} team members',
                                                retention: '{{count}}-year data retention',
                                                retentionDays: '{{count}}-day data retention'
                                            }
                                        }
                                    }
                                }
                            },
                            common: {
                                columns: { role: 'Role' },
                                copyControl: {
                                    copy: 'Copy',
                                    copied: 'Copied',
                                    failed: 'Copy failed',
                                    ariaLabel: 'Copy to clipboard'
                                }
                            },
                            teams: { roles: { owner: 'Owner' } },
                            signup: {
                                plans: {
                                    free: { name: 'Free', price: '€0', description: '60-day retention to validate your sites.' },
                                    pro: { name: 'Pro', price: '€15/mo', description: 'Longer retention and more room for a small team.' },
                                    business: { name: 'Business', price: '€39/mo', description: 'More scale for agencies.' }
                                }
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    }
                })
            ],
            providers: [
                {
                    provide: TeamService,
                    useValue: {
                        activeTeam
                    }
                },
                {
                    provide: AnalyticsService,
                    useValue: {
                        getSystemStatus: () => of(systemStatusResponse)
                    }
                },
                {
                    provide: CloudService,
                    useValue: {
                        createBillingPortalSession: vi.fn().mockReturnValue(of({ url: 'https://billing.stripe.test/session' })),
                        createBillingCheckoutSession: vi.fn().mockReturnValue(of({ url: 'https://checkout.stripe.test/session' })),
                        getPlans: vi.fn().mockReturnValue(
                            of([
                                {
                                    code: 'free',
                                    name: 'Free',
                                    entitlements: { max_sites_per_team: 3, max_team_members: 3, max_retention_days: 60, allow_sso: false, allow_custom_branding: false }
                                },
                                {
                                    code: 'pro',
                                    name: 'Pro',
                                    entitlements: { max_sites_per_team: 10, max_team_members: 5, max_retention_days: 365, allow_sso: false, allow_custom_branding: false }
                                },
                                {
                                    code: 'business',
                                    name: 'Business',
                                    entitlements: { max_sites_per_team: 50, max_team_members: 20, max_retention_days: 1095, allow_sso: true, allow_custom_branding: true }
                                }
                            ])
                        )
                    }
                }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(TeamOverviewPage);
        fixture.detectChanges();
    });

    it('renders usage cards for the active team', () => {
        const text = fixture.nativeElement.textContent;
        expect(text).toContain('Usage and limits');
        expect(text).toContain('Sites');
        expect(text).toContain('Members');
        expect(text).toContain('2 pending invites');
    });

    it('renders the active team ID with a copy control', () => {
        const text = fixture.nativeElement.textContent;
        const copyButton = fixture.nativeElement.querySelector('app-copy-control button') as HTMLButtonElement | null;

        expect(text).toContain('Team ID');
        expect(text).toContain('00000000-0000-0000-0000-000000000001');
        expect(copyButton).not.toBeNull();
        expect(copyButton?.getAttribute('aria-label')).toBe('Copy to clipboard');
    });

    it('renders cloud plan with inline upgrade comparison for free users', () => {
        const text = fixture.nativeElement.textContent;
        expect(text).toContain('Cloud plan');
        expect(text).toContain('Managed cloud');
        expect(text).toContain('EU');
        expect(text).toContain('365 days');
        expect(text).toContain('Current plan');
        expect(text).toContain('Upgrade to Pro');
        expect(text).toContain('Upgrade to Business');
    });

    it('creates a billing portal session from the cloud card', () => {
        const component = fixture.componentInstance as TeamOverviewTestAccess;
        const cloudService = TestBed.inject(CloudService);
        const redirectSpy = vi.spyOn(component, 'redirectTo').mockImplementation(() => undefined);

        component.openBillingPortal();

        expect(cloudService.createBillingPortalSession).toHaveBeenCalledWith({ locale: 'en' });
        expect(redirectSpy).toHaveBeenCalledWith('https://billing.stripe.test/session');
    });

    it('starts a checkout session when upgrading a free cloud team', () => {
        const component = fixture.componentInstance as TeamOverviewTestAccess;
        const cloudService = TestBed.inject(CloudService);
        const redirectSpy = vi.spyOn(component, 'redirectTo').mockImplementation(() => undefined);

        component.startUpgradeCheckout();

        expect(cloudService.createBillingCheckoutSession).toHaveBeenCalledWith({ plan_code: 'pro', locale: 'en' });
        expect(redirectSpy).toHaveBeenCalledWith('https://checkout.stripe.test/session');
    });

    it('hides usage limits and cloud plan details for OSS instances', () => {
        fixture.destroy();
        systemStatusResponse = {
            needs_setup: false,
            version: 'v2.0.0'
        };

        fixture = TestBed.createComponent(TeamOverviewPage);
        fixture.detectChanges();

        const text = fixture.nativeElement.textContent;
        expect(text).not.toContain('Usage and limits');
        expect(text).not.toContain('Cloud plan');
        expect(text).toContain('Acme Analytics');
        expect(text).toContain('Members');
    });
});
