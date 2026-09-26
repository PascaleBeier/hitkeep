import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { PreferencesService } from '@services/preferences.service';

describe('PreferencesService', () => {
    let service: PreferencesService;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [PreferencesService]
        });
        service = TestBed.inject(PreferencesService);

        // Mock LocalStorage
        const store: Record<string, string> = {};
        vi.spyOn(localStorage, 'getItem').mockImplementation((key: string) => store[key] || null);
        vi.spyOn(localStorage, 'setItem').mockImplementation((key: string, value: string) => {
            store[key] = value;
        });
    });

    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('should be created', () => {
        expect(service).toBeTruthy();
    });

    it('should toggle theme signal', () => {
        const initial = service.isDarkMode();
        service.toggleTheme();
        expect(service.isDarkMode()).not.toBe(initial);
    });

    it('should follow the system preference in auto mode', () => {
        vi.spyOn(window, 'matchMedia').mockReturnValue({
            matches: true,
            addEventListener: vi.fn()
        } as unknown as MediaQueryList);
        localStorage.setItem('hk_theme_mode', 'auto');

        TestBed.resetTestingModule();
        TestBed.configureTestingModule({
            providers: [PreferencesService]
        });
        const autoService = TestBed.inject(PreferencesService);

        expect(autoService.colorMode()).toBe('auto');
        expect(autoService.isDarkMode()).toBe(true);

        autoService.setColorMode('light');
        expect(autoService.isDarkMode()).toBe(false);
    });

    it('should expose three color modes', () => {
        service.setColorMode('auto');
        expect(service.colorMode()).toBe('auto');
        service.setColorMode('dark');
        expect(service.isDarkMode()).toBe(true);
        service.setColorMode('light');
        expect(service.isDarkMode()).toBe(false);
    });
});
