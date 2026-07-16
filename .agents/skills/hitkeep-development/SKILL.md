---
name: hitkeep-development
description: 'Implement and route HitKeep contributions through the repository-owned hk developer platform. Use for any HitKeep code, UI, API, database, analytics-feature, seed, screenshot, documentation, integration, or release-preparation change, as well as setup, local development, builds, smoke tests, and contributor workflow discovery.'
---

# HitKeep Development

Treat `AGENTS.md` as policy and `hk` as workflow truth. Never copy commands, build tags, environment defaults, variants, ports, tool versions, or QA gates into another instruction surface.

## Connect

Prefer the central local HitKeep developer MCP when configured. It is a root-routed stdio adapter over the same worktree-confined services as the CLI and is separate from the production analytics `/mcp` endpoint.

Register one long-lived clone's locally built `./hk` launcher in the host, then verify that the returned workspace ID matches the client's active root. Query `./hk mcp manifest --output json` for the live model-agnostic registration. If the client exposes multiple HitKeep roots, pass the intended root name, workspace ID, or path through the tools' optional `workspace` input. Never guess a workspace or silently edit client-owned global configuration.

When MCP is unavailable, use the equivalent command discovered through `./hk help` with `--output json`. Consume `schema_version`, `status`, `workspace_id`, `data`, and `error`; do not parse human output. Stop and report an unknown schema version instead of guessing.

Use the typed MCP surface rather than translating an action into shell yourself:

- Diagnose with `hk_doctor`; start setup with `hk_setup_start` only when needed.
- Start or stop development with `hk_dev_start` and `hk_dev_stop`.
- Start deterministic builds and image smokes with `hk_build_start` and `hk_smoke_start`.
- Observe or cancel returned runs with `hk_run_status`, `hk_logs_tail`, and `hk_run_cancel`.

MCP actions return immediately. CLI actions should use `--detach --output json` when an agent needs the same asynchronous behavior.

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

1. Read `AGENTS.md`; inspect workspace status and active runs before doing setup or starting services.
2. Diagnose prerequisites. Run setup only when diagnostics or missing dependencies require it; warm worktrees should reuse shared snapshots and caches.
3. Read the variant resource/catalog once per task before development, build, or smoke work. Refresh it after pulling or rebuilding `hk`.
4. Start only the runtime needed for the task. Use the catalog-provided variant and returned URLs; never infer tags, defaults, or ports.
5. Treat every action result as a run handle. Preserve its workspace ID and run ID, poll status, and read bounded logs only when progress or failure requires it.
6. Build and smoke through typed actions. Cloud images are local-only; never add or invoke publication behavior.
7. For Go source, discover the formatter and migration modes from CLI help. Checks are non-mutating; rewriting is a deliberate confined CLI action and is not exposed through MCP. Review changed paths before continuing.
8. Finish through `$hitkeep-qa`. Report the profile, stable gate IDs, result, and concrete blockers without copying successful logs.

Never request arbitrary shell execution through MCP, mutate Git, delete worktrees, perform cleanup, publish artifacts, manage credentials, or invoke infrastructure deployment as part of this skill. Do not use the developer MCP for product analytics.
