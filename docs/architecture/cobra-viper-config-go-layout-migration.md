# HitKeep 2.x Cobra, Viper, Configuration, Filesystem, and Go Layout Migration

Status: implementation in progress

Target: backward-compatible HitKeep 2.x releases

Contraction target: HitKeep 3.0 or later

Last updated: 2026-08-26

## 1. Outcome

Migrate HitKeep to:

- Cobra factories for the production `hitkeep` application and the `hk` developer CLI;
- one Viper-backed configuration assembly path that produces the existing typed configuration and preserves every 2.x environment-variable, flag, default, validation, normalization, logging, and build-variant behavior;
- one canonical configuration catalog that is mechanically projected into or checked against a generated default config file, Docker, Compose, Helm, examples, developer tooling, public documentation, and release verification;
- GoReleaser as the canonical binary archive/checksum release engine, initially preserving all current artifacts and deliberately enabling later Homebrew and Scoop distribution;
- injected spf13 filesystem implementations where filesystem substitution improves safety or testability, plus `fileflow` and `pathologize` for the narrower operations they actually own;
- flat, domain-named Go packages instead of the `internal/` directory tree, without coupling the structural move to runtime behavior changes.

This is not one large refactor. It is a sequence of independently releasable 2.x changes. Every phase must leave the repository buildable, deployable, downgradeable within the limits stated below, and behaviorally compatible with the previous phase.

### Implementation baseline

| Area | State on `feat/config_refactor` | Remaining proof |
|---|---|---|
| Catalog, example YAML, Viper assembly | Implemented | stabilization window and final legacy-oracle contraction only |
| Production Cobra routing and config commands | Implemented with 2.x compatibility routing | subprocess exit/stream grammar and final help/version contract |
| Distribution drift validation | Implemented for checked surfaces | private-docs attestation and semantic Compose/Helm upgrade gates |
| Issue #288 | Partial | immutable v2.12.0 pre-split fixture, recognizable API data, rollback snapshot, and release prerequisite |
| GoReleaser | Partial | tagged archive/checksum ownership and target-platform feasibility |
| Filesystem policy | Operation inventory and bounded migrations implemented | continue only where Afero/fileflow/pathologize semantics fit |
| Flat Go layout | In progress | dependency-ordered move-only waves and final active-path audit |

## 2. Why this work is necessary

HitKeep already has the beginnings of a canonical configuration model: `internal/config.Config` carries `env`, `flag`, `default`, `docdefault`, `desc`, `deprecated`, `sensitive`, and `cloud` metadata; `internal/config.Catalog` exposes a schema-versioned catalog; and catalog tests check coverage. Configuration loading is still assembled by the current loader, while startup and recovery routing remain distributed across the production and developer command surfaces.

That split allows a configuration change to be correct in Go and wrong in a publishing surface.

### Incident requirement: GitHub issue #288

Issue #288 is the regression case this migration must make mechanically detectable:

1. A Docker user upgraded automatically to 2.13.5.
2. The default-tenant split moved tenant analytics to `{data-path}/tenants/{tenant-id}/hitkeep.db`.
3. The image did not provide the new persistent `HITKEEP_DATA_PATH` container default on the mounted volume.
4. Helm and binary upgrade paths had been tested, but the plain Docker path had not.
5. Recreating the container—exactly what Watchtower-like tools do—lost access to tenant data written into the old container layer.
6. 2.13.6 added the Docker persistence default and interrupted-split recovery, but unarchived data could already be unrecoverable.

The lesson is broader than `HITKEEP_DATA_PATH`: any option that controls the location, durability, migration, encryption, authentication, or reachability of state must have an explicit distribution contract and an upgrade test. A minor or patch release must not require an operator to discover and add a new environment variable before an automated container recreation.

## 3. Non-negotiable 2.x compatibility contract

Before implementation, encode these rules as tests and keep them green through every phase.

### 3.1 Invocation compatibility

- Running `hitkeep` with no subcommand continues to start the application.
- Every existing production flag, deprecated flag, recovery invocation, healthcheck invocation, exit code, signal behavior, and stdout/stderr destination remains compatible.
- Every existing `hk` command path, flag, structured JSON envelope, workspace routing rule, MCP manifest, and finite-run/development-session behavior remains compatible.
- Cobra commands use fresh factories, `RunE`, command contexts, `SilenceUsage`, and `SilenceErrors`; business packages never import Cobra.
- Help may be reorganized, but existing accepted invocations cannot change meaning in 2.x.
- The 2.x bootstrap grammar recognizes an explicit config file only when `--config PATH` or `--config=PATH` is the leading argv element. It consumes that selector before handing the remaining argv to the legacy-compatible server parser. A later `--config`, `--`, positional arguments, duplicate selectors, and all non-leading flags retain their characterized legacy meaning.
- Only exact `config init` and `config validate` command paths claim the formerly server-routed `config` namespace. Bare `config`, unknown children, and legacy flags under `config` fall back unchanged. Subprocess tests, not only in-memory Cobra tests, own exit-code and stdout/stderr compatibility.

