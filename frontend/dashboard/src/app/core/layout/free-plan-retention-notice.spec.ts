import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { Team } from '@models/analytics.types';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { ShareService } from '@services/share.service';
import { TeamService } from '@services/team.service';
import { FreePlanRetentionNotice } from './free-plan-retention-notice';

describe('FreePlanRetentionNotice', () => {
    let fixture: ComponentFixture<FreePlanRetentionNotice>;
    let activeTeam = signal<Team | null>(freeTeam('team-a'));
    let cloudHosted = signal(true);
    let shareMode = signal(false);

    beforeEach(async () => {
        activeTeam = signal<Team | null>(freeTeam('team-a'));
        cloudHosted = signal(true);
        shareMode = signal(false);
        window.localStorage.removeItem('hitkeep.freeRetentionNotice.dismissed.team-a');
        window.localStorage.removeItem('hitkeep.freeRetentionNotice.dismissed.team-b');

        await TestBed.configureTestingModule({
            imports: [
                FreePlanRetentionNotice,
                NoopAnimationsModule,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            cloud: {
                                retentionNotice: {
                                    message: 'Free plan data is retained for {{count}} days.',
                                    hint: 'Upgrade to keep your full visitor history.',
                                    usageMessageSites: 'Your team is using {{current}} of {{limit}} sites on the Free plan.',
                                    usageMessageMembers: 'Your team is using {{current}} of {{limit}} team members on the Free plan.',
                                    upgradeAction: 'Upgrade to Pro',
                                    dismissAction: 'Dismiss retention notice'
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
                provideRouter([]),
                {
                    provide: DashboardBootstrapService,
                    useValue: {
                        cloudHosted
                    }
                },
                {
                    provide: TeamService,
                    useValue: {
                        activeTeam
                    }
                },
                {
                    provide: ShareService,
                    useValue: {
                        isShareMode: shareMode
                    }
                }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(FreePlanRetentionNotice);
        fixture.detectChanges();
    });

    afterEach(() => {
        window.localStorage.removeItem('hitkeep.freeRetentionNotice.dismissed.team-a');
        window.localStorage.removeItem('hitkeep.freeRetentionNotice.dismissed.team-b');
        window.localStorage.removeItem('hitkeep.freeUsageNotice.dismissed.team-a.sites');
        window.localStorage.removeItem('hitkeep.freeUsageNotice.dismissed.team-a.members');
    });

    it('shows the retention notice for hosted free-plan teams', () => {
        const notice = noticeElement();

        expect(notice).not.toBeNull();
        expect(notice?.textContent).toContain('Free plan data is retained for 60 days.');
        expect(notice?.textContent).toContain('Upgrade to keep your full visitor history.');
    });

    it('offers a single upgrade CTA that leads to the team overview page', () => {
        const notice = noticeElement();
        const upgradeButton = notice?.querySelector('p-button[routerlink="/admin/team/overview"]');

        expect(upgradeButton).not.toBeNull();
        expect(upgradeButton?.textContent).toContain('Upgrade to Pro');
        // The dismiss control is the only other interactive element — no competing CTAs.
        expect(notice?.querySelectorAll('a, p-button').length).toBe(1);
    });

    it('hides the notice outside cloud free-plan dashboard mode', () => {
        activeTeam.set({
            ...freeTeam('team-a'),
            plan: { code: 'pro', name: 'Pro' }
        });
        fixture.detectChanges();
        expect(noticeElement()).toBeNull();

        activeTeam.set(freeTeam('team-a'));
        cloudHosted.set(false);
        fixture.detectChanges();
        expect(noticeElement()).toBeNull();

        cloudHosted.set(true);
        shareMode.set(true);
        fixture.detectChanges();
        expect(noticeElement()).toBeNull();
    });

    it('escalates to a usage message when a plan limit is nearly exhausted', () => {
        activeTeam.set({ ...freeTeam('team-a'), usage: { current_sites: 3, current_members: 1, current_pending_invites: 0 } });
        fixture.detectChanges();

        expect(noticeElement()?.textContent).toContain('Your team is using 3 of 3 sites on the Free plan.');
    });

    it('surfaces usage pressure with its own dismissal even after the retention notice was dismissed', () => {
        window.localStorage.setItem('hitkeep.freeRetentionNotice.dismissed.team-a', 'dismissed');
        activeTeam.set({ ...freeTeam('team-a'), usage: { current_sites: 1, current_members: 3, current_pending_invites: 0 } });
        fixture.detectChanges();

        expect(noticeElement()?.textContent).toContain('Your team is using 3 of 3 team members on the Free plan.');

        dismissButton()?.click();
        fixture.detectChanges();

        expect(noticeElement()).toBeNull();
        expect(window.localStorage.getItem('hitkeep.freeUsageNotice.dismissed.team-a.members')).toBe('dismissed');
    });

    it('dismisses the notice per active team in localStorage', () => {
        dismissButton()?.click();
        fixture.detectChanges();

        expect(noticeElement()).toBeNull();
        expect(window.localStorage.getItem('hitkeep.freeRetentionNotice.dismissed.team-a')).toBe('dismissed');

        activeTeam.set(freeTeam('team-b'));
        fixture.detectChanges();
        expect(noticeElement()).not.toBeNull();

        activeTeam.set(freeTeam('team-a'));
        fixture.detectChanges();
        expect(noticeElement()).toBeNull();
    });

    function noticeElement(): HTMLElement | null {
        return fixture.nativeElement.querySelector('[data-testid="free-plan-retention-notice"]') as HTMLElement | null;
    }

    function dismissButton(): HTMLButtonElement | null {
        return fixture.nativeElement.querySelector('.free-plan-retention-notice__dismiss') as HTMLButtonElement | null;
    }
});

function freeTeam(id: string): Team {
    return {
        id,
        name: 'Acme Analytics',
        logo_url: '',
        role: 'owner',
        created_at: '2026-03-01T00:00:00Z',
        entitlements: {
            max_sites_per_team: 3,
            max_team_members: 3,
            max_retention_days: 60,
            allow_sso: false,
            allow_custom_branding: false
        },
        plan: {
            code: 'free',
            name: 'Free'
        }
    };
}
