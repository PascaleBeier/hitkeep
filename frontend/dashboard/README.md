# Dashboard

HitKeep's dashboard is an Angular 21 application that also builds the lightweight tracking snippet (`hk.js`).

## Development server

From the repository root, start the complete container-only development session:

```bash
./hk dev --seed
```

`hk` prints this workspace's URLs and streams the backend, frontend, and Mailpit logs. The dashboard automatically reloads when frontend sources change, and `Ctrl+C` stops the complete stack. Use `./hk dev --detach` for a background session and `./hk dev status --output json` to discover its workspace-scoped URLs.

Format frontend sources with `./hk fmt --scope frontend`; use `./hk fmt check --scope frontend` for the non-mutating check.

## Code scaffolding

Angular CLI includes powerful code scaffolding tools. To generate a new component, run:

```bash
ng generate component component-name
```

For a complete list of available schematics (such as `components`, `directives`, or `pipes`), run:

```bash
ng generate --help
```

## Building

Build the production application, including the embedded dashboard, through the repository workflow:

```bash
./hk build binary
```

The build compiles the production dashboard bundle, optimizes translations, rebuilds `hk.js`, syncs the result into the Go app's embedded `public/` directory, and produces the self-hosted binary.

The Scalar API Reference runtime (`vendor/scalar/standalone.js`) is copied into the build output from `node_modules/@scalar/api-reference/dist/browser/standalone.js` via Angular assets configuration, so it always matches the installed npm package version.

## Running unit tests

To execute unit tests, use:

```bash
./hk qa changed --gate frontend-unit
```

This runs Angular's Vitest-backed unit tests in non-watch headless mode. Use the command catalog's `agent_command` when invoking the gate from automation.

## Running end-to-end tests

For the real seeded end-to-end suite, run:

```bash
./hk qa changed --gate frontend-e2e
```

This is the same browser contract CI runs. The launcher:

- builds the production dashboard bundle
- builds the Go binary
- seeds realistic demo data
- starts disposable local HitKeep instances
- runs the browser journeys against the real app
- runs the deployment smoke under `/hitkeep`

The full command includes a subdirectory smoke run with `HITKEEP_E2E_PUBLIC_PATH=/hitkeep` and verifies the dashboard, authenticated route refreshes, app-owned API/resource/static image paths, the API reference iframe, tracker bundles, and ingest preflight under that prefix.

`./hk setup` prepares the pinned browser dependency on a fresh machine.

## Additional Resources

For more information on using the Angular CLI, including detailed command references, visit the [Angular CLI Overview and Command Reference](https://angular.dev/tools/cli) page.
