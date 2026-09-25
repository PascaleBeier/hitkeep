import { Service, afterNextRender, computed, signal } from '@angular/core';
import { palette, updatePrimaryPalette, updateSurfacePalette } from '@openng/optimus-ui-themes';
import { BUILT_IN_THEMES, DEFAULT_THEME_ID, type HitkeepTheme } from '@core/theme/theme.model';

const THEMES_STORAGE_KEY = 'hk_themes';
const ACTIVE_THEME_STORAGE_KEY = 'hk_active_theme';
const CUSTOM_STYLE_ELEMENT_ID = 'hk-theme-custom-css';
const FONT_FAMILY_PROPERTY = '--font-sans';

/**
 * Runtime theme manager. Built on top of the OptimusUI design-token API:
 * primary/surface palettes are regenerated from a seed color, the font is a
 * Tailwind CSS variable override, and any remaining styling lands in a
 * dedicated style element. The active theme persists in localStorage next to
 * the existing dark/light preference, so the mechanism works on share and
 * public report views as well.
 */
@Service()
export class ThemeManagerService {
    /**
     * Bumped on every apply. Chart options read this so echarts colors follow
     * the active theme even though the color source is a CSS variable.
     */
    readonly version = signal(0);

    private readonly customThemes = signal<HitkeepTheme[]>(this.loadCustomThemes());

    readonly themes = computed<HitkeepTheme[]>(() => [...BUILT_IN_THEMES, ...this.customThemes()]);

    readonly activeThemeId = signal<string>(this.loadActiveThemeId());

    readonly activeTheme = computed<HitkeepTheme>(() => {
        const id = this.activeThemeId();
        return this.themes().find((theme) => theme.id === id) ?? BUILT_IN_THEMES[0];
    });

    constructor() {
        if (typeof window === 'undefined') {
            return;
        }
        // Writing design tokens registers as a signal write, which Angular
        // rejects inside effect/computed contexts (NG0103) - including the
        // environment-initializer pass. Defer to the first render instead.
        afterNextRender(() => this.apply(this.activeTheme()));
    }

    setActiveTheme(id: string): void {
        if (!this.themes().some((theme) => theme.id === id)) {
            return;
        }
        this.activeThemeId.set(id);
        localStorage.setItem(ACTIVE_THEME_STORAGE_KEY, id);
        this.apply(this.activeTheme());
    }

    /** Persist a theme (built-in ids are stored as a custom copy under a new id). */
    saveTheme(theme: HitkeepTheme): void {
        const stored: HitkeepTheme = { ...theme, builtin: false };
        this.customThemes.update((themes) => {
            const index = themes.findIndex((entry) => entry.id === stored.id);
            if (index >= 0) {
                const next = [...themes];
                next[index] = stored;
                return next;
            }
            return [...themes, stored];
        });
        this.persistCustomThemes();
        this.setActiveTheme(stored.id);
    }

    deleteTheme(id: string): void {
        const theme = this.customThemes().find((entry) => entry.id === id);
        if (!theme) {
            return;
        }
        this.customThemes.update((themes) => themes.filter((entry) => entry.id !== id));
        this.persistCustomThemes();
        if (this.activeThemeId() === id) {
            this.setActiveTheme(DEFAULT_THEME_ID);
        } else {
            this.apply(this.activeTheme());
        }
    }

    /** Apply a theme without selecting or persisting it (live dialog preview). */
    previewTheme(theme: HitkeepTheme): void {
        this.apply(theme);
    }

    /** Re-apply the active theme, restoring state after an unsaved preview. */
    restoreActiveTheme(): void {
        this.apply(this.activeTheme());
    }

    private apply(theme: HitkeepTheme): void {
        // palette() is typed string | ColorScale but always returns the scale at runtime.
        updatePrimaryPalette(palette(theme.primary) as Parameters<typeof updatePrimaryPalette>[0]);
        if (theme.surface) {
            updateSurfacePalette(palette(theme.surface) as Parameters<typeof updateSurfacePalette>[0]);
        }
        this.applyFontFamily(theme.fontFamily);
        this.applyCustomCss(theme.customCss);
        this.version.update((version) => version + 1);
    }

    private applyFontFamily(fontFamily: string | undefined): void {
        const value = fontFamily?.trim();
        if (value) {
            document.documentElement.style.setProperty(FONT_FAMILY_PROPERTY, value);
        } else {
            document.documentElement.style.removeProperty(FONT_FAMILY_PROPERTY);
        }
    }

    private applyCustomCss(customCss: string | undefined): void {
        let element = document.getElementById(CUSTOM_STYLE_ELEMENT_ID);
        const css = customCss?.trim();
        if (!css) {
            element?.remove();
            return;
        }
        if (!element) {
            element = document.createElement('style');
            element.id = CUSTOM_STYLE_ELEMENT_ID;
            document.head.appendChild(element);
        }
        element.textContent = css;
    }

    private loadCustomThemes(): HitkeepTheme[] {
        if (typeof window === 'undefined') {
            return [];
        }
        try {
            const raw = localStorage.getItem(THEMES_STORAGE_KEY);
            if (!raw) {
                return [];
            }
            const parsed: unknown = JSON.parse(raw);
            if (!Array.isArray(parsed)) {
                return [];
            }
            return parsed.filter((entry): entry is HitkeepTheme => this.isValidTheme(entry));
        } catch {
            return [];
        }
    }

    private isValidTheme(entry: unknown): entry is HitkeepTheme {
        if (typeof entry !== 'object' || entry === null) {
            return false;
        }
        const theme = entry as Record<string, unknown>;
        return typeof theme['id'] === 'string' && typeof theme['name'] === 'string' && typeof theme['primary'] === 'string';
    }

    private persistCustomThemes(): void {
        localStorage.setItem(THEMES_STORAGE_KEY, JSON.stringify(this.customThemes()));
    }

    private loadActiveThemeId(): string {
        if (typeof window === 'undefined') {
            return DEFAULT_THEME_ID;
        }
        const saved = localStorage.getItem(ACTIVE_THEME_STORAGE_KEY);
        return saved && saved.length > 0 ? saved : DEFAULT_THEME_ID;
    }
}
