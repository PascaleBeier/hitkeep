import { graphic } from 'echarts/core';
import type { ECharts } from 'echarts/core';

export type AnnotationMarkerKind = 'line' | 'area';

export interface AnnotationInteractionOptions {
    /** Adding is offered only to people who may write notes. */
    canWrite: () => boolean;
    bucketCount: () => number;
    /** Bars occupy whole bands; lines and areas sit on the ticks. */
    isBar: () => boolean;
    previewColor: () => string;
    /** A click adds a point (`end` null); a drag across buckets adds a range. */
    onAdd: (start: number, end: number | null) => void;
    onOpen: (kind: AnnotationMarkerKind, dataIndex: number) => void;
}

interface ZrPointerEvent {
    offsetX: number;
    offsetY: number;
    event?: { button?: number; pointerType?: string; touches?: unknown };
}

interface MarkerEventParams {
    componentType?: string;
    dataIndex?: number;
}

interface Rect {
    x: number;
    y: number;
    width: number;
    height: number;
}

const MARKER_KINDS: Record<string, AnnotationMarkerKind> = { markLine: 'line', markArea: 'area' };

/**
 * Wires note gestures onto a live chart: press and release on the plot adds a
 * point, press and drag marks a range with a live band, Escape cancels, and a
 * marker click opens that note. Returns the teardown.
 */
export function bindAnnotationInteractions(chart: ECharts, options: AnnotationInteractionOptions): () => void {
    const zr = chart.getZr();
    let markerPressed = false;
    let drag: { start: number; current: number } | null = null;
    let preview: graphic.Rect | null = null;

    const bucketAt = (x: number): number => {
        const raw = chart.convertFromPixel({ xAxisIndex: 0 }, x) as unknown;
        const value = Array.isArray(raw) ? Number(raw[0]) : Number(raw);
        const last = Math.max(0, options.bucketCount() - 1);
        return Number.isFinite(value) ? Math.min(last, Math.max(0, Math.round(value))) : 0;
    };

    const pixelAt = (index: number): number => Number(chart.convertToPixel({ xAxisIndex: 0 }, index));

    const drawPreview = (): void => {
        if (!drag) {
            return;
        }
        const grid = gridRect(chart);
        const [from, to] = drag.start <= drag.current ? [drag.start, drag.current] : [drag.current, drag.start];
        const band = options.bucketCount() > 1 ? Math.abs(pixelAt(1) - pixelAt(0)) : 16;
        // Bars fill their band, so the band edge is where a bar ends; ticks mark line buckets.
        const pad = options.isBar() || from === to ? band / 2 : 0;
        const left = Math.max(grid.x, pixelAt(from) - pad);
        const right = Math.min(grid.x + grid.width, pixelAt(to) + pad);
        const shape = { x: left, y: grid.y, width: Math.max(2, right - left), height: grid.height, r: 4 };
        if (!preview) {
            const color = options.previewColor();
            preview = new graphic.Rect({ shape, silent: true, z: 1000, style: { fill: color, opacity: 0.16, stroke: color, lineWidth: 1, lineDash: [4, 3] } });
            zr.add(preview);
        } else {
            preview.setShape(shape);
        }
    };

    const clearPreview = (): void => {
        if (preview) {
            zr.remove(preview);
            preview = null;
        }
    };

    // The axis tooltip re-opens on every move; mute it while a range is being drawn.
    const setTooltipMuted = (muted: boolean): void => {
        chart.setOption({ tooltip: { triggerOn: muted ? 'none' : 'mousemove|click' } });
        if (muted) {
            chart.dispatchAction({ type: 'hideTip' });
        }
    };

    const stopDrag = (): void => {
        window.removeEventListener('mouseup', finishDrag);
        window.removeEventListener('keydown', onKeydown);
        if (drag && !chart.isDisposed()) {
            setTooltipMuted(false);
        }
        drag = null;
        clearPreview();
    };

    function finishDrag(): void {
        const done = drag;
        stopDrag();
        if (!done) {
            return;
        }
        const [start, end] = done.start <= done.current ? [done.start, done.current] : [done.current, done.start];
        options.onAdd(start, end > start ? end : null);
    }

    function onKeydown(event: KeyboardEvent): void {
        if (event.key === 'Escape') {
            stopDrag();
        }
    }

    const onMarkerDown = (params: MarkerEventParams): void => {
        if (params.componentType && MARKER_KINDS[params.componentType]) {
            markerPressed = true;
        }
    };

    const onMarkerClick = (params: MarkerEventParams): void => {
        const kind = params.componentType ? MARKER_KINDS[params.componentType] : undefined;
        if (kind && typeof params.dataIndex === 'number') {
            options.onOpen(kind, params.dataIndex);
        }
    };

    // ECharts dispatches its own mousedown before this zrender listener, so a
    // press that landed on a marker is already flagged here.
    const onPointerDown = (event: ZrPointerEvent): void => {
        const onMarker = markerPressed;
        markerPressed = false;
        if (onMarker || !options.canWrite() || (event.event?.button ?? 0) !== 0 || options.bucketCount() === 0) {
            return;
        }
        // A tap is how touch readers get the tooltip; they add notes from the button instead.
        if (event.event?.touches !== undefined || event.event?.pointerType === 'touch') {
            return;
        }
        if (!chart.containPixel('grid', [event.offsetX, event.offsetY])) {
            return;
        }
        const index = bucketAt(event.offsetX);
        drag = { start: index, current: index };
        setTooltipMuted(true);
        window.addEventListener('mouseup', finishDrag);
        window.addEventListener('keydown', onKeydown);
    };

    const onPointerMove = (event: ZrPointerEvent): void => {
        if (!drag) {
            if (options.canWrite() && chart.containPixel('grid', [event.offsetX, event.offsetY])) {
                zr.setCursorStyle('crosshair');
            }
            return;
        }
        const index = bucketAt(event.offsetX);
        if (index !== drag.current || !preview) {
            drag.current = index;
            drawPreview();
        }
    };

    chart.on('mousedown', onMarkerDown);
    chart.on('click', onMarkerClick);
    zr.on('mousedown', onPointerDown);
    zr.on('mousemove', onPointerMove);

    return () => {
        stopDrag();
        if (chart.isDisposed()) {
            return;
        }
        chart.off('mousedown', onMarkerDown);
        chart.off('click', onMarkerClick);
        zr.off('mousedown', onPointerDown);
        zr.off('mousemove', onPointerMove);
    };
}

/** The plot area in pixels; falls back to the whole canvas if ECharts has not laid out a grid yet. */
function gridRect(chart: ECharts): Rect {
    const model = (chart as unknown as { getModel?: () => { getComponent: (type: string, index: number) => { coordinateSystem?: { getRect: () => Rect } } | undefined } }).getModel?.();
    return model?.getComponent('grid', 0)?.coordinateSystem?.getRect() ?? { x: 0, y: 0, width: chart.getWidth(), height: chart.getHeight() };
}
