import { ChangeDetectionStrategy, Component, computed, effect, inject, input, signal } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { NgOptimizedImage } from '@angular/common';

import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { finalize } from 'rxjs';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';

import { ConfirmationService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { ConfirmDialogModule } from 'primeng/confirmdialog';
import { IconFieldModule } from 'primeng/iconfield';
import { InputIconModule } from 'primeng/inputicon';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';
import { SelectModule } from 'primeng/select';
import { TableModule } from 'primeng/table';

import { CountryOption, countryDisplayName, countryOptions } from '@core/i18n/country-options';
import { countryFlagUrl } from '@core/i18n/flag-utils';
import { IPExclusion, Site } from '@models/analytics.types';
import { ExclusionsService } from '@services/exclusions.service';
import { CopyControl } from '@components/copy-control/copy-control';
import { CrudDialog } from '@components/crud-dialog/crud-dialog';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { TableRowActionItem, TableRowActions } from '@components/table-row-actions/table-row-actions';

const ipOrCIDRPattern = /^(([0-9]{1,3}\.){3}[0-9]{1,3}(\/(3[0-2]|[12]?[0-9]))?|([0-9A-Fa-f:]+)(\/(12[0-8]|1[01][0-9]|[1-9]?[0-9]))?)$/;

interface ActionStatus {
    severity: 'success' | 'error';
    key: string;
    params?: Record<string, string | number>;
}

type ExclusionRuleType = IPExclusion['type'];
type ExclusionRow = IPExclusion & {
    type_label: string;
    scope_label: string;
    value_label: string;
    country_name: string;
    search_value: string;
};

@Component({
    selector: 'app-site-exclusion-settings',
    standalone: true,
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
        CopyControl,
        CrudDialog,
        RelativeDateTime,
        TableRowActions,
        TranslocoPipe
    ],
    templateUrl: './site-exclusion-settings.html',
    styleUrl: './site-exclusion-settings.css',
    changeDetection: ChangeDetectionStrategy.OnPush,
    providers: [ConfirmationService]
})
export class SiteExclusionSettings {
    site = input.required<Site | null>();

