import { TranslocoService } from '@jsverse/transloco';

import { AI_CATEGORY_ORDER, aiCategoryLabel } from '@features/analytics/ai-category-labels';
import type { MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import type { AIActivityReport, MetricStat } from '@models/analytics.types';

/** Dimensions an AI activity row can chip on the AI Agents page. */
export type AIActivityFilterType = 'ai_bot' | 'ai_bot_category' | 'ai_source' | 'path';

/**
 * Correlation rows already shaped for a metric list. The caller maps them once
 * per correlation report so their array identity survives this builder's other
 * recomputes — `MetricList` re-observes its frame whenever `data` changes.
 */
export interface AICorrelationRows {
    citationPaths: MetricStat[];
    opportunityPages: MetricStat[];
    failureHotspots: MetricStat[];
}

export interface AIActivityCardGroupsInput {
    transloco: TranslocoService;
    report: AIActivityReport | null;
    isLoading: boolean;
    activeValueFor: (type: AIActivityFilterType) => string | null;
    siteDomain?: string | null;
    /** The fetch-only correlation, or `null` wherever its endpoint is out of reach. */
    correlation?: { rows: AICorrelationRows | null; isLoading: boolean } | null;
}

/**
 * Every metric card group of the AI Agents page. The first three are built from
 * the one unified report, the fourth from the fetch-only correlation, so the
 * whole page renders a single card grid. Fixed order:
 *
 * - `ai-activity`: the merged breakdowns, each row a filter toggle.
 * - `by-category`: the same agents split by bot category, still chipping `ai_bot`.
 * - `fetch-depth`: what only the forwarded logs know. Read-only, because those
 *   dimensions have no tracked-side counterpart to filter the page by, and empty
 *   until logs actually contribute — an empty group renders nothing at all.
 * - `correlation`: which fetched paths later earned AI-referred visits. Empty
 *   wherever its own endpoint is out of reach.
 */
export function buildAIActivityCardGroups({ transloco, report, isLoading, activeValueFor, siteDomain = null, correlation = null }: AIActivityCardGroupsInput): MetricCardGroupTab<AIActivityFilterType>[] {
    const filterable = (type: AIActivityFilterType) => ({
        isRowClickable: true,
        activeValue: activeValueFor(type),
        filterType: type,
        showProvenance: true
    });
    // Correlation rows are not parts of one whole, so share-of-total stays off.
    const correlationCard = { isLoading: correlation?.isLoading ?? false, showShare: false };
    const correlationPathCard = { ...correlationCard, linkMode: 'path' as const, siteDomain, isRowClickable: true, activeValue: activeValueFor('path'), filterType: 'path' as const };
    const tableTitle = (key: string) => transloco.translate(`aiAgents.fetchDepth.tables.${key}.title`);

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
                    isLoading,
                    aiIconKind: 'agent',
                    ...filterable('ai_bot')
                },
                {
                    id: 'categories',
                    title: transloco.translate('common.metrics.aiBotCategories'),
                    icon: 'pi-tags',
                    data: report?.top_categories ?? [],
                    isLoading,
                    showAICategoryNames: true,
                    ...filterable('ai_bot_category')
                },
                {
                    id: 'paths',
                    title: transloco.translate('aiAgents.cards.paths'),
                    icon: 'pi-file',
                    data: report?.top_paths ?? [],
                    isLoading,
                    linkMode: 'path',
                    siteDomain,
                    ...filterable('path')
                },
                {
                    id: 'sources',
                    title: transloco.translate('common.metrics.aiSources'),
                    icon: 'pi-comments',
                    data: report?.top_sources ?? [],
                    isLoading,
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
                isLoading,
                aiIconKind: 'agent' as const,
                ...filterable('ai_bot')
            })).filter((card) => isLoading || card.data.length > 0)
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
                              isLoading,
                              isRowClickable: false,
                              showProvenance: true
                          },
                          {
                              id: 'resource-types',
                              title: transloco.translate('aiAgents.cards.resourceTypes'),
                              icon: 'pi-database',
                              data: report?.top_resource_types ?? [],
                              isLoading,
                              isRowClickable: false,
                              showProvenance: true
                          },
                          {
                              id: 'error-paths',
                              title: transloco.translate('aiAgents.cards.errorPaths'),
                              icon: 'pi-exclamation-triangle',
                              data: report?.top_error_paths ?? [],
                              isLoading,
                              linkMode: 'path' as const,
                              siteDomain,
                              isRowClickable: false,
                              showProvenance: true
                          }
                      ]
                    : []
        },
        {
            id: 'correlation',
            label: transloco.translate('aiAgents.fetchDepth.tables.title'),
            // Absent wherever the fetch-only request is: a share token and every
            // site without forwarded logs render no card here.
            cards: correlation
                ? [
                      {
                          // `citationPaths` is the server-side per-path section: distinct
                          // counts over the whole path, which the per-(path, assistant)
                          // citation-yield rows cannot be re-aggregated into.
                          id: 'citation-yield',
                          title: tableTitle('citationYield'),
                          icon: 'pi-link',
                          data: correlation.rows?.citationPaths ?? [],
                          ...correlationPathCard
                      },
                      {
                          id: 'opportunity-pages',
                          title: tableTitle('opportunityPages'),
                          icon: 'pi-bolt',
                          data: correlation.rows?.opportunityPages ?? [],
                          ...correlationPathCard
                      },
                      {
                          // Read-only: the row names an assistant and a path prefix at once,
                          // so neither page filter would reproduce what the row shows — and
                          // the label has to say both or two prefixes read as one row twice.
                          id: 'failure-hotspots',
                          title: tableTitle('failureHotspots'),
                          icon: 'pi-exclamation-triangle',
                          data: correlation.rows?.failureHotspots ?? [],
                          isRowClickable: false,
                          ...correlationCard
                      }
                  ]
                : []
        }
    ];
}
