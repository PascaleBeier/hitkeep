# Configuration and layout migration

Status: 2.14.0 release candidate, 2026-09-28. Git keeps the detailed slice history; this page records the current state. The [implementation plan](../architecture/cobra-viper-config-go-layout-migration.md) still holds the 2.x compatibility contract, and the [filesystem boundary](filesystem-layout-manifest.md) records which I/O stays native.

## Current state

| Area | State | Evidence or remaining gate |
| --- | --- | --- |
| Configuration loading | Complete | `config/load.go` uses Viper for catalog defaults, the explicit YAML file, and environment binding (including deprecated names). pflag owns the flags; deprecated flags are normalized onto canonical ones, so the last occurrence still wins. A small argv shim keeps the 2.x single-dash grammar (`-db x`). The reflection setters and stdlib flag set that ran underneath Viper are gone. |
| Legacy parity | Retired after proof | At `b88cf0b5` the new loader matched the legacy loader for every catalog setting and twelve legacy grammar edge cases, in both self-hosted and billing builds. `config/load_test.go` now characterizes every setting through each source, together with the legacy grammar. |
| Production commands | Complete | One context-aware `run` serves the root, healthcheck, config fallback, and `help`. Before this change, `hitkeep help` started the server without the signal context. Startup failures return `ExitError` with the same JSON log line, instead of panicking. Recovery subcommands keep their stdlib flag sets, because those flags are local, single-dash, and already follow the 2.x `-h`/exit-2 grammar. |
| Configuration publication | Implemented locally | Example YAML, Docker, Compose, Helm, runtime catalog, and private-doc digest contract are checked. A real cross-repository attestation rehearsal is still required. `config init` now creates owner-only files (`0600`). |
| Release and workflow checks | Tool-owned | Pinned `actionlint`, `helm lint --strict`, `zizmor`, and `goreleaser check` gates replace the Go tests that pattern-matched workflow, GoReleaser, chart, and script text. Some Go checks remain, because no linter can make them: version metadata consistency, the mapping from PR gates to CI groups, the cross-repository docs receiver pin, and the tests that execute the real release scripts against a stubbed `gh`. |
| Upgrade and rollback | Implemented locally | Digest-pinned v2.12 upgrade, repeated same-volume recreation, quiescent rollback, and interrupted default-tenant split recovery passed. Tagged workflow execution remains a release gate. |
| Release artifacts | Candidate ready | Native builds, snapshot archives, configuration manifest, checksums, and local Homebrew lifecycle have proof. Tagged archive/checksum ownership and the Windows/Scoop lifecycle still need release evidence. |
| Filesystem operations | Closed | Afero stays where it is injected today. None of the remaining candidates fit fileflow, because fileflow suffixes instead of replacing and the tests need real symlinks and modes. Toolchain extraction now writes through `os.Root`, and QA plan IDs are validated before they name a file. |
| Go package layout | Complete | All former `internal/` Go package trees are at the module root. Package names, exported APIs, and relative embeds are unchanged. |

## Size

Go lines at the pre-refactor base `f0a9a502`, at the layout-complete candidate `fc8e390f`, and after the simplification:

| Package | Production | Tests |
| --- | --- | --- |
| `config` | 987 → 1,306 → 1,266 | 1,171 → 2,340 → 1,868 |
| `cmd` | 6,516 → 6,711 → 6,609 | 2,602 → 4,220 → 4,222 |
| `devtool` | 9,607 → 10,357 → 9,911 | 3,411 → 7,005 → 5,021 |

The simplification removed a net 3,068 lines across 55 files.

## Current proof

- Both the HitKeep and private docs branches contained the latest `origin/main` on 2026-09-28.
- The simplified candidate passed full QA run `20260928T154321-d4039c31`, 28 of 29 gates including the new `actionlint` and `helm-lint`. The 29th gate, `self-hosted-image`, built the image but stopped because `HITKEEP_PREVIOUS_IMAGE` was not set. It then passed the v2.12 upgrade, recreation, and rollback smoke in rerun `20260928T161517-01459908`, using the supported floor image from `tests/fixtures/release-fixtures.json`.
- The layout-complete candidate `fc8e390f` passed full QA `20260928T125939-0a6d4b2e` (26 gates); `frontend-e2e` passed all 76 tests in rerun `20260928T133019-600118d0`. The v2.12 upgrade/recreation gate and both image variants passed.
- The draft post is `src/content/blog/hitkeep-2-14-0.mdx` in the private docs branch, with `draft: true`.

No stable 2.14.0 release, image, site publication, or tag has been created from this candidate.
