import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, input, signal } from '@angular/core';
import { takeUntilDestroyed, toSignal } from '@angular/core/rxjs-interop';

import { FormControl, FormsModule, ReactiveFormsModule } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { Site, SiteTrackingDomainOptions } from '@models/analytics.types';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { FieldsetModule } from '@openng/optimus-ui/fieldset';
import { MessageModule } from '@openng/optimus-ui/message';
import { SelectModule } from '@openng/optimus-ui/select';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';
import { TagModule } from '@openng/optimus-ui/tag';
import { ToggleSwitchModule } from '@openng/optimus-ui/toggleswitch';
import { SiteService, SiteTrackingStatus } from '@features/sites/services/site.service';
import { CodeBlock } from '@components/code-block/code-block';
import { CopyControl } from '@components/copy-control/copy-control';
import { DocsLink, DocsLinkVariant } from '@components/docs-link/docs-link';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { SettingsCard } from '@features/settings/components/settings-card';
import { DOCS_LINKS, NPM_PACKAGE_NAME, NPM_PACKAGE_URL, WORDPRESS_PLUGIN_URL } from '@core/config/docs-links';
import { injectActiveLang } from '@core/i18n/active-lang';
import { DISCLOSURE_FIELDSET_DESIGN_TOKENS } from '@core/theme/hitkeep-preset';
import { browserAbsoluteAppUrl } from '@core/interceptors/base-path.interceptor';
import { finalize } from 'rxjs';

interface TrackingDomainSelectOption {
    label: string;
    value: string | null;
}

const INSTALL_METHODS = ['script', 'npm', 'wordpress', 'server'] as const;

/** The four supported ways to get data into HitKeep. */
export type InstallMethod = (typeof INSTALL_METHODS)[number];

interface InstallLink {
    href: string;
    labelKey: string;
    testId: string;
    variant: DocsLinkVariant;
}

/** Each method's guides, primary first. Rendered as one row below the method body. */
const INSTALL_LINKS: Record<InstallMethod, readonly InstallLink[]> = {
    script: [{ href: DOCS_LINKS.trackerArchitecture, labelKey: 'sites.tracking.install.script.docsAction', testId: 'tracking-script-docs-link', variant: 'outlined' }],
    npm: [
        { href: DOCS_LINKS.npmPackage, labelKey: 'sites.tracking.install.npm.docsAction', testId: 'tracking-npm-docs-link', variant: 'outlined' },
        { href: NPM_PACKAGE_URL, labelKey: 'sites.tracking.install.npm.registryAction', testId: 'tracking-npm-registry-link', variant: 'text' }
    ],
    wordpress: [
        { href: WORDPRESS_PLUGIN_URL, labelKey: 'sites.tracking.install.wordpress.directoryAction', testId: 'tracking-wordpress-directory-link', variant: 'outlined' },
        { href: DOCS_LINKS.wordpress, labelKey: 'sites.tracking.install.wordpress.docsAction', testId: 'tracking-wordpress-docs-link', variant: 'text' }
    ],
    server: [
        { href: DOCS_LINKS.serverSideTracking, labelKey: 'sites.tracking.install.server.docsAction', testId: 'tracking-server-docs-link', variant: 'outlined' },
        { href: DOCS_LINKS.apiClients, labelKey: 'sites.tracking.install.server.tokenAction', testId: 'tracking-server-token-link', variant: 'text' }
    ]
};

