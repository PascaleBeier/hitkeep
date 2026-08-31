# Configuration Refactor Progress

Branch: `feat/config_refactor`

Implementation plan: [Cobra, Viper, configuration, filesystem, release, and layout migration](../architecture/cobra-viper-config-go-layout-migration.md)

Evidence manifests: [filesystem and package layout](filesystem-layout-manifest.md)

This folder is the durable progress ledger for the migration. Update it in the same slice that changes implementation state. Do not use it as a second command, configuration, dependency, or QA catalog; those remain owned by code and `hk`.

## Compatibility invariants

- Existing `HITKEEP_*` variables, flags, defaults, parsing, warnings, normalization, and build-variant behavior remain compatible throughout 2.x.
- Repeated canonical/deprecated aliases retain sequential last-occurrence-wins behavior.
- Config files are additive and explicitly selected in 2.x; existing env/flag deployments do not discover files implicitly.
- Persistence-sensitive defaults remain consistent across binary, image, Compose, Helm, examples, and docs, with issue #288 covered by container deletion/recreation upgrade tests.
- Every slice is independently releasable and reversible. Contraction is deferred until separately authorized.
- Runtime packages receive typed configuration; Viper and Cobra stay at assembly/routing boundaries.
- Storage durability, authorization, redaction, and production/developer dependency boundaries are never simplified away.

## Slice ledger

| Slice | State | Scope | Evidence |
|---|---|---|---|
| 0A | Completed | Characterize current config/catalog/flag contracts; inventory CLI and release surfaces | `go test -race ./internal/config`; table-driven canonical/deprecated order contract; Gortex CLI/release discovery |
| 0B | Completed | Add configuration-surface drift fixture and issue #288 failure-shaped upgrade fixture | Dockerfile data path must be image-defined beneath a declared volume; `go test -race ./internal/devtool` |
| 0C | Completed | Inventory filesystem operations and dependency-ordered `internal/` move manifest | The durable filesystem/package manifest records complete direct-child and filesystem-family evidence, classifications, native exclusions, rejected move candidates, and the shared sentinel. Every blocked or unproven disposition remains frozen: no record grants move readiness, and Phases 9/10 remain frozen pending separately authorized dependency/owner/build-tag/generated-file proof |
| 1 | Completed | Make catalog authoritative and generate neutral `hitkeep.example.yaml` | Catalog `config_file_key` added; schema `hitkeep.config/v2`; deterministic example covers each self-hosted key once |
| 2 | Completed | Add instance-based Viper assembler in shadow parity | Viper 1.21.0 + Afero explicit YAML; full catalog types, strict keys, normalization, warnings, and precedence proven |
| 3 | Completed | Production Cobra root with legacy-compatible routing | Factory-built Cobra root owns top-level execution, exact first-argument routing, leading-only config bootstrap normalization, Cobra recovery/import/updater routes, typed Viper client configuration, and the sole process signal owner with real SIGINT/SIGTERM cancellation proof |
| 4 | In progress | Switch production assembly to Viper with complete legacy parity | `config.Load` uses instance-based Viper through an OS-backed Afero boundary; catalog-wide tests cover self-hosted and cloud env/flag/default/alias/invalid-value parity and prove cloud-only descriptors are inert in self-hosted builds; stabilization and final legacy-oracle contraction remain pending |
| 4A | Completed | Add production configuration-file UX | Cobra-owned `config init --output` writes the canonical catalog-derived example without overwrite; `config validate --config` uses the strict explicit Viper loader without starting services or exposing values |
| 4B | Completed | Harden production configuration commands and raw-key validation | Only exact `config init` and `config validate` use Cobra; all other `config` argv falls back unchanged; write/close failures retain their pathname; raw YAML keys must exactly match catalog kebab-case names before Viper normalization |
| 4C | Completed | Make the explicit YAML trust boundary deterministic | Valid flat scalar files remain compatible; duplicate keys, multiple documents, aliases, merge keys, nulls, mappings, and sequences are rejected before Viper normalization without exposing values |
| 5 | Completed | Normalize `hk` Cobra factories and preserve MCP/JSON contracts | No rewrite required: `hk` already has one factory root, centralized render/envelope handling, deterministic command catalog, and guarded production boundary |
| 6 | In progress | Project catalog into Docker, Compose, Helm, examples, and docs artifact | Catalog-owned data-path policy rejects missing files, omitted declarations, and default drift across Docker, Compose, Helm, and examples; `1f53e391` plus hitkeep-docs `c570df3c` add a fail-closed pre-publication docs attestation bound to the exact release/source/docs commits and catalog/example/manifest digests; real cross-repository draft rehearsal and credential-scope proof remain pending |
| 7 | In progress | Enforce issue #288 upgrade, rollback, recreation, and migration interruption gates | Digest-pinned v2.12 migration, two candidate recreations, quiescent fresh-volume rollback, and forced process-kill/restart recovery across all 19 durable split boundaries are proven; Helm now binds the immutable v2.12 chart and candidate checkout chart, loads exact images into Kind, recreates the pod on the same PVC with fresh authenticated verification, and restores the legacy snapshot; real tagged-workflow execution remains pending |
| 8 | In progress | GoReleaser release ownership while preserving future package-manager compatibility | `a3f71980` adds the release digest manifest; tagged Linux amd64/arm64 archives, checksums, raw cloud assets, version metadata, exact-SHA snapshot publication, Darwin/arm64 CGO compilation, and the public Homebrew install/upgrade/uninstall lifecycle are proven; a stabilization release plus Windows CGO feasibility and Scoop lifecycle proof remain pending |
| 8A | In progress | Review and simplify the release process with Ponytail and spf13 Go guidance | Interim review removed three duplicated upgrade job definitions and exact-SHA snapshot `33017615814` passed on `c09d2c6d`; the requested whole-release review must be repeated after docs attestation, Helm lifecycle proof, final graph assembly, and stabilization evidence |
| 9 | In progress | Afero/fileflow/pathologize migration by operation risk | Runtime config uses injected Afero and one cross-filesystem release relocation uses fileflow; the complete operation inventory and remaining justified waves are pending, while database/WAL/fsync/lock operations stay native |
| 10 | In progress | Flatten `internal/` in dependency-order move-only slices | Pure foundations moved: `internal/appurl` → `appurl`, `internal/exportfmt` → `exportfmt`, `internal/hklog` → `hklog`, `internal/analyticscatalog` → `analyticscatalog`, `internal/jsonapi` → `jsonapi`, `internal/localization` → `localization`, `internal/mcptest` → `mcptest`, and stabilized `internal/config` → `config`; no compatibility shims because Go already prohibited external imports |
| 11 | Pending | Stabilize, full QA, CI, docs validation, completion audit | — |

