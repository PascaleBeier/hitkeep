import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { of } from 'rxjs';
import { Team, TeamAuditListResponse, TeamInvite, TeamMember } from '@models/analytics.types';
import { TEAM_CAPABILITIES } from '@core/access/capabilities';
import { PermissionService, UserPermissions } from '@services/permission.service';
import { TeamService } from '@services/team.service';
import { TeamAdminPage } from './team-admin';

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
        expect(tabs.map((tab) => tab.value)).toEqual(['0', '1', '2', '6', '3', '5', '4']);
        const dangerTab = tabs[tabs.length - 1];
        expect(dangerTab?.value).toBe('4');
        expect(dangerTab?.icon).toBe('pi pi-exclamation-triangle');
        expect(dangerTab?.danger).toBe(true);
        expect(fixture.nativeElement.querySelector('.hk-admin-tab-label--danger i.pi-exclamation-triangle')).toBeTruthy();
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

    it('should reset the active tab when audit access is lost', () => {
        component['activeTab'].set('3');
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

        expect(component['activeTab']()).toBe('0');
    });

    it('maps team tab query params to dedicated infrastructure tab deep links', () => {
        expect(component['tabValueFromQuery']('settings')).toBe('2');
        expect(component['tabValueFromQuery']('api-clients')).toBe('2');
        expect(component['tabValueFromQuery']('2')).toBe('2');
        expect(component['tabQueryFromValue']('2')).toBe('api-clients');
        expect(component['tabValueFromQuery']('custom-domains')).toBe('6');
        expect(component['tabValueFromQuery']('tracking-domains')).toBe('6');
        expect(component['tabValueFromQuery']('domains')).toBe('6');
        expect(component['tabQueryFromValue']('6')).toBe('custom-domains');
        expect(component['tabValueFromQuery']('branding')).toBe('3');
        expect(component['tabQueryFromValue']('3')).toBe('branding');
        expect(component['tabValueFromQuery']('danger-zone')).toBe('4');
        expect(component['tabValueFromQuery']('danger')).toBe('4');
        expect(component['tabQueryFromValue']('4')).toBe('danger-zone');
    });
});
