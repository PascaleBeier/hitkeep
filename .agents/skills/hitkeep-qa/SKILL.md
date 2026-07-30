---
name: hitkeep-qa
description: 'Plan, execute, investigate, and report HitKeep quality gates through hk. Use while iterating, before calling contributor work complete, when choosing change-aware versus PR-parity versus exhaustive validation, when CI parity matters, or when investigating a failed or asynchronous gate.'
---

# HitKeep QA

Treat `AGENTS.md` as repository policy and the live `hk` QA catalog as workflow truth. Never reproduce the gate matrix, commands, tags, or tool versions in this skill or in a contribution plan.

## Select

Use `hk_qa_plan` whenever it is callable. If it is absent or fails, report whether registration, startup, workspace routing, or task reload is blocking MCP and obtain explicit user approval before using an equivalent CLI action. After approval, discover the planning command through `./hk qa --help` and request `--output json`.

- Use the change-aware profile while iterating.
- Use the PR-parity profile before review.
- Use the exhaustive profile when release risk, cloud-tagged behavior, container behavior, or the request explicitly requires it.

Honor planner escalation. Query `hitkeep-dev://catalog/qa` or the structured CLI catalog for current profiles and gate definitions rather than guessing or copying commands.

Use `hk_run_status` to observe the returned run, `hk_logs_tail` for bounded diagnosis, and `hk_run_cancel` only for the intended active run. Gate-specific run resources provide deeper bounded failure context without loading complete logs.

## Run and Observe

1. Inspect workspace status for an equivalent active QA run before starting another.
2. Start through `hk_qa_start`. Use the structured CLI action discovered from help with `--detach --output json` only after the MCP blocker has been reported and the user has approved fallback. Keep the returned workspace ID and run ID.
3. Poll status rather than restarting a slow run or assuming client disconnect stopped it.
4. On failure, read the bounded run tail, then the gate-specific bounded resource. Open the complete local artifact only when the tail is insufficient.
5. Fix the root cause and rerun the smallest relevant selection from the live catalog before rerunning the required profile.
6. Let all selected gates finish; one failure must not hide independent results. Cancel only the intended validated run.

QA source checks never rewrite files. When they report formatting or Go migration drift, route the explicit write through the developer CLI, review the changed paths, and then rerun the failed gate.

## Completion Report

Report the workspace ID, profile, stable gate IDs, final run status, and any gates that could not run with the concrete reason. A green focused run does not replace required PR or exhaustive evidence. Keep passing output compact; retain run IDs and artifact paths instead of pasting logs.
