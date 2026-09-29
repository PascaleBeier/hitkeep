import type { ECharts } from 'echarts/core';
import { vi } from 'vitest';
import { bindAnnotationInteractions, type AnnotationInteractionOptions } from '@core/charts/annotation-interactions';

type Handler = (event: unknown) => void;

/** A chart whose buckets sit 100px apart starting at x=0, with the plot everywhere below y=300. */
function fakeChart() {
    const chartHandlers = new Map<string, Handler>();
    const zrHandlers = new Map<string, Handler>();
    const added: unknown[] = [];
    const zr = {
        on: (name: string, fn: Handler) => zrHandlers.set(name, fn),
        off: (name: string) => zrHandlers.delete(name),
        add: (el: unknown) => added.push(el),
        remove: (el: unknown) => added.splice(added.indexOf(el), 1),
        setCursorStyle: vi.fn()
    };
    const chart = {
        getZr: () => zr,
        on: (name: string, fn: Handler) => chartHandlers.set(name, fn),
        off: (name: string) => chartHandlers.delete(name),
        containPixel: (_: string, [, y]: number[]) => y < 300,
        convertFromPixel: (_: unknown, x: number) => x / 100,
        convertToPixel: (_: unknown, index: number) => index * 100,
        dispatchAction: vi.fn(),
        getWidth: () => 500,
        getHeight: () => 300,
        isDisposed: () => false
    };
    const press = (x: number, y = 100, event: object = { button: 0 }) => {
        chartHandlers.get('mousedown')?.({});
        zrHandlers.get('mousedown')?.({ offsetX: x, offsetY: y, event });
    };
    const move = (x: number, y = 100) => zrHandlers.get('mousemove')?.({ offsetX: x, offsetY: y });
    const release = () => window.dispatchEvent(new MouseEvent('mouseup'));
    return { chart: chart as unknown as ECharts, chartHandlers, zrHandlers, zr, added, press, move, release };
}

function bind(fake: ReturnType<typeof fakeChart>, overrides: Partial<AnnotationInteractionOptions> = {}) {
    const options: AnnotationInteractionOptions = {
        canWrite: () => true,
        bucketCount: () => 5,
        isBar: () => false,
        previewColor: () => '#a16207',
        onAdd: vi.fn(),
        onOpen: vi.fn(),
        ...overrides
    };
    return { options, dispose: bindAnnotationInteractions(fake.chart, options) };
}

describe('bindAnnotationInteractions', () => {
    it('adds a point note on a plain click', () => {
        const fake = fakeChart();
        const { options } = bind(fake);
        fake.press(210);
        fake.release();
        expect(options.onAdd).toHaveBeenCalledWith(2, null);
    });

    it('adds a range when dragging across buckets, in either direction, with a live preview', () => {
        const fake = fakeChart();
        const { options } = bind(fake);
        fake.press(390);
        fake.move(120);
        expect(fake.added).toHaveLength(1);
        fake.release();
        expect(options.onAdd).toHaveBeenCalledWith(1, 4);
        expect(fake.added).toHaveLength(0);
    });

    it('cancels a drag on Escape', () => {
        const fake = fakeChart();
        const { options } = bind(fake);
        fake.press(100);
        fake.move(300);
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
        fake.release();
        expect(options.onAdd).not.toHaveBeenCalled();
        expect(fake.added).toHaveLength(0);
    });

    it('opens a marker instead of adding when the press lands on one', () => {
        const fake = fakeChart();
        const { options } = bind(fake);
        fake.chartHandlers.get('mousedown')?.({ componentType: 'markLine' });
        fake.zrHandlers.get('mousedown')?.({ offsetX: 100, offsetY: 100, event: { button: 0 } });
        fake.chartHandlers.get('click')?.({ componentType: 'markArea', dataIndex: 3 });
        fake.release();
        expect(options.onAdd).not.toHaveBeenCalled();
        expect(options.onOpen).toHaveBeenCalledWith('area', 3);
    });

    it('offers nothing to readers who cannot write, outside the plot, or on touch', () => {
        const readOnly = fakeChart();
        const { options: readOptions } = bind(readOnly, { canWrite: () => false });
        readOnly.press(100);
        readOnly.release();
        readOnly.move(100);
        expect(readOptions.onAdd).not.toHaveBeenCalled();
        expect(readOnly.zr.setCursorStyle).not.toHaveBeenCalled();

        const writer = fakeChart();
        const { options } = bind(writer);
        writer.press(100, 400);
        writer.press(100, 100, { touches: [] });
        writer.release();
        expect(options.onAdd).not.toHaveBeenCalled();
    });

    it('still lets readers open a marker', () => {
        const fake = fakeChart();
        const { options } = bind(fake, { canWrite: () => false });
        fake.chartHandlers.get('click')?.({ componentType: 'markLine', dataIndex: 0 });
        expect(options.onOpen).toHaveBeenCalledWith('line', 0);
    });

    it('removes every listener on teardown', () => {
        const fake = fakeChart();
        const { dispose } = bind(fake);
        dispose();
        expect(fake.chartHandlers.size).toBe(0);
        expect(fake.zrHandlers.size).toBe(0);
    });
});
