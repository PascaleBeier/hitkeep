import { ChangeDetectionStrategy, Component, effect, inject, input, signal } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { FormControl, FormGroup, FormsModule, ReactiveFormsModule, Validators } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { ButtonModule } from '@openng/optimus-ui/button';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MessageModule } from '@openng/optimus-ui/message';
import { TagModule } from '@openng/optimus-ui/tag';
import { ToggleSwitchModule } from '@openng/optimus-ui/toggleswitch';
import { finalize } from 'rxjs';

import { CopyControl } from '@components/copy-control/copy-control';
import { CrudDialog } from '@components/crud-dialog/crud-dialog';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { AppTable, AppTableCell, AppTableColumn, AppTableSlot } from '@components/table/table';
import { TableRowActionItem } from '@components/table-row-actions/table-row-actions';
import { SettingsCard } from '@features/settings/components/settings-card';
import { CustomTrackingDomain, CustomTrackingDomainStatus } from '@models/analytics.types';
import { TeamService } from '@services/team.service';

const trackingHostnamePattern = /^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$/i;

@Component({
    selector: 'app-team-tracking-domains',
    standalone: true,
    imports: [ReactiveFormsModule, FormsModule, ButtonModule, SettingsCard, CopyControl, CrudDialog, InputTextModule, MessageModule, AppTable, AppTableCell, AppTableSlot, TagModule, ToggleSwitchModule, TranslocoPipe],
    templateUrl: './team-tracking-domains.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TeamTrackingDomains {
    private readonly teamService = inject(TeamService);
    private readonly confirmationService = inject(ConfirmationService);
    private readonly transloco = inject(TranslocoService);
    private loadedTeamID = '';
    private readonly statusOptions = (['pending', 'verified', 'failed'] as const).map((status) => ({ value: status, labelKey: `admin.team.settings.trackingDomains.status.${status}` }));
    protected readonly domainColumns: AppTableColumn<CustomTrackingDomain>[] = [
        { field: 'hostname', headerKey: 'admin.team.settings.trackingDomains.hostnameLabel', frozen: true },
        { field: 'verification_status', headerKey: 'admin.team.settings.trackingDomains.checks.ownership', type: 'enum', groupable: true, options: this.statusOptions },
        { field: 'target_status', headerKey: 'admin.team.settings.trackingDomains.checks.target', type: 'enum', groupable: true, options: this.statusOptions },
        { field: 'tls_status', headerKey: 'admin.team.settings.trackingDomains.checks.tls', type: 'enum', groupable: true, options: this.statusOptions },
        { field: 'last_checked_at', headerKey: 'admin.team.settings.trackingDomains.lastChecked', type: 'date' }
    ];
    protected readonly domainRowActions = (domain: CustomTrackingDomain) => this.domainActions(domain);
    protected readonly domainActionLoading = (domain: CustomTrackingDomain) => this.isBusy(domain.id);

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
