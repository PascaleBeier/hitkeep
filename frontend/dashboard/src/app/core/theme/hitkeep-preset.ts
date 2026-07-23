import { definePreset } from '@openng/optimus-ui-themes';
import Aura from '@openng/optimus-ui-themes/aura';
import type { Preset } from '@openng/optimus-ui-themes/types';
import type { CardDesignTokens } from '@openng/optimus-ui-themes/types/card';
import type { DividerDesignTokens } from '@openng/optimus-ui-themes/types/divider';
import type { FieldsetDesignTokens } from '@openng/optimus-ui-themes/types/fieldset';
import type { SelectButtonDesignTokens } from '@openng/optimus-ui-themes/types/selectbutton';

const messageWithoutShadow = { shadow: 'none' } as const;

export const hitKeepThemeOverrides = {
    semantic: {
        transitionDuration: '0.2s',
        focusRing: {
            width: '2px',
            style: 'solid',
            color: '{primary.color}',
            offset: '2px',
            shadow: 'none'
        },
        formField: {
            borderRadius: '{border.radius.md}',
            paddingX: '0.75rem',
            paddingY: '0.5rem'
        }
    },
    components: {
        button: {
            root: {
                label: { fontWeight: '600' },
                transitionDuration: '{transition.duration}'
            }
        },
        card: {
            root: {
                borderRadius: '{border.radius.lg}',
                shadow: '0 1px 2px color-mix(in srgb, {text.color}, transparent 94%)'
            },
            body: { padding: '1.25rem' }
        },
        dialog: {
            header: {
                padding: '1.25rem 1.5rem 0.5rem'
            },
            content: {
                padding: '0 1.5rem 1.5rem'
            }
        },
        message: {
            root: {
                borderRadius: '{border.radius.lg}'
            },
            content: {
                padding: '0.75rem 0.875rem',
                gap: '0.625rem'
            },
            text: {
                fontSize: '0.875rem',
                fontWeight: '600'
            },
            colorScheme: {
                light: {
                    info: messageWithoutShadow,
                    success: messageWithoutShadow,
                    warn: messageWithoutShadow,
                    error: messageWithoutShadow,
                    secondary: messageWithoutShadow,
                    contrast: messageWithoutShadow
                },
                dark: {
                    info: messageWithoutShadow,
                    success: messageWithoutShadow,
                    warn: messageWithoutShadow,
                    error: messageWithoutShadow,
                    secondary: messageWithoutShadow,
                    contrast: messageWithoutShadow
                }
            }
        }
    }
} satisfies Preset;

export const HitKeepPreset = definePreset(Aura, hitKeepThemeOverrides);

export const AUTH_CARD_DESIGN_TOKENS = {
    root: {
        background: '{content.background}',
        borderRadius: '{border.radius.xl}',
        color: '{content.color}',
        shadow: '0 1px 2px color-mix(in srgb, {text.color}, transparent 94%)'
    },
    body: {
        padding: 'clamp(1.25rem, 3vw, 2rem)',
        gap: '0'
    }
} satisfies CardDesignTokens;

export const AUTH_DIVIDER_DESIGN_TOKENS = {
    root: {
        borderColor: '{content.border.color}'
    },
    content: {
        background: '{content.background}',
        color: '{text.muted.color}'
    },
    horizontal: {
        margin: '0.125rem 0',
        padding: '0',
        content: {
            padding: '0 0.75rem'
        }
    }
} satisfies DividerDesignTokens;

export const AUTH_SELECT_BUTTON_DESIGN_TOKENS = {
    root: {
        borderRadius: '{border.radius.xl}'
    }
} satisfies SelectButtonDesignTokens;

export const AUTH_FIELDSET_DESIGN_TOKENS = {
    root: {
        background: '{content.hover.background}',
        borderColor: '{content.border.color}',
        borderRadius: '{border.radius.xl}',
        color: '{content.color}',
        padding: '0.75rem'
    },
    legend: {
        background: 'transparent',
        borderWidth: '0',
        color: '{text.color}',
        fontWeight: '600',
        padding: '0 0.25rem'
    },
    content: {
        padding: '0.25rem 0 0'
    }
} satisfies FieldsetDesignTokens;
