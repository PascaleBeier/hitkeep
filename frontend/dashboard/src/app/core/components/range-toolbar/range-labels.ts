import { type RangeValue } from './range-options';

interface RangeLabelTranslator {
    translate(key: string, params?: Record<string, unknown>): string;
}

interface TranslationRangeLabel {
    type: 'translation';
    labelKey: string;
    shortKey?: string;
    fallbackShort?: string;
}

interface RollingRangeLabel {
    type: 'rolling';
    labelKey: string;
    singularKey?: string;
    count: number;
    shortLabel: string;
}

type RangeLabelSpec = TranslationRangeLabel | RollingRangeLabel;

const RANGE_LABEL_SPECS: Record<RangeValue, RangeLabelSpec> = {
    today: {
        type: 'translation',
        labelKey: 'common.timeRanges.today',
        shortKey: 'common.timeRanges.todayShort'
    },
    yesterday: {
        type: 'translation',
        labelKey: 'common.timeRanges.yesterday',
        shortKey: 'common.timeRanges.yesterdayShort'
    },
    '30m': {
        type: 'rolling',
        labelKey: 'common.timeRanges.lastMinutes',
        count: 30,
        shortLabel: '30m'
    },
    '1h': {
        type: 'rolling',
        labelKey: 'common.timeRanges.lastHours',
        singularKey: 'common.timeRanges.lastHour',
        count: 1,
        shortLabel: '1h'
    },
    '6h': {
        type: 'rolling',
        labelKey: 'common.timeRanges.lastHours',
        count: 6,
        shortLabel: '6h'
    },
    '24h': {
        type: 'rolling',
        labelKey: 'common.timeRanges.lastHours',
        count: 24,
        shortLabel: '24h'
    },
    '3d': {
        type: 'rolling',
        labelKey: 'common.timeRanges.lastDays',
        count: 3,
        shortLabel: '3d'
    },
    '7d': {
        type: 'rolling',
        labelKey: 'common.timeRanges.lastDays',
        count: 7,
        shortLabel: '7d'
    },
    '14d': {
        type: 'rolling',
        labelKey: 'common.timeRanges.lastDays',
        count: 14,
        shortLabel: '14d'
    },
    '30d': {
        type: 'rolling',
        labelKey: 'common.timeRanges.lastDays',
        count: 30,
        shortLabel: '30d'
    },
    '60d': {
        type: 'rolling',
        labelKey: 'common.timeRanges.lastDays',
        count: 60,
        shortLabel: '60d'
    },
    '90d': {
        type: 'rolling',
        labelKey: 'common.timeRanges.lastDays',
        count: 90,
        shortLabel: '90d'
    },
    '180d': {
        type: 'rolling',
        labelKey: 'common.timeRanges.lastDays',
        count: 180,
        shortLabel: '180d'
    },
    thisWeek: {
        type: 'translation',
        labelKey: 'common.timeRanges.thisWeek',
        shortKey: 'common.timeRanges.thisWeekShort'
    },
    lastWeek: {
        type: 'translation',
        labelKey: 'common.timeRanges.lastWeek',
        shortKey: 'common.timeRanges.lastWeekShort'
    },
    thisMonth: {
        type: 'translation',
        labelKey: 'common.timeRanges.thisMonth',
        shortKey: 'common.timeRanges.thisMonthShort'
    },
    lastMonth: {
        type: 'translation',
        labelKey: 'common.timeRanges.lastMonth',
        shortKey: 'common.timeRanges.lastMonthShort'
    },
    thisYear: {
        type: 'translation',
        labelKey: 'common.timeRanges.thisYear',
        shortKey: 'common.timeRanges.thisYearShort'
    },
    '1y': {
        type: 'translation',
        labelKey: 'common.timeRanges.lastYear',
        shortKey: 'common.timeRanges.lastYearShort',
        fallbackShort: '1y'
    },
    custom: {
        type: 'translation',
        labelKey: 'common.timeRanges.customRange',
        shortKey: 'common.timeRanges.customShort'
    }
};

export function translateRangeLabel(translator: RangeLabelTranslator, value: RangeValue): string {
    const spec = RANGE_LABEL_SPECS[value];
    if (spec.type === 'rolling') {
        const labelKey = spec.count === 1 && spec.singularKey ? spec.singularKey : spec.labelKey;
        return translator.translate(labelKey, { count: spec.count });
    }

    return translator.translate(spec.labelKey);
}

export function translateRangeShortLabel(translator: RangeLabelTranslator, value: RangeValue): string {
    const spec = RANGE_LABEL_SPECS[value];
    if (spec.type === 'rolling') {
        return spec.shortLabel;
    }

    const key = spec.shortKey ?? spec.labelKey;
    const translation = translator.translate(key);
    return translation === key ? (spec.fallbackShort ?? translateRangeLabel(translator, value)) : translation;
}
