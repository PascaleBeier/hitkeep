# Backend, API, and database

Read this reference only when the change touches the Go runtime, HTTP contracts, permissions, MCP, AI, or DuckDB.

## Navigate

- Use `cmd/hitkeep` as the production entry point and inspect nearby orchestration under `cmd/`.
- Find configuration in `internal/config`, handlers in `internal/server`, public structs in `internal/api`, workers in `internal/worker`, ingestion in `internal/ingest`, and persistence in `internal/database`.
- Treat the runtime OpenAPI source and the adjacent docs OpenAPI file as one public contract.
- Follow the detailed database, production MCP, and AI-output invariants in `AGENTS.md`; do not restate or weaken them here.

## Implement backend behavior

1. Follow the nearest existing package and route style before introducing a new package or abstraction.
2. Add configurable behavior through the canonical config struct, environment/flag surfaces, and focused tests.
3. Register HTTP routes through the existing shared handler configuration so authentication, permission scope, tenant/site resolution, and rate limiting remain consistent.
4. Resolve tenant-local analytics storage before querying data-plane tables. Keep control-plane and tenant data-plane concerns separate.
5. Return stable errors and structured logs without secrets, raw provider responses, or internal details.
6. Give credential-, worker-, scheduler-, import-, or external-service-backed features a non-secret operator status surface.
7. Make mutations auditable with failure semantics appropriate to the data operation.
8. Update runtime OpenAPI, docs OpenAPI, and dashboard client types together when the public contract changes.

## Implement API and auth changes

- Reuse public request and response types where the contract crosses packages.
- Cover denial, invalid input, success, tenant/site isolation, and unavailable-node behavior where relevant.
- Keep the Go capability catalog canonical. Regenerate derived frontend capabilities through the repository generator instead of editing them manually.
- Never add a frontend-only permission string or a route description that the Go catalog does not support.

## Implement database changes

- Use parameterized values and interpolate only fixed, validated SQL fragments.
- Pass context through queries and return bounded results.
- Resolve the correct tenant store at the caller; never join across the control-plane/data-plane seam.
- Prefer the established DuckDB aggregation, JSON extraction, appender, and rollup patterns nearest the changed query.
- Cover empty data, realistic rows, isolation, invalid dimensions, date edges, and rollup/live-data interaction as applicable.

Run focused behavior tests while iterating, then delegate completion-gate selection to `$hitkeep-qa`.
