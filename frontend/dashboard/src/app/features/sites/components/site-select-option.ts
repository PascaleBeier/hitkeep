import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import { SiteFavicon, SiteFaviconSource } from '@features/sites/components/site-favicon';

@Component({
    selector: 'app-site-select-option',
    imports: [SiteFavicon],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <div class="site-select-option" [class.site-select-option--selected]="selected()">
            <app-site-favicon [site]="site()" />
            <span class="site-select-option__domain" [title]="domain()">{{ domain() }}</span>
        </div>
    `,
    styles: [
        `
            :host {
                display: block;
                width: 100%;
                min-width: 0;
                max-width: 100%;
            }

            .site-select-option {
                display: flex;
                width: 100%;
                min-width: 0;
                max-width: 100%;
                align-items: center;
                gap: 0.5rem;
            }

            .site-select-option__domain {
                flex: 1 1 auto;
                min-width: 0;
                overflow: hidden;
                text-overflow: ellipsis;
                white-space: nowrap;
            }

            .site-select-option--selected .site-select-option__domain {
                font-size: 0.875rem;
                font-weight: 600;
            }
        `
    ]
})
export class SiteSelectOption {
    readonly site = input.required<SiteFaviconSource | null>();
    readonly selected = input(false);
    protected readonly domain = computed(() => this.site()?.domain ?? '');
}
