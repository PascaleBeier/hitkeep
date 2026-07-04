import { buildHitkeepChartMergeOptions, buildHitkeepChartOptions, formatChartValue, hitkeepChartTheme, type HitkeepChartSeries } from './hitkeep-chart-options';

interface InspectableOption {
    aria: { enabled: boolean; label: { description: string } };
    tooltip: { trigger: string; valueFormatter: (value: unknown) => string };
    xAxis: { boundaryGap: boolean; data: string[] };
    yAxis: { splitNumber: number };
    series: {
        name: string;
        type: 'line' | 'bar';
        data: number[];
        areaStyle?: unknown;
        lineStyle?: { type?: string; width?: number };
        itemStyle?: { borderRadius?: number[] };
        showSymbol?: boolean;
        emphasis?: { focus?: string };
    }[];
}

describe('hitkeep chart options', () => {
    const theme = hitkeepChartTheme(false);
    const baseSeries: HitkeepChartSeries[] = [
        {
            id: 'views',
            label: 'Pageviews',
            color: '#6366f1',
            gradientFrom: 'rgba(99, 102, 241, 0.5)',
            gradientTo: 'rgba(99, 102, 241, 0)',
            data: [10, 20]
        }
    ];

    it('builds accessible dense area line charts by default', () => {
        const options = buildHitkeepChartOptions({
            ariaLabel: 'Traffic chart',
            labels: ['Jul 1', 'Jul 2'],
            locale: 'en-US',
            series: baseSeries,
            theme
        }) as unknown as InspectableOption;

        expect(options.aria.enabled).toBe(true);
        expect(options.aria.label.description).toBe('Traffic chart');
        expect(options.tooltip.trigger).toBe('axis');
        expect(options.xAxis.boundaryGap).toBe(false);
        expect(options.yAxis.splitNumber).toBe(6);
        expect(options.series[0]?.type).toBe('line');
        expect(options.series[0]?.areaStyle).toBeTruthy();
        expect(options.series[0]?.showSymbol).toBe(false);
        expect(options.series[0]?.emphasis?.focus).toBe('series');
    });

    it('renders comparison series as muted dashed lines', () => {
        const options = buildHitkeepChartOptions({
            ariaLabel: 'Series chart',
            labels: ['Jul 1', 'Jul 2'],
            locale: 'en-US',
            series: [
                ...baseSeries,
                {
                    id: 'views-comparison',
                    label: 'Pageviews (previous)',
                    color: 'rgba(99, 102, 241, 0.4)',
                    data: [8, 16],
                    muted: true,
                    dashed: true
                }
            ],
            theme
        }) as unknown as InspectableOption;

        expect(options.series[1]?.lineStyle?.type).toBe('dashed');
        expect(options.series[1]?.lineStyle?.width).toBe(1.5);
        expect(options.series[1]?.areaStyle).toBeUndefined();
    });

    it('supports a bar design variant without changing callers data shape', () => {
        const options = buildHitkeepChartOptions({
            ariaLabel: 'Bar chart',
            design: 'bar',
            labels: ['Jul 1', 'Jul 2'],
            locale: 'en-US',
            series: baseSeries,
            theme
        }) as unknown as InspectableOption;

        expect(options.xAxis.boundaryGap).toBe(true);
        expect(options.series[0]?.type).toBe('bar');
        expect(options.series[0]?.areaStyle).toBeUndefined();
        expect(options.series[0]?.itemStyle?.borderRadius).toEqual([4, 4, 0, 0]);
    });

    it('builds a merge payload for realtime label and series updates', () => {
        const merge = buildHitkeepChartMergeOptions({
            ariaLabel: 'Traffic chart',
            labels: ['10:00', '10:05', '10:10'],
            locale: 'en-US',
            series: [
                {
                    ...baseSeries[0]!,
                    data: [1, 3, 5]
                }
            ],
            theme
        }) as unknown as InspectableOption;

        expect(merge.aria.label.description).toBe('Traffic chart');
        expect(merge.xAxis.data).toEqual(['10:00', '10:05', '10:10']);
        expect(merge.series[0]?.data).toEqual([1, 3, 5]);
        expect(merge.series[0]?.type).toBe('line');
    });

    it('formats tooltip values with the active locale', () => {
        expect(formatChartValue(1234.56, 'de-DE')).toBe('1.234,56');
        expect(formatChartValue(42, 'en-US')).toBe('42');
    });
});
