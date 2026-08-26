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
| 0C | In progress | Inventory filesystem operations and dependency-ordered `internal/` move manifest | The durable filesystem/package manifest records remaining families, classifications, native exclusions, and four rejected move candidates; further Phase 9/10 waves remain frozen until every remaining direct child and operation has complete dependency/owner/build-tag/generated-file evidence |
| 1 | Completed | Make catalog authoritative and generate neutral `hitkeep.example.yaml` | Catalog `config_file_key` added; schema `hitkeep.config/v2`; deterministic example covers each self-hosted key once |
| 2 | Completed | Add instance-based Viper assembler in shadow parity | Viper 1.21.0 + Afero explicit YAML; full catalog types, strict keys, normalization, warnings, and precedence proven |
| 3 | In progress | Production Cobra root with legacy-compatible routing | Factory-built Cobra root owns top-level execution and exact first-argument routing; healthcheck now inherits Cobra context and returns a typed result to the executable boundary with exact subprocess parity; recovery and remaining actions still need context/error/stream ownership |
| 4 | In progress | Switch production assembly to Viper with complete legacy parity | `config.Load` uses instance-based Viper through an OS-backed Afero boundary; catalog-wide tests cover self-hosted and cloud env/flag/default/alias/invalid-value parity and prove cloud-only descriptors are inert in self-hosted builds; stabilization and final legacy-oracle contraction remain pending |
| 4A | Completed | Add production configuration-file UX | Cobra-owned `config init --output` writes the canonical catalog-derived example without overwrite; `config validate --config` uses the strict explicit Viper loader without starting services or exposing values |
| 4B | Completed | Harden production configuration commands and raw-key validation | Only exact `config init` and `config validate` use Cobra; all other `config` argv falls back unchanged; write/close failures retain their pathname; raw YAML keys must exactly match catalog kebab-case names before Viper normalization |
| 4C | Completed | Make the explicit YAML trust boundary deterministic | Valid flat scalar files remain compatible; duplicate keys, multiple documents, aliases, merge keys, nulls, mappings, and sequences are rejected before Viper normalization without exposing values |
| 5 | Completed | Normalize `hk` Cobra factories and preserve MCP/JSON contracts | No rewrite required: `hk` already has one factory root, centralized render/envelope handling, deterministic command catalog, and guarded production boundary |
| 6 | In progress | Project catalog into Docker, Compose, Helm, examples, and docs artifact | Catalog-owned data-path policy now rejects missing files, omitted declarations, and default drift across exact Docker, all root/example Compose, real Helm template/values, and canonical-example paths; remaining-setting classification, identical artifact distribution, and private-doc attestation remain pending |
| 7 | In progress | Enforce issue #288 upgrade, rollback, recreation, and migration interruption gates | Digest-pinned v2.12 migration, two candidate recreations, quiescent fresh-volume rollback, and forced process-kill/restart recovery across all 19 durable split boundaries are proven; release finalization now transitively requires the reusable interruption acceptance workflow; authenticated Helm lifecycle execution remains pending |
| 8 | In progress | GoReleaser release ownership while preserving future package-manager compatibility | Tagged Linux amd64/arm64 archives, checksums, raw cloud assets, version metadata, and exact-SHA snapshot publication are proven; a stabilization release plus Darwin/Windows CGO feasibility remain pending |
| 8A | In progress | Review and simplify the release process with Ponytail and spf13 Go guidance | Interim review removed three duplicated upgrade job definitions and exact-SHA snapshot `33017615814` passed on `c09d2c6d`; the requested whole-release review must be repeated after docs attestation, Helm lifecycle proof, final graph assembly, and stabilization evidence |
| 9 | In progress | Afero/fileflow/pathologize migration by operation risk | Runtime config uses injected Afero and one cross-filesystem release relocation uses fileflow; the complete operation inventory and remaining justified waves are pending, while database/WAL/fsync/lock operations stay native |
| 10 | In progress | Flatten `internal/` in dependency-order move-only slices | Pure foundations moved: `internal/appurl` → `appurl`, `internal/exportfmt` → `exportfmt`, `internal/hklog` → `hklog`, `internal/analyticscatalog` → `analyticscatalog`, `internal/jsonapi` → `jsonapi`, `internal/localization` → `localization`, `internal/mcptest` → `mcptest`, and stabilized `internal/config` → `config`; no compatibility shims because Go already prohibited external imports |
| 11 | Pending | Stabilize, full QA, CI, docs validation, completion audit | — |

## Completed discovery

- Production uses manual top-level dispatch; `cmd/hitkeep.Run` owns no-subcommand server startup and `config.Load` currently consumes `os.Args`.
- Recovery has five manual stdlib flag subcommands whose names, confirmation behavior, and output remain compatibility contracts.
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

Migrate recovery to Cobra-owned arguments, context, errors, and streams while preserving its five legacy commands and exact process behavior, then finish the remaining action closures. Follow with the versioned catalog/example digest manifest and exact-source private-docs attestation. Complete the operation/package inventories before any further filesystem or `internal/` wave. Run the Helm upgrade/recreation/rollback smoke in an authenticated disposable cluster before claiming Phase 7 complete. Enable Homebrew tap and Scoop bucket publishing only after real Darwin/Windows CGO artifacts pass install/upgrade/uninstall lifecycle smokes.
