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

    it('draws annotations as markers on the series that carries them only', () => {
        const annotations = { lines: [{ index: 1, label: 'Launch' }], areas: [{ start: 0, end: 1, label: 'Campaign' }] };
        for (const design of ['area', 'bar'] as const) {
            const option = buildHitkeepChartOptions({
                ariaLabel: 'Chart',
                labels: ['Jul 1', 'Jul 2'],
                locale: 'en-US',
                design,
                theme,
                series: [{ ...baseSeries[0], annotations }, { id: 'views-comparison', label: 'Prev', color: '#6366f1', data: [1, 2], muted: true, dashed: true }]
            }) as unknown as { series: { markLine?: { data: unknown[] }; markArea?: { data: unknown[] } }[] };

            expect(option.series[0].markLine?.data).toEqual([{ xAxis: 1, name: 'Launch' }]);
            expect(option.series[0].markArea?.data).toEqual([[{ xAxis: 0, name: 'Campaign', label: { distance: 6 } }, { xAxis: 1 }]]);
            expect(option.series[1].markLine).toBeUndefined();
            expect(option.series[1].markArea).toBeUndefined();
        }
    });

    it('stacks the labels of overlapping ranges on separate rows', () => {
        const areas = [
            { start: 0, end: 4, label: 'A' },
            { start: 2, end: 6, label: 'B' },
            { start: 5, end: 7, label: 'C' }
        ];
        const option = buildHitkeepChartOptions({ ariaLabel: 'Chart', labels: [], locale: 'en-US', theme, series: [{ ...baseSeries[0], annotations: { lines: [], areas } }] }) as unknown as {
            series: { markArea: { data: [{ label: { distance: number } }, unknown][] } }[];
        };
        expect(option.series[0].markArea.data.map(([start]) => start.label.distance)).toEqual([6, 22, 6]);
    });

    it('omits marker options when a series has no annotations', () => {
        const option = buildHitkeepChartMergeOptions({ ariaLabel: 'Chart', labels: ['Jul 1'], locale: 'en-US', theme, series: [{ ...baseSeries[0], annotations: { lines: [], areas: [] } }] }) as unknown as {
            series: object[];
        };
        expect(option.series[0]).not.toHaveProperty('markLine');
        expect(option.series[0]).not.toHaveProperty('markArea');
    });

    it('formats tooltip values with the active locale', () => {
        expect(formatChartValue(1234.56, 'de-DE')).toBe('1.234,56');
        expect(formatChartValue(42, 'en-US')).toBe('42');
    });
});
