import { ChangeDetectionStrategy, Component, computed, effect, inject, input, signal } from '@angular/core';
import { RouterLink } from '@angular/router';

import { toSignal } from '@angular/core/rxjs-interop';
import { AbstractControl, FormControl, FormGroup, ReactiveFormsModule, ValidationErrors, ValidatorFn, Validators } from '@angular/forms';
import { finalize } from 'rxjs';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';

import { ConfirmationService } from '@openng/optimus-ui/api';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { dialogCancelButton, dialogDangerButton, dialogPrimaryButton } from '@components/dialog-actions/dialog-actions';
import { IconFieldModule } from '@openng/optimus-ui/iconfield';
import { InputIconModule } from '@openng/optimus-ui/inputicon';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { SelectModule } from '@openng/optimus-ui/select';
import { TableModule } from '@openng/optimus-ui/table';
import { TagModule } from '@openng/optimus-ui/tag';

import { SettingsCard } from '@features/settings/components/settings-card';
import { CrudDialog } from '@components/crud-dialog/crud-dialog';
import { CrudTableToolbar } from '@components/crud-table-toolbar/crud-table-toolbar';
import { OneTimeCredential } from '@components/one-time-credential/one-time-credential';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { SiteScopeSummary, SiteScopeSummaryItem } from '@components/site-scope-summary/site-scope-summary';
import { TableRowActionItem, TableRowActions } from '@components/table-row-actions/table-row-actions';
import { APIClient, APIClientSiteRole, APIClientsService, CreateAPIClientRequest, InstanceRole, SiteRole } from '@services/api-clients.service';
import { PermissionService } from '@services/permission.service';
import { SiteSelectOption } from '@features/sites/components/site-select-option';
import { SiteService } from '@features/sites/services/site.service';

interface SelectOption<TValue extends string> {
    label: string;
    value: TValue;
}

type APIClientStatus = 'active' | 'inactive' | 'expired';
type APIClientFeedbackRegion = 'dialog' | 'list';

const truncateToMinute = (value: Date): number => {
    const copy = new Date(value);
    copy.setSeconds(0, 0);
    return copy.getTime();
};

const expiresAtNotPastValidator = (): ValidatorFn => {
    return (control: AbstractControl<string | null>): ValidationErrors | null => {
        const value = control.value;
        if (!value) {
            return null;
        }

        const parsed = new Date(value);
        if (Number.isNaN(parsed.getTime())) {
            return { invalidDateTime: true };
        }

        if (truncateToMinute(parsed) < truncateToMinute(new Date())) {
            return { pastDateTime: true };
        }

        return null;
    };
};

@Component({
    selector: 'app-settings-api-clients',
    imports: [
        RouterLink,
        ReactiveFormsModule,
        ButtonModule,
        ConfirmDialogModule,
        IconFieldModule,
        InputIconModule,
        InputTextModule,
        SelectModule,
        TableModule,
        TagModule,
        SettingsCard,
        CrudDialog,
        CrudTableToolbar,
        OneTimeCredential,
        RelativeDateTime,
        SiteScopeSummary,
        TableRowActions,
        SiteSelectOption,
        TranslocoPipe
    ],
    templateUrl: './settings-api-clients.html',
    styleUrl: './settings-api-clients.css',
    changeDetection: ChangeDetectionStrategy.OnPush,
    providers: [ConfirmationService]
})
export class SettingsAPIClients {
    private readonly apiClientsService = inject(APIClientsService);
    private readonly confirmationService = inject(ConfirmationService);
    private readonly perms = inject(PermissionService);
    private readonly siteService = inject(SiteService);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    readonly scope = input<'personal' | 'team'>('personal');
    readonly teamId = input<string | null>(null);
    readonly showTeamClientsLink = input(false);

    protected readonly isLoading = signal(false);
    protected readonly isSaving = signal(false);
    protected readonly error = signal<string | null>(null);
    protected readonly success = signal<string | null>(null);
    protected readonly createdToken = signal<string | null>(null);
    protected readonly feedbackRegion = signal<APIClientFeedbackRegion>('list');
    protected readonly isFormDialogVisible = signal(false);
    protected readonly editingClientID = signal<string | null>(null);

    protected readonly clients = signal<APIClient[]>([]);
    protected readonly selectedSiteRoles = signal<APIClientSiteRole[]>([]);

