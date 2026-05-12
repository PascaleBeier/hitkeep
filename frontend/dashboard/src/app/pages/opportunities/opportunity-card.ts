import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { ButtonModule } from 'primeng/button';
import { TagModule } from 'primeng/tag';
import { OpportunityView } from './opportunity-view';

@Component({
    selector: 'app-opportunity-card',
    imports: [ButtonModule, TagModule],
    templateUrl: './opportunity-card.html',
    styleUrl: './opportunity-card.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class OpportunityCard {
    opportunity = input.required<OpportunityView>();
    canManage = input(false);
    pending = input(false);
    priorityLabel = input.required<string>();
    saveLabel = input.required<string>();
    inspectLabel = input.required<string>();

    saveClicked = output<OpportunityView>();
    inspectClicked = output<OpportunityView>();
}
