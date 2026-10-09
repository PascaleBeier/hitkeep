import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router, RouterOutlet } from '@angular/router';
import { filter, map, startWith } from 'rxjs';
import { TranslocoPipe } from '@jsverse/transloco';
import { TabsModule } from '@openng/optimus-ui/tabs';
import { PageFrame } from '@components/page-frame/page-frame';
import { ShareService } from '@services/share.service';

type UtmTab = 'builder' | 'qr-codes';

@Component({
    selector: 'app-utm-hub',
    imports: [PageFrame, RouterOutlet, TabsModule, TranslocoPipe],
    templateUrl: './utm.html',
    styleUrl: './utm.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class UtmHub {
    private readonly router = inject(Router);
    private readonly shareService = inject(ShareService);

    protected readonly isShareMode = computed(() => this.shareService.isShareMode());

    protected readonly activeTab = toSignal(
        this.router.events.pipe(
            filter((event): event is NavigationEnd => event instanceof NavigationEnd),
            startWith(null),
            map(() => this.tabFromUrl(this.router.url))
        ),
        { initialValue: this.tabFromUrl(this.router.url) }
    );

    protected onTabChange(value: string | number | undefined): void {
        const tab = value === 'builder' || value === 'qr-codes' ? value : this.activeTab();
        void this.router.navigate([this.shareAwareLink(`/utm/${tab}`)]);
    }

    private tabFromUrl(url: string): UtmTab {
        return url.includes('/utm/qr-codes') ? 'qr-codes' : 'builder';
    }

    private shareAwareLink(link: string): string {
        const token = this.shareService.token();
        return token ? `/share/${token}${link}` : link;
    }
}
