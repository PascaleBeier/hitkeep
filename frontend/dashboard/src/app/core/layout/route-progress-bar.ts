import { ChangeDetectionStrategy, Component, computed, effect, inject, signal, untracked } from '@angular/core';
import { Router } from '@angular/router';

/** Delay before the bar appears so instant navigations never flash it. */
const SHOW_DELAY_MS = 120;
/** How long the completed bar stays mounted while it fills and fades out. */
const DONE_MS = 300;

type ProgressState = 'idle' | 'loading' | 'done';

@Component({
    selector: 'app-route-progress-bar',
    templateUrl: './route-progress-bar.html',
    styleUrl: './route-progress-bar.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class RouteProgressBar {
    private readonly router = inject(Router);

    private readonly navigating = computed(() => this.router.currentNavigation() !== null);
    protected readonly state = signal<ProgressState>('idle');

    constructor() {
        effect((onCleanup) => {
            if (this.navigating()) {
                if (untracked(this.state) === 'done') {
                    this.state.set('idle');
                }
                const timer = setTimeout(() => this.state.set('loading'), SHOW_DELAY_MS);
                onCleanup(() => clearTimeout(timer));
            } else if (untracked(this.state) === 'loading') {
                this.state.set('done');
                const timer = setTimeout(() => this.state.set('idle'), DONE_MS);
                onCleanup(() => clearTimeout(timer));
            }
        });
    }
}
