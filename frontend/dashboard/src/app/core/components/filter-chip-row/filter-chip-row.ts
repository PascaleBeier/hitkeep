import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';

/** One active filter, rendered as a removable chip. */
export interface FilterChipItem {
    /** Stable identity for `@for` tracking. */
    key: string;
    /** Already translated chip caption, e.g. `City: Berlin`. */
    label: string;
    /** Drops just this filter. */
    remove: () => void;
}

/** Row of active filter chips with a trailing "clear all" action. */
@Component({
    selector: 'app-filter-chip-row',
    imports: [TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    host: { class: 'flex flex-wrap items-center gap-2' },
    template: `
        @if (chips().length > 0) {
            @for (chip of chips(); track chip.key) {
                <span class="inline-flex items-center gap-2 px-3 py-1 rounded-full border border-[var(--p-primary-color)] text-sm">
                    <span class="font-medium text-[var(--p-text-color)]">{{ chip.label }}</span>
                    <button type="button" class="text-muted-color hover:text-[var(--p-text-color)]" (click)="chip.remove()" [attr.aria-label]="'common.removeFilterAria' | transloco">
                        <i class="pi pi-times" aria-hidden="true"></i>
                    </button>
                </span>
            }
            <button type="button" class="text-xs text-muted-color hover:text-[var(--p-text-color)] underline" (click)="clearAll.emit()">{{ 'common.actions.clearAll' | transloco }}</button>
        } @else {
            <span class="text-sm text-[var(--p-text-muted-color)]">{{ 'common.noActiveFilter' | transloco }}</span>
        }
    `
})
export class FilterChipRow {
    chips = input.required<readonly FilterChipItem[]>();
    clearAll = output<void>();
}
