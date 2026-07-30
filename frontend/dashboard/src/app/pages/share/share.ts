import { ChangeDetectionStrategy, Component, effect, inject, input, signal } from '@angular/core';
import { RouterOutlet } from '@angular/router';

import { TranslocoPipe } from '@jsverse/transloco';
import { ShareService } from '@services/share.service';
import { SiteService } from '@features/sites/services/site.service';

@Component({
    selector: 'app-share-dashboard',
    standalone: true,
    imports: [RouterOutlet, TranslocoPipe],
    templateUrl: './share.html',
    styleUrl: './share.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ShareDashboard {
    private shareService = inject(ShareService);
    private siteService = inject(SiteService);
    protected readonly token = input<string>();

    protected loading = signal(true);
    protected error = signal<string | null>(null);

    constructor() {
        effect((onCleanup) => {
            const token = this.token()?.trim();
            this.loading.set(true);
            this.error.set(null);
            this.siteService.sites.set([]);
            this.siteService.activeSite.set(null);
            if (!token) {
                this.error.set('share.page.missingToken');
                this.loading.set(false);
                return;
            }

            const subscription = this.shareService.loadShareSite(token).subscribe({
                next: (site) => {
                    this.siteService.sites.set([site]);
                    this.siteService.activeSite.set(site);
                    this.loading.set(false);
                },
                error: () => {
                    this.error.set('share.page.invalidOrExpired');
                    this.loading.set(false);
                }
            });
            onCleanup(() => subscription.unsubscribe());
        });
    }
}
