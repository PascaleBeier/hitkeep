export interface HitkeepTheme {
    id: string;
    name: string;
    builtin: boolean;
    /** Seed color for the primary palette (any CSS color). */
    primary: string;
    /** Optional seed color for the surface palette. */
    surface?: string;
    /** Optional seed color for the accent palette (charts, highlights). */
    accent?: string;
    /** Optional font family override applied to --font-sans. */
    fontFamily?: string;
    /** Optional base font size applied to the root element (e.g. '15px'). */
    fontSize?: string;
    /** Overall density of the dashboard. */
    density?: 'comfortable' | 'compact';
    /** Sidebar menu spacing. */
    menuSpacing?: 'default' | 'compact';
    /** Optional raw CSS appended to a dedicated theme style element. */
    customCss?: string;
}

export const DEFAULT_THEME_ID = 'hitkeep-default';

/**
 * Seeds for the pristine built-in theme. They match the stock OptimusUI Aura
 * ramp defaults (emerald primary) so the untouched theme is visually
 * identical to a dashboard without the theme manager.
 */
export const STOCK_PRIMARY = '#10b981';
export const STOCK_SURFACE = '#64748b';

export const BUILT_IN_THEMES: HitkeepTheme[] = [
    {
        id: DEFAULT_THEME_ID,
        name: 'HitKeep',
        builtin: true,
        primary: STOCK_PRIMARY
    },
    {
        id: 'hitkeep-teal',
        name: 'Teal',
        builtin: true,
        primary: '#0d9488'
    },
    {
        id: 'hitkeep-rose',
        name: 'Rose',
        builtin: true,
        primary: '#e11d48'
    },
    {
        id: 'hitkeep-amber',
        name: 'Amber',
        builtin: true,
        primary: '#d97706'
    },
    {
        id: 'hitkeep-blue',
        name: 'Blue',
        builtin: true,
        primary: '#2563eb'
    }
];

export function createThemeId(): string {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
        return `custom-${crypto.randomUUID()}`;
    }
    return `custom-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}
