import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { DOCUMENT, NgOptimizedImage } from '@angular/common';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';

export interface SiteFaviconSource {
    domain: string;
}

@Component({
    selector: 'app-site-favicon',
    standalone: true,
    imports: [NgOptimizedImage],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        @if (showImage()) {
            <img [ngSrc]="faviconUrl()" class="hk-site-favicon" [width]="20" [height]="20" loading="lazy" alt="" aria-hidden="true" (error)="showFallback()" />
        } @else {
            <span class="hk-site-favicon-fallback" aria-hidden="true">
                <i class="pi pi-globe"></i>
            </span>
        }
    `,
    styles: [
        `
            :host {
                display: inline-flex;
                width: 1.25rem;
                height: 1.25rem;
                flex: 0 0 auto;
            }

            .hk-site-favicon,
            .hk-site-favicon-fallback {
                width: 1.25rem;
                height: 1.25rem;
                flex: 0 0 auto;
                border-radius: 999px;
            }

            .hk-site-favicon {
                object-fit: cover;
            }

            .hk-site-favicon-fallback {
                display: inline-flex;
                align-items: center;
                justify-content: center;
                border: 1px solid var(--p-content-border-color);
                background: color-mix(in srgb, var(--p-primary-color) 10%, var(--p-content-background));
                color: var(--p-primary-color);
                line-height: 1;
            }

            :host-context(.p-dark) .hk-site-favicon-fallback {
                border-color: color-mix(in srgb, var(--p-content-border-color) 80%, var(--p-primary-color));
                background: color-mix(in srgb, var(--p-primary-color) 16%, var(--p-content-background));
            }

            .hk-site-favicon-fallback i {
                font-size: 0.7rem;
            }
        `
    ]
})
export class SiteFavicon {
    private document = inject(DOCUMENT);
    site = input.required<SiteFaviconSource | null>();
    private readonly failedDomain = signal<string | null>(null);
    protected domain = computed(() => this.site()?.domain ?? '');
    protected showImage = computed(() => {
        const domain = this.domain();
        return domain.length > 0 && this.failedDomain() !== domain;
    });
    protected faviconUrl = computed(() => {
        const domain = this.domain();
        return domain ? browserAppUrl(this.document, `/api/favicon/${encodeURIComponent(domain)}`) : '';
    });

    protected showFallback(): void {
        const domain = this.domain();
        if (domain) {
            this.failedDomain.set(domain);
        }
    }
}
