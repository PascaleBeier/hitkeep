import { Service, computed, effect, signal } from '@angular/core';

export type HitkeepColorMode = 'light' | 'dark' | 'auto';

const COLOR_MODE_STORAGE_KEY = 'hk_theme_mode';
const LEGACY_THEME_STORAGE_KEY = 'hk_theme';

@Service()
export class PreferencesService {
    readonly colorMode = signal<HitkeepColorMode>(this.loadColorMode());
    private readonly systemPrefersDark = signal<boolean>(this.mediaQuery()?.matches ?? false);

    readonly isDarkMode = computed(() => {
        const mode = this.colorMode();
        return mode === 'dark' || (mode === 'auto' && this.systemPrefersDark());
    });

    constructor() {
        if (typeof window !== 'undefined') {
            this.mediaQuery()?.addEventListener('change', (event) => {
                this.systemPrefersDark.set(event.matches);
            });

            effect(() => {
                if (this.isDarkMode()) {
                    document.documentElement.classList.add('p-dark');
                } else {
                    document.documentElement.classList.remove('p-dark');
                }
                localStorage.setItem(COLOR_MODE_STORAGE_KEY, this.colorMode());
            });
        }
    }

    toggleTheme() {
        this.colorMode.set(this.isDarkMode() ? 'light' : 'dark');
    }

    setColorMode(mode: HitkeepColorMode) {
        this.colorMode.set(mode);
    }

    private loadColorMode(): HitkeepColorMode {
        if (typeof window === 'undefined') {
            return 'light';
        }
        const saved = localStorage.getItem(COLOR_MODE_STORAGE_KEY);
        if (saved === 'light' || saved === 'dark' || saved === 'auto') {
            return saved;
        }
        // Migrate the pre-mode storage value.
        return localStorage.getItem(LEGACY_THEME_STORAGE_KEY) === 'dark' ? 'dark' : 'light';
    }

    private mediaQuery(): MediaQueryList | null {
        if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
            return null;
        }
        return window.matchMedia('(prefers-color-scheme: dark)');
    }
}
