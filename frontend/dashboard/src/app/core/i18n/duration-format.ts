type DurationUnit = 'year' | 'month' | 'week' | 'day' | 'hour' | 'minute' | 'second';

const DURATION_UNITS: readonly { unit: DurationUnit; seconds: number }[] = [
    { unit: 'year', seconds: 365 * 24 * 60 * 60 },
    { unit: 'month', seconds: 30 * 24 * 60 * 60 },
    { unit: 'week', seconds: 7 * 24 * 60 * 60 },
    { unit: 'day', seconds: 24 * 60 * 60 },
    { unit: 'hour', seconds: 60 * 60 },
    { unit: 'minute', seconds: 60 },
    { unit: 'second', seconds: 1 }
];

const LANGUAGE_LOCALES: Record<string, string> = {
    en: 'en-US',
    de: 'de-DE',
    es: 'es-ES',
    fr: 'fr-FR',
    it: 'it-IT'
};

export function localeForLanguage(language: string): string {
    return LANGUAGE_LOCALES[language] ?? (language || 'en-US');
}

export function formatDurationInterval(totalSeconds: number, languageOrLocale: string, unitDisplay: Intl.NumberFormatOptions['unitDisplay'] = 'long'): string {
    const seconds = Math.max(0, Math.ceil(totalSeconds));
    const selected = DURATION_UNITS.find((unit) => seconds >= unit.seconds) ?? DURATION_UNITS[DURATION_UNITS.length - 1];
    const value = selected.unit === 'second' ? seconds : Math.ceil(seconds / selected.seconds);

    return new Intl.NumberFormat(localeForLanguage(languageOrLocale), {
        style: 'unit',
        unit: selected.unit,
        unitDisplay,
        maximumFractionDigits: 0
    }).format(value);
}
