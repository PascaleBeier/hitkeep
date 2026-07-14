# Contributing to HitKeep

Thank you for improving HitKeep. The repository-owned `./hk` developer CLI is the canonical workflow surface for people, automation, and coding agents. It derives current build variants, workspace state, development defaults, and QA gates from one typed catalog.

Repository policy and product invariants live in [`AGENTS.md`](./AGENTS.md). Read it before making a change.

## Start in One Command

```bash
git clone https://github.com/pascalebeier/hitkeep.git
cd hitkeep
./hk setup
```

`./hk` bootstraps itself with the exact Go version from `go.mod`. When that Go toolchain is unavailable, it can build itself through the pinned Go container image using writable host caches. `setup` prepares Go and frontend dependencies for the selected worktree. Existing mutable `node_modules` directories are safely migrated to content-addressed, read-only snapshots shared by warm worktrees.

Check prerequisites and the current isolated workspace at any time:

```bash
./hk doctor
./hk workspace status
```

Use `./hk help` and subcommand help for the current command reference. Use `./hk catalog --output json` when a tool or agent needs the live variant and QA catalogs.

## Develop

Start the fast native workflow:

```bash
./hk dev --seed
```

The application and dashboard run natively; Docker Compose supplies the isolated Mailpit service. `./hk doctor` reports native development ready only when both the exact host toolchain and that service runtime are available.

Use the reproducible container-backed workflow when host prerequisites are unavailable or container parity matters:

```bash
./hk dev --runtime container --seed
```

Choose the cloud variant only for local managed-cloud parity work:

```bash
./hk dev --variant cloud
```

`hk` allocates a stable workspace ID, ports, Compose project, data directory, run logs, and generated runtime configuration for each Git worktree. Always open the URLs returned by `./hk workspace status`; do not assume the conventional ports are available.

Long operations wait by default for humans. Add `--detach` to receive a run ID immediately:

```bash
./hk dev --detach --output json
./hk run status <run_id> --output json
./hk run logs <run_id> --limit 80 --output json
```

Complete logs and artifacts stay on disk at the paths returned by `hk`, keeping successful terminal and agent output small.

List recent work before starting a duplicate run, and use `next_cursor` when polling logs so an agent does not reload the same context:

```bash
./hk run list --output json
./hk run logs <run_id> --cursor <next_cursor> --output json
```

## Maintain Go Source

Formatting and pinned Go migrations are part of the developer platform rather than ad hoc shell conventions:

```bash
./hk fmt
./hk fmt check
./hk fix check
./hk fix
```

`fmt` and `fix` deliberately rewrite repository Go files; their `check` modes are non-mutating and are what QA uses. The developer MCP does not expose source-rewrite tools, so an agent must make this mutation explicitly through the confined CLI and review the resulting diff.

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

Workspace state, application data, mutable frontend tool caches, services, logs, and generated configuration remain isolated. Immutable dependency snapshots and safe download/compiler caches are shared. This allows development and QA to run concurrently without fixed-port, dependency-cache, or Compose-project collisions.

## Local Developer MCP

MCP-capable agents can use the same application services without parsing shell output. The developer server is model-agnostic: Claude, Gemini, GPT, or another model receives the same typed tools when its host supports local stdio MCP.

Setup is automatic for supported hosts through small, committed project-scoped MCP registrations. They use only worktree-relative paths, so every Git worktree starts its own `hitkeep-dev` process without modifying a user's global client configuration. The live client catalog and registration paths come from:

```bash
./hk mcp manifest
```

Human output lists the checked-in integrations for Codex, Claude Code, Gemini CLI, Cursor, and VS Code, followed by a copyable generic `mcpServers` object for any other stdio-capable host. `./hk mcp manifest --output json` returns both in the standard `hk.dev/v1` envelope, including its own `hk.dev/mcp-manifest/v1` schema, workspace ID, isolated fallback server name, stable launcher path, transport, and arguments. Treat this output as authoritative instead of copying client paths into agent instructions.

On first use, approve or trust the repository when the host asks, then restart or reload the host and verify discovery with its MCP status UI or `hk_workspace_status`. Existing conversations generally do not dynamically acquire newly added MCP configuration. This one-time safety decision belongs to the host and is deliberately not bypassed by `hk`.

Every registration launches `./hk mcp serve` over stdio for the selected worktree. For a host without a checked-in project convention, use the generated generic registration:

The generated registration is equivalent to:

```json
{
  "mcpServers": {
    "hitkeep-dev-<workspace-id>": {
      "command": "/absolute/path/to/worktree/hk",
      "args": ["--workspace", "/absolute/path/to/worktree", "mcp", "serve"]
    }
  }
}
```

The developer MCP uses local stdio only. Configure one process per worktree. Use `hk_run_list` before starting work and pass the returned log cursor to incremental log reads. It is separate from HitKeep's production analytics `/mcp` endpoint and exposes only bounded workspace, setup, development, build, smoke, QA, run-status, cancellation, and log operations. It cannot execute arbitrary commands, rewrite source, mutate Git, publish artifacts, manage credentials, delete worktrees, perform cleanup, or deploy infrastructure. Stdout is reserved for JSON-RPC.

The canonical contributor skills live under [`.agents/skills`](./.agents/skills):

- `hitkeep-development` routes implementation and the contribution lifecycle.
- `hitkeep-workspace` owns isolated worktree state, services, ports, runs, logs, and handoff.
- `hitkeep-qa` owns live QA planning, execution, and completion evidence.
- `hitkeep-i18n` owns dashboard localization procedure.

These are normal publishable skill bodies, not generated proxies. Prefer the local developer MCP and use their structured CLI fallback when MCP is unavailable. The separate [`skills/`](./skills/) directory is the end-user analytics pack and supplies transport-neutral procedures to HitKeep Ask AI.

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

The Makefile remains a small compatibility adapter for familiar entry points; new workflows belong in `internal/devtool` and must be exposed consistently through CLI JSON and MCP adapters.
