# HitKeep Agent Guide

This file is public guidance for AI-assisted contributions to HitKeep. It is written for external contributors, maintainers, and coding agents working from the open-source repository.

`CLAUDE.md` is a compatibility bridge for Claude Code. Keep `AGENTS.md` as the canonical public instruction file and avoid duplicating the full guide in multiple places.

## Start Here

- Treat the current repository as the source of truth. If an issue, prompt, or older document disagrees with the code, inspect the code first.
- Use `./hk` as the workflow source of truth, but consume that truth through callable central developer MCP tools and resources whenever they cover the task. Reserve direct CLI discovery for MCP bootstrap or repair, explicitly approved fallback, and workflows intentionally absent from MCP such as source rewrites; never copy commands, runtime configuration facts, build tags, cloud defaults, ports, tool versions, or QA gates into instructions.
- Treat developer MCP as available only when the relevant `hk_*` tools are exposed and a read-only call succeeds; a configured or enabled host entry is not proof. Register one long-lived clone's locally built `./hk` launcher once, query `./hk mcp manifest --output json` only for the live bootstrap contract, and verify the returned workspace ID against the client's active root. If tools are absent or fail, inspect host MCP health, compare the registration with the manifest, report whether registration, startup, root routing, or task reload is blocking use, and obtain explicit user approval before invoking an equivalent versioned `./hk --output json` action. When that fallback is approved, `./hk catalog configuration --output json` is the structured runtime configuration documentation contract. Existing tasks may require a host reload before newly registered tools appear; reserve human output for people.
- Inspect the current workspace before setup, services, builds, or QA. Reuse an active run instead of starting duplicate work.
- Development is one container-only session per workspace. It has status and event cursors, not a run ID. Setup, QA, builds, and smokes remain finite runs.
- Use the developer CLI's formatter and Go migration surfaces for deliberate source rewrites. QA and MCP checks must remain non-mutating.
- Keep changes small and tied to the user-visible behavior or maintenance task being requested.
- Do not include credentials, customer data, private deployment details, local machine paths, or screenshots that reveal private analytics.
- Preserve HitKeep's product shape: one deployable application, clear operator controls, and no unnecessary service dependencies.

## Product Invariants

- HitKeep runs as a single Go binary with an embedded Angular dashboard.
- Toolchain versions come from repository version files and are diagnosed by `hk`; do not restate them here.
- The dashboard uses Angular, OptimusUI, Tailwind CSS, Transloco, and Angular Signals.
- DuckDB is the storage engine. NSQ runs in process for queueing.
- Do not introduce required PostgreSQL, Redis, Kafka, ClickHouse, hosted analytics, or a separate queue/cache/database service.
- Tracking is cookieless by default and should collect only what is needed for analytics.
- Managed cloud and self-hosted deployments share the same foundation. Cloud-only behavior must be explicit and guarded.
- Operator simplicity matters. Prefer clear flags, environment variables, startup behavior, shutdown behavior, and errors over clever hidden coupling.

## Repository Map

- `cmd/`: application entry points and tools.
- `internal/config`: runtime configuration.
- `internal/server`: HTTP server, handlers, middleware, and API surfaces.
- `internal/database`: DuckDB stores, migrations, and tenant-aware queries.
- `internal/ingest`: ingest consumers.
- `internal/worker`: background workers.
- `internal/mcpserver`: optional read-only Model Context Protocol server.
- `internal/ai` and `internal/opportunities`: optional AI provider integration and validated opportunity generation.
- `internal/devtool`: developer-only application services used by `cmd/hk`; production builds must not depend on it.
- `frontend/dashboard`: Angular dashboard and tracker source.
- `frontend/dashboard/public/i18n`: dashboard translation JSON files.
- `frontend/dashboard/src/app/core/i18n`: dashboard locale helpers and OptimusUI locale synchronization.
- `skills`: public product analytics skills and transport-neutral Ask AI procedures.
- `.agents/skills`: canonical contributor skills for changing HitKeep through `hk`.
- `server.json`: MCP Registry metadata.
- `tests`: e2e fixtures, launchers, and audit scripts.

