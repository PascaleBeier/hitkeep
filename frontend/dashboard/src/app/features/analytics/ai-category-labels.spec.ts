import { TranslocoService } from '@jsverse/transloco';

import { AI_CATEGORY_ORDER, aiCategoryLabel, aiFilterChipLabel } from '@features/analytics/ai-category-labels';

function translocoStub(translations: Record<string, string>): TranslocoService {
    return {
        translate: (key: string) => translations[key] ?? key
    } as unknown as TranslocoService;
}

/** Stub that interpolates `{{ value }}` so chip wording can be asserted. */
function interpolatingTranslocoStub(translations: Record<string, string>): TranslocoService {
    return {
        translate: (key: string, params?: Record<string, unknown>) => (translations[key] ?? key).replace('{{value}}', String(params?.['value'] ?? ''))
    } as unknown as TranslocoService;
}

describe('ai category labels', () => {
    it('exposes the canonical AI category display order', () => {
        expect([...AI_CATEGORY_ORDER]).toEqual(['ai_training_crawler', 'ai_search_indexer', 'ai_assistant', 'ai_agent', 'ai_coding_agent', 'other_ai']);
    });

    it('translates a category token', () => {
        const transloco = translocoStub({ 'common.aiCategories.ai_assistant': 'AI assistants' });

        expect(aiCategoryLabel(transloco, 'ai_assistant')).toBe('AI assistants');
    });

    it('trims the token before looking up the translation', () => {
        const transloco = translocoStub({ 'common.aiCategories.other_ai': 'Other AI' });

        expect(aiCategoryLabel(transloco, '  other_ai  ')).toBe('Other AI');
    });

    it('falls back to the raw value when there is no translation', () => {
        const transloco = translocoStub({});

        expect(aiCategoryLabel(transloco, 'brand_new_category')).toBe('brand_new_category');
        expect(aiCategoryLabel(transloco, '  spaced  ')).toBe('  spaced  ');
    });

    it('provides a translation for every category in the canonical order', () => {
        const transloco = translocoStub(Object.fromEntries(AI_CATEGORY_ORDER.map((category) => [`common.aiCategories.${category}`, `label:${category}`])));

        expect(AI_CATEGORY_ORDER.map((category) => aiCategoryLabel(transloco, category))).toEqual(AI_CATEGORY_ORDER.map((category) => `label:${category}`));
    });
});

describe('aiFilterChipLabel', () => {
    const chipTranslations = {
        'common.filters.aiBot': 'AI bot: {{value}}',
        'common.filters.aiBotCategory': 'AI category: {{value}}',
        'common.filters.aiSource': 'AI source: {{value}}',
        'common.aiCategories.ai_training_crawler': 'Training crawlers'
    };

    it('labels an AI bot filter with its raw value', () => {
        const transloco = interpolatingTranslocoStub(chipTranslations);

        expect(aiFilterChipLabel(transloco, 'ai_bot', 'GPTBot')).toBe('AI bot: GPTBot');
    });

    it('translates the category token before labelling a category filter', () => {
        const transloco = interpolatingTranslocoStub(chipTranslations);

        expect(aiFilterChipLabel(transloco, 'ai_bot_category', 'ai_training_crawler')).toBe('AI category: Training crawlers');
    });

    it('keeps an untranslated category token visible', () => {
        const transloco = interpolatingTranslocoStub(chipTranslations);

        expect(aiFilterChipLabel(transloco, 'ai_bot_category', 'brand_new_category')).toBe('AI category: brand_new_category');
    });

    it('labels an AI source filter with its raw value', () => {
        const transloco = interpolatingTranslocoStub(chipTranslations);

        expect(aiFilterChipLabel(transloco, 'ai_source', 'ChatGPT')).toBe('AI source: ChatGPT');
    });
});
