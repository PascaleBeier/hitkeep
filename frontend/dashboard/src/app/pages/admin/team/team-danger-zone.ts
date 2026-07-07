import { HttpErrorResponse } from '@angular/common/http';
import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { finalize } from 'rxjs';
import { ButtonModule } from 'primeng/button';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';

import { DialogShell } from '@components/dialog-shell/dialog-shell';
import { TEAM_CAPABILITIES } from '@core/access/capabilities';
import { SiteService } from '@features/sites/services/site.service';
import { AccessService } from '@services/access.service';
import { PermissionService } from '@services/permission.service';
import { TeamActionErrorResponse, TeamService } from '@services/team.service';

type TeamDangerAction = 'leave' | 'archive';

@Component({
    selector: 'app-team-danger-zone',
    imports: [ButtonModule, DialogShell, FormsModule, InputTextModule, MessageModule, TranslocoPipe],
    templateUrl: './team-danger-zone.html',
    styleUrl: './team-danger-zone.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TeamDangerZonePage {
    private readonly router = inject(Router);
    private readonly access = inject(AccessService);
    private readonly siteService = inject(SiteService);
    private readonly perms = inject(PermissionService);
    protected readonly teamService = inject(TeamService);
    protected readonly team = this.teamService.activeTeam;

    protected readonly isLeaving = signal(false);
    protected readonly isArchiving = signal(false);
    protected readonly leaveErrorKey = signal('');
    protected readonly leaveSuccessKey = signal('');
    protected readonly archiveErrorKey = signal('');
    protected readonly archiveSuccessKey = signal('');
    protected readonly pendingAction = signal<TeamDangerAction | null>(null);
    protected readonly confirmValue = signal('');

    protected readonly canArchive = computed(() => this.access.canActiveTeam(TEAM_CAPABILITIES.archive));
    protected readonly isBusy = computed(() => this.isLeaving() || this.isArchiving());
    protected readonly confirmDialogVisible = computed(() => this.pendingAction() !== null);
    protected readonly confirmTeamName = computed(() => this.team()?.name ?? '');
    protected readonly canSubmitConfirm = computed(() => {
        const name = this.confirmTeamName();
        return name.length > 0 && this.confirmValue().trim() === name;
    });
    protected readonly confirmTitleKey = computed(() => {
        switch (this.pendingAction()) {
            case 'leave':
                return 'admin.team.danger.leaveConfirmTitle';
            case 'archive':
                return 'admin.team.danger.archiveConfirmTitle';
            default:
                return '';
        }
    });
    protected readonly confirmActionKey = computed(() => {
        switch (this.pendingAction()) {
            case 'leave':
                return 'admin.team.settings.leaveAction';
            case 'archive':
                return 'admin.team.settings.archiveAction';
            default:
                return '';
        }
    });

    protected openConfirmDialog(action: TeamDangerAction): void {
        if (this.isBusy()) {
            return;
        }
        if (action === 'archive' && !this.canArchive()) {
            return;
        }
        this.confirmValue.set('');
        this.pendingAction.set(action);
    }

    protected onConfirmDialogVisibleChange(visible: boolean): void {
        if (!visible && !this.isBusy()) {
            this.pendingAction.set(null);
            this.confirmValue.set('');
        }
    }

    protected runConfirmedAction(): void {
        if (!this.canSubmitConfirm()) {
            return;
        }
        switch (this.pendingAction()) {
            case 'leave':
                this.leaveTeam();
                return;
            case 'archive':
                this.archiveTeam();
                return;
        }
    }

    private leaveTeam(): void {
        if (this.isLeaving()) {
            return;
        }

        const t = this.team();
        if (!t) {
            return;
        }

        this.leaveErrorKey.set('');
        this.leaveSuccessKey.set('');
        this.isLeaving.set(true);

        this.teamService
            .leaveTeam(t.id)
            .pipe(finalize(() => this.isLeaving.set(false)))
            .subscribe({
                next: () => {
                    this.leaveSuccessKey.set('admin.team.settings.leaveSuccess');
                    this.pendingAction.set(null);
                    this.confirmValue.set('');
                    this.refreshTeamContext();
                },
                error: (error: unknown) => {
                    const errorCode = this.extractTeamErrorCode(error);
                    if (errorCode === 'team_last_owner') {
                        this.leaveErrorKey.set('admin.team.settings.leaveErrors.lastOwner');
                        return;
                    }
                    if (errorCode === 'user_only_team') {
                        this.leaveErrorKey.set('admin.team.settings.leaveErrors.onlyTeam');
                        return;
                    }
                    if (error instanceof HttpErrorResponse && error.status === 403) {
                        this.leaveErrorKey.set('teams.management.errors.forbidden');
                        return;
                    }
                    this.leaveErrorKey.set('admin.team.settings.leaveErrors.generic');
                }
            });
    }

    private archiveTeam(): void {
        if (this.isArchiving()) {
            return;
        }

        const t = this.team();
        if (!t || !this.canArchive()) {
            return;
        }

        this.archiveErrorKey.set('');
        this.archiveSuccessKey.set('');
        this.isArchiving.set(true);

        this.teamService
            .archiveTeam(t.id)
            .pipe(finalize(() => this.isArchiving.set(false)))
            .subscribe({
                next: () => {
                    this.archiveSuccessKey.set('admin.team.settings.archiveSuccess');
                    this.pendingAction.set(null);
                    this.confirmValue.set('');
                    this.refreshTeamContext();
                },
                error: (error: unknown) => {
                    const errorCode = this.extractTeamErrorCode(error);
                    if (errorCode === 'team_archive_has_sites') {
                        this.archiveErrorKey.set('admin.team.settings.archiveErrors.hasSites');
                        return;
                    }
                    if (errorCode === 'team_archive_default_forbidden') {
                        this.archiveErrorKey.set('admin.team.settings.archiveErrors.defaultTeam');
                        return;
                    }
                    if (errorCode === 'team_archive_forbidden') {
                        this.archiveErrorKey.set('admin.team.settings.archiveErrors.forbidden');
                        return;
                    }
                    this.archiveErrorKey.set('admin.team.settings.archiveErrors.generic');
                }
            });
    }

    private refreshTeamContext(): void {
        this.siteService.sites.set([]);
        this.siteService.activeSite.set(null);
        this.siteService.loadSites();
        this.perms.loadPermissions().subscribe({ error: () => undefined });
        this.teamService.loadTeams().subscribe({
            next: () => {
                this.router.navigateByUrl('/dashboard');
            },
            error: () => this.router.navigateByUrl('/dashboard')
        });
    }

    private extractTeamErrorCode(error: unknown): string | null {
        if (!(error instanceof HttpErrorResponse)) {
            return null;
        }
        const body = error.error;
        if (!body || typeof body !== 'object') {
            return null;
        }
        const actionError = body as Partial<TeamActionErrorResponse>;
        return typeof actionError.code === 'string' ? actionError.code : null;
    }
}