### 3.2 Configuration compatibility

- Existing `HITKEEP_*` names remain canonical in 2.x.
- Existing flag names and deprecated aliases remain accepted. When aliases for the same field are repeated, preserve the current sequential parsing rule: the last occurrence on the command line wins, regardless of whether it is canonical or deprecated. Characterization tests lock this down before Cobra/Viper binding.
- Existing defaults remain identical for the binary, self-hosted container, cloud build, Helm deployment, development environment, and healthcheck path. Surface-specific defaults are recorded explicitly instead of being assumed to be universal.
- Preserve the effective precedence of existing sources. The target precedence is: explicit programmatic override, changed flag, environment variable, explicitly selected config file, catalog default.
- An empty environment value keeps the current meaning of “unset”; it must not unexpectedly erase a default.
- Invalid environment or flag values retain current fallback/error/warning behavior, including secret-safe messages that never include raw sensitive values.
- Preserve derived defaults and normalization, including runtime-derived DuckDB settings, recovery paths, custom-tracking normalization, generated JWT secret behavior, and the healthcheck path that skips runtime-only normalization.
- Cloud-only fields remain build-tagged/guarded exactly as today. Self-hosted builds must not accidentally expose cloud behavior.
- Viper is confined to the configuration boundary. Runtime packages receive the typed `config.Config`, never `*viper.Viper` or stringly typed keys.
- Configuration loading must not write a config file or mutate operator state.

### 3.3 Default config file and loading compatibility

Config files are additive in 2.x. They must improve first-run UX without altering an existing deployment that only uses flags and environment variables.

- Generate one canonical, comment-rich `hitkeep.example.yaml` from the configuration catalog. Ship the identical artifact in source releases, binary archives, the container image, Helm chart material, and public docs.
- The file contains every self-hosted setting in stable catalog order, its current literal default, accepted values/units, restart behavior, and a concise description. Derived/runtime-generated defaults such as `GOMAXPROCS`, `hostname-timestamp`, `<data-path>/recovery`, or `randomly generated` appear only in comments while the YAML key remains unset/commented, so the file preserves derivation. Sensitive fields are present only as empty/commented placeholders; no credential is ever generated into the artifact.
- Config-file keys are exact lowercase kebab-case canonical keys in one flat namespace, for example `data-path`. The file is exactly one YAML document containing one top-level mapping whose values are scalars. Nested keys, duplicate keys, aliases, merge keys, nulls, mappings, sequences, alternate casing, underscore aliases, and deprecated config-file aliases are rejected before Viper decoding. Quoted empty strings remain explicit scalar values and follow the field’s existing decode/validation behavior.
- Add `hitkeep config init --output <path>` to write the embedded/generated canonical file without overwriting an existing path, and `hitkeep config validate --config <path>` to run the same strict decode, validation, and redaction path as startup. These commands perform no runtime or database mutation.
- Add the explicit leading bootstrap selector `--config PATH` / `--config=PATH`. It is intentionally not a Cobra root-persistent flag in 2.x because interspersed parsing would change legacy server argv behavior. `HITKEEP_CONFIG` is deferred; adding it later requires a separately characterized bootstrap-precedence contract and it must not enter the runtime-key namespace.
- Do not silently search the home directory or working directory in 2.x. A missing explicitly requested file is an error; no requested file means no file read and preserves legacy behavior. Packagers may install the example under a shared documentation path, but must not create or overwrite an operator-owned active config.
- YAML is the documented and generated format. Support another Viper format only after it has the same strict-key and parity coverage. Do not add remote providers or live reload.
- Older binaries preserve environment behavior because environment variables remain the 2.x deployment contract. Operators who adopt config-file-only settings must translate them back to environment variables before downgrading to a pre-file-support release.

### 3.4 Release and storage compatibility

- A 2.x minor or patch release may add an optional key with a safe default. It may not remove or rename a key, change an existing default, change precedence, or require a new value for safe startup.
- Any storage layout change must preflight durability, permissions, free space, and rollback/recovery conditions before the first destructive write.
- The same image must survive stop, container deletion, recreation with the same mounted volume, restart, and repeated restart without losing or relocating state.
- Automatic update tooling is a supported upgrade shape for non-breaking 2.x releases. Release validation must recreate the container rather than merely restart the same container.
- A migration that cannot satisfy these conditions is a major-version change or must be made opt-in until the next major.
- The supported upgrade floor is explicit in a checked release-fixture manifest. Every release gate tests the oldest supported floor plus the immediate predecessor of each storage-layout transition; issue #288 specifically requires immutable v2.12.0 → candidate coverage because v2.12 was the last pre-split layout.
- Directly running a previous binary on a completed newer layout is unsupported unless a dedicated compatibility test proves it. The required 2.x rollback contract is a quiescent full-volume snapshot before migration, restore into a fresh volume, then previous-image data/readiness verification across restart.

