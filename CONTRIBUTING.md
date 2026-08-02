# Contributing to HitKeep

Thank you for improving HitKeep. The repository-owned `./hk` developer CLI is the canonical workflow surface for people, automation, and coding agents. The normal human path is deliberately small; isolated workspace and run details remain available for concurrent worktrees and structured tooling.

Repository policy and product invariants live in [`AGENTS.md`](./AGENTS.md). Read it before making a change.

## Start in One Command

```bash
git clone https://github.com/pascalebeier/hitkeep.git
cd hitkeep
./hk setup
```

The checked-in `./hk` file is an executable POSIX launcher, not a compiled artifact. It builds the current worktree's CLI and central MCP broker locally, caches the native executable by source content, and selects the host's macOS or Linux and AMD64 or ARM64 target. WSL2 follows the Linux path; WSL1 is not supported. Docker performs the first bootstrap when the exact Go version from `go.mod` is not already present on the host; a host Go installation is only an optional fast path.

Developer CLI binaries are neither committed nor published as HitKeep release assets. Git keeps the launcher and implementation in lockstep with every branch, while GitHub Releases remain limited to deployable HitKeep product artifacts. `setup` pulls and builds the development containers for the selected worktree, verifies and installs the repository-pinned Go and Node.js archives from their official SHA-256 metadata, then installs the pinned npm release through that managed Node.js toolchain. Docker Compose is the only application runtime; host Go, Node.js, and npm installations are never required for setup, QA, builds, source tooling, or the developer MCP.

Check prerequisites and local development status at any time:

```bash
./hk doctor
./hk workspace status
```

Use `./hk help` and subcommand help for the current command reference. Use `./hk catalog --output json` when a tool or agent needs the live variant and QA catalogs, and `./hk catalog commands --output json` for the complete command and flag inventory.

## Develop

Start the development session:

```bash
./hk dev --seed
```

`./hk dev` starts the workspace's Compose stack in the foreground, prints component-labelled colored output, and owns the complete lifecycle. Pressing `Ctrl+C` stops the backend, frontend, Mailpit, log follower, and orphaned Compose services. Running it again while the session is active reports its state and URLs and exits; it never attaches implicitly or starts a duplicate.

The everyday lifecycle is:

```bash
./hk dev
./hk dev logs
./hk dev restart
./hk dev stop
./hk dev reset --seed
```

`restart` preserves this worktree's local data. `reset` stops development, deletes only this worktree's local HitKeep data, and restarts. Use reset when a genuinely fresh seeded database is required; ordinary `--seed` updates the existing isolated demo data.

Explicitly create a background session when foreground ownership is not appropriate:

```bash
./hk dev --detach
```

`--detach` waits until every service is ready before returning. `./hk dev logs` follows the current session for people; pressing `Ctrl+C` detaches only that viewer. `./hk dev stop` requests cooperative shutdown and waits for completion.

Choose the cloud variant only for local managed-cloud parity work:

```bash
./hk dev --variant cloud
```

Automation and coding agents must use callable central MCP tools for covered operations. A configured or enabled host entry is not proof that MCP is available: verify discovery with an actual read-only call. `hk_dev_start`, `hk_dev_stop`, and `hk_dev_logs` stream lifecycle progress; `hk_dev_status` needs only the optional workspace selector. Development never uses a run ID. If MCP is unavailable, report the registration, startup, workspace-routing, or task-reload blocker and obtain explicit user approval before choosing an equivalent CLI output contract. JSON and NDJSON use the versioned `hk.dev/v2` envelope; `plain` is the uncolored scalar/text mode:

```bash
./hk dev --detach --output json
./hk dev status --output json
./hk dev logs --cursor <next_cursor> --output json
./hk dev logs --follow --output ndjson
```

Structured CLI and MCP starts also request machine-readable output from supported child tools. The QA catalog exposes the effective `agent_command` when it differs from the human command; tools without a JSON mode still run without terminal color and remain bounded by the surrounding `hk` envelope and log cursor contract.

For fast visual checks, batch up to eight local routes through the active seeded development session:

```bash
./hk screenshot /dashboard /admin/status
./hk screenshot /admin/status --viewport mobile --theme dark --output json
```

