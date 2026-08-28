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

### `internal/realtime`

Owner: `Broker`, `Publish`, and `Subscription` coordinate live delivery. Consumers: the site and shared realtime handlers in `internal/server`, plus the dashboard coordinator. Build tags: unconditional. Filesystem: none; its in-memory channels and HTTP delivery are native runtime semantics. Coupling/cycle: server-handler and channel-lifetime coupling make it non-leaf; no import cycle is proven. Tests: the site and shared realtime-handler tests. Disposition: **stay internal / blocked**; no move is approved until those consumers move together.

### `internal/reporting`

Owner: report scheduling and period helpers (`ValidateSchedule`, `NextOccurrence`, `PeriodBounds`, `CatchUpWindow`) and reporting tokens. Consumers: database report stores, worker reports/scheduler, user report handlers, and API reporting. Build tags: unconditional. Filesystem: none. Coupling/cycle: it crosses API, handlers, worker scheduling, and persistence; no import cycle is proven. Tests: schedule, database, handler, and worker reporting tests. Disposition: **stay internal / blocked** pending a coordinated reporting-domain move.

### `internal/searchconsole`

Owner: Search Console OAuth client, token/property operations, and error classification/diagnosis. Consumers: `cmd/hitkeep`, `internal/server`, shared server context, and report handlers. Build tags: unconditional. Filesystem: none. Coupling/cycle: OAuth, network, configuration, server wiring, and report-handler coupling make it non-leaf; no import cycle is proven. Tests: focused Search Console client/error tests and consuming server-handler tests. Disposition: **stay internal / blocked**; this security-sensitive integration must move atomically with its wiring.

### `internal/security`

Owner: TOTP, recovery-code, and passkey primitives. Consumers: user security/auth handlers and database security storage. Build tags: unconditional. Filesystem: none. Coupling/cycle: authentication and credential-persistence coupling is security-sensitive; no import cycle is proven. Tests: focused TOTP, recovery, passkey, handler, and storage tests. Disposition: **stay internal / blocked**; no move is approved outside a security-reviewed slice.

### `internal/takeout`

Owner: `TakeoutService` and its export query builders. Consumers: server takeout handlers. Build tags: unconditional. Filesystem: user-derived export paths and file creation require existing containment, permissions, and cleanup behavior. Coupling/cycle: database, authorization, export, and persistence semantics make it non-leaf; no import cycle is proven. Tests: focused takeout service and handler tests. Disposition: **stay internal / blocked**; do not move or abstract its filesystem behavior without a separately reviewed export-security slice.

### `internal/webhookdispatcher/webhooks`

Owner: webhook `Dispatcher`, `Emitter`, `Worker`, and `Sweeper`. Consumers: `cmd/hitkeep` leader-service startup, `internal/server.New`, and the webhook database store/migration. Build tags: unconditional. Filesystem: no ordinary filesystem boundary. Coupling/cycle: database, queue, network-delivery, and concurrent worker lifecycle coupling make it non-leaf; no import cycle is proven. Tests: dispatcher, emitter, worker, sweeper, and server webhook-handler tests. Disposition: **stay internal / blocked**; no move is approved outside a coordinated persistence and delivery slice.

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
