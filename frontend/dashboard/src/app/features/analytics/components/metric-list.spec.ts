import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { MetricList } from '@features/analytics/components/metric-list';

describe('MetricList', () => {
    let fixture: ComponentFixture<MetricList>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                MetricList,
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

        fixture = TestBed.createComponent(MetricList);
        fixture.componentRef.setInput('title', 'Top devices');
        fixture.componentRef.setInput('icon', 'pi-mobile');
        fixture.componentRef.setInput('data', [
            { name: 'Desktop', value: 70 },
            { name: 'Mobile', value: 30 }
        ]);
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(fixture.componentInstance).toBeTruthy();
    });

    it('should show distribution percentages for the rows', () => {
        expect(fixture.nativeElement.textContent).toContain('70%');
        expect(fixture.nativeElement.textContent).toContain('30%');
    });

    it('should render duplicate row labels without collapsing rows', () => {
        fixture.componentRef.setInput('data', [
            { name: '/docs', value: 70 },
            { name: '/docs', value: 30 }
        ]);
        fixture.detectChanges();

        const rows = fixture.debugElement.queryAll(By.css('.metric-list__row:not(.metric-list__row--empty)'));
        expect(rows.length).toBe(2);
        expect(fixture.nativeElement.textContent).toContain('70%');
        expect(fixture.nativeElement.textContent).toContain('30%');
    });

    it('should show an empty state instead of a zero-value row when there is no data', () => {
        fixture.componentRef.setInput('data', []);
        fixture.detectChanges();

        expect(fixture.debugElement.query(By.css('.metric-list__row--empty'))).not.toBeNull();
        expect(fixture.nativeElement.textContent).toContain('common.empty.noDataTitle');
        expect(fixture.nativeElement.textContent).not.toContain('0');
    });

    it('should mark the active row with the inset active style class', () => {
        fixture.componentRef.setInput('activeValue', 'Desktop');
        fixture.detectChanges();

        const active = fixture.debugElement.query(By.css('.metric-list__row--active'));
        expect(active).not.toBeNull();
        expect(active.nativeElement.textContent).toContain('Desktop');
    });

    it('should render distinct device icons', () => {
        fixture.componentRef.setInput('data', [
            { name: 'Desktop', value: 70 },
            { name: 'Tablet', value: 20 },
            { name: 'Mobile', value: 10 }
        ]);
        fixture.detectChanges();

        const icons = fixture.debugElement.queryAll(By.css('.metric-list__item-icon'));
        const classes = icons.map((icon) => icon.nativeElement.className as string);

        expect(classes.some((value) => value.includes('pi-desktop'))).toBeTruthy();
        expect(classes.some((value) => value.includes('pi-tablet'))).toBeTruthy();
        expect(classes.some((value) => value.includes('pi-mobile'))).toBeTruthy();
    });

    it('should not render a leading icon for top pages', () => {
        fixture.componentRef.setInput('icon', 'pi-file');
        fixture.componentRef.setInput('linkMode', 'path');
        fixture.componentRef.setInput('siteDomain', 'example.com');
        fixture.componentRef.setInput('data', [{ name: '/pricing', value: 12 }]);
        fixture.detectChanges();

        expect(fixture.debugElement.query(By.css('.metric-list__item-icon'))).toBeNull();
    });

    it('should render human-readable language names when enabled', () => {
        fixture.componentRef.setInput('title', 'Audience');
        fixture.componentRef.setInput('icon', 'pi-globe');
        fixture.componentRef.setInput('showLanguageNames', true);
        fixture.componentRef.setInput('data', [{ name: 'de', value: 12 }]);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('German');
    });

    it('should render representative flags for languages when enabled', () => {
        fixture.componentRef.setInput('title', 'Audience');
        fixture.componentRef.setInput('icon', 'pi-globe');
        fixture.componentRef.setInput('showLanguageFlags', true);
        fixture.componentRef.setInput('data', [{ name: 'en', value: 12 }]);
        fixture.detectChanges();

        const flag = fixture.debugElement.query(By.css('.metric-list__flag'));
        expect(flag).not.toBeNull();
        expect(flag.nativeElement.getAttribute('ngsrc') ?? flag.nativeElement.getAttribute('src')).toContain('/flags/gb.svg');
    });

    it('should map Czech to the Czech Republic flag', () => {
        fixture.componentRef.setInput('title', 'Audience');
        fixture.componentRef.setInput('icon', 'pi-globe');
        fixture.componentRef.setInput('showLanguageFlags', true);
        fixture.componentRef.setInput('data', [{ name: 'cs', value: 12 }]);
        fixture.detectChanges();

        const flag = fixture.debugElement.query(By.css('.metric-list__flag'));
        expect(flag.nativeElement.getAttribute('ngsrc') ?? flag.nativeElement.getAttribute('src')).toContain('/flags/cz.svg');
    });

    it('should map Norwegian Bokmal to a Norwegian flag', () => {
        fixture.componentRef.setInput('title', 'Audience');
        fixture.componentRef.setInput('icon', 'pi-globe');
        fixture.componentRef.setInput('showLanguageFlags', true);
        fixture.componentRef.setInput('data', [{ name: 'nb', value: 12 }]);
        fixture.detectChanges();

        const flag = fixture.debugElement.query(By.css('.metric-list__flag'));
        const source = flag.nativeElement.getAttribute('ngsrc') ?? flag.nativeElement.getAttribute('src');
        expect(source.includes('/flags/language/non.svg') || source.includes('/flags/no.svg')).toBeTruthy();
    });
});
