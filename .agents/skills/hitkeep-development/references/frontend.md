# Frontend and visual implementation

Read this reference only when the change touches the Angular dashboard, tracker-adjacent UI, seed data, browser flows, or screenshots. Use `$hitkeep-i18n` for language procedure.

## Navigate

- Inspect application code under `frontend/dashboard/src/app`, global styling and theme setup, existing shared UI primitives, nearby specs, and e2e flows before editing.
- Use the framework versions and component interfaces present in the repository; do not copy version facts into instructions.
- Obtain services, URLs, ports, and seeded identities from `$hitkeep-workspace` and live `hk` output.

## Implement dashboard behavior

1. Follow existing signal, dependency-injection, change-detection, component-file, and native control-flow patterns.
2. Prefer shared page frames, headers, range controls, state views, tables, dialogs, drawers, metric groups, and services before creating a new primitive.
3. Use PrimeNG for standard controls and the existing PrimeNG/Tailwind token system for surfaces, text, borders, severity, overlays, inputs, radii, and shadows.
4. Keep authorization visible and exact: route, navigation, tab, and action gating must match backend enforcement.
5. Give loading, empty, error, filtered, permission-denied, success, and destructive states behavior appropriate to the workflow.
6. Give every asynchronous or mutating action local pending, success, and failure feedback.
7. Keep dark mode, keyboard behavior, touch targets, scrolling, text expansion, and stable chart/table dimensions first-class.
8. Add focused specs near the changed component, route, or service.

## Visual quality

- Aim for calm, dense operator software, not a marketing surface.
- Reject nested decorative cards, random gradients, raw one-off colors, fake data, vague action labels, and custom controls that duplicate PrimeNG.
- Compare admin/settings work with a mature existing page for frame density, spacing, actions, and feedback.
- Inspect desktop, mobile, light/dark, and a long-string locale when the change creates those risks.

## Seed and screenshot work

- Reuse an active isolated seeded run when possible. Keep seed output deterministic, realistic, and fast enough for development and e2e.
- Exercise meaningful dates, sources, devices, events, permissions, and product-specific data instead of filler rows.
- Capture only states that document real behavior. Check clipping, overlaps, blank charts, loading-only captures, and stale UI.
- Keep screenshot filenames stable and update README/docs references only when the image adds user value.

Run focused non-watch tests and browser checks while iterating, then delegate current formatting, unit, localization, and e2e gates to `$hitkeep-qa`.
