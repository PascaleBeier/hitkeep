export const DASHBOARD_LANG_LOCALES = {
    en: 'en-US',
    de: 'de-DE',
    es: 'es-ES',
    fr: 'fr-FR',
    it: 'it-IT',
    nl: 'nl-NL',
    pt: 'pt-BR'
} as const;

export type DashboardLanguage = keyof typeof DASHBOARD_LANG_LOCALES;
export type SupportedLocale = (typeof DASHBOARD_LANG_LOCALES)[DashboardLanguage];

export const DEFAULT_DASHBOARD_LANGUAGE = 'en' satisfies DashboardLanguage;
export const SOURCE_LOCALE = DASHBOARD_LANG_LOCALES[DEFAULT_DASHBOARD_LANGUAGE];
export const DASHBOARD_LANGUAGE_CODES = Object.keys(DASHBOARD_LANG_LOCALES) as DashboardLanguage[];
export const SUPPORTED_LOCALES = Object.values(DASHBOARD_LANG_LOCALES) as SupportedLocale[];
export const DASHBOARD_LOCALE_MAPPING = Object.fromEntries([...Object.entries(DASHBOARD_LANG_LOCALES), ...SUPPORTED_LOCALES.map((locale) => [locale, locale])]) as Record<DashboardLanguage | SupportedLocale, SupportedLocale>;
