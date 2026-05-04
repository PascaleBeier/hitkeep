import { Component, input, signal, inject, ChangeDetectionStrategy, computed, DestroyRef } from '@angular/core';
import { toSignal, takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { finalize } from 'rxjs';

import { SplitButtonModule } from 'primeng/splitbutton';
import { MenuItem } from 'primeng/api';
import { CopyControl } from '@components/copy-control/copy-control';
import { Site } from '@models/analytics.types';
import { buildTakeoutExportMenuItems, DEFAULT_TAKEOUT_EXPORT_FORMAT, TakeoutExportFormat } from '@core/export/export-formats';
import { TakeoutDownloadService } from '@services/takeout-download.service';

@Component({
    selector: 'app-site-general-settings',
    standalone: true,
    imports: [SplitButtonModule, CopyControl, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    templateUrl: './site-general-settings.html',
    styleUrl: './site-general-settings.css'
})
export class SiteGeneralSettings {
    site = input.required<Site | null>();
    protected isExporting = signal(false);
    protected exportState = signal<'idle' | 'success' | 'error'>('idle');
    private destroyRef = inject(DestroyRef);
    private takeoutDownloadService = inject(TakeoutDownloadService);
    private transloco = inject(TranslocoService);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });
    protected readonly exportMenuItems = computed<MenuItem[]>(() => {
        this.activeLanguage();
        return buildTakeoutExportMenuItems(this.transloco, (format) => this.downloadData(format));
    });

    downloadData(format: TakeoutExportFormat = DEFAULT_TAKEOUT_EXPORT_FORMAT) {
        const site = this.site();
        if (!site?.id || this.isExporting()) return;

        this.isExporting.set(true);
        this.exportState.set('idle');

        this.takeoutDownloadService
            .downloadSiteTakeout(site.id, site.domain, format)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isExporting.set(false))
            )
            .subscribe({
                next: () => this.exportState.set('success'),
                error: () => this.exportState.set('error')
            });
    }
}
