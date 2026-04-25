import { ChangeDetectionStrategy, Component, computed, input, signal } from '@angular/core';

import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { Site } from '@models/analytics.types';
import { TranslocoPipe } from '@jsverse/transloco';
import { ToggleSwitchModule } from 'primeng/toggleswitch';

@Component({
    selector: 'app-site-tracking-settings',
    standalone: true,
    imports: [ReactiveFormsModule, ToggleSwitchModule, TranslocoPipe],
    template: `
        <div class="flex flex-col gap-6">
            <div class="text-[var(--p-text-muted-color)] leading-relaxed">
                <p>{{ "sites.tracking.description" | transloco }}</p>
            </div>

            <div class="flex flex-col gap-4 py-4">
                <h4 class="sr-only">{{ "sites.tracking.trackingCodeConfiguration" | transloco }}</h4>

                <div class="flex items-center justify-between gap-3">
                    <div class="flex min-w-0 flex-col pr-3">
                        <label id="collect-dnt-label" for="collect-dnt-switch" class="font-medium">{{ "sites.tracking.collectDntLabel" | transloco }}</label>
                        <span class="text-xs text-[var(--p-text-muted-color)]">{{ "sites.tracking.collectDntDescription" | transloco }}</span>
                    </div>
                    <p-toggleswitch inputId="collect-dnt-switch" ariaLabelledBy="collect-dnt-label" styleClass="shrink-0" [formControl]="trackingForm.collectDnt().control()"></p-toggleswitch>
                </div>

                <div class="flex items-center justify-between gap-3">
                    <div class="flex min-w-0 flex-col pr-3">
                        <label id="disable-beacon-label" for="disable-beacon-switch" class="font-medium">{{ "sites.tracking.disableBeaconLabel" | transloco }}</label>
                        <span class="text-xs text-[var(--p-text-muted-color)]">{{ "sites.tracking.disableBeaconDescription" | transloco }}</span>
                    </div>
                    <p-toggleswitch inputId="disable-beacon-switch" ariaLabelledBy="disable-beacon-label" styleClass="shrink-0" [formControl]="trackingForm.disableBeacon().control()"></p-toggleswitch>
                </div>

                <div class="pt-2 border-t border-[var(--p-surface-border)]">
                    <p class="text-sm font-medium">{{ "sites.tracking.autoTrackingTitle" | transloco }}</p>
                    <p class="text-xs text-[var(--p-text-muted-color)] mt-1">{{ "sites.tracking.autoTrackingDescription" | transloco }}</p>
                </div>

                <div class="flex items-center justify-between gap-3">
                    <div class="flex min-w-0 flex-col pr-3">
                        <label id="outbound-tracking-label" for="outbound-tracking-switch" class="font-medium">{{ "sites.tracking.outboundTrackingLabel" | transloco }}</label>
                        <span class="text-xs text-[var(--p-text-muted-color)]">{{ "sites.tracking.outboundTrackingDescription" | transloco }}</span>
                    </div>
                    <p-toggleswitch inputId="outbound-tracking-switch" ariaLabelledBy="outbound-tracking-label" styleClass="shrink-0" [formControl]="trackingForm.trackOutbound().control()"></p-toggleswitch>
                </div>

                <div class="flex items-center justify-between gap-3">
                    <div class="flex min-w-0 flex-col pr-3">
                        <label id="download-tracking-label" for="download-tracking-switch" class="font-medium">{{ "sites.tracking.downloadTrackingLabel" | transloco }}</label>
                        <span class="text-xs text-[var(--p-text-muted-color)]">{{ "sites.tracking.downloadTrackingDescription" | transloco }}</span>
                    </div>
                    <p-toggleswitch inputId="download-tracking-switch" ariaLabelledBy="download-tracking-label" styleClass="shrink-0" [formControl]="trackingForm.trackDownloads().control()"></p-toggleswitch>
                </div>

                <div class="flex items-center justify-between gap-3">
                    <div class="flex min-w-0 flex-col pr-3">
                        <label id="form-tracking-label" for="form-tracking-switch" class="font-medium">{{ "sites.tracking.formTrackingLabel" | transloco }}</label>
                        <span class="text-xs text-[var(--p-text-muted-color)]">{{ "sites.tracking.formTrackingDescription" | transloco }}</span>
                    </div>
                    <p-toggleswitch inputId="form-tracking-switch" ariaLabelledBy="form-tracking-label" styleClass="shrink-0" [formControl]="trackingForm.trackForms().control()"></p-toggleswitch>
                </div>
            </div>

            <div class="rounded-md border border-[var(--p-surface-border)] bg-[var(--p-surface-50)] dark:bg-[var(--p-surface-900)] overflow-hidden">
                <div class="flex justify-between items-center px-3 py-2 border-b border-[var(--p-surface-border)] bg-[var(--p-surface-100)] dark:bg-[var(--p-surface-800)]">
                    <span class="text-xs font-mono font-medium text-[var(--p-text-muted-color)]">{{ "sites.tracking.htmlLabel" | transloco }}</span>
                    <button
                        class="flex items-center gap-2 px-3 py-1.5 rounded hover:bg-[var(--p-surface-200)] dark:hover:bg-[var(--p-surface-700)] transition-colors text-xs font-medium text-[var(--p-text-color)] cursor-pointer focus:outline-none focus:ring-2 focus:ring-[var(--p-primary-color)]"
                        (click)="copySnippet()"
                        [attr.aria-label]="copyButtonLabel() | transloco"
                    >
                        <i [class]="copyButtonIcon()"></i>
                        <span>{{ copyButtonLabel() | transloco }}</span>
                    </button>
                </div>

                <pre class="p-4 m-0 text-sm overflow-x-auto font-mono whitespace-pre-wrap break-all text-[var(--p-text-color)]">{{ snippetCode() }}</pre>
            </div>
        </div>
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteTrackingSettings {
    site = input.required<Site | null>();
    private readonly trackingFormModel = signal({
        collectDnt: new FormControl(false, { nonNullable: true }),
        disableBeacon: new FormControl(false, { nonNullable: true }),
        trackOutbound: new FormControl(true, { nonNullable: true }),
        trackDownloads: new FormControl(true, { nonNullable: true }),
        trackForms: new FormControl(true, { nonNullable: true })
    });
    protected readonly trackingForm = compatForm(this.trackingFormModel);
    protected copyButtonLabel = signal('sites.tracking.copyCode');
    protected copyButtonIcon = signal('pi pi-copy');

    protected snippetCode = computed(() => {
        const origin = window.location.origin;

        let attrs = '';
        if (this.trackingForm.collectDnt().value()) attrs += ' data-collect-dnt="true"';
        if (this.trackingForm.disableBeacon().value()) attrs += ' data-disable-beacon="true"';
        if (!this.trackingForm.trackOutbound().value()) attrs += ' data-disable-outbound-tracking="true"';
        if (!this.trackingForm.trackDownloads().value()) attrs += ' data-disable-download-tracking="true"';
        if (!this.trackingForm.trackForms().value()) attrs += ' data-disable-form-tracking="true"';

        return `<script async src="${origin}/hk.js"${attrs}></script>`;
    });

    copySnippet() {
        navigator.clipboard.writeText(this.snippetCode()).then(() => {
            this.copyButtonLabel.set('common.copied');
            this.copyButtonIcon.set('pi pi-check');
            setTimeout(() => this.resetCopyButton(), 2000);
        });
    }

    private resetCopyButton() {
        this.copyButtonLabel.set('sites.tracking.copyCode');
        this.copyButtonIcon.set('pi pi-copy');
    }
}
