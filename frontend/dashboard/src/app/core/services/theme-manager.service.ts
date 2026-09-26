import { Service, afterNextRender, computed, signal } from '@angular/core';
import { palette, updatePreset, updatePrimaryPalette, updateSurfacePalette } from '@openng/optimus-ui-themes';
import { BUILT_IN_THEMES, DEFAULT_THEME_ID, type HitkeepTheme } from '@core/theme/theme.model';

const THEMES_STORAGE_KEY = 'hk_themes';
const ACTIVE_THEME_STORAGE_KEY = 'hk_active_theme';
const CUSTOM_STYLE_ELEMENT_ID = 'hk-theme-custom-css';
const DENSITY_STYLE_ELEMENT_ID = 'hk-theme-density-styles';
const FONT_FAMILY_PROPERTY = '--font-sans';
const DEFAULT_ACCENT = '#14b8a6';

/**
 * Compact-mode overrides, injected once and toggled via classes on <html>.
 * Kept as plain CSS against stable OptimusUI class names so density works
 * without rebuilding the preset per theme switch.
 */
const DENSITY_CSS = `
html.hk-density-compact .p-card-body { padding: 0.875rem; }
html.hk-density-compact .p-datatable-tbody > tr > td { padding: 0.375rem 0.75rem; }
html.hk-density-compact .p-datatable-thead > tr > th { padding: 0.5rem 0.75rem; }
html.hk-density-compact .p-dialog-content { padding: 0 1.25rem 1rem; }
/*
 * Sidebar rules live in an emulated-view component stylesheet, where
 * ":host .class" compiles to (0,3,0) specificity. Doubling the terminal class
 * keeps these overrides winning without !important.
 */
html.hk-menu-compact app-layout-sidebar aside { padding: 0.75rem; gap: 0.75rem; }
html.hk-menu-compact app-layout-sidebar aside > div { gap: 0.5rem; }
html.hk-menu-compact app-layout-sidebar .layout-sidebar-menu__section.layout-sidebar-menu__section { margin-top: 0.55rem; }
html.hk-menu-compact app-layout-sidebar .layout-sidebar-menu__section-label.layout-sidebar-menu__section-label { margin-bottom: 0.2rem; font-size: 0.6875rem; }
html.hk-menu-compact app-layout-sidebar .layout-sidebar-menu__item-row.layout-sidebar-menu__item-row { min-height: 1.95rem; }
html.hk-menu-compact app-layout-sidebar .layout-sidebar-menu__item-main.layout-sidebar-menu__item-main { min-height: 1.95rem; padding: 0.3rem 0.65rem; gap: 0.5rem; }
html.hk-menu-compact app-layout-sidebar .layout-sidebar-menu__list--nested.layout-sidebar-menu__list--nested { margin-left: 1rem; padding-left: 0.5rem; }
html.hk-menu-compact app-layout-sidebar .layout-sidebar-menu__list--nested .layout-sidebar-menu__item-main.layout-sidebar-menu__item-main { padding: 0.2rem 0.5rem; min-height: 1.8rem; }
`;

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
        // Accent always resets (even without one) because updatePreset merges
        // into the current preset: omitting it would keep the previous theme's accent.
        updatePreset({
            semantic: {
                extend: {
                    accent: palette(theme.accent ?? DEFAULT_ACCENT) as Record<string, string>
                }
            }
        } as Parameters<typeof updatePreset>[0]);
        this.applyFontFamily(theme.fontFamily);
        this.applyFontSize(theme.fontSize);
        this.applyDensity(theme);
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

    private applyFontSize(fontSize: string | undefined): void {
        const value = fontSize?.trim();
        if (value) {
            document.documentElement.style.fontSize = value;
        } else {
            document.documentElement.style.removeProperty('font-size');
        }
    }

    private applyDensity(theme: HitkeepTheme): void {
        if (!document.getElementById(DENSITY_STYLE_ELEMENT_ID)) {
            const element = document.createElement('style');
            element.id = DENSITY_STYLE_ELEMENT_ID;
            element.textContent = DENSITY_CSS;
            document.head.appendChild(element);
        }
        document.documentElement.classList.toggle('hk-density-compact', theme.density === 'compact');
        document.documentElement.classList.toggle('hk-menu-compact', theme.menuSpacing === 'compact');
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
