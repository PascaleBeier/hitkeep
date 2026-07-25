import { calcDelta, safeRate } from '@core/analytics/delta-utils';

describe('delta utils', () => {
    it('guards divide-by-zero in rates', () => {
        expect(safeRate(2, 0)).toBe(0);
        expect(safeRate(2, 5)).toBe(40);
    });

    it('returns a percentage share for non-zero denominators', () => {
        expect(safeRate(0, 5)).toBe(0);
        expect(safeRate(5, 5)).toBe(100);
    });

    it('returns null delta when there is no previous baseline', () => {
        expect(calcDelta(12, 0)).toBeNull();
        expect(calcDelta(15, 10)).toBe(50);
    });

    it('returns a negative delta when the current period declined', () => {
        expect(calcDelta(5, 10)).toBe(-50);
        expect(calcDelta(10, 10)).toBe(0);
    });
});
