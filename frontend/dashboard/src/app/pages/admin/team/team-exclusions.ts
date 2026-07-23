import { NgOptimizedImage } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { finalize } from 'rxjs';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { IconFieldModule } from '@openng/optimus-ui/iconfield';
import { InputIconModule } from '@openng/optimus-ui/inputicon';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MessageModule } from '@openng/optimus-ui/message';
import { SelectModule } from '@openng/optimus-ui/select';
import { TableModule } from '@openng/optimus-ui/table';
import { TagModule } from '@openng/optimus-ui/tag';
import { CopyControl } from '@components/copy-control/copy-control';
import { CrudDialog } from '@components/crud-dialog/crud-dialog';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { TableRowActionItem, TableRowActions } from '@components/table-row-actions/table-row-actions';
import { CountryOption, countryDisplayName, countryOptions } from '@core/i18n/country-options';
import { countryFlagUrl } from '@core/i18n/flag-utils';
import { SettingsCard } from '@features/settings/components/settings-card';
import { IPExclusion } from '@models/analytics.types';
import { ExclusionsService } from '@services/exclusions.service';
import { TeamService } from '@services/team.service';

const ipOrCIDRPattern = /^(([0-9]{1,3}\.){3}[0-9]{1,3}(\/(3[0-2]|[12]?[0-9]))?|([0-9A-Fa-f:]+)(\/(12[0-8]|1[01][0-9]|[1-9]?[0-9]))?)$/;

interface ActionStatus {
    severity: 'success' | 'error';
    key: string;
    params?: Record<string, string | number>;
}

type ExclusionRow = IPExclusion & {
    type_label: string;
    scope_label: string;
    value_label: string;
    country_name: string;
    search_value: string;
};

