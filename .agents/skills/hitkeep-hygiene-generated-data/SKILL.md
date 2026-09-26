---
name: hitkeep-hygiene-generated-data
description: Refresh HitKeep's checked-in bot/AI-agent, IP metadata, and spam-filter datasets through their canonical generators. Use for routine generated-data maintenance; do not use for hand-editing generated assets or changing detector policy.
---

# HitKeep Generated Data Hygiene

Treat `AGENTS.md` as policy and load `$hitkeep-development` plus `$hitkeep-qa`.

## Run canonical refreshers

Refresh the AI-agent, spam, and IP metadata families independently through the corresponding `ai-agents`, `spam`, and `ipmeta` targets of `go run ./cmd/data-refresh`. The `duckdb` target maintains the separate signed extension bundle; release builds keep the standalone updater's offline `-check` command.

Discover exact invocation, required container/runtime, inputs, and outputs from the tracked scripts, generator entry points, workflows, and live `hk` catalog. Execute those surfaces; do not recreate downloads or transformations. Never print or persist credentials. Treat downloaded content as untrusted and stop on a changed source contract, checksum failure, malformed input, or unexpectedly unbounded output.

Run the families independently so one failure does not hide the other results. Do not hand-correct generated output. If a generator fails, diagnose and report it; change generator code only when the user has authorized fixing the generator itself.

## Review and validate

For each family, record the canonical surface, upstream source/version metadata when available, changed generated files, and before/after size or record-count signals. Inspect diffs for truncation, schema drift, surprising deletions, binary bloat, private data, and unrelated changes. Rerun when practical and require an empty second diff as a determinism check.

Use the live QA planner for focused generator tests and generated-data integrity gates. If delegated, leave consolidated PR-parity QA to the parent. Return per-family status, stable gate IDs, run IDs, and blockers.
