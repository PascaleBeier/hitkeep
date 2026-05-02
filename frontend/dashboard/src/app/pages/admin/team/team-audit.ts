import { HttpErrorResponse } from '@angular/common/http';
import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import { finalize } from 'rxjs';

import { AuditTableComponent } from '@components/audit-table/audit-table';
import { AuditTableQuery } from '@components/audit-table/audit-table.types';
import { TeamAuditEntry } from '@models/analytics.types';
import { AuditPresentationService } from '@services/audit-presentation.service';
import { TeamService } from '@services/team.service';

@Component({
    selector: 'app-team-audit',
    imports: [AuditTableComponent, TranslocoPipe],
    templateUrl: './team-audit.html',
    styleUrl: './team-audit.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TeamAuditPage {
    private readonly teamService = inject(TeamService);
    private readonly presentation = inject(AuditPresentationService);
    private readonly activeLoadedTeamID = signal<string | null>(null);
    private auditRequestID = 0;

    protected readonly team = this.teamService.activeTeam;
    protected readonly rows = signal<TeamAuditEntry[]>([]);
    protected readonly isLoading = signal(false);
    protected readonly errorKey = signal<string | null>(null);
    protected readonly total = signal(0);
    protected readonly query = signal<AuditTableQuery>({
        limit: 25,
        offset: 0
    });

    protected readonly actionOptions = computed(() => this.presentation.actionOptions('team'));
    protected readonly outcomeOptions = computed(() => this.presentation.outcomeOptions());
    protected readonly targetTypeOptions = computed(() => this.presentation.targetTypeOptions());

    protected readonly canViewAudit = computed(() => {
        const role = this.team()?.role;
        return role === 'owner' || role === 'admin';
    });

    constructor() {
        effect(() => {
            const teamID = this.team()?.id ?? null;
            if (this.activeLoadedTeamID() !== teamID) {
                this.activeLoadedTeamID.set(teamID);
                this.query.update((current) => ({
                    ...current,
                    offset: 0
                }));
                return;
            }

            if (!teamID || !this.canViewAudit()) {
                this.rows.set([]);
                this.total.set(0);
                this.errorKey.set(null);
                return;
            }

            this.loadAudit(teamID, this.query());
        });
    }

    protected onQueryChange(query: AuditTableQuery) {
        this.query.set(query);
    }

    protected refresh() {
        const teamID = this.team()?.id ?? null;
        if (!teamID || !this.canViewAudit()) {
            return;
        }
        this.loadAudit(teamID, this.query());
    }

    private loadAudit(teamID: string, query: AuditTableQuery) {
        const requestID = ++this.auditRequestID;
        this.errorKey.set(null);
        this.isLoading.set(true);
        this.teamService
            .listTeamAudit(teamID, query)
            .pipe(
                finalize(() => {
                    if (requestID === this.auditRequestID) {
                        this.isLoading.set(false);
                    }
                })
            )
            .subscribe({
                next: (response) => {
                    if (requestID !== this.auditRequestID) {
                        return;
                    }
                    this.rows.set(response.entries);
                    this.total.set(response.total);
                },
                error: (error: unknown) => {
                    if (requestID !== this.auditRequestID) {
                        return;
                    }
                    if (error instanceof HttpErrorResponse && error.status === 403) {
                        this.errorKey.set('admin.team.audit.forbidden');
                        return;
                    }
                    this.errorKey.set('admin.team.audit.loadError');
                }
            });
    }
}
