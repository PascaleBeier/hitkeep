import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, OnInit, signal } from '@angular/core';
import { takeUntilDestroyed, toSignal } from '@angular/core/rxjs-interop';

import { DecimalPipe } from '@angular/common';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { ConfirmationService } from 'primeng/api';
import { ConfirmPopupModule } from 'primeng/confirmpopup';
import { TableModule } from 'primeng/table';
import { ButtonModule } from 'primeng/button';
import { SelectModule } from 'primeng/select';
import { TabsModule } from 'primeng/tabs';
import { IconFieldModule } from 'primeng/iconfield';
import { InputIconModule } from 'primeng/inputicon';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';
import { TagModule } from 'primeng/tag';
import { TooltipModule } from 'primeng/tooltip';
import { HttpClient } from '@angular/common/http';
import { HttpErrorResponse } from '@angular/common/http';
import { ActivatedRoute, Router } from '@angular/router';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { formatDurationInterval } from '@core/i18n/duration-format';
import { PermissionService } from '@services/permission.service';
import { TeamService } from '@services/team.service';
import { UserProfileService } from '@services/user-profile.service';
import { AdminPageFrame } from './components/admin-page-frame';
import { AdminGlobalExclusionSettings } from './components/admin-global-exclusion-settings';
import { SystemStatusCard } from './components/system-status-card';
import { SystemAudit } from './components/system-audit';
import {
    ActivationStatus,
    AdminSystemService,
    SystemActivationResponse,
    SystemActivationRow,
    SystemFeatureStatus,
    SystemInfo,
    SystemHealth,
    SystemStorage,
    SystemIngestStats,
    SystemBackupStatus,
    SystemSpamStatus,
    SystemImportStageCleanupStatus,
    SystemCacheStatus,
    SystemMailStatus
} from '@services/admin-system.service';
import { formatBytes } from '@pages/ai-visibility/ai-visibility.utils';
import { finalize } from 'rxjs';

type InstanceRole = 'owner' | 'admin' | 'user';
type AdminStatusTab = 'runtime' | 'operations' | 'activation' | 'audit';
type AdminSettingsTab = 'users' | 'sites' | 'teams' | 'globalFilters';

interface User {
    id: string;
    email: string;
    instance_role: InstanceRole;
    created_at: string;
}

interface Site {
    id: string;
    domain: string;
    user_id: string;
    owner_email?: string;
    created_at: string;
}

interface DeleteUserBlockedResponse {
    status: string;
    code: string;
    message: string;
    teams: {
        id: string;
        name: string;
    }[];
}

interface DeleteUserBlockState {
    email: string;
    teams: string[];
}

interface DisableUserMFAResponse {
    status: string;
    totp_disabled: boolean;
    passkeys_deleted: number;
    sessions_invalidated: number;
}

interface AdminTeam {
    id: string;
    name: string;
    is_default: boolean;
    is_archived: boolean;
    member_count: number;
    site_count: number;
    created_at: string;
}

interface StatusState {
    severity: 'success' | 'error';
    key: string;
    params?: Record<string, string | number>;
}

@Component({
    selector: 'app-admin-settings',
    imports: [
        DecimalPipe,
        ReactiveFormsModule,
        ConfirmPopupModule,
        TableModule,
        ButtonModule,
        SelectModule,
        TabsModule,
        IconFieldModule,
        InputIconModule,
        InputTextModule,
        MessageModule,
        TagModule,
        TooltipModule,
        AdminPageFrame,
        AdminGlobalExclusionSettings,
        SystemStatusCard,
        SystemAudit,
        RelativeDateTime,
        TranslocoPipe
    ],
    templateUrl: './admin-settings.html',
    styleUrl: './admin-settings.css',
    changeDetection: ChangeDetectionStrategy.OnPush,
    providers: [ConfirmationService]
})
export class AdminSettings implements OnInit {
    private http = inject(HttpClient);
    private confirmationService = inject(ConfirmationService);
    private transloco = inject(TranslocoService);
    private route = inject(ActivatedRoute);
    private router = inject(Router);
    private destroyRef = inject(DestroyRef);
    private profile = inject(UserProfileService);
    protected perms = inject(PermissionService);
    private userTeamService = inject(TeamService);
    private system = inject(AdminSystemService);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    protected activeAdminTab = signal<AdminStatusTab>('runtime');
    protected activeSettingsTab = signal<AdminSettingsTab>('users');
    private routeData = toSignal(this.route.data, { initialValue: this.route.snapshot.data });
    private loadedRuntime = signal(false);
    private loadedOperations = signal(false);
    private loadedActivation = signal(false);
    private loadedSettings = signal(false);

