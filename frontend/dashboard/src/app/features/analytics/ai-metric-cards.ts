import { TranslocoService } from '@jsverse/transloco';
import { AI_CATEGORY_ORDER, aiCategoryLabel } from '@features/analytics/ai-category-labels';
import type { MetricCardConfig, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import type { AIActivityReport, SiteStats } from '@models/analytics.types';

/** Filter dimensions the AI metric cards drive. */
export type AIMetricFilterType = 'ai_bot' | 'ai_bot_category' | 'ai_source';

/**
 * The `ai` and `bots` metric card groups of the dashboard. When the unified
 * report is available, the cards use its tracked-plus-fetch rows; the stats
 * breakdown remains a fallback while that report is loading or unavailable.
 */
export function buildAIMetricCardTabs(
    transloco: TranslocoService,
    stats: SiteStats | null,
    activity: AIActivityReport | null,
    statsLoading: boolean,
    activityLoading: boolean,
    activeValueFor: (type: AIMetricFilterType) => string | null
): MetricCardGroupTab<AIMetricFilterType>[] {
    const filterProps = (type: AIMetricFilterType): Pick<MetricCardConfig<AIMetricFilterType>, 'activeValue' | 'filterType'> => ({ activeValue: activeValueFor(type), filterType: type });
    const useActivity = activity !== null;
    const loading = activityLoading || (!useActivity && statsLoading);
    const topAgents = activity?.top_agents ?? stats?.top_ai_bots ?? [];
    const topCategories = activity?.top_categories ?? stats?.top_ai_bot_categories ?? [];
    const topSources = activity?.top_sources ?? stats?.top_ai_sources ?? [];
    const agentsByCategory = activity?.top_agents_by_category ?? stats?.top_ai_bots_by_category ?? {};

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
                    data: topAgents,
                    isLoading: loading,
                    isRowClickable: true,
                    aiIconKind: 'agent',
                    showProvenance: useActivity,
                    ...filterProps('ai_bot')
                },
                {
                    id: 'ai-bot-categories',
                    title: transloco.translate('common.metrics.aiBotCategories'),
                    icon: 'pi-tags',
                    data: topCategories,
                    isLoading: loading,
                    isRowClickable: true,
                    showAICategoryNames: true,
                    showProvenance: useActivity,
                    ...filterProps('ai_bot_category')
                },
                {
                    id: 'ai-sources',
                    title: transloco.translate('common.metrics.aiSources'),
                    icon: 'pi-comments',
                    data: topSources,
                    isLoading: loading,
                    isRowClickable: true,
                    aiIconKind: 'source',
                    showProvenance: useActivity,
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
                data: agentsByCategory[category] ?? [],
                isLoading: loading,
                isRowClickable: true,
                aiIconKind: 'agent' as const,
                showProvenance: useActivity,
                ...filterProps('ai_bot')
            })).filter((card) => loading || card.data.length > 0)
        }
    ];
}
