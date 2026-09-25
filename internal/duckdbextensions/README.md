# Bundled DuckDB extensions

HitKeep embeds the official signed `httpfs`, `aws`, and `excel` extensions for
its supported Linux and macOS architectures. Each binary embeds only its target
platform. The source distribution includes all four platforms so source builds,
including Homebrew, do not need to contact an extension repository.

The loader extracts these files atomically below `HITKEEP_DATA_PATH/.duckdb/extensions`. All stores share one native library
path per process; standalone database utilities use a per-user temporary cache.
Paths include the DuckDB version, platform, and artifact digest, so upgrades and
rollbacks do not replace one another's libraries. This is disposable cache data;
the binary can recreate it offline. DuckDB still verifies the official signatures
and ABI compatibility when loading. Automatic downloads and community extensions
remain disabled.

The extraction filesystem must permit native library mappings (it cannot be
mounted with `noexec`). File execute permission is not required.

## Updating DuckDB

After updating `github.com/duckdb/duckdb-go/v2` in `go.mod`, run from the repository
root:

```sh
go run ./internal/duckdbextensions/update
go run ./internal/duckdbextensions/update -check
```

The updater derives DuckDB's version from the upstream Go module's documented
version encoding; it downloads that version's official HTTPS artifacts and
records both compressed and extracted SHA-256 digests in `manifest.json`.
Review and commit the manifest and assets together with the dependency update.
Do not update the manifest alone or bypass signature checks.

The read-only check verifies the selected dependency version and every artifact.
HitKeep's binary/release builders and Docker source build run it before compiling.
`TestBundleMatchesDuckDBDependency` also runs it in QA. An unavailable extension
therefore blocks an upgrade instead of becoming an operator's startup download.

The database acceptance tests load the bundle with an unusable home directory
and an unreachable extension repository, exercise XLSX output, and check AWS
credential-chain setup. Run the repository's QA profile when updating artifacts;
checksum verification alone does not establish native ABI compatibility.

Artifacts originate at
`https://extensions.duckdb.org/<version>/<platform>/<extension>.duckdb_extension.gz`.
See [DuckDB's distribution documentation](https://duckdb.org/docs/current/extensions/extension_distribution)
for signing and compatibility guarantees and the upstream extension repositories
for source and licensing.