One browser launch and login serve the complete batch. Output is confined to managed workspace artifacts, includes timing and image metadata, and never syncs into tracked screenshots. MCP-capable agents use `hk_screenshot`, which returns the same structured result plus PNG resource links. It accepts local routes and presentation options only; it cannot browse arbitrary URLs, select an output directory, or receive credentials.

Setup, QA, builds, and smoke tests remain finite asynchronous runs:

```bash
./hk run status <run_id> --output json
./hk run logs <run_id> --limit 80 --output json
```

Complete logs and artifacts stay on disk at the paths returned by `hk`, keeping successful terminal and agent output small.

List recent finite work before starting a duplicate run, and use `next_cursor` when polling logs so an agent does not reload the same context:

```bash
./hk run list --output json
./hk run logs <run_id> --cursor <next_cursor> --output json
```

When multiple Git worktrees are in use, `hk` allocates a stable workspace ID, ports, Compose project, data directory, and run state for each one. Inspect those advanced details with `./hk workspace status` or `./hk workspace list`; agents should trust the workspace ID and URLs in structured output rather than assuming ports.

## Maintain Source

Formatting and pinned Go migrations are part of the developer platform rather than ad hoc shell conventions:

```bash
./hk fmt
./hk fmt check
./hk fmt --scope frontend
./hk fmt check --scope frontend
./hk fix check
./hk fix
```

`fmt` owns Go and frontend formatting; Go remains the default scope for compatibility. `fix` owns pinned Go migrations. Their `check` modes are non-mutating and are what QA uses. The developer MCP does not expose source-rewrite tools, so an agent must make this mutation explicitly through the confined CLI and review the resulting diff.

Shared dependency snapshots and workspace state are inspectable. Pruning is always a dry run unless `--apply` is supplied, and it can remove only hk-managed entries:

```bash
./hk cache status
./hk cache prune
```

## Build and Smoke Test

Build variants and targets are selected through the live catalog:

```bash
./hk build binary
./hk build image
./hk build image --variant cloud
./hk smoke --variant cloud
```

The cloud image is local-only and cannot be published by `hk`. Local image references include the workspace ID so concurrent worktrees cannot overwrite or smoke-test each other's images; query `./hk catalog --output json` for the exact reference. Public release images continue to contain only self-hosted binaries; managed cloud continues to consume its separate cloud-tagged ARM64 artifact.

## Validate

Use the change-aware profile while iterating, the PR profile before review, and the full profile for exhaustive self-hosted, cloud, and image coverage:

```bash
./hk qa
./hk qa pr
./hk qa full
```

Inspect a plan without running it:

```bash
./hk qa plan changed --output json
```

The PR profile is the CI contract. Workflows resolve their execution groups from `hk` rather than copying gate lists or cloud tags. Gate names are stable, all selected gates run even when another gate fails, and complete gate logs remain available as run artifacts. Before opening a PR, report the profile and gate IDs that passed and explain anything you could not run.

## Multiple Worktrees and Agents

Git worktree creation and deletion remain your responsibility. `hk` never creates or deletes a worktree and never runs `git clean`.

```bash
./hk workspace list
./hk workspace handoff --output json
```

Workspace state, application data, frontend dependency links, services, logs, and generated configuration remain isolated. Container images, verified managed toolchains, safe download/compiler caches, and explicitly shared package caches are reused. This allows development and QA to run concurrently without fixed-port, host-native dependency, or Compose-project collisions.

## Local Developer MCP

MCP-capable agents can use the same application services without parsing shell output. The developer server is model-agnostic: Claude, Gemini, GPT, or another model receives the same typed tools when its host supports local stdio MCP.

Choose one long-lived HitKeep clone as the central launch point. Its checked-in `./hk` launcher keeps that broker aligned with the clone's current source. Because the executable is produced locally instead of downloaded, macOS distribution signing and quarantine workarounds are not involved.

The live one-time registration comes from:

```bash
./hk mcp manifest
```