The rendered product documentation is public, but its source lives in the private `PascaleBeier/hitkeep-docs` repository by design. It is not part of HitKeep's public MIT-licensed source. Contributors without access should describe the documentation impact in their pull request or issue; maintainers apply the corresponding website update. When the private repository is checked out next to this repository, docs commands usually run from `../hitkeep-docs`.

## Contributor Workflow

1. Use `$hitkeep-development` to inspect prerequisites and route setup, development, variants, and builds.
2. Use `$hitkeep-workspace` before starting services, browser work, e2e, or concurrent QA. Trust returned workspace IDs, URLs, paths, development event cursors, and finite-operation run IDs.
3. Load the relevant backend, frontend, product, or delivery reference from `$hitkeep-development`; do not layer area-specific HitKeep skills over it.
4. Use `$hitkeep-qa` while iterating and before reporting completion. Preserve run IDs, inspect bounded failure logs, and report stable gate IDs rather than pasting successful logs.

Do not create or delete Git worktrees through `hk`. The developer MCP must not rewrite source. Do not run destructive cleanup, publish a cloud-enabled image, manage credentials, or invoke infrastructure deployment unless the user explicitly requests the separately authorized workflow.

## Database Rules

HitKeep runs on embedded DuckDB, which has no cascading deletes, no deferred constraints, and rewrites whole rows when an indexed column changes. The codebase encodes the safe patterns once; follow them instead of re-deriving workarounds.

- Give new site-scoped tables a `site_id` column and new team-scoped tables a `tenant_id` (or `team_id`) column. Deletion plans are derived from the live schema (`internal/database/fk_cleanup.go`), so correctly scoped tables are cleaned up, transferred, and purged automatically.
- Do not add static per-table delete or copy lists. If a table reaches its owner only through another table and the schema declares no foreign key for that hop, register the relationship in the relevant spec's `extraEdges` (see `siteDeleteSpec` and `tenantPurgeSpec`). Tables that must be nulled instead of deleted belong in `policyTables` with dedicated policy code.
- `Migrate` and `MigrateTenant` validate the cleanup plans after applying migrations. A table that references `sites` or `tenants` without a scope column fails startup with an explanatory error; fix the schema or register the exception rather than weakening the check.
- Updating a unique-indexed column on a foreign-key-referenced table (for example `sites.domain` or `users.email`) needs the shadow-row sequence; follow `Store.UpdateSiteDomain` or `Store.UpdateUserProfile`, including the separate transaction for the final shadow cleanup.
- Only base tables accept DML. Compatibility views (for example `team_audit_log`) appear in `information_schema.columns` but cannot be deleted from.
- Stats-reset and user-cleanup steps stay hand-written on purpose: they encode product policy (reset families, null-versus-delete choices), not foreign-key completeness.

Use focused database tests while iterating, then let `$hitkeep-qa` select the required completion gates from the live catalog.

## MCP Server Rules

HitKeep MCP is an optional, leader-only Streamable HTTP route for approved assistants and internal reporting tools. Keep this surface conservative.

- MCP tools must remain read-only and aggregate-only.
- Every MCP tool must set `ReadOnlyHint: true`.
- Analytics tools should use closed-world behavior. Only official docs lookup tools should declare open-world docs fetching.
- MCP must authenticate with API client bearer tokens. Do not accept dashboard cookies.
- Site analytics access must pass the same site-scoped permission checks as the REST and dashboard surfaces.
- Do not add MCP tools for write workflows, raw hit exports, token management, billing, site administration, goal mutation, exclusions, takeout, or dashboard session access.
- If a tool is added, renamed, removed, or changes behavior, update the MCP audit expectations, docs, public skills, and any registry metadata that changed.

Use the production MCP audit gates selected by `$hitkeep-qa`. Run a live endpoint audit only when the task explicitly requires it and a scoped test token is already available; never place that token in commands, logs, or reports.

## AI Output Rules

HitKeep's optional AI features must store safe, validated product data instead of raw model traffic.

