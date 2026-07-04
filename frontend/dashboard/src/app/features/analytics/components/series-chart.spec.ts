import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { SeriesChart } from '@features/analytics/components/series-chart';

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

    it('renders the ECharts directive instead of the PrimeNG chart element when data exists', () => {
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
        expect(fixture.debugElement.query(By.css('p-select'))).toBeTruthy();
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
            chartOptions: () => { series: { name: string; type: string; lineStyle?: { type?: string } }[] };
        };
        const options = inspectable.chartOptions();
        const frame = inspectable.chartFrameOptions();
        const merge = inspectable.chartMergeOptions();
        expect(options.series.find((series) => series.name === 'Events')?.type).toBe('bar');
        expect(options.series.find((series) => series.name === 'Events (prev.)')?.lineStyle?.type).toBe('dashed');
        expect(frame.series.find((series) => series.data.length > 0)).toBeUndefined();
        expect(merge.xAxis.data.length).toBe(2);
        expect(merge.series.find((series) => series.name === 'Events')?.data).toEqual([5, 9]);
    });
});
