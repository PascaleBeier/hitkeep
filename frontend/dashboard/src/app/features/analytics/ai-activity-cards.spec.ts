import { TestBed } from '@angular/core/testing';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';

import { buildAIActivityCardGroups } from '@features/analytics/ai-activity-cards';
import { aiActivityStat, emptyAIActivityReport } from '@testing/empty-ai-activity-report';

describe('buildAIActivityCardGroups', () => {
    let transloco: TranslocoService;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            aiAgents: {
                                groups: { activity: 'AI activity', byCategory: 'By category', fetchDepth: 'Fetch depth' },
                                cards: { paths: 'Pages crawled', families: 'Operators', resourceTypes: 'Resource types', errorPaths: 'Error paths' }
                            },
                            common: {
                                metrics: { aiBots: 'AI agents', aiBotCategories: 'Agent categories', aiSources: 'AI referrers' },
                                aiCategories: {
                                    ai_training_crawler: 'Training crawlers',
                                    ai_search_indexer: 'Search indexers',
                                    ai_assistant: 'Assistants',
                                    ai_agent: 'Agents',
                                    ai_coding_agent: 'Coding agents',
                                    other_ai: 'Other AI'
                                }
                            }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        transloco = TestBed.inject(TranslocoService);
    });

    const noFilters = () => null;

    it('builds the AI activity group as filter toggles over the merged breakdowns', () => {
        const report = emptyAIActivityReport({
            top_agents: [aiActivityStat('GPTBot', 10, 40)],
            top_categories: [aiActivityStat('ai_training_crawler', 10, 40)],
            top_paths: [aiActivityStat('/docs', 2, 8)],
            top_sources: [aiActivityStat('chatgpt.com', 4, 0)]
        });

        const activity = buildAIActivityCardGroups(transloco, report, false, noFilters, 'example.com').find((group) => group.id === 'ai-activity');

        expect(activity?.label).toBe('AI activity');
        const cards = activity?.cards ?? [];
        expect(cards.map((card) => card.id)).toEqual(['agents', 'categories', 'paths', 'sources']);
        expect(cards.map((card) => card.title)).toEqual(['AI agents', 'Agent categories', 'Pages crawled', 'AI referrers']);
        expect(cards.map((card) => card.filterType)).toEqual(['ai_bot', 'ai_bot_category', 'path', 'ai_source']);
        expect(cards.every((card) => card.isRowClickable === true)).toBe(true);
        expect(cards.every((card) => card.showProvenance === true)).toBe(true);
        expect(cards[0].aiIconKind).toBe('agent');
        expect(cards[0].data).toBe(report.top_agents);
        expect(cards.map((card) => card.data.length)).toEqual([1, 1, 1, 1]);
        expect(cards[1].showAICategoryNames).toBe(true);
        expect(cards[2].linkMode).toBe('path');
        expect(cards[2].siteDomain).toBe('example.com');
        expect(cards[3].aiIconKind).toBe('source');
    });

    it('reflects the active filter value per dimension', () => {
        const groups = buildAIActivityCardGroups(transloco, emptyAIActivityReport(), false, (type) => (type === 'ai_bot' ? 'GPTBot' : type === 'path' ? '/docs' : null));
        const cards = groups.find((group) => group.id === 'ai-activity')?.cards ?? [];

        expect(cards.map((card) => card.activeValue)).toEqual(['GPTBot', null, '/docs', null]);
    });

    it('renders one per-category card for every category with traffic', () => {
        const report = emptyAIActivityReport({
            top_agents_by_category: {
                ai_training_crawler: [aiActivityStat('GPTBot', 10, 40)],
                ai_assistant: []
            }
        });

        const byCategory = buildAIActivityCardGroups(transloco, report, false, noFilters).find((group) => group.id === 'by-category');

        expect(byCategory?.label).toBe('By category');
        expect(byCategory?.cards.map((card) => card.id)).toEqual(['category-ai_training_crawler']);
        expect(byCategory?.cards[0].title).toBe('Training crawlers');
        expect(byCategory?.cards[0].filterType).toBe('ai_bot');
        expect(byCategory?.cards[0].aiIconKind).toBe('agent');
    });

    it('keeps every per-category card visible while the report loads', () => {
        const byCategory = buildAIActivityCardGroups(transloco, null, true, noFilters).find((group) => group.id === 'by-category');

        expect(byCategory?.cards.map((card) => card.id)).toEqual(['category-ai_training_crawler', 'category-ai_search_indexer', 'category-ai_assistant', 'category-ai_agent', 'category-ai_coding_agent', 'category-other_ai']);
        expect(byCategory?.cards.every((card) => card.isLoading === true)).toBe(true);
    });

    it('adds the read-only fetch depth group once forwarded logs contribute', () => {
        const report = emptyAIActivityReport({
            fetch_count: 120,
            top_families: [aiActivityStat('OpenAI', 0, 120)],
            top_resource_types: [aiActivityStat('html', 0, 90)],
            top_error_paths: [aiActivityStat('/gone', 0, 4)]
        });

        const fetchDepth = buildAIActivityCardGroups(transloco, report, false, noFilters, 'example.com').find((group) => group.id === 'fetch-depth');

        expect(fetchDepth?.label).toBe('Fetch depth');
        expect(fetchDepth?.cards.map((card) => card.id)).toEqual(['families', 'resource-types', 'error-paths']);
        expect(fetchDepth?.cards.map((card) => card.title)).toEqual(['Operators', 'Resource types', 'Error paths']);
        expect(fetchDepth?.cards.every((card) => card.isRowClickable === false)).toBe(true);
        expect(fetchDepth?.cards.every((card) => card.filterType === undefined)).toBe(true);
        expect(fetchDepth?.cards[2].linkMode).toBe('path');
    });

    it('leaves the fetch depth group empty without any fetch data', () => {
        const report = emptyAIActivityReport({ fetch_count: 0, top_families: [aiActivityStat('OpenAI', 0, 0)] });

        const groups = buildAIActivityCardGroups(transloco, report, false, noFilters);

        expect(groups.map((group) => group.id)).toEqual(['ai-activity', 'by-category', 'fetch-depth']);
        expect(groups.find((group) => group.id === 'fetch-depth')?.cards).toEqual([]);
    });
});