    private exclusionsService = inject(ExclusionsService);
    private confirmationService = inject(ConfirmationService);
    private transloco = inject(TranslocoService);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

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
            { label: this.transloco.translate('sites.exclusions.ruleTypes.cidr'), value: 'cidr' },
            { label: this.transloco.translate('sites.exclusions.ruleTypes.country'), value: 'country' },
            { label: this.transloco.translate('sites.exclusions.ruleTypes.userAgent'), value: 'user_agent' },
            { label: this.transloco.translate('sites.exclusions.ruleTypes.path'), value: 'path' }
        ];
    });
    protected readonly countryOptions = computed<CountryOption[]>(() => countryOptions(this.activeLanguage()));
    protected readonly exclusionRows = computed<ExclusionRow[]>(() =>
        this.exclusions().map((rule) => {
            const countryName = rule.country_code ? countryDisplayName(rule.country_code, this.activeLanguage()) : '';
            const valueLabel = this.ruleValue(rule, countryName);
            const typeLabel = this.ruleTypeLabel(rule.type);
            const scopeLabel = this.scopeLabel(rule.scope ?? 'site');
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
        if (!status) {
            return '';
        }

        return this.transloco.translate(status.key, status.params);
    });

    protected readonly form = new FormGroup({
        type: new FormControl<ExclusionRuleType>('cidr', { nonNullable: true }),
        cidr: new FormControl('', { nonNullable: true, validators: [Validators.pattern(ipOrCIDRPattern)] }),
        countryCode: new FormControl('', { nonNullable: true }),
        userAgent: new FormControl('', { nonNullable: true }),
        path: new FormControl('', { nonNullable: true }),
        description: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(255)] })
    });

    constructor() {
        this.loadCurrentIP();

        effect(() => {
            const site = this.site();
            if (!site) {
                this.exclusions.set([]);
                return;
            }
            this.loadExclusions(site.id);
        });
    }

    protected addRule(): void {
        if (this.isSaving()) {
            return;
        }
        const site = this.site();
        if (!site) {
            return;
        }

        if (!this.validateRuleForm()) {
            this.form.markAllAsTouched();
            return;
        }

        this.createError.set(null);
        this.actionStatus.set(null);
        this.isSaving.set(true);

        this.exclusionsService
            .createSiteExclusion(site.id, {
                type: this.form.controls.type.value,
                cidr: this.form.controls.type.value === 'cidr' ? this.form.controls.cidr.value.trim() : undefined,
                country_code: this.form.controls.type.value === 'country' ? this.form.controls.countryCode.value.trim() : undefined,
                user_agent: this.form.controls.type.value === 'user_agent' ? this.form.controls.userAgent.value.trim() : undefined,
                path: this.form.controls.type.value === 'path' ? this.form.controls.path.value.trim() : undefined,
                description: this.form.controls.description.value.trim()
            })
            .pipe(finalize(() => this.isSaving.set(false)))
            .subscribe({
                next: (rule) => {
                    this.exclusions.update((current) => {
                        const firstSiteRule = current.findIndex((entry) => (entry.scope ?? 'site') === 'site');
                        const insertAt = firstSiteRule < 0 ? current.length : firstSiteRule;
                        return [...current.slice(0, insertAt), rule, ...current.slice(insertAt)];
                    });
                    this.actionStatus.set({
                        severity: 'success',
                        key: 'sites.exclusions.status.createSuccess',
                        params: { value: this.ruleValue(rule) }
                    });
                    this.closeAddDialog();
                },
                error: () => {
                    this.createError.set('sites.exclusions.errors.createFailed');
                }
            });
    }

    protected openAddDialog(): void {
        this.form.reset({ type: 'cidr', cidr: '', countryCode: '', userAgent: '', path: '', description: '' });
        this.createError.set(null);
        this.isAddDialogVisible.set(true);
    }

    protected onRuleTypeChange(): void {
        this.form.controls.cidr.setErrors(null);
        this.form.controls.countryCode.setErrors(null);
        this.form.controls.userAgent.setErrors(null);
        this.form.controls.path.setErrors(null);
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
        if (rule.inherited) {
            return;
        }
        const site = this.site();
        if (!site) {
            return;
        }

        this.confirmationService.confirm({
            message: this.transloco.translate('sites.exclusions.confirmDelete', { value: this.ruleValue(rule) }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('share.dialog.deleteAction')),
            accept: () => this.deleteRule(site.id, rule)
        });
    }

    protected deleteRule(siteID: string, rule: IPExclusion): void {
        this.error.set(null);
        this.actionStatus.set(null);
        this.deletingRuleID.set(rule.id);
        this.exclusionsService
            .deleteSiteExclusion(siteID, rule.id)
            .pipe(finalize(() => this.deletingRuleID.set(null)))
            .subscribe({
                next: () => {
                    this.exclusions.update((current) => current.filter((entry) => entry.id !== rule.id));
                    this.actionStatus.set({
                        severity: 'success',
                        key: 'sites.exclusions.status.deleteSuccess',
                        params: { value: this.ruleValue(rule) }
                    });
                },
                error: () => {
                    this.actionStatus.set(null);
                    this.error.set('sites.exclusions.errors.deleteFailed');
                }
            });
    }

    private loadCurrentIP(): void {
        this.isCurrentIPLoading.set(true);
        this.currentIPCIDR.set('');

        this.exclusionsService
            .getCurrentIP()
            .pipe(finalize(() => this.isCurrentIPLoading.set(false)))
            .subscribe({
                next: (currentIP) => {
                    this.currentIPCIDR.set(currentIP.cidr);
                },
                error: () => {
                    this.currentIPCIDR.set('');
                }
            });
    }

    private loadExclusions(siteID: string): void {
        this.isLoading.set(true);
        this.error.set(null);
        this.actionStatus.set(null);

        this.exclusionsService
            .listSiteExclusions(siteID, true)
            .pipe(finalize(() => this.isLoading.set(false)))
            .subscribe({
                next: (rules) => {
                    this.exclusions.set(rules);
                },
                error: () => {
                    this.error.set('sites.exclusions.errors.loadFailed');
                }
            });
    }

    private closeAddDialog(): void {
        this.isAddDialogVisible.set(false);
        this.form.reset({ type: 'cidr', cidr: '', countryCode: '', userAgent: '', path: '', description: '' });
        this.createError.set(null);
    }

    protected ruleTypeLabel(type: IPExclusion['type']): string {
        this.activeLanguage();
        const keys: Record<IPExclusion['type'], string> = {
            cidr: 'sites.exclusions.ruleTypes.cidr',
            country: 'sites.exclusions.ruleTypes.country',
            user_agent: 'sites.exclusions.ruleTypes.userAgent',
            path: 'sites.exclusions.ruleTypes.path'
        };
        return this.transloco.translate(keys[type]);
    }

    protected scopeLabel(scope: NonNullable<IPExclusion['scope']>): string {
        this.activeLanguage();
        return this.transloco.translate(`sites.exclusions.scopes.${scope}`);
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

    private validateRuleForm(): boolean {
        this.form.controls.cidr.setErrors(null);
        this.form.controls.countryCode.setErrors(null);
        this.form.controls.userAgent.setErrors(null);
        this.form.controls.path.setErrors(null);
        if (this.form.controls.description.invalid) {
            return false;
        }
        if (this.form.controls.type.value === 'country') {
            if (!this.form.controls.countryCode.value.trim()) {
                this.form.controls.countryCode.setErrors({ required: true });
                return false;
            }
            return true;
        }
        if (this.form.controls.type.value === 'user_agent') {
            if (!this.form.controls.userAgent.value.trim()) {
                this.form.controls.userAgent.setErrors({ required: true });
                return false;
            }
            return true;
        }
        if (this.form.controls.type.value === 'path') {
            if (!this.form.controls.path.value.trim()) {
                this.form.controls.path.setErrors({ required: true });
                return false;
            }
            return true;
        }
        if (!this.form.controls.cidr.value.trim() || !ipOrCIDRPattern.test(this.form.controls.cidr.value.trim())) {
            this.form.controls.cidr.setErrors({ invalidCidr: true });
            return false;
        }
        return true;
    }
}
