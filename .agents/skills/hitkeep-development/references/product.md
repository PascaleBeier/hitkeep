# Product and analytics features

Read this reference when shaping a product change or implementing an analytics capability across storage, API, dashboard, seed data, and documentation.

## Shape the change

Define before broad implementation:

1. The user problem and target user.
2. The primary workflow and decision the feature supports.
3. The minimum data required and its privacy impact.
4. Self-hosted and managed-cloud behavior.
5. Backend, API, dashboard, seed, docs, and migration impact.
6. Observable acceptance criteria and explicit non-goals.

For customer-facing behavior or customer-visible data, decide whether users would expect it through production MCP, takeout/open exports, and Playwright e2e. Turn applicable decisions into normal acceptance criteria; record a reason when a surface is intentionally excluded.

## Implement an analytics feature

1. Start from the user question, not the chart type.
2. Prefer existing query and presentation models before changing schema.
3. Update store/query behavior, permissioned handlers, public types/OpenAPI, dashboard services, and UI as one thin end-to-end slice.
4. Reuse established KPI, series, metric-list, range, table, and filter primitives.
5. Provide loading, empty, error, and populated states; make filters reversible and comparison math unambiguous.
6. Add realistic seed data so the isolated seeded workflow demonstrates the feature.
7. Update public docs and screenshots when the visible capability changes.

Keep scope honest. Preserve the single-binary and privacy model, avoid vanity metrics, and prefer the smallest version that helps a user make a decision.

Use `$hitkeep-workspace` for the seeded/browser workflow and `$hitkeep-qa` for cross-stack validation.
