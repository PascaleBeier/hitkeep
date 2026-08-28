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
- `mcpserver`, `opportunities`, `realtime`, `reporting`, `searchconsole`
- `security`, `server`, `socialauth`, `sso`, `takeout`
- `testutil`, `webhookdispatcher`, `webhooks`, `worker`

No remaining Go package-import cycle is currently proven. Reported Go cycles are call recursion or test-mock cycles, not import cycles; that does not make a package leaf-safe.

## Build and cloud configuration ownership

These surfaces form one build-time chain. The developer catalog owns supported build variants; every other surface projects or consumes that decision. None of these build defaults replaces production runtime configuration, whose settings remain owned by the runtime configuration catalog and loader.

| Surface | Role | Contract |
| --- | --- | --- |
| `internal/devtool/catalog.go::variants` | Canonical developer build owner | Defines variant IDs, build tags, developer-container environment, local image names, and publishability metadata. Its cloud values are developer/build defaults, not runtime settings. |
| `internal/devtool/app.go::App.ComposeEnvironment` | Workspace projection | Projects the selected variant plus workspace-scoped paths and ports into Compose variables; it does not define supported variants or production configuration precedence. |
| `internal/devtool/runs.go::App.executeBuild` | Build orchestrator | Resolves a catalog variant, enforces the production/developer dependency boundary, and invokes the selected binary or image build without redefining tags or defaults. |
| `Dockerfile` | Image-build consumer | Consumes explicit build arguments and produces the selected application image; it is not a configuration catalog or runtime parser. |
| `.goreleaser.yaml` | Release-build projection | Maps the canonical self-hosted and cloud build identities to release tags, CGO targets, archive contents, and names; it must remain aligned with the developer catalog. |
| `.github/workflows/pipeline.yml` | Delivery consumer | Supplies explicit version/ref inputs, restores verified assets, and invokes the canonical build projections. Publication and attestation policy lives here, but variant semantics do not. |

Disposition: **stay explicit / validate projections**. Do not consolidate these surfaces into production runtime configuration or move developer-only cloud defaults into the application binary.

## Rejected next move candidates

### `internal/assetstore`

Not a leaf. Its one implementation file has broad command, server QR-code, recovery, seed, and smoke consumers. It owns deletion and user/site-derived path construction. A future wave must review path containment and server wiring before moving it.

### `internal/blocking`

Not a leaf. It combines CIDR/IP filtering, embedded spam data, ordinary persisted feed files, HTTP refresh, background refresh, and ingest/AI-fetch/QR runtime consumers. Keep embedded data on `io/fs`; isolate only the ordinary persistence boundary in a separate Afero slice.

### `internal/analyticstools`

Not a leaf. It imports the analytics catalog, API, database, JSON API, and GoAI, with dependents in MCP, opportunities, and Ask AI server wiring. It has no direct filesystem work and belongs in a later coordinated domain wave.

### `internal/entitlements`

Not a leaf. It crosses configuration, database, worker, and cloud/OSS build-tag variants. All variants and consumers must move atomically in a later coordinated wave.

### `internal/devtool`

Owner: the developer-only `hk` workspace, QA, build, cache, artifact, toolchain, development-session, CLI, and MCP platform. Consumers: `cmd/hk`, with `internal/devtool/cli` and `internal/devtool/devmcp` depending inward; production-boundary tests exclude all three from `cmd/hitkeep`. Build tags/OS-CGO: no build tags, OS-specific files, or direct CGO were found. Filesystem: ordinary metadata reads, the `copyTree` fileflow candidate, and artifact/screenshot containment boundaries are distinct from native PID/lock/process/toolchain/cancellation state and embedded `tool-versions.json`. Tests: devsession, runs, cache, artifacts, CLI, MCP, and production-boundary tests. Coupling/cycle: workspace state and process orchestration are broad; no import cycle is proven. Disposition: **decomposition required / stay internal**; split core state, artifacts/cache, source/docs, CLI, and MCP before any move record.

