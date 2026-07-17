---
name: hitkeep-development
description: 'Implement and route HitKeep contributions through the repository-owned hk developer platform. Use for any HitKeep code, UI, API, database, analytics-feature, seed, screenshot, documentation, integration, or release-preparation change, as well as setup, local development, builds, smoke tests, and contributor workflow discovery.'
---

# HitKeep Development

Treat `AGENTS.md` as policy and `hk` as workflow truth. Never copy commands, build tags, environment defaults, variants, ports, tool versions, or QA gates into another instruction surface.

## Connect

Prefer the central local HitKeep developer MCP when configured. It is a root-routed stdio adapter over the same worktree-confined services as the CLI and is separate from the production analytics `/mcp` endpoint.

Register one long-lived clone's locally built `./hk` launcher in the host, then verify that the returned workspace ID matches the client's active root. Query `./hk mcp manifest --output json` for the live model-agnostic registration. If the client exposes multiple HitKeep roots, pass the intended root name, workspace ID, or path through the tools' optional `workspace` input. Never guess a workspace or silently edit client-owned global configuration.

When MCP is unavailable, discover the complete command and flag surface through `./hk catalog commands --output json`, then invoke the equivalent command with `--output json`. Consume `schema_version`, `status`, `workspace_id`, `data`, and `error`; do not parse human output. Stop and report an unknown schema version instead of guessing.

Use the typed MCP surface rather than translating an action into shell yourself:

- Diagnose with `hk_doctor`; start setup with `hk_setup_start` only when needed.
- Start, inspect, observe, or stop the workspace development session with `hk_dev_start`, `hk_dev_status`, `hk_dev_logs`, and `hk_dev_stop`.
- Start deterministic builds and image smokes with `hk_build_start` and `hk_smoke_start`.
- Observe or cancel finite returned runs with `hk_run_status`, `hk_logs_tail`, and `hk_run_cancel`.

Development start and stop stream progress and component logging until they reach a stable result. Development log following ends on client cancellation without stopping the session. Finite MCP actions return run handles immediately; equivalent CLI actions use `--detach --output json`.

## Route the Work

- Use `$hitkeep-workspace` for worktree isolation, ports, services, long-running operations, concurrent agents, logs, or handoff.
- Use `$hitkeep-qa` for gate selection, execution, investigation, and completion evidence.
- Use `$hitkeep-i18n` when user-visible dashboard language or localized formatting changes.
- Load only the relevant reference below for implementation guidance. Do not create or load area-specific HitKeep skills alongside this one.

## Implementation References

- Read [backend.md](references/backend.md) for Go runtime, API, auth, MCP, AI, or DuckDB changes.
- Read [frontend.md](references/frontend.md) for Angular, dashboard UX, seed data, browser verification, or screenshots.
- Read [product.md](references/product.md) for product shaping or an end-to-end analytics feature.
- Read [delivery.md](references/delivery.md) for docs, OpenAPI, integrations, release preparation, or infrastructure-adjacent work.

## Workflow

1. Read `AGENTS.md`; inspect workspace status, the development session, and active finite runs before doing setup or starting services.
2. Diagnose prerequisites. Development is container-only. Run setup only when diagnostics or missing container dependencies require it; warm worktrees should reuse images and caches.
3. Read the variant resource/catalog once per task before development, build, or smoke work. Refresh it after pulling or rebuilding `hk`.
4. Start development only when the task needs services. Use the catalog-provided variant and returned URLs; never infer tags, defaults, or ports.
5. Treat development as a workspace session with cursor-addressed events. Treat setup, QA, build, and smoke results as finite run handles. Preserve the workspace ID and the appropriate cursor or run ID, and read bounded logs only when progress or failure requires it.
6. Build and smoke through typed actions. Cloud images are local-only; never add or invoke publication behavior.
7. For Go or frontend source, discover formatter scopes and migration modes from the structured command catalog. Checks are non-mutating; rewriting is a deliberate confined CLI action and is not exposed through MCP. Review changed paths before continuing.
8. Finish through `$hitkeep-qa`. Report the profile, stable gate IDs, result, and concrete blockers without copying successful logs.

Never request arbitrary shell execution through MCP, mutate Git, delete worktrees, perform cleanup, publish artifacts, manage credentials, or invoke infrastructure deployment as part of this skill. Do not use the developer MCP for product analytics.