- AI provider calls are optional and disabled unless configured.
- Do not persist raw prompts, raw provider responses, raw external error bodies, provider headers, provider credentials, or unrestricted tool-call payloads.
- Saved AI output must pass the relevant structured-output validation before storage.
- Opportunity recommendations should store localization keys, interpolation params, cited evidence IDs, detector metadata, status, and safe audit metadata.
- Cited evidence IDs in AI output must refer to evidence that was actually supplied to the run.
- GoAI-backed Opportunity proposal changes must keep the key/param contract deterministic. Add validator coverage before accepting new saved fields, message keys, interpolation params, action types, or evidence shapes.
- Keep deterministic analytics and permission checks outside the model. AI may enrich or explain cited evidence, but it should not bypass product validation.

Use focused package tests while iterating and `$hitkeep-qa` for the required completion profile.

## Frontend And i18n Rules

- Keep user-visible dashboard text in Transloco locale files, not hardcoded in Angular templates or component state.
- Discover supported dashboard languages from the locale directory and runtime configuration; do not maintain the list here.
- Locale files live under `frontend/dashboard/public/i18n/`. Add the same key path and value shape to every supported locale when adding UI copy.
- Preserve interpolation variable names and placeholder syntax across locales.
- Use `TranslocoPipe` in templates and `TranslocoService` for computed TypeScript labels.
- When labels depend on language changes, make the computation depend on the active language so it recomputes after a switch.
- For dates, numbers, percentages, and durations, use existing locale helpers, `@jsverse/transloco-locale`, or browser `Intl` APIs.
- OptimusUI locale text is synchronized through `PrimeLocaleSyncService`. Do not hardcode OptimusUI component labels unless there is no localizable surface.
- Keep translation IDs and formatting-locale mappings aligned with the runtime configuration.

Use `$hitkeep-i18n` for localization procedure and `$hitkeep-qa` for the current frontend gates.

## Agent Skills

HitKeep has two canonical, non-overlapping Agent Skill packs. `skills/` contains product analytics skills for end-user assistants; `.agents/skills/` contains contributor skills for changing this repository. Neither pack contains credentials or queries data by itself.

- Keep the product analytics identities `hitkeep-analytics`, `hitkeep-traffic-diagnosis`, `hitkeep-ai-visibility-analyst`, `hitkeep-ecommerce-analyst`, and `hitkeep-tracking-verifier` as direct children of `skills/`.
- Keep `hitkeep-development`, `hitkeep-workspace`, `hitkeep-qa`, and `hitkeep-i18n` canonical under `.agents/skills/`. Do not create generated proxies or duplicate these bodies under `skills/`.
- Do not embed tokens, customer data, private URLs, or private screenshots in skills.
- Keep mutable workflow facts in `hk`. Skills provide routing, judgment, safety rules, and interpretation—not copied command catalogs.
- Put area-specific implementation guidance in direct references under `hitkeep-development`, not in separately triggerable skills.
- Contributor skills must use callable developer MCP operations before equivalent CLI actions. They may support structured CLI fallback only after diagnosing and reporting MCP unavailability and receiving explicit user approval.
- Contributor skills and external MCP adapter text must never enter Ask AI. Ask AI embeds only the exact transport-neutral `references/procedure.md` files from the five product analytics skills.
- Keep each product skill's external MCP adapter and shared procedure aligned with the live analytics surface and privacy boundary.
- Use and update `hitkeep-i18n` when dashboard copy, locale files, language behavior, or localized formatting changes.
- Update `skills/README.md`, `CONTRIBUTING.md`, and public docs when either pack's shape or intended use changes.

## Docs And API References

- Public behavior changes should update public docs.
- Runtime API contract changes should update the runtime OpenAPI source and the docs OpenAPI file.
- MCP, Agent Skills, AI provider configuration, privacy, and export behavior should be documented in reader-facing language.
- Keep documentation factual. Avoid release promises, SEO filler, and claims that HitKeep cannot prove from product behavior.

Use the delivery reference under `$hitkeep-development` for adjacent documentation workflow and verification, then include its outcome in the final QA report.

## Testing Expectations

Use `$hitkeep-qa` to plan the smallest useful checks while iterating, the PR-parity profile before review, and the exhaustive profile for release risk, cloud behavior, or image behavior. Query live `hk` catalogs; do not maintain a second command matrix here.

Before opening a PR, report the QA profile, stable gate IDs, and final run status, plus anything that could not run and why. AI-assisted changes receive the same review standard as human-written changes.
