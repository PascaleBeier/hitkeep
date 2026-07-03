import { DASHBOARD_LANG_LOCALES, DASHBOARD_LANGUAGE_CODES, DASHBOARD_LOCALE_MAPPING, DEFAULT_DASHBOARD_LANGUAGE, SOURCE_LOCALE, SUPPORTED_LOCALES } from './supported-locales';

describe('supported-locales', () => {
    it('derives provider locale arrays from the dashboard language map', () => {
        expect(DASHBOARD_LANGUAGE_CODES).toEqual(['en', 'de', 'es', 'fr', 'it', 'nl', 'pt']);
        expect(SUPPORTED_LOCALES).toEqual(['en-US', 'de-DE', 'es-ES', 'fr-FR', 'it-IT', 'nl-NL', 'pt-BR']);
        expect(SOURCE_LOCALE).toBe(DASHBOARD_LANG_LOCALES[DEFAULT_DASHBOARD_LANGUAGE]);
    });

    it('maps translation language codes and full locale tags to Intl locales', () => {
        for (const lang of DASHBOARD_LANGUAGE_CODES) {
            expect(DASHBOARD_LOCALE_MAPPING[lang]).toBe(DASHBOARD_LANG_LOCALES[lang]);
        }

        for (const locale of SUPPORTED_LOCALES) {
            expect(DASHBOARD_LOCALE_MAPPING[locale]).toBe(locale);
        }
    });
});