## 4. Target architecture

### 4.1 Ownership

- `config`: owns the typed configuration, canonical descriptors, Viper assembly, decoding, validation, normalization, redaction, and catalog export.
- Production Cobra package: owns only the `hitkeep` command tree, compatibility aliases, argument/flag binding, output routing, and calls into application/recovery functions.
- Developer Cobra package: owns only the `hk` command tree and calls concrete developer services. Developer code remains forbidden from production dependency paths.
- Domain packages: own runtime behavior and accept typed dependencies.
- Developer generator: consumes the configuration catalog and renders/checks repository publishing surfaces. It never becomes a second source of defaults.
- GoReleaser: owns tagged self-hosted binary archives, archive naming, embedded version metadata, and checksums after parity is proven. It does not own container builds, cloud publication, changelog generation, or deployment unless a later separately reviewed phase assigns that responsibility.
- Adjacent `hitkeep-docs`: consumes a versioned, non-secret catalog artifact and adds reader-facing explanation; it does not retype the option inventory.

### 4.2 Command layout

Use one fresh root factory and one fresh Viper instance per binary execution/test. Keep the two binaries separate so developer-only dependencies cannot leak into production.

Proposed end state:

```text
cmd/
  hitkeep/main.go       # signal context, NewRootCommand, print one error, exit
  hk/main.go            # signal context, NewRootCommand, print one error, exit
hitkeepcmd/             # production Cobra routing only
hkcmd/                  # developer Cobra routing only
config/                 # typed config, Viper assembly, catalog, validation
server/ database/ ...   # domain behavior
```

The exact package names should be confirmed against import-cycle analysis before the first move. Do not create generic `cli`, `common`, `utils`, `service`, or `repository` packages. Command packages may define small function fields for test seams; do not introduce single-implementation interfaces.

The production root must preserve no-subcommand startup. Every existing production configuration flag and deprecated alias remains a root persistent flag so `hitkeep --flag`, healthcheck, and recovery compatibility invocations retain the same reach and sequential last-occurrence-wins behavior; only truly command-specific operational flags are local. Recovery and healthcheck become explicit Cobra commands or compatibility flags only where their current invocation contract requires it. Cobra built-ins should own help, version, completion, argument validation, flag relationships, and error propagation when doing so preserves output compatibility.

### 4.3 Configuration assembly

Build a fresh instance for each load:

```text
Config descriptors
    -> register every known key/default/env/flag with fresh Viper
    -> read explicit file, if any
    -> bind environment and changed flags
    -> decode into typed Config
    -> apply legacy-compatible aliases
    -> validate and normalize
    -> return immutable-by-convention typed Config + non-secret source diagnostics
```

Register every key explicitly. Do not rely on `AutomaticEnv` alone because env-only values can be absent from `Unmarshal`. Prefer `BindEnv` from catalog descriptors; use the existing exact `HITKEEP_*` names rather than deriving names with a replacer. Bind only the active command’s flags so identically named local flags cannot overwrite each other.

Use a decoder configuration that preserves existing parsing for booleans, integers, floats, durations represented as existing units, comma-separated values, and empty values. Strictly compare the old and new result rather than assuming Viper/mapstructure coercion is compatible.

### 4.4 Canonical configuration catalog

Evolve the existing catalog instead of inventing a parallel schema. Each descriptor needs, as applicable:

- stable canonical key and typed field;
- exact environment name;
- current and deprecated flags;
- type and accepted values;
- binary default and any explicit surface override;
- derived-default description and derivation owner;
- description;
- sensitivity/redaction class;
- self-hosted/cloud availability;
- restart requirement;
- persistence/security/migration classification;
- publication policy: Docker, Compose, Helm, examples, docs, developer catalog;
- deprecation metadata with earliest removal major.

Keep secrets out of generated examples. A sensitive descriptor may render a placeholder name, never a live or default credential.

The catalog has two consumers:

1. Runtime registration: Viper defaults, environment binding, flags, decoding metadata.
2. Distribution projection: deterministic generation or validation of publishing surfaces.

One source does not mean every surface contains every setting. The descriptor’s publication policy decides what is required, optional, omitted, or explained for each surface. Surface-specific policy is data in the same catalog package, not an independent YAML inventory.

### 4.5 Filesystem ownership

Use the narrowest appropriate implementation:

- Use `afero.Fs` injection for components whose ordinary file reads/writes/directories need isolation and in-memory tests. Construct `afero.NewOsFs()` at binary composition roots and pass it down. Use `afero.NewMemMapFs()` in focused tests.
- `fileflow` is always an OS-backed boundary; it does not operate on virtual Afero paths. A component that reaches this boundary either receives a small injected move/copy function for unit tests or uses a real temporary filesystem integration test. Never pass a path that exists only in `afero.NewMemMapFs()` to fileflow.
- Use `fileflow.Move`, `Copy`, or `Rename` for user-visible moves/copies/renames where cross-filesystem behavior, atomic destination writes, or collision safety matters. Always use the returned final path and handle partial-success errors such as destination success plus source cleanup failure.
- Canonical state paths such as `hitkeep.db`, tenant databases, markers, and recovery bundles must never accept fileflow’s suffix fallback. Retain the proven OS commit primitive or use a configured `fileflow.Flow` whose conflict callback returns an error, whose `NoCreateDirs` is true when a mount must already exist, and whose caller syncs the destination file and parent directory before advancing durable state.
- Use `pathologize.Join` when untrusted path segments are placed under a trusted configured root. Configured roots must be operator-controlled and not attacker-writable. Because lexical containment does not prevent symlink redirection, security-sensitive writes must resolve/check the real parent beneath the trusted root at the OS boundary or reject writable/symlinked intermediates. Do not use `CleanPath` as a traversal defense.
- Keep `os`/`io/fs` where the operating-system primitive is the behavior: process environment, signals, executable lookup, file descriptors, locks, durability `Sync`, ownership/permissions not modeled by an abstraction, memory mapping, and DuckDB paths passed to the native database engine.
- Do not wrap Afero in a repository-wide custom filesystem interface. Inject the concrete small dependency a domain needs.
- Do not replace database durability operations with an in-memory abstraction. Split pure path/state planning from the OS-backed commit step and keep real-filesystem fault tests for migrations, backups, compaction, recovery, and atomic swaps.

This distinction prevents “move all filesystem access” from weakening the exact durability guarantees implicated by issue #288.

## 5. Delivery sequence

Each phase is one or more small pull requests. Never combine a package move with a configuration semantic change or a storage migration.

### Phase 0 — Freeze and inventory the current contracts

1. Export the current schema-versioned configuration catalog as a checked test fixture with secrets redacted.
2. Build a table-driven legacy loader characterization suite covering every descriptor and every source: absent, valid env, empty env, invalid env, current flag, deprecated flag, both flags, and build variant.
3. Capture golden command tests for both binaries: command paths, help/version shapes where public, exit codes, errors, stdout/stderr, cancellation, and structured `hk` JSON.
4. Inventory every publishing occurrence of `HITKEEP_*`, every flag, and every default across Dockerfile, all Compose files, examples, Helm templates/values/schema, workflows, developer configuration/catalog output, contributor skills, and adjacent documentation.
5. Classify every option by durability/security/migration impact. Mark `HITKEEP_DATA_PATH`, database, archive, recovery, backup, import staging, and any future state path as persistence-sensitive.
6. Produce a package-move manifest for every direct child of `internal/`: current import path, proposed domain path, owners, direct dependents, build tags, generated files, cgo/OS constraints, and cycle risk.
7. Produce a filesystem-operation inventory by operation rather than by import: read, write, temp file, create directory, walk, move, copy, rename, delete, sync, lock, executable/process, database-native path, and untrusted path construction.
8. Record the baseline QA plan and artifact hashes. No runtime behavior changes in this phase.

Exit criteria:

- every catalog descriptor is characterized;
- every current surface occurrence maps to a descriptor or an explicit non-runtime exception;
- the issue #288 upgrade fixture fails when the Docker data-path default is removed;
- the package and filesystem manifests have no unknown owners.

### Phase 1 — Make the catalog authoritative before adopting Viper

1. Extend the existing `Config` metadata/catalog representation with stable keys, type information, surface policy, and persistence classification.
2. Keep the current loader using the descriptors so this phase proves the descriptors are complete without changing parsing.
3. Add catalog validation for duplicate env names, duplicate flags, missing defaults, invalid sensitivity metadata, undocumented aliases, cloud/self-hosted leakage, and publication-policy contradictions.
4. Add deterministic JSON output for downstream tooling with a schema version. Sort all output; exclude runtime-generated secrets and machine-specific values.
5. Render the canonical `hitkeep.example.yaml` from the same descriptors, including comments, literal defaults, units, accepted values, commented/unset derived-default explanations, restart notes, and safe secret placeholders. Require it to pass `hitkeep config validate` and produce the same effective configuration as no config file, excluding intentionally runtime-generated values. Check in/embed that generated artifact so `config init`, archives, images, Helm, and docs all ship identical bytes and hashes.
6. Add a single developer-side render/check service. Write mode is an explicit source rewrite through `hk`; check mode is non-mutating and used by QA/CI. The developer MCP may expose only the read-only catalog/check result, never source rewriting.
7. Fail `developer-docs` when the JSON catalog, default YAML, or any checked-in projection differs from the catalog.

Rollback: revert descriptor additions; no operator state or data has changed.

### Phase 2 — Add Viper in shadow mode