@Component({
    selector: 'app-team-exclusions',
    imports: [
        ReactiveFormsModule,
        NgOptimizedImage,
        ButtonModule,
        ConfirmDialogModule,
        IconFieldModule,
        InputIconModule,
        InputTextModule,
        MessageModule,
        SelectModule,
        TableModule,
        TagModule,
        CopyControl,
        CrudDialog,
        RelativeDateTime,
        SettingsCard,
        TableRowActions,
        TranslocoPipe
    ],
    templateUrl: './team-exclusions.html',
    changeDetection: ChangeDetectionStrategy.OnPush,
    providers: [ConfirmationService]
})
export class TeamExclusionsPage {
    private readonly exclusionsService = inject(ExclusionsService);
    private readonly confirmationService = inject(ConfirmationService);
    private readonly teamService = inject(TeamService);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    protected readonly activeTeam = this.teamService.activeTeam;
    protected readonly exclusions = signal<IPExclusion[]>([]);
    protected readonly isLoading = signal(false);
    protected readonly isSaving = signal(false);
    protected readonly error = signal<string | null>(null);
    protected readonly createError = signal<string | null>(null);
    protected readonly actionStatus = signal<ActionStatus | null>(null);
    protected readonly deletingRuleID = signal<string | null>(null);
    protected readonly isAddDialogVisible = signal(false);
    protected readonly isCurrentIPLoading = signal(false);
    protected readonly currentIPCIDR = signal('');
    protected readonly hasInheritedExclusions = computed(() => this.exclusions().some((rule) => rule.inherited));
    protected readonly ruleTypeOptions = computed(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('admin.team.exclusions.ruleTypes.cidr'), value: 'cidr' },
            { label: this.transloco.translate('admin.team.exclusions.ruleTypes.country'), value: 'country' },
            { label: this.transloco.translate('admin.team.exclusions.ruleTypes.userAgent'), value: 'user_agent' },
            { label: this.transloco.translate('admin.team.exclusions.ruleTypes.path'), value: 'path' }
        ];
    });
    protected readonly countryOptions = computed<CountryOption[]>(() => countryOptions(this.activeLanguage()));
    protected readonly exclusionRows = computed<ExclusionRow[]>(() =>
        this.exclusions().map((rule) => {
            const countryName = rule.country_code ? countryDisplayName(rule.country_code, this.activeLanguage()) : '';
            const valueLabel = this.ruleValue(rule, countryName);
            const typeLabel = this.ruleTypeLabel(rule.type);
            const scopeLabel = this.scopeLabel(rule.scope ?? 'team');
            return {
                ...rule,
                type_label: typeLabel,
                scope_label: scopeLabel,
                value_label: valueLabel,
                country_name: countryName,
                search_value: `${typeLabel} ${scopeLabel} ${valueLabel} ${rule.description ?? ''} ${rule.created_at}`
            };
        })
    );
    protected readonly actionStatusMessage = computed(() => {
        this.activeLanguage();
        const status = this.actionStatus();
        return status ? this.transloco.translate(status.key, status.params) : '';
    });

    protected readonly form = new FormGroup({
        type: new FormControl<IPExclusion['type']>('cidr', { nonNullable: true }),
        cidr: new FormControl('', { nonNullable: true, validators: [Validators.pattern(ipOrCIDRPattern)] }),
        countryCode: new FormControl('', { nonNullable: true }),
        userAgent: new FormControl('', { nonNullable: true }),
        path: new FormControl('', { nonNullable: true }),
        description: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(255)] })
    });

    constructor() {
        this.loadCurrentIP();
        effect(() => {
            const team = this.activeTeam();
            if (!team) {
                this.exclusions.set([]);
                return;
            }
            this.loadExclusions(team.id);
        });
    }

    protected addRule(): void {
        const team = this.activeTeam();
        if (!team || this.isSaving()) {
            return;
        }
        if (!this.validateRuleForm()) {
            this.form.markAllAsTouched();
            return;
        }

        this.createError.set(null);
        this.actionStatus.set(null);
        this.isSaving.set(true);
        const type = this.form.controls.type.value;
        this.exclusionsService
            .createTeamExclusion(team.id, {
                type,
                cidr: type === 'cidr' ? this.form.controls.cidr.value.trim() : undefined,
                country_code: type === 'country' ? this.form.controls.countryCode.value.trim() : undefined,
                user_agent: type === 'user_agent' ? this.form.controls.userAgent.value.trim() : undefined,
                path: type === 'path' ? this.form.controls.path.value.trim() : undefined,
                description: this.form.controls.description.value.trim()
            })
            .pipe(finalize(() => this.isSaving.set(false)))
            .subscribe({
                next: (rule) => {
                    this.exclusions.update((current) => {
                        const firstTeamRule = current.findIndex((entry) => (entry.scope ?? 'team') === 'team');
                        const insertAt = firstTeamRule < 0 ? current.length : firstTeamRule;
                        return [...current.slice(0, insertAt), rule, ...current.slice(insertAt)];
                    });
                    this.actionStatus.set({ severity: 'success', key: 'admin.team.exclusions.status.createSuccess', params: { value: this.ruleValue(rule) } });
                    this.closeAddDialog();
                },
                error: () => this.createError.set('admin.team.exclusions.errors.createFailed')
            });
    }

    protected openAddDialog(): void {
        this.resetForm();
        this.createError.set(null);
        this.isAddDialogVisible.set(true);
    }

    protected onRuleTypeChange(): void {
        this.clearValueErrors();
    }

    protected onAddDialogVisibleChange(visible: boolean): void {
        if (!visible && this.isSaving()) {
            this.isAddDialogVisible.set(true);
            return;
        }
        this.isAddDialogVisible.set(visible);
        if (!visible) {
            this.closeAddDialog();
        }
    }

    protected ruleActions(rule: IPExclusion): TableRowActionItem[] {
        if (rule.inherited) {
            return [];
        }
        this.activeLanguage();
        return [
            {
                label: this.transloco.translate('share.dialog.deleteAction'),
                icon: 'pi pi-trash',
                danger: true,
                command: () => this.confirmDeleteRule(rule)
            }
        ];
    }

    protected confirmDeleteRule(rule: IPExclusion): void {
        const team = this.activeTeam();
        if (!team || rule.inherited) {
            return;
        }
        this.confirmationService.confirm({
            message: this.transloco.translate('admin.team.exclusions.confirmDelete', { value: this.ruleValue(rule) }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('share.dialog.deleteAction')),
            accept: () => this.deleteRule(team.id, rule)
        });
    }

    protected reload(): void {
        const team = this.activeTeam();
        if (team) {
            this.loadExclusions(team.id);
        }
    }

    private deleteRule(teamID: string, rule: IPExclusion): void {
        this.error.set(null);
        this.actionStatus.set(null);
        this.deletingRuleID.set(rule.id);
        this.exclusionsService
            .deleteTeamExclusion(teamID, rule.id)
            .pipe(finalize(() => this.deletingRuleID.set(null)))
            .subscribe({
                next: () => {
                    this.exclusions.update((current) => current.filter((entry) => entry.id !== rule.id));
                    this.actionStatus.set({ severity: 'success', key: 'admin.team.exclusions.status.deleteSuccess', params: { value: this.ruleValue(rule) } });
                },
                error: () => this.error.set('admin.team.exclusions.errors.deleteFailed')
            });
    }

    private loadExclusions(teamID: string): void {
        this.isLoading.set(true);
        this.error.set(null);
        this.actionStatus.set(null);
        this.exclusionsService
            .listTeamExclusions(teamID, true)
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: (rules) => this.exclusions.set(rules),
                error: () => this.error.set('admin.team.exclusions.errors.loadFailed')
            });
    }

    private loadCurrentIP(): void {
        this.isCurrentIPLoading.set(true);
        this.exclusionsService
            .getCurrentIP()
            .pipe(finalize(() => this.isCurrentIPLoading.set(false)))
            .subscribe({
                next: (currentIP) => this.currentIPCIDR.set(currentIP.cidr),
                error: () => this.currentIPCIDR.set('')
            });
    }

    private closeAddDialog(): void {
        this.isAddDialogVisible.set(false);
        this.resetForm();
        this.createError.set(null);
    }

    private resetForm(): void {
        this.form.reset({ type: 'cidr', cidr: '', countryCode: '', userAgent: '', path: '', description: '' });
    }

    protected ruleTypeLabel(type: IPExclusion['type']): string {
        this.activeLanguage();
        const keys: Record<IPExclusion['type'], string> = {
            cidr: 'admin.team.exclusions.ruleTypes.cidr',
            country: 'admin.team.exclusions.ruleTypes.country',
            user_agent: 'admin.team.exclusions.ruleTypes.userAgent',
            path: 'admin.team.exclusions.ruleTypes.path'
        };
        return this.transloco.translate(keys[type]);
    }

    protected scopeLabel(scope: NonNullable<IPExclusion['scope']>): string {
        this.activeLanguage();
        return this.transloco.translate(`admin.team.exclusions.scopes.${scope}`);
    }

    protected ruleValue(rule: IPExclusion, countryName?: string): string {
        if (rule.type === 'country' && rule.country_code) {
            return `${countryName ?? countryDisplayName(rule.country_code, this.activeLanguage())} (${rule.country_code})`;
        }
        if (rule.type === 'user_agent') {
            return rule.user_agent ?? '';
        }
        if (rule.type === 'path') {
            return rule.path ?? '';
        }
        return rule.cidr ?? '';
    }

    protected countryFlagUrl(code: string): string {
        return countryFlagUrl(code);
    }

    private clearValueErrors(): void {
        this.form.controls.cidr.setErrors(null);
        this.form.controls.countryCode.setErrors(null);
        this.form.controls.userAgent.setErrors(null);
        this.form.controls.path.setErrors(null);
    }

    private validateRuleForm(): boolean {
        this.clearValueErrors();
        if (this.form.controls.description.invalid) {
            return false;
        }
        const type = this.form.controls.type.value;
        const controls = this.form.controls;
        if (type === 'country' && !controls.countryCode.value.trim()) {
            controls.countryCode.setErrors({ required: true });
            return false;
        }
        if (type === 'user_agent' && !controls.userAgent.value.trim()) {
            controls.userAgent.setErrors({ required: true });
            return false;
        }
        if (type === 'path' && !controls.path.value.trim()) {
            controls.path.setErrors({ required: true });
            return false;
        }
        if (type === 'cidr' && (!controls.cidr.value.trim() || !ipOrCIDRPattern.test(controls.cidr.value.trim()))) {
            controls.cidr.setErrors({ invalidCidr: true });
            return false;
        }
        return true;
    }
}