## Current delivery status

- `10045411`: catalog publication classification recorded for distribution-surface synchronization.
- `c05df878`: recovery migrated through the Cobra boundary while retaining its legacy command contract.
- `a3f71980`: release digest manifest added for release-surface verification.
- Phase 0C evidence gate is complete: every direct-child/filesystem family record and the shared sentinel are reconciled. This does not approve a move; all blocked/unproven dispositions remain in force, and Phases 9/10 remain frozen.
- Phase 0C: focused `internal/assetstore` evidence now covers rooted QR asset mkdir, temporary creation, rename cleanup, open, removal, recursive removal, parent pruning, and symlink handling with `os.OpenRoot`/`os.Root` or `os.OpenInRoot`; it is not broad consumer or platform proof.
- Phase 0C: `internal/blocking` remains **decomposition required / stay internal / blocked**. The current proof covers fixed-feed response closure/bounds, same-directory staged cache replacement, final-symlink replacement, whole-update serialization, and nonblocking cancellation-aware leader refresh; it does not claim fsync, durability, cross-platform atomic replacement, or full consumer closure.
- Phase 0C: `internal/analyticstools` has a one-file, three-consumer evidence record and remains **decomposition required / stay internal / blocked** pending comparison-range, authorization, and direct-package proof; later Phase 9/10 waves remain frozen.
- Phase 0C: `internal/entitlements` has a bounded blocked record: managed-cloud site quota proof is atomic across create and transfer, while `MaxTeams`, `MaxTeamMembers`, and membership check-then-act races remain explicit gaps. It remains **decomposition required / stay internal / blocked**; later Phase 9/10 waves remain frozen.
- Phase 0C: `internal/mailer` and its SMTP driver have an exact 31-file, 35-importer blocked record. Embedded locales/templates, sanitized delivery diagnostics, canonical forgot-password enumeration resistance, and admin mail availability/redaction are pinned; live SMTP/TLS/auth/timeout/concurrency and `SendWithHeaders` boundary proof remain gaps. It remains **decomposition required / stay internal / blocked**; later Phase 9/10 waves remain frozen.
- Phase 0C: `internal/realtime` has an exact three-file package record and 14 external importers, with adjacent SSE/server lifecycle proof. Its mutex lifecycle, active-subscriber-only history, resync cursors, shutdown-first close, and one-minute write deadline are pinned; immediate revocation, mock `Last-Event-ID`, persistence/cross-leader replay, and exact broader-consumer closure remain gaps. It remains **decomposition required / stay internal / blocked**; Phases 9/10 remain frozen.
- Phase 0C: `internal/reporting` has an exact three-file, eight-importer blocked record. Deterministic schedule/timezone/DST/period helpers and 32-byte confirmation plus report/recipient-bound HMAC unsubscribe tokens are pinned; broader consumer closure, end-to-end authorization/delivery, persistence, and full timezone/provider edges remain gaps. No runtime defect is asserted. It remains **decomposition required / stay internal / blocked**; Phases 9/10 remain frozen.
- Phase 0C: `internal/searchconsole` has an exact three-file, nine-importer blocked record. One-minute provider and due-run contexts preserve earlier cancellation, while Search Analytics has explicit 25,000-row pages, 250,000-row total, and 100-page error ceilings; live provider/rate-limit behavior, pagination fixtures, and exact wider closure remain gaps. It remains **decomposition required / stay internal / blocked**; Phases 9/10 remain frozen.
- Phase 0C: `internal/security` has an exact six-file, 12-importer blocked record. Recovery-code verification rejects persisted Argon2 parameters outside its canonical work factors before derivation; direct passkey tests, trusted-forwarded-proto proof, and caller bounds for positive TOTP windows/random challenge sizes remain gaps. It remains **decomposition required / stay internal / blocked**; Phases 9/10 remain frozen.
- Phase 0C: `internal/takeout` has an exact three-file, four-importer blocked record. Its user/site/QR source selection routes through DuckDB query/COPY for XLSX, CSV, Parquet, JSON, and NDJSON; `OpenExportFile`/`CleanupExportFile` retain `cleanExportPath` containment. Transitive cycle closure, export size/time/concurrency/durability/platform behavior, caller authorization, and complete privacy/table/retention lifecycle remain gaps. It remains **decomposition required / stay internal / blocked**; Phases 9/10 remain frozen.
- Phase 0C: coupled `internal/webhookdispatcher` and `internal/webhooks` have an exact 15-file, 24-external-importer blocked record. Context-cancelable delivery lifecycle, database-owned deletion/retention, URL/event/signed-delivery boundaries, and no-cycle evidence are pinned; remote delivery, durable retry/retention, raw payload/log privacy, and exhaustive closures remain gaps. No runtime defect is asserted. They remain **decomposition required / stay internal / blocked**; Phases 9/10 remain frozen.
- `6347ccbc`: import and both updater routes are real Cobra children; import client settings resolve through typed Viper/catalog configuration; `ExecuteRoot` is the compatibility adapter for only leading `--config` and first-token server grammar.
- `570bb9aa`: `main` is the sole signal owner; a bounded child-process test invokes the production executable/root, waits for the real application-ready log, then proves graceful SIGINT and SIGTERM cancellation. No production hook or fake Cobra root is used.
- `ec747bf2` and `e82f7c46`: finite `hk` runs own isolated temporary/build/cache roots, remove terminal roots, and validate paths through an `os.OpenRoot` boundary; active/unknown roots and global caches remain untouched.
- `5f614a06` through `283bd589`: `opportunities-smoke` now uses a fresh Cobra/Viper instance and typed catalog-projected configuration while preserving the complete legacy stdlib flag grammar, ordering, streams, and exit codes.
- `8d228bed` through `14c5a06e`: production signal and import/updater subprocess proofs fully drain/reap children, isolate production argv from Go test flags, and assert exactly one error/usage marker; these repair the `go-race-rest` timeout observed in CI `33030507910`.
- `bdb79b37` through `1f53e391`: release finalization now requires an exact successful hitkeep-docs attestation run/artifact before publication; docs commit `c570df3c` separates draft attestation from post-publication metadata/deployment and pins the exact workflow bytes.
- Exact pushed head `e4537eba`: docs CI `33035679971`, docs Actions audit `33035679795`, HitKeep security `33035676326`, and Govulncheck `33035676364` passed; HitKeep CI `33035676334` exposed one Go 1.27 migration and a shared gossip-port collision in the production signal subprocess proof.
- `ffa920b9`: the signal subprocess now assigns a concrete isolated gossip address and the smoke parser uses `errors.AsType`; the combined production CLI race suite passes.
- `9992baa4`, `5bbe658d`, and `4f8efbfe`: the Helm release gate binds the digest-verified v2.12 OCI chart and exact candidate chart, loads both image digests into the named Kind cluster, proves real authenticated fixture continuity after three same-PVC pod recreations with fresh port-forwards, and restores/re-upgrades the quiescent legacy snapshot.
- `1c126f9c` through `6e6f4ee2`: `hk cache status/prune` now owns only dangling, generated HitKeep Compose cache volumes, re-inventories before apply, and excludes current-project names, data/archive roles, foreign, anonymous, and malformed volumes; non-Docker tests use a Docker-free path. About 14.7 GB of obsolete and current warm HitKeep caches were reclaimed; the current caches were removed unintentionally by the pre-isolation test but are rebuildable, and no application data, backup, or archive volume was touched. A disposable real-Docker volume then passed status, apply, and removal verification through `hk`.

