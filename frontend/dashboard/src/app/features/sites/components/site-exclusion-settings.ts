import { ChangeDetectionStrategy, Component, effect, inject, input, signal } from '@angular/core';

import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { finalize } from 'rxjs';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';

import { ConfirmationService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { ConfirmPopupModule } from 'primeng/confirmpopup';
import { InputTextModule } from 'primeng/inputtext';
import { TableModule } from 'primeng/table';

import { IPExclusion, Site } from '@models/analytics.types';
import { ExclusionsService } from '@services/exclusions.service';
import { CopyControl } from '@components/copy-control/copy-control';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';

const ipOrCIDRPattern = /^(([0-9]{1,3}\.){3}[0-9]{1,3}(\/(3[0-2]|[12]?[0-9]))?|([0-9A-Fa-f:]+)(\/(12[0-8]|1[01][0-9]|[1-9]?[0-9]))?)$/;

@Component({
    selector: 'app-site-exclusion-settings',
    standalone: true,
    imports: [ReactiveFormsModule, ButtonModule, ConfirmPopupModule, InputTextModule, TableModule, CopyControl, RelativeDateTime, TranslocoPipe],
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

    protected readonly exclusions = signal<IPExclusion[]>([]);
    protected readonly isLoading = signal(false);
    protected readonly isSaving = signal(false);
    protected readonly error = signal<string | null>(null);
    protected readonly isCurrentIPLoading = signal(false);
    protected readonly currentIPCIDR = signal('');

    protected readonly form = new FormGroup({
        cidr: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.pattern(ipOrCIDRPattern)] }),
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
        const site = this.site();
        if (!site) {
            return;
        }

        if (this.form.invalid) {
            this.form.markAllAsTouched();
            return;
        }

        this.error.set(null);
        this.isSaving.set(true);

        this.exclusionsService
            .createSiteExclusion(site.id, {
                cidr: this.form.controls.cidr.value.trim(),
                description: this.form.controls.description.value.trim()
            })
            .pipe(finalize(() => this.isSaving.set(false)))
            .subscribe({
                next: (rule) => {
                    this.exclusions.update((current) => [rule, ...current]);
                    this.form.reset({ cidr: '', description: '' });
                },
                error: () => {
                    this.error.set('sites.exclusions.errors.createFailed');
                }
            });
    }

    protected confirmDeleteRule(event: Event, rule: IPExclusion): void {
        const site = this.site();
        if (!site) {
            return;
        }

        this.confirmationService.confirm({
            key: 'site-exclusion-delete',
            target: event.currentTarget as EventTarget,
            message: this.transloco.translate('sites.exclusions.confirmDelete', { cidr: rule.cidr }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: {
                label: this.transloco.translate('common.actions.cancel'),
                severity: 'secondary',
                outlined: true
            },
            acceptButtonProps: {
                label: this.transloco.translate('share.dialog.deleteAction'),
                severity: 'danger'
            },
            accept: () => this.deleteRule(site.id, rule)
        });
    }

    private deleteRule(siteID: string, rule: IPExclusion): void {
        this.error.set(null);
        this.exclusionsService.deleteSiteExclusion(siteID, rule.id).subscribe({
            next: () => {
                this.exclusions.update((current) => current.filter((entry) => entry.id !== rule.id));
            },
            error: () => {
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

        this.exclusionsService
            .listSiteExclusions(siteID)
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
}
