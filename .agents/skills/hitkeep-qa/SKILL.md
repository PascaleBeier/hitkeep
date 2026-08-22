---
name: hitkeep-qa
description: 'Plan, execute, investigate, and report HitKeep quality gates through hk. Use while iterating, before calling contributor work complete, when choosing change-aware versus PR-parity versus exhaustive validation, when CI parity matters, or when investigating a failed or asynchronous gate.'
---

# HitKeep QA

Treat `AGENTS.md` as repository policy and the live `hk` QA catalog as workflow truth. Never reproduce the gate matrix, commands, tags, or tool versions in this skill or in a contribution plan.

## Select

Use `hk_qa_plan` whenever it is callable. If it is absent or fails, report whether registration, startup, workspace routing, or task reload is blocking MCP and obtain explicit user approval before using an equivalent CLI action. After approval, discover the planning command through `./hk qa --help` and request `--output json`.

- Use `changed` while iterating.
- Use `complete` before declaring contributor work complete; it runs the deepest locally appropriate gates for affected areas.
- Use `pr` only for exact unchanged CI parity.
- Use `full` when release risk, cloud-tagged behavior, container behavior, or the request explicitly requires exhaustive validation.

Honor planner escalation. Query `hitkeep-dev://catalog/qa` or the structured CLI catalog for current profiles and gate definitions rather than guessing or copying commands.

Use `hk_run_status` to observe the returned run and request bounded cursor-addressed run or gate logs. Use `hk_run_cancel` only for the intended exact observed active run.

## Run and Observe

1. Inspect `hk_context` and known run status for equivalent active QA work before starting another.
2. Persist a source-bound plan with `hk_qa_plan`, then pass its required `plan_id` to `hk_run_start`. Use a structured CLI action with `--detach --output json` only after the MCP blocker has been reported and the user has approved fallback. Keep the returned workspace ID and run ID.
3. Poll `hk_run_status` rather than restarting a slow run or assuming client disconnect stopped it.
4. On failure, request the bounded run tail, then a bounded gate-specific view from `hk_run_status`. Open the complete local artifact only when that context is insufficient.
5. Fix the root cause and rerun the smallest relevant selection from the live catalog before rerunning the required profile.
6. Let all selected gates finish; one failure must not hide independent results. Cancel only the intended validated run.

QA source checks never rewrite files. When they report formatting or Go migration drift, route the explicit write through the developer CLI, review the changed paths, and then rerun the failed gate.

## Completion Report

Report the workspace ID, profile, stable gate IDs, final run status, and any gates that could not run with the concrete reason. A green focused run does not replace required PR or exhaustive evidence. Keep passing output compact; retain run IDs and artifact paths instead of pasting logs.