- Stabilization preparation: release workflow predecessor selection now reads `supported_upgrade_floor` from `tests/fixtures/release-fixtures.json`; the immutable v2.12.0 floor remains defined only by that manifest and the architecture contract. QA-plan JSON/v2 read-path compatibility preserves `omitempty` bytes, `PlanID`, and legacy resume behavior; affected `go test -race ./internal/devtool` passed. This does not close the stabilization release or any external release gates.
- Darwin/Homebrew distribution at `4ffafb31`: native Darwin/arm64 builds with `CGO_ENABLED=1` passed for both production tag sets (`hashicorpmetrics,timetzdata` and `hashicorpmetrics,timetzdata,s3,billing,tenancy`) with version injection. The public `PascaleBeier/homebrew-hitkeep` tap builds stable tags from source without signing, passed strict audit, install, formula test, `2.13.11` → `2.13.12` upgrade, remote retap, and uninstall, and polls only GitHub's latest stable release so PR snapshots cannot update the formula. Signing and notarization are explicitly out of scope.

### Explicitly unproven gates

- Real cross-repository draft docs-attestation rehearsal and deployed token-scope proof.
- Real tagged-release execution of the authenticated disposable Kind/Helm lifecycle, including deployed GHCR token scope and private prepublication artifact availability.
- Stabilization release.
- Windows CGO artifacts and Scoop install, upgrade, and uninstall lifecycle proof. Local mingw-w64 11 linkage against the DuckDB Windows static libraries currently fails on unresolved C++/MSVCRT symbols; no signing path is planned.
- Remaining dependency-ordered `internal/` layout and filesystem migration waves.
- Final Ponytail and spf13 Go review after the remaining gates close.

