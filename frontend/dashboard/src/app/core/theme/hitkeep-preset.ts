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
        },
        /**
         * A tile is a small label/value pane sitting inside a card. Aura has no
         * token for it, and the obvious hand-rolled surface — mixing
         * `{surface.0}` into the card — is wrong: the surface ramp is a fixed
         * light palette in both schemes, so it paints a near-white tile in dark
         * mode under white value text. These live here rather than in component
         * CSS so the scheme flip comes from the preset instead of a paired
         * `:host-context(.p-dark)` selector in every page that wants a tile.
         */
        extend: {
            tile: {
                borderColor: '{content.border.color}',
                // `{text.muted.color}` alone is only ~4.4:1 on a tinted tile,
                // under the 4.5:1 floor for a 12px label. Pull it toward the
                // body color; both schemes stay above 5:1.
                labelColor: 'color-mix(in srgb, {text.muted.color} 82%, {text.color})'
            },
            /**
             * Aura dropped PrimeNG v3's `surface-border` / `surface-card` /
             * `surface-ground` names, but the dashboard still references all
             * three in ~25 files. An undefined var is invalid at computed-value
             * time, so today those declarations silently do nothing: borders
             * render at zero width, backgrounds fall back to transparent. These
             * aliases give the existing call sites their intended value; each
             * resolves through a semantic token, so both schemes are covered.
             */
            surface: {
                border: '{content.border.color}',
                card: '{content.background}'
            },
            /**
             * Code surfaces (snippet panels, inline command chips). Like `tile`, the
             * scheme flip lives here rather than in a `:host-context(.p-dark)` block
             * per component. Light sinks the panel to a dark slab; dark *lifts* it
             * above the card, because sinking it further leaves near-black on
             * near-black with only the border separating the two.
             */
            code: {
                borderColor: '{content.border.color}',
                fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace'
            }
        },
        colorScheme: {
            // Light lifts the tile off the card, dark recesses it. Either way it
            // stays within ~1.05:1 of the card, so the theme's own text tokens
            // keep the contrast they were designed for.
            //
            // `surface.ground` is the page backdrop behind the cards. It has to
            // be declared per scheme rather than as a semantic alias because it
            // mirrors `index.html`'s `bg-surface-50 dark:bg-surface-950` on
            // `<body>` — which is what has been masking the dead token.
            light: {
                extend: {
                    tile: { background: 'color-mix(in srgb, {content.hover.background} 55%, {content.background})' },
                    surface: { ground: '{surface.50}' },
                    code: { background: '{surface.900}', color: '{surface.0}' }
                }
            },
            dark: {
                extend: {
                    tile: { background: 'color-mix(in srgb, {content.background} 72%, {surface.950})' },
                    surface: { ground: '{surface.950}' },
                    code: { background: '{surface.800}', color: '{surface.100}' }
                }
            }
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

/**
 * Disclosure fieldset: the legend button is the only chrome while collapsed, so an
 * always-on box does not sit empty under the label.
 */
export const DISCLOSURE_FIELDSET_DESIGN_TOKENS = {
    root: {
        background: 'transparent',
        borderColor: 'transparent',
        color: '{content.color}',
        padding: '0'
    },
    legend: {
        background: 'transparent',
        borderWidth: '0',
        color: '{text.color}',
        fontWeight: '600',
        padding: '0'
    },
    content: {
        padding: '0.75rem 0 0'
    }
} satisfies FieldsetDesignTokens;
