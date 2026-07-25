import { SeriesChartPoint } from '@features/analytics/components/series-chart';
import { EventSeriesPoint } from '@models/analytics.types';

export type ChatbotMetricKey = 'started' | 'sent' | 'rendered' | 'clicked' | 'handoff' | 'assisted';

/** Metrics plotted on the conversation activity chart, in stacking order. */
const CHARTED_METRIC_KEYS = ['started', 'rendered', 'handoff', 'assisted'] as const satisfies readonly ChatbotMetricKey[];

export interface ChatbotSeriesState {
    started: EventSeriesPoint[];
    sent: EventSeriesPoint[];
    rendered: EventSeriesPoint[];
    clicked: EventSeriesPoint[];
    handoff: EventSeriesPoint[];
    assisted: EventSeriesPoint[];
}

export function createEmptySeries(): ChatbotSeriesState {
    return {
        started: [],
        sent: [],
        rendered: [],
        clicked: [],
        handoff: [],
        assisted: []
    };
}

export function computeComparisonPeriod(from: string, to: string): { from: string; to: string } {
    const start = new Date(from);
    const end = new Date(to);
    const duration = end.getTime() - start.getTime();
    const comparisonEnd = new Date(start.getTime() - 1);
    return {
        from: new Date(comparisonEnd.getTime() - duration).toISOString(),
        to: comparisonEnd.toISOString()
    };
}

export function totalFor(key: ChatbotMetricKey, state: ChatbotSeriesState): number {
    return state[key].reduce((sum, point) => sum + point.count, 0);
}

/** Merges the charted chatbot metrics into chronologically sorted chart points. */
export function mergeChatbotChartSeries(state: ChatbotSeriesState): SeriesChartPoint[] {
    const byTime = new Map<string, SeriesChartPoint>();
    for (const key of CHARTED_METRIC_KEYS) {
        for (const point of state[key]) {
            const current = byTime.get(point.time) ?? {
                time: point.time,
                started: 0,
                rendered: 0,
                handoff: 0,
                assisted: 0
            };
            current[key] = point.count;
            byTime.set(point.time, current);
        }
    }
    return [...byTime.values()].sort((a, b) => new Date(a.time).getTime() - new Date(b.time).getTime());
}
