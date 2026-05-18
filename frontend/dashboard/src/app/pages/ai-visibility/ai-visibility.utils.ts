import { SeriesChartPoint } from '@features/analytics/components/series-chart';
import { AIFetchSeriesPoint } from '@models/analytics.types';

export interface AIFilterChip {
    key: 'assistantName' | 'assistantFamily' | 'resourceType' | 'path';
    label: string;
}

export function mapAIFetchSeries(points: AIFetchSeriesPoint[]): SeriesChartPoint[] {
    return points.map((point) => ({ time: point.time, count: point.count }));
}

export function formatBytes(value: number, locale: string): string {
    if (value <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let size = value;
    let unitIndex = 0;

    while (size >= 1024 && unitIndex < units.length - 1) {
        size /= 1024;
        unitIndex++;
    }

    const maximumFractionDigits = size >= 10 || unitIndex === 0 ? 0 : 1;
    return `${new Intl.NumberFormat(locale, { maximumFractionDigits }).format(size)} ${units[unitIndex]}`;
}

export function formatResponseMs(value: number, locale: string): string {
    if (value <= 0) return '0 ms';
    return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(value)} ms`;
}
