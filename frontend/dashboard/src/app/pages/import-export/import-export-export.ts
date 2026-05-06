import { ChangeDetectionStrategy, Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { takeUntilDestroyed, toSignal } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { finalize } from 'rxjs';

import { buildTakeoutExportMenuItems, DEFAULT_TAKEOUT_EXPORT_FORMAT, TakeoutExportFormat } from '@core/export/export-formats';
import { Site } from '@models/analytics.types';
import { TakeoutDownloadService } from '@services/takeout-download.service';
import { SiteService } from '@features/sites/services/site.service';
import { MenuItem } from 'primeng/api';
import { CardModule } from 'primeng/card';
import { MessageModule } from 'primeng/message';
import { SplitButtonModule } from 'primeng/splitbutton';

type ExportState = 'idle' | 'success' | 'error';

@Component({
    selector: 'app-import-export-export-page',
    imports: [CardModule, MessageModule, SplitButtonModule, TranslocoPipe],
    templateUrl: './import-export-export.html',
    styleUrl: './import-export-export.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ImportExportExportPage {
    protected readonly isAllSitesExporting = signal(false);
    protected readonly allSitesExportState = signal<ExportState>('idle');
    protected readonly siteExportStates = signal<Record<string, ExportState>>({});
    protected readonly siteExportingIDs = signal<ReadonlySet<string>>(new Set());

    private readonly destroyRef = inject(DestroyRef);
    private readonly takeoutDownloadService = inject(TakeoutDownloadService);
    private readonly siteService = inject(SiteService);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });
    protected readonly sites = this.siteService.sites;
    protected readonly isSitesLoading = this.siteService.isLoading;
    protected readonly allSitesExportMenuItems = computed<MenuItem[]>(() => {
        this.activeLanguage();
        return buildTakeoutExportMenuItems(this.transloco, (format) => this.downloadAllSites(format));
    });

    protected downloadAllSites(format: TakeoutExportFormat = DEFAULT_TAKEOUT_EXPORT_FORMAT): void {
        if (this.isAllSitesExporting()) {
            return;
        }

        this.isAllSitesExporting.set(true);
        this.allSitesExportState.set('idle');

        this.takeoutDownloadService
            .downloadUserTakeout(format)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isAllSitesExporting.set(false))
            )
            .subscribe({
                next: () => this.allSitesExportState.set('success'),
                error: () => this.allSitesExportState.set('error')
            });
    }

    protected siteExportMenuItems(siteID: string): MenuItem[] {
        this.activeLanguage();
        return buildTakeoutExportMenuItems(this.transloco, (format) => {
            const site = this.sites().find((entry) => entry.id === siteID);
            if (site) {
                this.downloadSite(site, format);
            }
        });
    }

    protected isSiteExporting(siteID: string): boolean {
        return this.siteExportingIDs().has(siteID);
    }

    protected siteExportState(siteID: string): ExportState {
        return this.siteExportStates()[siteID] ?? 'idle';
    }

    protected downloadSite(site: Site, format: TakeoutExportFormat = DEFAULT_TAKEOUT_EXPORT_FORMAT): void {
        if (this.isSiteExporting(site.id)) {
            return;
        }

        this.siteExportingIDs.update((ids) => new Set([...ids, site.id]));
        this.siteExportStates.update((states) => ({ ...states, [site.id]: 'idle' }));

        this.takeoutDownloadService
            .downloadSiteTakeout(site.id, site.domain, format)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.siteExportingIDs.update((ids) => new Set([...ids].filter((id) => id !== site.id))))
            )
            .subscribe({
                next: () => this.siteExportStates.update((states) => ({ ...states, [site.id]: 'success' })),
                error: () => this.siteExportStates.update((states) => ({ ...states, [site.id]: 'error' }))
            });
    }
}
