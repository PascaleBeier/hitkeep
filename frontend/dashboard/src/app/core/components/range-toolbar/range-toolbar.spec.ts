import { Signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { PrimeNG } from 'primeng/config';

import { DEFAULT_RANGE_OPTIONS, RangeOption, RangeToolbar } from './range-toolbar';
import { PrimeLocaleSyncService } from '@core/i18n/prime-locale-sync.service';

describe('RangeToolbar', () => {
    let fixture: ComponentFixture<RangeToolbar>;
    let component: RangeToolbar;
    let transloco: TranslocoService;
    let primeNg: PrimeNG;
    const darkModeTestProperties = [
        '--p-content-background',
        '--p-content-hover-background',
        '--p-content-border-color',
        '--p-surface-0',
        '--p-surface-100',
        '--p-surface-400',
        '--p-surface-700',
        '--p-surface-800',
        '--p-surface-950',
        '--p-text-color',
        '--p-text-muted-color'
    ];

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                RangeToolbar,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                timeRanges: {
                                    today: 'Today',
                                    todayShort: 'Today',
                                    yesterday: 'Yesterday',
                                    yesterdayShort: 'Yesterday',
                                    lastMinute: 'Last minute',
                                    lastMinutes: 'Last {{count}} minutes',
                                    lastHour: 'Last hour',
                                    lastHours: 'Last {{count}} hours',
                                    lastDay: 'Last day',
                                    lastDays: 'Last {{count}} days',
                                    thisWeek: 'This week',
                                    thisWeekShort: 'This week',
                                    lastWeek: 'Last week',
                                    lastWeekShort: 'Last week',
                                    thisMonth: 'This month',
                                    thisMonthShort: 'This month',
                                    lastMonth: 'Last month',
                                    lastMonthShort: 'Last month',
                                    thisYear: 'This year',
                                    thisYearShort: 'YTD',
                                    lastYear: 'Last year',
                                    lastYearShort: '1y',
                                    customRange: 'Custom range',
                                    customShort: 'Custom',
                                    moreRanges: 'More ranges'
                                },
                                actions: {
                                    refresh: 'Refresh'
                                },
                                timeRangeSelectorAria: 'Select range',
                                refreshDataTooltip: 'Refresh'
                            }
                        },
                        de: {
                            common: {
                                timeRanges: {
                                    today: 'Heute',
                                    todayShort: 'Heute',
                                    yesterday: 'Gestern',
                                    yesterdayShort: 'Gestern',
                                    lastMinute: 'Letzte Minute',
                                    lastMinutes: 'Letzte {{count}} Minuten',
                                    lastHour: 'Letzte Stunde',
                                    lastHours: 'Letzte {{count}} Stunden',
                                    lastDay: 'Letzter Tag',
                                    lastDays: 'Letzte {{count}} Tage',
                                    thisWeek: 'Diese Woche',
                                    thisWeekShort: 'Diese Woche',
                                    lastWeek: 'Letzte Woche',
                                    lastWeekShort: 'Letzte Woche',
                                    thisMonth: 'Dieser Monat',
                                    thisMonthShort: 'Dieser Monat',
                                    lastMonth: 'Letzter Monat',
                                    lastMonthShort: 'Letzter Monat',
                                    thisYear: 'Dieses Jahr',
                                    thisYearShort: 'Dieses Jahr',
                                    lastYear: 'Letztes Jahr',
                                    lastYearShort: '1J',
                                    customRange: 'Benutzerdefiniert',
                                    customShort: 'Benutzerdef.',
                                    moreRanges: 'Weitere Zeiträume'
                                },
                                actions: {
                                    refresh: 'Aktualisieren'
                                },
                                timeRangeSelectorAria: 'Zeitraum auswählen',
                                refreshDataTooltip: 'Aktualisieren'
                            }
                        },
                        pt: {
                            common: {
                                timeRanges: {
                                    today: 'Hoje',
                                    todayShort: 'Hoje',
                                    yesterday: 'Ontem',
                                    yesterdayShort: 'Ontem',
                                    lastMinute: 'Último minuto',
                                    lastMinutes: 'Últimos {{count}} minutos',
                                    lastHour: 'Última hora',
                                    lastHours: 'Últimas {{count}} horas',
                                    lastDay: 'Último dia',
                                    lastDays: 'Últimos {{count}} dias',
                                    thisWeek: 'Esta semana',
                                    thisWeekShort: 'Esta sem.',
                                    lastWeek: 'Semana passada',
                                    lastWeekShort: 'Sem. passada',
                                    thisMonth: 'Este mês',
                                    thisMonthShort: 'Este mês',
                                    lastMonth: 'Mês passado',
                                    lastMonthShort: 'Mês passado',
                                    thisYear: 'Este ano',
                                    thisYearShort: 'Este ano',
                                    lastYear: 'Último ano',
                                    lastYearShort: '1a',
                                    customRange: 'Intervalo personalizado',
                                    customShort: 'Personalizado',
                                    moreRanges: 'Mais períodos'
                                },
                                actions: {
                                    refresh: 'Atualizar'
                                },
                                timeRangeSelectorAria: 'Selecionar intervalo',
                                refreshDataTooltip: 'Atualizar'
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en', 'de', 'pt'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                PrimeLocaleSyncService,
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: {
                        en: 'en-US',
                        de: 'de-DE',
                        pt: 'pt-BR'
                    }
                })
            ]
        }).compileComponents();

        transloco = TestBed.inject(TranslocoService);
        primeNg = TestBed.inject(PrimeNG);
        TestBed.inject(PrimeLocaleSyncService);
        fixture = TestBed.createComponent(RangeToolbar);
        component = fixture.componentInstance;
        fixture.componentRef.setInput('timeRanges', DEFAULT_RANGE_OPTIONS);
        fixture.componentRef.setInput('selectedRange', DEFAULT_RANGE_OPTIONS[2] as RangeOption);
        fixture.detectChanges();
    });

    afterEach(() => {
        document.documentElement.classList.remove('p-dark');
        for (const property of darkModeTestProperties) {
            document.documentElement.style.removeProperty(property);
        }
    });

    const translatedLabels = (toolbar: RangeToolbar) => {
        const { translatedTimeRanges } = toolbar as unknown as {
            translatedTimeRanges: Signal<RangeOption[]>;
        };
        return translatedTimeRanges().map((option) => option.label);
    };

    const translatedShortLabels = (toolbar: RangeToolbar) => {
        const { translatedTimeRanges } = toolbar as unknown as {
            translatedTimeRanges: Signal<(RangeOption & { shortLabel: string })[]>;
        };
        return translatedTimeRanges().map((option) => option.shortLabel);
    };

    const datePickerFormat = (toolbar: RangeToolbar) => {
        const { datePickerDateFormat } = toolbar as unknown as {
            datePickerDateFormat: Signal<string>;
        };
        return datePickerDateFormat();
    };

    const datePickerHourFormat = (toolbar: RangeToolbar) => {
        const { datePickerHourFormat } = toolbar as unknown as {
            datePickerHourFormat: Signal<'12' | '24'>;
        };
        return datePickerHourFormat();
    };

    it('translates default ranges from the active language', () => {
        expect(translatedLabels(component)).toEqual([
            'Today',
            'Yesterday',
            'Last 24 hours',
            'Last 7 days',
            'Last 30 days',
            'Last 30 minutes',
            'Last hour',
            'Last 6 hours',
            'Last 3 days',
            'Last 14 days',
            'Last 60 days',
            'Last 90 days',
            'Last 180 days',
            'This week',
            'Last week',
            'This month',
            'Last month',
            'This year',
            'Last year',
            'Custom range'
        ]);
    });

    it('updates translated labels when the active language changes', async () => {
        transloco.setActiveLang('de');
        fixture.detectChanges();
        await fixture.whenStable();

        expect(translatedLabels(component)).toEqual([
            'Heute',
            'Gestern',
            'Letzte 24 Stunden',
            'Letzte 7 Tage',
            'Letzte 30 Tage',
            'Letzte 30 Minuten',
            'Letzte Stunde',
            'Letzte 6 Stunden',
            'Letzte 3 Tage',
            'Letzte 14 Tage',
            'Letzte 60 Tage',
            'Letzte 90 Tage',
            'Letzte 180 Tage',
            'Diese Woche',
            'Letzte Woche',
            'Dieser Monat',
            'Letzter Monat',
            'Dieses Jahr',
            'Letztes Jahr',
            'Benutzerdefiniert'
        ]);
    });

    it('uses localized compact labels when available', async () => {
        transloco.setActiveLang('pt');
        fixture.detectChanges();
        await fixture.whenStable();

        expect(translatedShortLabels(component)).toEqual(['Hoje', 'Ontem', '24h', '7d', '30d', '30m', '1h', '6h', '3d', '14d', '60d', '90d', '180d', 'Esta sem.', 'Sem. passada', 'Este mês', 'Mês passado', 'Este ano', '1a', 'Personalizado']);
    });

    it('emits range changes when a visible preset is selected', () => {
        const selectedRangeEvents: RangeOption[] = [];
        const rangeEvents: { value: RangeOption }[] = [];
        component.selectedRangeChange.subscribe((event) => selectedRangeEvents.push(event));
        component.rangeChange.subscribe((event) => rangeEvents.push(event));

        const yesterdayButton = Array.from<HTMLElement>(fixture.nativeElement.querySelectorAll('p-togglebutton')).find((button) => button.textContent?.trim() === 'Yesterday');
        yesterdayButton?.click();
        fixture.detectChanges();

        expect(selectedRangeEvents[0]?.value).toBe('yesterday');
        expect(rangeEvents[0]?.value.value).toBe('yesterday');
    });

    it('emits range changes when an overflow preset is selected', () => {
        const selectedRangeEvents: RangeOption[] = [];
        const rangeEvents: { value: RangeOption }[] = [];
        component.selectedRangeChange.subscribe((event) => selectedRangeEvents.push(event));
        component.rangeChange.subscribe((event) => rangeEvents.push(event));

        const { overflowMenuItems } = component as unknown as {
            overflowMenuItems: Signal<{ label?: string; command?: () => void }[]>;
        };
        overflowMenuItems()
            .find((item) => item.label === 'Last 14 days')
            ?.command?.();

        expect(selectedRangeEvents[0]?.value).toBe('14d');
        expect(rangeEvents[0]?.value.value).toBe('14d');
    });

    it('keeps the segmented toolbar on dark content surfaces in dark mode', () => {
        document.documentElement.classList.add('p-dark');
        document.documentElement.style.setProperty('--p-content-background', 'rgb(24, 24, 27)');
        document.documentElement.style.setProperty('--p-content-hover-background', 'rgb(39, 39, 42)');
        document.documentElement.style.setProperty('--p-content-border-color', 'rgb(63, 63, 70)');
        document.documentElement.style.setProperty('--p-surface-0', 'rgb(250, 250, 250)');
        document.documentElement.style.setProperty('--p-surface-100', 'rgb(244, 244, 245)');
        document.documentElement.style.setProperty('--p-surface-400', 'rgb(161, 161, 170)');
        document.documentElement.style.setProperty('--p-surface-700', 'rgb(63, 63, 70)');
        document.documentElement.style.setProperty('--p-surface-800', 'rgb(39, 39, 42)');
        document.documentElement.style.setProperty('--p-surface-950', 'rgb(9, 9, 11)');
        document.documentElement.style.setProperty('--p-text-color', 'rgb(250, 250, 250)');
        document.documentElement.style.setProperty('--p-text-muted-color', 'rgb(161, 161, 170)');
        fixture.detectChanges();

        const group = fixture.nativeElement.querySelector('.range-toolbar__range-group') as HTMLElement;
        const segment = fixture.nativeElement.querySelector('p-togglebutton') as HTMLElement;
        const overflowButton = fixture.nativeElement.querySelector('.range-toolbar__more-button') as HTMLElement;
        const groupStyle = getComputedStyle(group);

        expect(groupStyle.backgroundColor).toBe('rgb(39, 39, 42)');
        expect(groupStyle.borderTopColor).toBe('rgb(63, 63, 70)');
        expect(groupStyle.getPropertyValue('--p-togglebutton-content-checked-background').trim()).toBe('rgb(9, 9, 11)');
        expect(groupStyle.getPropertyValue('--p-button-secondary-background').trim()).toBe('rgb(9, 9, 11)');
        expect(getComputedStyle(segment).backgroundColor).not.toBe('rgb(255, 255, 255)');
        expect(getComputedStyle(overflowButton).backgroundColor).not.toBe('rgb(255, 255, 255)');
    });

    it('uses the active locale for the custom date picker format', async () => {
        expect(datePickerFormat(component)).toBe('mm/dd/yy');
        expect(datePickerHourFormat(component)).toBe('12');

        transloco.setActiveLang('de');
        fixture.detectChanges();
        await fixture.whenStable();

        expect(datePickerFormat(component)).toBe('dd.mm.yy');
        expect(datePickerHourFormat(component)).toBe('24');
    });

    it('syncs PrimeNG calendar translations with the active locale', async () => {
        expect(primeNg.translation.dayNames?.[0]).toBe('Sunday');
        expect(primeNg.translation.monthNames?.[0]).toBe('January');
        expect(primeNg.translation.firstDayOfWeek).toBe(0);

        transloco.setActiveLang('de');
        fixture.detectChanges();
        await fixture.whenStable();

        expect(primeNg.translation.dayNames?.[0]).toBe('Sonntag');
        expect(primeNg.translation.monthNames?.[0]).toBe('Januar');
        expect(primeNg.translation.firstDayOfWeek).toBe(1);
    });
});
