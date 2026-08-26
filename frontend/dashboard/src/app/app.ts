import { Location } from '@angular/common';
import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { NavigationCancel, NavigationCancellationCode, NavigationEnd, NavigationError, NavigationStart, Router, RouterOutlet } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { RouteProgressBar } from '@layout/route-progress-bar';
import { isApplicationErrorUrl } from '@services/application-error-navigation.service';

@Component({
    selector: 'app-root',
    imports: [RouterOutlet, RouteProgressBar, TranslocoPipe],
    templateUrl: './app.html',
    changeDetection: ChangeDetectionStrategy.OnPush,
    styleUrl: './app.css'
})
export class App {
    private readonly router = inject(Router);
    private readonly location = inject(Location);
    protected readonly initialNavigationCompleted = signal(false);
    protected readonly routeUnavailable = signal(false);

    constructor() {
        this.router.events.pipe(takeUntilDestroyed()).subscribe((event) => {
            if (event instanceof NavigationStart && !this.initialNavigationCompleted()) {
                this.routeUnavailable.set(false);
            }
            if (event instanceof NavigationEnd) {
                this.initialNavigationCompleted.set(true);
                this.routeUnavailable.set(false);
                return;
            }
            const terminalCancellation = event instanceof NavigationCancel && event.code !== undefined && isTerminalInitialNavigationCancellation(event.code);
            const errorPageFailed = event instanceof NavigationError && isApplicationErrorUrl(event.url);
            if ((terminalCancellation && !this.initialNavigationCompleted()) || errorPageFailed) {
                this.routeUnavailable.set(true);
            }
        });
    }

    protected reloadApplication(): void {
        this.location.historyGo(0);
    }
}

export function isTerminalInitialNavigationCancellation(code: NavigationCancellationCode): boolean {
    return code === NavigationCancellationCode.GuardRejected || code === NavigationCancellationCode.NoDataFromResolver || code === NavigationCancellationCode.Aborted;
}
