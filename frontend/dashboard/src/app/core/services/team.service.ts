import { Injectable, computed, inject, signal } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { finalize, of, tap } from 'rxjs';
import { Team, TeamAuditListResponse, TeamInvite, TeamMember, TeamRole, UserTeamsResponse } from '@models/analytics.types';

interface SetActiveTeamResponse {
    status: string;
    active_team_id: string;
    recent_team_ids?: string[];
}

interface UpsertTeamMemberRequest {
    email: string;
    role: TeamRole;
}

interface UpsertTeamMemberResponse {
    status: string;
    is_invite: boolean;
    invite?: TeamInvite;
}

interface RemoveTeamMemberResponse {
    status: string;
}

interface TeamActionErrorResponse {
    status: string;
    code: string;
    message: string;
}

interface LeaveTeamResponse {
    status: string;
    active_team_id: string;
    recent_team_ids?: string[];
}

@Injectable({ providedIn: 'root' })
export class TeamService {
    private http = inject(HttpClient);

    readonly teams = signal<Team[]>([]);
    readonly activeTeamId = signal<string>('');
    readonly isLoading = signal(false);
    readonly isSwitching = signal(false);

    readonly activeTeam = computed(() => this.teams().find((team) => team.id === this.activeTeamId()) ?? null);
    readonly hasMultipleTeams = computed(() => this.teams().length > 1);

    loadTeams() {
        this.isLoading.set(true);
        return this.http.get<UserTeamsResponse>('/api/user/teams').pipe(
            tap((response) => this.applyTeams(response)),
            finalize(() => this.isLoading.set(false))
        );
    }

    applyTeams(response: UserTeamsResponse) {
        const teams = response.teams ?? [];
        this.teams.set(teams);
        this.activeTeamId.set(response.active_team_id || teams[0]?.id || '');
    }

    setActiveTeam(teamID: string) {
        if (!teamID || teamID === this.activeTeamId()) {
            return of({
                status: 'ok',
                active_team_id: this.activeTeamId()
            });
        }

        this.isSwitching.set(true);
        return this.http.put<SetActiveTeamResponse>('/api/user/teams/active', { team_id: teamID }).pipe(
            tap((response) => {
                this.activeTeamId.set(response.active_team_id || teamID);
            }),
            finalize(() => this.isSwitching.set(false))
        );
    }

    listTeamMembers(teamID: string) {
        return this.http.get<TeamMember[]>(`/api/user/teams/${encodeURIComponent(teamID)}/members`);
    }

    listTeamInvites(teamID: string) {
        return this.http.get<TeamInvite[]>(`/api/user/teams/${encodeURIComponent(teamID)}/invites`);
    }

    upsertTeamMember(teamID: string, payload: UpsertTeamMemberRequest) {
        return this.http.post<UpsertTeamMemberResponse>(`/api/user/teams/${encodeURIComponent(teamID)}/members`, payload);
    }

    resendTeamInvite(teamID: string, inviteID: string) {
        return this.http.post<{ status: string; invite: TeamInvite }>(`/api/user/teams/${encodeURIComponent(teamID)}/invites/${encodeURIComponent(inviteID)}/resend`, {});
    }

    revokeTeamInvite(teamID: string, inviteID: string) {
        return this.http.delete<RemoveTeamMemberResponse>(`/api/user/teams/${encodeURIComponent(teamID)}/invites/${encodeURIComponent(inviteID)}`);
    }

    removeTeamMember(teamID: string, userID: string) {
        return this.http.delete<RemoveTeamMemberResponse>(`/api/user/teams/${encodeURIComponent(teamID)}/members/${encodeURIComponent(userID)}`);
    }

    listTeamAudit(
        teamID: string,
        params: {
            action?: string;
            outcome?: string;
            target_type?: string;
            query?: string;
            from?: string;
            to?: string;
            limit?: number;
            offset?: number;
        } = {}
    ) {
        let httpParams = new HttpParams();
        if (params.action) {
            httpParams = httpParams.set('action', params.action);
        }
        if (params.outcome) {
            httpParams = httpParams.set('outcome', params.outcome);
        }
        if (params.target_type) {
            httpParams = httpParams.set('target_type', params.target_type);
        }
        if (params.query) {
            httpParams = httpParams.set('query', params.query);
        }
        if (params.from) {
            httpParams = httpParams.set('from', params.from);
        }
        if (params.to) {
            httpParams = httpParams.set('to', params.to);
        }
        if (typeof params.limit === 'number') {
            httpParams = httpParams.set('limit', String(params.limit));
        }
        if (typeof params.offset === 'number') {
            httpParams = httpParams.set('offset', String(params.offset));
        }

        return this.http.get<TeamAuditListResponse>(`/api/user/teams/${encodeURIComponent(teamID)}/audit`, { params: httpParams });
    }

    transferTeamOwnership(teamID: string, targetUserID: string) {
        return this.http.post<{ status: string }>(`/api/user/teams/${encodeURIComponent(teamID)}/transfer-ownership`, {
            target_user_id: targetUserID
        });
    }

    leaveTeam(teamID: string) {
        this.isSwitching.set(true);
        return this.http.delete<LeaveTeamResponse>(`/api/user/teams/${encodeURIComponent(teamID)}/leave`).pipe(
            tap((response) => {
                this.teams.update((teams) => teams.filter((team) => team.id !== teamID));
                this.activeTeamId.set(response.active_team_id || this.teams()[0]?.id || '');
            }),
            finalize(() => this.isSwitching.set(false))
        );
    }

    archiveTeam(teamID: string) {
        this.isSwitching.set(true);
        return this.http.post<LeaveTeamResponse>(`/api/user/teams/${encodeURIComponent(teamID)}/archive`, {}).pipe(
            tap((response) => {
                this.teams.update((teams) => teams.filter((team) => team.id !== teamID));
                this.activeTeamId.set(response.active_team_id || this.teams()[0]?.id || '');
            }),
            finalize(() => this.isSwitching.set(false))
        );
    }

    createTeam(payload: { name: string; logo_url: string }) {
        return this.http.post<{ team: Team }>('/api/user/teams', payload).pipe(
            tap((response) => {
                this.teams.update((teams) => [...teams, response.team]);
                this.activeTeamId.set(response.team.id);
            })
        );
    }

    updateTeam(teamID: string, payload: { name: string; logo_url: string }) {
        return this.http.patch<{ status: string }>(`/api/user/teams/${encodeURIComponent(teamID)}`, payload).pipe(
            tap(() => {
                this.teams.update((teams) => teams.map((t) => (t.id === teamID ? { ...t, name: payload.name, logo_url: payload.logo_url } : t)));
            })
        );
    }
}

export type { TeamActionErrorResponse, LeaveTeamResponse, UpsertTeamMemberResponse };
