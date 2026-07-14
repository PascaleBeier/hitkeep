import { ChangeDetectionStrategy, Component, computed, effect, inject, input, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import { finalize } from 'rxjs';
import { TranslocoPipe } from '@jsverse/transloco';

import { DialogShell } from '@components/dialog-shell/dialog-shell';
import { Site } from '@models/analytics.types';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';
import { SiteService, SiteStatsResetResponse } from '@features/sites/services/site.service';
import { NavigationNoticeService } from '@services/navigation-notice.service';
import { ButtonModule } from 'primeng/button';
import { InputTextModule } from 'primeng/inputtext';

type SiteDangerAction = 'reset' | 'delete';

@Component({
    selector: 'app-site-danger-zone',
    standalone: true,
    imports: [ButtonModule, DialogShell, FormsModule, InputTextModule, TranslocoPipe],
    template: `
        <div class="site-settings-stack">
            @if (canDeleteSite()) {
                <section class="site-settings-card site-settings-card--danger">
                    <header class="site-settings-card__header">
                        <div class="site-settings-card__title-row">
                            <span class="site-settings-card__icon site-settings-card__icon--danger"><i class="pi pi-refresh" aria-hidden="true"></i></span>
                            <div>
                                <h3>{{ 'sites.danger.resetTitle' | transloco }}</h3>
                                <p>{{ 'sites.danger.resetDescription' | transloco }}</p>
                            </div>
                        </div>
                    </header>
                    <div class="site-settings-card__body">
                        @if (resetSuccess()) {
                            <div class="site-settings-alert site-settings-alert--success">
                                {{ 'sites.danger.resetSuccess' | transloco: { rows: resetSuccess()?.rows_cleared ?? 0, imports: resetSuccess()?.imports_marked_deleted ?? 0 } }}
                            </div>
                        }
                        @if (resetError()) {
                            <div class="site-settings-alert site-settings-alert--error">{{ resetError() | transloco }}</div>
                        }
                    </div>
                    <footer class="site-settings-card__footer">
                        <p-button
                            styleClass="site-settings-danger-action"
                            [label]="'sites.danger.resetAction' | transloco"
                            icon="pi pi-refresh"
                            severity="danger"
                            [disabled]="isBusy()"
                            [loading]="isResetting()"
                            (onClick)="openConfirmDialog('reset')"
                        />
                    </footer>
                </section>
                <section class="site-settings-card site-settings-card--danger">
                    <header class="site-settings-card__header">
                        <div class="site-settings-card__title-row">
                            <span class="site-settings-card__icon site-settings-card__icon--danger"><i class="pi pi-trash" aria-hidden="true"></i></span>
                            <div>
                                <h3>{{ 'sites.danger.deleteTitle' | transloco }}</h3>
                                <p>{{ 'sites.danger.deleteDescription' | transloco }}</p>
                            </div>
                        </div>
                    </header>
                    <div class="site-settings-card__body">
                        @if (deleteError()) {
                            <div class="site-settings-alert site-settings-alert--error">{{ deleteError() | transloco }}</div>
                        }
                    </div>
                    <footer class="site-settings-card__footer">
                        <p-button
                            styleClass="site-settings-danger-action"
                            [label]="'sites.danger.deleteAction' | transloco"
                            icon="pi pi-trash"
                            severity="danger"
                            [disabled]="isBusy()"
                            [loading]="isDeleting()"
                            (onClick)="openConfirmDialog('delete')"
                        />
                    </footer>
                </section>
            }

            <app-dialog-shell
                [title]="confirmTitleKey() | transloco"
                [visible]="confirmDialogVisible()"
                [busy]="isBusy()"
                role="alertdialog"
                [secondaryLabel]="'common.actions.cancel' | transloco"
                [primaryLabel]="confirmActionKey() | transloco"
                primarySeverity="danger"
                [primaryDisabled]="!canSubmitConfirm()"
                [primaryLoading]="isBusy()"
                (visibleChange)="onConfirmDialogVisibleChange($event)"
                (primaryAction)="runConfirmedAction()"
            >
                @if (site(); as activeSite) {
                    <div class="site-settings-field">
                        <p id="site-danger-confirm-instruction">{{ confirmMessageKey() | transloco: { domain: activeSite.domain } }}</p>
                        <input
                            id="site-danger-confirm"
                            pInputText
                            class="w-full"
                            [ngModel]="confirmValue()"
                            (ngModelChange)="confirmValue.set($event)"
                            [placeholder]="activeSite.domain"
                            aria-labelledby="site-danger-confirm-instruction"
                            autocomplete="off"
                        />
                        @if (pendingAction() === 'reset') {
                            <small class="site-settings-field-hint">{{ 'sites.danger.resetConfirmHint' | transloco }}</small>
                        }
                    </div>
                }
            </app-dialog-shell>
        </div>
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteDangerZone {
    private access = inject(AccessService);
    private siteService = inject(SiteService);
    private router = inject(Router);
    private navigationNotice = inject(NavigationNoticeService);

    site = input.required<Site | null>();
    protected isDeleting = signal(false);
    protected isResetting = signal(false);
    protected pendingAction = signal<SiteDangerAction | null>(null);
    protected confirmValue = signal('');
    protected resetSuccess = signal<SiteStatsResetResponse | null>(null);
    protected resetError = signal<string | null>(null);
    protected deleteError = signal<string | null>(null);
    protected canDeleteSite = computed(() => {
        const site = this.site();
        if (!site) return false;
        return this.access.canSite(site.id, SITE_CAPABILITIES.delete);
    });
    protected isBusy = computed(() => this.isDeleting() || this.isResetting());
    protected confirmDialogVisible = computed(() => this.pendingAction() !== null);
    protected confirmDomain = computed(() => this.site()?.domain ?? '');
    protected canSubmitConfirm = computed(() => {
        const domain = this.confirmDomain();
        return domain.length > 0 && this.confirmValue().trim().toLowerCase() === domain.toLowerCase();
    });
    protected confirmTitleKey = computed(() => {
        switch (this.pendingAction()) {
            case 'reset':
                return 'sites.danger.resetConfirmTitle';
            case 'delete':
                return 'sites.danger.deleteConfirmTitle';
            default:
                return '';
        }
    });
    protected confirmMessageKey = computed(() => {
        switch (this.pendingAction()) {
            case 'reset':
                return 'sites.danger.resetConfirmMessage';
            case 'delete':
                return 'sites.danger.deleteConfirmMessage';
            default:
                return '';
        }
    });
    protected confirmActionKey = computed(() => {
        switch (this.pendingAction()) {
            case 'reset':
                return 'sites.danger.resetAction';
            case 'delete':
                return 'sites.danger.deleteAction';
            default:
                return '';
        }
    });

    constructor() {
        effect(() => {
            const site = this.site();
            if (site) {
                this.confirmValue.set('');
                this.pendingAction.set(null);
                this.resetSuccess.set(null);
                this.resetError.set(null);
                this.deleteError.set(null);
            }
        });
    }

    protected openConfirmDialog(action: SiteDangerAction) {
        if (this.isBusy()) return;
        this.confirmValue.set('');
        this.pendingAction.set(action);
    }

    protected onConfirmDialogVisibleChange(visible: boolean) {
        if (!visible && !this.isBusy()) {
            this.pendingAction.set(null);
            this.confirmValue.set('');
        }
    }

    protected runConfirmedAction() {
        if (!this.canSubmitConfirm()) return;
        switch (this.pendingAction()) {
            case 'reset':
                this.resetStats();
                return;
            case 'delete':
                this.deleteSite();
                return;
        }
    }

    deleteSite() {
        const site = this.site();
        if (!site || this.isDeleting()) return;
        if (!this.canSubmitConfirm()) return;

        this.isDeleting.set(true);
        this.deleteError.set(null);
        this.siteService
            .deleteSite(site.id)
            .pipe(finalize(() => this.isDeleting.set(false)))
            .subscribe({
                next: () => {
                    this.pendingAction.set(null);
                    this.confirmValue.set('');
                    void this.router.navigate(['/overview']).then((navigated) => {
                        if (navigated) this.navigationNotice.show('sites.settings.notices.siteDeleted');
                    });
                },
                error: () => this.deleteError.set('sites.danger.deleteFailed')
            });
    }

    resetStats() {
        const site = this.site();
        if (!site || this.isResetting()) return;
        if (!this.canSubmitConfirm()) return;

        const confirmDomain = this.confirmValue().trim();
        this.isResetting.set(true);
        this.resetSuccess.set(null);
        this.resetError.set(null);
        this.siteService
            .resetSiteStats(site.id, confirmDomain)
            .pipe(finalize(() => this.isResetting.set(false)))
            .subscribe({
                next: (result) => {
                    this.resetSuccess.set(result);
                    this.pendingAction.set(null);
                    this.confirmValue.set('');
                    this.siteService.loadSites();
                },
                error: () => this.resetError.set('sites.danger.resetFailed')
            });
    }
}
