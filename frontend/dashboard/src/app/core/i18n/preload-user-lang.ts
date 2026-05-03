import { inject, provideAppInitializer } from '@angular/core';
import { TranslocoService } from '@jsverse/transloco';
import { firstValueFrom } from 'rxjs';
import { getBaseLanguage, getLocaleDirection, normalizeLocaleTag } from '@core/i18n/locale-utils';

type AvailableLang = string | { id: string; label: string };

function availableLangIds(transloco: TranslocoService): string[] {
    return (transloco.getAvailableLangs() as AvailableLang[])
        .map((entry) => (typeof entry === 'string' ? entry : entry.id))
        .map((entry) => normalizeLocaleTag(entry))
        .filter((entry): entry is string => Boolean(entry));
}

function resolveTranslationLang(locale: string, available: string[], fallback: string): string {
    const normalized = normalizeLocaleTag(locale);
    if (!normalized) return fallback;
    if (available.includes(normalized)) return normalized;

    const base = getBaseLanguage(normalized) || normalized;
    if (available.includes(base)) return base;

    const scoped = available.find((entry) => entry.startsWith(`${base}-`));
    return scoped ?? fallback;
}

function browserLocaleCandidates(): string[] {
    if (typeof navigator === 'undefined') return [];
    const candidates = [...(navigator.languages ?? []), navigator.language];
    return Array.from(new Set(candidates.map((entry) => normalizeLocaleTag(entry ?? '')).filter((entry): entry is string => Boolean(entry))));
}

function applyDocumentLocale(locale: string): void {
    if (typeof document === 'undefined') return;
    const normalized = normalizeLocaleTag(locale);
    if (!normalized) return;
    document.documentElement.lang = normalized;
    document.documentElement.dir = getLocaleDirection(normalized);
}

async function activateLanguage(transloco: TranslocoService, lang: string, locale: string): Promise<void> {
    if (transloco.getActiveLang() !== lang) {
        transloco.setActiveLang(lang);
    }
    await firstValueFrom(transloco.load(lang));
    applyDocumentLocale(locale);
}

export function preloadUserLang() {
    const transloco = inject(TranslocoService);

    return (async () => {
        const available = availableLangIds(transloco);
        const defaultLang = resolveTranslationLang(transloco.getDefaultLang(), available, 'en');

        const browserLocale = browserLocaleCandidates()[0] ?? defaultLang;
        const browserLang = resolveTranslationLang(browserLocale, available, defaultLang);
        await activateLanguage(transloco, browserLang, browserLocale);
    })();
}

export function providePreloadUserLang() {
    return provideAppInitializer(preloadUserLang);
}
