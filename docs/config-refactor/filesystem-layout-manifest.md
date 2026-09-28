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

The layout move does not change filesystem behavior. The audit of the former Afero and fileflow candidates closed without a migration:

- `aianalytics`, `importables`, and `ipmeta/ipmetagen` make only a few direct reads and writes. Injecting Afero would widen their function contracts without isolating anything their `t.TempDir` tests don't already isolate.
- `blocking/spamfeed`, `devtool` runs and artifacts, and `devtool/runs.go::copyTree` deliberately replace files, rotate logs, or swap directories. fileflow never overwrites; it writes a suffixed copy instead. Their tests also rely on real symlinks and file modes, which `MemMapFs` does not model.
- Untrusted path segments are either allowlisted already or handled by `os.Root`, as in `assetstore`, `devtool/artifacts.go`, `devtool/runs.go::copyTree`, and managed toolchain extraction. Use `os.Root` for new cases. Apply pathologize only to untrusted segments beneath a trusted root; never sanitize configured paths silently.

Afero stays where it is already injected: configuration loading and `config init`.

Keep embedded `io/fs` assets, DuckDB database and migration files, WAL and recovery state, fsync, locks, atomic replacement, process/PID coordination, `/proc`, and cgroup probes native. Fileflow and Afero do not substitute for those durability or process contracts.

The mechanical move is reversible by reverting the layout commit and restoring the old imports and build paths. It performs no data migration and changes no persisted file format.
