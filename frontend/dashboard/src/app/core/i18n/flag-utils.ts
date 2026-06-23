import { getBaseLanguage, normalizeLocaleTag } from './locale-utils';

const EARTH_FLAG_URL = '/flags/other/earth.svg';

const LANGUAGE_FLAG_ASSET_CODES = new Set(['ar', 'eo', 'es-mx', 'ia', 'ie', 'interslavic', 'io', 'la', 'mr', 'non', 'pt-br', 'vo', 'yi']);

const LANGUAGE_FLAG_ASSET_BY_CODE: Record<string, string> = {
    no: 'non',
    nb: 'non',
    nn: 'non'
};

const REPRESENTATIVE_COUNTRY_FLAG_BY_LANGUAGE: Record<string, string> = {
    ar: 'sa',
    cs: 'cz',
    da: 'dk',
    de: 'de',
    en: 'gb',
    es: 'es',
    fi: 'fi',
    fr: 'fr',
    he: 'il',
    hi: 'in',
    it: 'it',
    ja: 'jp',
    ko: 'kr',
    nl: 'nl',
    no: 'no',
    nb: 'no',
    nn: 'no',
    pl: 'pl',
    pt: 'br',
    ru: 'ru',
    sv: 'se',
    th: 'th',
    tr: 'tr',
    uk: 'ua',
    vi: 'vn',
    zh: 'cn'
};

export function countryFlagUrl(value: string | null | undefined): string {
    const code = (value ?? '').trim().toLowerCase();
    if (!/^[a-z]{2}$/.test(code)) {
        return EARTH_FLAG_URL;
    }
    return `/flags/${code}.svg`;
}

export function languageFlagUrl(value: string | null | undefined): string {
    const normalized = normalizeLocaleTag(value ?? '');
    if (!normalized) {
        return EARTH_FLAG_URL;
    }

    const code = getBaseLanguage(normalized) || normalized.toLowerCase();
    if (!/^[a-z]{2,3}$/.test(code)) {
        return EARTH_FLAG_URL;
    }

    const languageAssetCode = LANGUAGE_FLAG_ASSET_BY_CODE[code] ?? code;
    if (LANGUAGE_FLAG_ASSET_CODES.has(languageAssetCode)) {
        return `/flags/language/${languageAssetCode}.svg`;
    }

    const countryCode = REPRESENTATIVE_COUNTRY_FLAG_BY_LANGUAGE[code] ?? code;
    return countryFlagUrl(countryCode);
}

export function localeFlagUrl(locale: string | null | undefined): string {
    const normalized = normalizeLocaleTag(locale ?? '');
    if (!normalized) {
        return EARTH_FLAG_URL;
    }

    const region = normalized.split('-').find((part) => /^[A-Z]{2}$/.test(part));
    if (region) {
        return countryFlagUrl(region);
    }

    return languageFlagUrl(normalized);
}