@Component({
    selector: 'app-site-tracking-settings',
    standalone: true,
    imports: [FormsModule, ReactiveFormsModule, ButtonModule, FieldsetModule, MessageModule, SelectModule, SelectButtonModule, TagModule, ToggleSwitchModule, CodeBlock, CopyControl, DocsLink, RelativeDateTime, SettingsCard, TranslocoPipe],
    templateUrl: './site-tracking-settings.html',
    styleUrl: './site-tracking-settings.css',
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
    private statusRequestID = 0;
    private statusLoadingRequestID = 0;
    private domainOptionsRequestID = 0;
    /** The instance's own tracker URL. Derived from `<base href>`, which never changes. */
    private readonly defaultTrackerURL = browserAbsoluteAppUrl(this.document, '/hk.js');
    protected readonly trackingDomainControl = new FormControl<string | null>(null);
    private readonly selectedTrackingDomainID = toSignal(this.trackingDomainControl.valueChanges, { initialValue: this.trackingDomainControl.value });
    private readonly activeLanguage = injectActiveLang();
    private readonly trackingFormModel = signal({
        collectDnt: new FormControl(false, { nonNullable: true }),
        disableBeacon: new FormControl(false, { nonNullable: true }),
        enableWebVitals: new FormControl(false, { nonNullable: true }),
        trackOutbound: new FormControl(true, { nonNullable: true }),
        trackDownloads: new FormControl(true, { nonNullable: true }),
        trackForms: new FormControl(true, { nonNullable: true })
    });
    protected readonly trackingForm = compatForm(this.trackingFormModel);

    protected readonly installLinks = INSTALL_LINKS;
    protected readonly fieldsetDesignTokens = DISCLOSURE_FIELDSET_DESIGN_TOKENS;

    protected readonly installMethod = signal<InstallMethod>('script');
    protected readonly verifierExpanded = signal(false);

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
        return this.trackingDomainOptions()?.default_url || this.defaultTrackerURL;
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

    protected readonly installMethodOptions = computed(() => {
        this.activeLanguage();
        return INSTALL_METHODS.map((value) => ({ value, label: this.transloco.translate(`sites.tracking.install.methods.${value}`) }));
    });

    protected readonly installTitle = computed(() => {
        this.activeLanguage();
        const domain = this.site()?.domain;
        return domain ? this.transloco.translate('sites.tracking.install.title', { domain }) : this.transloco.translate('sites.tracking.install.titleFallback');
    });

    /** URL of the tracker script itself, honoring a selected custom tracking domain. */
    private readonly snippetScriptURL = computed(() => {
        const selectedDomain = this.selectedSnippetTrackingDomain();
        return selectedDomain ? `https://${selectedDomain.hostname}/hk.js` : this.defaultTrackerURL;
    });

    /**
     * Base URL of the HitKeep instance serving the tracker. Derived by stripping the
     * script filename rather than recomputing, so subdirectory installs keep their prefix.
     */
    protected readonly trackerHostURL = computed(() => this.snippetScriptURL().replace(/\/hk\.js$/, ''));

    protected readonly snippetCode = computed(() => {
        const scriptURL = this.snippetScriptURL();

        let attrs = '';
        if (this.trackingForm.collectDnt().value()) attrs += ' data-collect-dnt="true"';
        if (this.trackingForm.disableBeacon().value()) attrs += ' data-disable-beacon="true"';
        if (this.trackingForm.enableWebVitals().value()) attrs += ' data-enable-web-vitals="true"';
        if (!this.trackingForm.trackOutbound().value()) attrs += ' data-disable-outbound-tracking="true"';
        if (!this.trackingForm.trackDownloads().value()) attrs += ' data-disable-download-tracking="true"';
        if (!this.trackingForm.trackForms().value()) attrs += ' data-disable-form-tracking="true"';

        return `<script async src="${scriptURL}"${attrs}></script>`;
    });

    protected readonly npmInstallCommand = `npm install ${NPM_PACKAGE_NAME}`;

    protected readonly npmInitSnippet = computed(() => `import { init } from '${NPM_PACKAGE_NAME}';\n\ninit({ host: '${this.trackerHostURL()}' });`);

    protected readonly serverIngestSnippet = computed(() => {
        const domain = this.site()?.domain || 'example.com';
        return [
            `curl -X POST "${this.trackerHostURL()}/api/ingest/server/pageview" \\`,
            '  -H "Authorization: Bearer $HITKEEP_API_TOKEN" \\',
            '  -H "Content-Type: application/json" \\',
            "  -d '{",
            `    "url": "https://${domain}/pricing",`,
            '    "timestamp": "2026-04-03T12:30:45Z",',
            '    "visitor_ip": "203.0.113.42",',
            '    "user_agent": "Mozilla/5.0"',
            "  }'"
        ].join('\n');
    });

    /** How many snippet options differ from their defaults, shown on the collapsed fieldset. */
    protected readonly changedOptionCount = computed(
        () =>
            [
                this.trackingForm.collectDnt().value(),
                this.trackingForm.disableBeacon().value(),
                this.trackingForm.enableWebVitals().value(),
                !this.trackingForm.trackOutbound().value(),
                !this.trackingForm.trackDownloads().value(),
                !this.trackingForm.trackForms().value(),
                this.selectedSnippetTrackingDomain() !== null
            ].filter(Boolean).length
    );

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
    }

    protected selectInstallMethod(method: InstallMethod | null) {
        if (method) {
            this.installMethod.set(method);
        }
    }

    protected toggleVerifierDetails() {
        this.verifierExpanded.update((expanded) => !expanded);
    }

    protected refreshStatus() {
        const site = this.site();
        if (!site) return;
        this.loadStatus(site.id);
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
