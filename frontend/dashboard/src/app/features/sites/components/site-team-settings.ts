import { ChangeDetectionStrategy, Component, computed, effect, inject, input, signal } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { compatForm } from '@angular/forms/signals/compat';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CrudDialog } from '@components/crud-dialog/crud-dialog';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { TableRowActionItem, TableRowActions } from '@components/table-row-actions/table-row-actions';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { Site } from '@models/analytics.types';
import { AccessService } from '@services/access.service';
import { TeamService } from '@services/team.service';
import { SiteService } from '@features/sites/services/site.service';
import { NavigationNoticeService } from '@services/navigation-notice.service';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { IconFieldModule } from '@openng/optimus-ui/iconfield';
import { InputIconModule } from '@openng/optimus-ui/inputicon';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MessageModule } from '@openng/optimus-ui/message';
import { SelectModule } from '@openng/optimus-ui/select';
import { TableModule } from '@openng/optimus-ui/table';

interface SiteMember {
    id: string;
    user_id: string;
    email: string;
    role: string;
    added_at: string;
}

@Component({
    selector: 'app-site-team-settings',
    imports: [ReactiveFormsModule, ConfirmDialogModule, TableModule, ButtonModule, SelectModule, IconFieldModule, InputIconModule, InputTextModule, MessageModule, CrudDialog, RelativeDateTime, TableRowActions, TranslocoPipe],
    providers: [ConfirmationService],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <p-confirmdialog />
        <app-crud-dialog
            [title]="'sites.team.addMemberDialogTitle' | transloco"
            [visible]="isAddMemberDialogVisible()"
            (visibleChange)="onAddMemberDialogVisibleChange($event)"
            [submitLabel]="'sites.team.addMemberAction' | transloco"
            [cancelLabel]="'common.actions.cancel' | transloco"
            submitIcon="pi pi-user-plus"
            [saving]="isAdding()"
            (submitted)="addMember()"
        >
            <form class="site-settings-dialog-form" (ngSubmit)="addMember()">
                <div class="site-settings-field-grid site-settings-member-grid">
                    <div class="site-settings-field">
                        <label for="member-email">{{ 'common.emailAddress' | transloco }}</label>
                        <input
                            id="member-email"
                            pInputText
                            [formControl]="memberForm.email().control()"
                            [placeholder]="'sites.team.emailPlaceholder' | transloco"
                            [class.ng-invalid]="memberForm.email().touched() && memberForm.email().invalid()"
                            [class.ng-dirty]="memberForm.email().dirty()"
                            inputmode="email"
                            autocapitalize="none"
                            spellcheck="false"
                            class="w-full"
                        />
                    </div>

                    <div class="site-settings-field">
                        <label for="member-role">{{ 'common.columns.role' | transloco }}</label>
                        <p-select inputId="member-role" [options]="roleOptions()" [formControl]="memberForm.role().control()" optionLabel="label" optionValue="value" appendTo="body" class="w-full" />
                    </div>
                </div>
                @if (memberErrorKey(); as key) {
                    <p-message severity="error" [text]="key | transloco" />
                }
            </form>
        </app-crud-dialog>
        <div class="site-settings-stack">
            @if (canManageTeam() && availableTransferTeams().length) {
                <section class="site-settings-card">
                    <header class="site-settings-card__header">
                        <div class="site-settings-card__title-row">
                            <span class="site-settings-card__icon"><i class="pi pi-arrow-right-arrow-left" aria-hidden="true"></i></span>
                            <div>
                                <h3>{{ 'sites.team.transfer.title' | transloco }}</h3>
                                <p>{{ 'sites.team.transfer.description' | transloco }}</p>
                            </div>
                        </div>
                    </header>
                    <div class="site-settings-card__body">
                        @if (transferSuccessKey(); as key) {
                            <p-message severity="success" [text]="key | transloco" />
                        }
                        @if (transferErrorKey(); as key) {
                            <p-message severity="error" [text]="key | transloco" />
                        }
                        <div class="site-settings-field">
                            <label for="site-transfer-team">{{ 'sites.team.transfer.teamLabel' | transloco }}</label>
                            <p-select
                                inputId="site-transfer-team"
                                [options]="availableTransferTeams()"
                                [formControl]="transferForm.teamId().control()"
                                optionLabel="label"
                                optionValue="value"
                                [placeholder]="'sites.team.transfer.teamPlaceholder' | transloco"
                                class="w-full"
                            />
                        </div>
                    </div>
                    <footer class="site-settings-card__footer">
                        <p-button
                            styleClass="site-settings-action-btn"
                            [label]="'sites.team.transfer.action' | transloco"
                            icon="pi pi-arrow-right-arrow-left"
                            [loading]="isTransferring()"
                            [disabled]="isTransferring() || transferForm().invalid()"
                            (onClick)="transferSite()"
                        />
                    </footer>
                </section>
            }

            <section class="site-settings-card">
                <header class="site-settings-card__header">
                    <div class="site-settings-card__title-row">
                        <span class="site-settings-card__icon"><i class="pi pi-users" aria-hidden="true"></i></span>
                        <div>
                            <h3>{{ 'sites.settings.tabs.access' | transloco }}</h3>
                        </div>
                    </div>
                </header>
                <div class="site-settings-card__body">
                    @if (memberSuccessKey(); as key) {
                        <p-message severity="success" [text]="key | transloco" />
                    }
                    @if (memberErrorKey() && !isAddMemberDialogVisible()) {
                        <p-message severity="error" [text]="memberErrorKey() | transloco" />
                    }
                    <div class="site-settings-table-shell">
                        <div class="site-settings-table-toolbar">
                            <span class="site-settings-chip">{{ members().length }}</span>
                            <div class="site-settings-table-actions">
                                @if (canManageTeam()) {
                                    <p-button styleClass="site-settings-action-btn" [label]="'sites.team.addMemberAction' | transloco" icon="pi pi-user-plus" [type]="'button'" (onClick)="openAddMemberDialog()" />
                                }
                                <p-iconfield class="hk-crud-search">
                                    <p-inputicon class="pi pi-search" />
                                    <input pInputText #memberSearch [placeholder]="'common.searchPlaceholder' | transloco" (input)="membersTable.filterGlobal($any($event.target).value, 'contains')" class="w-full" />
                                </p-iconfield>
                            </div>
                        </div>
                        <div class="hk-crud-table-wrap">
                            <p-table #membersTable [value]="members()" [loading]="isLoading()" [globalFilterFields]="['email', 'role', 'added_at']" [sortField]="'added_at'" [sortOrder]="-1" styleClass="hk-crud-table p-datatable-sm">
                                <ng-template pTemplate="header">
                                    <tr>
                                        <th pSortableColumn="email">
                                            {{ 'common.columns.email' | transloco }}
                                            <p-sortIcon field="email" />
                                        </th>
                                        <th pSortableColumn="role">
                                            {{ 'common.columns.role' | transloco }}
                                            <p-sortIcon field="role" />
                                        </th>
                                        <th pSortableColumn="added_at">
                                            {{ 'common.columns.added' | transloco }}
                                            <p-sortIcon field="added_at" />
                                        </th>
                                        @if (canManageTeam()) {
                                            <th>{{ 'common.columns.actions' | transloco }}</th>
                                        }
                                    </tr>
                                </ng-template>

                                <ng-template pTemplate="body" let-member>
                                    <tr>
                                        <td>{{ member.email }}</td>
                                        <td>
                                            <span [class]="getRoleBadgeClass(member.role)">
                                                {{ getRoleLabel(member.role) }}
                                            </span>
                                        </td>
                                        <td><app-relative-date-time [value]="member.added_at" /></td>
                                        @if (canManageTeam()) {
                                            <td>
                                                <app-table-row-actions [items]="memberActions(member)" />
                                            </td>
                                        }
                                    </tr>
                                </ng-template>
                            </p-table>
                        </div>
                    </div>
                </div>
            </section>
        </div>
    `
})
export class SiteTeamSettings {
    private http = inject(HttpClient);
    private confirmationService = inject(ConfirmationService);
    private transloco = inject(TranslocoService);
    private teamService = inject(TeamService);
    private siteService = inject(SiteService);
    private router = inject(Router);
    private navigationNotice = inject(NavigationNoticeService);
    private access = inject(AccessService);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    site = input.required<Site | null>();

    protected members = signal<SiteMember[]>([]);
    protected isLoading = signal(false);
    protected isAdding = signal(false);
    protected isAddMemberDialogVisible = signal(false);
    protected isTransferring = signal(false);
    protected memberErrorKey = signal<string | null>(null);
    protected memberSuccessKey = signal<string | null>(null);
    protected transferErrorKey = signal<string | null>(null);
    protected transferSuccessKey = signal<string | null>(null);

    private readonly memberFormModel = signal({
        email: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.email] }),
        role: new FormControl('viewer', { nonNullable: true, validators: [Validators.required] })
    });
    protected readonly memberForm = compatForm(this.memberFormModel);
    private readonly transferFormModel = signal({
        teamId: new FormControl('', { nonNullable: true, validators: [Validators.required] })
    });
    protected readonly transferForm = compatForm(this.transferFormModel);

    protected roleOptions = computed(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('roles.owner'), value: 'owner' },
            { label: this.transloco.translate('roles.admin'), value: 'admin' },
            { label: this.transloco.translate('roles.editor'), value: 'editor' },
            { label: this.transloco.translate('roles.viewer'), value: 'viewer' }
        ];
    });
    protected readonly availableTransferTeams = computed(() => {
        if (!this.canManageTeam()) return [];
        this.activeLanguage();
        const currentTeamId = this.teamService.activeTeamId();
        return this.teamService
            .teams()
            .filter((team) => team.id !== currentTeamId && (team.role === 'owner' || team.role === 'admin'))
            .map((team) => ({
                label: team.name,
                value: team.id
            }));
    });
    protected readonly canManageTeam = computed(() => {
        const currentSite = this.site();
        return !!currentSite && this.access.canSite(currentSite.id, SITE_CAPABILITIES.manageTeam);
    });

    constructor() {
        // Automatically reload members whenever the 'site' input signal changes
        effect(() => {
            const currentSite = this.site();
            if (currentSite) {
                this.loadMembers(currentSite.id);
            } else {
                this.members.set([]);
            }
        });
    }

    loadMembers(siteId: string) {
        this.isLoading.set(true);
        this.http.get<SiteMember[]>(`/api/sites/${siteId}/members`).subscribe({
            next: (members) => {
                this.members.set(members);
                this.isLoading.set(false);
            },
            error: (err) => {
                console.error('Failed to load members', err);
                this.isLoading.set(false);
            }
        });
    }

    openAddMemberDialog() {
        if (!this.canManageTeam()) return;
        this.memberErrorKey.set(null);
        this.isAddMemberDialogVisible.set(true);
    }

    onAddMemberDialogVisibleChange(visible: boolean) {
        this.isAddMemberDialogVisible.set(visible);
        if (!visible) {
            this.resetMemberForm();
        }
    }

    addMember() {
        const siteId = this.site()?.id;
        const email = this.memberForm.email().value().trim();
        const role = this.memberForm.role().value();
        if (!siteId || !this.canManageTeam()) return;
        if (this.memberForm().invalid() || !email) {
            this.memberForm.email().markAsTouched();
            this.memberForm.role().markAsTouched();
            return;
        }

        this.isAdding.set(true);
        this.memberErrorKey.set(null);
        this.memberSuccessKey.set(null);
        this.http
            .post(`/api/sites/${siteId}/members`, {
                email,
                role
            })
            .subscribe({
                next: () => {
                    this.closeAddMemberDialog();
                    this.memberSuccessKey.set('sites.team.addMemberSuccess');
                    this.isAdding.set(false);
                    this.loadMembers(siteId);
                },
                error: (err) => {
                    console.error('Failed to add member', err);
                    this.isAdding.set(false);
                    this.memberErrorKey.set('sites.team.errors.addFailed');
                }
            });
    }

    transferSite() {
        const siteId = this.site()?.id;
        const teamId = this.transferForm.teamId().value().trim();
        if (!siteId || !teamId || !this.canManageTeam()) return;

        this.isTransferring.set(true);
        this.transferErrorKey.set(null);
        this.transferSuccessKey.set(null);

        this.http
            .post(`/api/sites/${siteId}/transfer-team`, {
                team_id: teamId
            })
            .subscribe({
                next: () => {
                    this.transferForm.teamId().control().reset('');
                    this.transferSuccessKey.set('sites.team.transfer.success');
                    this.siteService.sites.update((sites) => sites.filter((site) => site.id !== siteId));
                    if (this.siteService.activeSite()?.id === siteId) {
                        this.siteService.activeSite.set(null);
                    }
                    this.siteService.loadSites();
                    this.isTransferring.set(false);
                    void this.router.navigate(['/overview']).then((navigated) => {
                        if (navigated) this.navigationNotice.show('sites.settings.notices.siteTransferred');
                    });
                },
                error: (error: unknown) => {
                    if (error instanceof HttpErrorResponse && error.status === 403) {
                        this.transferErrorKey.set('sites.team.transfer.errors.forbidden');
                    } else {
                        this.transferErrorKey.set('sites.team.transfer.errors.generic');
                    }
                    this.isTransferring.set(false);
                }
            });
    }

    memberActions(member: SiteMember): TableRowActionItem[] {
        this.activeLanguage();
        return [
            {
                label: this.transloco.translate('teams.management.removeAction'),
                icon: 'pi pi-trash',
                danger: true,
                command: () => this.confirmRemoveMember(member)
            }
        ];
    }

    confirmRemoveMember(member: SiteMember) {
        const siteId = this.site()?.id;
        if (!siteId || !this.canManageTeam()) return;

        this.confirmationService.confirm({
            message: this.transloco.translate('sites.team.confirmRemove', { email: member.email }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('teams.management.removeAction')),
            accept: () => {
                this.memberErrorKey.set(null);
                this.memberSuccessKey.set(null);
                this.http.delete(`/api/sites/${siteId}/members/${member.user_id}`).subscribe({
                    next: () => {
                        this.memberSuccessKey.set('sites.team.removeMemberSuccess');
                        this.loadMembers(siteId);
                    },
                    error: (err) => {
                        console.error('Failed to remove member', err);
                        this.memberErrorKey.set('sites.team.errors.removeFailed');
                    }
                });
            }
        });
    }

    private closeAddMemberDialog() {
        this.isAddMemberDialogVisible.set(false);
        this.resetMemberForm();
    }

    private resetMemberForm() {
        this.memberForm.email().control().reset('');
        this.memberForm.role().control().reset('viewer');
    }

    getRoleLabel(role: string): string {
        return this.roleOptions().find((r) => r.value === role)?.label || role;
    }

    getRoleBadgeClass(role: string): string {
        switch (role) {
            case 'owner':
                return 'site-settings-role-chip site-settings-role-chip--owner';
            case 'admin':
                return 'site-settings-role-chip site-settings-role-chip--admin';
            case 'editor':
                return 'site-settings-role-chip site-settings-role-chip--editor';
            case 'viewer':
                return 'site-settings-role-chip site-settings-role-chip--viewer';
            default:
                return 'site-settings-role-chip site-settings-role-chip--viewer';
        }
    }
}
