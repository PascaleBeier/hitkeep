import { ChangeDetectionStrategy, Component, effect, inject, input, signal } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { FormControl, FormGroup, FormsModule, ReactiveFormsModule, Validators } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ConfirmationService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { IconFieldModule } from 'primeng/iconfield';
import { InputIconModule } from 'primeng/inputicon';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';
import { TableModule } from 'primeng/table';
import { TagModule } from 'primeng/tag';
import { ToggleSwitchModule } from 'primeng/toggleswitch';
import { finalize } from 'rxjs';

import { CopyControl } from '@components/copy-control/copy-control';
import { CrudDialog } from '@components/crud-dialog/crud-dialog';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { TableRowActionItem, TableRowActions } from '@components/table-row-actions/table-row-actions';
import { SettingsCard } from '@features/settings/components/settings-card';
import { CustomTrackingDomain, CustomTrackingDomainStatus } from '@models/analytics.types';
import { TeamService } from '@services/team.service';

const trackingHostnamePattern = /^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$/i;

@Component({
    selector: 'app-team-tracking-domains',
    standalone: true,
    imports: [
        ReactiveFormsModule,
        FormsModule,
        ButtonModule,
        SettingsCard,
        CopyControl,
        CrudDialog,
        IconFieldModule,
        InputIconModule,
        InputTextModule,
        MessageModule,
        RelativeDateTime,
        TableModule,
        TableRowActions,
        TagModule,
        ToggleSwitchModule,
        TranslocoPipe
    ],
    template: `
        <app-crud-dialog
            [title]="'admin.team.settings.trackingDomains.addAction' | transloco"
            [visible]="isAddDialogVisible()"
            (visibleChange)="onAddDialogVisibleChange($event)"
            [submitLabel]="'admin.team.settings.trackingDomains.addAction' | transloco"
            [cancelLabel]="'common.actions.cancel' | transloco"
            submitIcon="pi pi-plus"
            [saving]="isAdding()"
            (submitted)="submitAdd()"
        >
            <form class="site-settings-dialog-form" [formGroup]="form" (ngSubmit)="submitAdd()">
                <div class="site-settings-field">
                    <label for="team-tracking-domain-hostname">{{ 'admin.team.settings.trackingDomains.hostnameLabel' | transloco }}</label>
                    <input
                        id="team-tracking-domain-hostname"
                        type="text"
                        pInputText
                        class="w-full"
                        formControlName="hostname"
                        [placeholder]="'admin.team.settings.trackingDomains.hostnamePlaceholder' | transloco"
                        autocomplete="off"
                        autocapitalize="none"
                        spellcheck="false"
                    />
                    @if (hostnameControl.touched && hostnameControl.hasError('required')) {
                        <p-message severity="error" variant="simple" size="small">{{ 'admin.team.settings.trackingDomains.hostnameRequired' | transloco }}</p-message>
                    } @else if (hostnameControl.touched && hostnameControl.hasError('maxlength')) {
                        <p-message severity="error" variant="simple" size="small">{{ 'admin.team.settings.trackingDomains.hostnameTooLong' | transloco }}</p-message>
                    } @else if (hostnameControl.touched && hostnameControl.hasError('pattern')) {
                        <p-message severity="error" variant="simple" size="small">{{ 'admin.team.settings.trackingDomains.hostnameInvalid' | transloco }}</p-message>
                    }
                </div>
                @if (dialogErrorKey(); as key) {
                    <p-message severity="error">{{ key | transloco }}</p-message>
                }
            </form>
        </app-crud-dialog>

        <app-crud-dialog
            [title]="'admin.team.settings.trackingDomains.setupAction' | transloco"
            [visible]="isSetupDialogVisible()"
            (visibleChange)="onSetupDialogVisibleChange($event)"
            [submitLabel]="'admin.team.settings.saveAction' | transloco"
            [cancelLabel]="'common.actions.cancel' | transloco"
            [saving]="isSavingSetup()"
            (submitted)="submitSetup()"
        >
            @if (setupDomain(); as domain) {
                <div class="site-settings-stack">
                    <div>
                        <h3 class="m-0 break-all text-base font-bold">{{ domain.hostname }}</h3>
                        <p class="mt-1 mb-0 text-sm text-muted-color">{{ 'admin.team.settings.trackingDomains.setupDescription' | transloco }}</p>
                    </div>

                    <div class="grid gap-2.5">
                        <div class="grid items-center gap-2.5 sm:grid-cols-[minmax(7rem,10rem)_minmax(0,1fr)_auto]">
                            <span class="site-settings-field-label">{{ 'admin.team.settings.trackingDomains.dns.txtName' | transloco }}</span>
                            <code class="rounded-md border border-surface-200 bg-surface-50 px-2 py-1 text-xs break-all dark:border-surface-700 dark:bg-surface-800">{{ domain.dns_txt_name }}</code>
                            <app-copy-control [value]="domain.dns_txt_name" [text]="true" size="small" />
                        </div>
                        <div class="grid items-center gap-2.5 sm:grid-cols-[minmax(7rem,10rem)_minmax(0,1fr)_auto]">
                            <span class="site-settings-field-label">{{ 'admin.team.settings.trackingDomains.dns.txtValue' | transloco }}</span>
                            <code class="rounded-md border border-surface-200 bg-surface-50 px-2 py-1 text-xs break-all dark:border-surface-700 dark:bg-surface-800">{{ domain.dns_txt_value }}</code>
                            <app-copy-control [value]="domain.dns_txt_value" [text]="true" size="small" />
                        </div>
                        <div class="grid items-center gap-2.5 sm:grid-cols-[minmax(7rem,10rem)_minmax(0,1fr)_auto]">
                            <span class="site-settings-field-label">{{ 'admin.team.settings.trackingDomains.dns.target' | transloco }}</span>
                            <code class="rounded-md border border-surface-200 bg-surface-50 px-2 py-1 text-xs break-all dark:border-surface-700 dark:bg-surface-800">{{ domain.dns_target || '-' }}</code>
                            <app-copy-control [value]="domain.dns_target" [text]="true" size="small" />
                        </div>
                    </div>

                    <div class="site-settings-status-grid" [attr.aria-label]="'admin.team.settings.trackingDomains.checksLabel' | transloco">
                        <div>
                            <span>{{ 'admin.team.settings.trackingDomains.checks.ownership' | transloco }}</span>
                            <p-tag [severity]="statusSeverity(domain.verification_status)" [value]="statusKey(domain.verification_status) | transloco" />
                        </div>
                        <div>
                            <span>{{ 'admin.team.settings.trackingDomains.checks.target' | transloco }}</span>
                            <p-tag [severity]="statusSeverity(domain.target_status)" [value]="statusKey(domain.target_status) | transloco" />
                        </div>
                        <div>
                            <span>{{ 'admin.team.settings.trackingDomains.checks.tls' | transloco }}</span>
                            <p-tag [severity]="statusSeverity(domain.tls_status)" [value]="statusKey(domain.tls_status) | transloco" />
                        </div>
                    </div>

                    @if (domain.last_error) {
                        <p-message severity="error">{{ domain.last_error }}</p-message>
                    }

                    <label class="flex items-center justify-between gap-3" for="team-tracking-domain-enabled">
                        <span class="site-settings-field-label">{{ 'admin.team.settings.trackingDomains.enabled' | transloco }}</span>
                        <p-toggleswitch inputId="team-tracking-domain-enabled" [ngModel]="setupEnabled()" (ngModelChange)="setupEnabled.set($event)" />
                    </label>
                </div>
            }
        </app-crud-dialog>

        <app-settings-card [title]="'admin.team.settings.trackingDomains.title' | transloco" [subtitle]="'admin.team.settings.trackingDomains.description' | transloco" icon="pi pi-globe">
            <div settings-card-header class="flex flex-wrap items-center gap-2">
                <p-button [label]="'admin.team.settings.trackingDomains.addAction' | transloco" icon="pi pi-plus" [type]="'button'" (onClick)="openAddDialog()" />
            </div>

            <div settings-card-body class="flex flex-col gap-4">
                @if (successKey(); as key) {
                    <p-message severity="success">{{ key | transloco }}</p-message>
                }
                @if (errorKey(); as key) {
                    <p-message severity="error">{{ key | transloco }}</p-message>
                }

                @if (domains().length === 0 && !isLoading()) {
                    <div class="hk-empty-state">
                        <p class="text-sm text-[var(--p-text-muted-color)]">{{ 'admin.team.settings.trackingDomains.empty' | transloco }}</p>
                        <p-button [label]="'admin.team.settings.trackingDomains.addAction' | transloco" icon="pi pi-plus" [outlined]="true" [type]="'button'" (onClick)="openAddDialog()" />
                    </div>
                } @else {
                    <div class="hk-crud-table-wrap">
                        <p-table
                            #domainsTable
                            [value]="domains()"
                            [loading]="isLoading()"
                            [globalFilterFields]="['hostname']"
                            [sortField]="'hostname'"
                            [sortOrder]="1"
                            [paginator]="true"
                            [rows]="10"
                            [rowsPerPageOptions]="[10, 25, 50]"
                            [rowHover]="true"
                            responsiveLayout="scroll"
                            styleClass="hk-crud-table p-datatable-sm"
                            [tableStyle]="{ 'min-width': '880px' }"
                        >
                            <ng-template pTemplate="caption">
                                <div class="flex items-center justify-end gap-2">
                                    <p-iconfield class="hk-crud-search">
                                        <p-inputicon class="pi pi-search" />
                                        <input pInputText #domainsSearch [placeholder]="'common.searchPlaceholder' | transloco" (input)="domainsTable.filterGlobal($any($event.target).value, 'contains')" class="w-full" />
                                    </p-iconfield>
                                    <p-button icon="pi pi-refresh" [text]="true" [rounded]="true" [type]="'button'" [ariaLabel]="'common.actions.refresh' | transloco" [disabled]="isLoading()" (onClick)="loadDomains()" />
                                </div>
                            </ng-template>

                            <ng-template pTemplate="header">
                                <tr>
                                    <th pSortableColumn="hostname">
                                        {{ 'admin.team.settings.trackingDomains.hostnameLabel' | transloco }}
                                        <p-sortIcon field="hostname" />
                                    </th>
                                    <th class="text-center">{{ 'admin.team.settings.trackingDomains.checks.ownership' | transloco }}</th>
                                    <th class="text-center">{{ 'admin.team.settings.trackingDomains.checks.target' | transloco }}</th>
                                    <th class="text-center">{{ 'admin.team.settings.trackingDomains.checks.tls' | transloco }}</th>
                                    <th pSortableColumn="last_checked_at">
                                        {{ 'admin.team.settings.trackingDomains.lastChecked' | transloco }}
                                        <p-sortIcon field="last_checked_at" />
                                    </th>
                                    <th class="hk-actions-column">{{ 'common.columns.actions' | transloco }}</th>
                                </tr>
                            </ng-template>

                            <ng-template pTemplate="body" let-domain>
                                <tr>
                                    <td>
                                        <div class="flex flex-col gap-1">
                                            <span class="inline-flex min-w-0 items-center gap-2">
                                                <i [class]="domainStatusIcon(domain)" role="img" [attr.aria-label]="domainStatusLabelKey(domain) | transloco" [attr.title]="domainStatusLabelKey(domain) | transloco"></i>
                                                <span class="font-semibold break-all">{{ domain.hostname }}</span>
                                            </span>
                                            <span class="text-xs text-[var(--p-text-muted-color)]">{{ tlsModeKey(domain) | transloco }}</span>
                                        </div>
                                    </td>
                                    <td class="text-center">
                                        <i [class]="checkIcon(domain.verification_status)" role="img" [attr.aria-label]="statusKey(domain.verification_status) | transloco" [attr.title]="statusKey(domain.verification_status) | transloco"></i>
                                    </td>
                                    <td class="text-center">
                                        <i [class]="checkIcon(domain.target_status)" role="img" [attr.aria-label]="statusKey(domain.target_status) | transloco" [attr.title]="statusKey(domain.target_status) | transloco"></i>
                                    </td>
                                    <td class="text-center">
                                        <i [class]="checkIcon(domain.tls_status)" role="img" [attr.aria-label]="statusKey(domain.tls_status) | transloco" [attr.title]="statusKey(domain.tls_status) | transloco"></i>
                                    </td>
                                    <td class="whitespace-nowrap">
                                        @if (domain.last_checked_at) {
                                            <app-relative-date-time [value]="domain.last_checked_at" />
                                        } @else {
                                            -
                                        }
                                    </td>
                                    <td class="hk-actions-cell">
                                        <app-table-row-actions [items]="domainActions(domain)" [loading]="isBusy(domain.id)" />
                                    </td>
                                </tr>
                            </ng-template>

                            <ng-template pTemplate="emptymessage">
                                <tr>
                                    <td colspan="6" class="text-center text-muted-color py-4">{{ 'admin.team.settings.trackingDomains.empty' | transloco }}</td>
                                </tr>
                            </ng-template>
                        </p-table>
                    </div>
                }
            </div>
        </app-settings-card>
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TeamTrackingDomains {
    private readonly teamService = inject(TeamService);
    private readonly confirmationService = inject(ConfirmationService);
    private readonly transloco = inject(TranslocoService);
    private loadedTeamID = '';

    readonly teamId = input.required<string>();
    protected readonly form = new FormGroup({
        hostname: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.maxLength(253), Validators.pattern(trackingHostnamePattern)] })
    });
    protected readonly hostnameControl = this.form.controls.hostname;
    protected readonly domains = signal<CustomTrackingDomain[]>([]);
    protected readonly isLoading = signal(false);
    protected readonly isAdding = signal(false);
    protected readonly verifyingDomainID = signal('');
    protected readonly updatingDomainID = signal('');
    protected readonly deletingDomainID = signal('');
    protected readonly errorKey = signal('');
    protected readonly successKey = signal('');
    protected readonly dialogErrorKey = signal('');
    protected readonly isAddDialogVisible = signal(false);
    protected readonly isSetupDialogVisible = signal(false);
    protected readonly setupDomain = signal<CustomTrackingDomain | null>(null);
    protected readonly setupEnabled = signal(true);
    protected readonly isSavingSetup = signal(false);

    constructor() {
        effect(() => {
            const teamID = this.teamId();
            if (!teamID || teamID === this.loadedTeamID) {
                return;
            }
            this.loadedTeamID = teamID;
            this.loadDomains(teamID);
        });
    }

    protected loadDomains(teamID = this.teamId()): void {
        if (!teamID) return;
        this.errorKey.set('');
        this.isLoading.set(true);
        this.teamService
            .listTrackingDomains(teamID)
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: (domains) => this.domains.set(domains),
                error: () => this.errorKey.set('admin.team.settings.trackingDomains.errors.load')
            });
    }

    protected openAddDialog(): void {
        this.dialogErrorKey.set('');
        this.hostnameControl.reset('');
        this.isAddDialogVisible.set(true);
    }

    protected onAddDialogVisibleChange(visible: boolean): void {
        this.isAddDialogVisible.set(visible);
        if (!visible) {
            this.dialogErrorKey.set('');
            this.hostnameControl.reset('');
        }
    }

    protected submitAdd(): void {
        const teamID = this.teamId();
        const hostname = this.hostnameControl.value.trim();
        if (!teamID || this.isAdding()) return;
        if (!hostname || this.hostnameControl.invalid) {
            this.hostnameControl.markAsTouched();
            return;
        }

        this.errorKey.set('');
        this.successKey.set('');
        this.dialogErrorKey.set('');
        this.isAdding.set(true);
        this.teamService
            .createTrackingDomain(teamID, { hostname })
            .pipe(finalize(() => this.isAdding.set(false)))
            .subscribe({
                next: (domain) => {
                    this.upsertDomain(domain);
                    this.successKey.set('admin.team.settings.trackingDomains.added');
                    this.isAddDialogVisible.set(false);
                    this.hostnameControl.reset('');
                    this.openSetupDialog(domain);
                },
                error: (error: unknown) => {
                    this.dialogErrorKey.set(this.domainErrorKey(error, 'admin.team.settings.trackingDomains.errors.add'));
                }
            });
    }

    protected openSetupDialog(domain: CustomTrackingDomain): void {
        this.setupDomain.set(domain);
        this.setupEnabled.set(domain.enabled);
        this.isSetupDialogVisible.set(true);
    }

    protected onSetupDialogVisibleChange(visible: boolean): void {
        this.isSetupDialogVisible.set(visible);
        if (!visible) {
            this.setupDomain.set(null);
        }
    }

    protected submitSetup(): void {
        const domain = this.setupDomain();
        if (!domain || this.isSavingSetup()) return;
        if (this.setupEnabled() === domain.enabled) {
            this.isSetupDialogVisible.set(false);
            this.setupDomain.set(null);
            return;
        }
        this.isSavingSetup.set(true);
        this.updateEnabled(domain, this.setupEnabled(), () => {
            this.isSavingSetup.set(false);
            this.isSetupDialogVisible.set(false);
            this.setupDomain.set(null);
        });
    }

    protected verifyDomain(domain: CustomTrackingDomain): void {
        const teamID = this.teamId();
        if (!teamID || this.isBusy(domain.id)) return;

        this.errorKey.set('');
        this.successKey.set('');
        this.verifyingDomainID.set(domain.id);
        this.teamService
            .verifyTrackingDomain(teamID, domain.id)
            .pipe(finalize(() => this.verifyingDomainID.set('')))
            .subscribe({
                next: (updated) => {
                    this.upsertDomain(updated);
                    this.successKey.set(updated.active ? 'admin.team.settings.trackingDomains.verified' : 'admin.team.settings.trackingDomains.checked');
                },
                error: () => this.errorKey.set('admin.team.settings.trackingDomains.errors.verify')
            });
    }

    protected setEnabled(domain: CustomTrackingDomain, enabled: boolean): void {
        if (this.isBusy(domain.id)) return;
        this.updateEnabled(domain, enabled);
    }

    protected confirmDelete(domain: CustomTrackingDomain): void {
        if (this.isBusy(domain.id)) return;
        this.confirmationService.confirm({
            message: this.transloco.translate('admin.team.settings.trackingDomains.deleteConfirm', { hostname: domain.hostname }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('common.actions.delete')),
            accept: () => this.deleteDomain(domain)
        });
    }

    protected domainActions(domain: CustomTrackingDomain): TableRowActionItem[] {
        const busy = this.isBusy(domain.id);
        return [
            {
                label: this.transloco.translate('admin.team.settings.trackingDomains.verifyAction'),
                icon: 'pi pi-shield',
                disabled: busy,
                command: () => this.verifyDomain(domain)
            },
            {
                label: this.transloco.translate('admin.team.settings.trackingDomains.setupAction'),
                icon: 'pi pi-wrench',
                disabled: busy,
                command: () => this.openSetupDialog(domain)
            },
            {
                label: this.transloco.translate(domain.enabled ? 'admin.team.settings.trackingDomains.disableAction' : 'admin.team.settings.trackingDomains.enableAction'),
                icon: domain.enabled ? 'pi pi-pause' : 'pi pi-play',
                disabled: busy,
                command: () => this.setEnabled(domain, !domain.enabled)
            },
            {
                label: this.transloco.translate('common.actions.delete'),
                icon: 'pi pi-trash',
                danger: true,
                disabled: busy,
                command: () => this.confirmDelete(domain)
            }
        ];
    }

    protected isBusy(domainID: string): boolean {
        return this.verifyingDomainID() === domainID || this.updatingDomainID() === domainID || this.deletingDomainID() === domainID;
    }

    protected statusKey(status: CustomTrackingDomainStatus): string {
        return `admin.team.settings.trackingDomains.status.${status}`;
    }

    protected statusSeverity(status: CustomTrackingDomainStatus): 'success' | 'warn' | 'danger' | 'secondary' {
        switch (status) {
            case 'verified':
                return 'success';
            case 'failed':
                return 'danger';
            case 'pending':
                return 'warn';
            default:
                return 'secondary';
        }
    }

    protected checkIcon(status: CustomTrackingDomainStatus): string {
        switch (status) {
            case 'verified':
                return 'pi pi-check-circle hk-status-icon hk-status-icon--ok';
            case 'failed':
                return 'pi pi-times-circle hk-status-icon hk-status-icon--error';
            default:
                return 'pi pi-clock hk-status-icon hk-status-icon--warn';
        }
    }

    protected domainStatusIcon(domain: CustomTrackingDomain): string {
        if (!domain.enabled) {
            return 'pi pi-ban hk-status-icon hk-status-icon--muted';
        }
        if (domain.active) {
            return 'pi pi-check-circle hk-status-icon hk-status-icon--ok';
        }
        if (domain.verification_status === 'failed' || domain.target_status === 'failed' || domain.tls_status === 'failed') {
            return 'pi pi-times-circle hk-status-icon hk-status-icon--error';
        }
        return 'pi pi-clock hk-status-icon hk-status-icon--warn';
    }

    protected domainStatusLabelKey(domain: CustomTrackingDomain): string {
        if (!domain.enabled) {
            return 'admin.team.settings.trackingDomains.disabled';
        }
        if (domain.active) {
            return 'admin.team.settings.trackingDomains.active';
        }
        if (domain.verification_status === 'failed' || domain.target_status === 'failed' || domain.tls_status === 'failed') {
            return 'admin.team.settings.trackingDomains.status.failed';
        }
        return 'admin.team.settings.trackingDomains.status.pending';
    }

    protected tlsModeKey(domain: CustomTrackingDomain): string {
        return `admin.team.settings.trackingDomains.tlsMode.${domain.tls_mode}`;
    }

    private updateEnabled(domain: CustomTrackingDomain, enabled: boolean, done?: () => void): void {
        const teamID = this.teamId();
        if (!teamID) {
            done?.();
            return;
        }

        this.errorKey.set('');
        this.successKey.set('');
        this.updatingDomainID.set(domain.id);
        this.teamService
            .updateTrackingDomain(teamID, domain.id, { enabled })
            .pipe(
                finalize(() => {
                    this.updatingDomainID.set('');
                    done?.();
                })
            )
            .subscribe({
                next: (updated) => {
                    this.upsertDomain(updated);
                    this.successKey.set(enabled ? 'admin.team.settings.trackingDomains.enabledSuccess' : 'admin.team.settings.trackingDomains.disabledSuccess');
                },
                error: () => this.errorKey.set('admin.team.settings.trackingDomains.errors.update')
            });
    }

    private deleteDomain(domain: CustomTrackingDomain): void {
        const teamID = this.teamId();
        if (!teamID || this.isBusy(domain.id)) return;

        this.errorKey.set('');
        this.successKey.set('');
        this.deletingDomainID.set(domain.id);
        this.teamService
            .deleteTrackingDomain(teamID, domain.id)
            .pipe(finalize(() => this.deletingDomainID.set('')))
            .subscribe({
                next: () => {
                    this.domains.update((domains) => domains.filter((entry) => entry.id !== domain.id));
                    this.successKey.set('admin.team.settings.trackingDomains.deleted');
                },
                error: () => this.errorKey.set('admin.team.settings.trackingDomains.errors.delete')
            });
    }

    private upsertDomain(domain: CustomTrackingDomain): void {
        this.domains.update((domains) => {
            const without = domains.filter((entry) => entry.id !== domain.id);
            return [...without, domain].sort((a, b) => a.hostname.localeCompare(b.hostname, undefined, { numeric: true, sensitivity: 'base' }));
        });
    }

    private domainErrorKey(error: unknown, fallback: string): string {
        if (error instanceof HttpErrorResponse && error.status === 409) {
            return 'admin.team.settings.trackingDomains.errors.conflict';
        }
        return fallback;
    }
}
