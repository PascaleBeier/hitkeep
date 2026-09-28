# Configuration and layout migration

Status: 2.14.0 release candidate, 2026-09-28. The detailed slice history is retained in Git; this page records the current state. The [implementation plan](../architecture/cobra-viper-config-go-layout-migration.md) and [filesystem boundary](filesystem-layout-manifest.md) contain the remaining contracts.

## Current state

| Area | State | Evidence or remaining gate |
| --- | --- | --- |
| Cobra and Viper runtime configuration | Implemented | Catalog-backed flags and strict explicit YAML preserve the legacy env/flag/default and alias behavior in self-hosted and cloud builds. The test-only legacy oracle remains until a stabilization release passes. |
| Configuration publication | Implemented locally | Example YAML, Docker, Compose, Helm, runtime catalog, and private-doc digest contract are checked. A real cross-repository attestation rehearsal is still required. |
| Upgrade and rollback | Implemented locally | Digest-pinned v2.12 upgrade, repeated same-volume recreation, quiescent rollback, and interrupted default-tenant split recovery passed. Tagged workflow execution remains a release gate. |
| Release artifacts | Candidate ready | Native builds, snapshot archives, configuration manifest, checksums, and local Homebrew lifecycle have proof. Tagged archive/checksum ownership and Windows/Scoop lifecycle need release evidence. |
| Filesystem operations | Boundary recorded | The package move leaves I/O behavior unchanged. Afero, fileflow, and pathologize candidates in the [filesystem boundary](filesystem-layout-manifest.md) are separate follow-up work; DuckDB/WAL/fsync/lock operations stay native. |
| Go package layout | Complete | All former `internal/` Go package trees are at the module root, including `devtool` merged with its existing `cli` and `devmcp` children. Package names, exported APIs, and relative embeds remain unchanged. No compatibility shim was added. |
| Release QA | Local gates complete | Full QA passed 26 of 27 gates on the moved source; the browser gate passed all 76 tests on a same-commit rerun after one transient translation-key failure. Both image variants and the v2.12 upgrade/recreation gate passed. Tagged workflow and publication checks remain release-time gates. |

## Current proof

- Both the HitKeep and private docs branches contain the latest fetched `origin/main` as of 2026-09-28.
- The pre-layout candidate passed full QA `20260928T110224-ba0108a8` (26 selected gates); the self-hosted upgrade/image gate passed separately as `20260928T105510-471bb2fa`. The docs build, SEO checks, content metadata validation, and all 13 release tests passed. This evidence predates the final layout move.
- The `listrefresh` move passed `go test -race ./listrefresh`; all command race-test packages passed, and changed QA `20260928T121951-8ab50b17` passed all nine selected gates. The all-package move passes `go list ./...` and `go test ./...`. Full QA `20260928T125939-0a6d4b2e` passed 26 gates; the sole failing `frontend-e2e` gate passed all 76 tests on the same commit in rerun `20260928T133019-600118d0`. The first browser run showed untranslated menu labels in one export test; that timing failure is not claimed fixed.
- The draft post is `src/content/blog/hitkeep-2-14-0.mdx` in the private docs branch with `draft: true`. It includes Reports, Ask AI, themes, offline DuckDB startup, and explicit configuration. Current Reports desktop/mobile and Ask AI screenshots are checked in; the Ask AI answer is labeled as seeded demo data.

No stable 2.14.0 release, image, site publication, or tag has been created from this candidate.
