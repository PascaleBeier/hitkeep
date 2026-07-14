import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { ActivatedRoute, Router } from '@angular/router';
import { BehaviorSubject, EMPTY, Subject, of } from 'rxjs';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { vi } from 'vitest';
import { TEAM_CAPABILITIES } from '@core/access/capabilities';
import { TeamMember } from '@models/analytics.types';
import { PermissionService, UserPermissions } from '@services/permission.service';
import { NavigationNoticeService } from '@services/navigation-notice.service';
import { TeamMembersPage } from './team-members';
import { TeamService } from '@services/team.service';

interface TeamMembersTestAccess {
    members(): unknown[];
    pendingInvites(): unknown[];
    inviteForm: {
        email(): {
            control(): {
                setValue(value: string): void;
            };
        };
    };
    inviteMember(): void;
    isInviteDialogVisible(): boolean;
    successKey(): string | null;
    roleControlFor(member: TeamMember): {
        disabled: boolean;
    };
    onRoleChange(member: TeamMember, role: TeamMember['role']): void;
}

describe('TeamMembersPage', () => {
    let fixture: ComponentFixture<TeamMembersPage>;
    let component: TeamMembersPage;
    const routeData = new BehaviorSubject<Record<string, unknown>>({});
    const activatedRouteMock = {
        data: routeData.asObservable(),
        snapshot: { data: {} }
    };
    const routerMock = {
        events: EMPTY,
        navigate: vi.fn((commands: unknown[], extras?: unknown) => {
            void commands;
            void extras;
            return Promise.resolve(true);
        })
    };

    const teamServiceMock = {
        activeTeamId: signal('team-1'),
        activeTeam: signal({
            id: 'team-1',
            name: 'Acme',
            logo_url: '',
            role: 'owner' as const,
            created_at: '2026-01-01T00:00:00Z'
        }),
        listTeamMembers: vi.fn((teamID: string) => {
            void teamID;
            return of([
                {
                    id: 'member-row',
                    user_id: 'user-1',
                    email: 'owner@example.com',
                    role: 'owner' as const,
                    added_at: '2026-01-01T00:00:00Z'
                },
                {
                    id: 'member-admin-row',
                    user_id: 'user-2',
                    email: 'admin@example.com',
                    role: 'admin' as const,
                    added_at: '2026-01-02T00:00:00Z'
                }
            ]);
        }),
        listTeamInvites: vi.fn((teamID: string) => {
            void teamID;
            return of([
                {
                    id: 'invite-1',
                    team_id: 'team-1',
                    email: 'invitee@example.com',
                    role: 'admin' as const,
                    status: 'pending' as const,
                    created_at: '2026-01-03T00:00:00Z',
                    expires_at: '2026-01-10T00:00:00Z'
                }
            ]);
        }),
        upsertTeamMember: vi.fn((teamID: string, payload: { email: string; role: string }) => {
            void teamID;
            void payload;
            return of({ status: 'ok', is_invite: true });
        }),
        removeTeamMember: vi.fn(() => of({ status: 'ok' })),
        resendTeamInvite: vi.fn(() => of({ status: 'ok' })),
        revokeTeamInvite: vi.fn(() => of({ status: 'ok' })),
        transferTeamOwnership: vi.fn(() => of({ status: 'ok' })),
        loadTeams: vi.fn(() => of({ active_team_id: 'team-1', teams: [] }))
    };
    const permissionServiceMock = {
        permissions: signal<UserPermissions>({
            instance_role: 'user' as const,
            permissions: {},
            active_team_id: 'team-1',
            active_team_role: 'owner' as const,
            active_team_capabilities: [TEAM_CAPABILITIES.manageMembers, TEAM_CAPABILITIES.transferOwnership]
        })
    };

    beforeEach(async () => {
        routeData.next({});
        routerMock.navigate.mockClear();
        permissionServiceMock.permissions.set({
            instance_role: 'user',
            permissions: {},
            active_team_id: 'team-1',
            active_team_role: 'owner',
            active_team_capabilities: [TEAM_CAPABILITIES.manageMembers, TEAM_CAPABILITIES.transferOwnership]
        });
        await TestBed.configureTestingModule({
            imports: [
                TeamMembersPage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            teams: {
                                management: {
                                    title: 'Team members',
                                    subtitle: 'Manage access and roles for {{team}}.',
                                    inviteDialogTitle: 'Invite team member',
                                    invite: {
                                        emailLabel: 'Email address',
                                        emailPlaceholder: 'user@example.com',
                                        emailInvalid: 'Enter a valid email address.',
                                        roleLabel: 'Role',
                                        submitAction: 'Invite member'
                                    },
                                    errors: {
                                        inviteForbidden: 'You do not have permission to invite team members.'
                                    }
                                },
                                roles: {
                                    owner: 'Owner',
                                    admin: 'Admin',
                                    member: 'Member'
                                }
                            },
                            common: {
                                actions: {
                                    cancel: 'Cancel',
                                    refresh: 'Refresh'
                                },
                                columns: {
                                    actions: 'Actions',
                                    added: 'Added',
                                    email: 'Email',
                                    role: 'Role'
                                },
                                searchPlaceholder: 'Search...'
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
                { provide: TeamService, useValue: teamServiceMock },
                { provide: PermissionService, useValue: permissionServiceMock },
                { provide: ActivatedRoute, useValue: activatedRouteMock },
                { provide: Router, useValue: routerMock },
                provideTranslocoLocale({
                    langToLocaleMapping: {
                        en: 'en-US'
                    }
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(TeamMembersPage);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('loads members and pending invites for managers', () => {
        const access = component as unknown as TeamMembersTestAccess;
        expect(teamServiceMock.listTeamMembers).toHaveBeenCalledWith('team-1');
        expect(teamServiceMock.listTeamInvites).toHaveBeenCalledWith('team-1');
        expect(access.members().length).toBe(2);
        expect(access.pendingInvites().length).toBe(1);
    });

    it('routes the table action to the invitation URL', () => {
        expect(fixture.nativeElement.querySelector('#team-member-email')).toBeNull();

        const inviteButton = Array.from<HTMLButtonElement>(fixture.nativeElement.querySelectorAll('button')).find((button) => button.textContent?.includes('Invite member'));
        expect(inviteButton).toBeTruthy();

        inviteButton?.click();
        fixture.detectChanges();

        expect(routerMock.navigate).toHaveBeenCalledWith(['invite'], { relativeTo: activatedRouteMock });
    });

    it('opens the invitation dialog when activated through the invite child route', () => {
        routeData.next({ openInvite: true });
        fixture.detectChanges();

        expect(document.body.querySelector('#team-member-email')).toBeTruthy();
        expect(document.body.textContent).toContain('Invite team member');
    });

    it('invites a member from the dialog and returns feedback to the member surface', () => {
        const access = component as unknown as TeamMembersTestAccess;
        routeData.next({ openInvite: true });
        fixture.detectChanges();

        access.inviteForm.email().control().setValue('New.Member@Example.com');
        access.inviteMember();

        expect(teamServiceMock.upsertTeamMember).toHaveBeenCalled();
        expect(teamServiceMock.upsertTeamMember.mock.calls.at(-1)).toEqual([
            'team-1',
            {
                email: 'new.member@example.com',
                role: 'member'
            }
        ]);
        expect(access.isInviteDialogVisible()).toBe(false);
        expect(access.successKey()).toBe('teams.management.status.inviteSent');
        expect(routerMock.navigate).toHaveBeenCalledWith(['../'], { relativeTo: activatedRouteMock, replaceUrl: true });
    });

    it('falls back to the members page when the invite route is unavailable', async () => {
        permissionServiceMock.permissions.set({
            instance_role: 'user',
            permissions: {},
            active_team_id: 'team-1',
            active_team_role: 'member',
            active_team_capabilities: []
        });

        routeData.next({ openInvite: true });
        fixture.detectChanges();
        await fixture.whenStable();

        const access = component as unknown as TeamMembersTestAccess;
        expect(access.isInviteDialogVisible()).toBe(false);
        expect(routerMock.navigate).toHaveBeenCalledWith(['/admin/team/members'], {
            replaceUrl: true
        });
        expect(TestBed.inject(NavigationNoticeService).key()).toBe('teams.management.errors.inviteForbidden');
    });

    it('disables role form controls through the control state while role updates are pending', () => {
        const access = component as unknown as TeamMembersTestAccess;
        const adminMember = access.members().find((member): member is TeamMember => {
            return typeof member === 'object' && member !== null && 'user_id' in member && (member as TeamMember).user_id === 'user-2';
        });
        if (!adminMember) {
            throw new Error('expected admin member fixture');
        }

        const roleUpdate = new Subject<{ status: string; is_invite: boolean }>();
        teamServiceMock.upsertTeamMember.mockReturnValueOnce(roleUpdate.asObservable());
        const control = access.roleControlFor(adminMember);

        expect(control.disabled).toBe(false);
        access.onRoleChange(adminMember, 'member');
        expect(control.disabled).toBe(true);

        roleUpdate.next({ status: 'ok', is_invite: false });
        roleUpdate.complete();

        expect(control.disabled).toBe(false);
    });
});
