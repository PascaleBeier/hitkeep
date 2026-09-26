import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { isStockAppearance, ThemeManagerService } from '@services/theme-manager.service';
import { BUILT_IN_THEMES, DEFAULT_THEME_ID, STOCK_PRIMARY, type HitkeepTheme } from '@core/theme/theme.model';

const updatePrimaryPalette = vi.fn();
const updateSurfacePalette = vi.fn();
const updatePreset = vi.fn();
const palette = vi.fn((color: string) => ({ 500: color }));

vi.mock('@openng/optimus-ui-themes', () => ({
    updatePrimaryPalette: (...args: unknown[]) => updatePrimaryPalette(...args),
    updateSurfacePalette: (...args: unknown[]) => updateSurfacePalette(...args),
    updatePreset: (...args: unknown[]) => updatePreset(...args),
    palette: (color: string) => palette(color)
}));

describe('ThemeManagerService', () => {
    let service: ThemeManagerService;
    let store: Record<string, string>;

    const customTheme: HitkeepTheme = {
        id: 'custom-1',
        name: 'Mine',
        builtin: false,
        primary: '#e11d48',
        fontFamily: 'Inter, sans-serif',
        customCss: '.hk-custom { color: red; }'
    };

    beforeEach(() => {
        vi.clearAllMocks();
        store = {};
        vi.spyOn(localStorage, 'getItem').mockImplementation((key: string) => store[key] || null);
        vi.spyOn(localStorage, 'setItem').mockImplementation((key: string, value: string) => {
            store[key] = value;
        });
        vi.spyOn(localStorage, 'removeItem').mockImplementation((key: string) => {
            delete store[key];
        });

        TestBed.configureTestingModule({
            providers: [ThemeManagerService]
        });
        service = TestBed.inject(ThemeManagerService);
    });

    afterEach(() => {
        vi.restoreAllMocks();
        document.getElementById('hk-theme-custom-css')?.remove();
        document.documentElement.style.removeProperty('--font-sans');
    });

    it('defaults to the built-in HitKeep theme', () => {
        expect(service.activeThemeId()).toBe(DEFAULT_THEME_ID);
        expect(service.activeTheme().builtin).toBe(true);
    });

    it('saves a custom theme, persists it and selects it', () => {
        service.saveTheme(customTheme);

        expect(service.activeThemeId()).toBe('custom-1');
        expect(store['hk_active_theme']).toBe('custom-1');
        expect(store['hk_themes']).toContain('custom-1');

        const stored = JSON.parse(store['hk_themes']) as HitkeepTheme[];
        expect(stored).toHaveLength(1);
        expect(stored[0].name).toBe('Mine');
    });

    it('restores custom themes and the active id from storage', () => {
        store['hk_themes'] = JSON.stringify([customTheme]);
        store['hk_active_theme'] = 'custom-1';

        TestBed.resetTestingModule();
        TestBed.configureTestingModule({
            providers: [ThemeManagerService]
        });
        const restored = TestBed.inject(ThemeManagerService);

        expect(restored.themes().some((theme) => theme.id === 'custom-1')).toBe(true);
        expect(restored.activeThemeId()).toBe('custom-1');
        expect(updateSurfacePalette).not.toHaveBeenCalled();
    });

    it('applies font family and custom CSS of the active theme', () => {
        service.saveTheme(customTheme);

        expect(document.documentElement.style.getPropertyValue('--font-sans')).toBe('Inter, sans-serif');
        expect(document.getElementById('hk-theme-custom-css')?.textContent).toBe('.hk-custom { color: red; }');
    });

    it('deletes a custom theme and falls back to the default', () => {
        service.saveTheme(customTheme);
        service.deleteTheme('custom-1');

        expect(service.activeThemeId()).toBe(DEFAULT_THEME_ID);
        expect(service.themes().some((theme) => theme.id === 'custom-1')).toBe(false);
    });

    it('ignores unknown theme ids when selecting', () => {
        service.setActiveTheme('does-not-exist');
        expect(service.activeThemeId()).toBe(DEFAULT_THEME_ID);
    });

    it('ignores corrupted stored themes', () => {
        store['hk_themes'] = 'not json at all';

        TestBed.resetTestingModule();
        TestBed.configureTestingModule({
            providers: [ThemeManagerService]
        });
        const restored = TestBed.inject(ThemeManagerService);

        expect(restored.themes()).toEqual(BUILT_IN_THEMES);
    });

    it('bumps the version on every apply so charts re-resolve colors', () => {
        const before = service.version();
        service.previewTheme({ id: 'x', name: 'X', builtin: false, primary: '#0d9488' });
        expect(service.version()).toBeGreaterThan(before);
    });

    it('leaves the preset untouched while the pristine default theme is active', () => {
        service.restoreActiveTheme();

        expect(service.activeThemeId()).toBe(DEFAULT_THEME_ID);
        expect(updatePrimaryPalette).not.toHaveBeenCalled();
        expect(updateSurfacePalette).not.toHaveBeenCalled();
    });
});

describe('isStockAppearance', () => {
    const stockTheme: HitkeepTheme = { ...BUILT_IN_THEMES[0] };

    it('matches the pristine built-in theme', () => {
        expect(isStockAppearance(stockTheme)).toBe(true);
    });

    it('matches a saved default that only renamed the theme', () => {
        expect(isStockAppearance({ ...stockTheme, name: 'My brand', builtin: false })).toBe(true);
    });

    it('rejects a changed primary seed', () => {
        expect(isStockAppearance({ ...stockTheme, primary: '#e11d48' })).toBe(false);
    });

    it('rejects any additional customization', () => {
        expect(isStockAppearance({ ...stockTheme, surface: '#64748b' })).toBe(false);
        expect(isStockAppearance({ ...stockTheme, accent: '#14b8a6' })).toBe(false);
        expect(isStockAppearance({ ...stockTheme, fontFamily: 'Inter, sans-serif' })).toBe(false);
        expect(isStockAppearance({ ...stockTheme, fontSize: '15px' })).toBe(false);
        expect(isStockAppearance({ ...stockTheme, density: 'compact' })).toBe(false);
        expect(isStockAppearance({ ...stockTheme, menuSpacing: 'compact' })).toBe(false);
        expect(isStockAppearance({ ...stockTheme, customCss: '.a { color: red; }' })).toBe(false);
    });

    it('rejects other theme ids even with stock values', () => {
        expect(isStockAppearance({ ...stockTheme, id: 'custom-1' })).toBe(false);
    });

    it('accepts uppercase and padded stock hex', () => {
        expect(isStockAppearance({ ...stockTheme, primary: ` ${STOCK_PRIMARY.toUpperCase()} ` })).toBe(true);
    });
});
