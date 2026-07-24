import { Injectable, effect, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { Optimus } from '@openng/optimus-ui/config';
import { Translation } from '@openng/optimus-ui/api';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { TranslocoService } from '@jsverse/transloco';
import { map, switchMap } from 'rxjs';

type TranslationDictionary = Record<string, unknown>;

@Injectable({ providedIn: 'root' })
export class OptimusLocaleSyncService {
    private readonly optimus = inject(Optimus);
    private readonly transloco = inject(TranslocoService);
    private readonly localeService = inject(TranslocoLocaleService);
    private readonly translationState = toSignal(
        this.transloco.langChanges$.pipe(
            switchMap((lang) =>
                this.transloco.selectTranslation(lang).pipe(
                    map((translation) => ({
                        lang,
                        translation: translation as TranslationDictionary
                    }))
                )
            )
        ),
        {
            initialValue: {
                lang: this.transloco.getActiveLang(),
                translation: this.readTranslation(this.transloco.getActiveLang())
            }
        }
    );

    constructor() {
        effect(() => {
            const { translation } = this.translationState();
            this.optimus.setTranslation(this.buildTranslation(this.localeService.getLocale(), translation));
        });
    }

    private buildTranslation(locale: string, translation: TranslationDictionary): Translation {
        return {
            dayNames: this.buildWeekdayNames(locale, 'long'),
            dayNamesShort: this.buildWeekdayNames(locale, 'short'),
            dayNamesMin: this.buildWeekdayNames(locale, 'narrow'),
            monthNames: this.buildMonthNames(locale, 'long'),
            monthNamesShort: this.buildMonthNames(locale, 'short'),
            firstDayOfWeek: this.resolveFirstDayOfWeek(locale),
            dateFormat: this.buildDatePickerDateFormat(locale),
            clear: this.translationValue(translation, 'common.actions.clearAll', 'Clear all'),
            apply: this.translationValue(translation, 'common.actions.apply', 'Apply'),
            cancel: this.translationValue(translation, 'common.actions.cancel', 'Cancel'),
            chooseDate: this.translationValue(translation, 'common.selectDateRange', 'Select date range'),
            am: this.buildDayPeriod(locale, 'am'),
            pm: this.buildDayPeriod(locale, 'pm')
        };
    }

    private readTranslation(lang: string): TranslationDictionary {
        try {
            const translation = this.transloco.getTranslation(lang);
            return translation && typeof translation === 'object' ? (translation as TranslationDictionary) : {};
        } catch {
            return {};
        }
    }

    private translationValue(translation: TranslationDictionary, key: string, fallback: string): string {
        const value = key.split('.').reduce<unknown>((current, segment) => {
            if (!current || typeof current !== 'object') {
                return undefined;
            }
            return (current as TranslationDictionary)[segment];
        }, translation);

        return typeof value === 'string' && value.trim() ? value : fallback;
    }

    private buildWeekdayNames(locale: string, width: 'long' | 'short' | 'narrow'): string[] {
        const formatter = new Intl.DateTimeFormat(locale, { weekday: width, timeZone: 'UTC' });
        const sunday = Date.UTC(2026, 0, 4, 12, 0, 0);
        return Array.from({ length: 7 }, (_, index) => formatter.format(new Date(sunday + index * 24 * 60 * 60 * 1000)));
    }

    private buildMonthNames(locale: string, width: 'long' | 'short'): string[] {
        const formatter = new Intl.DateTimeFormat(locale, { month: width, timeZone: 'UTC' });
        return Array.from({ length: 12 }, (_, index) => formatter.format(new Date(Date.UTC(2026, index, 1, 12, 0, 0))));
    }

    private buildDatePickerDateFormat(locale: string): string {
        const parts = new Intl.DateTimeFormat(locale, {
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            timeZone: 'UTC'
        }).formatToParts(new Date(Date.UTC(2026, 2, 8, 12, 0, 0)));

        return parts
            .map((part) => {
                switch (part.type) {
                    case 'day':
                        return 'dd';
                    case 'month':
                        return 'mm';
                    case 'year':
                        return 'yy';
                    case 'literal':
                        return part.value;
                    default:
                        return '';
                }
            })
            .join('');
    }

    private resolveFirstDayOfWeek(locale: string): number {
        try {
            const weekInfo = (new Intl.Locale(locale) as Intl.Locale & { weekInfo?: { firstDay?: number } }).weekInfo;
            const firstDay = weekInfo?.firstDay;
            if (typeof firstDay === 'number') {
                return firstDay % 7;
            }
        } catch {
            // Ignore unsupported Intl.Locale weekInfo implementations and use the fallback.
        }

        return locale.toLowerCase().startsWith('en-us') ? 0 : 1;
    }

    private buildDayPeriod(locale: string, dayPeriod: 'am' | 'pm'): string {
        const hour = dayPeriod === 'am' ? 9 : 21;
        const formatter = new Intl.DateTimeFormat(locale, {
            hour: 'numeric',
            hour12: true,
            dayPeriod: 'short',
            timeZone: 'UTC'
        });
        const parts = formatter.formatToParts(new Date(Date.UTC(2026, 2, 8, hour, 0, 0)));
        return parts.find((part) => part.type === 'dayPeriod')?.value ?? dayPeriod.toUpperCase();
    }
}
