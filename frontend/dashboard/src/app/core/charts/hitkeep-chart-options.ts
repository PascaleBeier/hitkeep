import type { EChartsCoreOption } from 'echarts/core';

export type HitkeepChartDesign = 'area' | 'line' | 'bar';

export const HITKEEP_CHART_PALETTE = {
    primary: '#6366f1',
    secondary: '#14b8a6',
    warning: '#d97706'
} as const;

export interface HitkeepChartTheme {
    textColor: string;
    gridColor: string;
    tooltipBackgroundColor: string;
    tooltipTextColor: string;
    tooltipBorderColor: string;
    axisPointerColor: string;
}

export interface HitkeepChartSeries {
    id: string;
    label: string;
    color: string;
    data: number[];
    gradientFrom?: string;
    gradientTo?: string;
    design?: HitkeepChartDesign;
    dashed?: boolean;
    muted?: boolean;
    smooth?: boolean;
}

interface BuildHitkeepChartOptionsInput {
    ariaLabel: string;
    labels: string[];
    locale: string;
    series: HitkeepChartSeries[];
    theme: HitkeepChartTheme;
    design?: HitkeepChartDesign;
    yAxisTicks?: number;
}

interface LinearGradientFill {
    type: 'linear';
    x: number;
    y: number;
    x2: number;
    y2: number;
    colorStops: { offset: number; color: string }[];
}

export function hitkeepChartTheme(isDark: boolean): HitkeepChartTheme {
    return {
        textColor: isDark ? '#94a3b8' : '#64748b',
        gridColor: isDark ? 'rgba(255, 255, 255, 0.05)' : 'rgba(0, 0, 0, 0.05)',
        tooltipBackgroundColor: isDark ? 'rgba(15, 23, 42, 0.94)' : 'rgba(255, 255, 255, 0.96)',
        tooltipTextColor: isDark ? '#f8fafc' : '#0f172a',
        tooltipBorderColor: isDark ? '#334155' : '#e2e8f0',
        axisPointerColor: isDark ? 'rgba(148, 163, 184, 0.38)' : 'rgba(100, 116, 139, 0.28)'
    };
}

/**
 * Charts morph between date ranges instead of being torn down, which makes the
 * animation itself the change indicator. Readers who asked for less motion get
 * the new shape immediately instead.
 */
function motionDuration(ms: number): number {
    const reduced = typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
    return reduced ? 0 : ms;
}

export function buildHitkeepChartOptions(input: BuildHitkeepChartOptionsInput): EChartsCoreOption {
    const design = input.design ?? 'area';
    const hasBarSeries = input.series.some((series) => resolveSeriesDesign(series, design) === 'bar');
    const option = {
        animationDuration: motionDuration(260),
        animationDurationUpdate: motionDuration(240),
        animationEasingUpdate: 'cubicOut',
        aria: {
            enabled: true,
            decal: { show: true },
            label: {
                description: input.ariaLabel
            }
        },
        color: input.series.map((series) => series.color),
        grid: {
            left: 8,
            right: 12,
            top: 32,
            bottom: 48,
            containLabel: true
        },
        legend: {
            show: input.series.length > 1,
            type: 'scroll',
            bottom: 0,
            icon: 'circle',
            itemWidth: 8,
            itemHeight: 8,
            textStyle: {
                color: input.theme.textColor,
                fontSize: 12
            },
            pageIconColor: input.theme.textColor,
            pageIconInactiveColor: withChartAlpha(input.theme.textColor, 0.32),
            pageTextStyle: {
                color: input.theme.textColor
            }
        },
        tooltip: {
            trigger: 'axis',
            confine: true,
            appendToBody: false,
            backgroundColor: input.theme.tooltipBackgroundColor,
            borderColor: input.theme.tooltipBorderColor,
            borderWidth: 1,
            padding: [8, 10],
            textStyle: {
                color: input.theme.tooltipTextColor,
                fontSize: 12
            },
            axisPointer: {
                type: hasBarSeries ? 'shadow' : 'line',
                snap: true,
                lineStyle: {
                    color: input.theme.axisPointerColor,
                    width: 1
                },
                shadowStyle: {
                    color: input.theme.axisPointerColor
                }
            },
            valueFormatter: (value: unknown) => formatChartValue(value, input.locale)
        },
        xAxis: {
            type: 'category',
            boundaryGap: hasBarSeries,
            data: input.labels,
            axisLabel: {
                color: input.theme.textColor,
                hideOverlap: true,
                margin: 12
            },
            axisLine: { show: false },
            axisTick: { show: false },
            splitLine: { show: false }
        },
        yAxis: {
            type: 'value',
            min: 0,
            splitNumber: input.yAxisTicks ?? 6,
            axisLabel: {
                color: input.theme.textColor,
                formatter: (value: number) => formatChartValue(value, input.locale)
            },
            axisLine: { show: false },
            axisTick: { show: false },
            splitLine: {
                lineStyle: {
                    color: input.theme.gridColor
                }
            }
        },
        series: input.series.map((series) => buildSeriesOption(series, design))
    };
    return option as EChartsCoreOption;
}