1. Add Viper and its existing pflag/Cobra-compatible dependencies to the production Go module using the repository’s dependency policy.
2. Implement a fresh, instance-based Viper assembler inside `config`; do not use package globals.
3. Register every catalog key with `SetDefault` and exact `BindEnv`; bind only parsed active flags.
4. Add explicit config-file parsing and strict unknown-key validation, but do not expose it publicly yet.
5. In tests, run the legacy and Viper assemblers over the complete Phase 0 matrix and compare the typed result, normalized result, warnings, error class, and redacted log output.
6. Add fuzz coverage for source combinations and malformed scalar values where coercion differs.
7. Resolve mismatches by configuring decode hooks or keeping a small compatibility decoder at the boundary. Do not change accepted values to match Viper defaults.

Exit criteria: zero unexplained parity differences in self-hosted, cloud, healthcheck, recovery, and developer catalog paths.

Rollback: remove the unused Viper assembler. Runtime remains on the legacy loader.

### Phase 3 — Introduce the production Cobra root without changing configuration

1. Create the production root factory with a fresh command tree per call.
2. Make no-subcommand execution call the existing application runner.
3. Route existing healthcheck and recovery paths through Cobra while preserving aliases and argument validation.
4. Move signal context creation to the minimal `cmd/hitkeep/main.go`; pass `cmd.Context()` through startup/recovery I/O.
5. Print an error once at the main boundary. Preserve secret redaction and existing exit codes.
6. Execute all command tests in memory through the root factory using `SetArgs`, `SetOut`, and `SetErr`.
7. Keep application, database, server, and recovery logic Cobra-free.

Rollback: restore the former entrypoint/router. Config and data are unchanged.

### Phase 4 — Switch production configuration assembly to Viper

1. Expose the explicit `--config` path on the production root.
2. Bind root persistent flags and the active command’s local flags after Cobra parsing.
3. Switch the production root from the legacy loader to the parity-proven Viper assembler.
4. Retain the legacy loader only as a test oracle for one stabilization window; do not add a user-facing “config engine” toggle.
5. Add non-secret startup diagnostics that can state which source class won for a key without logging its raw value. Keep this bounded and debug-level except for invalid configuration.
6. Run upgrade/restart tests using only legacy env/flags, then mixed env plus explicit file, then explicit file alone.

Exit criteria: legacy deployments produce identical typed config and observable startup behavior; explicit config files work; repeated starts are idempotent.

Rollback: deploy the prior 2.x binary with the same environment and flags. Config-file-only adopters must restore equivalent env vars before downgrade. No config state is written.

### Phase 5 — Normalize the `hk` developer CLI on Cobra

1. Inventory which `hk` paths already use Cobra and preserve reusable factories rather than replacing working code.
2. Ensure the root and every subcommand are factory-built, use `RunE`, validate arguments through Cobra, write through command streams, and receive context.
3. Keep the developer application concrete and separate from routing. Do not move workflow facts from `hk` into skills or docs.
4. Preserve the MCP manifest, server catalog resolution, workspace ID rules, JSON schema envelope, async start/status contracts, and command catalog.
5. Add boundary tests proving the production binary cannot import developer packages before and after later package moves.

Rollback: command factories can be reverted without changing workspace state formats or application configuration.

### Phase 6 — Project configuration into every publishing surface

1. Generate or validate a marked configuration block in the Dockerfile, including the container-specific persistent data root.
2. Generate the environment/config sections of `compose.yaml`, `compose.cluster.yaml`, `compose.dev.yaml`, and every example Compose file. Preserve hand-written service topology outside marked blocks.
3. Generate or validate Helm values, templates, schema descriptions, persistence mounts, and upgrade-safe defaults from the same descriptors. Preserve chart-specific templating as a thin projection, not another inventory.
4. Emit a versioned, redacted reference artifact for `hitkeep-docs`, including catalog schema, content hash, and application release identity. The private docs repository verifies that exact artifact and reports a required commit status/attestation back to release preparation. A missing or stale attestation leaves the release draft pending; `continue-on-error` documentation dispatch is not synchronization.
5. Project the canonical `hitkeep.example.yaml` unchanged and generate any `.env` example from the same descriptors. Images and packages place the example in a documented read-only/share path; startup and upgrades never copy it over an operator-owned active config. Never emit secrets.
6. Make release preparation fail if a persistence-sensitive option lacks all required surface projections or an upgrade test.
7. Update contributor skills to point at the catalog/generator workflow, not copied option lists.

Do not generate whole Docker, Compose, Helm, or prose files when only a bounded block is configuration-owned. Small marked projections minimize churn while making drift testable.

### Phase 7 — Permanently encode the issue #288 regression

Add a release-grade container upgrade test with this exact shape:

