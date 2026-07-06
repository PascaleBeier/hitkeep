import { TestBed } from '@angular/core/testing';

import { DEFAULT_RANGE_OPTIONS, RangeOption } from '@components/range-toolbar/range-toolbar';
import { ReportRangePreferencesService } from '@services/report-range-preferences.service';

const STORAGE_KEY = 'hitkeep.reportRange';

describe('ReportRangePreferencesService', () => {
    let service: ReportRangePreferencesService;
    let initialPathname: string;

    beforeEach(() => {
        initialPathname = window.location.pathname;
        localStorage.clear();
        TestBed.configureTestingModule({});
        service = TestBed.inject(ReportRangePreferencesService);
    });

    afterEach(() => {
        window.history.replaceState({}, '', initialPathname);
        localStorage.clear();
    });

    it('falls back to the supplied default when no range is stored', () => {
        const fallback = DEFAULT_RANGE_OPTIONS.find((option) => option.value === '30d') as RangeOption;

        expect(service.initialSelection(DEFAULT_RANGE_OPTIONS, fallback)).toEqual({ range: fallback, customRangeDates: null });
    });

    it('restores a stored preset range', () => {
        localStorage.setItem(STORAGE_KEY, JSON.stringify({ value: '7d' }));

        expect(service.initialSelection().range.value).toBe('7d');
    });

    it('restores a stored custom range', () => {
        localStorage.setItem(
            STORAGE_KEY,
            JSON.stringify({
                value: 'custom',
                customRange: ['2026-07-01T00:00:00.000Z', '2026-07-02T00:00:00.000Z']
            })
        );

        const selection = service.initialSelection();

        expect(selection.range.value).toBe('custom');
        expect(selection.customRangeDates?.map((date) => date.toISOString())).toEqual(['2026-07-01T00:00:00.000Z', '2026-07-02T00:00:00.000Z']);
    });

    it('ignores invalid stored ranges', () => {
        localStorage.setItem(STORAGE_KEY, JSON.stringify({ value: 'custom', customRange: ['2026-07-02T00:00:00.000Z', '2026-07-01T00:00:00.000Z'] }));

        expect(service.initialSelection().range.value).toBe('today');
        expect(service.initialSelection().customRangeDates).toBeNull();
    });

    it('persists a selected preset range', () => {
        const range = DEFAULT_RANGE_OPTIONS.find((option) => option.value === '14d') as RangeOption;

        expect(service.saveSelection({ value: range })).toBeNull();
        expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toEqual({ value: '14d' });
    });

    it('persists a selected custom range', () => {
        const range = DEFAULT_RANGE_OPTIONS.find((option) => option.value === 'custom') as RangeOption;
        const customRange = [new Date('2026-07-01T00:00:00.000Z'), new Date('2026-07-02T00:00:00.000Z')];

        expect(service.saveSelection({ value: range, customRange })?.map((date) => date.toISOString())).toEqual(['2026-07-01T00:00:00.000Z', '2026-07-02T00:00:00.000Z']);
        expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toEqual({
            value: 'custom',
            customRange: ['2026-07-01T00:00:00.000Z', '2026-07-02T00:00:00.000Z']
        });
    });

    it('does not persist report ranges on public share routes', () => {
        window.history.replaceState({}, '', '/share/token/dashboard');
        const range = DEFAULT_RANGE_OPTIONS.find((option) => option.value === '7d') as RangeOption;

        service.saveSelection({ value: range });

        expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
        expect(service.initialSelection().range.value).toBe('today');
    });
});
