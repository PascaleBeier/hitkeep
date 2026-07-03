import { ChangeDetectionStrategy, Component, computed, effect, inject, input, signal } from '@angular/core';
import { finalize } from 'rxjs';
import { TranslocoPipe } from '@jsverse/transloco';

import { Site } from '@models/analytics.types';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';
import { SiteService, SiteStatsResetResponse } from '@features/sites/services/site.service';
import { ButtonModule } from 'primeng/button';
import { InputTextModule } from 'primeng/inputtext';

@Component({
    selector: 'app-site-danger-zone',
    standalone: true,
    imports: [ButtonModule, InputTextModule, TranslocoPipe],
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
                        <div class="site-settings-field">
                            <label for="reset-site-confirm">{{ 'sites.danger.resetConfirmLabel' | transloco: { domain: confirmDomain() } }}</label>
                            <input
                                id="reset-site-confirm"
                                pInputText
                                class="w-full"
                                [value]="resetConfirmValue()"
                                #resetConfirmInput
                                (input)="resetConfirmValue.set(resetConfirmInput.value)"
                                [placeholder]="'sites.danger.confirmPlaceholder' | transloco"
                            />
                            <small class="site-settings-field-hint">{{ 'sites.danger.resetConfirmHint' | transloco }}</small>
                        </div>
                    </div>
                    <footer class="site-settings-card__footer">
                        <p-button styleClass="site-settings-danger-action" [label]="'sites.danger.resetAction' | transloco" icon="pi pi-refresh" severity="danger" [disabled]="!canConfirmReset()" [loading]="isResetting()" (onClick)="resetStats()" />
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
                        <div class="site-settings-field">
                            <label for="delete-site-confirm">{{ 'sites.danger.confirmLabel' | transloco: { domain: deleteConfirmDomain() } }}</label>
                            <input id="delete-site-confirm" pInputText class="w-full" [value]="confirmValue()" #confirmInput (input)="confirmValue.set(confirmInput.value)" [placeholder]="'sites.danger.confirmPlaceholder' | transloco" />
                        </div>
                    </div>
                    <footer class="site-settings-card__footer">
                        <p-button styleClass="site-settings-danger-action" [label]="'sites.danger.deleteAction' | transloco" icon="pi pi-trash" severity="danger" [disabled]="!canConfirmDelete()" [loading]="isDeleting()" (onClick)="deleteSite()" />
                    </footer>
                </section>
            }
        </div>
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteDangerZone {
    private access = inject(AccessService);
    private siteService = inject(SiteService);

    site = input.required<Site | null>();
    protected isDeleting = signal(false);
    protected isResetting = signal(false);
    protected confirmValue = signal('');
    protected resetConfirmValue = signal('');
    protected resetSuccess = signal<SiteStatsResetResponse | null>(null);
    protected resetError = signal<string | null>(null);
    protected deleteError = signal<string | null>(null);
    protected canDeleteSite = computed(() => {
        const site = this.site();
        if (!site) return false;
        return this.access.canSite(site.id, SITE_CAPABILITIES.delete);
    });
    protected canConfirmDelete = computed(() => {
        const site = this.site();
        if (!site) return false;
        return this.confirmValue().trim().toLowerCase() === site.domain.toLowerCase();
    });
    protected canConfirmReset = computed(() => {
        const site = this.site();
        if (!site) return false;
        return this.resetConfirmValue().trim().toLowerCase() === site.domain.toLowerCase();
    });
    protected confirmDomain = computed(() => this.site()?.domain ?? '');
    protected deleteConfirmDomain = this.confirmDomain;

    constructor() {
        effect(() => {
            const site = this.site();
            if (site) {
                this.confirmValue.set('');
                this.resetConfirmValue.set('');
                this.resetSuccess.set(null);
                this.resetError.set(null);
                this.deleteError.set(null);
            }
        });
    }

    deleteSite() {
        const site = this.site();
        if (!site || this.isDeleting()) return;
        if (!this.canConfirmDelete()) return;

        this.isDeleting.set(true);
        this.deleteError.set(null);
        this.siteService
            .deleteSite(site.id)
            .pipe(finalize(() => this.isDeleting.set(false)))
            .subscribe({
                error: () => this.deleteError.set('sites.danger.deleteFailed')
            });
    }

    resetStats() {
        const site = this.site();
        if (!site || this.isResetting()) return;
        if (!this.canConfirmReset()) return;

        const confirmDomain = this.resetConfirmValue().trim();
        this.isResetting.set(true);
        this.resetSuccess.set(null);
        this.resetError.set(null);
        this.siteService
            .resetSiteStats(site.id, confirmDomain)
            .pipe(finalize(() => this.isResetting.set(false)))
            .subscribe({
                next: (result) => {
                    this.resetSuccess.set(result);
                    this.resetConfirmValue.set('');
                    this.siteService.loadSites();
                },
                error: () => this.resetError.set('sites.danger.resetFailed')
            });
    }
}