1. Read the checked release-fixture manifest and test every required predecessor: the oldest supported floor plus the immediate predecessor of each storage-layout transition. For issue #288 the required floor is v2.12.0, pinned by immutable per-platform image digest and expected version. Reject mutable, same-version, newer, wrong-platform, or post-transition substitutes. Start it with documented plain-Docker defaults and one named/bind-mounted persistent data volume.
2. Seed recognizable control-plane and default-tenant analytics data.
3. Stop and delete the container.
4. Start the candidate image with the same volume, deliberately omitting `HITKEEP_DATA_PATH` and other newly introduced variables.
5. Allow the default-tenant split or any current mandatory migration to complete.
6. Stop and delete the candidate container again, then recreate it with the same volume.
7. Verify control-plane rows, tenant analytics, archive/recovery state, marker/file consistency, ownership, and application readiness.
8. Repeat startup to prove idempotency.
9. Exercise an injected interruption at every durable migration boundary and verify either safe resume or safe rollback with an actionable, non-secret error.
10. Parse/inspect the candidate image and assert every persistence-sensitive container default resolves under the declared mounted persistence root.
11. Before candidate migration, take a quiescent complete volume snapshot. After candidate verification, restore it into a fresh volume, start the pinned previous image with its original arguments, and verify the same control-plane and analytics identities across another restart. Never test rollback by running the previous binary directly on the migrated volume unless that downgrade path is separately proven.

Run the same semantic check for Compose and Helm. Static rendered-value checks are necessary but do not replace old deployment → candidate upgrade → workload deletion/recreation → API data verification. A Watchtower-specific dependency is unnecessary: container deletion and recreation with unchanged image arguments and volume is the behavior that matters.

Release policy:

- a missing persistence projection blocks the release;
- an unsafe new mandatory variable blocks a 2.x release;
- migration instructions never substitute for an automated safe default in a minor/patch release;
- release notes describe backup and rollback, but safety must not depend on the operator reading them.

### Phase 8 — Migrate tagged binary releases to GoReleaser

GoReleaser replaces hand-rolled binary/archive/checksum assembly only after it can reproduce the current release contract. Release Please may continue to own version/changelog/tag orchestration; GoReleaser consumes the immutable tag and must not create a second version source.

1. Record the current release artifact manifest from `BuildReleaseBinaries`, `GenerateReleaseChecksums`, release workflows, and published releases: binary/archive names, formats, OS/architecture matrix, build tags, cgo requirements, permissions, version output, checksum filename/algorithm, bundled files, and GitHub release attachment behavior.
2. Add one `.goreleaser.yaml` using the pinned GoReleaser configuration schema for the publishable self-hosted binary, and independently pin the GoReleaser executable/action version under the repository’s dependency policy. The config schema cannot pin the tool. Do not copy the common `CGO_ENABLED=0` example blindly: DuckDB/native dependencies and existing build tags determine the supported target matrix. Keep cloud-tagged artifacts local-only unless separately authorized for publication.
3. Feed GoReleaser’s version, commit, and build date into the same Cobra version surface used by ordinary builds. Prefer `debug.ReadBuildInfo` as the fallback for `go install`; use ldflags only for release metadata the runtime cannot obtain reliably.
4. Use `-trimpath`. Strip symbols only if the existing debugging/core-dump policy allows it; artifact size is not worth losing required production diagnostics.
5. Bundle `LICENSE`, the minimal install/readme material, and the generated `hitkeep.example.yaml` in every binary archive. The config artifact hash must match the catalog-generated source artifact.
6. Run GoReleaser snapshot builds in CI with publication disabled and compare the artifact manifest against the legacy builder for every supported target. Keep the legacy builder authoritative until names, formats, executable modes, version output, and checksum coverage match exactly.
7. Cut over the release workflow so Release Please creates the immutable version/tag and a draft release. Build binaries and a SHA-addressed candidate image, then run artifact, issue #288, Compose, and Helm verification against exact immutable inputs. Upload exact-version artifacts first. One final job that transitively depends on every required gate publishes the GitHub release and promotes mutable image/npm pointers. A failure leaves an unpublished draft and never advances `latest`, major/minor image tags, Helm, or npm dist-tags. Remove the legacy binary/checksum assembly only after GoReleaser owns tagged archives/checksums and one stabilization release succeeds.
8. Verify `go install` at the existing `cmd/hitkeep` package path if it is currently supported; otherwise document it as unsupported rather than accidentally promising it during this migration. Never ship `replace` directives or depend on `go.work`.
9. Treat tags and published module versions as immutable. A failed pre-publication run falls back to the legacy builder; a bad published release rolls forward with a fixed patch release and, when module consumers are affected, a `retract` directive—never delete or retag it.

Future package-manager enablement:

- Keep archive names, checksums, version output, config-file location guidance, and install layout stable, but do not call this package-manager-ready until native snapshot builds and install/upgrade/uninstall smokes pass for the intended Darwin/Linux Homebrew and Windows Scoop targets. DuckDB CGO/platform feasibility is an explicit prerequisite.
- After the core GoReleaser pipeline is stable, use separate reviewed changes to add a Homebrew tap and Scoop bucket. Those changes require explicit authorization for repository creation, credentials/tokens, and publication.
- Package-manager formulas/manifests must reference immutable release artifacts and checksums, install the example config without overwriting operator state, expose the same service/config invocation contract, and pass install/upgrade/uninstall smoke tests.
- Add deb/rpm, signing, provenance, or further package managers only when there is an actual distribution requirement; they are not prerequisites for this migration.

