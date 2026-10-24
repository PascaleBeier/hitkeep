import { ChangeDetectionStrategy, Component, computed, DestroyRef, effect, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { TextareaModule } from '@openng/optimus-ui/textarea';
import { MessageModule } from '@openng/optimus-ui/message';
import { finalize } from 'rxjs';

import { PageFrame } from '@components/page-frame/page-frame';
import { CrudDialog } from '@components/crud-dialog/crud-dialog';
import { DialogShell } from '@components/dialog-shell/dialog-shell';
import { AppTable, AppTableCell, AppTableColumn } from '@components/table/table';
import { TableRowActionItem } from '@components/table-row-actions/table-row-actions';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { TEAM_CAPABILITIES } from '@core/access/capabilities';
import { OAuthClientRegistration } from '@models/oauth.types';
import { OAuthService } from '@services/oauth.service';
import { TeamService } from '@services/team.service';
import { AccessService } from '@services/access.service';

@Component({
    selector: 'app-oauth-clients',
    imports: [PageFrame, CrudDialog, DialogShell, AppTable, AppTableCell, ReactiveFormsModule, TranslocoPipe, ButtonModule, ConfirmDialogModule, InputTextModule, TextareaModule, MessageModule],
    providers: [ConfirmationService],
    templateUrl: './oauth-clients.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class OAuthClientsPage {
    private readonly oauth = inject(OAuthService);
    private readonly teams = inject(TeamService);
    private readonly access = inject(AccessService);
    private readonly transloco = inject(TranslocoService);
    private readonly confirmations = inject(ConfirmationService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly refresh = signal(0);
    protected readonly canManage = computed(() => !!this.teams.activeTeam() && this.access.canActiveTeam(TEAM_CAPABILITIES.manageApiClients));
    protected readonly clients = signal<OAuthClientRegistration[]>([]);
    protected readonly loading = signal(false);
    protected readonly pending = signal(false);
    protected readonly loadError = signal<string | null>(null);
    protected readonly error = signal<string | null>(null);
    protected readonly success = signal<string | null>(null);
    protected readonly visible = signal(false);
    protected readonly editing = signal<string | null>(null);
    protected readonly selectedClient = signal<OAuthClientRegistration | null>(null);
    protected readonly columns: AppTableColumn<OAuthClientRegistration>[] = [
        { field: 'client_name', headerKey: 'common.columns.name', frozen: true },
        { field: 'client_id', headerKey: 'oauth.applicationId' }
    ];
    protected readonly actions = (client: OAuthClientRegistration): TableRowActionItem[] => [
        { label: this.transloco.translate('oauth.connectionDetails'), icon: 'pi pi-info-circle', command: () => this.showDetails(client) },
        { label: this.transloco.translate('common.actions.edit'), icon: 'pi pi-pencil', command: () => this.open(client) },
        { label: this.transloco.translate('oauth.revoke'), icon: 'pi pi-ban', danger: true, command: () => this.confirmRevoke(client) }
    ];
    protected readonly form = new FormGroup({
        name: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.maxLength(128)] }),
        uri: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(2048)] }),
        redirects: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.maxLength(32784)] })
    });

    constructor() {
        effect((onCleanup) => {
            const team = this.teams.activeTeam();
            const allowed = this.canManage();
            this.refresh();
            this.clients.set([]);
            this.visible.set(false);
            this.selectedClient.set(null);
            this.confirmations.close();
            this.error.set(null);
            this.success.set(null);
            this.loadError.set(null);
            this.loading.set(false);
            if (!team || !allowed) return;
            this.loading.set(true);
            const subscription = this.oauth.clients(team.id).subscribe({
                next: (clients) => {
                    this.clients.set(clients);
                    this.loading.set(false);
                },
                error: () => {
                    this.loadError.set('oauth.loadFailed');
                    this.loading.set(false);
                }
            });
            onCleanup(() => subscription.unsubscribe());
        });
    }

    protected reload(): void {
        this.refresh.update((n) => n + 1);
    }

    protected open(client?: OAuthClientRegistration): void {
        if (!this.canManage() || this.pending()) return;
        this.selectedClient.set(null);
        this.editing.set(client?.client_id ?? null);
        this.form.reset({ name: client?.client_name ?? '', uri: client?.client_uri ?? '', redirects: client?.redirect_uris.join('\n') ?? '' });
        this.error.set(null);
        this.visible.set(true);
    }

    protected showDetails(client: OAuthClientRegistration): void {
        if (!this.canManage() || this.pending()) return;
        this.selectedClient.set(client);
    }

    protected submit(): void {
        const team = this.teams.activeTeam();
        if (!team || !this.canManage() || this.pending()) return;
        this.form.markAllAsTouched();
        if (this.form.invalid) return;
        const value = this.form.getRawValue();
        this.pending.set(true);
        this.error.set(null);
        this.success.set(null);
        this.oauth
            .saveClient(team.id, this.editing(), {
                client_name: value.name.trim(),
                client_uri: value.uri.trim(),
                redirect_uris: value.redirects
                    .split('\n')
                    .map((uri) => uri.trim())
                    .filter(Boolean)
            })
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.pending.set(false))
            )
            .subscribe({
                next: (client) => {
                    if (team.id !== this.teams.activeTeam()?.id) return;
                    this.clients.update((clients) => [client, ...clients.filter((c) => c.client_id !== client.client_id)]);
                    this.visible.set(false);
                    this.success.set('oauthClients.saved');
                },
                error: () => {
                    if (team.id === this.teams.activeTeam()?.id) this.error.set('oauthClients.saveFailed');
                }
            });
    }

    protected confirmRevoke(client: OAuthClientRegistration): void {
        const team = this.teams.activeTeam();
        if (!team || !this.canManage() || this.pending()) return;
        this.confirmations.confirm({
            header: this.transloco.translate('oauthClients.revokeTitle'),
            message: this.transloco.translate('oauthClients.revokeConfirm', { application: client.client_name }),
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('oauth.revoke')),
            accept: () => {
                if (team.id !== this.teams.activeTeam()?.id || !this.canManage() || this.pending()) return;
                this.pending.set(true);
                this.error.set(null);
                this.success.set(null);
                this.oauth
                    .revokeClient(team.id, client.client_id)
                    .pipe(
                        takeUntilDestroyed(this.destroyRef),
                        finalize(() => this.pending.set(false))
                    )
                    .subscribe({
                        next: () => {
                            if (team.id !== this.teams.activeTeam()?.id) return;
                            this.clients.update((clients) => clients.filter((c) => c.client_id !== client.client_id));
                            if (this.selectedClient()?.client_id === client.client_id) this.selectedClient.set(null);
                            this.success.set('oauthClients.revoked');
                        },
                        error: () => {
                            if (team.id === this.teams.activeTeam()?.id) this.error.set('oauth.revokeFailed');
                        }
                    });
            }
        });
    }
}