    // System console data
    protected systemInfo = signal<SystemInfo | null>(null);
    protected systemHealth = signal<SystemHealth | null>(null);
    protected systemStorage = signal<SystemStorage | null>(null);
    protected systemIngest = signal<SystemIngestStats | null>(null);
    protected systemBackups = signal<SystemBackupStatus | null>(null);
    protected systemSpam = signal<SystemSpamStatus | null>(null);
    protected systemImportCleanup = signal<SystemImportStageCleanupStatus | null>(null);
    protected systemCaches = signal<SystemCacheStatus | null>(null);
    protected systemMail = signal<SystemMailStatus | null>(null);
    protected systemActivation = signal<SystemActivationResponse | null>(null);

    protected isLoadingSystem = signal(false);
    protected isLoadingHealth = signal(false);
    protected isLoadingStorage = signal(false);
    protected isLoadingIngest = signal(false);
    protected isLoadingBackups = signal(false);
    protected isLoadingSpam = signal(false);
    protected isLoadingImportCleanup = signal(false);
    protected isLoadingCaches = signal(false);
    protected isLoadingMail = signal(false);
    protected isLoadingActivation = signal(false);
    protected isRefreshingSpam = signal(false);
    protected isRunningImportCleanup = signal(false);
    protected isTestingMail = signal(false);
    protected activationStatusFilter = signal('');
    protected activationTeamFilter = signal('');
    protected activationDomainFilter = signal('');
    protected activationCopyStatus = signal<'idle' | 'success' | 'error'>('idle');
    protected openingActivationTeamId = signal('');
    protected activationStatusControl = new FormControl<ActivationStatus | ''>('', { nonNullable: true });
    private activationCopyResetTimer: ReturnType<typeof setTimeout> | null = null;
    private activationRequestID = 0;