### `internal/importables`

Owner: Plausible and Simple Analytics source parsing, validation, and import manifests. Consumers: database imported-analytics storage, server import handlers, and takeout/retention/tenant-manager tests. Build tags/OS-CGO: none found. Filesystem: provider `scanCSVFile` paths call `os.Open(source.Path)`, and `PlausibleProvider.scanZip` calls `zip.OpenReader(source.Path)`; classify CSV and ZIP source reads as ordinary injectable reads plus an untrusted staging/source-path containment boundary. Tests: Plausible and Simple Analytics parser/import tests plus fixture-path containment tests; focused archive-entry containment coverage remains a pre-move proof gap. Coupling/cycle: source validation, server staging, persistence, and cleanup are coupled; no import cycle is proven. Disposition: **stay internal / blocked** pending exact path-containment evidence.

### `internal/ingest`

Owner: `Batcher` and `Consumer` runtime ingestion coordination. Consumers: production startup in `cmd/hitkeep`. Build tags/OS-CGO: none found. Filesystem: no direct filesystem operations found. Tests: consumer and consumer-configuration tests. Coupling/cycle: `cmd/hitkeep` is the direct production consumer, and the exported batching/consumer surface coordinates queue and database behavior; no import cycle is proven. Disposition: **stay internal / blocked** pending review of that public surface and its direct consumer.

### `internal/ipmeta` and `internal/ipmeta/ipmetagen`

Owner: `internal/ipmeta` provides runtime IP metadata lookup; `ipmetagen` owns generated lookup assets. Consumers: server ingest and QR handlers plus shared country lookup for runtime, and `cmd/ipmeta-generate` for generation; production-dependency tests prohibit generator linkage into the application. Build tags/OS-CGO: none found. Filesystem: runtime lookup data is embedded `io/fs` and stays unchanged; generator `writePublicGeneratedFile` and embed-source writers are ordinary generated-file candidates. Tests: runtime block-asset/IP metadata tests, packed benchmarks, generator tests, and framed-asset tests. Coupling/cycle: runtime and generator boundaries are intentionally separate; no import cycle is proven. Disposition: **stay internal / blocked**; runtime and generator require separate move records.

### `internal/mailables`

Owner: product-specific mail construction for authentication, signup, invitations, reporting, and cloud billing lifecycle. Consumers: preview-email tooling, server auth/admin/cloud/user handlers, and report/cloud workers. Build tags/OS-CGO: cloud lifecycle and signup files use the `billing` build tag; the remainder is unconditional, with no OS/CGO files found. Filesystem: no direct filesystem operations found; it consumes `internal/mailer`. Tests: cloud lifecycle and report mailables plus consuming handler/worker tests. Coupling/cycle: product events, localization, billing variants, and delivery are coupled; no import cycle is proven. Disposition: **stay internal / blocked**; any move must preserve the billing variant and consuming handlers/workers.

### `internal/testutil`