## Completed discovery

- Production uses a Cobra factory root; `ExecuteRoot` is the only leading-`--config`/first-token compatibility bootstrap before Cobra selects a route, and no config path is a persistent Cobra flag.
- Recovery (`c05df878`) and the import/updater routes (`6347ccbc`) preserve their legacy stdlib parsers, names, confirmations, streams, help, and exit behavior while Cobra owns context and routing.
- `hk` already uses Cobra factories, centralized JSON/NDJSON envelopes, and a generated command catalog; preserve and normalize this implementation.
- Repeated canonical/deprecated config flags share one destination and resolve sequentially: the last occurrence wins in either spelling order.
- Current release binaries are CGO-enabled Linux amd64/arm64 self-hosted and cloud variants. Version injection targets `hitkeep/cmd.Version`.
- Release checksums are sorted SHA-256 entries in `SHA256SUMS`; GoReleaser parity must preserve artifact names and this format before cutover.
- Release Please remains the tag/changelog owner; image, Helm, and downstream jobs retain their existing dependency sequencing.
- Filesystem migration separates ordinary injectable reads/writes from native DuckDB/WAL/fsync/lock durability; fileflow never receives Afero-only paths.
- `internal/appurl` is the leading first move-only leaf candidate; `config`, `database`, and `devtool` move only after their behavioral boundaries stabilize.

## Most recently completed slice: 10

### Test-first work

1. Reused the Afero boundary already established by the Viper runtime loader instead of adding a repository-wide virtual filesystem wrapper.
2. Replaced the developer release-context `os.Rename` with `fileflow.Flow{NoCreateDirs: true}.Move`, preserving explicit quarantine ownership while supporting cross-filesystem workspace state.
3. Kept fixed trusted artifact names out of pathologize and retained native OS operations for database swaps, WAL recovery, fsync, locks, cache quarantine, atomic generated output, and log rotation.
4. Added one GoReleaser v2 configuration with separate self-hosted and cloud build IDs, preserving Linux architectures, CGO, build tags, version injection, raw binary names, and Release Please ownership.
2. Reused the existing `BuildReleaseBinaries` API and native-architecture runners; only its build implementation now delegates to the pinned GoReleaser release contract.
3. Added a PR/full `goreleaser-check` gate and delivery-path classification without creating a second release orchestrator.
4. Built both variants on Linux amd64 and arm64, generated the public configuration catalog, and passed the existing sorted SHA-256 release verifier over all six required artifacts.
5. Extended the existing image smoke rather than adding a second Docker harness: a unique named volume survives forced container removal and recreation, then the marker, real database, and healthcheck are verified.
2. Paired the recreation smoke with the existing opt-in default-tenant fault-boundary resume acceptance in the full profile.
3. Reused the existing catalog-derived example generator and configuration documentation validator instead of adding a second projection system.
2. Verified every published `HITKEEP_*` name is catalog-known, Compose-style defaults match runtime defaults unless explicitly justified, and the image data path remains beneath a declared volume.
3. Audited `hk` before editing and retained its existing factory-built Cobra root, avoiding a behavior-neutral rewrite.
2. Verified centralized JSON/NDJSON envelopes, invalid-output rejection, command-catalog generation, MCP manifest routing, and production/developer dependency separation.
3. Kept the legacy loader as a test oracle and added direct exported `config.Load` parity coverage.
2. Switched only the production assembly boundary to the local Viper instance with the existing non-empty environment semantics.
3. Kept explicit config-file loading separate from runtime selection, so 2.x still performs no implicit discovery.
4. Preserved global `os.Args`, recovery/import/update stdlib flag parsers, output, confirmations, signals, and exit behavior.
5. Added leading `--config PATH` and `--config=PATH` server selection at the Cobra boundary; config after another legacy flag remains legacy server input.
6. Returned malformed paths and YAML as normal Cobra startup errors without exposing configuration values.

