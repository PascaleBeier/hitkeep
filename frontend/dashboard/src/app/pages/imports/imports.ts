import { DatePipe, DecimalPipe } from '@angular/common';
import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, signal, viewChild } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { firstValueFrom } from 'rxjs';
import { ButtonModule } from 'primeng/button';
import { CardModule } from 'primeng/card';
import { FileUpload, FileUploadHandlerEvent, FileUploadModule, FileRemoveEvent, FileSelectEvent } from 'primeng/fileupload';
import { MessageModule } from 'primeng/message';
import { ProgressBarModule } from 'primeng/progressbar';
import { TagModule } from 'primeng/tag';
import { TableModule } from 'primeng/table';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { SiteService } from '@features/sites/services/site.service';
import { PermissionService } from '@services/permission.service';
import { ImportJob, ImportManifest, ImportProviderDescriptor, ImportsService } from '@services/imports.service';
import { injectActiveLang } from '@core/i18n/active-lang';

type ImportStep = 'driver' | 'files' | 'review' | 'import' | 'complete';

const importGuideURLs: Record<string, string> = {
    plausible: 'https://hitkeep.com/guides/data/import-plausible/',
    simpleanalytics: 'https://hitkeep.com/guides/data/import-simple-analytics/'
};

export function importGuideUrl(providerKey: string): string {
    return importGuideURLs[providerKey] ?? '';
}

export function acceptsImportFileExtension(provider: ImportProviderDescriptor | null, filename: string): boolean {
    if (!provider) return false;
    const name = filename.trim().toLowerCase();
    const extensionIndex = name.lastIndexOf('.');
    const extension = extensionIndex >= 0 ? name.slice(extensionIndex) : '';
    return provider.accepted_extensions.includes(extension);
}

export function importManifestHasDatasetEvents(manifest: ImportManifest | null | undefined): boolean {
    return Array.isArray(manifest?.datasets) && manifest.datasets.some((dataset) => (dataset.events ?? 0) > 0);
}

export function importManifestHasEventCoverage(manifest: ImportManifest | null | undefined): boolean {
    const coverage = manifest?.event_coverage;
    if (!coverage) return false;
    return coverage.rows_scanned > 0 || coverage.rows_accepted > 0 || coverage.events > 0 || listLength(coverage.event_names) > 0 || listLength(coverage.property_keys) > 0;
}

export function importManifestHasEventProperties(manifest: ImportManifest | null | undefined): boolean {
    const coverage = manifest?.event_property_coverage;
    if (!coverage) return false;
    return (
        coverage.attributed_rows > 0 ||
        coverage.unattributed_rows > 0 ||
        coverage.attributed_events > 0 ||
        coverage.unattributed_events > 0 ||
        listLength(coverage.attributed_property_keys) > 0 ||
        listLength(coverage.unattributed_property_keys) > 0 ||
        !!coverage.unavailable_relationship_message
    );
}

export function importManifestHasEventDimensions(manifest: ImportManifest | null | undefined): boolean {
    const coverage = manifest?.event_dimension_coverage;
    if (!coverage) return false;
    return (importManifestHasEventCoverage(manifest) || importManifestHasEventProperties(manifest)) && (listLength(coverage.available) > 0 || listLength(coverage.unavailable) > 0 || !!coverage.reason);
}

function listLength(value: readonly unknown[] | null | undefined): number {
    return Array.isArray(value) ? value.length : 0;
}

function previewList<T>(value: readonly T[] | null | undefined, limit: number): T[] {
    return Array.isArray(value) ? value.slice(0, limit) : [];
}

function safeList<T>(value: readonly T[] | null | undefined): T[] {
    return Array.isArray(value) ? [...value] : [];
}

