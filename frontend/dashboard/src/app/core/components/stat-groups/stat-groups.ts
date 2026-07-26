import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';

/**
 * One label/value pair of a strip. Values arrive pre-formatted for the active
 * locale, so the template renders them verbatim.
 */
export interface StatEntry {
    label: string;
    value: string;
}

/**
 * One themed strip. Splitting scalars by what they answer lets a reader
 * cross-reference within a theme instead of scanning one undifferentiated row.
 *
 * `isLoading` sits on the strip rather than the whole block because that is
 * where it varies: a card may mix reports that arrive in separate requests.
 */
export interface StatGroup {
    id: string;
    label: string;
    isLoading: boolean;
    stats: StatEntry[];
}

/**
 * A grid of themed label/value strips, for the dense scalar summaries that sit
 * inside an analytics card. Colors come from the shared `tile` design tokens,
 * so this renders correctly in both schemes without a per-scheme selector.
 */
@Component({
    selector: 'app-stat-groups',
    imports: [SkeletonModule],
    template: `
        <div class="stat-groups">
            @for (group of groups(); track group.id) {
                <section [attr.data-testid]="testIdPrefix() ? testIdPrefix() + group.id : null">
                    <h3 class="stat-groups__heading">{{ group.label }}</h3>
                    <dl class="stat-groups__strip">
                        @for (stat of group.stats; track stat.label) {
                            <div class="stat-groups__tile">
                                <dt [attr.title]="stat.label">{{ stat.label }}</dt>
                                <dd>
                                    @if (group.isLoading) {
                                        <!-- Height matches the dd's line box, so the tile does not grow when the value lands. -->
                                        <p-skeleton width="4rem" height="1.25rem" borderRadius="999px" />
                                    } @else {
                                        {{ stat.value }}
                                    }
                                </dd>
                            </div>
                        }
                    </dl>
                </section>
            }
        </div>
    `,
    styleUrl: './stat-groups.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class StatGroups {
    groups = input.required<StatGroup[]>();
    /** Suffixed with each group's ID, so a page can address one strip in tests. */
    testIdPrefix = input('');
}
