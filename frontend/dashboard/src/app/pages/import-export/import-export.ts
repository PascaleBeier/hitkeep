import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router, RouterOutlet } from '@angular/router';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { filter, map, startWith } from 'rxjs';
import { TabsModule } from 'primeng/tabs';
import { PageFrame } from '@components/page-frame/page-frame';
import { PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';

type ImportExportTab = 'import' | 'export';

@Component({
    selector: 'app-import-export-page',
    imports: [PageFrame, RouterOutlet, TabsModule, TranslocoPipe],
    templateUrl: './import-export.html',
    styleUrl: './import-export.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ImportExportPage {
    private readonly router = inject(Router);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, {
        initialValue: this.transloco.getActiveLang()
    });

    protected readonly activeTab = toSignal(
        this.router.events.pipe(
            filter((event): event is NavigationEnd => event instanceof NavigationEnd),
            startWith(null),
            map(() => this.tabFromUrl(this.router.url))
        ),
        { initialValue: this.tabFromUrl(this.router.url) }
    );

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        return [
            {
                label: this.transloco.translate('importExport.title'),
                isCurrent: true
            }
        ];
    });

    protected onTabChange(value: string | number | undefined): void {
        const tab = value === 'import' || value === 'export' ? value : this.activeTab();
        void this.router.navigate(['/import-export', tab]);
    }

    private tabFromUrl(url: string): ImportExportTab {
        return url.includes('/import-export/export') ? 'export' : 'import';
    }
}
