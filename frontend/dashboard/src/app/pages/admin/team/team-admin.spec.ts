import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Component, signal } from '@angular/core';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { of } from 'rxjs';
import { Team, TeamAuditListResponse, TeamInvite, TeamMember } from '@models/analytics.types';
import { TEAM_CAPABILITIES } from '@core/access/capabilities';
import { PermissionService, UserPermissions } from '@services/permission.service';
import { TeamService } from '@services/team.service';
import { TeamAdminPage } from './team-admin';

@Component({ selector: 'app-team-admin-test-child', template: '<p>child</p>' })
class TeamAdminTestChild {}

describe('TeamAdminPage', () => {
    let component: TeamAdminPage;
    let fixture: ComponentFixture<TeamAdminPage>;
    const activeTeam = signal<Team>({
        id: 'team-1',
        name: 'Acme',
        logo_url: '',
        role: 'owner',
        created_at: '2026-01-01T00:00:00Z'
    });
    const teams = signal<Team[]>([activeTeam()]);
    const members: TeamMember[] = [
        {
            id: 'membership-1',
            user_id: 'user-1',
            email: 'owner@example.com',
            role: 'owner',
            added_at: '2026-01-01T00:00:00Z'
        }
    ];
    const invites: TeamInvite[] = [];
    const auditResponse: TeamAuditListResponse = {
        entries: [],
        total: 0,
        limit: 25,
        offset: 0,
        has_more: false
    };
    const teamServiceMock = {
        activeTeam,
        activeTeamId: signal('team-1'),
        teams,
        listTeamMembers: () => of(members),
        listTeamInvites: () => of(invites),
        listTeamAudit: () => of(auditResponse),
        updateTeam: () => of({ status: 'ok' }),
        leaveTeam: () => of({ status: 'ok', active_team_id: '' }),
        archiveTeam: () => of({ status: 'ok', active_team_id: '' }),
        loadTeams: () => {
            teams.set([activeTeam()]);
            return of({ teams: teams(), active_team_id: activeTeam().id });
        },
        listTrackingDomains: () => of([]),
        createTrackingDomain: () => of({}),
        verifyTrackingDomain: () => of({}),
        updateTrackingDomain: () => of({}),
        deleteTrackingDomain: () => of(undefined)
    };
    const permissionServiceMock = {
        permissions: signal<UserPermissions>({
            instance_role: 'user' as const,
            permissions: {},
            active_team_id: 'team-1',
            active_team_role: 'owner' as const,
            active_team_capabilities: [TEAM_CAPABILITIES.viewAudit, TEAM_CAPABILITIES.manageMembers, TEAM_CAPABILITIES.manageSettings, TEAM_CAPABILITIES.archive]
        })
    };

    beforeEach(async () => {
        permissionServiceMock.permissions.set({
            instance_role: 'user',
            permissions: {},
            active_team_id: 'team-1',
            active_team_role: 'owner',
            active_team_capabilities: [TEAM_CAPABILITIES.viewAudit, TEAM_CAPABILITIES.manageMembers, TEAM_CAPABILITIES.manageSettings, TEAM_CAPABILITIES.archive]
        });
        await TestBed.configureTestingModule({
            imports: [
                TeamAdminPage,
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
                provideHttpClient(),
                provideHttpClientTesting(),
                provideRouter([]),
                provideTranslocoLocale({
                    langToLocaleMapping: {
                        en: 'en-US'
                    }
                }),
                {
                    provide: TeamService,
                    useValue: teamServiceMock
                },
                {
                    provide: PermissionService,
                    useValue: permissionServiceMock
                }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(TeamAdminPage);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should show the activity tab for team admins and owners', () => {
        expect(fixture.nativeElement.textContent).toContain('admin.team.tabs.activity');
    });

    it('shows dedicated infrastructure, branding, and danger-zone tabs for team admins and owners', () => {
        expect(fixture.nativeElement.textContent).toContain('admin.team.tabs.apiClients');
        expect(fixture.nativeElement.textContent).toContain('admin.team.tabs.customDomains');
        expect(fixture.nativeElement.textContent).toContain('admin.team.tabs.branding');
        expect(fixture.nativeElement.textContent).toContain('admin.team.tabs.dangerZone');
        expect(fixture.nativeElement.textContent).not.toContain('admin.team.tabs.settings');
    });

    it('renders danger zone as the last visible team tab with danger styling', () => {
        const tabs = component['tabs']();
        expect(tabs.map((tab) => tab.route)).toEqual(['overview', 'members', 'api-clients', 'custom-domains', 'branding', 'activity', 'danger-zone']);
        const dangerTab = tabs[tabs.length - 1];
        expect(dangerTab?.route).toBe('danger-zone');
        expect(dangerTab?.icon).toBe('pi pi-exclamation-triangle');
        expect(dangerTab?.danger).toBe(true);
        expect(fixture.nativeElement.querySelector('.hk-admin-tab-label--danger i.pi-exclamation-triangle')).toBeTruthy();
    });

    it('defaults the active tab to the overview child route and links every tab to its child path', () => {
        expect(component['activeTab']()).toBe('overview');
        const routerLinks = Array.from(fixture.nativeElement.querySelectorAll('p-tab a, p-tab[href], p-tab')).length;
        expect(routerLinks).toBeGreaterThan(0);
        expect(component['tabs']().every((tab) => tab.route.length > 0)).toBe(true);
    });

    it('should hide the activity tab for non-managers', () => {
        activeTeam.set({
            id: 'team-1',
            name: 'Acme',
            logo_url: '',
            role: 'member',
            created_at: '2026-01-01T00:00:00Z'
        });
        permissionServiceMock.permissions.set({
            instance_role: 'user',
            permissions: {},
            active_team_id: 'team-1',
            active_team_role: 'member',
            active_team_capabilities: []
        });

        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).not.toContain('admin.team.tabs.apiClients');
        expect(fixture.nativeElement.textContent).not.toContain('admin.team.tabs.customDomains');
        expect(fixture.nativeElement.textContent).not.toContain('admin.team.tabs.activity');
        expect(fixture.nativeElement.textContent).not.toContain('admin.team.tabs.branding');
        expect(fixture.nativeElement.textContent).not.toContain('admin.team.tabs.dangerZone');
    });

});

describe('TeamAdminPage routing', () => {
    const activeTeam = signal<Team>({ id: 'team-1', name: 'Acme', logo_url: '', role: 'owner', created_at: '2026-01-01T00:00:00Z' });
    const teamServiceMock = { activeTeam, activeTeamId: signal('team-1'), teams: signal<Team[]>([activeTeam()]) };
    const permissionServiceMock = {
        permissions: signal<UserPermissions>({
            instance_role: 'user',
            permissions: {},
            active_team_id: 'team-1',
            active_team_role: 'owner',
            active_team_capabilities: [TEAM_CAPABILITIES.viewAudit, TEAM_CAPABILITIES.manageMembers, TEAM_CAPABILITIES.manageSettings, TEAM_CAPABILITIES.archive]
        })
    };

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [TranslocoTestingModule.forRoot({ langs: { en: {} }, translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }, preloadLangs: true })],
            providers: [
                provideHttpClient(),
                provideHttpClientTesting(),
                provideRouter([
                    {
                        path: 'team',
                        component: TeamAdminPage,
                        children: [
                            { path: '', pathMatch: 'full', redirectTo: 'overview' },
                            { path: 'overview', component: TeamAdminTestChild },
                            { path: 'branding', component: TeamAdminTestChild }
                        ]
                    }
                ]),
                provideTranslocoLocale({ langToLocaleMapping: { en: 'en-US' } }),
                { provide: TeamService, useValue: teamServiceMock },
                { provide: PermissionService, useValue: permissionServiceMock }
            ]
        }).compileComponents();
    });

    it('mounts on a directly-activated child route without crashing and reflects it in the tab bar', async () => {
        const harness = await RouterTestingHarness.create('/team/branding');
        const tabBar = harness.routeNativeElement!.ownerDocument.querySelector('[role="tablist"]');

        expect(tabBar?.textContent).toContain('admin.team.tabs.branding');
        const selected = harness.routeNativeElement!.ownerDocument.querySelector('[role="tab"][aria-selected="true"]');
        expect(selected?.textContent).toContain('admin.team.tabs.branding');
    });

    it('redirects the bare team path to the overview child route', async () => {
        const harness = await RouterTestingHarness.create('/team');

        const selected = harness.routeNativeElement!.ownerDocument.querySelector('[role="tab"][aria-selected="true"]');
        expect(selected?.textContent).toContain('admin.team.tabs.overview');
    });
});