    protected spamActionStatus = signal<StatusState | null>(null);
    protected importCleanupActionStatus = signal<StatusState | null>(null);
    protected mailTestResult = signal<{ severity: 'success' | 'error'; message: string } | null>(null);
    protected mailTestRecipient = new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.email, Validators.maxLength(320)] });

    protected readonly cacheRows = computed(() => {
        this.activeLanguage();
        const caches = this.systemCaches();
        if (!caches) return [];
        return [
            {
                name: this.transloco.translate('admin.system.caches.permissions'),
                size: caches.permissions_cache.size,
                maxSize: caches.permissions_cache.max_size,
                ttl: caches.permissions_cache.ttl,
                pressure: this.cachePressure(caches.permissions_cache.size, caches.permissions_cache.max_size)
            },
            {
                name: this.transloco.translate('admin.system.caches.apiClients'),
                size: caches.api_client_cache.size,
                maxSize: caches.api_client_cache.max_size,
                ttl: caches.api_client_cache.ttl,
                pressure: this.cachePressure(caches.api_client_cache.size, caches.api_client_cache.max_size)
            },
            {
                name: this.transloco.translate('admin.system.caches.rateLimiter'),
                size: caches.rate_limiter_cache.size,
                maxSize: caches.rate_limiter_cache.max_size,
                ttl: caches.rate_limiter_cache.ttl,
                pressure: this.cachePressure(caches.rate_limiter_cache.size, caches.rate_limiter_cache.max_size)
            }
        ];
    });
    protected readonly enabledFeatureRows = computed(() => {
        this.activeLanguage();
        const info = this.systemInfo();
        if (!info) return [];

        return (info.enabled_features ?? []).map((feature) => ({
            label: this.featureLabel(feature),
            detail: this.featureDetail(feature),
            value: this.transloco.translate(feature.enabled ? 'common.enabled' : 'common.disabled'),
            enabled: feature.enabled
        }));
    });
    protected readonly isManagedCloud = computed(() => {
        const features = this.systemInfo()?.enabled_features ?? [];
        return features.some((feature) => feature.key === 'managed_cloud' && feature.enabled);
    });
    protected readonly instanceOwners = computed(() => this.users().filter((user) => user.instance_role === 'owner').length);
    protected readonly instanceAdmins = computed(() => this.users().filter((user) => user.instance_role === 'admin').length);
    protected readonly activeTeams = computed(() => this.teams().filter((team) => !team.is_archived).length);
    protected readonly totalSites = computed(() => this.sites().length);
    protected readonly totalStorageBytes = computed(() => {
        const storage = this.systemStorage();
        if (!storage) return 0;
        return storage.shared_db_bytes + (storage.tenant_dbs ?? []).reduce((total, db) => total + db.bytes, 0);
    });
    protected readonly diskUsedPercent = computed(() => {
        const storage = this.systemStorage();
        if (!storage || storage.disk_total_bytes <= 0) return 0;
        const used = Math.max(storage.disk_total_bytes - storage.disk_available_bytes, 0);
        return Math.round((used / storage.disk_total_bytes) * 100);
    });
    protected readonly recentHits = computed(() => this.systemIngest()?.recent_hits ?? 0);
    protected readonly importCleanupActionDisabled = computed(() => {
        const cleanup = this.systemImportCleanup();
        return !cleanup?.enabled || cleanup.stale_files === 0;
    });
    protected readonly activationRows = computed(() => this.systemActivation()?.rows ?? []);
    protected readonly activationLiveSites = computed(() => this.activationRows().filter((row) => row.status === 'live').length);
    protected readonly activationStatusOptions = computed(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('admin.system.activation.filters.anyStatus'), value: '' },
            { label: this.transloco.translate('admin.system.activation.status.waiting'), value: 'waiting' },
            { label: this.transloco.translate('admin.system.activation.status.live'), value: 'live' },
            { label: this.transloco.translate('admin.system.activation.status.dormant'), value: 'dormant' },
            { label: this.transloco.translate('admin.system.activation.status.domain_mismatch'), value: 'domain_mismatch' }
        ];
    });
    protected readonly runtimeStatusKey = computed(() => {
        const health = this.systemHealth();
        if (this.isLoadingHealth() && !health) {
            return 'admin.system.console.status.loading';
        }
        if (!health) {
            return 'admin.system.console.status.unknown';
        }
        if (health.status === 'healthy' && health.database === 'ok' && health.workers === 'ok') {
            return 'admin.system.console.status.healthy';
        }
        return 'admin.system.console.status.attention';
    });
    protected readonly runtimeStatusSeverity = computed(() => {
        const health = this.systemHealth();
        if (!health) return 'secondary';
        return health.status === 'healthy' && health.database === 'ok' && health.workers === 'ok' ? 'success' : 'warn';
    });
    protected readonly pageTitleKey = computed(() => (this.routeData()['adminPage'] === 'settings' ? 'nav.systemSettings' : 'nav.systemStatus'));
    protected readonly isSettingsPage = computed(() => this.routeData()['adminPage'] === 'settings');

    protected users = signal<User[]>([]);
    protected sites = signal<Site[]>([]);
    protected teams = signal<AdminTeam[]>([]);
    protected isLoading = signal(false);
    protected isLoadingSites = signal(false);
    protected isLoadingTeams = signal(false);
    protected disablingUserId = signal('');
    protected deletingUserId = signal('');
    protected deletingSiteId = signal('');
    protected deletingTeamId = signal('');
    protected currentUserId = signal<string>('');
    protected roleControls = signal<Record<string, FormControl<InstanceRole>>>({});
    protected deleteUserBlock = signal<DeleteUserBlockState | null>(null);
    protected userMfaStatus = signal<StatusState | null>(null);
    protected userActionStatus = signal<StatusState | null>(null);
    protected siteActionStatus = signal<StatusState | null>(null);
    protected teamActionStatus = signal<StatusState | null>(null);
    protected readonly usersByID = computed(() => new Map(this.users().map((user) => [user.id, user] as const)));
    protected readonly deleteUserBlockMessage = computed(() => {
        const block = this.deleteUserBlock();
        if (!block) {
            return '';
        }

        return this.transloco.translate('admin.errors.deleteUserBlockedOwnership', {
            email: block.email,
            teams: block.teams.join(', ')
        });
    });
    protected readonly userMfaStatusMessage = computed(() => {
        return this.actionStatusMessage(this.userMfaStatus());
    });
    protected readonly spamActionStatusMessage = computed(() => this.actionStatusMessage(this.spamActionStatus()));
    protected readonly importCleanupActionStatusMessage = computed(() => this.actionStatusMessage(this.importCleanupActionStatus()));
    protected readonly userActionStatusMessage = computed(() => this.actionStatusMessage(this.userActionStatus()));
    protected readonly siteActionStatusMessage = computed(() => this.actionStatusMessage(this.siteActionStatus()));
    protected readonly teamActionStatusMessage = computed(() => this.actionStatusMessage(this.teamActionStatus()));
    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        return [{ label: this.transloco.translate('nav.administration') }, { label: this.transloco.translate(this.pageTitleKey()), isCurrent: true }];
    });
    protected readonly canDisableUserMfa = computed(() => this.perms.isInstanceOwner() || this.usersByID().get(this.currentUserId())?.instance_role === 'owner');

    protected roleOptions = computed(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('admin.roles.instanceOwner'), value: 'owner' },
            { label: this.transloco.translate('admin.roles.instanceAdmin'), value: 'admin' },
            { label: this.transloco.translate('admin.roles.user'), value: 'user' }
        ];
    });

    protected readonly localeTag = computed(() => {
        const lang = this.activeLanguage();
        switch (lang) {
            case 'de':
                return 'de-DE';
            case 'es':
                return 'es-ES';
            case 'fr':
                return 'fr-FR';
            case 'it':
                return 'it-IT';
            default:
                return 'en-US';
        }
    });

    constructor() {
        this.activationStatusControl.valueChanges.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((value) => {
            this.activationStatusFilter.set(value);
        });

        this.destroyRef.onDestroy(() => {
            if (this.activationCopyResetTimer) {
                clearTimeout(this.activationCopyResetTimer);
            }
        });

        effect(() => {
            const profile = this.profile.profile();
            this.currentUserId.set(profile?.id ?? '');
            if (profile?.email && (this.mailTestRecipient.pristine || !this.mailTestRecipient.value)) {
                this.mailTestRecipient.setValue(profile.email, { emitEvent: false });
            }
        });

        effect(() => {
            const currentId = this.currentUserId();
            const users = this.users();
            const controls = this.roleControls();

            for (const user of users) {
                const control = controls[user.id];
                if (!control) continue;

                const shouldDisable = user.id === currentId;
                if (shouldDisable && control.enabled) {
                    control.disable({ emitEvent: false });
                } else if (!shouldDisable && control.disabled) {
                    control.enable({ emitEvent: false });
                }
            }
        });

        effect(() => {
            if (this.isSettingsPage()) {
                this.activeSettingsTab.set('users');
                if (!this.loadedSettings()) {
                    this.loadedSettings.set(true);
                    this.loadUsers();
                    this.loadSites();
                    this.loadTeams();
                }
                return;
            }

            switch (this.activeAdminTab()) {
                case 'runtime':
                    if (!this.loadedRuntime()) {
                        this.loadedRuntime.set(true);
                        this.refreshRuntime();
                    }
                    break;
                case 'operations':
                    if (!this.loadedOperations()) {
                        this.loadedOperations.set(true);
                        this.refreshOperations();
                    }
                    break;
                case 'activation':
                    if (this.perms.isInstanceOwner() && !this.loadedActivation()) {
                        this.loadedActivation.set(true);
                        this.loadSystemActivation();
                    }
                    break;
            }
        });
    }

    ngOnInit() {
        if (!this.profile.profile()) {
            this.profile
                .loadProfile()
                .pipe(takeUntilDestroyed(this.destroyRef))
                .subscribe({ error: (err) => console.error('Failed to load profile', err) });
        }
    }

    protected loadSystemInfo() {
        this.isLoadingSystem.set(true);
        this.system.getSystem().subscribe({
            next: (info) => {
                this.systemInfo.set(info);
                this.isLoadingSystem.set(false);
            },
            error: () => this.isLoadingSystem.set(false)
        });
    }

    protected loadSystemHealth() {
        this.isLoadingHealth.set(true);
        this.system.getHealth().subscribe({
            next: (h) => {
                this.systemHealth.set(h);
                this.isLoadingHealth.set(false);
            },
            error: () => this.isLoadingHealth.set(false)
        });
    }

    protected loadSystemStorage() {
        this.isLoadingStorage.set(true);
        this.system.getStorage().subscribe({
            next: (s) => {
                this.systemStorage.set(s);
                this.isLoadingStorage.set(false);
            },
            error: () => this.isLoadingStorage.set(false)
        });
    }

    protected loadSystemIngest() {
        this.isLoadingIngest.set(true);
        this.system
            .getIngestStats()
            .pipe(finalize(() => this.isLoadingIngest.set(false)))
            .subscribe({
                next: (s) => this.systemIngest.set(s)
            });
    }

    protected loadSystemBackups() {
        this.isLoadingBackups.set(true);
        this.system
            .getBackups()
            .pipe(finalize(() => this.isLoadingBackups.set(false)))
            .subscribe({
                next: (b) => this.systemBackups.set(b)
            });
    }

    protected loadSystemSpam() {
        this.isLoadingSpam.set(true);
        this.system
            .getSpamFilter()
            .pipe(finalize(() => this.isLoadingSpam.set(false)))
            .subscribe({
                next: (s) => this.systemSpam.set(s)
            });
    }

    protected loadImportStageCleanup() {
        this.isLoadingImportCleanup.set(true);
        this.system
            .getImportStageCleanup()
            .pipe(finalize(() => this.isLoadingImportCleanup.set(false)))
            .subscribe({
                next: (s) => this.systemImportCleanup.set(s)
            });
    }

    protected loadSystemCaches() {
        this.isLoadingCaches.set(true);
        this.system
            .getCaches()
            .pipe(finalize(() => this.isLoadingCaches.set(false)))
            .subscribe({
                next: (c) => this.systemCaches.set(c)
            });
    }

    protected loadSystemMail() {
        this.isLoadingMail.set(true);
        this.system
            .getMail()
            .pipe(finalize(() => this.isLoadingMail.set(false)))
            .subscribe({
                next: (m) => this.systemMail.set(m)
            });
    }

    protected refreshSpamFilter() {
        this.isRefreshingSpam.set(true);
        this.spamActionStatus.set(null);
        this.system
            .refreshSpamFilter()
            .pipe(finalize(() => this.isRefreshingSpam.set(false)))
            .subscribe({
                next: () => {
                    this.spamActionStatus.set({
                        severity: 'success',
                        key: 'admin.system.spam.refreshTriggered'
                    });
                    this.loadSystemSpam();
                },
                error: () => {
                    this.spamActionStatus.set({
                        severity: 'error',
                        key: 'admin.system.spam.refreshFailed'
                    });
                }
            });
    }

    protected runImportStageCleanup() {
        this.isRunningImportCleanup.set(true);
        this.importCleanupActionStatus.set(null);
        this.system
            .runImportStageCleanup()
            .pipe(finalize(() => this.isRunningImportCleanup.set(false)))
            .subscribe({
                next: (res) => {
                    this.importCleanupActionStatus.set({
                        severity: 'success',
                        key: 'admin.system.importCleanup.runSuccess',
                        params: {
                            files: res.result.files_cleaned,
                            bytes: this.formatBytesValue(res.result.bytes_cleaned)
                        }
                    });
                    this.loadImportStageCleanup();
                },
                error: () => {
                    this.importCleanupActionStatus.set({
                        severity: 'error',
                        key: 'admin.system.importCleanup.runFailed'
                    });
                }
            });
    }

    protected testMail() {
        this.mailTestRecipient.markAsTouched();
        if (this.mailTestRecipient.invalid) {
            this.mailTestResult.set({ severity: 'error', message: this.transloco.translate('admin.system.mail.recipientInvalid') });
            return;
        }

        const recipient = this.mailTestRecipient.value.trim();
        this.isTestingMail.set(true);
        this.mailTestResult.set(null);
        this.system
            .testMail(recipient)
            .pipe(finalize(() => this.isTestingMail.set(false)))
            .subscribe({
                next: (res) => {
                    this.mailTestResult.set({ severity: 'success', message: res.message });
                    this.loadSystemMail();
                },
                error: (err) => {
                    this.mailTestResult.set({ severity: 'error', message: err.error?.message || this.transloco.translate('admin.system.mail.testFailed') });
                }
            });
    }

    protected refreshRuntime() {
        this.loadSystemInfo();
        this.loadSystemHealth();
        this.loadSystemStorage();
        this.loadSystemIngest();
    }

    protected refreshOperations() {
        this.loadSystemBackups();
        this.loadSystemSpam();
        this.loadImportStageCleanup();
        this.loadSystemCaches();
        this.loadSystemMail();
    }

    protected loadSystemActivation(offset = 0) {
        if (!this.perms.isInstanceOwner()) return;
        const requestID = ++this.activationRequestID;
        this.isLoadingActivation.set(true);
        this.system
            .getActivation({
                status: this.activationStatusFilter(),
                team: this.activationTeamFilter(),
                domain: this.activationDomainFilter(),
                limit: this.systemActivation()?.limit ?? 50,
                offset
            })
            .pipe(
                finalize(() => {
                    if (requestID === this.activationRequestID) {
                        this.isLoadingActivation.set(false);
                    }
                }),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: (activation) => {
                    if (requestID === this.activationRequestID) {
                        this.systemActivation.set(activation);
                    }
                },
                error: () => {
                    if (requestID === this.activationRequestID) {
                        this.systemActivation.set({ rows: [], total: 0, limit: 50, offset: 0, has_more: false });
                    }
                }
            });
    }

    protected applyActivationFilters() {
        this.loadSystemActivation(0);
    }

    protected clearActivationFilters() {
        this.activationStatusControl.setValue('', { emitEvent: false });
        this.activationStatusFilter.set('');
        this.activationTeamFilter.set('');
        this.activationDomainFilter.set('');
        this.loadSystemActivation(0);
    }

    protected previousActivationOffset(): number {
        const current = this.systemActivation();
        if (!current) return 0;
        return Math.max(current.offset - current.limit, 0);
    }

    protected filterActivationTeam(row: SystemActivationRow) {
        this.activationTeamFilter.set(row.team_name);
        this.loadSystemActivation(0);
    }

    protected copyActivationContext(row: SystemActivationRow) {
        const lines = [
            `Team: ${row.team_name} (${row.team_id})`,
            `Owner: ${row.owner_email || '-'}`,
            `Site: ${row.site_domain} (${row.site_id})`,
            `Status: ${row.status}`,
            `First hit: ${row.first_hit_at || '-'}`,
            `Last hit: ${row.last_hit_at || '-'}`,
            `Last event: ${row.last_event_at || '-'}${row.last_event_name ? ` (${row.last_event_name})` : ''}`,
            `Hits 24h: ${row.hits_last_24h}`,
            `Hits 7d: ${row.hits_last_7d}`,
            `Events 7d: ${row.events_last_7d}`,
            `Tracker: ${row.tracker_source || '-'}${row.tracker_version ? ` ${row.tracker_version}` : ''}`
        ].join('\n');
        const clipboard = navigator.clipboard;
        if (!clipboard) {
            this.setActivationCopyStatus('error');
            return;
        }
        this.activationCopyStatus.set('idle');
        clipboard
            .writeText(lines)
            .then(() => this.setActivationCopyStatus('success'))
            .catch(() => this.setActivationCopyStatus('error'));
    }

    protected openActivationTeam(row: SystemActivationRow) {
        if (!this.perms.isInstanceOwner() || this.openingActivationTeamId()) return;
        this.openingActivationTeamId.set(row.team_id);
        this.userTeamService
            .setActiveTeam(row.team_id)
            .pipe(
                finalize(() => this.openingActivationTeamId.set('')),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: () => void this.router.navigate(['/admin/team']),
                error: () => undefined
            });
    }

    private setActivationCopyStatus(status: 'success' | 'error') {
        this.activationCopyStatus.set(status);
        if (this.activationCopyResetTimer) {
            clearTimeout(this.activationCopyResetTimer);
        }
        this.activationCopyResetTimer = setTimeout(() => {
            this.activationCopyStatus.set('idle');
            this.activationCopyResetTimer = null;
        }, 3000);
    }

    protected activationStatusLabel(status: ActivationStatus): string {
        this.activeLanguage();
        return this.transloco.translate(`admin.system.activation.status.${status}`);
    }

    protected activationStatusSeverity(status: ActivationStatus): 'success' | 'danger' | 'warn' | 'secondary' | 'info' | 'contrast' {
        switch (status) {
            case 'live':
                return 'success';
            case 'domain_mismatch':
                return 'danger';
            case 'dormant':
                return 'warn';
            default:
                return 'secondary';
        }
    }

    protected formatBytesValue(value: number | null | undefined): string {
        return formatBytes(value ?? 0, this.localeTag());
    }

    protected formatMinutes(minutes: number | null | undefined): string {
        this.activeLanguage();
        if (!minutes || minutes <= 0) {
            return '-';
        }
        return formatDurationInterval(minutes * 60, this.localeTag(), 'short');
    }

    protected statusSeverity(status: string | null | undefined): 'success' | 'danger' | 'warn' | 'secondary' | 'info' | 'contrast' {
        switch ((status ?? '').toLowerCase()) {
            case 'healthy':
            case 'ok':
            case 'success':
            case 'active':
                return 'success';
            case 'failed':
            case 'failure':
            case 'error':
            case 'unhealthy':
                return 'danger';
            case 'warn':
            case 'warning':
            case 'pressure':
                return 'warn';
            default:
                return 'secondary';
        }
    }

    private cachePressure(size: number, maxSize: number): number {
        if (maxSize <= 0) return 0;
        return Math.round((size / maxSize) * 100);
    }

    private featureLabel(feature: SystemFeatureStatus): string {
        const key = `admin.system.features.${feature.key}.label`;
        const label = this.transloco.translate(key);
        return label === key ? feature.key.replaceAll('_', ' ') : label;
    }

    private featureDetail(feature: SystemFeatureStatus): string {
        const detail = feature.detail?.trim();
        if (!detail) {
            return '';
        }

        if (feature.key === 'automatic_backups') {
            const key = `admin.system.featureDetails.backup.${detail}`;
            const label = this.transloco.translate(key);
            return label === key ? detail : label;
        }
        if (feature.key === 'mail_delivery') {
            return this.transloco.translate('admin.system.featureDetails.driver', { driver: detail });
        }
        if (feature.key === 'spam_auto_update') {
            return this.transloco.translate('admin.system.featureDetails.interval', { interval: detail });
        }
        if (feature.key === 'billing' && detail === 'stripe') {
            return this.transloco.translate('admin.system.featureDetails.stripe');
        }
        if (feature.key === 'managed_cloud') {
            return this.transloco.translate('admin.system.featureDetails.plan', { plan: detail });
        }

        return detail;
    }

    loadUsers() {
        this.isLoading.set(true);
        this.http.get<User[]>('/api/admin/users').subscribe({
            next: (users) => {
                this.deleteUserBlock.update((current) => {
                    if (!current) {
                        return null;
                    }
                    const stillExists = users.some((user) => user.email === current.email);
                    return stillExists ? current : null;
                });
                const normalizedUsers = users.map((user) => ({
                    ...user,
                    instance_role: this.normalizeInstanceRole(user.instance_role)
                }));
                this.users.set(normalizedUsers);
                this.roleControls.set(
                    normalizedUsers.reduce<Record<string, FormControl<InstanceRole>>>((controls, user) => {
                        controls[user.id] = new FormControl<InstanceRole>(
                            {
                                value: user.instance_role,
                                disabled: user.id === this.currentUserId()
                            },
                            { nonNullable: true }
                        );
                        return controls;
                    }, {})
                );
                this.isLoading.set(false);
            },
            error: (err) => {
                console.error('Failed to load users', err);
                this.isLoading.set(false);
            }
        });
    }

    loadSites() {
        this.isLoadingSites.set(true);
        this.http.get<Site[]>('/api/admin/sites').subscribe({
            next: (sites) => {
                this.sites.set(
                    sites.map((site) => ({
                        ...site,
                        owner_email: (site.owner_email ?? '').trim()
                    }))
                );
                this.isLoadingSites.set(false);
            },
            error: (err) => {
                console.error('Failed to load sites', err);
                this.isLoadingSites.set(false);
            }
        });
    }

    private updateUserRole(user: User, nextRole: InstanceRole, previousRole: InstanceRole): void {
        this.http
            .post(`/api/admin/users/${user.id}/role`, {
                role: nextRole
            })
            .subscribe({
                next: () => this.roleControl(user.id).setValue(nextRole, { emitEvent: false }),
                error: (err) => {
                    user.instance_role = previousRole;
                    this.roleControl(user.id).setValue(previousRole, { emitEvent: false });
                    console.error('Failed to update role', err);
                }
            });
    }

    protected roleControl(userId: string): FormControl<InstanceRole> {
        const existing = this.roleControls()[userId];
        if (existing) {
            return existing;
        }

        const fallback = new FormControl<InstanceRole>('user', { nonNullable: true });
        this.roleControls.update((controls) => ({ ...controls, [userId]: fallback }));
        return fallback;
    }

    protected onRoleChange(user: User, role: InstanceRole | null | undefined): void {
        if (!role || role === user.instance_role) {
            return;
        }

        const previousRole = user.instance_role;
        user.instance_role = role;
        this.updateUserRole(user, role, previousRole);
    }

    protected isInstanceOwner(user: User): boolean {
        return user.instance_role === 'owner';
    }

    protected isDisablingUser(user: User): boolean {
        return this.disablingUserId() === user.id;
    }

    protected isDeletingUser(user: User): boolean {
        return this.deletingUserId() === user.id;
    }

    protected isDeletingSite(site: Site): boolean {
        return this.deletingSiteId() === site.id;
    }

    protected isDeletingTeam(team: AdminTeam): boolean {
        return this.deletingTeamId() === team.id;
    }

    protected siteOwnerEmail(site: Site): string {
        return site.owner_email || this.usersByID().get(site.user_id)?.email || this.transloco.translate('admin.sites.ownerUnknown');
    }

    protected siteOwnerInstanceRole(site: Site): InstanceRole | null {
        return this.usersByID().get(site.user_id)?.instance_role ?? null;
    }

    protected roleLabel(role: InstanceRole): string {
        return this.roleOptions().find((entry) => entry.value === role)?.label ?? role;
    }

    private normalizeInstanceRole(role: string | null | undefined): InstanceRole {
        if (role === 'owner' || role === 'admin' || role === 'user') {
            return role;
        }
        return 'user';
    }

    protected confirmDisableUserMfa(event: Event, user: User): void {
        const target = event.currentTarget;
        if (!(target instanceof HTMLElement) || !this.canDisableUserMfa() || this.isDisablingUser(user)) {
            return;
        }

        this.confirmationService.confirm({
            key: 'admin-disable-mfa',
            target,
            message: this.transloco.translate('admin.confirmDisable2fa', { email: user.email }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: {
                label: this.transloco.translate('common.actions.cancel'),
                severity: 'secondary',
                outlined: true
            },
            acceptButtonProps: {
                label: this.transloco.translate('admin.users.disable2faAction'),
                severity: 'warn'
            },
            accept: () => {
                this.userMfaStatus.set(null);
                this.disablingUserId.set(user.id);
                this.http
                    .post<DisableUserMFAResponse>(`/api/admin/users/${user.id}/disable-2fa`, {})
                    .pipe(finalize(() => this.disablingUserId.set('')))
                    .subscribe({
                        next: () => {
                            this.userMfaStatus.set({
                                severity: 'success',
                                key: 'admin.status.disable2faSuccess',
                                params: { email: user.email }
                            });
                        },
                        error: () => {
                            this.userMfaStatus.set({
                                severity: 'error',
                                key: 'admin.errors.disable2faFailed',
                                params: { email: user.email }
                            });
                        }
                    });
            }
        });
    }

    confirmDeleteUser(event: Event, user: User) {
        const target = event.currentTarget;
        if (!(target instanceof HTMLElement) || user.id === this.currentUserId() || this.isDeletingUser(user)) {
            return;
        }

        this.confirmationService.confirm({
            key: 'admin-delete',
            target,
            message: this.transloco.translate('admin.confirmDeleteUser', { email: user.email }),
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
            accept: () => {
                this.deleteUserBlock.set(null);
                this.userMfaStatus.set(null);
                this.userActionStatus.set(null);
                this.deletingUserId.set(user.id);
                this.http
                    .delete(`/api/admin/users/${user.id}?force=true`)
                    .pipe(finalize(() => this.deletingUserId.set('')))
                    .subscribe({
                        next: () => {
                            this.userActionStatus.set({
                                severity: 'success',
                                key: 'admin.status.deleteUserSuccess',
                                params: { email: user.email }
                            });
                            this.loadUsers();
                        },
                        error: (err) => {
                            if (this.handleDeleteUserError(err, user)) {
                                return;
                            }
                            this.userActionStatus.set({
                                severity: 'error',
                                key: this.resolveDeleteErrorKey(err, 'admin.errors.deleteUserFailed'),
                                params: { email: user.email }
                            });
                            console.error('Failed to delete user', err);
                        }
                    });
            }
        });
    }

    confirmDeleteSite(event: Event, site: Site) {
        const target = event.currentTarget;
        if (!(target instanceof HTMLElement) || this.isDeletingSite(site)) {
            return;
        }

        this.confirmationService.confirm({
            key: 'admin-delete',
            target,
            message: this.transloco.translate('admin.confirmDeleteSite', { domain: site.domain }),
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
            accept: () => {
                this.siteActionStatus.set(null);
                this.deletingSiteId.set(site.id);
                this.http
                    .delete(`/api/admin/sites/${site.id}`)
                    .pipe(finalize(() => this.deletingSiteId.set('')))
                    .subscribe({
                        next: () => {
                            this.siteActionStatus.set({
                                severity: 'success',
                                key: 'admin.status.deleteSiteSuccess',
                                params: { domain: site.domain }
                            });
                            this.loadSites();
                        },
                        error: (err) => {
                            this.siteActionStatus.set({
                                severity: 'error',
                                key: this.resolveDeleteErrorKey(err, 'admin.errors.deleteSiteFailed'),
                                params: { domain: site.domain }
                            });
                            console.error('Failed to delete site', err);
                        }
                    });
            }
        });
    }

    loadTeams() {
        this.isLoadingTeams.set(true);
        this.http.get<AdminTeam[]>('/api/admin/teams').subscribe({
            next: (teams) => {
                this.teams.set(teams);
                this.isLoadingTeams.set(false);
            },
            error: (err) => {
                console.error('Failed to load teams', err);
                this.isLoadingTeams.set(false);
            }
        });
    }

    confirmDeleteTeam(event: Event, team: AdminTeam) {
        const target = event.currentTarget;
        if (!(target instanceof HTMLElement) || this.isDeletingTeam(team)) {
            return;
        }

        const messageKey = team.site_count > 0 ? 'admin.confirmDeleteTeamWithSites' : 'admin.confirmDeleteTeam';

        this.confirmationService.confirm({
            key: 'admin-delete',
            target,
            message: this.transloco.translate(messageKey, { name: team.name, sites: team.site_count }),
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
            accept: () => {
                this.teamActionStatus.set(null);
                this.deletingTeamId.set(team.id);
                this.http
                    .delete(`/api/admin/teams/${team.id}?force=true`)
                    .pipe(finalize(() => this.deletingTeamId.set('')))
                    .subscribe({
                        next: () => {
                            this.teamActionStatus.set({
                                severity: 'success',
                                key: 'admin.status.deleteTeamSuccess',
                                params: { name: team.name }
                            });
                            this.loadTeams();
                            this.loadSites();
                        },
                        error: (err) => {
                            this.teamActionStatus.set({
                                severity: 'error',
                                key: this.resolveDeleteErrorKey(err, 'admin.errors.deleteTeamFailed'),
                                params: { name: team.name }
                            });
                            console.error('Failed to delete team', err);
                        }
                    });
            }
        });
    }

    private actionStatusMessage(state: StatusState | null): string {
        this.activeLanguage();
        if (!state) {
            return '';
        }

        return this.transloco.translate(state.key, state.params);
    }

    private handleDeleteUserError(err: unknown, user: User): boolean {
        const httpErr = err instanceof HttpErrorResponse ? err : null;
        const response = httpErr?.error as DeleteUserBlockedResponse | undefined;
        if (!response || response.code !== 'user_owns_teams' || !Array.isArray(response.teams)) {
            return false;
        }

        const teamNames = response.teams.map((team) => team?.name?.trim()).filter((name): name is string => !!name);

        if (teamNames.length === 0) {
            return false;
        }

        this.deleteUserBlock.set({
            email: user.email,
            teams: teamNames
        });
        return true;
    }

    private resolveDeleteErrorKey(err: unknown, fallbackKey: string): string {
        if (!(err instanceof HttpErrorResponse)) {
            return fallbackKey;
        }

        const detail = this.deleteErrorDetail(err);

        if (err.status === 403) {
            return 'admin.errors.deleteForbidden';
        }
        if (err.status === 404) {
            return 'admin.errors.deleteNotFound';
        }
        if (err.status === 503) {
            return 'admin.errors.deleteUnavailable';
        }
        if (detail.includes('default team')) {
            return 'admin.errors.deleteDefaultTeam';
        }
        if (detail.includes('archive the team')) {
            return 'admin.errors.deleteTeamNotArchived';
        }
        if (detail.includes('transfer or delete all sites')) {
            return 'admin.errors.deleteTeamHasSites';
        }

        return fallbackKey;
    }

    private deleteErrorDetail(err: HttpErrorResponse): string {
        if (typeof err.error === 'string') {
            return err.error.toLowerCase();
        }
        if (err.error && typeof err.error === 'object') {
            const body = err.error as Record<string, unknown>;
            return [body['message'], body['error'], body['code']]
                .filter((value): value is string => typeof value === 'string')
                .join(' ')
                .toLowerCase();
        }
        return '';
    }
}