Human output prints a copyable generic `mcpServers` object. `./hk mcp manifest --output json` returns the same registration in the standard `hk.dev/v2` envelope, with the `hk.dev/mcp-manifest/v4` schema, stateless protocol mode, central scope, server-catalog routing, workspace-MCP delegation, progress-only notifications, stable server name, absolute local launcher, transport, and pinned `--fallback-workspace` argument. Treat this output as authoritative instead of copying host-specific config paths into agent instructions. Regenerate existing registrations after this manifest change.

Add that object to the host's user-level MCP configuration, approve the local binary when the host asks, then restart or reload the host. Confirm the registered server name, absolute executable, and arguments against the manifest, then verify discovery with an actual read-only call such as `hk_workspace_status`; a configured or enabled status alone is insufficient. Existing conversations generally do not dynamically acquire newly added MCP configuration. This one-time safety decision belongs to the host and is deliberately not bypassed by `hk`.

The generated central registration is equivalent to:

```json
{
  "mcpServers": {
    "hitkeep-dev": {
      "command": "/absolute/path/to/the/central/hitkeep/clone/hk",
      "args": ["mcp", "serve", "--fallback-workspace", "/absolute/path/to/the/central/hitkeep/clone"]
    }
  }
}
```

The central `./hk mcp serve --fallback-workspace <path>` resolves each request from immutable server configuration and the fallback clone's live HitKeep workspace catalog, then delegates through a request-scoped child stdio connection to that workspace's own `./hk --workspace <path> mcp serve`. The optional `workspace` selector accepts a catalogued workspace ID or absolute path and defaults to the configured fallback. `./hk --workspace <path> mcp serve` remains the fixed-worktree compatibility form; no-flag `./hk mcp serve` uses the current directory as the fallback.

The central MCP uses local stdio only. It forwards cancellation and progress to the selected workspace MCP, then closes that request's child connection. Use `hk_dev_status` and development event cursors for services; use `hk_run_list` and finite-run log cursors for setup, QA, build, and smoke work. Visual QA uses the bounded synchronous `hk_screenshot` operation against the selected workspace's active local session. It is separate from HitKeep's production analytics `/mcp` endpoint and exposes only bounded workspace, setup, development, screenshot, build, smoke, QA, run-status, cancellation, and log operations. It cannot execute arbitrary commands, rewrite source, mutate Git, publish artifacts, manage credentials, delete worktrees, perform cleanup, or deploy infrastructure. Stdout is reserved for JSON-RPC.

The canonical contributor skills live under [`.agents/skills`](./.agents/skills):

- `hitkeep-development` routes implementation and the contribution lifecycle.
- `hitkeep-workspace` owns isolated worktree state, services, ports, runs, logs, and handoff.
- `hitkeep-qa` owns live QA planning, execution, and completion evidence.
- `hitkeep-i18n` owns dashboard localization procedure.

These are normal publishable skill bodies, not generated proxies. Use callable local developer MCP operations for every covered action. Diagnose and report MCP unavailability, then obtain explicit user approval before using an equivalent structured CLI fallback. The separate [`skills/`](./skills/) directory is the end-user analytics pack and supplies transport-neutral procedures to HitKeep Ask AI.

## Documentation Authority

Development information has one owner:

1. `./hk help` and structured catalogs describe current commands and facts.
2. Developer MCP exposes typed live workspace operations.
3. Contributor skills provide workflow judgment and routing.
4. `AGENTS.md` defines repository policy and invariants.
5. This guide provides the human onboarding narrative.

Do not copy build tags, cloud defaults, port assignments, tool versions, or QA matrices into new documentation. Query `hk` instead. `./hk skills check` verifies both canonical skill packs and the product/contributor boundary.

## Pull Requests

- Keep changes focused and explain the user-visible or maintenance outcome.
- Add tests at the narrowest useful level, then run the appropriate `hk` QA profile.
- Describe the documentation impact when public behavior changes. The rendered docs are public, but their source repository is private; maintainers apply website changes that external contributors cannot make directly.
- Do not include credentials, customer data, private infrastructure details, or private screenshots.
- Follow the repository's conventional-commit and release guidance.

New developer workflows belong in `internal/devtool` and must be exposed consistently through the human CLI, structured CLI output, and MCP adapters. Do not add parallel Make or shell-script entry points.
