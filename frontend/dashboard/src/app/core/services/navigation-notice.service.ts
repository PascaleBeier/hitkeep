import { Injectable, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { NavigationStart, Router } from '@angular/router';
import { filter } from 'rxjs';

@Injectable({ providedIn: 'root' })
export class NavigationNoticeService {
    private readonly router = inject(Router);
    private navigationsToPreserve = 0;
    readonly key = signal<string | null>(null);

    constructor() {
        this.router.events
            .pipe(
                filter((event): event is NavigationStart => event instanceof NavigationStart),
                takeUntilDestroyed()
            )
            .subscribe(() => {
                if (this.navigationsToPreserve > 0) {
                    this.navigationsToPreserve -= 1;
                    return;
                }
                this.clear();
            });
    }

    show(key: string, options: { preserveNextNavigation?: boolean } = {}): void {
        this.key.set(key);
        this.navigationsToPreserve = options.preserveNextNavigation ? 1 : 0;
    }

    consume(): string | null {
        const key = this.key();
        this.clear();
        return key;
    }

    clear(): void {
        this.key.set(null);
        this.navigationsToPreserve = 0;
    }
}
