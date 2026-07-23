import { NgOptimizedImage } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';

import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { toSignal } from '@angular/core/rxjs-interop';
import { ButtonModule } from '@openng/optimus-ui/button';
import { CardModule } from '@openng/optimus-ui/card';
import { SelectModule } from '@openng/optimus-ui/select';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { localeFlagUrl } from '@core/i18n/flag-utils';
import { UserPreferences, UserPreferencesService } from '@services/user-preferences.service';
import { getBaseLanguage, getLocaleDirection, normalizeLocaleTag, TextDirection } from '@core/i18n/locale-utils';

interface LanguageOption {
    label: string;
    value: string;
    flagUrl: string;
    direction: TextDirection;
}

type AvailableLang = string | { id: string; label: string };

@Component({
    selector: 'app-preferences',
    imports: [ReactiveFormsModule, ButtonModule, CardModule, SelectModule, PageHeader, PageHeaderLeft, PageBreadcrumb, TranslocoPipe, NgOptimizedImage],
    templateUrl: './preferences.html',
    styleUrl: './preferences.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class Preferences {
    private preferencesService = inject(UserPreferencesService);
    private transloco = inject(TranslocoService);
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });
    private readonly preferencesFormModel = signal({
        defaultLocale: new FormControl<LanguageOption | null>(null)
    });
    protected readonly preferencesForm = compatForm(this.preferencesFormModel);

    protected readonly isLoading = this.preferencesService.isLoading;
    protected readonly isSaving = this.preferencesService.isSaving;
    protected readonly loadError = signal<string | null>(null);
    protected readonly saveState = signal<'idle' | 'saved' | 'error'>('idle');
    protected readonly initialPreferences = signal<UserPreferences | null>(null);

    protected readonly languageOptions = computed(() => {
        this.activeLanguage();
        const options: LanguageOption[] = [];
        const seen = new Set<string>();
        const available = this.transloco.getAvailableLangs() as AvailableLang[];

        for (const entry of available) {
            const raw = typeof entry === 'string' ? entry : entry.id;
            const normalized = normalizeLocaleTag(raw);
            const base = getBaseLanguage(normalized) || normalized;
            if (!base || seen.has(base)) {
                continue;
            }
            seen.add(base);

            const label = this.optionLabel(base);
            options.push({
                value: base,
                label,
                flagUrl: localeFlagUrl(base),
                direction: getLocaleDirection(base)
            });
        }

        return options.sort((a, b) => a.label.localeCompare(b.label, 'en', { sensitivity: 'base' }));
    });
    protected readonly languageOptionsByValue = computed(() => {
        const byValue = new Map<string, LanguageOption>();
        for (const option of this.languageOptions()) {
            byValue.set(option.value, option);
        }
        return byValue;
    });

    protected readonly canSave = computed(() => {
        if (this.isSaving()) return false;
        if (this.isLoading()) return false;
        const initial = this.initialPreferences();
        if (!initial) return false;
        const current = this.currentPreferences();
        if (!current.default_locale) return false;
        return !this.preferencesEqual(initial, current);
    });

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        return [{ label: this.transloco.translate('preferences.breadcrumb'), isCurrent: true }];
    });

    constructor() {
        this.preferencesService.load().subscribe({
            error: () => {
                this.loadError.set('preferences.errors.loadFailed');
            }
        });

        effect(() => {
            const control = this.preferencesForm.defaultLocale().control();
            const shouldDisable = this.isLoading() || this.isSaving();
            if (shouldDisable && control.enabled) {
                control.disable({ emitEvent: false });
            } else if (!shouldDisable && control.disabled) {
                control.enable({ emitEvent: false });
            }
        });

        effect(() => {
            const prefs = this.preferencesService.preferences();
            if (!prefs) return;
            const base = getBaseLanguage(prefs.default_locale);
            const selectedLocale = base || prefs.default_locale;
            const selectedOption = this.languageOptionsByValue().get(selectedLocale) ?? null;
            this.preferencesForm.defaultLocale().control().setValue(selectedOption, { emitEvent: false });
            this.initialPreferences.set({
                default_locale: selectedLocale
            });
            this.saveState.set('idle');
            this.loadError.set(null);
        });
    }

    protected save() {
        if (!this.canSave()) return;
        const payload = this.currentPreferences();
        this.saveState.set('idle');
        this.preferencesService.save(payload).subscribe({
            next: (prefs) => {
                this.initialPreferences.set(prefs);
                this.saveState.set('saved');
            },
            error: () => {
                this.saveState.set('error');
            }
        });
    }

    protected onLocaleChange(option: LanguageOption | null | undefined): void {
        if (this.saveState() !== 'idle') {
            this.saveState.set('idle');
        }
        this.applyLocalePreview(option ?? null);
    }

    protected optionLabel(locale: string): string {
        const normalized = normalizeLocaleTag(locale);
        if (!normalized) return locale;
        const uiLanguage = getBaseLanguage(this.activeLanguage()) || 'en';
        const languageNames = this.displayNames('language', uiLanguage);
        const language = normalized.split('-')[0];
        const languageName = languageNames?.of(language) ?? language;
        return languageName;
    }

    protected currentPreferences(): UserPreferences {
        const selected = this.preferencesForm.defaultLocale().value();
        const defaultLocale = normalizeLocaleTag(selected?.value ?? '');
        const base = getBaseLanguage(defaultLocale);
        const normalizedDefault = base || '';
        return {
            default_locale: normalizedDefault
        };
    }

    protected preferencesEqual(a: UserPreferences, b: UserPreferences): boolean {
        return a.default_locale === b.default_locale;
    }
    private applyLocalePreview(locale: LanguageOption | string | null | undefined): void {
        const resolvedLocale = typeof locale === 'string' ? locale : locale?.value;
        if (!resolvedLocale) {
            return;
        }

        const normalized = normalizeLocaleTag(resolvedLocale);
        const base = getBaseLanguage(normalized) || normalized;
        if (!base) {
            return;
        }

        const translationLang = this.resolveTranslationLang(base);
        if (translationLang && this.transloco.getActiveLang() !== translationLang) {
            this.transloco.setActiveLang(translationLang);
        }

        if (typeof document !== 'undefined') {
            document.documentElement.lang = base;
            document.documentElement.dir = getLocaleDirection(base);
        }
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

    private displayNames(type: 'language' | 'region' | 'script', locale: string): Intl.DisplayNames | null {
        if (typeof Intl === 'undefined' || !('DisplayNames' in Intl)) {
            return null;
        }
        try {
            return new Intl.DisplayNames([locale], { type });
        } catch {
            return null;
        }
    }
}
