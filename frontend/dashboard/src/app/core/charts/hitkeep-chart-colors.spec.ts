import { describe, it, expect, vi, afterEach } from 'vitest';
import { HITKEEP_CHART_PALETTE, resolveChartColor } from '@core/charts/hitkeep-chart-options';

/**
 * Mocks every design-token lookup on the root element as unset. The full
 * preset is applied by other specs in the same browser page, which defines
 * all chart token colors.
 */
function mockTokensUnset(): void {
    const style = getComputedStyle(document.documentElement);
    vi.spyOn(style, 'getPropertyValue').mockReturnValue('');
}

afterEach(() => {
    vi.restoreAllMocks();
    document.documentElement.style.removeProperty('--p-primary-500');
});

describe('resolveChartColor', () => {
    it('returns the original color when no token mapping exists', () => {
        expect(resolveChartColor('#123456')).toBe('#123456');
    });

    it('resolves mapped colors from the design token when set', () => {
        document.documentElement.style.setProperty('--p-primary-500', '#ff0000');

        expect(resolveChartColor('#6366f1')).toBe('#ff0000');
    });

    it('falls back to the original hex when the token is unset', () => {
        mockTokensUnset();

        expect(resolveChartColor('#6366F1')).toBe('#6366F1');
    });
});

describe('HITKEEP_CHART_PALETTE', () => {
    it('resolves through the token map', () => {
        document.documentElement.style.setProperty('--p-primary-500', '#ff0000');
        expect(HITKEEP_CHART_PALETTE.primary).toBe('#ff0000');
    });

    it('falls back to the default palette when tokens are unset', () => {
        mockTokensUnset();

        expect(HITKEEP_CHART_PALETTE.primary).toBe('#6366f1');
        expect(HITKEEP_CHART_PALETTE.secondary).toBe('#14b8a6');
        expect(HITKEEP_CHART_PALETTE.warning).toBe('#a16207');
    });
});
