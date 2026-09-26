import { DOCUMENT, NgOptimizedImage } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';

@Component({
    selector: 'app-brand',
    standalone: true,
    imports: [NgOptimizedImage, RouterLink],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <a class="flex items-center select-none no-underline" routerLink="/">
            <img [ngSrc]="logoUrl()" alt="HitKeep Analytics" class="object-contain" [class]="imgClass()" [width]="imgWidth()" [height]="imgSize()" priority />
        </a>
    `
})
export class Brand {
    private document = inject(DOCUMENT);
    size = input<'small' | 'large'>('small');

    private static readonly aspect = 209.03 / 48.207;

    protected logoUrl = computed(() => browserAppUrl(this.document, '/brand-logo.svg'));

    protected imgSize = computed(() => {
        return this.size() === 'large' ? 48 : 32;
    });

    protected imgWidth = computed(() => {
        return Math.round(this.imgSize() * Brand.aspect);
    });

    protected imgClass = computed(() => {
        return this.size() === 'large' ? 'h-12 w-auto' : 'h-8 w-auto';
    });
}
