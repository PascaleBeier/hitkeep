import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { PopoverModule } from '@openng/optimus-ui/popover';
import { TagModule } from '@openng/optimus-ui/tag';

export type SiteScopeSummarySeverity = 'success' | 'info' | 'warn' | 'danger' | 'secondary' | 'contrast';

export interface SiteScopeSummaryItem {
    id: string;
    label: string;
    detail?: string;
    severity?: SiteScopeSummarySeverity;
}

@Component({
    selector: 'app-site-scope-summary',
    imports: [ButtonModule, PopoverModule, TagModule],
    templateUrl: './site-scope-summary.html',
    styleUrl: './site-scope-summary.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteScopeSummary {
    readonly items = input.required<readonly SiteScopeSummaryItem[]>();
    readonly countLabel = input.required<string>();
    readonly popoverTitle = input.required<string>();
    readonly emptyLabel = input('');
    readonly emptyIcon = input('pi pi-ban');

    protected readonly singleItem = computed(() => (this.items().length === 1 ? this.items()[0] : undefined));

    protected itemLabel(item: SiteScopeSummaryItem): string {
        return item.detail ? `${item.label} · ${item.detail}` : item.label;
    }
}