export function buildHitkeepChartMergeOptions(input: BuildHitkeepChartOptionsInput): EChartsCoreOption {
    const design = input.design ?? 'area';
    const hasBarSeries = input.series.some((series) => resolveSeriesDesign(series, design) === 'bar');
    return {
        animationDurationUpdate: motionDuration(240),
        animationEasingUpdate: 'cubicOut',
        aria: {
            label: {
                description: input.ariaLabel
            }
        },
        color: input.series.map((series) => series.color),
        tooltip: {
            axisPointer: {
                type: hasBarSeries ? 'shadow' : 'line',
                snap: true,
                lineStyle: {
                    color: input.theme.axisPointerColor,
                    width: 1
                },
                shadowStyle: {
                    color: input.theme.axisPointerColor
                }
            },
            valueFormatter: (value: unknown) => formatChartValue(value, input.locale)
        },
        xAxis: {
            boundaryGap: hasBarSeries,
            data: input.labels
        },
        series: input.series.map((series) => buildSeriesOption(series, design))
    } as EChartsCoreOption;
}

export function formatChartValue(value: unknown, locale: string): string {
    const numberValue = typeof value === 'number' ? value : typeof value === 'string' && value.trim() !== '' ? Number(value) : Number.NaN;
    if (!Number.isFinite(numberValue)) {
        return value === null || value === undefined ? '' : String(value);
    }
    return new Intl.NumberFormat(locale, {
        maximumFractionDigits: Number.isInteger(numberValue) ? 0 : 2
    }).format(numberValue);
}

export function withChartAlpha(color: string, alpha: number): string {
    const boundedAlpha = Math.min(1, Math.max(0, alpha));
    const hex = /^#?([a-f\d]{3}|[a-f\d]{6})$/i.exec(color.trim());
    if (hex?.[1]) {
        const raw =
            hex[1].length === 3
                ? hex[1]
                      .split('')
                      .map((part) => part + part)
                      .join('')
                : hex[1];
        const r = parseInt(raw.slice(0, 2), 16);
        const g = parseInt(raw.slice(2, 4), 16);
        const b = parseInt(raw.slice(4, 6), 16);
        return `rgba(${r}, ${g}, ${b}, ${boundedAlpha})`;
    }

    const rgb = /^rgba?\(\s*([.\d]+)\s*,\s*([.\d]+)\s*,\s*([.\d]+)(?:\s*,\s*([.\d]+))?\s*\)$/i.exec(color.trim());
    if (rgb?.[1] && rgb[2] && rgb[3]) {
        return `rgba(${rgb[1]}, ${rgb[2]}, ${rgb[3]}, ${boundedAlpha})`;
    }
    return color;
}

function buildSeriesOption(series: HitkeepChartSeries, defaultDesign: HitkeepChartDesign): object {
    const design = resolveSeriesDesign(series, defaultDesign);
    if (design === 'bar') {
        return {
            id: series.id,
            name: series.label,
            type: 'bar',
            data: series.data,
            barMaxWidth: 30,
            itemStyle: {
                color: series.muted ? withChartAlpha(series.color, 0.38) : series.color,
                borderRadius: [4, 4, 0, 0]
            },
            emphasis: {
                focus: 'series',
                itemStyle: {
                    color: series.color
                }
            }
        };
    }

    const areaStyle = design === 'area' && !series.muted && !series.dashed ? { color: gradientFill(series) } : undefined;
    return {
        id: series.id,
        name: series.label,
        type: 'line',
        data: series.data,
        smooth: series.smooth ?? !series.dashed,
        showSymbol: false,
        symbol: 'circle',
        symbolSize: series.muted ? 5 : 6,
        connectNulls: true,
        sampling: 'lttb',
        lineStyle: {
            color: series.color,
            width: series.muted ? 1.5 : 2,
            type: series.dashed ? 'dashed' : 'solid'
        },
        itemStyle: {
            color: series.color
        },
        areaStyle,
        emphasis: {
            focus: 'series',
            scale: 1.35
        }
    };
}

function resolveSeriesDesign(series: HitkeepChartSeries, defaultDesign: HitkeepChartDesign): HitkeepChartDesign {
    if (series.muted || series.dashed) {
        return 'line';
    }
    return series.design ?? defaultDesign;
}

function gradientFill(series: HitkeepChartSeries): string | LinearGradientFill {
    const start = series.gradientFrom ?? withChartAlpha(series.color, 0.28);
    const end = series.gradientTo ?? withChartAlpha(series.color, 0);
    return {
        type: 'linear',
        x: 0,
        y: 0,
        x2: 0,
        y2: 1,
        colorStops: [
            { offset: 0, color: start },
            { offset: 1, color: end }
        ]
    };
}
