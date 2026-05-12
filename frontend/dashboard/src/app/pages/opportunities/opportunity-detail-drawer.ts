import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { ButtonModule } from 'primeng/button';
import { DrawerModule } from 'primeng/drawer';
import { MessageModule } from 'primeng/message';
import { TagModule } from 'primeng/tag';
import { OpportunityView } from './opportunity-view';

@Component({
    selector: 'app-opportunity-detail-drawer',
    imports: [ButtonModule, DrawerModule, MessageModule, TagModule],
    templateUrl: './opportunity-detail-drawer.html',
    styleUrl: './opportunity-detail-drawer.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class OpportunityDetailDrawer {
    visible = input(false);
    opportunity = input<OpportunityView | null>(null);
    canManage = input(false);
    pending = input(false);
    estimatedImpactLabel = input.required<string>();
    priorityScoreLabel = input.required<string>();
    priorityHintLabel = input.required<string>();
    whyLabel = input.required<string>();
    nextBestActionLabel = input.required<string>();
    markDoneLabel = input.required<string>();
    saveLabel = input.required<string>();
    dismissLabel = input.required<string>();
    readOnlyLabel = input.required<string>();

    visibleChange = output<boolean>();
    markDoneClicked = output<OpportunityView>();
    saveClicked = output<OpportunityView>();
    dismissClicked = output<OpportunityView>();
}
