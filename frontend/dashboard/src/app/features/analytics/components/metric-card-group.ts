import { ChangeDetectionStrategy, Component, computed, input, output, signal } from '@angular/core';
import { CardModule } from '@openng/optimus-ui/card';
import { ButtonModule } from '@openng/optimus-ui/button';
import { TabsModule } from '@openng/optimus-ui/tabs';
import { MetricList, MetricListItem } from './metric-list';

export interface MetricCardConfig<TFilter extends string = string> {
    id: string;
    title: string;
    icon?: string;
    data: MetricListItem[];
    isLoading?: boolean;
    linkMode?: 'none' | 'path' | 'url' | 'details';
    siteDomain?: string | null;
    isRowClickable?: boolean;
    activeValue?: string | null;
    showBrowserIcons?: boolean;
    showCountryFlags?: boolean;
    showCountryNames?: boolean;
    showLanguageFlags?: boolean;
    showLanguageNames?: boolean;
    showAICategoryNames?: boolean;
    aiIconKind?: 'agent' | 'source';
    /** Annotates AI activity rows with their tracked/log provenance split. */
    showProvenance?: boolean;
    /** Share-of-total column; off for rows that are not parts of one whole. */
    showShare?: boolean;
    filterType?: TFilter;
    actionId?: string;
    actionLabel?: string;
    actionAriaLabel?: string;
    actionIcon?: string;
}

export interface MetricCardGroupTab<TFilter extends string = string> {
    id: string;
    label: string;
    icon?: string;
    cards: MetricCardConfig<TFilter>[];
}

export interface MetricCardGroupRowClick<TFilter extends string = string> {
    tabId: string;
    cardId: string;
    filterType: TFilter;
    metric: MetricListItem;
}

export interface MetricCardGroupAction {
    tabId: string;
    cardId: string;
    actionId: string;
}

@Component({
    selector: 'app-metric-card-group',
    imports: [ButtonModule, CardModule, TabsModule, MetricList],
    templateUrl: './metric-card-group.html',
    styleUrl: './metric-card-group.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class MetricCardGroup {
    tabs = input.required<MetricCardGroupTab[]>();

    rowClicked = output<MetricCardGroupRowClick>();
    actionClicked = output<MetricCardGroupAction>();

    protected readonly requestedCards = signal<Record<string, string>>({});
    protected readonly visibleGroups = computed(() => this.tabs().filter((tab) => tab.cards.length > 0));

    protected activeCardValue(group: MetricCardGroupTab): string {
        const requested = this.requestedCards()[group.id];
        if (requested && group.cards.some((card) => card.id === requested)) {
            return requested;
        }
        return group.cards[0]?.id ?? '';
    }

    protected activeCard(group: MetricCardGroupTab): MetricCardConfig | null {
        return group.cards.find((card) => card.id === this.activeCardValue(group)) ?? group.cards[0] ?? null;
    }

    protected setActiveCard(groupId: string, value: string | number | undefined): void {
        if (value === undefined) return;
        this.requestedCards.update((cards) => ({ ...cards, [groupId]: String(value) }));
    }

    protected cardHeading(group: MetricCardGroupTab): string {
        return group.label;
    }

    protected handleRowClick(tab: MetricCardGroupTab, card: MetricCardConfig, metric: MetricListItem): void {
        if (!card.filterType) return;
        this.rowClicked.emit({
            tabId: tab.id,
            cardId: card.id,
            filterType: card.filterType,
            metric
        });
    }

    protected handleAction(tab: MetricCardGroupTab, card: MetricCardConfig): void {
        if (!card.actionId) return;
        this.actionClicked.emit({ tabId: tab.id, cardId: card.id, actionId: card.actionId });
    }
}
