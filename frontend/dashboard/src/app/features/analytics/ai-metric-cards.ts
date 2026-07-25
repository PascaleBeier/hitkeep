import { TranslocoService } from '@jsverse/transloco';
import { AI_CATEGORY_ORDER, aiCategoryLabel } from '@features/analytics/ai-category-labels';
import type { MetricCardConfig, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import type { SiteStats } from '@models/analytics.types';

/** Filter dimensions the AI metric cards drive. */
export type AIMetricFilterType = 'ai_bot' | 'ai_bot_category' | 'ai_source';

/**
 * The `ai` and `bots` metric card groups of the dashboard, read from
 * `SiteStats`. The AI Agents page has its own richer builder over the unified
 * AI activity report; this one only serves the dashboard's stats-derived cards.
 */
export function buildAIMetricCardTabs(transloco: TranslocoService, stats: SiteStats | null, loading: boolean, activeValueFor: (type: AIMetricFilterType) => string | null): MetricCardGroupTab<AIMetricFilterType>[] {
    const filterProps = (type: AIMetricFilterType): Pick<MetricCardConfig<AIMetricFilterType>, 'activeValue' | 'filterType'> => ({ activeValue: activeValueFor(type), filterType: type });

    return [
        {
            id: 'ai',
            label: transloco.translate('common.metricGroups.ai'),
            icon: 'pi-sparkles',
            cards: [
                {
                    id: 'ai-bots',
                    title: transloco.translate('common.metrics.aiBots'),
                    icon: 'pi-sparkles',
                    data: stats?.top_ai_bots ?? [],
                    isLoading: loading,
                    isRowClickable: true,
                    aiIconKind: 'agent',
                    ...filterProps('ai_bot')
                },
                {
                    id: 'ai-bot-categories',
                    title: transloco.translate('common.metrics.aiBotCategories'),
                    icon: 'pi-tags',
                    data: stats?.top_ai_bot_categories ?? [],
                    isLoading: loading,
                    isRowClickable: true,
                    showAICategoryNames: true,
                    ...filterProps('ai_bot_category')
                },
                {
                    id: 'ai-sources',
                    title: transloco.translate('common.metrics.aiSources'),
                    icon: 'pi-comments',
                    data: stats?.top_ai_sources ?? [],
                    isLoading: loading,
                    isRowClickable: true,
                    aiIconKind: 'source',
                    ...filterProps('ai_source')
                }
            ]
        },
        {
            id: 'bots',
            label: transloco.translate('common.metricGroups.bots'),
            icon: 'pi-android',
            // Only categories with traffic get a card; while stats load, all of
            // them stay visible to avoid layout shift. A group without cards is
            // hidden entirely by the card group component.
            cards: AI_CATEGORY_ORDER.map((category) => ({
                id: `bots-${category}`,
                title: aiCategoryLabel(transloco, category),
                icon: 'pi-sparkles',
                data: stats?.top_ai_bots_by_category?.[category] ?? [],
                isLoading: loading,
                isRowClickable: true,
                aiIconKind: 'agent' as const,
                ...filterProps('ai_bot')
            })).filter((card) => loading || card.data.length > 0)
        }
    ];
}