Rollback: before publication, switch the workflow back to the retained legacy builder. After publication, preserve the tag and artifacts and roll forward; never mutate an immutable release.

### Phase 9 — Migrate filesystem access by risk and operation

Perform this after configuration assembly is stable so path values and source diagnostics are trustworthy.

Wave A: generators, fixtures, bounded metadata, and ordinary reads/writes.

- Inject `afero.Fs` from composition roots.
- Convert tests to in-memory filesystems where OS semantics are irrelevant.
- Keep golden integration tests on a real temporary directory.

Wave B: imports, exports, archives, downloads, and user-derived paths.

- Use `pathologize.Join` for untrusted segments under trusted configured roots.
- Use `fileflow` for moves/copies/renames that must cross volumes or avoid clobbering.
- Assert and persist the returned final path.
- Preserve permissions, size limits, cleanup behavior, and partial-failure observability.

Wave C: backups, recovery, compaction, default-tenant split, and database swaps.

- First extract pure plans/state machines from OS execution without changing behavior.
- Keep OS-backed durability primitives where Afero cannot express fsync, atomic replacement, locks, or DuckDB/native file behavior.
- Adopt `fileflow` only after fault-injection tests prove its conflict behavior matches the migration invariant; automatic suffixing is wrong for canonical database filenames unless the caller treats a conflict as failure.
- Require real-filesystem, cross-filesystem where available, crash-boundary, permission, low-space, and retry tests.

Wave D: delete duplicated hand-rolled helpers only after every caller has moved and parity tests pass.

Rollback is per wave. Never change the on-disk layout merely to adopt an abstraction.

### Phase 10 — Flatten `internal/` in dependency-order waves

Moving out of `internal/` changes compile-time visibility, not just paths. This plan explicitly accepts that root packages become importable while declaring that HitKeep remains an application and does not offer a supported Go library API. Add package documentation and the public contribution policy stating that these imports carry no compatibility promise; do not market them or publish library examples. There is no replacement in Go for compiler-enforced `internal` visibility: if non-importability is required, the goal to remove `internal/` must be reconsidered rather than simulated with a nested module or wrapper maze. Enforce only the boundary HitKeep itself controls—production code must exclude `hkcmd` and developer packages—using a transitive import-graph test. Adding importable packages is additive in 2.x; any future decision to promise a public library API requires a separate API baseline and release policy.

For each wave:

1. Recompute the import graph and select leaf domains with no cycle risk.
2. Move one coherent domain with semantic refactoring so imports update atomically and Git history remains readable.
3. Keep package names stable unless the old name is layer-oriented or ambiguous.
4. Do not add forwarding packages unless an actual supported consumer needs a compatibility import. External code cannot legally import the current `internal/` paths, so most shims would be useless.
5. Run focused tests, import-cycle checks, build-tag variants, and production-boundary checks.
6. Release the wave before starting the next high-risk domain.

Recommended order:

1. leaf utilities with clear domain names and no developer/runtime seam;
2. logging, configuration, assets, parsing, and other shared foundations;
3. isolated integrations and analytics domains;
4. workers, ingest, AI/opportunities, MCP, and other orchestrated domains;
5. database and server last because they have the largest dependency and test surface;
6. developer tooling in its own wave, retaining a hard production import prohibition;
7. delete the empty `internal/` directory only after graph and filesystem searches show no imports, paths, generators, build scripts, skills, or docs referring to it.

Do not mirror the old directory tree at the repository root. Consolidate only when two packages already form one domain and always change structure separately from behavior.

Rollback: revert one move wave. There is no data migration and no public 2.x import-path promise to preserve for formerly internal packages.

### Phase 11 — Stabilize, document, and release

1. Remove the legacy loader test oracle only after at least one stabilization release and complete parity evidence. Removing old env names, flags, aliases, defaults, or compatibility branches is not authorized in 2.x.
2. Publish the catalog schema and generated artifact with the release.
3. Update adjacent `hitkeep-docs` configuration reference, binary guide, Docker guide, Compose examples, Helm guide, automated-update guidance, backup/restore guide, and upgrade policy.
4. Explain explicit config-file precedence and downgrade requirements.
5. Run the complete HitKeep QA profile. Use full/release gates for container, cloud, Helm, or storage migration changes.
6. Report stable gate IDs and separately report docs-repository validation.

## 6. Verification matrix

Every phase selects the smallest relevant subset; completion of the whole migration requires all rows.

