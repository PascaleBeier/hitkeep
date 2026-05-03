import { Injectable, effect, inject, signal } from '@angular/core';
import { HttpClient, HttpContext } from '@angular/common/http';
import { finalize, tap } from 'rxjs';
import { TranslocoService } from '@jsverse/transloco';
import { getBaseLanguage, getLocaleDirection, normalizeLocaleTag } from '@core/i18n/locale-utils';
import { SKIP_AUTH_REDIRECT } from '@core/interceptors/auth.interceptor';

export interface UserPreferences {
    default_locale: string;
    dismissed_onboarding_at?: string;
}

type AvailableLang = string | { id: string; label: string };

@Injectable({ providedIn: 'root' })
export class UserPreferencesService {
    private http = inject(HttpClient);
    private transloco = inject(TranslocoService);

    readonly preferences = signal<UserPreferences | null>(null);
    readonly isLoading = signal(false);
    readonly isSaving = signal(false);

    constructor() {
        if (typeof document === 'undefined') {
            return;
        }

        effect(() => {
            const prefs = this.preferences();
            if (!prefs?.default_locale) return;
            const normalized = normalizeLocaleTag(prefs.default_locale);
            if (!normalized) return;
            const base = getBaseLanguage(normalized) || normalized;
            const translationLang = this.resolveTranslationLang(base);

            document.documentElement.lang = normalized;
            document.documentElement.dir = getLocaleDirection(normalized);
            if (this.transloco.getActiveLang() !== translationLang) {
                this.transloco.setActiveLang(translationLang);
            }
        });
    }

    load(options?: { skipAuthRedirect?: boolean }) {
        this.isLoading.set(true);
        const context = options?.skipAuthRedirect ? new HttpContext().set(SKIP_AUTH_REDIRECT, true) : undefined;
        return this.http.get<UserPreferences>('/api/user/preferences', { context }).pipe(
            tap((prefs) => this.applyPreferences(prefs)),
            finalize(() => {
                this.isLoading.set(false);
            })
        );
    }

    save(preferences: UserPreferences) {
        this.isSaving.set(true);
        return this.http.put<UserPreferences>('/api/user/preferences', preferences).pipe(
            tap((prefs) => this.applyPreferences(prefs)),
            finalize(() => {
                this.isSaving.set(false);
            })
        );
    }

    applyPreferences(prefs: UserPreferences) {
        this.preferences.set(prefs);
    }

    private resolveTranslationLang(locale: string): string {
        const normalized = normalizeLocaleTag(locale);
        const base = getBaseLanguage(normalized) || normalized;
        const available = this.transloco.getAvailableLangs() as AvailableLang[];
        const availableIds = available
            .map((entry) => (typeof entry === 'string' ? entry : entry.id))
            .map((entry) => normalizeLocaleTag(entry))
            .filter((entry): entry is string => Boolean(entry));

        if (base && availableIds.includes(base)) {
            return base;
        }
        if (base) {
            const scoped = availableIds.find((entry) => entry.startsWith(`${base}-`));
            if (scoped) {
                return scoped;
            }
        }
        return this.transloco.getDefaultLang();
    }
}
