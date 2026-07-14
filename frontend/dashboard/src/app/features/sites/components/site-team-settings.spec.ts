import { ComponentFixture, TestBed } from '@angular/core/testing';
import { WritableSignal, signal } from '@angular/core';
import { Router, provideRouter } from '@angular/router';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { vi } from 'vitest';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';
import { SiteService } from '@features/sites/services/site.service';
import { TeamService } from '@services/team.service';
import { NavigationNoticeService } from '@services/navigation-notice.service';
import { SiteTeamSettings } from './site-team-settings';

interface SiteTeamSettingsTestAccess {
    availableTransferTeams(): { label: string; value: string }[];
    addMember(): void;
    transferSite(): void;
    confirmRemoveMember(member: { user_id: string; email: string }): void;
    memberForm: {
        email(): {
            control(): {
                setValue(value: string): void;
            };
        };
    };
    isAddMemberDialogVisible(): boolean;
    memberSuccessKey(): string | null;
    transferForm: {
        teamId(): {
            control(): {
                setValue(value: string): void;
            };
        };
    };
    transferSuccessKey(): string | null;
}

describe('SiteTeamSettings', () => {
    let fixture: ComponentFixture<SiteTeamSettings>;
    let component: SiteTeamSettings;
    let httpMock: HttpTestingController;
    let canSiteMock: ReturnType<typeof vi.fn>;
    let allowedSiteCapabilities: WritableSignal<string[] | null>;

    const currentSite = {
        id: 'site-1',
        user_id: 'user-1',
        domain: 'example.com',
        created_at: '2026-01-01T00:00:00Z'
    };

    const siteServiceMock = {
        sites: signal([currentSite]),
        activeSite: signal(currentSite),
        loadSites: vi.fn()
    };

    const teamServiceMock = {
        activeTeamId: signal('team-1'),
        teams: signal([
            {
                id: 'team-1',
                name: 'Current team',
                logo_url: '',
                role: 'owner' as const,
                created_at: '2026-01-01T00:00:00Z'
            },
            {
                id: 'team-2',
                name: 'Destination team',
                logo_url: '',
                role: 'admin' as const,
                created_at: '2026-01-02T00:00:00Z'
            },
            {
                id: 'team-3',
                name: 'Viewer team',
                logo_url: '',
                role: 'member' as const,
                created_at: '2026-01-03T00:00:00Z'
            }
        ])
    };

    beforeEach(async () => {
        allowedSiteCapabilities = signal<string[] | null>(null);
        canSiteMock = vi.fn((_siteId: string, capability: string) => allowedSiteCapabilities()?.includes(capability) ?? true);

        await TestBed.configureTestingModule({
            imports: [
                SiteTeamSettings,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                actions: {
                                    cancel: 'Cancel'
                                },
                                columns: {
                                    actions: 'Actions',
                                    added: 'Added',
                                    email: 'Email',
                                    role: 'Role'
                                },
                                emailAddress: 'Email address',
                                searchPlaceholder: 'Search...'
                            },
                            roles: {
                                owner: 'Owner',
                                admin: 'Admin',
                                editor: 'Editor',
                                viewer: 'Viewer'
                            },
                            sites: {
                                settings: {
                                    tabs: {
                                        team: 'Team'
                                    }
                                },
                                team: {
                                    emailPlaceholder: 'user@example.com',
                                    addMemberAction: 'Add site member',
                                    addMemberDialogTitle: 'Add site member',
                                    addMemberSuccess: 'Site member added.',
                                    confirmRemove: 'Remove {{email}} from site?',
                                    transfer: {
                                        title: 'Transfer site',
                                        description: 'Move this site and its analytics data into another team you can administer.',
                                        teamLabel: 'Destination team',
                                        teamPlaceholder: 'Select a destination team',
                                        action: 'Transfer site',
                                        success: 'Site transferred successfully.',
                                        errors: {
                                            forbidden: 'You do not have permission to transfer this site to the selected team.',
                                            generic: 'Failed to transfer site.'
                                        }
                                    },
                                    errors: {
                                        addFailed: 'Failed to add member. Ensure user exists.'
                                    }
                                }
                            },
                            teams: {
                                management: {
                                    removeAction: 'Remove'
                                }
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                provideHttpClient(),
                provideHttpClientTesting(),
                provideRouter([]),
                { provide: TeamService, useValue: teamServiceMock },
                { provide: SiteService, useValue: siteServiceMock },
                {
                    provide: AccessService,
                    useValue: {
                        canSite: canSiteMock
                    }
                },
                provideTranslocoLocale({
                    langToLocaleMapping: {
                        en: 'en-US'
                    }
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(SiteTeamSettings);
        component = fixture.componentInstance;
        fixture.componentRef.setInput('site', currentSite);
        fixture.detectChanges();

        httpMock = TestBed.inject(HttpTestingController);
        httpMock.expectOne('/api/sites/site-1/members').flush([]);
    });

    afterEach(() => {
        vi.restoreAllMocks();
        httpMock.verify();
    });

    it('filters transfer targets to teams the user can manage', () => {
        const access = component as unknown as SiteTeamSettingsTestAccess;
        expect(access.availableTransferTeams()).toEqual([
            {
                label: 'Destination team',
                value: 'team-2'
            }
        ]);
    });

    it('transfers the site and returns to overview with feedback', async () => {
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
        const access = component as unknown as SiteTeamSettingsTestAccess;
        access.transferForm.teamId().control().setValue('team-2');

        access.transferSite();

        const request = httpMock.expectOne('/api/sites/site-1/transfer-team');
        expect(request.request.method).toBe('POST');
        expect(request.request.body).toEqual({ team_id: 'team-2' });
        request.flush({
            status: 'ok',
            site_id: 'site-1',
            source_team_id: 'team-1',
            destination_team_id: 'team-2'
        });
        await fixture.whenStable();

        expect(siteServiceMock.sites()).toEqual([]);
        expect(siteServiceMock.activeSite()).toBeNull();
        expect(siteServiceMock.loadSites).toHaveBeenCalled();
        expect(access.transferSuccessKey()).toBe('sites.team.transfer.success');
        expect(navigate).toHaveBeenCalledWith(['/overview']);
        expect(TestBed.inject(NavigationNoticeService).key()).toBe('sites.settings.notices.siteTransferred');
    });

    it('does not call write endpoints without site team-management capability', () => {
        allowedSiteCapabilities.set([SITE_CAPABILITIES.view]);
        const access = component as unknown as SiteTeamSettingsTestAccess;
        access.memberForm.email().control().setValue('teammate@example.com');
        access.transferForm.teamId().control().setValue('team-2');

        access.addMember();
        access.transferSite();
        access.confirmRemoveMember({
            user_id: 'user-2',
            email: 'teammate@example.com'
        });

        httpMock.expectNone('/api/sites/site-1/members');
        httpMock.expectNone('/api/sites/site-1/transfer-team');
        httpMock.expectNone('/api/sites/site-1/members/user-2');
    });

    it('opens the add-site-member form in a CRUD dialog from the member table', () => {
        expect(fixture.nativeElement.querySelector('#member-email')).toBeNull();

        const addButton = Array.from<HTMLButtonElement>(fixture.nativeElement.querySelectorAll('button')).find((button) => button.textContent?.includes('Add site member'));
        expect(addButton).toBeTruthy();

        addButton?.click();
        fixture.detectChanges();

        expect(document.body.querySelector('#member-email')).toBeTruthy();
        expect(document.body.textContent).toContain('Add site member');
    });

    it('adds a site member from the dialog and shows local table feedback', () => {
        const access = component as unknown as SiteTeamSettingsTestAccess;
        const addButton = Array.from<HTMLButtonElement>(fixture.nativeElement.querySelectorAll('button')).find((button) => button.textContent?.includes('Add site member'));
        addButton?.click();
        fixture.detectChanges();

        access.memberForm.email().control().setValue('teammate@example.com');
        access.addMember();

        const request = httpMock.expectOne('/api/sites/site-1/members');
        expect(request.request.method).toBe('POST');
        expect(request.request.body).toEqual({
            email: 'teammate@example.com',
            role: 'viewer'
        });
        request.flush({ status: 'ok' });
        httpMock.expectOne('/api/sites/site-1/members').flush([]);

        expect(access.isAddMemberDialogVisible()).toBe(false);
        expect(access.memberSuccessKey()).toBe('sites.team.addMemberSuccess');
    });
});