    protected readonly form = new FormGroup({
        name: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.maxLength(120)] }),
        description: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(500)] }),
        instanceRole: new FormControl<InstanceRole>('user', { nonNullable: true, validators: [Validators.required] }),
        expiresAt: new FormControl<string | null>(null, { validators: [expiresAtNotPastValidator()] })
    });

    protected readonly siteRoleForm = new FormGroup({
        siteID: new FormControl<string | null>(null, { validators: [Validators.required] }),
        role: new FormControl<SiteRole>('viewer', { nonNullable: true, validators: [Validators.required] })
    });

    protected readonly maxInstanceRole = computed<InstanceRole>(() => {
        const current = this.perms.permissions()?.instance_role;
        if (current === 'owner' || current === 'admin' || current === 'user') {
            return current;
        }
        return 'user';
    });

    protected readonly instanceRoleOptions = computed<SelectOption<InstanceRole>[]>(() => {
        this.activeLanguage();
        const all: SelectOption<InstanceRole>[] = [
            { label: this.transloco.translate('admin.roles.instanceOwner'), value: 'owner' },
            { label: this.transloco.translate('admin.roles.instanceAdmin'), value: 'admin' },
            { label: this.transloco.translate('admin.roles.user'), value: 'user' }
        ];

        switch (this.maxInstanceRole()) {
            case 'owner':
                return all;
            case 'admin':
                return all.filter((entry) => entry.value !== 'owner');
            default:
                return all.filter((entry) => entry.value === 'user');
        }
    });

    protected readonly siteRoleOptions = computed<SelectOption<SiteRole>[]>(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('roles.owner'), value: 'owner' },
            { label: this.transloco.translate('roles.admin'), value: 'admin' },
            { label: this.transloco.translate('roles.editor'), value: 'editor' },
            { label: this.transloco.translate('roles.viewer'), value: 'viewer' }
        ];
    });

    protected readonly siteOptions = computed(() => this.siteService.sites());

    protected readonly isEditing = computed(() => this.editingClientID() !== null);
    protected readonly isTeamScope = computed(() => this.scope() === 'team');
    protected readonly titleKey = computed(() => (this.isTeamScope() ? 'settings.apiClients.teamTitle' : 'settings.apiClients.title'));
    protected readonly descriptionKey = computed(() => (this.isTeamScope() ? 'settings.apiClients.teamDescription' : 'settings.apiClients.description'));
    protected readonly dialogTitleKey = computed(() => (this.isEditing() ? 'settings.apiClients.editDialogTitle' : 'settings.apiClients.createDialogTitle'));
    protected readonly dialogSubmitLabelKey = computed(() => (this.isEditing() ? 'settings.apiClients.actions.save' : 'settings.apiClients.actions.create'));
    protected readonly dialogError = computed(() => (this.feedbackRegion() === 'dialog' ? this.error() : null));
    protected readonly listError = computed(() => (this.feedbackRegion() === 'list' ? this.error() : null));
    protected readonly listSuccess = computed(() => (this.feedbackRegion() === 'list' ? this.success() : null));
    protected readonly listCreatedToken = computed(() => (this.feedbackRegion() === 'list' ? this.createdToken() : null));

    constructor() {
        effect(() => {
            this.scope();
            this.teamId();
            this.reload();
        });
    }

    protected reload(): void {
        this.feedbackRegion.set('list');
        this.isLoading.set(true);
        this.error.set(null);

        this.apiClientsService
            .listClients(this.teamIdForRequests())
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: (clients) => {
                    this.clients.set(clients);
                },
                error: () => {
                    this.error.set('settings.apiClients.errors.loadFailed');
                }
            });
    }

    protected submit(): void {
        if (this.isSaving()) {
            return;
        }
        this.feedbackRegion.set('dialog');
        this.error.set(null);
        this.form.controls.expiresAt.updateValueAndValidity({ emitEvent: false });

        if (this.form.invalid) {
            this.form.markAllAsTouched();
            return;
        }

        const payload = this.buildPayload();
        if (!payload) {
            this.feedbackRegion.set('dialog');
            this.error.set('settings.apiClients.errors.invalidExpiration');
            return;
        }

        const editingClientID = this.editingClientID();
        this.error.set(null);
        this.success.set(null);
        this.isSaving.set(true);

        if (!editingClientID) {
            this.apiClientsService
                .createClient(payload, this.teamIdForRequests())
                .pipe(finalize(() => this.isSaving.set(false)))
                .subscribe({
                    next: (resp) => {
                        this.clients.update((current) => [resp.client, ...current]);
                        this.createdToken.set(resp.token);
                        this.success.set('settings.apiClients.messages.created');
                        this.closeFormDialog();
                        this.feedbackRegion.set('list');
                    },
                    error: () => {
                        this.error.set('settings.apiClients.errors.createFailed');
                    }
                });
            return;
        }

        const existing = this.clients().find((client) => client.id === editingClientID);
        if (!existing) {
            this.isSaving.set(false);
            this.feedbackRegion.set('dialog');
            this.error.set('settings.apiClients.errors.notFound');
            return;
        }

        this.apiClientsService
            .updateClient(
                editingClientID,
                {
                    ...payload,
                    revoked: Boolean(existing.revoked_at)
                },
                this.teamIdForRequests()
            )
            .pipe(finalize(() => this.isSaving.set(false)))
            .subscribe({
                next: (updated) => {
                    this.clients.update((current) => current.map((entry) => (entry.id === updated.id ? updated : entry)));
                    this.success.set('settings.apiClients.messages.updated');
                    this.createdToken.set(null);
                    this.closeFormDialog();
                    this.feedbackRegion.set('list');
                },
                error: () => {
                    this.error.set('settings.apiClients.errors.updateFailed');
                }
            });
    }

    protected startEdit(client: APIClient): void {
        this.feedbackRegion.set('dialog');
        this.editingClientID.set(client.id);
        this.error.set(null);
        this.success.set(null);
        this.createdToken.set(null);
        this.isFormDialogVisible.set(true);

        this.form.controls.name.setValue(client.name);
        this.form.controls.description.setValue(client.description ?? '');
        this.form.controls.instanceRole.setValue(this.resolveEditableInstanceRole(client.instance_role));
        this.form.controls.expiresAt.setValue(this.toDateTimeLocal(client.expires_at));
        this.selectedSiteRoles.set((client.site_roles ?? []).map((entry) => ({ ...entry })));
        this.siteRoleForm.reset({ siteID: null, role: 'viewer' });
    }

    protected cancelEdit(): void {
        this.closeFormDialog();
    }

    protected openCreateDialog(): void {
        this.resetForm();
        this.feedbackRegion.set('dialog');
        this.error.set(null);
        this.success.set(null);
        this.createdToken.set(null);
        this.isFormDialogVisible.set(true);
    }

    protected onFormDialogVisibleChange(visible: boolean): void {
        if (!visible && this.isSaving()) {
            this.isFormDialogVisible.set(true);
            return;
        }
        this.isFormDialogVisible.set(visible);
        if (!visible) {
            this.closeFormDialog();
        }
    }

    protected apiClientActions(client: APIClient): TableRowActionItem[] {
        this.activeLanguage();
        return [
            {
                label: this.transloco.translate('settings.apiClients.actions.edit'),
                icon: 'pi pi-pencil',
                command: () => this.startEdit(client)
            },
            {
                label: this.transloco.translate('settings.apiClients.actions.rollToken'),
                icon: 'pi pi-refresh',
                disabled: !this.canRotateClient(client) || this.isSaving(),
                command: () => this.confirmRotateClient(client)
            },
            {
                label: this.transloco.translate(client.revoked_at ? 'settings.apiClients.actions.reactivate' : 'settings.apiClients.actions.revoke'),
                icon: client.revoked_at ? 'pi pi-lock-open' : 'pi pi-lock',
                disabled: this.isSaving(),
                command: () => this.toggleRevoked(client)
            },
            { separator: true },
            {
                label: this.transloco.translate('settings.apiClients.actions.delete'),
                icon: 'pi pi-trash',
                danger: true,
                disabled: this.isSaving(),
                command: () => this.confirmDeleteClient(client)
            }
        ];
    }

    protected confirmDeleteClient(client: APIClient): void {
        this.confirmationService.confirm({
            message: this.transloco.translate('settings.apiClients.confirmDelete', { name: client.name }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('settings.apiClients.actions.delete')),
            accept: () => this.deleteClient(client)
        });
    }

    protected confirmRotateClient(client: APIClient): void {
        if (!this.canRotateClient(client)) {
            return;
        }
        this.confirmationService.confirm({
            message: this.transloco.translate('settings.apiClients.confirmRotate', { name: client.name }),
            icon: 'pi pi-refresh',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogPrimaryButton(this.transloco.translate('settings.apiClients.actions.rollToken')),
            accept: () => this.rotateClient(client)
        });
    }

    private deleteClient(client: APIClient): void {
        this.feedbackRegion.set('list');
        this.error.set(null);
        this.success.set(null);
        this.isSaving.set(true);
        this.apiClientsService
            .deleteClient(client.id, this.teamIdForRequests())
            .pipe(finalize(() => this.isSaving.set(false)))
            .subscribe({
                next: () => {
                    this.clients.update((current) => current.filter((entry) => entry.id !== client.id));
                    this.success.set('settings.apiClients.messages.deleted');
                    if (this.editingClientID() === client.id) {
                        this.closeFormDialog();
                    }
                },
                error: () => {
                    this.error.set('settings.apiClients.errors.deleteFailed');
                }
            });
    }

    protected toggleRevoked(client: APIClient): void {
        this.feedbackRegion.set('list');
        this.error.set(null);
        this.success.set(null);
        this.isSaving.set(true);

        this.apiClientsService
            .updateClient(
                client.id,
                {
                    name: client.name,
                    description: client.description ?? '',
                    instance_role: this.resolveEditableInstanceRole(client.instance_role),
                    expires_at: client.expires_at ?? null,
                    revoked: !client.revoked_at,
                    site_roles: (client.site_roles ?? []).map((entry) => ({ ...entry }))
                },
                this.teamIdForRequests()
            )
            .pipe(finalize(() => this.isSaving.set(false)))
            .subscribe({
                next: (updated) => {
                    this.clients.update((current) => current.map((entry) => (entry.id === updated.id ? updated : entry)));
                    this.success.set(updated.revoked_at ? 'settings.apiClients.messages.revoked' : 'settings.apiClients.messages.reactivated');
                },
                error: () => {
                    this.error.set('settings.apiClients.errors.updateFailed');
                }
            });
    }

    protected rotateClient(client: APIClient): void {
        if (!this.canRotateClient(client)) {
            return;
        }
        this.feedbackRegion.set('list');
        this.error.set(null);
        this.success.set(null);
        this.isSaving.set(true);

        this.apiClientsService
            .rotateClient(client.id, this.teamIdForRequests())
            .pipe(finalize(() => this.isSaving.set(false)))
            .subscribe({
                next: (resp) => {
                    this.clients.update((current) => current.map((entry) => (entry.id === resp.client.id ? resp.client : entry)));
                    this.createdToken.set(resp.token);
                    this.success.set('settings.apiClients.messages.rotated');
                    if (this.editingClientID() === resp.client.id) {
                        this.closeFormDialog();
                    }
                },
                error: () => {
                    this.error.set('settings.apiClients.errors.rotateFailed');
                }
            });
    }

    protected canRotateClient(client: APIClient): boolean {
        return !client.revoked_at && !this.isExpired(client);
    }

    protected clientStatus(client: APIClient): APIClientStatus {
        if (client.revoked_at) {
            return 'inactive';
        }
        if (this.isExpired(client)) {
            return 'expired';
        }
        return 'active';
    }

    protected clientStatusIconClass(client: APIClient): string {
        switch (this.clientStatus(client)) {
            case 'inactive':
                return 'pi pi-ban hk-status-icon hk-status-icon--error';
            case 'expired':
                return 'pi pi-clock hk-status-icon hk-status-icon--warn';
            case 'active':
            default:
                return 'pi pi-check-circle hk-status-icon hk-status-icon--ok';
        }
    }

    protected clientStatusLabelKey(client: APIClient): string {
        return `settings.apiClients.status.${this.clientStatus(client)}`;
    }

    protected isExpired(client: APIClient): boolean {
        if (!client.expires_at) {
            return false;
        }
        const parsed = new Date(client.expires_at);
        return !Number.isNaN(parsed.getTime()) && parsed.getTime() <= Date.now();
    }

    protected emptyGrantLabelKey(client: APIClient): string {
        if (!this.isTeamScope() && (client.instance_role === 'owner' || client.instance_role === 'admin')) {
            return 'settings.apiClients.noSiteAccessInstanceOnly';
        }
        return 'settings.apiClients.noSiteAccess';
    }

    protected addSiteScope(): void {
        if (this.siteRoleForm.invalid) {
            this.siteRoleForm.markAllAsTouched();
            return;
        }

        const siteID = this.siteRoleForm.controls.siteID.value;
        const role = this.siteRoleForm.controls.role.value;
        if (!siteID || !role) {
            return;
        }

        this.selectedSiteRoles.update((current) => {
            const existingIndex = current.findIndex((entry) => entry.site_id === siteID);
            if (existingIndex >= 0) {
                const next = [...current];
                next[existingIndex] = { site_id: siteID, role };
                return next;
            }
            return [...current, { site_id: siteID, role }];
        });

        this.siteRoleForm.reset({ siteID: null, role: 'viewer' });
    }

    protected removeSiteScope(siteID: string): void {
        this.selectedSiteRoles.update((current) => current.filter((entry) => entry.site_id !== siteID));
    }

    protected siteDomain(siteID: string): string {
        return this.siteService.sites().find((site) => site.id === siteID)?.domain ?? siteID;
    }

    protected siteGrantSummaryItems(client: APIClient): SiteScopeSummaryItem[] {
        this.activeLanguage();
        return client.site_roles.map((scope) => ({
            id: scope.site_id,
            label: this.siteDomain(scope.site_id),
            detail: this.transloco.translate(`roles.${scope.role}`),
            severity: this.siteGrantSeverity(scope.role)
        }));
    }

    protected siteGrantCountLabel(count: number): string {
        this.activeLanguage();
        return this.transloco.translate('settings.apiClients.siteGrantCount', { count });
    }

    protected siteGrantSeverity(role: SiteRole): 'success' | 'info' | 'secondary' | 'contrast' {
        switch (role) {
            case 'owner':
                return 'contrast';
            case 'admin':
                return 'info';
            case 'editor':
                return 'success';
            case 'viewer':
            default:
                return 'secondary';
        }
    }

    protected expiresAtMin(): string {
        return this.toDateTimeLocal(new Date().toISOString()) ?? '';
    }

    private buildPayload(): CreateAPIClientRequest | null {
        const expiresAtRaw = this.form.controls.expiresAt.value;
        let expiresAt: string | null = null;

        if (expiresAtRaw) {
            const parsed = new Date(expiresAtRaw);
            if (Number.isNaN(parsed.getTime())) {
                return null;
            }
            expiresAt = parsed.toISOString();
        }

        const role = this.form.controls.instanceRole.value;
        const resolvedRole = this.isTeamScope() ? 'user' : this.resolveEditableInstanceRole(role);

        return {
            name: (this.form.controls.name.value ?? '').trim(),
            description: (this.form.controls.description.value ?? '').trim(),
            instance_role: resolvedRole,
            expires_at: expiresAt,
            site_roles: [...this.selectedSiteRoles()]
        };
    }

    private resolveEditableInstanceRole(role: string | null | undefined): InstanceRole {
        if (this.isTeamScope()) {
            return 'user';
        }
        const options = this.instanceRoleOptions().map((entry) => entry.value);
        const fallback = options[options.length - 1] ?? 'user';
        if (role === 'owner' || role === 'admin' || role === 'user') {
            return options.includes(role) ? role : fallback;
        }
        return fallback;
    }

    private resetForm(): void {
        this.editingClientID.set(null);
        this.selectedSiteRoles.set([]);
        this.form.reset({
            name: '',
            description: '',
            instanceRole: this.resolveEditableInstanceRole(this.maxInstanceRole()),
            expiresAt: null
        });
        this.siteRoleForm.reset({ siteID: null, role: 'viewer' });
    }

    private closeFormDialog(): void {
        this.isFormDialogVisible.set(false);
        this.resetForm();
        this.error.set(null);
    }

    private toDateTimeLocal(value: string | null | undefined): string | null {
        if (!value) return null;
        const parsed = new Date(value);
        if (Number.isNaN(parsed.getTime())) return null;
        const pad = (n: number) => `${n}`.padStart(2, '0');
        const year = parsed.getFullYear();
        const month = pad(parsed.getMonth() + 1);
        const day = pad(parsed.getDate());
        const hour = pad(parsed.getHours());
        const minute = pad(parsed.getMinutes());
        return `${year}-${month}-${day}T${hour}:${minute}`;
    }

    private teamIdForRequests(): string | null {
        return this.isTeamScope() ? this.teamId() : null;
    }
}
