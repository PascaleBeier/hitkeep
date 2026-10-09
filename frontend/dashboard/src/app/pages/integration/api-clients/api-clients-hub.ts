import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router, RouterOutlet } from '@angular/router';
import { filter, map, startWith } from 'rxjs';
import { TranslocoPipe } from '@jsverse/transloco';
import { TabsModule } from '@openng/optimus-ui/tabs';
import { PageFrame } from '@components/page-frame/page-frame';

type ApiClientsTab = 'clients' | 'reference';

@Component({
    selector: 'app-api-clients-hub',
    imports: [PageFrame, RouterOutlet, TabsModule, TranslocoPipe],
    templateUrl: './api-clients-hub.html',
    styleUrl: './api-clients-hub.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ApiClientsHub {
    private readonly router = inject(Router);

    protected readonly activeTab = toSignal(
        this.router.events.pipe(
            filter((event): event is NavigationEnd => event instanceof NavigationEnd),
            startWith(null),
            map(() => this.tabFromUrl(this.router.url))
        ),
        { initialValue: this.tabFromUrl(this.router.url) }
    );

    protected onTabChange(value: string | number | undefined): void {
        const tab = value === 'clients' || value === 'reference' ? value : this.activeTab();
        void this.router.navigate(['/integration/api-clients', tab]);
    }

    private tabFromUrl(url: string): ApiClientsTab {
        return url.includes('/integration/api-clients/reference') ? 'reference' : 'clients';
    }
}