@Component({
    selector: 'app-imports',
    imports: [DatePipe, DecimalPipe, TranslocoPipe, ButtonModule, CardModule, FileUploadModule, MessageModule, ProgressBarModule, TagModule, TableModule, PageHeader, PageHeaderLeft, PageBreadcrumb],
    templateUrl: './imports.html',
    styleUrl: './imports.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ImportsPage {
    private readonly fileUpload = viewChild<FileUpload>('fileUpload');
    private readonly imports = inject(ImportsService);
    private readonly siteService = inject(SiteService);
    private readonly perms = inject(PermissionService);
    private readonly transloco = inject(TranslocoService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly activeLanguage = injectActiveLang();

    protected readonly activeSite = computed(() => this.siteService.activeSite());
    protected readonly canImport = computed(() => {
        const site = this.activeSite();
        return !!site && this.perms.canManageSite(site.id);
    });
    protected readonly providers = signal<ImportProviderDescriptor[]>([]);
    protected readonly selectedProvider = signal('');
    protected readonly selectedFiles = signal<File[]>([]);
    protected readonly currentJob = signal<ImportJob | null>(null);
    protected readonly history = signal<ImportJob[]>([]);
    protected readonly isBusy = signal(false);
    protected readonly isLoadingProviders = signal(false);
    protected readonly uploadProgress = signal(0);
    protected readonly errorMessage = signal('');

    private pollHandle: ReturnType<typeof setTimeout> | null = null;

    protected readonly step = computed<ImportStep>(() => {
        const job = this.currentJob();
        if (!this.selectedProvider()) return 'driver';
        if (!job) return 'files';
        if (job.status === 'validated') return 'review';
        if (['queued', 'running', 'validating', 'uploading'].includes(job.status)) return 'import';
        return 'complete';
    });
    protected readonly selectedProviderDescriptor = computed(() => this.providers().find((provider) => provider.key === this.selectedProvider()) ?? null);
    protected readonly acceptedFileInput = computed(() => this.selectedProviderDescriptor()?.accepted_extensions.join(',') ?? '');
    protected readonly acceptedFileHint = computed(() => this.acceptedFileInput().replaceAll(',', ', '));
    protected readonly selectedProviderName = computed(() => {
        this.activeLanguage();
        return this.selectedProviderDescriptor()?.name ?? this.transloco.translate('imports.providerFallback');
    });
    protected readonly importGuideUrl = importGuideUrl;

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        const site = this.activeSite();
        if (!site) return [{ label: this.transloco.translate('imports.title'), isCurrent: true }];
        return [
            { label: site.domain, favicon: site, routerLink: '/dashboard' },
            { label: this.transloco.translate('imports.title'), isCurrent: true }
        ];
    });

    protected readonly selectedFileBytes = computed(() => this.selectedFiles().reduce((sum, file) => sum + file.size, 0));
    protected readonly manifest = computed(() => this.currentJob()?.manifest ?? null);
    protected readonly manifestDatasets = computed(() => safeList(this.manifest()?.datasets));
    protected readonly eventNamesPreview = computed(() => previewList(this.manifest()?.event_coverage.event_names, 8));
    protected readonly eventPropertyKeysPreview = computed(() => previewList(this.manifest()?.event_property_coverage.attributed_property_keys, 8));
    protected readonly availableEventDimensionsPreview = computed(() => previewList(this.manifest()?.event_dimension_coverage.available, 10));
    protected readonly unavailableEventDimensionsPreview = computed(() => previewList(this.manifest()?.event_dimension_coverage.unavailable, 10));
    protected readonly showDatasetEventsColumn = computed(() => importManifestHasDatasetEvents(this.manifest()));
    protected readonly showEventCoverage = computed(() => importManifestHasEventCoverage(this.manifest()));
    protected readonly showEventProperties = computed(() => importManifestHasEventProperties(this.manifest()));
    protected readonly showEventDimensions = computed(() => importManifestHasEventDimensions(this.manifest()));
    protected readonly showEventValidationDetails = computed(() => this.showEventCoverage() || this.showEventProperties() || this.showEventDimensions());
    protected readonly hasOverlapEstimate = computed(() => {
        const overlap = this.manifest()?.overlap;
        return !!overlap && (overlap.native_traffic_days > 0 || overlap.native_event_keys > 0 || overlap.estimated_skipped_rows > 0);
    });

    constructor() {
        effect(() => {
            const site = this.activeSite();
            this.stopPolling();
            this.currentJob.set(null);
            this.selectedProvider.set('');
            this.selectedFiles.set([]);
            this.uploadProgress.set(0);
            if (site && this.canImport()) {
                this.loadImporters(site.id);
                this.refreshHistory();
            } else {
                this.providers.set([]);
                this.isLoadingProviders.set(false);
                this.history.set([]);
            }
        });

        this.destroyRef.onDestroy(() => this.stopPolling());
    }

    protected selectProvider(provider: ImportProviderDescriptor) {
        if (this.isBusy() || this.currentJob()) return;
        this.selectedProvider.set(provider.key);
        this.selectedFiles.set([]);
        this.errorMessage.set('');
        this.uploadProgress.set(0);
        this.fileUpload()?.clear();
    }

    protected changeProvider() {
        if (this.isBusy() || this.currentJob()) return;
        this.selectedProvider.set('');
        this.selectedFiles.set([]);
        this.errorMessage.set('');
        this.uploadProgress.set(0);
        this.fileUpload()?.clear();
    }

    protected onFilesSelected(event: FileSelectEvent) {
        this.currentJob.set(null);
        this.selectedFiles.set(event.currentFiles.filter((file) => this.isAcceptedFile(file)));
    }

    protected onFileRemoved(event: FileRemoveEvent) {
        this.selectedFiles.update((files) => files.filter((file) => file !== event.file));
    }

    protected onFilesCleared() {
        this.selectedFiles.set([]);
    }

    protected resetFlow() {
        this.stopPolling();
        this.currentJob.set(null);
        this.selectedProvider.set('');
        this.selectedFiles.set([]);
        this.uploadProgress.set(0);
        this.errorMessage.set('');
        this.fileUpload()?.clear();
    }

    protected async uploadAndValidate(event: FileUploadHandlerEvent) {
        const site = this.activeSite();
        const provider = this.selectedProvider();
        const files = event.files.filter((file) => this.isAcceptedFile(file));
        if (!site || !provider || files.length === 0 || this.isBusy()) return;

        this.isBusy.set(true);
        this.errorMessage.set('');
        this.selectedFiles.set(files);
        this.uploadProgress.set(0);
        try {
            const upload = await firstValueFrom(
                this.imports.createUpload(
                    site.id,
                    provider,
                    files.map((file) => ({ filename: file.name, size_bytes: file.size }))
                )
            );

            let uploadedBytes = 0;
            for (let i = 0; i < files.length; i++) {
                const file = files[i]!;
                const target = upload.files[i]!;
                const chunkSize = upload.chunk_size || 8 * 1024 * 1024;
                for (let offset = 0; offset < file.size; offset += chunkSize) {
                    const chunk = file.slice(offset, Math.min(offset + chunkSize, file.size));
                    await firstValueFrom(this.imports.uploadChunk(site.id, upload.import_id, target.id, offset, chunk));
                    uploadedBytes += chunk.size;
                    this.uploadProgress.set(Math.round((uploadedBytes / this.selectedFileBytes()) * 100));
                }
            }

            const job = await firstValueFrom(this.imports.validate(site.id, upload.import_id));
            this.currentJob.set(job);
            this.fileUpload()?.clear();
            this.refreshHistory();
        } catch (error) {
            this.errorMessage.set(this.describeError(error));
        } finally {
            this.isBusy.set(false);
        }
    }

    protected async startImport() {
        const site = this.activeSite();
        const job = this.currentJob();
        if (!site || !job || this.isBusy()) return;

        this.isBusy.set(true);
        this.errorMessage.set('');
        try {
            const started = await firstValueFrom(this.imports.start(site.id, job.id));
            this.currentJob.set(started);
            this.pollImport(site.id, job.id);
        } catch (error) {
            this.errorMessage.set(this.describeError(error));
        } finally {
            this.isBusy.set(false);
        }
    }

    protected refreshHistory() {
        const site = this.activeSite();
        if (!site) return;
        this.imports.list(site.id).subscribe({
            next: (response) => this.history.set(response.imports),
            error: () => this.errorMessage.set(this.transloco.translate('imports.errors.loadHistory'))
        });
    }

    protected async deleteImport(job: ImportJob) {
        const site = this.activeSite();
        if (!site || ['queued', 'running', 'validating'].includes(job.status)) return;
        this.errorMessage.set('');
        try {
            await firstValueFrom(this.imports.delete(site.id, job.id));
            if (this.currentJob()?.id === job.id) {
                this.resetFlow();
            }
            this.refreshHistory();
        } catch (error) {
            this.errorMessage.set(this.describeError(error));
        }
    }

    protected statusSeverity(status: string): 'success' | 'info' | 'warn' | 'danger' | 'secondary' {
        switch (status) {
            case 'completed':
            case 'validated':
                return 'success';
            case 'queued':
            case 'running':
            case 'validating':
                return 'info';
            case 'failed':
            case 'validation_failed':
                return 'danger';
            case 'deleted':
                return 'secondary';
            default:
                return 'warn';
        }
    }

    protected formatBytes(bytes: number): string {
        if (bytes < 1024) return `${bytes} B`;
        if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
        if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
        return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
    }

    protected isZipFile(file: File): boolean {
        return file.name.toLowerCase().endsWith('.zip');
    }

    protected listLength(value: readonly unknown[] | null | undefined): number {
        return listLength(value);
    }

    private isAcceptedFile(file: File): boolean {
        return acceptsImportFileExtension(this.selectedProviderDescriptor(), file.name);
    }

    private loadImporters(siteId: string) {
        this.isLoadingProviders.set(true);
        this.imports.listImporters(siteId).subscribe({
            next: (providers) => {
                this.providers.set(providers);
                if (this.selectedProvider() && !providers.some((provider) => provider.key === this.selectedProvider())) {
                    this.selectedProvider.set('');
                    this.selectedFiles.set([]);
                    this.fileUpload()?.clear();
                }
                this.isLoadingProviders.set(false);
            },
            error: () => {
                this.isLoadingProviders.set(false);
                this.errorMessage.set(this.transloco.translate('imports.errors.loadImporters'));
            }
        });
    }

    private pollImport(siteId: string, importId: string) {
        this.stopPolling();
        const tick = async () => {
            try {
                const job = await firstValueFrom(this.imports.get(siteId, importId));
                this.currentJob.set(job);
                this.refreshHistory();
                if (['queued', 'running', 'validating'].includes(job.status)) {
                    this.pollHandle = setTimeout(tick, 2000);
                }
            } catch (error) {
                this.errorMessage.set(this.describeError(error));
            }
        };
        this.pollHandle = setTimeout(tick, 1500);
    }

    private stopPolling() {
        if (this.pollHandle) {
            clearTimeout(this.pollHandle);
            this.pollHandle = null;
        }
    }

    private describeError(error: unknown): string {
        if (typeof error === 'object' && error && 'error' in error) {
            const body = (error as { error?: unknown }).error;
            if (typeof body === 'string' && body.trim()) return body.trim();
        }
        return this.transloco.translate('imports.errors.generic');
    }
}
