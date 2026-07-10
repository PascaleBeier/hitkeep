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
});
