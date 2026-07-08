# HitKeep Agent Guide

This file is public guidance for AI-assisted contributions to HitKeep. It is written for external contributors, maintainers, and coding agents working from the open-source repository.

`CLAUDE.md` is a compatibility bridge for Claude Code. Keep `AGENTS.md` as the canonical public instruction file and avoid duplicating the full guide in multiple places.

## Start Here

- Treat the current repository as the source of truth. If an issue, prompt, or older document disagrees with the code, inspect the code first.
- Keep changes small and tied to the user-visible behavior or maintenance task being requested.
- Do not include credentials, customer data, private deployment details, local machine paths, or screenshots that reveal private analytics.
- Preserve HitKeep's product shape: one deployable application, clear operator controls, and no unnecessary service dependencies.

## Product Invariants

- HitKeep runs as a single Go binary with an embedded Angular dashboard.
- The backend targets Go 1.26.5.
- The dashboard uses Angular 22, PrimeNG 21, Tailwind CSS 4, Transloco, and Angular Signals.
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
- `frontend/dashboard`: Angular dashboard and tracker source.
- `frontend/dashboard/public/i18n`: dashboard translation JSON files.
- `frontend/dashboard/src/app/core/i18n`: dashboard locale helpers and PrimeNG locale synchronization.
- `skills`: public HitKeep Agent Skills.
- `server.json`: MCP Registry metadata.
- `tests`: e2e fixtures, launchers, and audit scripts.

Public documentation lives in the separate `PascaleBeier/hitkeep-docs` repository. When it is checked out next to this repository, docs commands usually run from `../hitkeep-docs`.

## MCP Server Rules

HitKeep MCP is an optional, leader-only Streamable HTTP route for approved assistants and internal reporting tools. Keep this surface conservative.

- MCP tools must remain read-only and aggregate-only.
- Every MCP tool must set `ReadOnlyHint: true`.
- Analytics tools should use closed-world behavior. Only official docs lookup tools should declare open-world docs fetching.
- MCP must authenticate with API client bearer tokens. Do not accept dashboard cookies.
- Site analytics access must pass the same site-scoped permission checks as the REST and dashboard surfaces.
- Do not add MCP tools for write workflows, raw hit exports, token management, billing, site administration, goal mutation, exclusions, takeout, or dashboard session access.
- If a tool is added, renamed, removed, or changes behavior, update the MCP audit expectations, docs, public skills, and any registry metadata that changed.

Useful checks:

```bash
GOFLAGS="$(./scripts/go-build-tags.sh goflags)" go test ./internal/mcpserver -run 'TestMCP.*Audit'
tests/scripts/mcp-audit.sh --schema-only
```

Use the live audit only when you have a running MCP endpoint and a scoped test token:

```bash
HITKEEP_MCP_URL=http://127.0.0.1:8080/mcp \
HITKEEP_MCP_TOKEN=<hitkeep-api-client-token> \
tests/scripts/mcp-audit.sh
```

## AI Output Rules

HitKeep's optional AI features must store safe, validated product data instead of raw model traffic.

- AI provider calls are optional and disabled unless configured.
- Do not persist raw prompts, raw provider responses, raw external error bodies, provider headers, provider credentials, or unrestricted tool-call payloads.
- Saved AI output must pass the relevant structured-output validation before storage.
- Opportunity recommendations should store localization keys, interpolation params, cited evidence IDs, detector metadata, status, and safe audit metadata.
- Cited evidence IDs in AI output must refer to evidence that was actually supplied to the run.
- GoAI-backed Opportunity proposal changes must keep the key/param contract deterministic. Add validator coverage before accepting new saved fields, message keys, interpolation params, action types, or evidence shapes.
- Keep deterministic analytics and permission checks outside the model. AI may enrich or explain cited evidence, but it should not bypass product validation.

Useful focused checks when AI behavior changes:

```bash
GOFLAGS="$(./scripts/go-build-tags.sh goflags)" go test ./internal/ai ./internal/opportunities ./internal/database ./internal/mcpserver
```

## Frontend And i18n Rules

- Keep user-visible dashboard text in Transloco locale files, not hardcoded in Angular templates or component state.
- Current dashboard languages are `en`, `de`, `es`, `fr`, `it`, `nl`, and `pt`.
- Locale files live under `frontend/dashboard/public/i18n/`.
- Add the same key path to all seven locale JSON files when adding UI copy.
- Preserve interpolation variable names and placeholder syntax across locales.
- Use `TranslocoPipe` in templates and `TranslocoService` for computed TypeScript labels.
- When labels depend on language changes, make the computation depend on the active language so it recomputes after a switch.
- For dates, numbers, percentages, and durations, use existing locale helpers, `@jsverse/transloco-locale`, or browser `Intl` APIs.
- PrimeNG locale text is synchronized through `PrimeLocaleSyncService`. Do not hardcode PrimeNG component labels unless there is no localizable surface.
- Portuguese dashboard copy uses the `pt` translation file and maps to `pt-BR` for formatting.

Useful checks:

```bash
cd frontend/dashboard && npm run i18n:check
cd frontend/dashboard && npm run fmt:check
```

## Agent Skills

The `skills/` directory contains public HitKeep Agent Skills. These are instructions for end-user assistants. They are not credentials and they do not query data by themselves.

- Keep skill directories as direct children of `skills/` so clients can discover them.
- Do not embed tokens, customer data, private URLs, or private screenshots in skills.
- Keep the parent `hitkeep-analytics` skill aligned with the current MCP tool list and privacy boundary.
- Update narrower skills when relevant MCP tools, filters, metrics, or caveats change.
- Use and update `hitkeep-i18n` when dashboard copy, locale files, language behavior, or localized formatting changes.
- Update `skills/README.md` when adding, removing, renaming, or changing the intended use of a skill.

## Docs And API References

- Public behavior changes should update public docs.
- Runtime API contract changes should update the runtime OpenAPI source and the docs OpenAPI file.
- MCP, Agent Skills, AI provider configuration, privacy, and export behavior should be documented in reader-facing language.
- Keep documentation factual. Avoid release promises, SEO filler, and claims that HitKeep cannot prove from product behavior.

Docs verification when the docs repository is available:

```bash
cd ../hitkeep-docs && npm run build
```

## Testing Expectations

Run the smallest useful checks while iterating, then broaden based on risk.

Backend:

```bash
GOFLAGS="$(./scripts/go-build-tags.sh goflags)" go test ./...
GOFLAGS="$(./scripts/go-build-tags.sh goflags)" go test -race ./...
```

Frontend:

```bash
cd frontend/dashboard && npm run fmt && npm run fmt:check
cd frontend/dashboard && npm run lint
cd frontend/dashboard && npm run test:ci
cd frontend/dashboard && npm run i18n:check
```

Docs:

```bash
cd ../hitkeep-docs && npm run build
```

Before opening a PR, report what you ran and what you could not run. AI-assisted changes receive the same review standard as human-written changes.
