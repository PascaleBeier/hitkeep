import { TestBed } from '@angular/core/testing';

import { ChartDesignPreferencesService } from '@services/chart-design-preferences.service';

const STORAGE_KEY = 'hitkeep.chartDesign';

describe('ChartDesignPreferencesService', () => {
    let initialPathname: string;

    beforeEach(() => {
        initialPathname = window.location.pathname;
        localStorage.clear();
        TestBed.configureTestingModule({});
    });

    afterEach(() => {
        window.history.replaceState({}, '', initialPathname);
        localStorage.clear();
    });

    it('has no remembered design by default', () => {
        const service = TestBed.inject(ChartDesignPreferencesService);

        expect(service.design()).toBeNull();
    });

    it('remembers and persists a selected design', () => {
        const service = TestBed.inject(ChartDesignPreferencesService);

        service.setDesign('bar');

        expect(service.design()).toBe('bar');
        expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '""')).toBe('bar');
    });

    it('restores a stored design', () => {
        localStorage.setItem(STORAGE_KEY, JSON.stringify('line'));

        const service = TestBed.inject(ChartDesignPreferencesService);

        expect(service.design()).toBe('line');
    });

    it('ignores invalid stored designs', () => {
        localStorage.setItem(STORAGE_KEY, JSON.stringify('pie'));

        const service = TestBed.inject(ChartDesignPreferencesService);

        expect(service.design()).toBeNull();
    });

    it('applies but does not persist designs on public share routes', () => {
        window.history.replaceState({}, '', '/share/token/dashboard');
        const service = TestBed.inject(ChartDesignPreferencesService);

        service.setDesign('bar');

        expect(service.design()).toBe('bar');
        expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
    });
});