- **Old path → new path:** `internal/testutil` → not applicable; no move is approved.
- **Owner and purpose:** test-only support for handler and package tests; it is not a production runtime service or reusable filesystem layer.
- **Subrecord — `passkeys.go`:** owner: authentication and user-security handler test support. Its complete direct importer files are `internal/server/auth/handlers_test.go` and `internal/server/user/security_handlers_test.go`; each is a `_test.go` file. Within the indexed closure, transitive impact is confined to those packages' test binaries; no production importer is evidenced. Its imports are `crypto/ecdsa`, `crypto/elliptic`, `crypto/rand`, `crypto/sha256`, `crypto/x509`, `encoding/base64`, `encoding/binary`, `fmt`, `math`, `github.com/go-webauthn/webauthn/protocol`, `github.com/go-webauthn/webauthn/protocol/webauthncbor`, `github.com/go-webauthn/webauthn/protocol/webauthncose`, `github.com/go-webauthn/webauthn/webauthn`, and `hitkeep/jsonapi` (imported as `json`). It uses crypto and in-memory credential encoding and has no filesystem operations, build tags, OS-specific behavior, CGO, embedded assets, generated inputs, or import cycle. Exact filesystem class: none. Focused tests: `go test ./internal/server/auth ./internal/server/user`. Rollback: delete the documentation record only; no compatibility shim. Conservative disposition: stay internal / blocked.
- **Subrecord — `testdb/testdb.go`:** owner: shared DuckDB fixture setup. Its complete 13 direct importer files are `internal/server/access/context_test.go`, `internal/server/admin/handlers_test.go`, `internal/server/admin/system_handlers_test.go`, `internal/server/auth/handlers_test.go`, `internal/server/server_test.go`, `internal/server/share/ai_activity_handlers_test.go`, `internal/server/share/handlers_test.go`, `internal/server/shared/team_capability_test.go`, `internal/server/user/security_handlers_test.go`, `internal/server/user/team_handlers_billing_test.go`, `internal/worker/cloud_lifecycle_billing_test.go`, `internal/worker/reports_test.go`, and `internal/worker/retention_test.go`. Every direct importer listed is a `_test.go` file. Within the indexed closure, transitive impact is confined to those packages' test binaries; no production importer is evidenced. `Tenant` has no current indexed consumers, so tenant-fixture coverage is not claimed. It imports `context`, `os`, `filepath`, `sync`, `testing`, and `internal/database`; it has no import cycle. `testdb` owns the temporary fixture lifecycle and clone creation: `os.MkdirTemp("", "hitkeep-testdb-")`, deferred `os.RemoveAll`, `filepath.Join(dir, "fixture.db")`, `os.ReadFile(path)` after store close, `filepath.Join(t.TempDir(), name+".db")`, `filepath.Dir`, `os.MkdirAll(..., 0o700)`, `os.WriteFile(..., 0o600)`, and `t.Cleanup` store close. `internal/database` owns `database.NewStore`, `Connect`, `Migrate`, and `MigrateTenant`, including DuckDB opening, migration, and WAL semantics; no abstraction is approved. It is not an Afero candidate. It has no build tags, OS-specific behavior, CGO, embedded assets, or generated inputs. Exact filesystem class: ordinary native temporary fixture lifecycle and byte cloning in `testdb`, with native DuckDB opening, migration, and WAL semantics in `internal/database`; test-only boundary. Focused tests: `go test ./internal/server/access ./internal/server/admin ./internal/server/auth ./internal/server/share ./internal/server/shared ./internal/server/user ./internal/server` and `go test -tags billing ./internal/worker`. Rollback: retain the existing native fixture path; no shim. Conservative disposition: stay internal / blocked.

### `internal/aianalytics`

