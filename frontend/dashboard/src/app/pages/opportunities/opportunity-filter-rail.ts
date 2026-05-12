import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { OpportunityFilter, OpportunityFilterItem, StatusFilter } from './opportunity-view';

@Component({
    selector: 'app-opportunity-filter-rail',
    template: `
        <aside class="opportunities-sidebar" [attr.aria-label]="ariaLabel()">
            <section>
                <h2>{{ typeTitle() }}</h2>
                <div class="opportunities-filter-list">
                    @for (filter of typeItems(); track filter.value) {
                        <button type="button" [class]="filter.active ? 'opportunities-filter opportunities-filter--active' : 'opportunities-filter'" (click)="typeSelected.emit(filter.value)">
                            <span>{{ filter.label }}</span>
                            <strong>{{ filter.count }}</strong>
                        </button>
                    }
                </div>
            </section>

            <section>
                <h2>{{ statusTitle() }}</h2>
                <div class="opportunities-filter-list">
                    @for (filter of statusItems(); track filter.value) {
                        <button type="button" [class]="filter.active ? 'opportunities-filter opportunities-filter--active' : 'opportunities-filter'" (click)="statusSelected.emit(filter.value)">
                            <span>{{ filter.label }}</span>
                            <strong>{{ filter.count }}</strong>
                        </button>
                    }
                </div>
            </section>
        </aside>
    `,
    styleUrl: './opportunity-filter-rail.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class OpportunityFilterRail {
    ariaLabel = input.required<string>();
    typeTitle = input.required<string>();
    statusTitle = input.required<string>();
    typeItems = input.required<OpportunityFilterItem<OpportunityFilter>[]>();
    statusItems = input.required<OpportunityFilterItem<StatusFilter>[]>();

    typeSelected = output<OpportunityFilter>();
    statusSelected = output<StatusFilter>();
}
