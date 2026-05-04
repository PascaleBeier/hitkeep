import { NgOptimizedImage } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';

import { takeUntilDestroyed, toSignal } from '@angular/core/rxjs-interop';
import { ChangeDetectionStrategy, Component, computed, effect, inject, signal, DestroyRef } from '@angular/core';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { CopyControl } from '@components/copy-control/copy-control';
import { PageFrame } from '@components/page-frame/page-frame';
import { localeFlagUrl } from '@core/i18n/flag-utils';
import { getBaseLanguage, getLocaleDirection, normalizeLocaleTag, TextDirection } from '@core/i18n/locale-utils';
import { buildTakeoutExportMenuItems, DEFAULT_TAKEOUT_EXPORT_FORMAT, TakeoutExportFormat } from '@core/export/export-formats';
import { SettingsCard } from '@features/settings/components/settings-card';
import { SettingsSecurity } from '@features/settings/components/settings-security';
import { UserPreferences, UserPreferencesService } from '@services/user-preferences.service';
import { UserProfile, UserProfileService } from '@services/user-profile.service';
import { TakeoutDownloadService } from '@services/takeout-download.service';
import { MenuItem } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { InputTextModule } from 'primeng/inputtext';
import { SelectModule } from 'primeng/select';
import { SplitButtonModule } from 'primeng/splitbutton';
import { TabsModule } from 'primeng/tabs';
import { finalize } from 'rxjs';

interface LanguageOption {
    label: string;
    value: string;
    flagUrl: string;
    direction: TextDirection;
}

interface EditableProfile {
    email: string;
    given_name: string;
    last_name: string;
}

type AvailableLang = string | { id: string; label: string };
type UserSettingsTab = 'account' | 'security' | 'export';

