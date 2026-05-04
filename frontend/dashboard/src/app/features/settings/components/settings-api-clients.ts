import { ChangeDetectionStrategy, Component, computed, effect, inject, input, signal } from '@angular/core';

import { toSignal } from '@angular/core/rxjs-interop';
import { AbstractControl, FormControl, FormGroup, ReactiveFormsModule, ValidationErrors, ValidatorFn, Validators } from '@angular/forms';
import { finalize, forkJoin } from 'rxjs';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';

import { ConfirmationService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { ConfirmPopupModule } from 'primeng/confirmpopup';
import { InputTextModule } from 'primeng/inputtext';
import { SelectModule } from 'primeng/select';
import { TableModule } from 'primeng/table';

import { SettingsCard } from '@features/settings/components/settings-card';
import { CopyControl } from '@components/copy-control/copy-control';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { APIClient, APIClientSiteRole, APIClientsService, CreateAPIClientRequest, InstanceRole, SiteRole } from '@services/api-clients.service';
import { PermissionService } from '@services/permission.service';

interface SelectOption<TValue extends string> {
    label: string;
    value: TValue;
}

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
    imports: [ReactiveFormsModule, ButtonModule, ConfirmPopupModule, InputTextModule, SelectModule, TableModule, SettingsCard, CopyControl, RelativeDateTime, TranslocoPipe],
    templateUrl: './settings-api-clients.html',
    styleUrl: './settings-api-clients.css',
    changeDetection: ChangeDetectionStrategy.OnPush,
    providers: [ConfirmationService]
})
export class SettingsAPIClients {
    private readonly apiClientsService = inject(APIClientsService);
    private readonly confirmationService = inject(ConfirmationService);
    private readonly perms = inject(PermissionService);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    readonly scope = input<'personal' | 'team'>('personal');
    readonly teamId = input<string | null>(null);

    protected readonly isLoading = signal(false);
    protected readonly isSaving = signal(false);
    protected readonly error = signal<string | null>(null);
    protected readonly success = signal<string | null>(null);
    protected readonly createdToken = signal<string | null>(null);
    protected readonly editingClientID = signal<string | null>(null);

    protected readonly clients = signal<APIClient[]>([]);
    protected readonly sites = signal<{ id: string; domain: string }[]>([]);
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

    protected readonly siteOptions = computed<SelectOption<string>[]>(() => {
        return this.sites()
            .map((site) => ({ label: site.domain, value: site.id }))
            .sort((a, b) => a.label.localeCompare(b.label, 'en', { sensitivity: 'base' }));
    });

    protected readonly isEditing = computed(() => this.editingClientID() !== null);
    protected readonly isTeamScope = computed(() => this.scope() === 'team');
    protected readonly titleKey = computed(() => (this.isTeamScope() ? 'settings.apiClients.teamTitle' : 'settings.apiClients.title'));
    protected readonly descriptionKey = computed(() => (this.isTeamScope() ? 'settings.apiClients.teamDescription' : 'settings.apiClients.description'));

    constructor() {
        effect(() => {
            this.scope();
            this.teamId();
            this.reload();
        });
    }

    protected reload(): void {
        this.isLoading.set(true);
        this.error.set(null);

        forkJoin({
            clients: this.apiClientsService.listClients(this.teamIdForRequests()),
            sites: this.apiClientsService.listSites()
        })
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: ({ clients, sites }) => {
                    this.clients.set(clients);
                    this.sites.set(sites);
                },
                error: () => {
                    this.error.set('settings.apiClients.errors.loadFailed');
                }
            });
    }

    protected submit(): void {
        this.error.set(null);
        this.form.controls.expiresAt.updateValueAndValidity({ emitEvent: false });

        if (this.form.invalid) {
            this.form.markAllAsTouched();
            return;
        }

        const payload = this.buildPayload();
        if (!payload) {
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
                        this.resetForm();
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
                    this.resetForm();
                },
                error: () => {
                    this.error.set('settings.apiClients.errors.updateFailed');
                }
            });
    }

    protected startEdit(client: APIClient): void {
        this.editingClientID.set(client.id);
        this.error.set(null);
        this.success.set(null);
        this.createdToken.set(null);

        this.form.controls.name.setValue(client.name);
        this.form.controls.description.setValue(client.description ?? '');
        this.form.controls.instanceRole.setValue(this.resolveEditableInstanceRole(client.instance_role));
        this.form.controls.expiresAt.setValue(this.toDateTimeLocal(client.expires_at));
        this.selectedSiteRoles.set((client.site_roles ?? []).map((entry) => ({ ...entry })));
        this.siteRoleForm.reset({ siteID: null, role: 'viewer' });
    }

    protected cancelEdit(): void {
        this.resetForm();
        this.error.set(null);
        this.success.set(null);
    }

    protected confirmDeleteClient(event: Event, client: APIClient): void {
        this.confirmationService.confirm({
            key: 'api-client-delete',
            target: event.currentTarget as EventTarget,
            message: this.transloco.translate('settings.apiClients.confirmDelete', { name: client.name }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: {
                label: this.transloco.translate('common.actions.cancel'),
                severity: 'secondary',
                outlined: true
            },
            acceptButtonProps: {
                label: this.transloco.translate('settings.apiClients.actions.delete'),
                severity: 'danger'
            },
            accept: () => this.deleteClient(client)
        });
    }

    private deleteClient(client: APIClient): void {
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
                        this.resetForm();
                    }
                },
                error: () => {
                    this.error.set('settings.apiClients.errors.deleteFailed');
                }
            });
    }

    protected toggleRevoked(client: APIClient): void {
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
        return this.sites().find((site) => site.id === siteID)?.domain ?? siteID;
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
