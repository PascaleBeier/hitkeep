import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { SeriesChart } from '@features/analytics/components/series-chart';
import { ReportSubjectService } from '@services/report-subject.service';
import { SiteAnnotationsService } from '@features/annotations/site-annotations.service';
import type { Annotation } from '@models/analytics.types';
import { signal } from '@angular/core';
import { vi } from 'vitest';

function stubAnnotations() {
    return {
        annotations: signal<Annotation[]>([]),
        canWrite: signal(false),
        load: vi.fn(),
        open: vi.fn(),
        openExisting: vi.fn()
    };
}

/** A live ECharts instance as far as SeriesChart touches it. */
function liveChart(setOption = vi.fn(), disposed = false) {
    const handlers = new Map<string, (params: unknown) => void>();
    return {
        setOption,
        handlers,
        isDisposed: () => disposed,
        on: (name: string, fn: (params: unknown) => void) => handlers.set(name, fn),
        off: vi.fn(),
        getZr: () => ({ on: vi.fn(), off: vi.fn(), setCursorStyle: vi.fn() })
    };
}

describe('SeriesChart', () => {
    let component: SeriesChart;
    let fixture: ComponentFixture<SeriesChart>;
    let notes: ReturnType<typeof stubAnnotations>;

    beforeEach(async () => {
        notes = stubAnnotations();
        await TestBed.configureTestingModule({
            imports: [
                SeriesChart,
                TranslocoTestingModule.forRoot({
                    langs: { en: {} },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                { provide: SiteAnnotationsService, useValue: notes },
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: {
                        en: 'en-US',
                        'en-US': 'en-US'
                    }
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(SeriesChart);
        component = fixture.componentInstance;
        fixture.componentRef.setInput('data', []);
        fixture.componentRef.setInput('series', []);
        fixture.detectChanges();
    });

    it('keeps an image role and accessible label', () => {
        const container = fixture.debugElement.query(By.css('div[role="img"]'));
        expect(container).toBeTruthy();
        expect(container.nativeElement.getAttribute('aria-label')).toBeTruthy();
    });

    it('renders the ECharts directive instead of the OptimusUI chart element when data exists', () => {
        fixture.componentRef.setInput('data', [
            { time: '2026-07-01T00:00:00Z', count: 5 },
            { time: '2026-07-02T00:00:00Z', count: 9 }
        ]);
        fixture.componentRef.setInput('series', [
            {
                key: 'count',
                label: 'Events',
                color: '#2563eb',
                gradientFrom: 'rgba(37, 99, 235, 0.3)',
                gradientTo: 'rgba(37, 99, 235, 0)'
            }
        ]);
        fixture.detectChanges();

        expect(fixture.debugElement.query(By.css('[echarts]'))).toBeTruthy();
        expect(fixture.debugElement.query(By.css('app-chart-design-toggle'))).toBeTruthy();
        expect(fixture.debugElement.query(By.css('p-' + 'chart'))).toBeNull();
    });

    it('applies comparison series, chart design variants, and merge updates', () => {
        fixture.componentRef.setInput('data', [
            { time: '2026-07-01T00:00:00Z', count: 5 },
            { time: '2026-07-02T00:00:00Z', count: 9 }
        ]);
        fixture.componentRef.setInput('comparisonData', [
            { time: '2026-06-29T00:00:00Z', count: 3 },
            { time: '2026-06-30T00:00:00Z', count: 4 }
        ]);
        fixture.componentRef.setInput('series', [
            {
                key: 'count',
                label: 'Events',
                color: '#2563eb',
                gradientFrom: 'rgba(37, 99, 235, 0.3)',
                gradientTo: 'rgba(37, 99, 235, 0)'
            }
        ]);
        fixture.componentRef.setInput('design', 'bar');
        fixture.detectChanges();

        const inspectable = component as unknown as {
            chartFrameOptions: () => { series: { data: number[] }[] };
            chartMergeOptions: () => { xAxis: { data: string[] }; series: { name: string; type: string; data: number[]; lineStyle?: { type?: string } }[] };
        };
        const frame = inspectable.chartFrameOptions();
        const merge = inspectable.chartMergeOptions();
        expect(merge.series.find((series) => series.name === 'Events')?.type).toBe('bar');
        expect(merge.series.find((series) => series.name === 'Events (prev.)')?.lineStyle?.type).toBe('dashed');
        expect(frame.series.find((series) => series.data.length > 0)).toBeUndefined();
        expect(merge.xAxis.data.length).toBe(2);
        expect(merge.series.find((series) => series.name === 'Events')?.data).toEqual([5, 9]);
    });

    it('patches a live chart with replaceMerge so dropped series cannot linger', async () => {
        const setOption = vi.fn();
        const chart = liveChart(setOption);
        fixture.componentRef.setInput('data', [{ time: '2026-07-01T00:00:00Z', count: 5 }]);
        fixture.componentRef.setInput('comparisonData', [{ time: '2026-06-30T00:00:00Z', count: 3 }]);
        fixture.componentRef.setInput('series', [{ key: 'count', label: 'Events', color: '#2563eb' }]);
        fixture.detectChanges();

        (component as unknown as { onChartInit: (chart: unknown) => void }).onChartInit(chart);
        fixture.detectChanges();
        await fixture.whenStable();

        const firstPatch = setOption.mock.lastCall as [{ series: { name: string }[] }, { replaceMerge: string[] }];
        expect(firstPatch[1]).toEqual({ replaceMerge: ['series'] });
        expect(firstPatch[0].series.map((series) => series.name)).toContain('Events (prev.)');

        fixture.componentRef.setInput('comparisonData', []);
        fixture.detectChanges();
        await fixture.whenStable();

        const secondPatch = setOption.mock.lastCall as [{ series: { name: string }[] }, { replaceMerge: string[] }];
        expect(secondPatch[0].series.map((series) => series.name)).toEqual(['Events']);
    });

    it('leaves a disposed chart alone', async () => {
        const setOption = vi.fn();
        fixture.componentRef.setInput('data', [{ time: '2026-07-01T00:00:00Z', count: 5 }]);
        fixture.componentRef.setInput('series', [{ key: 'count', label: 'Events', color: '#2563eb' }]);
        fixture.detectChanges();

        (component as unknown as { onChartInit: (chart: unknown) => void }).onChartInit(liveChart(setOption, true));
        fixture.componentRef.setInput('data', [{ time: '2026-07-02T00:00:00Z', count: 9 }]);
        fixture.detectChanges();
        await fixture.whenStable();

        expect(setOption).not.toHaveBeenCalled();
    });

    it('keeps the rendered chart through a reload of the same subject', async () => {
        fixture.componentRef.setInput('data', [{ time: '2026-07-01T00:00:00Z', count: 5 }]);
        fixture.componentRef.setInput('series', [{ key: 'count', label: 'Events', color: '#2563eb' }]);
        fixture.detectChanges();
        expect(fixture.debugElement.query(By.css('[echarts]'))).toBeTruthy();

        fixture.componentRef.setInput('isLoading', true);
        await fixture.whenStable();

        expect(fixture.debugElement.query(By.css('[echarts]'))).toBeTruthy();
        expect(fixture.debugElement.query(By.css('.pi-spinner'))).toBeNull();
    });

    it('shows the spinner for a first load and after the subject changes', async () => {
        fixture.componentRef.setInput('data', [{ time: '2026-07-01T00:00:00Z', count: 5 }]);
        fixture.componentRef.setInput('series', [{ key: 'count', label: 'Events', color: '#2563eb' }]);
        fixture.detectChanges();

        TestBed.inject(ReportSubjectService).set('another-site');
        fixture.componentRef.setInput('isLoading', true);
        await fixture.whenStable();

        expect(fixture.debugElement.query(By.css('.pi-spinner'))).toBeTruthy();
        expect(fixture.debugElement.query(By.css('[echarts]'))).toBeNull();
    });

    describe('annotations', () => {
        const days = [
            { time: '2026-07-01T00:00:00Z', count: 5 },
            { time: '2026-07-02T00:00:00Z', count: 9 },
            { time: '2026-07-03T00:00:00Z', count: 4 }
        ];
        const launch: Annotation = { id: 'n1', site_id: 's', starts_at: '2026-07-02T09:00:00Z', body: 'Launch', created_at: '2026-07-02T09:00:00Z' };
        const campaign: Annotation = { id: 'n2', site_id: 's', starts_at: '2026-07-01T00:00:00Z', ends_at: '2026-07-03T00:00:00Z', body: 'Campaign', created_at: '2026-07-01T00:00:00Z' };

        beforeEach(() => {
            fixture.componentRef.setInput('data', days);
            fixture.componentRef.setInput('series', [{ key: 'count', label: 'Events', color: '#2563eb' }]);
        });

        it('asks for notes once a chart exists', () => {
            expect(notes.load).toHaveBeenCalled();
        });

        it('draws notes on the current series only and lists them for keyboard readers', () => {
            notes.annotations.set([launch, campaign]);
            fixture.componentRef.setInput('comparisonData', days);
            fixture.detectChanges();

            const merge = (component as unknown as { chartMergeOptions: () => { series: { name: string; markLine?: { data: unknown[] }; markArea?: { data: unknown[] } }[] } }).chartMergeOptions();
            const current = merge.series.find((series) => series.name === 'Events');
            expect(current?.markLine?.data).toEqual([{ xAxis: 1, name: 'Launch' }]);
            expect(current?.markArea?.data).toEqual([[{ xAxis: 0, name: 'Campaign' }, { xAxis: 2 }]]);
            expect(merge.series.find((series) => series.name !== 'Events')?.markLine).toBeUndefined();

            const chips = fixture.debugElement.queryAll(By.css('app-annotation-strip button'));
            expect(chips.map((chip) => chip.nativeElement.textContent)).toEqual([expect.stringContaining('Campaign'), expect.stringContaining('Launch')]);
            chips[1].nativeElement.click();
            expect(notes.openExisting).toHaveBeenCalledWith(launch, 'day');
        });

        it('offers the add button only to people who can write notes', () => {
            fixture.detectChanges();
            expect(fixture.debugElement.query(By.css('p-button'))).toBeNull();

            notes.canWrite.set(true);
            fixture.detectChanges();
            fixture.debugElement.query(By.css('p-button button')).nativeElement.click();
            expect(notes.open).toHaveBeenCalledWith({ startsAt: new Date('2026-07-03T00:00:00Z'), endsAt: null, body: '', granularity: 'day' });
        });

        it('opens the note behind a clicked marker', () => {
            notes.annotations.set([launch, campaign]);
            fixture.detectChanges();
            const chart = liveChart();
            (component as unknown as { onChartInit: (chart: unknown) => void }).onChartInit(chart);

            chart.handlers.get('click')?.({ componentType: 'markArea', dataIndex: 0 });
            expect(notes.openExisting).toHaveBeenCalledWith(campaign, 'day');
        });

        it('counts notes in the chart label', () => {
            notes.annotations.set([launch]);
            fixture.detectChanges();
            const label = fixture.debugElement.query(By.css('div[role="img"]')).nativeElement.getAttribute('aria-label');
            expect(label).toContain('annotations.chart.aria');
        });
    });
});