@Component({
    selector: 'app-user-settings',
    imports: [ReactiveFormsModule, ButtonModule, InputTextModule, SelectModule, SplitButtonModule, TabsModule, SettingsCard, SettingsSecurity, PageFrame, CopyControl, TranslocoPipe, NgOptimizedImage],
    templateUrl: './user-settings.html',
    styleUrl: './user-settings.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class UserSettings {
    private preferencesService = inject(UserPreferencesService);
    private profileService = inject(UserProfileService);
    private takeoutDownloadService = inject(TakeoutDownloadService);
    private transloco = inject(TranslocoService);
    private destroyRef = inject(DestroyRef);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    private readonly profileFormModel = signal({
        email: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.email, Validators.maxLength(320)] }),
        givenName: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(120)] }),
        lastName: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(120)] })
    });
    protected readonly profileForm = compatForm(this.profileFormModel);

    private readonly preferencesFormModel = signal({
        defaultLocale: new FormControl<LanguageOption | null>(null)
    });
    protected readonly preferencesForm = compatForm(this.preferencesFormModel);

    protected readonly isProfileLoading = this.profileService.isLoading;
    protected readonly isProfileSaving = this.profileService.isSaving;
    protected readonly profileLoadError = signal<string | null>(null);
    protected readonly profileSaveError = signal<string | null>(null);
    protected readonly profileSaveState = signal<'idle' | 'saved' | 'error'>('idle');
    protected readonly initialProfile = signal<EditableProfile | null>(null);
    protected readonly profile = this.profileService.profile;

    protected readonly isLoading = this.preferencesService.isLoading;
    protected readonly isSaving = this.preferencesService.isSaving;
    protected readonly loadError = signal<string | null>(null);
    protected readonly saveState = signal<'idle' | 'saved' | 'error'>('idle');
    protected readonly initialPreferences = signal<UserPreferences | null>(null);
    protected readonly isExporting = signal(false);
    protected readonly exportState = signal<'idle' | 'success' | 'error'>('idle');
    protected readonly activeTab = signal<UserSettingsTab>('account');

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        return [{ label: this.transloco.translate('settings.user.breadcrumb'), isCurrent: true }];
    });
    protected readonly exportMenuItems = computed<MenuItem[]>(() => {
        this.activeLanguage();
        return buildTakeoutExportMenuItems(this.transloco, (format) => this.downloadData(format));
    });

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

    protected readonly canSaveProfile = computed(() => {
        if (this.isProfileLoading() || this.isProfileSaving()) return false;
        if (this.profileForm().invalid()) return false;
        const initial = this.initialProfile();
        if (!initial) return false;
        return !this.profilesEqual(initial, this.currentProfile());
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

    constructor() {
        effect(() => {
            const shouldDisable = this.isProfileLoading() || this.isProfileSaving();
            const controls = [this.profileForm.email().control(), this.profileForm.givenName().control(), this.profileForm.lastName().control()];
            for (const control of controls) {
                if (shouldDisable && control.enabled) {
                    control.disable({ emitEvent: false });
                } else if (!shouldDisable && control.disabled) {
                    control.enable({ emitEvent: false });
                }
            }
        });

        effect(() => {
            const profile = this.profileService.profile();
            if (!profile) return;
            const editable = this.editableProfile(profile);
            this.profileForm.email().control().setValue(editable.email, { emitEvent: false });
            this.profileForm.givenName().control().setValue(editable.given_name, { emitEvent: false });
            this.profileForm.lastName().control().setValue(editable.last_name, { emitEvent: false });
            this.initialProfile.set(editable);
            this.profileSaveState.set('idle');
            this.profileSaveError.set(null);
            this.profileLoadError.set(null);
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

    protected saveProfile(): void {
        if (this.profileForm().invalid()) {
            this.markProfileFieldsTouched();
            return;
        }
        if (!this.canSaveProfile()) {
            return;
        }

        this.profileSaveState.set('idle');
        this.profileSaveError.set(null);
        const payload = this.currentProfile();

        this.profileService.updateProfile(payload).subscribe({
            next: (profile) => {
                this.initialProfile.set(this.editableProfile(profile));
                this.profileSaveState.set('saved');
            },
            error: (error) => {
                this.profileSaveState.set('error');
                this.profileSaveError.set(this.profileErrorKey(error));
            }
        });
    }

    protected onProfileFieldChange(): void {
        if (this.profileSaveState() !== 'idle') {
            this.profileSaveState.set('idle');
        }
        if (this.profileSaveError()) {
            this.profileSaveError.set(null);
        }
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

    protected onLocaleChange(): void {
        if (this.saveState() !== 'idle') {
            this.saveState.set('idle');
        }
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

    protected downloadData(format: TakeoutExportFormat = DEFAULT_TAKEOUT_EXPORT_FORMAT): void {
        if (this.isExporting()) return;

        this.isExporting.set(true);
        this.exportState.set('idle');

        this.takeoutDownloadService
            .downloadUserTakeout(format)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isExporting.set(false))
            )
            .subscribe({
                next: () => this.exportState.set('success'),
                error: () => this.exportState.set('error')
            });
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

    private currentProfile(): EditableProfile {
        return {
            email: this.profileForm.email().value().trim().toLowerCase(),
            given_name: this.profileForm.givenName().value().trim(),
            last_name: this.profileForm.lastName().value().trim()
        };
    }

    private editableProfile(profile: UserProfile): EditableProfile {
        return {
            email: profile.email,
            given_name: (profile.given_name ?? '').trim(),
            last_name: (profile.last_name ?? '').trim()
        };
    }

    private profilesEqual(a: EditableProfile, b: EditableProfile): boolean {
        return a.email === b.email && a.given_name === b.given_name && a.last_name === b.last_name;
    }

    private profileErrorKey(error: unknown): string {
        if (!(error instanceof HttpErrorResponse)) {
            return 'settings.user.profile.errors.updateFailed';
        }

        if (error.status === 409) {
            return 'settings.user.profile.errors.emailTaken';
        }
        if (error.status === 404) {
            return 'settings.user.profile.errors.notFound';
        }
        if (error.status === 400) {
            return 'settings.user.profile.errors.invalidInput';
        }

        return 'settings.user.profile.errors.updateFailed';
    }

    private markProfileFieldsTouched(): void {
        this.profileForm.email().markAsTouched();
        this.profileForm.givenName().markAsTouched();
        this.profileForm.lastName().markAsTouched();
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
