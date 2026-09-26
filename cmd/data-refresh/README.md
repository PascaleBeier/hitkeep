# Data refresh

Run one target from the repository root:

```sh
go run ./cmd/data-refresh ai-agents [-output PATH]
go run ./cmd/data-refresh spam [-output PATH]
go run ./cmd/data-refresh ipmeta [ipmeta-generate flags]
go run ./cmd/data-refresh duckdb [-check]
```

AI agents and spam default to their embedded JSON paths. They fetch and validate upstream data on every run, but leave an existing file untouched when only its local generation timestamp differs. Changed entries or source metadata update the file. The `ipmeta` target keeps the existing IP generator flags and needs an IP2Location download token for the default release refresh. The `duckdb` target refreshes assets for the dependency already selected in `go.mod`; it does not update the Go module. Release builds use the standalone, offline `go run ./internal/duckdbextensions/update -check` command.

The existing `hitkeep update-ai-agent-lists`, `hitkeep update-spam-lists`, shell wrappers, `go run ./cmd/ipmeta-generate`, and standalone DuckDB updater remain available for compatibility.
