import { ChangeDetectionStrategy, Component, effect, inject, model, signal } from '@angular/core';
import { FormBuilder, FormsModule, ReactiveFormsModule, Validators } from '@angular/forms';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ColorPickerModule } from '@openng/optimus-ui/colorpicker';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { SelectModule } from '@openng/optimus-ui/select';
import { TextareaModule } from '@openng/optimus-ui/textarea';
import { TranslocoPipe } from '@jsverse/transloco';
import { DialogShell } from '@components/dialog-shell/dialog-shell';
import { PreferencesService, type HitkeepColorMode } from '@services/preferences.service';
import { ThemeManagerService } from '@services/theme-manager.service';
import { BUILT_IN_THEMES, DEFAULT_THEME_ID, createThemeId, type HitkeepTheme } from '@core/theme/theme.model';

@Component({
    selector: 'app-theme-dialog',
    imports: [DialogShell, ReactiveFormsModule, FormsModule, ButtonModule, ColorPickerModule, InputTextModule, SelectModule, TextareaModule, TranslocoPipe],
    templateUrl: './theme-dialog.html',
    styleUrl: './theme-dialog.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ThemeDialog {
    readonly visible = model(false);

    private readonly themeManager = inject(ThemeManagerService);
    private readonly prefs = inject(PreferencesService);
    private readonly fb = inject(FormBuilder);

    protected readonly themes = this.themeManager.themes;
    protected readonly activeThemeId = this.themeManager.activeThemeId;
    protected readonly colorMode = this.prefs.colorMode;
    protected readonly draftIsCustom = signal(false);

    protected readonly colorModeOptions: { labelKey: string; value: HitkeepColorMode }[] = [
        { labelKey: 'theme.mode.auto', value: 'auto' },
        { labelKey: 'theme.mode.light', value: 'light' },
        { labelKey: 'theme.mode.dark', value: 'dark' }
    ];

    protected readonly densityOptions: { labelKey: string; value: 'comfortable' | 'compact' }[] = [
        { labelKey: 'theme.fields.densityComfortable', value: 'comfortable' },
        { labelKey: 'theme.fields.densityCompact', value: 'compact' }
    ];

    protected readonly menuSpacingOptions: { labelKey: string; value: 'default' | 'compact' }[] = [
        { labelKey: 'theme.fields.menuSpacingDefault', value: 'default' },
        { labelKey: 'theme.fields.menuSpacingCompact', value: 'compact' }
    ];

    protected readonly form = this.fb.nonNullable.group({
        id: [''],
        name: ['', Validators.required],
        primary: ['#6366f1', Validators.required],
        surface: [''],
        accent: [''],
        fontFamily: [''],
        fontSize: [''],
        density: this.fb.nonNullable.control<'comfortable' | 'compact'>('comfortable'),
        menuSpacing: this.fb.nonNullable.control<'default' | 'compact'>('default'),
        customCss: ['']
    });

    constructor() {
        // Open: seed the editor from the active theme.
        effect(() => {
            if (this.visible()) {
                this.loadTheme(this.themeManager.activeTheme());
            }
        });
        // Live preview: every edit reaches the design tokens immediately so
        // the dashboard behind the dialog shows the result in real time. The
        // valueChanges stream fires inside change detection, where signal
        // writes (design tokens, version bump) are rejected (NG0103), so the
        // apply is deferred to a microtask after the current pass.
        this.form.valueChanges.subscribe(() => {
            if (!this.visible()) {
                return;
            }
            const draft = this.draftFromForm();
            queueMicrotask(() => this.themeManager.previewTheme(draft));
        });
    }

    protected selectTheme(theme: HitkeepTheme): void {
        this.loadTheme(theme);
    }

    protected save(): void {
        if (this.form.invalid) {
            this.form.markAllAsTouched();
            return;
        }
        const draft = this.draftFromForm();
        const isBuiltin = BUILT_IN_THEMES.some((theme) => theme.id === draft.id);
        this.themeManager.saveTheme({
            ...draft,
            id: isBuiltin ? createThemeId() : draft.id
        });
        this.visible.set(false);
    }

    protected deleteDraft(): void {
        const id = this.form.controls.id.value;
        if (!id || !this.draftIsCustom()) {
            return;
        }
        this.themeManager.deleteTheme(id);
        this.loadTheme(this.themeManager.activeTheme());
    }

    protected resetDefaults(): void {
        this.loadTheme(BUILT_IN_THEMES[0]);
        this.themeManager.setActiveTheme(DEFAULT_THEME_ID);
    }

    protected onColorModeChange(mode: HitkeepColorMode): void {
        this.prefs.setColorMode(mode);
    }

    protected onVisibleChange(visible: boolean): void {
        if (!visible) {
            // Dialog dismissed without saving: drop the unsaved preview.
            this.themeManager.restoreActiveTheme();
        }
        this.visible.set(visible);
    }

    private loadTheme(theme: HitkeepTheme): void {
        this.draftIsCustom.set(!theme.builtin);
        this.form.setValue({
            id: theme.id,
            name: theme.name,
            primary: theme.primary,
            surface: theme.surface ?? '',
            accent: theme.accent ?? '',
            fontFamily: theme.fontFamily ?? '',
            fontSize: theme.fontSize ?? '',
            density: theme.density ?? 'comfortable',
            menuSpacing: theme.menuSpacing ?? 'default',
            customCss: theme.customCss ?? ''
        });
    }

    private draftFromForm(): HitkeepTheme {
        const value = this.form.getRawValue();
        return {
            id: value.id || createThemeId(),
            name: value.name.trim() || 'Custom',
            builtin: false,
            primary: value.primary,
            surface: value.surface.trim() || undefined,
            accent: value.accent.trim() || undefined,
            fontFamily: value.fontFamily.trim() || undefined,
            fontSize: value.fontSize.trim() || undefined,
            density: value.density,
            menuSpacing: value.menuSpacing,
            customCss: value.customCss.trim() || undefined
        };
    }
}