### Decisions

- Extend the existing configuration catalog; do not create a parallel schema.
- Viper is instance-based; the legacy loader remains only as a parity oracle while runtime assembly uses Viper.
- The initial Cobra root is a routing boundary only; leaf stdlib parsers remain authoritative until migrated with exit/output parity tests.
- No `internal/` moves in behavioral slices.
- The generated example config must validate and remain behaviorally neutral versus no config file, excluding intentionally generated runtime values.
- GoReleaser must reproduce the existing artifact manifest before replacing the legacy builder.

### Verification

Record focused commands as stable test targets or `hk` gate IDs, not pasted successful logs.

| Date | Slice | Check | Result |
|---|---|---|---|
| 2026-08-26 | 0A | Workspace and live QA catalog inspected (`3ef33158d8113930`) | Passed; existing dev supervisor failed, finite QA remains available |
| 2026-08-26 | 0A | `go test -race ./internal/config` | Passed; canonical/deprecated flags proven last-occurrence-wins in both orders |
| 2026-08-26 | 0B | `go test -race ./internal/devtool -count=1` | Passed; Docker data path default and volume containment enforced |
| 2026-08-26 | 1 | `go test -race ./internal/config` | Passed; deterministic unique catalog v2 keys and checked-in neutral example YAML enforced |
| 2026-08-26 | 0A–1 | Changed QA `20260826T111826-92e002b4` | Passed: `go-format`, `go-fix`, `go-lint`, `go-vet`, `go-staticcheck`, `developer-mcp`, `developer-docs` |
| 2026-08-26 | 1 | Changed QA `20260826T113313-3af5ab8a` | Passed; root example config classified into backend and documentation areas; same seven gates passed |
| 2026-08-26 | 2 | `go test -race ./internal/config -count=1` | Passed; Viper shadow matches legacy defaults, env, flags, warnings, alias order, and deterministic normalization |
| 2026-08-26 | 2 | `go build ./internal/config/... && go test -race ./internal/config/...` | Passed; explicit Afero YAML, strict errors, no discovery, full catalog types, and four-layer precedence proven |
| 2026-08-26 | 2 | Changed QA `20260826T114130-28c942a4` | Passed: `go-format`, `go-fix`, `go-lint`, `go-vet`, `go-staticcheck`, `developer-mcp`, `developer-docs` |
| 2026-08-26 | 2 | Changed QA `20260826T115108-06b63ed6` | Passed; completed explicit-file/full-catalog shadow slice with the same seven gates |
| 2026-08-26 | 3 | `go test -race ./cmd ./cmd/hitkeep -count=1` | Passed; Cobra root preserves first-argument routing and existing command/server fallthrough |
| 2026-08-26 | 3 | Changed QA `20260826T115948-eb201dc1` | Passed: `go-format`, `go-fix`, `go-lint`, `go-vet`, `go-staticcheck`, `developer-mcp`, `developer-docs` |
| 2026-08-26 | 4 | `go test -race ./cmd ./cmd/hitkeep ./internal/config ./internal/database ./internal/devtool/devmcp ./internal/mcpserver ./internal/server/admin ./internal/server/aifetch ./internal/server/ingest` | Passed; production Viper cutover preserves typed runtime behavior across affected startup consumers |
| 2026-08-26 | 4 | Changed QA `20260826T121008-9708c7a2` | Passed: `go-format`, `go-fix`, `go-lint`, `go-vet`, `go-staticcheck`, `developer-mcp`, `developer-docs` |
| 2026-08-26 | 4 | `go test -race ./cmd ./internal/config` | Passed; leading explicit config selection, OS-file loading, missing-path errors, and legacy routing parity proven |
| 2026-08-26 | 4 | Changed QA `20260826T122352-73c142f6` | Passed: `go-format`, `go-fix`, `go-lint`, `go-vet`, `go-staticcheck`, `developer-mcp`, `developer-docs` |
| 2026-08-26 | 5 | `go test -race ./internal/devtool/cli ./cmd/hk`; structured command catalog and MCP manifest smoke | Passed; existing Cobra factories and machine contracts retained without source changes |
| 2026-08-26 | 6 | `go test -race ./internal/config ./internal/devtool`; `./hk docs check --output json` | Passed; generated example and Docker/Compose/chart/example/docs drift contracts are current |
| 2026-08-26 | 7 | `bash -n scripts/docker-smoke.sh`; focused `go test ./internal/devtool` | Passed; immutable previous-image, variant, omitted-new-env, and repeated candidate recreation contract preflight verified; real cross-release row fixture remains pending |
| 2026-08-26 | 7 | Full-profile selected gates `20260826T123333-1282e6d1` | Passed: `default-tenant-migration-acceptance`, `self-hosted-image` (real build, deletion/recreation, persistence) |
| 2026-08-26 | 7 | Changed QA `20260826T124011-6bb97907` | Passed: `go-format`, `go-fix`, `go-lint`, `go-vet`, `go-staticcheck`, `developer-mcp`, `developer-docs`; Docker smoke path is now classified as delivery |
| 2026-08-26 | 8 | `go test -race ./cmd/hk ./internal/devtool ./internal/devtool/cli` | Passed; release builder, CLI envelope, QA classification, and production/developer boundary coverage |
| 2026-08-26 | 8 | GoReleaser v2.18.0 `check` plus Linux amd64/arm64 snapshots | Passed; self-hosted/cloud tags, CGO, version linker value, and exact four binary names preserved |
| 2026-08-26 | 8 | `hk ci release-checksums` and `hk ci verify-release` | Passed over four binaries, `hitkeep-configuration.json`, and sorted `SHA256SUMS` |
| 2026-08-26 | 8 | Changed QA planning | Blocked before gate execution: persisted plan snapshot remains stale after deterministic replanning; focused race and release checks above passed and the guard was not bypassed |
| 2026-08-26 | 9 | `go test -race ./internal/devtool ./internal/devtool/cli` | Passed; public-image isolation, release artifacts, CLI routing, and developer boundary remain covered |
| 2026-08-26 | 9 | Gortex filesystem impact/contract review | One tested cross-filesystem candidate changed; database/native atomic operations explicitly excluded; no guard violations |
| 2026-08-26 | 10 | `internal/appurl` → `appurl` semantic move | Passed `go test -race ./appurl ./internal/server/auth`, all affected server/social/worker package tests, and `go test -race ./cmd/hk`; old import path absent |
| 2026-08-26 | 10 | `internal/exportfmt` → `exportfmt` guarded file move | Passed Gortex-prescribed race suite across seed, export formats, AI analytics, config, database, server takeout, and takeout; old import path absent |
| 2026-08-26 | 10 | `internal/hklog` → `hklog` guarded package move | Passed `go test -race ./hklog` plus affected command, seed, cluster, database, ingest, shared server, webhook dispatcher, and worker package tests; old import path absent |
| 2026-08-26 | 10 | `internal/analyticscatalog` → `analyticscatalog` guarded package move | Passed race tests across the catalog consumers in analytics tools, MCP, opportunities, and Ask AI; old import path absent |
| 2026-08-26 | 10 | `internal/jsonapi` → `jsonapi` guarded high-fanout package move | Full-module compile and focused race tests passed across commands, AI, database, MCP, every server package, social auth, SSO, takeout, webhook dispatch, and workers; all 160 old imports removed |
| 2026-08-26 | 4A | Production `config init` and `config validate` | Canonical-byte, exclusive no-overwrite, strict missing/malformed/unknown-key, and sensitive-value redaction tests passed; Gortex-prescribed startup/config race suite passed |
| 2026-08-26 | 4B | Config command and Viper raw-key hardening | Focused command/config race suite passed; exact fallback, replacement-safe write errors, and pre-normalization underscore/uppercase-key rejection are covered |
| 2026-08-26 | 4C | Explicit YAML grammar hardening | TDD red/green coverage and `go test -race ./config ./cmd` passed for duplicate, multi-document, alias, merge, null, mapping, and sequence rejection |
| 2026-08-26 | 10 | `internal/localization` → `localization` guarded pure-leaf move | Passed package/auth race tests plus OSS and `billing`-tag cloud race tests; old import path absent; the package has no filesystem operations and needs neither Afero nor fileflow |
| 2026-08-26 | 10 | `internal/config` → `config` guarded stabilized-boundary move | Full default race suite plus affected `billing`-tag race suite passed; all 73 import consumers use `hitkeep/config`; old import path absent; no shim or dependency added |
| 2026-08-26 | 10 | `internal/mcptest` → `mcptest` guarded test-helper move | Focused MCP contract tests and both affected package race suites passed; old import path absent; no filesystem operations, so Afero, fileflow, and pathologize do not apply |
| 2026-08-26 | 7 | Real v2.12.0 cross-release migration fixture and mandatory release gate | Pinned per-platform previous-image digests seed and verify public-API rows; the exact candidate digest survives migration and two graceful recreation cycles with ownership/mode and archive/recovery checks; final publication depends on the gate |
| 2026-08-26 | 7 | Quiescent full-volume rollback acceptance | Live smoke passed: stop v2.12 cleanly, snapshot the complete volume, migrate/verify the candidate, restore into a fresh volume, then prove the original pre-split user/site/hit identities, storage shape, ownership/modes, readiness, graceful shutdown, and another legacy restart |
| 2026-08-26 | 4 | Catalog-driven self-hosted legacy/Viper parity | Every active setting runs absent, empty, valid env, catalog flag-over-env, invalid typed warning/redaction, and deprecated-alias order through normal startup; focused race tests and independent review passed |
| 2026-08-27 | 4 | Cloud/build-variant legacy/Viper parity (`5a1c86a9`) | Default and `cloud`-tag race suites passed; cloud builds must exercise cloud-only descriptors and self-hosted builds prove their env/flags cannot activate hidden fields |
| 2026-08-26 | 6 | Persistence-sensitive data-path publication contract | Exact required files and defaults are catalog-owned; literal omission, drift, and per-path deletion tests cover Dockerfile, all root/example Compose manifests, Helm template plus structurally decoded values, and the canonical example; independent review passed |
| 2026-08-26 | 7 | Release publication hardening | Finalizer uses scoped `github.token`; tracker publication consumes the verified packed artifact and rejects retry integrity mismatches; mutable image tags and the public GitHub release remain last |
| 2026-08-26 | 7 | Gortex-selected race suite and independent release review | Passed; workflow contracts cover deterministic images, mandatory dependencies, graceful exit checks, exact tracker artifact identity, and retry ordering; live tagged workflow execution remains release-time proof |
| 2026-08-27 | 7 | Forced-process-kill split recovery (`193b58bc`) | Race-enabled real-filesystem child-process test force-killed and resumed every 19 durable fault boundaries; split marker, control site, zero control hits, and tenant hit integrity passed without production changes |
| 2026-08-27 | 7 | Release-finalizer interruption prerequisite (`e4b40a50`) | The reusable acceptance workflow is callable, runs only after a created release and successful build, and must succeed before finalization; focused/full devtool race tests, YAML parsing, and zizmor passed |
| 2026-08-27 | 3 | Healthcheck Cobra context/process boundary (`0de7546c`) | Healthcheck runtime returns a typed error instead of exiting; the executable alone preserves legacy exit 1 and stderr, Cobra context/cancellation reaches execution, and focused plus full affected command race suites passed |
| 2026-08-27 | 3/7 | Changed QA `20260826T230815-0f4a513b` | Passed: `go-format`, `go-fix`, `go-lint`, `go-vet`, `go-staticcheck`, `developer-mcp`, and `developer-docs` |
| 2026-08-27 | Plan | spf13 Go spec review | Approved after resolving leading-config grammar, canonical fileflow semantics, inventory ordering, immutable docs producer trust, release prerequisite ordering, command-package ownership, and `go install` scope |
| 2026-08-27 | 8A | Ponytail/Go release workflow consolidation | Five-file change removed 84 net lines while preserving parallel Docker, Compose, and Helm gates; focused/full developer tests, YAML parsing, zizmor, and the broader affected-package race contract passed |
| 2026-08-27 | 8A | Changed QA `20260826T215429-d0a8cfa3` | Passed: `go-format`, `go-fix`, `go-lint`, `go-vet`, `go-staticcheck`, `developer-mcp`, `developer-docs` |
| 2026-08-27 | 8A | Exact-SHA snapshot `33017615814` | Passed on `c09d2c6d`: deterministic dashboard, native Linux amd64/arm64 binaries and version checks, multi-architecture GoReleaser archives, multi-platform image, attestations, and release upload |
| 2026-08-27 | 3 | Recovery Cobra boundary (`c05df878`) | Recovery now retains its exact legacy command/parser/stream contract through Cobra-owned context and executable error handling |
| 2026-08-27 | 6 | Publication classification (`10045411`) | Catalog classification is recorded as the distribution-surface source of truth; private-doc attestation remains unproven |
| 2026-08-27 | 8 | Release digest manifest (`a3f71980`) | Release verification has a digest manifest; stabilization release and Darwin/Windows/Homebrew/Scoop evidence remain unproven |
| 2026-08-27 | 3 | Import/updater Cobra and typed Viper client configuration (`6347ccbc`) | Focused command race and subprocess parity tests passed; compatibility bootstrap is limited to leading `--config` and first-token server grammar |
| 2026-08-27 | 3 | Sole signal owner and production SIGINT/SIGTERM proof (`570bb9aa`) | Bounded real-main child-process test passed for both signals; focused race execution was bounded but longer than the interactive harness output window |
| 2026-08-27 | Delivery | Pushed SHA `0f4ff2d5` | Snapshot run `33022662630` passed; CI `33022594311` failed once only on `TestMCPStdioActionRunLifecycle` TempDir cleanup race and its rerun passed—no fix is claimed |
| 2026-08-27 | Developer artifacts | Run-local temporary/cache isolation (`ec747bf2`, `e82f7c46`) | Focused and full `internal/devtool` race tests passed; the G703 path traversal finding is removed; approximately 22 GB of orphaned global/temp build data was reclaimed without adding global-cache deletion to `hk` |
| 2026-08-27 | 3 | Opportunities smoke Cobra/Viper completion (`5f614a06`–`283bd589`) | Exact old/new binary parity covers help spellings, false help values, positional/help/unknown ordering, syntax/runtime/release exits and streams; catalog default/env/sensitivity projection and secret-safe tests pass under race; independent review approved |
| 2026-08-27 | 3 | Production subprocess harness repair (`8d228bed`–`14c5a06e`) | Real-main SIGINT/SIGTERM drains stdout before `Wait`, kills/drains/reaps on timeout, serializes concrete NSQ ports, and isolates helper argv; full `cmd/hitkeep` race package passed and independent review approved |
| 2026-08-27 | 6 | Fail-closed private-doc attestation (`bdb79b37`–`1f53e391`; docs `28bf7b0`–`c570df3`) | Finalizer depends on exact docs run/artifact provenance and versioned digest subject; prepublication cannot mutate public metadata; postpublication owns stable sync; combined Go race, docs 13/13 release tests, and independent cross-repo review passed; live draft rehearsal remains pending |
| 2026-08-27 | Delivery | Exact pushed head `1f53e391` | Local combined race suite passed; superseded by the exact `e4537eba` runs below |
| 2026-08-27 | Delivery | Exact pushed head `e4537eba` | Docs CI `33035679971`, docs Zizmor `33035679795`, HitKeep security `33035676326`, and Govulncheck `33035676364` passed; CI `33035676334` failed on one `go fix` migration and signal subprocess gossip-port collision, both corrected in `ffa920b9` |
| 2026-08-27 | 7 | Helm chart/image/PVC lifecycle (`9992baa4`, `5bbe658d`, `4f8efbfe`) | Combined race tests for `internal/devtool`, upgrade fixture, `cmd/hitkeep`, `cmd/opportunities-smoke`, and `cmd/hk` passed; shell syntax, workflow YAML, formatter, and Gortex contracts pass; independent review approved after exact previous-chart handoff and fresh port-forward corrections |
| 2026-08-27 | Developer artifacts | Safe dangling Compose cache ownership (`1c126f9c`–`6e6f4ee2`) | `internal/devtool` and `cmd/hk` race tests passed; realistic Docker labels and dangling re-inventory are covered; a disposable real volume passed status/apply/removal; about 14.7 GB was reclaimed, including current warm caches unintentionally removed before test isolation, while all application-data/backup/archive volumes remained intact; free space rose from 19 GB to 34 GB |
| 2026-08-29 | 4 stabilization | Real OS-backed `LoadArgs` precedence and complete QA | Passed; `config/viper_shadow_test.go` pins flag > non-empty environment > explicit YAML > catalog default, `go test -race ./config` passed, and source-bound complete QA `20260829T183048-74f03e32` passed all 15 selected gates, including `developer-docs`; the stabilization-release and legacy-oracle-removal gates remain open |

## Interim release simplification: 8A

- Reused the existing release workflow and smoke scripts; no new reusable workflow, helper service, or release orchestrator was added.
- Collapsed three duplicated v2.12 upgrade job definitions into one fail-independent matrix while retaining distinct surface names and parallel execution.
- Strengthened the repository contract so every surface script must receive both immutable fixture outputs before the aggregate gate can satisfy finalization.
- Kept the PR draft and proved the exact pushed commit through the existing snapshot workflow.

## Most recently completed slice: 3 healthcheck boundary

- Reused Cobra's existing command context and the production command factory; no new executor abstraction or persistent `--config` flag was added.
- Replaced healthcheck-specific deep `os.Exit` calls with one typed error whose process mapping remains solely in `cmd/hitkeep/main.go`.
- Preserved `DisableFlagParsing`, leading-only `--config` extraction, no-subcommand fallback, legacy output streams, and exact exit behavior with in-memory and subprocess tests.

## Next update

Phase 0C evidence gate is complete. Its records and sentinel do not approve a move: every blocked or unproven disposition remains in force, and Phases 9/10 remain frozen. Next work returns to the existing active migration and stabilization plan.
