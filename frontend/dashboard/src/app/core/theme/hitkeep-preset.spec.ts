import { describe, expect, it } from 'vitest';

import { AUTH_CARD_DESIGN_TOKENS, AUTH_DIVIDER_DESIGN_TOKENS, AUTH_FIELDSET_DESIGN_TOKENS, AUTH_SELECT_BUTTON_DESIGN_TOKENS, hitKeepThemeOverrides } from './hitkeep-preset';

describe('HitKeep PrimeNG preset', () => {
    it('keeps shared component styling in PrimeNG design tokens', () => {
        expect(hitKeepThemeOverrides.semantic?.formField?.borderRadius).toBe('{border.radius.md}');
        expect(hitKeepThemeOverrides.components?.button?.root?.label?.fontWeight).toBe('600');
        expect(hitKeepThemeOverrides.components?.card?.root?.borderRadius).toBe('{border.radius.lg}');
        expect(hitKeepThemeOverrides.components?.message?.content?.padding).toBe('0.75rem 0.875rem');
        expect(hitKeepThemeOverrides.components?.dialog?.header?.padding).toBe('1.25rem 1.5rem 0.5rem');
    });

    it('keeps the auth card exception scoped and semantic', () => {
        expect(AUTH_CARD_DESIGN_TOKENS.root.background).toBe('{content.background}');
        expect(AUTH_CARD_DESIGN_TOKENS.root.borderRadius).toBe('{border.radius.xl}');
        expect(AUTH_CARD_DESIGN_TOKENS.body.padding).toContain('clamp(');
        expect(AUTH_DIVIDER_DESIGN_TOKENS.content.color).toBe('{text.muted.color}');
        expect(AUTH_FIELDSET_DESIGN_TOKENS.root.background).toBe('{content.hover.background}');
        expect(AUTH_SELECT_BUTTON_DESIGN_TOKENS.root.borderRadius).toBe('{border.radius.xl}');
    });
});