- **Old path → new path:** `internal/aianalytics` → not applicable; no move is approved. The package mixes runtime classification/macros with refresh-time network and host-filesystem behavior, so destination selection requires an explicit whole-package versus decomposition decision.
- **Owner and purpose:** **Runtime subrecord:** `agents_data.go`, `curated.go`, `detector.go`, `sqlgen.go`, and generated embedded `default_ai_agents.json` own validated bot/referrer data, Go classification, and deterministic DuckDB macro generation. **Updater subrecord:** `updater.go`, `cmd/update_ai_agent_lists.go`, the refresh script, and scheduled workflow own release-time upstream fetching, validation, and regeneration of the embedded JSON.
- **Direct and transitive dependents:** the eight indexed direct importer files are `cmd/seed/traffic.go`, `cmd/update_ai_agent_lists.go`, `internal/database/ai_macros.go`, `internal/database/ai_macros_test.go`, `internal/database/store_ai_activity_test.go`, `internal/database/store_analytics_comparison_test.go`, `internal/server/aifetch/catalog_handlers.go`, and `internal/server/aifetch/handlers.go`. Bounded chains include `UpdateAIAgentLists` → `cmd.NewRootCommand` → `cmd/hitkeep/main.go::main`; `Store.ensureAIClassificationMacros` → control and tenant after-schema hooks → control/tenant migration; `handler.handleCreateAIFetch` → `aifetch.Register` → `Server.setupRoutes`; and `ClassifyBot` → seed AI visibility helpers → `cmd/seed/main.go::main`. The full repository closure is not claimed: Gortex impact was a truncated 127–128-symbol lower bound, so refresh the full transitive closure immediately before any move.
- **Imports and cycle result:** combined production imports are `bufio`, `context`, `embed`, `errors`, `fmt`, `io`, `log/slog`, `net`, `net/http`, `net/url`, `os`, `path/filepath`, `slices`, `strings`, `sync`, `time`, and `hitkeep/jsonapi`; `curated.go` has no imports. No Go import cycle involving `aianalytics` is evidenced; the available cycle scan returned truncated call cycles, so reconfirm absence before moving.
- **Build/generated/production boundary:** no package file has build tags, OS-specific suffixes, direct CGO, or generated Go source. Generated `default_ai_agents.json` is embedded through `//go:embed`; `curated.go` is hand-maintained. The refresh script uses canonical build tags, and the scheduled workflow regenerates and tests the asset. Runtime database/server/seed consumers import this package, and the refresh command is reachable from the production `cmd/hitkeep` root; no build boundary separates `updater.go` from runtime source.
- **Filesystem and network classification:** `embeddedAIAgentDataFS.ReadFile` is embedded `io/fs` and remains unchanged. `LoadAIAgentData` uses ordinary operator-selected host `os.ReadFile(path)`; `SaveAIAgentData` uses `filepath.Dir`, `os.MkdirAll(..., 0755)`, and `os.WriteFile(..., 0600)` with current truncation/replacement semantics, not atomic durable replacement. The CLI path is operator-supplied, not an untrusted segment beneath a fixed root. `fetchAgentFeed` makes fixed HTTPS GETs to three upstreams through an injectable client; a nil client gets the 30-second default timeout, while the command timeout is two minutes. Responses are 2xx-only with 10 MiB bounded bodies; successful body `Close` delegates to the underlying `resp.Body`, and non-2xx bodies close before returning an error. The bounded reader caps/truncates a 10 MiB+1 response at 10 MiB rather than rejecting it. A 1 MiB YAML scanner-line limit applies. Primary `ai.robots.txt` failure aborts; secondary Device Detector/crawler failures warn and degrade. Logs retain `error_kind` only, not raw external bodies. No package Go code performs temp-file creation, rename/move, deletion, directory walk, sync, locks, or executable/process work. `update-default-ai-agents.sh` and scheduled workflow commands are adjacent release-time delivery orchestration outside this package boundary. No Afero abstraction is approved.
- **Tests:** focused coverage: `agents_data_test.go`, `detector_test.go`, `sqlgen_test.go`, and `updater_test.go`; affected coverage includes database macro/activity/comparison, `cmd/seed`, update-command/subprocess, and `internal/server/aifetch` handler tests. Targets: `go test ./internal/aianalytics`; `go test ./cmd ./cmd/hitkeep ./cmd/seed`; and `go test ./internal/database ./internal/server/aifetch`. `TestFetchAgentFeedBoundsAndClosesSuccessfulBody` covers the 10 MiB+1 capped/truncated response and delegated body close; `TestFetchAgentFeedClosesNonSuccessBody` covers non-2xx closure. Gaps: save/load coverage does not assert `0755`/`0600` modes, overwrite/truncation, or partial-write failure; no focused test proves timeout/cancellation classification.
- **Rollback/no shim/disposition:** Rollback for this documentation decision is removal of the record; no source move exists. Any later move restores the old import path and atomically updates the eight in-repository importers. No compatibility shim is justified because this is an `internal` package with no external consumer contract. Decomposition is justified by embedded runtime data, updater/network behavior, and SQL macro projection, but no move plan is approved. **Disposition: decomposition required / stay internal / blocked** until runtime-versus-updater ownership, complete transitive closure, and focused filesystem/network proof gaps are resolved.

