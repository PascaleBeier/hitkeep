import { ChangeDetectionStrategy, Component, computed, inject, input, output } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import type { Annotation } from '@models/analytics.types';
import { formatAnnotationWhen } from '@features/annotations/annotation-dates';
import type { AnnotationGranularity } from '@features/annotations/site-annotations.service';

/**
 * The keyboard and screen-reader way into a chart's notes: one button per
 * note in view, in time order. Pointer readers also get markers on the chart.
 */
@Component({
    selector: 'app-annotation-strip',
    changeDetection: ChangeDetectionStrategy.OnPush,
    imports: [TranslocoPipe],
    template: `
        @if (items().length) {
            <ul class="m-0 mt-3 flex list-none flex-wrap gap-2 p-0" [attr.aria-label]="'annotations.strip.label' | transloco">
                @for (item of items(); track item.annotation.id) {
                    <li class="min-w-0">
                        <button type="button" class="annotation-chip" [attr.aria-label]="item.ariaLabel" (click)="selected.emit(item.annotation)">
                            <span class="annotation-chip__mark" [class.annotation-chip__mark--range]="!!item.annotation.ends_at" aria-hidden="true"></span>
                            <span class="shrink-0 font-medium tabular-nums">{{ item.when }}</span>
                            <span class="truncate text-[var(--p-text-muted-color)]">{{ item.annotation.body }}</span>
                        </button>
                    </li>
                }
            </ul>
        }
    `,
    styles: `
        .annotation-chip {
            display: inline-flex;
            max-width: 18rem;
            align-items: center;
            gap: 0.4rem;
            border: 1px solid var(--p-content-border-color);
            border-radius: 999px;
            background: var(--p-content-background);
            padding: 0.25rem 0.65rem;
            font-size: 0.75rem;
            line-height: 1.25rem;
            color: var(--p-text-color);
            cursor: pointer;
            transition:
                border-color 120ms ease,
                background-color 120ms ease;
        }
        .annotation-chip:hover {
            border-color: var(--p-amber-600);
            background: color-mix(in srgb, var(--p-amber-500) 8%, var(--p-content-background));
        }
        .annotation-chip:focus-visible {
            outline: 2px solid var(--p-focus-ring-color, var(--p-primary-color));
            outline-offset: 2px;
        }
        .annotation-chip__mark {
            width: 0.5rem;
            height: 0.5rem;
            flex-shrink: 0;
            border-radius: 999px;
            background: var(--p-amber-600);
        }
        .annotation-chip__mark--range {
            width: 0.85rem;
            border-radius: 3px;
            background: color-mix(in srgb, var(--p-amber-600) 35%, transparent);
            outline: 1px dashed var(--p-amber-600);
        }
        @media (prefers-reduced-motion: reduce) {
            .annotation-chip {
                transition: none;
            }
        }
    `
})
export class AnnotationStrip {
    readonly annotations = input.required<Annotation[]>();
    readonly granularity = input<AnnotationGranularity>('day');
    readonly selected = output<Annotation>();

    private readonly transloco = inject(TranslocoService);
    private readonly locale = inject(TranslocoLocaleService);
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    protected readonly items = computed(() => {
        this.activeLanguage();
        const locale = this.locale.getLocale();
        return this.annotations().map((annotation) => {
            const when = formatAnnotationWhen(annotation, this.granularity(), locale);
            return { annotation, when, ariaLabel: this.transloco.translate('annotations.strip.open', { when, body: annotation.body }) };
        });
    });
}
