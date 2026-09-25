import { ChangeDetectionStrategy, Component, effect, inject, model, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ColorPickerModule } from '@openng/optimus-ui/colorpicker';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { TextareaModule } from '@openng/optimus-ui/textarea';
import { TranslocoPipe } from '@jsverse/transloco';
import { DialogShell } from '@components/dialog-shell/dialog-shell';
import { ThemeManagerService } from '@services/theme-manager.service';
import { BUILT_IN_THEMES, DEFAULT_THEME_ID, createThemeId, type HitkeepTheme } from '@core/theme/theme.model';

@Component({
    selector: 'app-theme-dialog',
    imports: [DialogShell, ReactiveFormsModule, ButtonModule, ColorPickerModule, InputTextModule, TextareaModule, TranslocoPipe],
    templateUrl: './theme-dialog.html',
    styleUrl: './theme-dialog.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ThemeDialog {
    readonly visible = model(false);

    private readonly themeManager = inject(ThemeManagerService);
    private readonly fb = inject(FormBuilder);

    protected readonly themes = this.themeManager.themes;
    protected readonly activeThemeId = this.themeManager.activeThemeId;
    protected readonly draftIsCustom = signal(false);

    protected readonly form = this.fb.nonNullable.group({
        id: [''],
        name: ['', Validators.required],
        primary: ['#6366f1', Validators.required],
        surface: [''],
        fontFamily: [''],
        customCss: ['']
    });

    constructor() {
        // Open: seed the editor from the active theme. Close: drop any
        // unsaved preview and restore the persisted theme.
        effect(() => {
            if (this.visible()) {
                this.loadTheme(this.themeManager.activeTheme());
            } else {
                this.themeManager.restoreActiveTheme();
            }
        });
        // Live preview: every edit reaches the design tokens immediately so
        // the dashboard behind the dialog shows the result in real time.
        this.form.valueChanges.subscribe(() => {
            if (!this.visible()) {
                return;
            }
            this.themeManager.previewTheme(this.draftFromForm());
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

    protected onVisibleChange(visible: boolean): void {
        this.visible.set(visible);
    }

    private loadTheme(theme: HitkeepTheme): void {
        this.draftIsCustom.set(!theme.builtin);
        this.form.setValue({
            id: theme.id,
            name: theme.name,
            primary: theme.primary,
            surface: theme.surface ?? '',
            fontFamily: theme.fontFamily ?? '',
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
            fontFamily: value.fontFamily.trim() || undefined,
            customCss: value.customCss.trim() || undefined
        };
    }
}
