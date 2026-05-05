import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, signal, untracked } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { FormsModule } from '@angular/forms';
import { takeUntilDestroyed, toSignal } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ConfirmationService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { ConfirmDialogModule } from 'primeng/confirmdialog';
import { MessageModule } from 'primeng/message';
import { SelectModule } from 'primeng/select';
import { TagModule } from 'primeng/tag';
import { TooltipModule } from 'primeng/tooltip';
import { finalize } from 'rxjs';

import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { SiteService } from '@features/sites/services/site.service';
import { TeamService } from '@services/team.service';
import { GoogleSearchConsoleProperty, GoogleSearchConsoleService, GoogleSearchConsoleSiteMapping, GoogleSearchConsoleStatus } from '@services/google-search-console.service';

interface GoogleSearchConsoleNotice {
    kind: 'success' | 'warning';
    key: string;
}

@Component({
    selector: 'app-google-search-console-page',
    imports: [FormsModule, PageHeader, PageHeaderLeft, PageBreadcrumb, RelativeDateTime, ButtonModule, ConfirmDialogModule, MessageModule, SelectModule, TagModule, TooltipModule, TranslocoPipe],
    providers: [ConfirmationService],
    templateUrl: './google-search-console.html',
    styleUrl: './google-search-console.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class GoogleSearchConsolePage {
    private transloco = inject(TranslocoService);
    private confirmation = inject(ConfirmationService);
    private destroyRef = inject(DestroyRef);
    private teamService = inject(TeamService);
    private siteService = inject(SiteService);
    private integration = inject(GoogleSearchConsoleService);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    protected readonly status = signal<GoogleSearchConsoleStatus | null>(null);
    protected readonly properties = signal<GoogleSearchConsoleProperty[]>([]);
    protected readonly mapping = signal<GoogleSearchConsoleSiteMapping | null>(null);
    protected readonly selectedPropertyURI = signal('');
    protected readonly loading = signal(false);
    protected readonly propertyLoading = signal(false);
    protected readonly mappingLoading = signal(false);
    protected readonly actionBusy = signal(false);
    protected readonly propertyActionBusy = signal(false);
    protected readonly syncActionBusy = signal(false);
    protected readonly errorKey = signal<string | null>(null);
    protected readonly propertyErrorKey = signal<string | null>(null);
    protected readonly syncActionErrorKey = signal<string | null>(null);
    protected readonly notice = signal<GoogleSearchConsoleNotice | null>(null);
    private statusRequestID = 0;
    private propertiesRequestID = 0;
    private mappingRequestID = 0;

    protected readonly docsURL = 'https://hitkeep.com/guides/integrations/google-search-console/';
    protected readonly activeSite = computed(() => this.siteService.activeSite());

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('nav.integration'), routerLink: '/integration/api-clients' },
            { label: this.transloco.translate('integration.googleSearchConsole.title'), isCurrent: true }
        ];
    });

    protected readonly statusKey = computed(() => {
        const state = this.status()?.status ?? 'disconnected';
        return `integration.googleSearchConsole.status.${state}`;
    });

    protected readonly stateMessageKey = computed(() => {
        const state = this.status()?.status;
        if (state === 'credentials_missing') {
            return 'integration.googleSearchConsole.states.missingCredentials';
        }
        if (state === 'connected') {
            return null;
        }
        return 'integration.googleSearchConsole.states.disconnected';
    });

    protected readonly mappingMessageKey = computed(() => {
        if (this.mappingLoading()) {
            return 'integration.googleSearchConsole.states.loadingMapping';
        }
        if (this.mapping()?.mapped) {
            return 'integration.googleSearchConsole.states.propertyMapped';
        }
        if (this.showPropertyPicker() && this.properties().length === 0 && !this.propertyLoading() && !this.propertyErrorKey()) {
            return 'integration.googleSearchConsole.states.noProperties';
        }
        return 'integration.googleSearchConsole.states.propertyUnmapped';
    });

    protected readonly canManageMapping = computed(() => Boolean(this.status()?.can_manage && this.mapping()?.can_manage));
    protected readonly showPropertyPicker = computed(() => Boolean(this.canManageMapping() && !this.mapping()?.mapped));
    protected readonly propertyOptions = computed(() => this.properties().map((property) => ({ label: property.uri, value: property.uri })));
    protected readonly canMapSelectedProperty = computed(() => {
        const selected = this.selectedPropertyURI();
        return Boolean(selected && this.properties().some((property) => property.uri === selected));
    });

    protected readonly syncStatus = computed(() => this.mapping()?.sync_status ?? null);

    protected readonly syncStatusKey = computed(() => `integration.googleSearchConsole.sync.${this.syncStatus()?.state ?? 'idle'}`);

    protected readonly syncInProgress = computed(() => {
        const state = this.syncStatus()?.state;
        return state === 'pending' || state === 'running';
    });

    protected readonly syncButtonKey = computed(() => {
        if (this.syncActionBusy()) {
            return 'integration.googleSearchConsole.actions.syncRequesting';
        }
        if (this.syncStatus()?.state === 'pending') {
            return 'integration.googleSearchConsole.actions.syncQueued';
        }
        if (this.syncStatus()?.state === 'running') {
            return 'integration.googleSearchConsole.actions.syncRunning';
        }
        return 'integration.googleSearchConsole.actions.syncNow';
    });

    protected readonly syncFeedbackKey = computed(() => {
        const state = this.syncStatus()?.state;
        if (state === 'pending') {
            return 'integration.googleSearchConsole.sync.queuedFeedback';
        }
        if (state === 'running') {
            return 'integration.googleSearchConsole.sync.runningFeedback';
        }
        return null;
    });

    protected readonly syncSummaryKey = computed(() => {
        const syncStatus = this.syncStatus();
        if (!syncStatus) {
            return 'integration.googleSearchConsole.sync.noHistory';
        }
        if (syncStatus.state === 'pending' && syncStatus.manual) {
            return 'integration.googleSearchConsole.sync.manualPending';
        }
        if (syncStatus.imported_start_date && syncStatus.imported_end_date) {
            return 'integration.googleSearchConsole.sync.succeededWithRange';
        }
        return 'integration.googleSearchConsole.sync.noHistory';
    });

    protected readonly syncSummaryParams = computed(() => ({
        start: this.formatDateOnly(this.syncStatus()?.imported_start_date),
        end: this.formatDateOnly(this.syncStatus()?.imported_end_date)
    }));

    protected readonly syncImportedRangeLabel = computed(() => {
        const start = this.formatDateOnly(this.syncStatus()?.imported_start_date);
        const end = this.formatDateOnly(this.syncStatus()?.imported_end_date);
        if (!start && !end) {
            return '- - -';
        }
        return `${start || '-'} - ${end || '-'}`;
    });

    protected readonly syncErrorKey = computed(() => {
        const category = this.syncStatus()?.last_error_category?.trim();
        if (!category) {
            return null;
        }
        return `integration.googleSearchConsole.sync.${category}`;
    });

    protected readonly showReconnectHint = computed(() => {
        const category = this.syncStatus()?.last_error_category;
        return category === 'authorization_revoked' || category === 'token_refresh_failed' || category === 'credentials_invalid';
    });

    constructor() {
        effect(() => {
            const teamID = this.teamService.activeTeamId();
            if (!teamID) {
                this.status.set(null);
                return;
            }
            untracked(() => this.loadStatusForTeam(teamID));
        });

        effect(() => {
            const teamID = this.teamService.activeTeamId();
            const siteID = this.activeSite()?.id;
            const connected = this.status()?.connected;
            if (!teamID || !connected) {
                this.clearPropertyState();
                return;
            }

            if (siteID) {
                untracked(() => this.loadMappingForSite(siteID));
            } else {
                this.clearPropertyState();
            }
        });

        effect(() => {
            const teamID = this.teamService.activeTeamId();
            const connected = this.status()?.connected;
            const showPropertyPicker = this.showPropertyPicker();
            if (!teamID || !connected || !showPropertyPicker) {
                this.propertiesRequestID++;
                this.properties.set([]);
                this.propertyLoading.set(false);
                return;
            }
            untracked(() => this.loadPropertiesForTeam(teamID));
        });
    }

    protected loadStatus(): void {
        const teamID = this.teamService.activeTeamId();
        if (!teamID) {
            this.status.set(null);
            return;
        }
        this.loadStatusForTeam(teamID);
    }

    private loadStatusForTeam(teamID: string): void {
        const requestID = ++this.statusRequestID;
        this.loading.set(true);
        this.errorKey.set(null);
        this.status.set(null);
        this.clearPropertyState();
        this.integration
            .getStatus(teamID)
            .pipe(
                finalize(() => {
                    if (requestID === this.statusRequestID) {
                        this.loading.set(false);
                    }
                }),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: (status) => {
                    if (requestID === this.statusRequestID) {
                        this.status.set(status);
                    }
                },
                error: () => {
                    if (requestID === this.statusRequestID) {
                        this.errorKey.set('integration.googleSearchConsole.errors.load');
                    }
                }
            });
    }

    private loadPropertiesForTeam(teamID: string): void {
        const requestID = ++this.propertiesRequestID;
        this.propertyLoading.set(true);
        this.propertyErrorKey.set(null);
        this.properties.set([]);
        this.selectedPropertyURI.set('');
        this.integration
            .listProperties(teamID)
            .pipe(
                finalize(() => {
                    if (requestID === this.propertiesRequestID) {
                        this.propertyLoading.set(false);
                    }
                }),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: (response) => {
                    if (requestID !== this.propertiesRequestID) {
                        return;
                    }
                    this.properties.set(response.properties);
                    const selected = this.selectedPropertyURI();
                    if (!selected || !response.properties.some((property) => property.uri === selected)) {
                        this.selectedPropertyURI.set(response.properties[0]?.uri ?? '');
                    }
                },
                error: (error: unknown) => {
                    if (requestID === this.propertiesRequestID) {
                        this.properties.set([]);
                        this.selectedPropertyURI.set('');
                        this.propertyErrorKey.set(googleSearchConsolePropertyErrorKey(error));
                    }
                }
            });
    }

    private loadMappingForSite(siteID: string): void {
        this.clearPropertyState();
        const requestID = ++this.mappingRequestID;
        this.mappingLoading.set(true);
        this.propertyErrorKey.set(null);
        this.integration
            .getSiteMapping(siteID)
            .pipe(
                finalize(() => {
                    if (requestID === this.mappingRequestID) {
                        this.mappingLoading.set(false);
                    }
                }),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: (mapping) => {
                    if (requestID !== this.mappingRequestID) {
                        return;
                    }
                    this.mapping.set(mapping);
                    if (mapping.property_uri) {
                        this.selectedPropertyURI.set(mapping.property_uri);
                    } else {
                        this.selectedPropertyURI.set('');
                    }
                },
                error: () => {
                    if (requestID === this.mappingRequestID) {
                        this.propertyErrorKey.set('integration.googleSearchConsole.errors.mapping');
                    }
                }
            });
    }

    private clearPropertyState(): void {
        this.propertiesRequestID++;
        this.mappingRequestID++;
        this.properties.set([]);
        this.mapping.set(null);
        this.selectedPropertyURI.set('');
        this.propertyLoading.set(false);
        this.mappingLoading.set(false);
        this.propertyActionBusy.set(false);
        this.propertyErrorKey.set(null);
        this.syncActionErrorKey.set(null);
    }

    protected connect(): void {
        const teamID = this.teamService.activeTeamId();
        if (!teamID) {
            return;
        }

        this.actionBusy.set(true);
        this.errorKey.set(null);
        this.integration
            .connect(teamID, '/integration/google-search-console')
            .pipe(
                finalize(() => this.actionBusy.set(false)),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: (response) => {
                    window.location.assign(response.auth_url);
                },
                error: () => this.errorKey.set('integration.googleSearchConsole.errors.connect')
            });
    }

    protected confirmDisconnect(): void {
        const teamID = this.teamService.activeTeamId();
        if (!teamID || !this.status()?.can_manage) {
            return;
        }
        this.confirmation.confirm({
            message: this.transloco.translate('integration.googleSearchConsole.confirm.disconnectMessage'),
            header: this.transloco.translate('integration.googleSearchConsole.actions.confirmDisconnect'),
            icon: 'pi pi-exclamation-triangle',
            acceptLabel: this.transloco.translate('integration.googleSearchConsole.confirm.disconnectAccept'),
            rejectLabel: this.transloco.translate('integration.googleSearchConsole.confirm.cancel'),
            acceptButtonStyleClass: 'p-button-danger',
            accept: () => this.disconnect(teamID)
        });
    }

    private disconnect(teamID: string): void {
        this.actionBusy.set(true);
        this.errorKey.set(null);
        this.integration
            .disconnect(teamID)
            .pipe(
                finalize(() => this.actionBusy.set(false)),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: () => {
                    this.notice.set({ kind: 'success', key: 'integration.googleSearchConsole.states.disconnectSuccess' });
                    this.loadStatus();
                },
                error: () => this.errorKey.set('integration.googleSearchConsole.errors.disconnect')
            });
    }

    protected selectProperty(propertyURI: string): void {
        this.selectedPropertyURI.set(propertyURI);
    }

    protected mapSelectedProperty(): void {
        const siteID = this.activeSite()?.id;
        const propertyURI = this.selectedPropertyURI();
        if (!siteID || !propertyURI || !this.canMapSelectedProperty()) {
            return;
        }

        this.propertyActionBusy.set(true);
        this.propertyErrorKey.set(null);
        this.integration
            .mapSiteProperty(siteID, propertyURI)
            .pipe(
                finalize(() => this.propertyActionBusy.set(false)),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: (mapping) => {
                    if (!this.isCurrentSiteResponse(siteID, mapping.site_id)) {
                        return;
                    }
                    this.mapping.set(mapping);
                },
                error: () => this.propertyErrorKey.set('integration.googleSearchConsole.errors.mapProperty')
            });
    }

    protected confirmRemoveMapping(): void {
        const siteID = this.activeSite()?.id;
        if (!siteID || !this.mapping()?.mapped || !this.canManageMapping()) {
            return;
        }
        this.confirmation.confirm({
            message: this.transloco.translate('integration.googleSearchConsole.confirm.removeMappingMessage'),
            header: this.transloco.translate('integration.googleSearchConsole.actions.confirmRemoveMapping'),
            icon: 'pi pi-exclamation-triangle',
            acceptLabel: this.transloco.translate('integration.googleSearchConsole.confirm.removeMappingAccept'),
            rejectLabel: this.transloco.translate('integration.googleSearchConsole.confirm.cancel'),
            acceptButtonStyleClass: 'p-button-danger',
            accept: () => this.unmapProperty(siteID)
        });
    }

    private unmapProperty(siteID: string): void {
        this.propertyActionBusy.set(true);
        this.propertyErrorKey.set(null);
        this.integration
            .unmapSiteProperty(siteID)
            .pipe(
                finalize(() => this.propertyActionBusy.set(false)),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: (mapping) => {
                    if (!this.isCurrentSiteResponse(siteID, mapping.site_id)) {
                        return;
                    }
                    this.mapping.set(mapping);
                    this.selectedPropertyURI.set('');
                },
                error: () => this.propertyErrorKey.set('integration.googleSearchConsole.errors.unmapProperty')
            });
    }

    protected requestSync(): void {
        const siteID = this.activeSite()?.id;
        if (!siteID || !this.mapping()?.mapped || !this.canManageMapping()) {
            return;
        }

        this.syncActionBusy.set(true);
        this.propertyErrorKey.set(null);
        this.syncActionErrorKey.set(null);
        this.notice.set(null);
        this.integration
            .requestSync(siteID)
            .pipe(
                finalize(() => this.syncActionBusy.set(false)),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: (mapping) => {
                    if (!this.isCurrentSiteResponse(siteID, mapping.site_id)) {
                        return;
                    }
                    this.mapping.set(mapping);
                    this.notice.set({ kind: 'success', key: 'integration.googleSearchConsole.states.syncRequested' });
                },
                error: () => this.syncActionErrorKey.set('integration.googleSearchConsole.errors.sync')
            });
    }

    private isCurrentSiteResponse(requestedSiteID: string, responseSiteID: string): boolean {
        return this.activeSite()?.id === requestedSiteID && responseSiteID === requestedSiteID;
    }

    private formatDateOnly(value: string | null | undefined): string {
        const raw = value?.trim();
        if (!raw) {
            return '';
        }
        this.activeLanguage();
        const date = new Date(`${raw}T00:00:00Z`);
        if (Number.isNaN(date.getTime())) {
            return raw;
        }
        return new Intl.DateTimeFormat(this.transloco.getActiveLang(), { dateStyle: 'medium', timeZone: 'UTC' }).format(date);
    }
}

function googleSearchConsolePropertyErrorKey(error: unknown): string {
    if (error instanceof HttpErrorResponse && error.error && typeof error.error === 'object' && 'code' in error.error) {
        const code = String(error.error.code || '').trim();
        if (code === 'api_disabled') {
            return 'integration.googleSearchConsole.errors.apiDisabled';
        }
        if (code === 'authorization_revoked' || code === 'token_refresh_failed') {
            return 'integration.googleSearchConsole.errors.reconnect';
        }
        if (code === 'property_access_lost') {
            return 'integration.googleSearchConsole.errors.propertyAccess';
        }
    }
    return 'integration.googleSearchConsole.errors.properties';
}
