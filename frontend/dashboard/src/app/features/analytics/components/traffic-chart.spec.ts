import { ComponentFixture, TestBed } from '@angular/core/testing';
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
        const options = (component as unknown as { chartOptions: () => { scales: { y: { ticks: { maxTicksLimit: number; stepSize?: number } } } } }).chartOptions();

        expect(options.scales.y.ticks.maxTicksLimit).toBe(6);
        expect(options.scales.y.ticks.stepSize).toBeUndefined();
    });
});
