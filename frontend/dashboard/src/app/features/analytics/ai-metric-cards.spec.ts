import { TestBed } from '@angular/core/testing';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';

import { buildAIMetricCardTabs } from '@features/analytics/ai-metric-cards';
import { emptySiteStats } from '@testing/empty-site-stats';

describe('buildAIMetricCardTabs', () => {
    let transloco: TranslocoService;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                metricGroups: { ai: 'AI', bots: 'AI bots' },
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

    it('builds the AI group from the stats breakdowns', () => {
        const stats = emptySiteStats({
            top_ai_bots: [{ name: 'GPTBot', value: 12 }],
            top_ai_bot_categories: [{ name: 'ai_training_crawler', value: 12 }],
            top_ai_sources: [{ name: 'chatgpt.com', value: 4 }]
        });

        const cards = buildAIMetricCardTabs(transloco, stats, false, noFilters).find((tab) => tab.id === 'ai')?.cards ?? [];

        expect(cards.map((card) => card.id)).toEqual(['ai-bots', 'ai-bot-categories', 'ai-sources']);
        expect(cards.map((card) => card.title)).toEqual(['AI agents', 'Agent categories', 'AI referrers']);
        expect(cards.map((card) => card.data)).toEqual([[{ name: 'GPTBot', value: 12 }], [{ name: 'ai_training_crawler', value: 12 }], [{ name: 'chatgpt.com', value: 4 }]]);
        expect(cards[0].aiIconKind).toBe('agent');
        expect(cards[1].showAICategoryNames).toBe(true);
        expect(cards[2].aiIconKind).toBe('source');
    });

    it('wires filter types and the active value from the required lookup', () => {
        const tabs = buildAIMetricCardTabs(transloco, emptySiteStats({ top_ai_bots_by_category: { ai_agent: [{ name: 'ChatGPT-User', value: 3 }] } }), false, (type) => (type === 'ai_bot' ? 'GPTBot' : null));
        const aiCards = tabs.find((tab) => tab.id === 'ai')?.cards ?? [];
        const botCards = tabs.find((tab) => tab.id === 'bots')?.cards ?? [];

        expect(aiCards.map((card) => card.filterType)).toEqual(['ai_bot', 'ai_bot_category', 'ai_source']);
        expect(aiCards.map((card) => card.activeValue)).toEqual(['GPTBot', null, null]);
        expect(aiCards.every((card) => card.isRowClickable === true)).toBe(true);
        expect(botCards.map((card) => card.id)).toEqual(['bots-ai_agent']);
        expect(botCards[0].filterType).toBe('ai_bot');
        expect(botCards[0].activeValue).toBe('GPTBot');
    });

    it('keeps every category card visible while stats load and drops empty ones afterwards', () => {
        const stats = emptySiteStats({ top_ai_bots_by_category: { ai_search_indexer: [{ name: 'OAI-SearchBot', value: 2 }], ai_assistant: [] } });

        const loadingCards = buildAIMetricCardTabs(transloco, null, true, noFilters).find((tab) => tab.id === 'bots')?.cards ?? [];
        const settledCards = buildAIMetricCardTabs(transloco, stats, false, noFilters).find((tab) => tab.id === 'bots')?.cards ?? [];

        expect(loadingCards.map((card) => card.id)).toEqual(['bots-ai_training_crawler', 'bots-ai_search_indexer', 'bots-ai_assistant', 'bots-ai_agent', 'bots-ai_coding_agent', 'bots-other_ai']);
        expect(loadingCards.map((card) => card.title)).toEqual(['Training crawlers', 'Search indexers', 'Assistants', 'Agents', 'Coding agents', 'Other AI']);
        expect(settledCards.map((card) => card.id)).toEqual(['bots-ai_search_indexer']);
    });
});
