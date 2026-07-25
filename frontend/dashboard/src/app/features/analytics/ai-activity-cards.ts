import { TranslocoService } from '@jsverse/transloco';

import { AI_CATEGORY_ORDER, aiCategoryLabel } from '@features/analytics/ai-category-labels';
import type { MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import type { AIActivityReport } from '@models/analytics.types';

/** Dimensions an AI activity row can chip on the AI Agents page. */
export type AIActivityFilterType = 'ai_bot' | 'ai_bot_category' | 'ai_source' | 'path';

/**
 * Every metric card group of the AI Agents page, built from the one unified
 * report. Three groups in a fixed order:
 *
 * - `ai-activity`: the merged breakdowns, each row a filter toggle.
 * - `by-category`: the same agents split by bot category, still chipping `ai_bot`.
 * - `fetch-depth`: what only the forwarded logs know. Read-only, because those
 *   dimensions have no tracked-side counterpart to filter the page by, and empty
 *   until logs actually contribute — an empty group renders nothing at all.
 */
export function buildAIActivityCardGroups(
    transloco: TranslocoService,
    report: AIActivityReport | null,
    loading: boolean,
    activeValueFor: (type: AIActivityFilterType) => string | null,
    siteDomain: string | null = null
): MetricCardGroupTab<AIActivityFilterType>[] {
    const filterable = (type: AIActivityFilterType) => ({
        isRowClickable: true,
        activeValue: activeValueFor(type),
        filterType: type,
        showProvenance: true
    });

    return [
        {
            id: 'ai-activity',
            label: transloco.translate('aiAgents.groups.activity'),
            icon: 'pi-sparkles',
            cards: [
                {
                    id: 'agents',
                    title: transloco.translate('common.metrics.aiBots'),
                    icon: 'pi-sparkles',
                    data: report?.top_agents ?? [],
                    isLoading: loading,
                    aiIconKind: 'agent',
                    ...filterable('ai_bot')
                },
                {
                    id: 'categories',
                    title: transloco.translate('common.metrics.aiBotCategories'),
                    icon: 'pi-tags',
                    data: report?.top_categories ?? [],
                    isLoading: loading,
                    showAICategoryNames: true,
                    ...filterable('ai_bot_category')
                },
                {
                    id: 'paths',
                    title: transloco.translate('aiAgents.cards.paths'),
                    icon: 'pi-file',
                    data: report?.top_paths ?? [],
                    isLoading: loading,
                    linkMode: 'path',
                    siteDomain,
                    ...filterable('path')
                },
                {
                    id: 'sources',
                    title: transloco.translate('common.metrics.aiSources'),
                    icon: 'pi-comments',
                    data: report?.top_sources ?? [],
                    isLoading: loading,
                    aiIconKind: 'source',
                    ...filterable('ai_source')
                }
            ]
        },
        {
            id: 'by-category',
            label: transloco.translate('aiAgents.groups.byCategory'),
            icon: 'pi-android',
            // While the report loads every category stays visible to avoid layout
            // shift; afterwards only the ones with traffic keep a card.
            cards: AI_CATEGORY_ORDER.map((category) => ({
                id: `category-${category}`,
                title: aiCategoryLabel(transloco, category),
                icon: 'pi-sparkles',
                data: report?.top_agents_by_category?.[category] ?? [],
                isLoading: loading,
                aiIconKind: 'agent' as const,
                ...filterable('ai_bot')
            })).filter((card) => loading || card.data.length > 0)
        },
        {
            id: 'fetch-depth',
            label: transloco.translate('aiAgents.groups.fetchDepth'),
            icon: 'pi-cloud-upload',
            cards:
                (report?.fetch_count ?? 0) > 0
                    ? [
                          {
                              id: 'families',
                              title: transloco.translate('aiAgents.cards.families'),
                              icon: 'pi-sitemap',
                              data: report?.top_families ?? [],
                              isLoading: loading,
                              isRowClickable: false,
                              showProvenance: true
                          },
                          {
                              id: 'resource-types',
                              title: transloco.translate('aiAgents.cards.resourceTypes'),
                              icon: 'pi-database',
                              data: report?.top_resource_types ?? [],
                              isLoading: loading,
                              isRowClickable: false,
                              showProvenance: true
                          },
                          {
                              id: 'error-paths',
                              title: transloco.translate('aiAgents.cards.errorPaths'),
                              icon: 'pi-exclamation-triangle',
                              data: report?.top_error_paths ?? [],
                              isLoading: loading,
                              linkMode: 'path' as const,
                              siteDomain,
                              isRowClickable: false,
                              showProvenance: true
                          }
                      ]
                    : []
        }
    ];
}
