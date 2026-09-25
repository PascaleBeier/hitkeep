import { toggleDimensionFilter } from '@core/analytics/filter-utils';

describe('dimension filters', () => {
    it('adds, replaces in place, and toggles off without mutating the input', () => {
        const original = [
            { type: 'country', value: 'DE' },
            { type: 'device', value: 'desktop' }
        ];
        const snapshot = structuredClone(original);
        expect(toggleDimensionFilter(original, 'path', '/')).toEqual([...original, { type: 'path', value: '/' }]);
        expect(toggleDimensionFilter(original, 'country', 'FR')).toEqual([{ type: 'country', value: 'FR' }, original[1]]);
        expect(toggleDimensionFilter(original, 'country', 'DE')).toEqual([original[1]]);
        expect(toggleDimensionFilter(original, 'country', '')).toBe(original);
        expect(original).toEqual(snapshot);
    });
});
