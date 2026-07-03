import { DOCUMENT, NgOptimizedImage } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { browserAppUrl, browserBasePath } from '@core/interceptors/base-path.interceptor';

@Component({
    selector: 'app-brand',
    standalone: true,
    imports: [NgOptimizedImage],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <a class="flex items-center gap-3 select-none no-underline" [href]="rootUrl()">
            <img [ngSrc]="iconUrl()" alt="HitKeep Logo" class="object-cover" [class]="imgClass()" [width]="imgSize()" [height]="imgSize()" priority />
            <span class="font-bold tracking-tight text-[var(--p-text-color)]" [class]="textClass()"> HitKeep </span>
        </a>
    `
})
export class Brand {
    private document = inject(DOCUMENT);
    size = input<'small' | 'large'>('small');

    protected iconUrl = computed(() => browserAppUrl(this.document, '/icon.png'));
    protected rootUrl = computed(() => browserBasePath(this.document));

    protected imgSize = computed(() => {
        return this.size() === 'large' ? 48 : 32;
    });

    protected imgClass = computed(() => {
        return this.size() === 'large' ? 'w-12 h-12' : 'w-8 h-8';
    });

    protected textClass = computed(() => {
        return this.size() === 'large' ? 'text-3xl' : 'text-xl';
    });
}
