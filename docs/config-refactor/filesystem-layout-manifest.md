# Filesystem and package layout

The Go packages formerly under `internal/` now live at the module root. The 24 remaining direct-child families moved together so their dependency edges and relative embedded assets stayed intact. Package names and exported signatures did not change. Existing callers use the new import paths; no forwarding packages are needed because the old paths were module-internal.

`devtool`'s root Go files joined the existing `devtool/cli` and `devtool/devmcp` tree. `go list ./...` and compile-only tests across every package are the first layout checks. Full source-bound QA and both image variants remain the release checks for this move.

## Build and cloud configuration ownership

These surfaces form one build-time chain. The developer catalog owns supported build variants; every other surface projects or consumes that decision. Production runtime settings remain owned by the runtime configuration catalog and loader.

| Surface | Role | Contract |
| --- | --- | --- |
| `devtool/catalog.go::variants` | Canonical developer build owner | Defines variant IDs, build tags, developer-container environment, local image names, and publishability metadata. Its cloud values are developer/build defaults, not runtime settings. |
| `devtool/app.go::App.ComposeEnvironment` | Workspace projection | Projects the selected variant plus workspace-scoped paths and ports into Compose variables; it does not define supported variants or production configuration precedence. |
| `devtool/runs.go::App.executeBuild` | Build orchestrator | Resolves a catalog variant, enforces the production/developer dependency boundary, and invokes the selected binary or image build without redefining tags or defaults. |
| `Dockerfile` | Image-build consumer | Consumes explicit build arguments and produces the selected application image; it is not a configuration catalog or runtime parser. |
| `.goreleaser.yaml` | Release-build projection | Maps the canonical self-hosted and cloud build identities to release tags, CGO targets, archive contents, and names; it must remain aligned with the developer catalog. |
| `.github/workflows/pipeline.yml` | Delivery consumer | Supplies explicit version/ref inputs, restores verified assets, and invokes the canonical build projections. Publication and attestation policy lives here, but variant semantics do not. |

## Filesystem operation boundaries

The layout move does not change filesystem behavior. Ordinary injectable host reads and writes remain Afero candidates in `aianalytics`, `blocking`, `devtool`, `importables`, and `ipmeta/ipmetagen`. Each future change must preserve permissions, no-overwrite and replacement behavior, error paths, and real-filesystem proof where OS behavior matters.

`devtool/runs.go::copyTree` remains the ordinary fileflow copy candidate. Verify final destination paths, conflict behavior, permissions, cleanup, partial failures, and cross-filesystem behavior before changing it. Apply pathologize only to untrusted segments beneath a trusted root; configured paths must not be silently sanitized.

Keep embedded `io/fs` assets, DuckDB database and migration files, WAL and recovery state, fsync, locks, atomic replacement, process/PID coordination, `/proc`, and cgroup probes native. Fileflow and Afero do not substitute for those durability or process contracts.

The mechanical move is reversible by reverting the layout commit and restoring the old imports and build paths. It performs no data migration and changes no persisted file format.
