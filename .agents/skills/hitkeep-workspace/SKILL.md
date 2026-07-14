---
name: hitkeep-workspace
description: 'Operate HitKeep safely in isolated Git worktrees. Use when inspecting workspace state, resolving ports or URLs, starting or stopping development services, coordinating concurrent agents or QA runs, reusing active runs, reading bounded logs, or preparing a secret-free handoff.'
---

# HitKeep Workspace

Treat `AGENTS.md` as policy and `hk` as the live workspace authority. Git worktree creation and deletion remain external responsibilities. Never infer workspace identity from the directory name or branch.

## Inspect First

Prefer these local developer MCP operations:

- `hk_workspace_status` for the current worktree, ports, URLs, and change summary.
- `hk_workspace_list` to see isolated HitKeep workspaces.
- `hk_workspace_handoff` for compact continuation context.
- `hk_run_status` and `hk_logs_tail` for asynchronous work.
- `hk_run_list` to reuse or inspect bounded recent work before starting another run.
- `hk_dev_start` and `hk_dev_stop` for this worktree's services.
- `hk_run_cancel` for one validated active run.

When MCP is unavailable, discover the equivalent workspace and run commands through `./hk help` and request `--output json`; add `--detach` for action parity. Do not parse terminal prose.

Always use the workspace ID, Compose project, paths, ports, URLs, and run IDs returned by `hk`. Verify an envelope's workspace ID before using a run ID. Never assume conventional ports are free or share mutable state between worktrees.

## Operate

1. Inspect status and active runs before setup, development, browser work, e2e, or QA. Reuse an equivalent active run rather than duplicating it.
2. Query the live variant catalog, then start development only when the task needs runtime services.
3. Record the returned run ID and poll status with reasonable intervals. Do not treat client disconnect or a slow call as failure.
4. Read bounded log tails for diagnosis. Carry `next_cursor` into subsequent reads so repeated polling does not reload old output. Use returned artifact paths for complete local diagnostics instead of loading full logs into context.
5. Stop only services owned by the selected workspace. Cancellation targets one validated run ID; it is not workspace cleanup.
6. Before handoff, refresh status and request handoff context. Report workspace ID, active runs, usable URLs, changed-path summary, failures, and the next safe action.

Do not run `git clean`, remove unmanaged files, delete worktrees, expose secrets, copy mutable state between workspaces, or manually reserve ports. Reject paths that resolve outside the selected worktree, including symlink escapes.