### `internal/mailer` and `internal/mailer/drivers`

Owner: mail manager, types/errors, localization/template rendering, and SMTP delivery. Consumers: `cmd/hitkeep`, preview-email tooling, `internal/mailables`, server setup/handlers, and workers. Build tags/OS-CGO: none found. Filesystem: locales and MJML/text templates are embedded `io/fs` and stay unchanged; SMTP is a network boundary. Tests: manager/errors, SMTP driver, mailables, and consuming handler tests. Coupling/cycle: runtime configuration, template localization, product mail construction, and network delivery are broad; no import cycle is proven. Disposition: **stay internal / blocked**; core manager and driver require separate move records.

### `internal/realtime`

Owner: `Broker`, `Publish`, and `Subscription` coordinate live delivery. Consumers: the site and shared realtime handlers in `internal/server`, plus the dashboard coordinator. Build tags: unconditional. Filesystem: none; its in-memory channels and HTTP delivery are native runtime semantics. Coupling/cycle: server-handler and channel-lifetime coupling make it non-leaf; no import cycle is proven. Tests: the site and shared realtime-handler tests. Disposition: **stay internal / blocked**; no move is approved until those consumers move together.

### `internal/reporting`

Owner: report scheduling and period helpers (`ValidateSchedule`, `NextOccurrence`, `PeriodBounds`, `CatchUpWindow`) and reporting tokens. Consumers: database report stores, worker reports/scheduler, user report handlers, and API reporting. Build tags: unconditional. Filesystem: none. Coupling/cycle: it crosses API, handlers, worker scheduling, and persistence; no import cycle is proven. Tests: schedule, database, handler, and worker reporting tests. Disposition: **stay internal / blocked** pending a coordinated reporting-domain move.

### `internal/searchconsole`

Owner: Search Console OAuth client, token/property operations, and error classification/diagnosis. Consumers: `cmd/hitkeep`, `internal/server`, shared server context, and report handlers. Build tags: unconditional. Filesystem: none. Coupling/cycle: OAuth, network, configuration, server wiring, and report-handler coupling make it non-leaf; no import cycle is proven. Tests: focused Search Console client/error tests and consuming server-handler tests. Disposition: **stay internal / blocked**; this security-sensitive integration must move atomically with its wiring.

### `internal/security`

Owner: TOTP, recovery-code, and passkey primitives. Consumers: user security/auth handlers and database security storage. Build tags: unconditional. Filesystem: none. Coupling/cycle: authentication and credential-persistence coupling is security-sensitive; no import cycle is proven. Tests: focused TOTP, recovery, passkey, handler, and storage tests. Disposition: **stay internal / blocked**; no move is approved outside a security-reviewed slice.

### `internal/socialauth`

