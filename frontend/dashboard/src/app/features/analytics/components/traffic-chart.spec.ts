import { ComponentFixture, TestBed } from '@angular/core/testing';
import { SeriesChart } from '@features/analytics/components/series-chart';
import { TrafficChart } from '@features/analytics/components/traffic-chart';
import { By } from '@angular/platform-browser';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';

describe('TrafficChart', () => {
    let component: TrafficChart;
    let fixture: ComponentFixture<TrafficChart>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                TrafficChart,
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

        fixture = TestBed.createComponent(TrafficChart);
        component = fixture.componentInstance;
        fixture.componentRef.setInput('data', []);
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('A11Y: container should have img role and accessible label', () => {
        const container = fixture.debugElement.query(By.css('div[role="img"]'));
        expect(container).toBeTruthy();
        expect(container.nativeElement.getAttribute('aria-label')).toBeTruthy();
    });

    it('A11Y: loading state should be polite aria-live', () => {
        fixture.componentRef.setInput('isLoading', true);
        fixture.detectChanges();
        const loader = fixture.debugElement.query(By.css('[aria-live="polite"]'));
        expect(loader).toBeTruthy();
    });

    it('should cap y-axis ticks without forcing one tick per pageview', () => {
        const seriesChart = fixture.debugElement.query(By.directive(SeriesChart)).componentInstance as unknown as {
            chartFrameOptions: () => { yAxis: { splitNumber: number; interval?: number } };
        };
        const options = seriesChart.chartFrameOptions();

        expect(options.yAxis.splitNumber).toBe(6);
        expect(options.yAxis.interval).toBeUndefined();
    });

    it('renders the ECharts directive instead of the PrimeNG chart element when traffic exists', () => {
        fixture.componentRef.setInput('data', [
            { time: '2026-07-01T00:00:00Z', pageviews: 12, visitors: 7 },
            { time: '2026-07-02T00:00:00Z', pageviews: 18, visitors: 10 }
        ]);
        fixture.detectChanges();

        expect(fixture.debugElement.query(By.css('[echarts]'))).toBeTruthy();
        expect(fixture.debugElement.query(By.css('app-chart-design-toggle'))).toBeTruthy();
        expect(fixture.debugElement.query(By.css('p-' + 'chart'))).toBeNull();
    });

    it('can render primary traffic series with the bar design variant and merge data updates', () => {
        fixture.componentRef.setInput('data', [
            { time: '2026-07-01T00:00:00Z', pageviews: 12, visitors: 7 },
            { time: '2026-07-02T00:00:00Z', pageviews: 18, visitors: 10 }
        ]);
        fixture.componentRef.setInput('design', 'bar');
        fixture.detectChanges();

        const inspectable = fixture.debugElement.query(By.directive(SeriesChart)).componentInstance as unknown as {
            chartFrameOptions: () => { series: { data: number[] }[] };
            chartMergeOptions: () => { xAxis: { data: string[] }; series: { name: string; type: string; data: number[] }[] };
        };
        const frame = inspectable.chartFrameOptions();
        const merge = inspectable.chartMergeOptions();
        expect(merge.series.find((series) => series.name === 'dashboard.kpis.pageviews')?.type).toBe('bar');
        expect(merge.series.find((series) => series.name === 'dashboard.traffic.trendLine')?.type).toBe('line');
        expect(frame.series.find((series) => series.data.length > 0)).toBeUndefined();
        expect(merge.xAxis.data.length).toBe(2);
        expect(merge.series.find((series) => series.name === 'dashboard.kpis.pageviews')?.data).toEqual([12, 18]);
    });
});
