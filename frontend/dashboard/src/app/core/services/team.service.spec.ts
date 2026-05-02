import { TestBed } from '@angular/core/testing';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideHttpClient } from '@angular/common/http';
import { TeamService } from '@services/team.service';

describe('TeamService', () => {
    let service: TeamService;
    let httpMock: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [TeamService, provideHttpClient(), provideHttpClientTesting()]
        });

        service = TestBed.inject(TeamService);
        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('should be created', () => {
        expect(service).toBeTruthy();
    });

    it('should load teams and set active team', () => {
        service.loadTeams().subscribe();

        const req = httpMock.expectOne('/api/user/teams');
        expect(req.request.method).toBe('GET');
        req.flush({
            active_team_id: '00000000-0000-0000-0000-000000000002',
            teams: [
                {
                    id: '00000000-0000-0000-0000-000000000001',
                    name: 'Alpha',
                    logo_url: '',
                    role: 'owner',
                    created_at: '2026-01-01T00:00:00Z'
                },
                {
                    id: '00000000-0000-0000-0000-000000000002',
                    name: 'Bravo',
                    logo_url: '',
                    role: 'admin',
                    created_at: '2026-01-02T00:00:00Z'
                }
            ]
        });

        expect(service.teams().length).toBe(2);
        expect(service.activeTeamId()).toBe('00000000-0000-0000-0000-000000000002');
        expect(service.activeTeam()?.name).toBe('Bravo');
        expect(service.hasMultipleTeams()).toBe(true);
    });

    it('should switch active team', () => {
        service.teams.set([
            { id: '00000000-0000-0000-0000-000000000001', name: 'Alpha', logo_url: '', role: 'owner', created_at: '2026-01-01T00:00:00Z' },
            { id: '00000000-0000-0000-0000-000000000002', name: 'Bravo', logo_url: '', role: 'admin', created_at: '2026-01-02T00:00:00Z' }
        ]);
        service.activeTeamId.set('00000000-0000-0000-0000-000000000001');

        service.setActiveTeam('00000000-0000-0000-0000-000000000002').subscribe();

        const req = httpMock.expectOne('/api/user/teams/active');
        expect(req.request.method).toBe('PUT');
        expect(req.request.body).toEqual({ team_id: '00000000-0000-0000-0000-000000000002' });
        req.flush({
            status: 'ok',
            active_team_id: '00000000-0000-0000-0000-000000000002'
        });

        expect(service.activeTeamId()).toBe('00000000-0000-0000-0000-000000000002');
    });

    it('should skip request when selecting current team', () => {
        service.activeTeamId.set('00000000-0000-0000-0000-000000000001');

        service.setActiveTeam('00000000-0000-0000-0000-000000000001').subscribe((response) => {
            expect(response.active_team_id).toBe('00000000-0000-0000-0000-000000000001');
        });

        httpMock.expectNone('/api/user/teams/active');
    });

    it('should list team members', () => {
        service.listTeamMembers('team-id').subscribe((members) => {
            expect(members.length).toBe(1);
            expect(members[0].email).toBe('member@example.com');
        });

        const req = httpMock.expectOne('/api/user/teams/team-id/members');
        expect(req.request.method).toBe('GET');
        req.flush([
            {
                id: 'member-row-id',
                user_id: 'user-id',
                email: 'member@example.com',
                role: 'member',
                added_at: '2026-01-03T00:00:00Z'
            }
        ]);
    });

    it('should list pending team invites', () => {
        service.listTeamInvites('team-id').subscribe((invites) => {
            expect(invites.length).toBe(1);
            expect(invites[0].email).toBe('invitee@example.com');
        });

        const req = httpMock.expectOne('/api/user/teams/team-id/invites');
        expect(req.request.method).toBe('GET');
        req.flush([
            {
                id: 'invite-id',
                team_id: 'team-id',
                email: 'invitee@example.com',
                role: 'admin',
                status: 'pending',
                created_at: '2026-01-03T00:00:00Z',
                expires_at: '2026-01-10T00:00:00Z'
            }
        ]);
    });

    it('should upsert team member', () => {
        service
            .upsertTeamMember('team-id', {
                email: 'new@example.com',
                role: 'admin'
            })
            .subscribe((response) => {
                expect(response.status).toBe('ok');
            });

        const req = httpMock.expectOne('/api/user/teams/team-id/members');
        expect(req.request.method).toBe('POST');
        expect(req.request.body).toEqual({
            email: 'new@example.com',
            role: 'admin'
        });
        req.flush({
            status: 'ok',
            is_invite: true
        });
    });

    it('should resend team invite', () => {
        service.resendTeamInvite('team-id', 'invite-id').subscribe((response) => {
            expect(response.status).toBe('ok');
            expect(response.invite.id).toBe('invite-id');
        });

        const req = httpMock.expectOne('/api/user/teams/team-id/invites/invite-id/resend');
        expect(req.request.method).toBe('POST');
        expect(req.request.body).toEqual({});
        req.flush({
            status: 'ok',
            invite: {
                id: 'invite-id',
                team_id: 'team-id',
                email: 'invitee@example.com',
                role: 'member',
                status: 'pending',
                created_at: '2026-01-03T00:00:00Z',
                expires_at: '2026-01-10T00:00:00Z'
            }
        });
    });

    it('should revoke team invite', () => {
        service.revokeTeamInvite('team-id', 'invite-id').subscribe((response) => {
            expect(response.status).toBe('ok');
        });

        const req = httpMock.expectOne('/api/user/teams/team-id/invites/invite-id');
        expect(req.request.method).toBe('DELETE');
        req.flush({
            status: 'ok'
        });
    });

    it('should remove team member', () => {
        service.removeTeamMember('team-id', 'user-id').subscribe((response) => {
            expect(response.status).toBe('ok');
        });

        const req = httpMock.expectOne('/api/user/teams/team-id/members/user-id');
        expect(req.request.method).toBe('DELETE');
        req.flush({
            status: 'ok'
        });
    });

    it('should list team audit entries', () => {
        service
            .listTeamAudit('team-id', {
                action: 'member.role_updated',
                outcome: 'success',
                target_type: 'user',
                query: 'member@example.com',
                from: '2026-01-01T00:00:00Z',
                to: '2026-01-31T23:59:59Z',
                limit: 50,
                offset: 100
            })
            .subscribe((response) => {
                expect(response.entries.length).toBe(1);
                expect(response.entries[0].action).toBe('member.role_updated');
            });

        const req = httpMock.expectOne((request) => request.url === '/api/user/teams/team-id/audit');
        expect(req.request.method).toBe('GET');
        expect(req.request.params.get('action')).toBe('member.role_updated');
        expect(req.request.params.get('outcome')).toBe('success');
        expect(req.request.params.get('target_type')).toBe('user');
        expect(req.request.params.get('query')).toBe('member@example.com');
        expect(req.request.params.get('from')).toBe('2026-01-01T00:00:00Z');
        expect(req.request.params.get('to')).toBe('2026-01-31T23:59:59Z');
        expect(req.request.params.get('limit')).toBe('50');
        expect(req.request.params.get('offset')).toBe('100');
        req.flush({
            entries: [
                {
                    id: 'audit-1',
                    team_id: 'team-id',
                    action: 'member.role_updated',
                    details: 'Role changed from member to admin',
                    actor_email: 'owner@example.com',
                    target_email: 'member@example.com',
                    created_at: '2026-01-04T00:00:00Z'
                }
            ],
            total: 1,
            limit: 25,
            offset: 0,
            has_more: false
        });
    });

    it('should transfer team ownership', () => {
        service.transferTeamOwnership('team-id', 'user-id').subscribe((response) => {
            expect(response.status).toBe('ok');
        });

        const req = httpMock.expectOne('/api/user/teams/team-id/transfer-ownership');
        expect(req.request.method).toBe('POST');
        expect(req.request.body).toEqual({ target_user_id: 'user-id' });
        req.flush({ status: 'ok' });
    });

    it('should leave team and update local state', () => {
        service.teams.set([
            { id: 'team-a', name: 'Team A', logo_url: '', role: 'owner', created_at: '2026-01-01T00:00:00Z' },
            { id: 'team-b', name: 'Team B', logo_url: '', role: 'admin', created_at: '2026-01-02T00:00:00Z' }
        ]);
        service.activeTeamId.set('team-b');

        service.leaveTeam('team-b').subscribe((response) => {
            expect(response.status).toBe('ok');
        });

        const req = httpMock.expectOne('/api/user/teams/team-b/leave');
        expect(req.request.method).toBe('DELETE');
        req.flush({
            status: 'ok',
            active_team_id: 'team-a'
        });

        expect(service.activeTeamId()).toBe('team-a');
        expect(service.teams().map((team) => team.id)).toEqual(['team-a']);
    });

    it('should archive team and update local state', () => {
        service.teams.set([
            { id: 'team-a', name: 'Team A', logo_url: '', role: 'owner', created_at: '2026-01-01T00:00:00Z' },
            { id: 'team-b', name: 'Team B', logo_url: '', role: 'owner', created_at: '2026-01-02T00:00:00Z' }
        ]);
        service.activeTeamId.set('team-b');

        service.archiveTeam('team-b').subscribe((response) => {
            expect(response.status).toBe('ok');
        });

        const req = httpMock.expectOne('/api/user/teams/team-b/archive');
        expect(req.request.method).toBe('POST');
        expect(req.request.body).toEqual({});
        req.flush({
            status: 'ok',
            active_team_id: 'team-a'
        });

        expect(service.activeTeamId()).toBe('team-a');
        expect(service.teams().map((team) => team.id)).toEqual(['team-a']);
    });
});