- **Old → new path:** `internal/socialauth` → `internal/socialauth`; the approved current disposition is **stay internal / blocked**, not a package move.
- **Owner, providers, responsibilities:** owned by the server authentication boundary; implements Google OIDC, GitHub OAuth, and Microsoft OIDC provider configuration, authorization-code exchange, identity verification, and short-lived in-memory `FlowState` for the social-auth flow.
- **Import graph:** four direct production importers are `internal/server/admin/system_handlers.go`, `internal/server/auth/social_handlers.go`, `internal/server/server.go`, and `internal/server/shared/context.go`; the direct test importer is `internal/server/auth/social_handlers_test.go`. Bounded, intentionally truncated chains are `cmd/hitkeep` → `internal/server/server.go` → `internal/socialauth`, admin system handlers → `internal/server/admin/system_handlers.go` → `internal/socialauth`, auth social handlers → `internal/server/auth/social_handlers.go` → `internal/socialauth`, and server shared context → `internal/server/shared/context.go` → `internal/socialauth`; this is not a full transitive closure.
- **Imports and cycle state:** direct imports are `context`, `crypto/subtle`, `encoding/base64`, `errors`, `fmt`, `io`, `net/http`, `net/url`, `strconv`, `strings`, `time`, `github.com/coreos/go-oidc/v3/oidc`, `github.com/google/uuid`, `golang.org/x/oauth2`, `hitkeep/appurl`, `hitkeep/config`, `hitkeep/internal/security`, `hitkeep/internal/sso`, and `hitkeep/jsonapi`. There is no current import cycle; re-confirm after any coordinated move or split.
- **Build/runtime classification:** no build tags, OS/CGO split, generated source, embedded assets, or dev-only build constraint.
- **Filesystem/process/persistence:** no filesystem or process ownership and no persistence; `FlowState` is in memory only.
- **Security and network contract:** provider identity handling uses `crypto/subtle`, base64 state handling, PKCE, nonce, audience, issuer, tenant, signing-key, and verified-email checks. HTTP is injectable for tests. `Client.Complete` gives provider completion a 10s timeout; Google Begin discovery delegates to `sso.Client.Discover` without adding one. The 1 MiB body cap applies only to custom Microsoft JWKS and GitHub JSON reads, not general OIDC discovery or OAuth token exchange; capped response bodies are closed and externally visible errors are sanitized.
- **Tests and affected targets:** package tests are `TestProviderStatusesRequireCompleteConfiguration`, `TestMicrosoftConfigurationRejectsInvalidTenantSelector`, `TestGoogleOIDCUsesPKCEAndValidatesNonceAudienceIssuerAndVerifiedEmail`, `TestGoogleIssuerAllowsOnlyOfficialDocumentedVariants`, `TestGitHubUsesMinimalScopesPKCEAndPrimaryVerifiedEmail`, `TestMicrosoftIdentityUsesTenantAndObjectIDsAndTreatsEmailAsMetadata`, and `TestMicrosoftOIDCValidatesPKCENonceAudienceTenantAndSigningKeyIssuer`; coordinated-move checks must include affected `internal/server/auth`, `internal/server/admin`, and `internal/server/shared` targets.
- **Rollback and compatibility:** rollback is to retain this package at its current path; add no compatibility shim or forwarding package.
- **Disposition:** security-reviewed, coordinated move required; **stay internal / blocked** until the auth/admin/shared import boundary is intentionally redesigned. Do not claim a full transitive closure or approve a move/split from this record.

### `internal/takeout`

Owner: `TakeoutService` and its export query builders. Consumers: server takeout handlers. Build tags: unconditional. Filesystem: user-derived export paths and file creation require existing containment, permissions, and cleanup behavior. Coupling/cycle: database, authorization, export, and persistence semantics make it non-leaf; no import cycle is proven. Tests: focused takeout service and handler tests. Disposition: **stay internal / blocked**; do not move or abstract its filesystem behavior without a separately reviewed export-security slice.

### `internal/webhookdispatcher` and `internal/webhooks`

Owner: `internal/webhookdispatcher` owns the delivery `Dispatcher`, `Worker`, and `Sweeper`; `internal/webhooks` owns event emission through `Emitter`. Consumers: `cmd/hitkeep` leader-service startup, `internal/server.New`, and the webhook database store/migration. Build tags: unconditional. Filesystem: no ordinary filesystem boundary. Coupling/cycle: database, queue, network-delivery, and concurrent worker lifecycle coupling make it non-leaf; no import cycle is proven. Tests: dispatcher, emitter, worker, sweeper, and server webhook-handler tests. Disposition: **stay internal / blocked**; no move is approved outside a coordinated persistence and delivery slice.

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
