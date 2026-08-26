# Filesystem and package-layout migration manifest

This manifest is the Phase 0C evidence gate for Phases 9 and 10. It records current ownership and risk; it is not permission to move a package or replace native filesystem behavior. Refresh dependency evidence immediately before each slice.

## Rules

- Move packages only in dependency order and only after callers, imports, build tags, generated files, OS/CGO constraints, tests, and cycle risk are known.
- Preserve behavior in move-only slices. Do not combine a package move with filesystem abstraction or runtime changes.
- Use Afero for ordinary injectable host I/O, fileflow for conflict-safe ordinary copy/move operations, and pathologize only for untrusted path segments beneath a trusted root.
- Keep embedded `io/fs`, DuckDB files, WAL and recovery state, fsync, locks, atomic replacement, process/PID coordination, `/proc`, and cgroup probes native.
- Canonical database and migration paths must retain native no-replace and durability semantics. Identical fileflow destinations are not implicit success where source deletion would alter canonical state.

## Current top-level package state

Already moved from `internal/`: `appurl`, `exportfmt`, `hklog`, `analyticscatalog`, `jsonapi`, `localization`, `config`, and `mcptest`.

Remaining indexed families:

- `ai`, `aianalytics`, `analyticstools`, `api`, `assetstore`, `auth`
- `blocking`, `cluster`, `database`, `devtool`, `entitlements`
- `importables`, `ingest`, `ipmeta`, `mailables`, `mailer`
- `mcpserver`, `opportunities`, `server`, `socialauth`, `sso`
- `testutil`, `webhookdispatcher`, `worker`

No remaining Go package-import cycle is currently proven. Reported Go cycles are call recursion or test-mock cycles, not import cycles; that does not make a package leaf-safe.

## Rejected next move candidates

### `internal/assetstore`

Not a leaf. Its one implementation file has broad command, server QR-code, recovery, seed, and smoke consumers. It owns deletion and user/site-derived path construction. A future wave must review path containment and server wiring before moving it.

### `internal/blocking`

Not a leaf. It combines CIDR/IP filtering, embedded spam data, ordinary persisted feed files, HTTP refresh, background refresh, and ingest/AI-fetch/QR runtime consumers. Keep embedded data on `io/fs`; isolate only the ordinary persistence boundary in a separate Afero slice.

### `internal/analyticstools`

Not a leaf. It imports the analytics catalog, API, database, JSON API, and GoAI, with dependents in MCP, opportunities, and Ask AI server wiring. It has no direct filesystem work and belongs in a later coordinated domain wave.

### `internal/entitlements`

Not a leaf. It crosses configuration, database, worker, and cloud/OSS build-tag variants. All variants and consumers must move atomically in a later coordinated wave.

Conclusion: no currently reviewed candidate is approved for the next move-only wave. Select a different low-coupling package only after authoritative dependency evidence is available.

## Filesystem operation classes

### Wave A: ordinary Afero candidates

These are ordinary reads/writes where a filesystem passed from the composition root may improve isolation without weakening OS semantics:

- `internal/aianalytics/agents_data.go`: `LoadAIAgentData`, `SaveAIAgentData`
- `internal/blocking/spam_data.go`: `LoadSpamFeedData`, `SaveSpamFeedData`
- bounded developer metadata/state reads in `internal/devtool/docs.go`, `qa_evidence.go`, `runs.go`, and `workspace.go`
- import-source reads in `internal/importables`
- generated public-file writes in `internal/ipmeta/ipmetagen/bin_assets.go`

Each slice must preserve permissions, no-overwrite/replacement behavior, error paths, and real-filesystem coverage where OS behavior matters. Do not introduce a repository-wide filesystem wrapper.

### Wave B: fileflow candidates

The clearest ordinary copy candidate is `internal/devtool/runs.go::copyTree`. Before adoption, prove:

- final returned destination paths are honored
- conflicts never clobber different content
- identical destination behavior matches source-retention requirements
- permissions, cleanup, partial failures, and cross-filesystem behavior remain explicit

Do not use fileflow for database relocation, compaction, recovery, tenant splitting, migration markers, locks, or atomic canonical-state swaps.

### Wave B: pathologize review boundaries

Use `pathologize.Join` only when an untrusted segment is placed under a trusted configured root. Review:

- import staging and source filenames
- retention archive discovery/import paths
- staged server upload paths
- takeout export filenames
- developer artifact and screenshot paths
- asset-store key/site path construction

Existing containment and escape tests remain authoritative. Trusted configured paths must not be silently sanitized.

### Embedded `io/fs`: keep unchanged

- AI-agent embedded data
- spam embedded data
- mailer locales
- control and tenant migrations
- embedded dashboard/static assets

Wrapping these in Afero adds no useful test seam.

### Native exceptions: keep unchanged

- `/proc` and cgroup memory probes
- DuckDB database, WAL, compaction, recovery, split, rebuild, and migration files
- migration checkpoints, markers, manifests, directory sync, locks, and atomic replacement
- devtool process execution, PID/lock coordination, managed toolchains, cancellation markers, and Git/GitHub orchestration

Any future change in these areas requires a separately reviewed state-machine/fault-injection slice. Ordinary filesystem abstractions are not a durability substitute.

## Required pre-move record

Every future move entry must record:

1. old and new import paths
2. package owner and one-sentence domain purpose
3. direct and transitive dependents
4. imports and import-cycle result
5. build tags, OS/CGO files, embedded/generated files, and developer/production boundary status
6. filesystem classification for every operation
7. focused and affected-package test targets
8. rollback procedure and confirmation that no compatibility shim is required

No additional `internal/` move begins until this record is complete for the selected package.
