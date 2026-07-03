export type RangeValue = 'today' | 'yesterday' | '30m' | '1h' | '6h' | '24h' | '3d' | '7d' | '14d' | '30d' | '60d' | '90d' | '180d' | 'thisWeek' | 'lastWeek' | 'thisMonth' | 'lastMonth' | 'thisYear' | '1y' | 'custom';

export interface RangeOption {
    label?: string;
    value: RangeValue;
}

export interface DateRange {
    from: string;
    to: string;
}

export const DEFAULT_RANGE_VALUE: RangeValue = 'today';

export const DEFAULT_RANGE_OPTIONS: RangeOption[] = [
    { value: 'today' },
    { value: 'yesterday' },
    { value: '24h' },
    { value: '7d' },
    { value: '30d' },
    { value: '30m' },
    { value: '1h' },
    { value: '6h' },
    { value: '3d' },
    { value: '14d' },
    { value: '60d' },
    { value: '90d' },
    { value: '180d' },
    { value: 'thisWeek' },
    { value: 'lastWeek' },
    { value: 'thisMonth' },
    { value: 'lastMonth' },
    { value: 'thisYear' },
    { value: '1y' },
    { value: 'custom' }
];

const SHORT_RANGE_VALUES = new Set<RangeValue>(['today', 'yesterday', '30m', '1h', '6h', '24h']);

function startOfLocalDay(date: Date): Date {
    const start = new Date(date);
    start.setHours(0, 0, 0, 0);
    return start;
}

function endOfLocalDay(date: Date): Date {
    const end = new Date(date);
    end.setHours(23, 59, 59, 999);
    return end;
}

function startOfIsoWeek(date: Date): Date {
    const start = startOfLocalDay(date);
    start.setDate(start.getDate() - ((start.getDay() + 6) % 7));
    return start;
}

export function selectDefaultRange(ranges: readonly RangeOption[], previous?: RangeOption | null): RangeOption {
    const value = previous?.value ?? DEFAULT_RANGE_VALUE;
    return ranges.find((range) => range.value === value) ?? ranges.find((range) => range.value === DEFAULT_RANGE_VALUE) ?? ranges[0]!;
}

export function resolveDateRange(range: RangeOption, customRangeDates: Date[] | null = null, now = new Date()): DateRange | null {
    if (range.value === 'custom') {
        if (!customRangeDates || customRangeDates.length !== 2 || !customRangeDates[0] || !customRangeDates[1]) {
            return null;
        }

        const [from, to] = customRangeDates;
        if (from.getTime() > to.getTime()) {
            return null;
        }

        return { from: from.toISOString(), to: to.toISOString() };
    }

    let from = new Date(now);
    let to = new Date(now);

    switch (range.value) {
        case 'today':
            from = startOfLocalDay(now);
            break;
        case 'yesterday':
            from = startOfLocalDay(now);
            from.setDate(from.getDate() - 1);
            to.setDate(to.getDate() - 1);
            to = endOfLocalDay(to);
            break;
        case '30m':
            from.setMinutes(to.getMinutes() - 30);
            break;
        case '1h':
            from.setHours(to.getHours() - 1);
            break;
        case '6h':
            from.setHours(to.getHours() - 6);
            break;
        case '24h':
            from.setHours(to.getHours() - 24);
            break;
        case '3d':
            from.setDate(to.getDate() - 3);
            break;
        case '7d':
            from.setDate(to.getDate() - 7);
            break;
        case '14d':
            from.setDate(to.getDate() - 14);
            break;
        case '30d':
            from.setDate(to.getDate() - 30);
            break;
        case '60d':
            from.setDate(to.getDate() - 60);
            break;
        case '90d':
            from.setDate(to.getDate() - 90);
            break;
        case '180d':
            from.setDate(to.getDate() - 180);
            break;
        case 'thisWeek':
            from = startOfIsoWeek(now);
            break;
        case 'lastWeek': {
            const thisWeekStart = startOfIsoWeek(now);
            from = new Date(thisWeekStart);
            from.setDate(from.getDate() - 7);
            to = new Date(thisWeekStart);
            to.setMilliseconds(-1);
            break;
        }
        case 'thisMonth':
            from = new Date(now.getFullYear(), now.getMonth(), 1);
            break;
        case 'lastMonth':
            from = new Date(now.getFullYear(), now.getMonth() - 1, 1);
            to = new Date(now.getFullYear(), now.getMonth(), 1);
            to.setMilliseconds(-1);
            break;
        case 'thisYear':
            from = new Date(now.getFullYear(), 0, 1);
            break;
        case '1y':
            from.setFullYear(to.getFullYear() - 1);
            break;
    }

    return { from: from.toISOString(), to: to.toISOString() };
}

export function isShortRange(range: RangeOption, customRangeDates: Date[] | null = null): boolean {
    if (SHORT_RANGE_VALUES.has(range.value)) {
        return true;
    }

    if (range.value !== 'custom' || !customRangeDates || customRangeDates.length !== 2 || !customRangeDates[0] || !customRangeDates[1]) {
        return false;
    }

    return customRangeDates[1].getTime() - customRangeDates[0].getTime() < 48 * 60 * 60 * 1000;
}
