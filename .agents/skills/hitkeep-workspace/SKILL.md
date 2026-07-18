---
name: hitkeep-workspace
description: 'Operate HitKeep safely in isolated Git worktrees. Use when inspecting workspace state, resolving ports or URLs, starting or stopping development services, coordinating concurrent agents or QA runs, reusing active runs, reading bounded logs, or preparing a secret-free handoff.'
---

# HitKeep Workspace

Treat `AGENTS.md` as policy and `hk` as the live workspace authority. Git worktree creation and deletion remain external responsibilities. Never infer workspace identity from the directory name or branch.

## Inspect First

Use these local developer MCP operations whenever they are callable:

- `hk_workspace_status` for the current worktree, ports, URLs, and change summary.
- `hk_workspace_list` to see isolated HitKeep workspaces.
- `hk_workspace_handoff` for compact continuation context.
- `hk_run_status` and `hk_logs_tail` for asynchronous work.
- `hk_run_list` to reuse or inspect bounded recent work before starting another run.
- `hk_dev_start`, `hk_dev_status`, `hk_dev_logs`, and `hk_dev_stop` for this worktree's container-only development session.
- `hk_screenshot` for batched, workspace-managed visual-QA artifacts from the active local session.
- `hk_run_cancel` for one validated active run.

Do not invoke equivalent workspace or run CLI commands unless the relevant MCP tool is not callable, the registration, startup, root-routing, or task-reload blocker has been reported, and the user has given explicit user approval. After approval, discover the equivalent command through `./hk catalog commands --output json`, request `--output json`, and add `--detach` for action parity. Do not parse terminal prose.

Always use the workspace ID, Compose project, paths, ports, URLs, development event cursors, and finite-operation run IDs returned by `hk`. Verify an envelope's workspace ID before using a cursor or run ID. Never assume conventional ports are free or share mutable state between worktrees.

## Operate

1. Inspect status, the development session, and active finite runs before setup, browser work, e2e, or QA. Reuse equivalent active work rather than duplicating it.
2. Query the live variant catalog, then start the container-only development session only when the task needs services.
3. Development start streams until ready or failed and requires no session ID. Finite operations return run IDs and are polled with reasonable intervals. Do not treat client disconnect or a slow call as failure.
4. Read bounded log tails for diagnosis. Carry `next_cursor` into subsequent development or run reads so repeated polling does not reload old output. Use returned artifact paths for complete local diagnostics instead of loading full logs into context.
5. Stop development with the workspace-scoped stop operation. Cancelling a development log observer never stops services; finite run cancellation still targets one validated run ID.
6. Before handoff, refresh status and request handoff context. Report workspace ID, active runs, usable URLs, changed-path summary, failures, and the next safe action.

Do not run `git clean`, remove unmanaged files, delete worktrees, expose secrets, copy mutable state between workspaces, or manually reserve ports. Reject paths that resolve outside the selected worktree, including symlink escapes.
