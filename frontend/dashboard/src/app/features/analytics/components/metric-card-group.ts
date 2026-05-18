import { ChangeDetectionStrategy, Component, computed, input, output, signal } from '@angular/core';
import { CardModule } from 'primeng/card';
import { TabsModule } from 'primeng/tabs';
import { MetricStat } from '@models/analytics.types';
import { MetricList } from './metric-list';

export interface MetricCardConfig<TFilter extends string = string> {
    id: string;
    title: string;
    icon?: string;
    data: MetricStat[];
    isLoading?: boolean;
    linkMode?: 'none' | 'path' | 'url';
    siteDomain?: string | null;
    isRowClickable?: boolean;
    activeValue?: string | null;
    showBrowserIcons?: boolean;
    showCountryFlags?: boolean;
    showCountryNames?: boolean;
    showLanguageFlags?: boolean;
    showLanguageNames?: boolean;
    filterType?: TFilter;
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
    metric: MetricStat;
}

@Component({
    selector: 'app-metric-card-group',
    imports: [CardModule, TabsModule, MetricList],
    templateUrl: './metric-card-group.html',
    styleUrl: './metric-card-group.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class MetricCardGroup {
    tabs = input.required<MetricCardGroupTab[]>();

    rowClicked = output<MetricCardGroupRowClick>();

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

    protected handleRowClick(tab: MetricCardGroupTab, card: MetricCardConfig, metric: MetricStat): void {
        if (!card.filterType) return;
        this.rowClicked.emit({
            tabId: tab.id,
            cardId: card.id,
            filterType: card.filterType,
            metric
        });
    }
}
