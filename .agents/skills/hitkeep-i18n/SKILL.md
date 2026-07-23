---
name: hitkeep-i18n
description: 'Maintain HitKeep dashboard localization. Use whenever a contributor adds or changes user-visible dashboard text, Transloco keys, locale JSON files, OptimusUI locale behavior, language switching, translated labels, localized layout, date/number/duration formatting, or supported dashboard languages.'
---

# HitKeep i18n

Treat `AGENTS.md` as policy, the locale files and runtime configuration as language truth, and `hk` as QA truth. Do not copy the supported-language list or validation commands into other instructions.

HitKeep uses Transloco JSON for UI copy, locale-aware formatting helpers and browser `Intl`, and a synchronization service for OptimusUI labels. Discover the current locale set from `frontend/dashboard/public/i18n` and confirm it against runtime configuration before editing.

## Workflow

1. Search for the existing feature key before creating a new key.
2. Put user-visible Angular template text behind `TranslocoPipe`.
3. Use `TranslocoService` for computed labels in TypeScript.
4. Add the same key path and value shape to every currently supported locale file.
5. Keep key names semantic and grouped by feature.
6. Preserve interpolation variables exactly. Do not translate variable names or change placeholder syntax.
7. For computed option labels, depend on the active language so labels recompute after language switches.
8. For dates, numbers, percentages, and durations, prefer the existing locale services or helpers instead of manual string formatting.
9. Let OptimusUI locale text flow through the existing sync service instead of hardcoding component labels.
10. Keep labels usable in buttons, tabs, chips, table columns, dialogs, and mobile layouts; visually inspect a long-string locale when layout risk exists.

Inspect only the surfaces relevant to the change:

- `frontend/dashboard/src/app/app.config.ts`
- `frontend/dashboard/src/app/transloco-loader.ts`
- `frontend/dashboard/src/app/core/i18n/`
- `frontend/dashboard/scripts/check-i18n-locales.mjs`
- `frontend/dashboard/scripts/sync-i18n-locales.mjs`

Do not infer a new supported language from an Angular locale helper alone. Change language support only when Transloco configuration, locale files, formatting mappings, tests, and product documentation agree.

## Translation Quality

Translate product UI, not individual English words.

- Write natural product language with correct punctuation, accents, and grammar.
- Reuse established feature terminology from nearby keys rather than translating English words in isolation.
- Preserve product names and technical identifiers intentionally; do not translate interpolation variables.
- Do not claim native-speaker review unless it happened.

Keep analytics terminology consistent across all languages. If the repo already uses a term for "site", "team", "event", "goal", "funnel", "share link", "API client", "QR campaign", or "Opportunity", reuse it.

## Validate

Add or update focused tests for language switching, computed labels, locale formatting, or OptimusUI synchronization when behavior changes. Use `$hitkeep-qa` to query and run the current gates matching locale, formatting, unit, and browser risk; do not reproduce their commands here. If a locale synchronization utility changes files, review every resulting change before keeping it.

## Output For Reviews

Report changed key paths, the locale files updated, focused behavior coverage, the `hk` QA profile and stable gate results, and any wording or layout caveats. Keep successful logs out of the report.