| Contract | Required proof |
|---|---|
| Legacy env | Every `HITKEEP_*` key yields the same typed and normalized value under old/new loaders |
| Flags | Current/deprecated/both/invalid/empty cases preserve precedence, warnings, and exit behavior |
| Defaults | Binary, container, Compose, Helm, development, self-hosted, cloud, and healthcheck defaults are explicit and compared |
| Config files | Explicit YAML valid/unknown/missing/malformed/sensitive cases; no implicit file discovery |
| Secrets | Logs, errors, catalogs, examples, and generated docs contain no raw secret values |
| Cobra | Fresh in-memory root per case; no state leakage; context cancellation; stdout/stderr capture; exit compatibility |
| Developer CLI/MCP | Command catalog, JSON envelopes, workspace routing, status cursors, and manifest remain stable |
| Docker upgrade | Previous image -> deleted container -> candidate image -> deleted/recreated candidate; same volume and omitted new env |
| Migration safety | Preflight, interruption at every boundary, retry, rollback/resume, marker/file consistency, low space, permissions |
| Surface drift | Deterministic render/check for Docker, Compose, Helm, examples, docs artifact, workflows/skills as applicable |
| Filesystem | In-memory unit proof plus real OS durability/cross-device/security tests where semantics require it |
| Package moves | Import graph, cycles, self-hosted/cloud builds, production/devtool boundary, all affected package tests |
| Release | `developer-docs`, Go static gates, focused races, vulnerability scan, GoReleaser snapshot/legacy artifact-manifest parity, binary/image builds, immutable-tag behavior, and required full profile gates |

## 7. Observability and failure behavior

- Configuration errors name the canonical key and source class, not a sensitive value.
- The application logs the selected explicit config-file path and catalog schema version at debug level.
- Migration preflight reports required versus available space, resolved persistent roots, and the safe next action without credentials or customer data.
- Durable migrations expose named stages and idempotent resume state. A marker must never claim completion before the required file and directory syncs succeed.
- Generated-surface checks report the descriptor and surface that drifted, with a command/actionable workflow to regenerate.
- File operations report source, requested destination, actual final destination, and whether source cleanup failed, subject to path redaction policy.

## 8. Risks and controls

- **Viper coercion changes behavior:** shadow parity matrix and explicit decode hooks; no runtime switch until parity is exact.
- **An unchanged pflag default masks env/file input:** bind changed flags correctly and test all source pairs.
- **Automatic config discovery changes old deployments:** explicit-file-only policy in 2.x.
- **Catalog becomes another stale inventory:** runtime registration and distribution checks consume it directly; catalog coverage is mandatory.
- **Generated files become unreadable:** generate only marked config-owned blocks and keep surrounding topology hand-written.
- **Afero weakens durability semantics:** keep real OS operations at database/migration boundaries and retain crash/fault tests.
- **fileflow collision suffixes a canonical database file:** disallow suffix acceptance for canonical state paths; treat conflicts according to migration policy.
- **Flattening creates cycles or accidental supported APIs:** dependency-order waves, no layer packages, no shims without consumers, boundary tests, and app-not-library documentation.
- **Large rename diffs hide behavior changes:** move-only commits/PRs; behavior changes land before or after, never inside a move.
- **Cross-repo docs drift:** versioned artifact and schema compatibility check; docs validation reported separately.

## 9. Explicitly deferred contraction

The following are not permitted as part of this 2.x plan:

- removing or renaming an existing environment variable or flag;
- changing existing source precedence or defaults;
- enabling implicit config-file discovery;
- deleting deprecated aliases before a separately planned major release;
- requiring config files instead of environment variables;
- changing on-disk paths solely for package/filesystem cleanup;
- making cloud-only behavior available in self-hosted builds;
- treating newly root-level packages as a stable public library API;
- remote configuration, live reload, config watchers, or a configuration service;
- a custom filesystem abstraction layered over Afero;
- a big-bang `internal/` move.

A future 3.0 contraction may remove aliases after measured deprecation, reconsider implicit file discovery, and formalize any public package API. It requires a separate compatibility and migration plan.

## 10. Definition of done

The migration is complete when:

- both binaries are factory-built Cobra applications with context-aware, in-memory-tested routing;
- all runtime configuration is assembled by a fresh Viper instance into the typed config with legacy parity proven;
- the canonical catalog drives runtime registration, the clear/generated `hitkeep.example.yaml`, and mechanical validation of every required publishing surface;
- GoReleaser reproducibly owns tagged self-hosted archives and checksums without changing artifact contracts; Homebrew/Scoop enablement remains incomplete until supported Darwin/Windows CGO builds and package-manager lifecycle smokes pass;
- the issue #288 container recreation test is a required release gate and fails on any missing persistent default;
- filesystem operations use Afero, fileflow, pathologize, or OS primitives according to their actual semantics, with no duplicated unsafe move/path code;
- no Go source, generator, workflow, skill, or documentation path depends on the `internal/` directory tree;
- self-hosted and cloud builds, complete QA, storage fault tests, image upgrade tests, and adjacent docs validation pass;
- no 2.x compatibility contraction or destructive data step was smuggled into the migration.
