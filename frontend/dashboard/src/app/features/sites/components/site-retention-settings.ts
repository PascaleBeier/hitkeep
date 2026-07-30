import { ChangeDetectionStrategy, Component, inject, signal, effect, input } from '@angular/core';

import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { InputNumberModule } from '@openng/optimus-ui/inputnumber';
import { SiteService } from '@features/sites/services/site.service';
import { AnalyticsService } from '@services/analytics.service';
import { Site } from '@models/analytics.types';

@Component({
    selector: 'app-site-retention-settings',
    standalone: true,
    imports: [ReactiveFormsModule, ButtonModule, InputNumberModule, TranslocoPipe],
    template: `
        <div class="site-settings-stack">
            <section class="site-settings-card">
                <header class="site-settings-card__header">
                    <div class="site-settings-card__title-row">
                        <span class="site-settings-card__icon"><i class="pi pi-history" aria-hidden="true"></i></span>
                        <div>
                            <h3>{{ "sites.retention.infoTitle" | transloco }}</h3>
                            <p>{{ "sites.retention.infoDescription" | transloco }}</p>
                        </div>
                    </div>
                </header>
                <div class="site-settings-card__body">
                    <div class="site-settings-field">
                        <label for="retentionDays">{{ "sites.retention.periodLabel" | transloco }}</label>
                        <div class="site-settings-inline-control site-settings-inline-control--retention">
                            <p-inputnumber inputId="retentionDays"
                                [formControl]="retentionForm.retentionDays().control()"
                                [min]="1"
                                [max]="3650"
                                [showButtons]="true"
                                buttonLayout="horizontal"
                                spinnerMode="horizontal"
                                decrementButtonClass="p-button-secondary"
                                incrementButtonClass="p-button-secondary"
                                incrementButtonIcon="pi pi-plus"
                                decrementButtonIcon="pi pi-minus"
                                styleClass="site-settings-number-input"
                             />
                            <span class="site-settings-field-hint">{{ "sites.retention.hotDataSuffix" | transloco }}</span>
                        </div>
                        <small class="site-settings-field-hint">{{ "sites.retention.defaultHint" | transloco }}</small>
                    </div>
                </div>
                <footer class="site-settings-card__footer">
                    <p-button styleClass="site-settings-action-btn" [label]="'sites.retention.savePolicy' | transloco" icon="pi pi-check" (onClick)="savePolicy()" [loading]="saving()" [disabled]="!hasChanged()" />
                </footer>
            </section>
        </div>
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteRetentionSettings {
    site = input.required<Site | null>();
    private siteService = inject(SiteService);
    private analyticsService = inject(AnalyticsService);

    private readonly retentionFormModel = signal({
        retentionDays: new FormControl(365, { nonNullable: true })
    });
    protected readonly retentionForm = compatForm(this.retentionFormModel);
    saving = signal(false);
    originalDays = signal<number>(365);

    constructor() {
        effect(() => {
            const site = this.site();
            if (site) {
                const days = site.data_retention_days ?? 365;
                this.retentionForm.retentionDays().control().setValue(days, { emitEvent: false });
                this.originalDays.set(days);
            }
        });
    }

    hasChanged() {
        return this.retentionForm.retentionDays().value() !== this.originalDays();
    }

    savePolicy() {
        const site = this.site();
        if (!site) return;
        const retentionDays = this.retentionForm.retentionDays().value();

        this.saving.set(true);
        this.analyticsService.updateSiteRetention(site.id, retentionDays).subscribe({
            next: () => {
                this.saving.set(false);
                this.originalDays.set(retentionDays);
                // Optionally refresh site data
                this.siteService.loadSites();
            },
            error: () => this.saving.set(false)
        });
    }
}
