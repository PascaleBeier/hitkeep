import { describe, it, expect, afterEach } from 'vitest';
import { HITKEEP_CHART_PALETTE, resolveChartColor } from '@core/charts/hitkeep-chart-options';

describe('resolveChartColor', () => {
    afterEach(() => {
        document.documentElement.style.removeProperty('--p-primary-500');
    });

    it('returns the original color when no token mapping exists', () => {
        expect(resolveChartColor('#123456')).toBe('#123456');
    });

    it('resolves mapped colors from the design token when set', () => {
        document.documentElement.style.setProperty('--p-primary-500', '#ff0000');

        expect(resolveChartColor('#6366f1')).toBe('#ff0000');
    });

    it('falls back to the original hex when the token is unset', () => {
        // Other specs may have applied the full preset, which defines this
        // token - force it empty so the fallback path is exercised.
        document.documentElement.style.setProperty('--p-primary-500', '');

        expect(resolveChartColor('#6366F1')).toBe('#6366F1');
    });
});

describe('HITKEEP_CHART_PALETTE', () => {
    afterEach(() => {
        document.documentElement.style.removeProperty('--p-primary-500');
    });

    it('resolves through the token map', () => {
        document.documentElement.style.setProperty('--p-primary-500', '#ff0000');
        expect(HITKEEP_CHART_PALETTE.primary).toBe('#ff0000');
    });

    it('falls back to the default palette when tokens are unset', () => {
        expect(HITKEEP_CHART_PALETTE.primary).toBe('#6366f1');
        expect(HITKEEP_CHART_PALETTE.secondary).toBe('#14b8a6');
    });
});
