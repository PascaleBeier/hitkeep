import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';

/**
 * Centred placeholder shown on analytics pages while no site is selected.
 * The headline is shared; each page supplies its own icon and description key.
 */
@Component({
    selector: 'app-no-site-selected',
    imports: [TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <div class="flex flex-col items-center justify-center h-[50vh] gap-4">
            <i [class]="iconClass()"></i>
            <h2 class="text-2xl font-semibold text-muted-color">{{ 'common.noSiteSelected' | transloco }}</h2>
            <p class="text-muted-color">{{ descriptionKey() | transloco }}</p>
        </div>
    `
})
export class NoSiteSelected {
    /** Icon class, e.g. `pi pi-sparkles`. */
    icon = input<string>('pi pi-chart-bar');
    /** Translation key for the page specific description line. */
    descriptionKey = input.required<string>();

    protected readonly iconClass = computed(() => `${this.icon()} text-6xl text-primary opacity-20`);
}
