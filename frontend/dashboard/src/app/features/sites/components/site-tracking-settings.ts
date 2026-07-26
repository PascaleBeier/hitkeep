import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, input, signal } from '@angular/core';
import { takeUntilDestroyed, toSignal } from '@angular/core/rxjs-interop';

import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { Site, SiteTrackingDomainOptions } from '@models/analytics.types';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { SelectModule } from '@openng/optimus-ui/select';
import { TagModule } from '@openng/optimus-ui/tag';
import { ToggleSwitchModule } from '@openng/optimus-ui/toggleswitch';
import { SiteService, SiteTrackingStatus } from '@features/sites/services/site.service';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { browserAbsoluteAppUrl } from '@core/interceptors/base-path.interceptor';
import { finalize } from 'rxjs';

interface TrackingDomainSelectOption {
    label: string;
    value: string | null;
}

@Component({
    selector: 'app-site-tracking-settings',
    standalone: true,
    imports: [ReactiveFormsModule, ButtonModule, SelectModule, TagModule, ToggleSwitchModule, RelativeDateTime, TranslocoPipe],
    template: `
        <div class="site-settings-stack">
            <section class="site-settings-card">
                <header class="site-settings-card__header">
                    <div class="site-settings-card__title-row">
                        <span class="site-settings-card__icon"><i class="pi pi-code" aria-hidden="true"></i></span>
                        <div>
                            <h3>{{ "sites.tracking.htmlLabel" | transloco }}</h3>
                            <p>{{ "sites.tracking.trackingCodeConfiguration" | transloco }}</p>
                        </div>
                    </div>
                    <p-button [label]="copyButtonLabel() | transloco" [icon]="copyButtonIcon()" [text]="true" [type]="'button'" size="small" (onClick)="copySnippet()" />
                </header>
                <div class="site-settings-card__body">
                    <pre class="site-settings-code-panel">{{ snippetCode() }}</pre>
                </div>
            </section>
            <section class="site-settings-card">
                <header class="site-settings-card__header">
                    <div class="site-settings-card__title-row">
                        <span class="site-settings-card__icon"><i class="pi pi-bolt" aria-hidden="true"></i></span>
                        <div>
                            <h3>{{ "sites.tracking.verifier.title" | transloco }}</h3>
                            <p>{{ "sites.tracking.verifier.description" | transloco }}</p>
                        </div>
                    </div>
                    <div class="site-settings-verifier-actions">
                        @if (trackingStatus(); as status) {
                        <p-tag [severity]="statusSeverity()" [value]="statusLabelKey() | transloco" />
                        }
                        <p-button [ariaLabel]="'common.actions.refresh' | transloco" icon="pi pi-refresh" [text]="true" [rounded]="true" [loading]="isLoadingStatus()" [type]="'button'" size="small" (onClick)="refreshStatus()" />
                    </div>
                </header>
                <div class="site-settings-card__body">
                    @if (trackingStatus(); as status) {
                    <div class="site-settings-status-grid">
                        <div>
                            <span>{{ "sites.tracking.verifier.fields.firstHit" | transloco }}</span>
                            <strong>
                                @if (status.first_hit_at) {
                                <app-relative-date-time [value]="status.first_hit_at" />
                                } @else { - }
                            </strong>
                        </div>
                        <div>
                            <span>{{ "sites.tracking.verifier.fields.lastHit" | transloco }}</span>
                            <strong>
                                @if (status.last_hit_at) {
                                <app-relative-date-time [value]="status.last_hit_at" />
                                } @else { - }
                            </strong>
                        </div>
                        <div>
                            <span>{{ "sites.tracking.verifier.fields.lastEvent" | transloco }}</span>
                            <strong>
                                @if (status.last_event_at) {
                                <app-relative-date-time [value]="status.last_event_at" />
                                } @else { - }
                            </strong>
                        </div>
                        <div>
                            <span>{{ "sites.tracking.verifier.fields.detectedHostname" | transloco }}</span>
                            <strong>{{ status.last_hostname || "-" }}</strong>
                        </div>
                        <div>
                            <span>{{ "sites.tracking.verifier.fields.automaticEvent" | transloco }}</span>
                            <strong>{{ status.last_automatic_event_name || "-" }}</strong>
                        </div>
                        <div>
                            <span>{{ "sites.tracking.verifier.fields.tracker" | transloco }}</span>
                            <strong>{{ trackerLabel(status) }}</strong>
                        </div>
                    </div>
                    @if (status.status === "domain_mismatch") {
                    <div class="site-settings-alert site-settings-alert--warn">
                        {{ "sites.tracking.verifier.mismatchHint" | transloco }}
                    </div>
                    } @else if (status.status === "waiting") {
                    <div class="site-settings-alert site-settings-alert--info">
                        {{ "sites.tracking.verifier.waitingHint" | transloco }}
                    </div>
                    }
                    } @else if (isLoadingStatus()) {
                    <div class="site-settings-empty"><i class="pi pi-spin pi-spinner" aria-hidden="true"></i>{{ "common.loading" | transloco }}</div>
                    } @else {
                    <div class="site-settings-empty">{{ "sites.tracking.verifier.empty" | transloco }}</div>
                    }
                </div>
            </section>

            <section class="site-settings-card">
                <header class="site-settings-card__header">
                    <div class="site-settings-card__title-row">
                        <span class="site-settings-card__icon"><i class="pi pi-sliders-h" aria-hidden="true"></i></span>
                        <div>
                            <h3>{{ "sites.tracking.trackingCodeConfiguration" | transloco }}</h3>
                            <p>{{ "sites.tracking.description" | transloco }}</p>
                        </div>
                    </div>
                </header>
                <div class="site-settings-card__body">
                    <div class="site-settings-toggle-list">
                        <div class="site-settings-toggle-row site-settings-toggle-row--domain">
                            <div class="site-settings-toggle-row__text">
                                <label id="tracking-domain-label" for="tracking-domain-select" class="site-settings-toggle-row__title">{{ "sites.tracking.domains.label" | transloco }}</label>
                                <span class="site-settings-field-hint">{{ "sites.tracking.domains.description" | transloco }}</span>
                            </div>
                            <div class="site-settings-domain-select">
                                <p-select
                                    inputId="tracking-domain-select"
                                    ariaLabelledBy="tracking-domain-label"
                                    styleClass="w-full"
                                    [options]="trackingDomainSelectOptions()"
                                    optionLabel="label"
                                    optionValue="value"
                                    [formControl]="trackingDomainControl"
                                    [loading]="isLoadingTrackingDomains()"
                                    appendTo="body"
                                />
                                <span class="site-settings-field-hint">{{ selectedTrackingDomainHint() }}</span>
                            </div>
                        </div>

                        <div class="site-settings-toggle-row">
                            <div class="site-settings-toggle-row__text">
                                <label id="collect-dnt-label" for="collect-dnt-switch" class="site-settings-toggle-row__title">{{ "sites.tracking.collectDntLabel" | transloco }}</label>
                                <span class="site-settings-field-hint">{{ "sites.tracking.collectDntDescription" | transloco }}</span>
                            </div>
                            <p-toggleswitch inputId="collect-dnt-switch" ariaLabelledBy="collect-dnt-label" styleClass="shrink-0" [formControl]="trackingForm.collectDnt().control()"></p-toggleswitch>
                        </div>

                        <div class="site-settings-toggle-row">
                            <div class="site-settings-toggle-row__text">
                                <label id="disable-beacon-label" for="disable-beacon-switch" class="site-settings-toggle-row__title">{{ "sites.tracking.disableBeaconLabel" | transloco }}</label>
                                <span class="site-settings-field-hint">{{ "sites.tracking.disableBeaconDescription" | transloco }}</span>
                            </div>
                            <p-toggleswitch inputId="disable-beacon-switch" ariaLabelledBy="disable-beacon-label" styleClass="shrink-0" [formControl]="trackingForm.disableBeacon().control()"></p-toggleswitch>
                        </div>

                        <div class="site-settings-toggle-row">
                            <div class="site-settings-toggle-row__text">
                                <label id="web-vitals-label" for="web-vitals-switch" class="site-settings-toggle-row__title">{{ "sites.tracking.webVitalsLabel" | transloco }}</label>
                                <span class="site-settings-field-hint">{{ "sites.tracking.webVitalsDescription" | transloco }}</span>
                            </div>
                            <p-toggleswitch inputId="web-vitals-switch" ariaLabelledBy="web-vitals-label" styleClass="shrink-0" [formControl]="trackingForm.enableWebVitals().control()"></p-toggleswitch>
                        </div>

                        <div class="site-settings-subsection">
                            <h4>{{ "sites.tracking.autoTrackingTitle" | transloco }}</h4>
                            <p>{{ "sites.tracking.autoTrackingDescription" | transloco }}</p>
                        </div>

                        <div class="site-settings-toggle-row">
                            <div class="site-settings-toggle-row__text">
                                <label id="outbound-tracking-label" for="outbound-tracking-switch" class="site-settings-toggle-row__title">{{ "sites.tracking.outboundTrackingLabel" | transloco }}</label>
                                <span class="site-settings-field-hint">{{ "sites.tracking.outboundTrackingDescription" | transloco }}</span>
                            </div>
                            <p-toggleswitch inputId="outbound-tracking-switch" ariaLabelledBy="outbound-tracking-label" styleClass="shrink-0" [formControl]="trackingForm.trackOutbound().control()"></p-toggleswitch>
                        </div>

                        <div class="site-settings-toggle-row">
                            <div class="site-settings-toggle-row__text">
                                <label id="download-tracking-label" for="download-tracking-switch" class="site-settings-toggle-row__title">{{ "sites.tracking.downloadTrackingLabel" | transloco }}</label>
                                <span class="site-settings-field-hint">{{ "sites.tracking.downloadTrackingDescription" | transloco }}</span>
                            </div>
                            <p-toggleswitch inputId="download-tracking-switch" ariaLabelledBy="download-tracking-label" styleClass="shrink-0" [formControl]="trackingForm.trackDownloads().control()"></p-toggleswitch>
                        </div>

                        <div class="site-settings-toggle-row">
                            <div class="site-settings-toggle-row__text">
                                <label id="form-tracking-label" for="form-tracking-switch" class="site-settings-toggle-row__title">{{ "sites.tracking.formTrackingLabel" | transloco }}</label>
                                <span class="site-settings-field-hint">{{ "sites.tracking.formTrackingDescription" | transloco }}</span>
                            </div>
                            <p-toggleswitch inputId="form-tracking-switch" ariaLabelledBy="form-tracking-label" styleClass="shrink-0" [formControl]="trackingForm.trackForms().control()"></p-toggleswitch>
                        </div>
                    </div>
                </div>
            </section>
        </div>
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteTrackingSettings {
    private siteService = inject(SiteService);
    private destroyRef = inject(DestroyRef);
    private document = inject(DOCUMENT);
    private transloco = inject(TranslocoService);
    site = input.required<Site | null>();
    protected trackingStatus = signal<SiteTrackingStatus | null>(null);
    protected isLoadingStatus = signal(false);
    protected trackingDomainOptions = signal<SiteTrackingDomainOptions | null>(null);
    protected isLoadingTrackingDomains = signal(false);
    private copyResetTimer: ReturnType<typeof setTimeout> | null = null;
    private statusRequestID = 0;
    private statusLoadingRequestID = 0;
    private domainOptionsRequestID = 0;
    protected readonly trackingDomainControl = new FormControl<string | null>(null);
    private readonly selectedTrackingDomainID = toSignal(this.trackingDomainControl.valueChanges, { initialValue: this.trackingDomainControl.value });
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });
    private readonly trackingFormModel = signal({
        collectDnt: new FormControl(false, { nonNullable: true }),
        disableBeacon: new FormControl(false, { nonNullable: true }),
        enableWebVitals: new FormControl(false, { nonNullable: true }),
        trackOutbound: new FormControl(true, { nonNullable: true }),
        trackDownloads: new FormControl(true, { nonNullable: true }),
        trackForms: new FormControl(true, { nonNullable: true })
    });
    protected readonly trackingForm = compatForm(this.trackingFormModel);
    protected copyButtonLabel = signal('sites.tracking.copyCode');
    protected copyButtonIcon = signal('pi pi-copy');
    protected statusLabelKey = computed(() => `sites.tracking.verifier.status.${this.trackingStatus()?.status ?? 'waiting'}`);
    protected activeTrackingDomains = computed(() => (this.trackingDomainOptions()?.domains ?? []).filter((domain) => domain.active));
    protected selectedSnippetTrackingDomain = computed(() => {
        const selectedID = this.selectedTrackingDomainID();
        return selectedID ? (this.activeTrackingDomains().find((domain) => domain.id === selectedID) ?? null) : null;
    });
    protected trackingDomainSelectOptions = computed<TrackingDomainSelectOption[]>(() => {
        this.activeLanguage();
        return [{ label: this.transloco.translate('sites.tracking.domains.defaultOption'), value: null }, ...this.activeTrackingDomains().map((domain) => ({ label: domain.hostname, value: domain.id }))];
    });
    protected selectedTrackingDomainHint = computed(() => {
        this.activeLanguage();
        const selectedDomain = this.selectedSnippetTrackingDomain();
        if (selectedDomain) {
            return this.transloco.translate('sites.tracking.domains.selectedHint', { hostname: selectedDomain.hostname });
        }
        return this.trackingDomainOptions()?.default_url || this.defaultTrackerURL();
    });
    protected statusSeverity = computed<'success' | 'warn' | 'secondary' | 'danger'>(() => {
        switch (this.trackingStatus()?.status) {
            case 'live':
                return 'success';
            case 'dormant':
                return 'warn';
            case 'domain_mismatch':
                return 'danger';
            default:
                return 'secondary';
        }
    });

    protected snippetCode = computed(() => {
        const selectedDomain = this.selectedSnippetTrackingDomain();
        const scriptURL = selectedDomain ? `https://${selectedDomain.hostname}/hk.js` : this.defaultTrackerURL();

        let attrs = '';
        if (this.trackingForm.collectDnt().value()) attrs += ' data-collect-dnt="true"';
        if (this.trackingForm.disableBeacon().value()) attrs += ' data-disable-beacon="true"';
        if (this.trackingForm.enableWebVitals().value()) attrs += ' data-enable-web-vitals="true"';
        if (!this.trackingForm.trackOutbound().value()) attrs += ' data-disable-outbound-tracking="true"';
        if (!this.trackingForm.trackDownloads().value()) attrs += ' data-disable-download-tracking="true"';
        if (!this.trackingForm.trackForms().value()) attrs += ' data-disable-form-tracking="true"';

        return `<script async src="${scriptURL}"${attrs}></script>`;
    });

    copySnippet() {
        const clipboard = navigator.clipboard;
        if (!clipboard) {
            this.setCopyButtonState('common.saveFailed', 'pi pi-exclamation-triangle');
            return;
        }

        clipboard
            .writeText(this.snippetCode())
            .then(() => {
                this.setCopyButtonState('common.copied', 'pi pi-check');
            })
            .catch(() => {
                this.setCopyButtonState('common.saveFailed', 'pi pi-exclamation-triangle');
            });
    }

    private setCopyButtonState(label: string, icon: string) {
        this.copyButtonLabel.set(label);
        this.copyButtonIcon.set(icon);
        if (this.copyResetTimer) {
            clearTimeout(this.copyResetTimer);
        }
        this.copyResetTimer = setTimeout(() => this.resetCopyButton(), 2000);
    }

    private resetCopyButton() {
        this.copyResetTimer = null;
        this.copyButtonLabel.set('sites.tracking.copyCode');
        this.copyButtonIcon.set('pi pi-copy');
    }

    constructor() {
        effect((onCleanup) => {
            const site = this.site();
            this.statusRequestID += 1;
            this.trackingStatus.set(null);
            this.trackingDomainOptions.set(null);
            this.trackingDomainControl.setValue(null);
            if (!site) {
                return;
            }

            this.loadStatus(site.id);
            this.loadTrackingDomainOptions(site.id);
            const startedAt = Date.now();
            const timer = setInterval(() => {
                const current = this.trackingStatus();
                if (Date.now() - startedAt > 120000 || (current && current.status !== 'waiting')) {
                    clearInterval(timer);
                    return;
                }
                this.loadStatus(site.id, { quiet: true });
            }, 3000);
            onCleanup(() => clearInterval(timer));
        });

        this.destroyRef.onDestroy(() => {
            if (this.copyResetTimer) {
                clearTimeout(this.copyResetTimer);
            }
        });
    }

    protected refreshStatus() {
        const site = this.site();
        if (!site) return;
        this.loadStatus(site.id);
    }

    protected defaultTrackerURL(): string {
        return browserAbsoluteAppUrl(this.document, '/hk.js');
    }

    protected trackerLabel(status: SiteTrackingStatus): string {
        const source = status.tracker_source || 'hk.js';
        return status.tracker_version ? `${source} ${status.tracker_version}` : source;
    }

    private loadStatus(siteId: string, options: { quiet?: boolean } = {}) {
        const requestID = ++this.statusRequestID;
        const loadingRequestID = options.quiet ? this.statusLoadingRequestID : ++this.statusLoadingRequestID;
        if (!options.quiet) {
            this.isLoadingStatus.set(true);
        }
        this.siteService
            .getTrackingStatus(siteId)
            .pipe(
                finalize(() => {
                    if (!options.quiet && loadingRequestID === this.statusLoadingRequestID) {
                        this.isLoadingStatus.set(false);
                    }
                }),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: (status) => {
                    if (requestID === this.statusRequestID && this.site()?.id === siteId) {
                        this.trackingStatus.set(status);
                    }
                },
                error: () => {
                    if (requestID === this.statusRequestID && this.site()?.id === siteId) {
                        this.trackingStatus.set(null);
                    }
                }
            });
    }

    private loadTrackingDomainOptions(siteId: string) {
        const requestID = ++this.domainOptionsRequestID;
        this.isLoadingTrackingDomains.set(true);
        this.siteService
            .getTrackingDomainOptions(siteId)
            .pipe(
                finalize(() => {
                    if (requestID === this.domainOptionsRequestID) {
                        this.isLoadingTrackingDomains.set(false);
                    }
                }),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: (options) => {
                    if (requestID === this.domainOptionsRequestID && this.site()?.id === siteId) {
                        this.applyTrackingDomainOptions(options);
                    }
                },
                error: () => {
                    if (requestID === this.domainOptionsRequestID && this.site()?.id === siteId) {
                        this.trackingDomainOptions.set(null);
                        this.trackingDomainControl.setValue(null);
                    }
                }
            });
    }

    private applyTrackingDomainOptions(options: SiteTrackingDomainOptions) {
        this.trackingDomainOptions.set(options);
        const selectedID = this.trackingDomainControl.value;
        if (selectedID && !options.domains.some((domain) => domain.id === selectedID && domain.active)) {
            this.trackingDomainControl.setValue(null);
        }
    }
}
