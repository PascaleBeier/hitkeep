import { Clipboard } from '@angular/cdk/clipboard';
import { ChangeDetectionStrategy, Component, computed, DestroyRef, effect, inject, signal } from '@angular/core';
import { takeUntilDestroyed, toSignal } from '@angular/core/rxjs-interop';
import { AbstractControl, FormControl, FormGroup, FormsModule, ReactiveFormsModule, ValidationErrors, Validators } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ConfirmationService, MenuItem } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { ColorPickerModule } from 'primeng/colorpicker';
import { ConfirmDialogModule } from 'primeng/confirmdialog';
import { DrawerModule } from 'primeng/drawer';
import { FileSelectEvent, FileUploadModule } from 'primeng/fileupload';
import { IconFieldModule } from 'primeng/iconfield';
import { InputIconModule } from 'primeng/inputicon';
import { InputNumberModule } from 'primeng/inputnumber';
import { InputTextModule } from 'primeng/inputtext';
import { SelectModule } from 'primeng/select';
import { SplitButtonModule } from 'primeng/splitbutton';
import { TableModule } from 'primeng/table';
import { TagModule } from 'primeng/tag';
import { TextareaModule } from 'primeng/textarea';
import { map } from 'rxjs';
import { buildTakeoutExportMenuItems, TakeoutExportFormat } from '@core/export/export-formats';
import { injectActiveLang } from '@core/i18n/active-lang';
import { CopyControl } from '@components/copy-control/copy-control';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { DialogShell } from '@components/dialog-shell/dialog-shell';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageState } from '@components/page-state/page-state';
import { ReportRangeToolbar } from '@components/report-range-toolbar/report-range-toolbar';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { TableRowActionItem, TableRowActions } from '@components/table-row-actions/table-row-actions';
import { KpiCard } from '@features/analytics/components/kpi-card';
import { MetricCardGroup, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import { SeriesChart, SeriesChartPoint, SeriesDefinition } from '@features/analytics/components/series-chart';
import { QRCodePreview } from '@features/qr/qr-code-preview';
import { SiteSelectOption } from '@features/sites/components/site-select-option';
import { SiteService } from '@features/sites/services/site.service';
import { MetricStat, QRCode, QRCodeRequest, QRCodeShareLink, QRCodeStyle, QRCodeSummary, Site } from '@models/analytics.types';
import { QRCodesService, buildQRCodeDestination, qrExportFilename } from '@services/qr-codes.service';
import { ShareService } from '@services/share.service';
import { injectReportRange } from '@services/report-range-preferences.service';

type QRMetricGroup = 'pages' | 'referrers' | 'devices' | 'countries';
type QRExportSize = 1024 | 2048 | 4096;

interface ShareNotice {
    kind: 'success' | 'error';
    key: string;
}

interface CustomParamRow {
    id: string;
    key: string;
    value: string;
}

const QR_DOT_OPTIONS = ['square', 'dots', 'rounded', 'extra-rounded', 'classy', 'classy-rounded'] as const;
const QR_CORNER_OPTIONS = ['square', 'dot', 'extra-rounded'] as const;

@Component({
    selector: 'app-qr-codes',
    standalone: true,
    imports: [
        RouterLink,
        FormsModule,
        ReactiveFormsModule,
        TranslocoPipe,
        ButtonModule,
        ColorPickerModule,
        ConfirmDialogModule,
        DrawerModule,
        FileUploadModule,
        IconFieldModule,
        InputIconModule,
        InputNumberModule,
        InputTextModule,
        SelectModule,
        SplitButtonModule,
        TableModule,
        TagModule,
        TextareaModule,
        CopyControl,
        DialogShell,
        PageBreadcrumb,
        PageHeader,
        PageHeaderLeft,
        PageState,
        RelativeDateTime,
        TableRowActions,
        ReportRangeToolbar,
        KpiCard,
        MetricCardGroup,
        SeriesChart,
        QRCodePreview,
        SiteSelectOption
    ],
    templateUrl: './qr-codes.html',
    styleUrl: './qr-codes.css',
    changeDetection: ChangeDetectionStrategy.OnPush,
    providers: [ConfirmationService]
})
export class QRCodesPage {
    protected readonly siteService = inject(SiteService);
    private readonly service = inject(QRCodesService);
    private readonly share = inject(ShareService);
    private readonly route = inject(ActivatedRoute);
    private readonly router = inject(Router);
    private readonly transloco = inject(TranslocoService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly activeLanguage = injectActiveLang();
    private readonly clipboard = inject(Clipboard);
    private readonly confirmation = inject(ConfirmationService);
    private readonly reportRange = injectReportRange();

    protected readonly qrs = signal<QRCode[]>([]);
    protected readonly summary = signal<QRCodeSummary | null>(null);
    protected readonly openSeries = signal<SeriesChartPoint[]>([]);
    protected readonly shares = signal<QRCodeShareLink[]>([]);
    protected readonly isLoading = signal(false);
    protected readonly isStatsLoading = signal(false);
    protected readonly isSaving = signal(false);
    protected readonly isUploading = signal(false);
    protected readonly errorKey = signal<string | null>(null);
    protected readonly saveErrorKey = signal<string | null>(null);
    protected readonly selectedFile = signal<File | null>(null);
    protected readonly selectedFilePreviewURL = signal<string | null>(null);
    protected readonly editorVisible = signal(false);
    protected readonly editingQR = signal<QRCode | null>(null);
    protected readonly selectedSite = signal<Site | null>(null);
    protected readonly customParamRows = signal<CustomParamRow[]>([]);
    protected readonly shareDialogVisible = signal(false);
    protected readonly shareDialogQR = signal<QRCode | null>(null);
    protected readonly shareNotice = signal<ShareNotice | null>(null);
    protected readonly sharesLoading = signal(false);
    protected readonly shareCreating = signal(false);
    protected readonly deletingShareID = signal<string | null>(null);
    protected readonly archivingQRID = signal<string | null>(null);
    protected readonly pageNotice = signal<ShareNotice | null>(null);

    private listRequestID = 0;
    private statsRequestID = 0;
    private sharesRequestID = 0;

    private readonly routeQRID = toSignal(this.route.paramMap.pipe(map((params) => params.get('qrID'))), { initialValue: this.route.snapshot.paramMap.get('qrID') });

    protected readonly isShareMode = computed(() => this.share.isShareMode());
    protected readonly isDetailRoute = computed(() => !!this.routeQRID());

    protected readonly form = new FormGroup({
        name: new FormControl('', { nonNullable: true, validators: [Validators.required] }),
        destination_url: new FormControl('', { nonNullable: true, validators: [Validators.required, this.urlValidator] }),
        utm_source: new FormControl('', { nonNullable: true }),
        utm_medium: new FormControl('', { nonNullable: true }),
        utm_campaign: new FormControl('', { nonNullable: true }),
        utm_term: new FormControl('', { nonNullable: true }),
        utm_content: new FormControl('', { nonNullable: true }),
        custom_params: new FormControl('', { nonNullable: true }),
        foreground: new FormControl('#111827', { nonNullable: true }),
        background: new FormControl('#ffffff', { nonNullable: true }),
        dots: new FormControl<(typeof QR_DOT_OPTIONS)[number]>('rounded', { nonNullable: true }),
        corners: new FormControl<(typeof QR_CORNER_OPTIONS)[number]>('extra-rounded', { nonNullable: true }),
        image_margin: new FormControl(6, { nonNullable: true })
    });
    private readonly formValue = toSignal(this.form.valueChanges, { initialValue: this.form.getRawValue() });

    protected readonly selectedQR = computed(() => {
        const id = this.routeQRID();
        const items = this.qrs();
        if (id) return items.find((qr) => qr.id === id) ?? null;
        return null;
    });

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        const site = this.siteService.activeSite();
        const items: PageBreadcrumbItem[] = [];
        if (site && !this.isShareMode()) {
            items.push({ label: site.domain, favicon: site, routerLink: '/dashboard' });
        }
        items.push({ label: this.transloco.translate('nav.utm'), routerLink: this.shareAwareLink('/utm') });
        items.push({ label: this.transloco.translate('qrCodes.breadcrumb'), isCurrent: true });
        return items;
    });

    protected readonly dotOptions = computed(() =>
        QR_DOT_OPTIONS.map((value) => ({
            label: this.transloco.translate(`qrCodes.style.dots.${value}`),
            value
        }))
    );
    protected readonly cornerOptions = computed(() =>
        QR_CORNER_OPTIONS.map((value) => ({
            label: this.transloco.translate(`qrCodes.style.corners.${value}`),
            value
        }))
    );

    protected readonly editorStyle = computed<QRCodeStyle>(() => this.formStyle());
    protected readonly editorDestinationPreview = computed(() => buildQRCodeDestination(this.requestFromForm(), this.editingQR()?.id));
    protected readonly editorQRPreviewURL = computed(() => this.editingQR()?.redirect_url || this.editorDestinationPreview() || 'https://hitkeep.com');
    protected readonly editorAssetURL = computed(() => {
        const selectedPreview = this.selectedFilePreviewURL();
        if (selectedPreview) return selectedPreview;
        const qr = this.editingQR();
        if (!qr?.has_asset) return null;
        return `${this.service.assetURL(qr.site_id, qr.id)}?v=${encodeURIComponent(qr.updated_at)}`;
    });
    protected readonly safetyWarnings = computed(() => this.scanSafetyWarnings(this.requestFromForm(), this.editorDestinationPreview()));
    protected readonly selectedAssetURL = computed(() => {
        const qr = this.selectedQR();
        if (!qr?.has_asset) return null;
        return `${this.service.assetURL(qr.site_id, qr.id)}?v=${encodeURIComponent(qr.updated_at)}`;
    });

    protected readonly kpis = computed(() => {
        this.activeLanguage();
        const summary = this.summary();
        const loading = this.isStatsLoading();
        return [
            {
                label: this.transloco.translate('qrCodes.kpis.opens'),
                value: summary?.open_count ?? 0,
                loading
            },
            {
                label: this.transloco.translate('dashboard.kpis.pageviews'),
                value: summary?.pageviews ?? 0,
                loading
            },
            {
                label: this.transloco.translate('dashboard.traffic.visitors'),
                value: summary?.visitors ?? 0,
                loading
            }
        ];
    });

    protected readonly openSeriesConfig = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        return [
            {
                key: 'opens',
                label: this.transloco.translate('qrCodes.kpis.opens'),
                color: '#2563eb',
                gradientFrom: 'rgba(37, 99, 235, 0.22)',
                gradientTo: 'rgba(37, 99, 235, 0.02)'
            }
        ];
    });

    protected readonly metricTabs = computed<MetricCardGroupTab<QRMetricGroup>[]>(() => {
        this.activeLanguage();
        const summary = this.summary();
        const loading = this.isStatsLoading();
        return [
            {
                id: 'qr',
                label: this.transloco.translate('qrCodes.analytics.fullAnalytics'),
                icon: 'pi-chart-line',
                cards: [
                    { id: 'pages', title: this.transloco.translate('common.metrics.topPages'), icon: 'pi-file', data: summary?.top_pages ?? [], isLoading: loading },
                    { id: 'referrers', title: this.transloco.translate('common.metrics.topReferrers'), icon: 'pi-link', data: summary?.top_referrers ?? [], isLoading: loading },
                    { id: 'devices', title: this.transloco.translate('common.metrics.devices'), icon: 'pi-desktop', data: summary?.top_devices ?? [], isLoading: loading },
                    { id: 'countries', title: this.transloco.translate('common.metrics.countries'), icon: 'pi-globe', data: summary?.top_countries ?? [], isLoading: loading, showCountryFlags: true, showCountryNames: true }
                ]
            }
        ];
    });

    constructor() {
        this.destroyRef.onDestroy(() => this.clearSelectedFile());

        effect(() => {
            const site = this.siteService.activeSite();
            this.loadList(site?.id ?? null);
        });

        effect(() => {
            const site = this.siteService.activeSite();
            const qr = this.selectedQR();
            const range = this.currentDateRange();
            if (!site || !qr || !range) {
                this.summary.set(null);
                this.openSeries.set([]);
                this.shares.set([]);
                return;
            }
            this.loadStats(site.id, qr.id, range.from, range.to);
            this.loadShares(site.id, qr.id);
        });
    }

    protected refresh(): void {
        const site = this.siteService.activeSite();
        this.loadList(site?.id ?? null);
        const qr = this.selectedQR();
        const range = this.currentDateRange();
        if (site && qr && range) {
            this.loadStats(site.id, qr.id, range.from, range.to);
            this.loadShares(site.id, qr.id);
        }
    }

    protected openCreate(): void {
        this.editingQR.set(null);
        this.clearSelectedFile();
        this.selectedSite.set(null);
        this.setCustomParamRows({});
        this.saveErrorKey.set(null);
        this.form.reset({
            name: '',
            destination_url: '',
            utm_source: 'qr',
            utm_medium: 'offline',
            utm_campaign: '',
            utm_term: '',
            utm_content: '',
            custom_params: '',
            foreground: '#111827',
            background: '#ffffff',
            dots: 'rounded',
            corners: 'extra-rounded',
            image_margin: 6
        });
        this.editorVisible.set(true);
    }

    protected openEdit(qr: QRCode): void {
        this.editingQR.set(qr);
        this.clearSelectedFile();
        this.selectedSite.set(null);
        this.setCustomParamRows(qr.custom_params ?? {});
        this.saveErrorKey.set(null);
        this.form.reset({
            name: qr.name,
            destination_url: qr.destination_url,
            utm_source: qr.utm_source ?? '',
            utm_medium: qr.utm_medium ?? '',
            utm_campaign: qr.utm_campaign ?? '',
            utm_term: qr.utm_term ?? '',
            utm_content: qr.utm_content ?? '',
            custom_params: '',
            foreground: qr.style?.foreground ?? '#111827',
            background: qr.style?.background ?? '#ffffff',
            dots: qr.style?.dots ?? 'rounded',
            corners: qr.style?.corners ?? 'extra-rounded',
            image_margin: qr.style?.image_margin ?? 6
        });
        this.editorVisible.set(true);
    }

    protected save(): void {
        const site = this.siteService.activeSite();
        if (!site || this.form.invalid || this.isSaving()) {
            this.form.markAllAsTouched();
            return;
        }

        const request = this.requestFromForm();
        const editing = this.editingQR();
        this.isSaving.set(true);
        this.saveErrorKey.set(null);
        const action = editing ? this.service.update(site.id, editing.id, request) : this.service.create(site.id, request);
        action.pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
            next: (qr) => this.saveAssetIfNeeded(site.id, qr),
            error: () => {
                this.saveErrorKey.set('qrCodes.editor.saveError');
                this.isSaving.set(false);
            }
        });
    }

    protected archive(qr: QRCode): void {
        const site = this.siteService.activeSite();
        if (!site || this.isShareMode() || this.archivingQRID()) return;
        this.archivingQRID.set(qr.id);
        this.pageNotice.set(null);
        this.service
            .archive(site.id, qr.id)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: () => {
                    this.pageNotice.set({ kind: 'success', key: 'qrCodes.status.archiveSuccess' });
                    this.refresh();
                    if (this.selectedQR()?.id === qr.id) {
                        void this.router.navigateByUrl(this.shareAwareLink('/utm/qr-codes'));
                    }
                },
                error: () => {
                    this.pageNotice.set({ kind: 'error', key: 'qrCodes.errors.archive' });
                    this.archivingQRID.set(null);
                },
                complete: () => this.archivingQRID.set(null)
            });
    }

    protected confirmArchive(qr: QRCode): void {
        if (this.isShareMode() || this.archivingQRID()) return;

        this.confirmation.confirm({
            message: this.transloco.translate('qrCodes.confirm.archiveMessage', { name: qr.name }),
            header: this.transloco.translate('qrCodes.confirm.archiveTitle'),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('common.actions.archive')),
            accept: () => this.archive(qr)
        });
    }

    protected selectQR(qr: QRCode): void {
        void this.router.navigateByUrl(this.shareAwareLink(`/utm/qr-codes/${qr.id}`));
    }

    protected qrLink(qr: QRCode): string {
        return this.shareAwareLink(`/utm/qr-codes/${qr.id}`);
    }

    protected onAssetSelected(event: FileSelectEvent): void {
        const file = event.files?.[0] ?? event.currentFiles?.[0] ?? null;
        this.selectedFile.set(file);
        const previous = this.selectedFilePreviewURL();
        if (previous) URL.revokeObjectURL(previous);
        this.selectedFilePreviewURL.set(file ? URL.createObjectURL(file) : null);
    }

    protected onAssetRemoved(): void {
        this.clearSelectedFile();
    }

    protected removeAsset(): void {
        const site = this.siteService.activeSite();
        const qr = this.editingQR();
        if (!site || !qr) return;
        this.isUploading.set(true);
        this.service
            .deleteAsset(site.id, qr.id)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: () => {
                    this.editingQR.update((current) => (current ? { ...current, has_asset: false } : current));
                    this.refresh();
                },
                error: () => this.saveErrorKey.set('qrCodes.editor.assetDeleteError'),
                complete: () => this.isUploading.set(false)
            });
    }

    protected openShareDialog(qr: QRCode): void {
        if (this.isShareMode()) return;
        this.shareDialogQR.set(qr);
        this.shareNotice.set(null);
        this.shareDialogVisible.set(true);
        this.loadShares(qr.site_id, qr.id);
    }

    protected onShareDialogVisibleChange(visible: boolean): void {
        this.shareDialogVisible.set(visible);
        if (!visible) {
            this.shareDialogQR.set(null);
            this.shareNotice.set(null);
            this.deletingShareID.set(null);
        }
    }

    protected createShare(): void {
        const site = this.siteService.activeSite();
        const qr = this.shareDialogQR() ?? this.selectedQR();
        if (!site || !qr || this.isShareMode() || this.shareCreating()) return;
        this.shareCreating.set(true);
        this.shareNotice.set(null);
        this.service
            .createShare(site.id, qr.id)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (link) => {
                    this.shares.update((links) => [link, ...links.filter((existing) => existing.id !== link.id)]);
                    this.shareNotice.set({ kind: 'success', key: 'qrCodes.share.createSuccess' });
                    this.loadShares(site.id, qr.id);
                },
                error: () => {
                    this.shareNotice.set({ kind: 'error', key: 'qrCodes.errors.share' });
                    this.shareCreating.set(false);
                },
                complete: () => this.shareCreating.set(false)
            });
    }

    protected deleteShare(shareID: string): void {
        const site = this.siteService.activeSite();
        const qr = this.shareDialogQR() ?? this.selectedQR();
        if (!site || !qr || this.isShareMode() || this.deletingShareID()) return;
        this.deletingShareID.set(shareID);
        this.shareNotice.set(null);
        this.service
            .deleteShare(site.id, qr.id, shareID)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: () => {
                    this.shareNotice.set({ kind: 'success', key: 'qrCodes.share.deleteSuccess' });
                    this.loadShares(site.id, qr.id);
                },
                error: () => {
                    this.shareNotice.set({ kind: 'error', key: 'qrCodes.errors.share' });
                    this.deletingShareID.set(null);
                },
                complete: () => this.deletingShareID.set(null)
            });
    }

    protected confirmDeleteShare(link: QRCodeShareLink): void {
        if (this.deletingShareID() !== null || this.isShareMode()) return;

        this.confirmation.confirm({
            message: this.transloco.translate('qrCodes.share.deleteConfirmMessage'),
            header: this.transloco.translate('qrCodes.share.deleteConfirmTitle'),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('common.actions.delete')),
            accept: () => this.deleteShare(link.id)
        });
    }

    protected exportQR(preview: QRCodePreview, qr: QRCode, extension: 'svg' | 'png', size?: QRExportSize): void {
        const site = this.siteService.activeSite();
        const exportSize = size ?? (extension === 'svg' ? 2048 : 2048);
        const suffix = extension === 'svg' ? 'print-vector' : `print-${exportSize}px`;
        void preview.export(qrExportFilename(site?.domain, qr.name, suffix, extension), extension, exportSize);
    }

    protected printExportMenuItems(preview: QRCodePreview, qr: QRCode): MenuItem[] {
        this.activeLanguage();
        return [
            {
                label: this.transloco.translate('qrCodes.actions.exportPngSize', { size: 1024 }),
                icon: 'pi pi-image',
                command: () => this.exportQR(preview, qr, 'png', 1024)
            },
            {
                label: this.transloco.translate('qrCodes.actions.exportPngSize', { size: 2048 }),
                icon: 'pi pi-image',
                command: () => this.exportQR(preview, qr, 'png', 2048)
            },
            {
                label: this.transloco.translate('qrCodes.actions.exportPngSize', { size: 4096 }),
                icon: 'pi pi-image',
                command: () => this.exportQR(preview, qr, 'png', 4096)
            }
        ];
    }

    protected exportTakeout(qr: QRCode, format: TakeoutExportFormat): void {
        const site = this.siteService.activeSite();
        if (!site) return;
        this.pageNotice.set(null);
        this.service
            .downloadTakeout(site.id, qr, format, site.domain)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: () => this.pageNotice.set({ kind: 'success', key: 'qrCodes.status.takeoutSuccess' }),
                error: () => this.pageNotice.set({ kind: 'error', key: 'qrCodes.errors.takeout' })
            });
    }

    protected takeoutMenuItems(qr: QRCode): MenuItem[] {
        this.activeLanguage();
        return buildTakeoutExportMenuItems(this.transloco, (format) => this.exportTakeout(qr, format));
    }

    protected shareAwareLink(path: string): string {
        const token = this.share.token();
        return token ? `/share/${token}${path}` : path;
    }

    protected qrActions(qr: QRCode): TableRowActionItem[] {
        this.activeLanguage();
        const actions: TableRowActionItem[] = [
            {
                label: this.transloco.translate('qrCodes.actions.viewAnalytics'),
                icon: 'pi pi-chart-line',
                command: () => this.selectQR(qr)
            }
        ];
        if (!this.isShareMode()) {
            actions.push(
                {
                    label: this.transloco.translate('common.actions.edit'),
                    icon: 'pi pi-pencil',
                    command: () => this.openEdit(qr)
                },
                {
                    label: this.transloco.translate('qrCodes.share.create'),
                    icon: 'pi pi-share-alt',
                    command: () => this.openShareDialog(qr)
                },
                { separator: true },
                {
                    label: this.transloco.translate('common.actions.archive'),
                    icon: 'pi pi-trash',
                    danger: true,
                    disabled: this.archivingQRID() !== null,
                    command: () => this.confirmArchive(qr)
                }
            );
        }
        return actions;
    }

    protected shareLinkActions(link: QRCodeShareLink): TableRowActionItem[] {
        this.activeLanguage();
        return [
            {
                label: this.transloco.translate('common.copyControl.copy'),
                icon: 'pi pi-copy',
                disabled: !link.url,
                command: () => this.copyShareLink(link)
            },
            { separator: true },
            {
                label: this.transloco.translate('common.actions.delete'),
                icon: 'pi pi-trash',
                danger: true,
                disabled: this.deletingShareID() !== null,
                command: () => this.confirmDeleteShare(link)
            }
        ];
    }

    protected copyShareLink(link: QRCodeShareLink): void {
        if (!link.url) return;
        const copied = this.clipboard.copy(link.url);
        this.shareNotice.set({ kind: copied ? 'success' : 'error', key: copied ? 'common.copyControl.copied' : 'common.copyControl.failed' });
    }

    protected prefillFromSite(site: Site | null): void {
        if (!site) return;
        this.selectedSite.set(site);
        this.form.controls.destination_url.setValue(`https://${site.domain}`);
        this.form.controls.destination_url.markAsDirty();
        setTimeout(() => this.selectedSite.set(null), 0);
    }

    protected addCustomParamRow(): void {
        this.customParamRows.update((rows) => [...rows, { id: crypto.randomUUID(), key: '', value: '' }]);
    }

    protected updateCustomParamRow(id: string, field: 'key' | 'value', value: string): void {
        this.customParamRows.update((rows) => rows.map((row) => (row.id === id ? { ...row, [field]: value } : row)));
    }

    protected removeCustomParamRow(id: string): void {
        this.customParamRows.update((rows) => rows.filter((row) => row.id !== id));
    }

    protected formatMetricRows(rows: MetricStat[] | undefined): MetricStat[] {
        return rows ?? [];
    }

    protected finalDestination(qr: QRCode): string {
        return buildQRCodeDestination(
            {
                destination_url: qr.destination_url,
                utm_source: qr.utm_source ?? '',
                utm_medium: qr.utm_medium ?? '',
                utm_campaign: qr.utm_campaign ?? '',
                utm_term: qr.utm_term ?? '',
                utm_content: qr.utm_content ?? '',
                custom_params: qr.custom_params ?? {}
            },
            qr.id
        );
    }

    private loadList(siteID: string | null): void {
        const requestID = ++this.listRequestID;
        this.qrs.set([]);
        this.summary.set(null);
        this.openSeries.set([]);
        if (!siteID) return;

        this.isLoading.set(true);
        this.errorKey.set(null);
        this.service
            .list(siteID)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (qrs) => {
                    if (requestID !== this.listRequestID) return;
                    this.qrs.set(qrs);
                },
                error: () => {
                    if (requestID !== this.listRequestID) return;
                    this.errorKey.set('qrCodes.errors.load');
                },
                complete: () => {
                    if (requestID === this.listRequestID) this.isLoading.set(false);
                }
            });
    }

    private loadStats(siteID: string, qrID: string, from: string, to: string): void {
        const requestID = ++this.statsRequestID;
        this.isStatsLoading.set(true);
        this.service
            .summary(siteID, qrID, from, to)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (summary) => {
                    if (requestID !== this.statsRequestID) return;
                    this.summary.set(summary);
                },
                error: () => {
                    if (requestID !== this.statsRequestID) return;
                    this.errorKey.set('qrCodes.errors.stats');
                },
                complete: () => {
                    if (requestID === this.statsRequestID) this.isStatsLoading.set(false);
                }
            });
        this.service
            .openSeries(siteID, qrID, from, to)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (points) => {
                    if (requestID !== this.statsRequestID) return;
                    this.openSeries.set(points.map((point) => ({ time: point.time, opens: point.opens })));
                },
                error: () => {
                    if (requestID === this.statsRequestID) this.errorKey.set('qrCodes.errors.stats');
                }
            });
    }

    private loadShares(siteID: string, qrID: string): void {
        if (this.isShareMode()) return;
        const requestID = ++this.sharesRequestID;
        this.sharesLoading.set(true);
        this.service
            .listShares(siteID, qrID)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (shares) => {
                    if (requestID !== this.sharesRequestID) return;
                    this.shares.set(this.mergeKnownShareURLs(shares));
                },
                error: () => {
                    if (requestID === this.sharesRequestID) {
                        this.errorKey.set('qrCodes.errors.share');
                        this.sharesLoading.set(false);
                    }
                },
                complete: () => {
                    if (requestID === this.sharesRequestID) this.sharesLoading.set(false);
                }
            });
    }

    private mergeKnownShareURLs(links: QRCodeShareLink[]): QRCodeShareLink[] {
        const knownURLs = new Map<string, string>();
        for (const link of this.shares()) {
            if (link.url) knownURLs.set(link.id, link.url);
        }
        return links.map((link) => ({ ...link, url: link.url || knownURLs.get(link.id) || '' }));
    }

    private saveAssetIfNeeded(siteID: string, qr: QRCode): void {
        const file = this.selectedFile();
        if (!file) {
            this.afterSaved(qr);
            return;
        }

        this.isUploading.set(true);
        this.service
            .uploadAsset(siteID, qr.id, file)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: () => this.afterSaved({ ...qr, has_asset: true }),
                error: () => {
                    this.saveErrorKey.set('qrCodes.editor.assetError');
                    this.isSaving.set(false);
                    this.isUploading.set(false);
                },
                complete: () => this.isUploading.set(false)
            });
    }

    private afterSaved(qr: QRCode): void {
        this.isSaving.set(false);
        this.editorVisible.set(false);
        this.clearSelectedFile();
        this.refresh();
        this.router.navigate([this.shareAwareLink(`/utm/qr-codes/${qr.id}`)]);
    }

    private clearSelectedFile(): void {
        const previous = this.selectedFilePreviewURL();
        if (previous) URL.revokeObjectURL(previous);
        this.selectedFile.set(null);
        this.selectedFilePreviewURL.set(null);
    }

    private requestFromForm(): QRCodeRequest {
        const value = this.formValue();
        return {
            name: value.name ?? '',
            destination_url: value.destination_url ?? '',
            utm_source: value.utm_source ?? '',
            utm_medium: value.utm_medium ?? '',
            utm_campaign: value.utm_campaign ?? '',
            utm_term: value.utm_term ?? '',
            utm_content: value.utm_content ?? '',
            custom_params: this.customParamsFromRows(),
            style: this.formStyle()
        };
    }

    private formStyle(): QRCodeStyle {
        const value = this.formValue();
        return {
            foreground: value.foreground ?? '#111827',
            background: value.background ?? '#ffffff',
            dots: value.dots ?? 'rounded',
            corners: value.corners ?? 'extra-rounded',
            image_margin: Number(value.image_margin ?? 6)
        };
    }

    private customParamsFromRows(): Record<string, string> {
        const params: Record<string, string> = {};
        for (const row of this.customParamRows()) {
            const key = row.key.trim();
            const value = row.value.trim();
            if (key && value) {
                params[key] = value;
            }
        }
        return params;
    }

    private setCustomParamRows(params: Record<string, string>): void {
        const rows = Object.entries(params).map(([key, value]) => ({ id: crypto.randomUUID(), key, value }));
        this.customParamRows.set(rows.length ? rows : [{ id: crypto.randomUUID(), key: '', value: '' }]);
    }

    private scanSafetyWarnings(request: QRCodeRequest, finalURL: string): string[] {
        this.activeLanguage();
        const warnings: string[] = [];
        if (!finalURL) {
            warnings.push(this.transloco.translate('qrCodes.warnings.invalidDestination'));
        }
        if (!request.utm_source.trim() && !request.utm_campaign.trim()) {
            warnings.push(this.transloco.translate('qrCodes.warnings.missingCampaign'));
        }
        if (finalURL.length > 1800) {
            warnings.push(this.transloco.translate('qrCodes.warnings.longUrl'));
        }
        if (request.custom_params['hk_qr']) {
            warnings.push(this.transloco.translate('qrCodes.warnings.reservedParam'));
        }
        return warnings;
    }

    private currentDateRange(): { from: string; to: string } | null {
        return this.reportRange.currentDateRange();
    }

    private urlValidator(control: AbstractControl<string>): ValidationErrors | null {
        const value = control.value?.trim() ?? '';
        if (!value) return null;
        try {
            const url = new URL(value);
            return url.protocol === 'http:' || url.protocol === 'https:' ? null : { urlInvalid: true };
        } catch {
            return { urlInvalid: true };
        }
    }
}
