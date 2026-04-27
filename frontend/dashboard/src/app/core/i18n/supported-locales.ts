export const SOURCE_LOCALE = 'en-US';
export const SUPPORTED_LOCALES = ['en-US'] as const;

export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];
