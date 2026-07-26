import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { SeriesChart } from '@features/analytics/components/series-chart';
import { ReportSubjectService } from '@services/report-subject.service';
import { vi } from 'vitest';

describe('SeriesChart', () => {
    let component: SeriesChart;
    let fixture: ComponentFixture<SeriesChart>;

    beforeEach(async () => {
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
        const chart = { setOption, isDisposed: () => false };
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

        (component as unknown as { onChartInit: (chart: unknown) => void }).onChartInit({ setOption, isDisposed: () => true });
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
});
