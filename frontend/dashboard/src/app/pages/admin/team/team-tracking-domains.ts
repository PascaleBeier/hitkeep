import { ChangeDetectionStrategy, Component, effect, inject, input, signal } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ConfirmationService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';
import { TagModule } from 'primeng/tag';
import { finalize } from 'rxjs';

import { CopyControl } from '@components/copy-control/copy-control';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { SettingsCard } from '@features/settings/components/settings-card';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { CustomTrackingDomain, CustomTrackingDomainStatus } from '@models/analytics.types';
import { TeamService } from '@services/team.service';

const trackingHostnamePattern = /^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$/i;

@Component({
    selector: 'app-team-tracking-domains',
    standalone: true,
    imports: [ReactiveFormsModule, ButtonModule, SettingsCard, CopyControl, InputTextModule, MessageModule, RelativeDateTime, TagModule, TranslocoPipe],
    template: `
        <app-settings-card [title]="'admin.team.settings.trackingDomains.title' | transloco" [subtitle]="'admin.team.settings.trackingDomains.description' | transloco" icon="pi pi-globe">
            <div settings-card-header class="tracking-domains__header-actions">
                <p-button [ariaLabel]="'common.actions.refresh' | transloco" icon="pi pi-refresh" [text]="true" [rounded]="true" [loading]="isLoading()" [type]="'button'" size="small" (onClick)="loadDomains()" />
            </div>

            <div settings-card-body class="tracking-domains">
                @if (successKey(); as key) {
                    <p-message severity="success" [text]="key | transloco" />
                }
                @if (errorKey(); as key) {
                    <p-message severity="error" [text]="key | transloco" />
                }

                <form class="tracking-domains__form" [formGroup]="form" (ngSubmit)="addDomain()">
                    <div class="tracking-domains__field">
                        <input type="text" pInputText formControlName="hostname" [placeholder]="'admin.team.settings.trackingDomains.hostnamePlaceholder' | transloco" autocomplete="off" autocapitalize="none" spellcheck="false" />
                        @if (hostnameControl.touched && hostnameControl.hasError('required')) {
                            <small>{{ 'admin.team.settings.trackingDomains.hostnameRequired' | transloco }}</small>
                        } @else if (hostnameControl.touched && hostnameControl.hasError('maxlength')) {
                            <small>{{ 'admin.team.settings.trackingDomains.hostnameTooLong' | transloco }}</small>
                        } @else if (hostnameControl.touched && hostnameControl.hasError('pattern')) {
                            <small>{{ 'admin.team.settings.trackingDomains.hostnameInvalid' | transloco }}</small>
                        }
                    </div>
                    <p-button icon="pi pi-plus" [label]="'admin.team.settings.trackingDomains.addAction' | transloco" [loading]="isAdding()" [disabled]="hostnameControl.invalid || isAdding()" [type]="'submit'" />
                </form>

                @if (isLoading()) {
                    <div class="tracking-domains__empty"><i class="pi pi-spin pi-spinner" aria-hidden="true"></i>{{ 'common.loading' | transloco }}</div>
                } @else if (domains().length === 0) {
                    <div class="tracking-domains__empty">{{ 'admin.team.settings.trackingDomains.empty' | transloco }}</div>
                } @else {
                    <div class="tracking-domains__list">
                        @for (domain of domains(); track domain.id) {
                            <article class="tracking-domain">
                                <header class="tracking-domain__header">
                                    <div class="tracking-domain__title">
                                        <strong>{{ domain.hostname }}</strong>
                                        <span>{{ tlsModeKey(domain) | transloco }}</span>
                                    </div>
                                    <div class="tracking-domain__tags">
                                        <p-tag [severity]="domain.enabled ? 'success' : 'secondary'" [value]="(domain.enabled ? 'admin.team.settings.trackingDomains.enabled' : 'admin.team.settings.trackingDomains.disabled') | transloco" />
                                        <p-tag [severity]="statusSeverity(domain.verification_status)" [value]="statusKey(domain.verification_status) | transloco" />
                                        <p-tag [severity]="domain.active ? 'success' : 'secondary'" [value]="(domain.active ? 'admin.team.settings.trackingDomains.active' : 'admin.team.settings.trackingDomains.notActive') | transloco" />
                                    </div>
                                </header>

                                <div class="tracking-domain__records">
                                    <div>
                                        <span>{{ 'admin.team.settings.trackingDomains.dns.txtName' | transloco }}</span>
                                        <code>{{ domain.dns_txt_name }}</code>
                                        <app-copy-control [value]="domain.dns_txt_name" [text]="true" size="small" />
                                    </div>
                                    <div>
                                        <span>{{ 'admin.team.settings.trackingDomains.dns.txtValue' | transloco }}</span>
                                        <code>{{ domain.dns_txt_value }}</code>
                                        <app-copy-control [value]="domain.dns_txt_value" [text]="true" size="small" />
                                    </div>
                                    <div>
                                        <span>{{ 'admin.team.settings.trackingDomains.dns.target' | transloco }}</span>
                                        <code>{{ domain.dns_target || '-' }}</code>
                                        <app-copy-control [value]="domain.dns_target" [text]="true" size="small" />
                                    </div>
                                </div>

                                <div class="tracking-domain__checks" [attr.aria-label]="'admin.team.settings.trackingDomains.checksLabel' | transloco">
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

                                <div class="tracking-domain__meta">
                                    @if (domain.last_checked_at) {
                                        <span>{{ 'admin.team.settings.trackingDomains.lastChecked' | transloco }} <app-relative-date-time [value]="domain.last_checked_at" /></span>
                                    }
                                    @if (domain.last_tls_ask_at) {
                                        <span>{{ 'admin.team.settings.trackingDomains.lastTLSAsk' | transloco }} <app-relative-date-time [value]="domain.last_tls_ask_at" /></span>
                                    }
                                </div>

                                @if (domain.last_error) {
                                    <div class="tracking-domain__error">{{ domain.last_error }}</div>
                                }

                                <footer class="tracking-domain__actions">
                                    <p-button
                                        icon="pi pi-shield"
                                        [label]="'admin.team.settings.trackingDomains.verifyAction' | transloco"
                                        [loading]="verifyingDomainID() === domain.id"
                                        [disabled]="isBusy(domain.id)"
                                        [type]="'button'"
                                        size="small"
                                        (onClick)="verifyDomain(domain)"
                                    />
                                    <p-button
                                        [icon]="domain.enabled ? 'pi pi-pause' : 'pi pi-play'"
                                        [label]="(domain.enabled ? 'admin.team.settings.trackingDomains.disableAction' : 'admin.team.settings.trackingDomains.enableAction') | transloco"
                                        [outlined]="true"
                                        [loading]="updatingDomainID() === domain.id"
                                        [disabled]="isBusy(domain.id)"
                                        [type]="'button'"
                                        size="small"
                                        (onClick)="setEnabled(domain, !domain.enabled)"
                                    />
                                    <p-button
                                        icon="pi pi-trash"
                                        [ariaLabel]="'common.actions.delete' | transloco"
                                        severity="danger"
                                        [text]="true"
                                        [rounded]="true"
                                        [loading]="deletingDomainID() === domain.id"
                                        [disabled]="isBusy(domain.id)"
                                        [type]="'button'"
                                        size="small"
                                        (onClick)="confirmDelete(domain)"
                                    />
                                </footer>
                            </article>
                        }
                    </div>
                }
            </div>
        </app-settings-card>
    `,
    styles: [
        `
            .tracking-domains {
                display: grid;
                gap: 1rem;
            }

            .tracking-domains__header-actions,
            .tracking-domain__header,
            .tracking-domain__actions {
                display: flex;
                align-items: flex-start;
                justify-content: space-between;
                gap: 1rem;
            }

            .tracking-domains__form {
                display: grid;
                grid-template-columns: minmax(0, 1fr) auto;
                gap: 0.75rem;
                align-items: start;
            }

            .tracking-domains__field {
                display: grid;
                gap: 0.25rem;
                min-width: 0;
            }

            .tracking-domains__field small {
                color: var(--p-red-600);
                font-size: 0.8125rem;
            }

            .tracking-domains__list {
                display: grid;
                gap: 0.75rem;
            }

            .tracking-domain {
                display: grid;
                gap: 0.875rem;
                padding: 1rem;
                border: 1px solid var(--p-content-border-color);
                border-radius: 8px;
                background: var(--p-content-background);
            }

            .tracking-domain__title {
                display: grid;
                gap: 0.15rem;
                min-width: 0;
            }

            .tracking-domain__title strong,
            .tracking-domain__records code {
                overflow-wrap: anywhere;
            }

            .tracking-domain__title span,
            .tracking-domain__records span,
            .tracking-domain__checks span,
            .tracking-domain__meta {
                color: var(--p-text-muted-color);
                font-size: 0.8125rem;
            }

            .tracking-domain__tags,
            .tracking-domain__actions {
                flex-wrap: wrap;
            }

            .tracking-domain__records {
                display: grid;
                gap: 0.5rem;
            }

            .tracking-domain__records > div {
                display: grid;
                grid-template-columns: minmax(7rem, 10rem) minmax(0, 1fr) auto;
                gap: 0.625rem;
                align-items: center;
            }

            .tracking-domain__records code {
                padding: 0.25rem 0.4rem;
                border: 1px solid var(--p-content-border-color);
                border-radius: 6px;
                background: var(--p-content-hover-background);
                font-size: 0.8125rem;
            }

            .tracking-domain__checks {
                display: grid;
                grid-template-columns: repeat(3, minmax(0, 1fr));
                gap: 0.5rem;
            }

            .tracking-domain__checks > div {
                display: flex;
                min-width: 0;
                align-items: center;
                justify-content: space-between;
                gap: 0.5rem;
                padding: 0.5rem 0.625rem;
                border: 1px solid var(--p-content-border-color);
                border-radius: 8px;
                background: var(--p-content-hover-background);
            }

            .tracking-domain__meta {
                display: flex;
                flex-wrap: wrap;
                gap: 0.5rem 1rem;
            }

            .tracking-domain__error {
                padding: 0.625rem 0.75rem;
                border: 1px solid color-mix(in srgb, var(--p-red-500) 24%, var(--p-content-border-color));
                border-radius: 8px;
                background: color-mix(in srgb, var(--p-red-500) 8%, transparent);
                color: var(--p-red-600);
                font-size: 0.8125rem;
                overflow-wrap: anywhere;
            }

            .tracking-domains__empty {
                display: flex;
                align-items: center;
                justify-content: center;
                gap: 0.5rem;
                min-height: 5rem;
                color: var(--p-text-muted-color);
                font-size: 0.875rem;
            }

            @media (max-width: 640px) {
                .tracking-domains__header-actions,
                .tracking-domain__header,
                .tracking-domain__actions {
                    align-items: stretch;
                    flex-direction: column;
                }

                .tracking-domains__form {
                    grid-template-columns: minmax(0, 1fr);
                }

                .tracking-domain__records > div {
                    grid-template-columns: minmax(0, 1fr);
                }

                .tracking-domain__checks {
                    grid-template-columns: minmax(0, 1fr);
                }
            }
        `
    ],
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

    protected addDomain(): void {
        const teamID = this.teamId();
        const hostname = this.hostnameControl.value.trim();
        if (!teamID || this.isAdding()) return;
        if (!hostname || this.hostnameControl.invalid) {
            this.hostnameControl.markAsTouched();
            return;
        }

        this.errorKey.set('');
        this.successKey.set('');
        this.isAdding.set(true);
        this.teamService
            .createTrackingDomain(teamID, { hostname })
            .pipe(finalize(() => this.isAdding.set(false)))
            .subscribe({
                next: (domain) => {
                    this.hostnameControl.reset('');
                    this.upsertDomain(domain);
                    this.successKey.set('admin.team.settings.trackingDomains.added');
                },
                error: (error: unknown) => {
                    this.errorKey.set(this.domainErrorKey(error, 'admin.team.settings.trackingDomains.errors.add'));
                }
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
        const teamID = this.teamId();
        if (!teamID || this.isBusy(domain.id)) return;

        this.errorKey.set('');
        this.successKey.set('');
        this.updatingDomainID.set(domain.id);
        this.teamService
            .updateTrackingDomain(teamID, domain.id, { enabled })
            .pipe(finalize(() => this.updatingDomainID.set('')))
            .subscribe({
                next: (updated) => {
                    this.upsertDomain(updated);
                    this.successKey.set(enabled ? 'admin.team.settings.trackingDomains.enabledSuccess' : 'admin.team.settings.trackingDomains.disabledSuccess');
                },
                error: () => this.errorKey.set('admin.team.settings.trackingDomains.errors.update')
            });
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

    protected tlsModeKey(domain: CustomTrackingDomain): string {
        return `admin.team.settings.trackingDomains.tlsMode.${domain.tls_mode}`;
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
