import { TranslocoService } from '@jsverse/transloco';

/** Canonical display order for AI bot categories across analytics surfaces. */
export const AI_CATEGORY_ORDER = ['ai_training_crawler', 'ai_search_indexer', 'ai_assistant', 'ai_agent', 'ai_coding_agent', 'other_ai'] as const;

export type AICategory = (typeof AI_CATEGORY_ORDER)[number];

/** Translated label for an AI bot category; unknown categories keep their raw value. */
export function aiCategoryLabel(transloco: TranslocoService, value: string): string {
    const key = `common.aiCategories.${value.trim()}`;
    const translated = transloco.translate(key);
    return translated === key ? value : translated;
}

/** The AI filter types every analytics surface can chip. */
export type AIFilterType = 'ai_bot' | 'ai_bot_category' | 'ai_source';

const AI_FILTER_LABEL_KEYS: Record<AIFilterType, string> = {
    ai_bot: 'common.filters.aiBot',
    ai_bot_category: 'common.filters.aiBotCategory',
    ai_source: 'common.filters.aiSource'
};

/**
 * Chip label for an active AI filter. Category values are translated through
 * `aiCategoryLabel` first, so a chip never shows a raw `ai_*` token, and every
 * surface that chips AI filters reads the same wording from one place.
 */
export function aiFilterChipLabel(transloco: TranslocoService, type: AIFilterType, value: string): string {
    return transloco.translate(AI_FILTER_LABEL_KEYS[type], {
        value: type === 'ai_bot_category' ? aiCategoryLabel(transloco, value) : value
    });
}
