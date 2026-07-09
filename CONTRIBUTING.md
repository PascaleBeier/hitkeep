# Contributing to HitKeep

HitKeep is an open-source project and contributions are welcome. This guide covers everything you need to set up a local development environment and submit a change.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [AI-Assisted Contributions](#ai-assisted-contributions)
- [Project Structure](#project-structure)
- [Commit Conventions](#commit-conventions)
- [Submitting a Pull Request](#submitting-a-pull-request)
- [Reporting Bugs & Security Issues](#reporting-bugs--security-issues)

---

## Prerequisites

The easiest local setup only needs Docker with Docker Compose support. The
Docker dev stack runs Go, Air, Angular, and Mailpit in containers.

| Tool       | Version | Purpose                                  |
|:-----------|:--------|:-----------------------------------------|
| **Docker** | current | Full local development stack with Compose |

If you prefer native development on your host, also install:

| Tool                  | Version        | Purpose                                           |
|:----------------------|:---------------|:--------------------------------------------------|
| **Go**                | 1.26+          | Backend compilation and pinned Go tools           |
| **CGo / C toolchain** | system default | Required by DuckDB's Go bindings                  |
| **Node.js + npm**     | 24+ (LTS)      | Angular dashboard and tracker snippet build       |
| **Mailpit**           | latest         | Local SMTP inbox for mail flows                   |

Air is pinned in `go.mod` and runs through `go tool air`; do not install a
separate global Air binary for normal HitKeep development.

### Native macOS

```bash
# Go (via Homebrew or https://go.dev/dl/)
brew install go

# C toolchain (required for CGo / DuckDB)
xcode-select --install

# Node.js (via fnm, nvm, or Homebrew)
brew install fnm
fnm install 24
fnm use 24

# Local SMTP inbox
brew install mailpit
```

### Native Linux (Ubuntu / Debian)

```bash
# Go — download from https://go.dev/dl/ and follow official instructions
# Or via snap:
sudo snap install go --classic

# C toolchain
sudo apt-get install -y gcc g++ make

# Node.js (via fnm or nvm)
curl -fsSL https://fnm.vercel.app/install | bash
fnm install 24 && fnm use 24

# Local SMTP inbox
# See https://mailpit.axllent.org/docs/install/
```

---

## Getting Started

**1. Clone the repository:**

```bash
git clone https://github.com/pascalebeier/hitkeep.git
cd hitkeep
```

**2. Start the full Docker development stack:**

```bash
make dev-docker-seed
```

This starts:

- **Backend:** Go 1.26.5 with Air live reload on `http://localhost:8080`
- **Frontend:** Angular dev server with hot reload on `http://localhost:4200`
- **Mailpit:** local mail UI on `http://localhost:8025`
- **Seed data:** demo user, site, analytics, ecommerce, AI visibility, and chatbot data

Open `http://localhost:4200` and sign in with:

```text
demo@example.com
demo1234
```

If you do not have `make`, use Docker Compose directly:

```bash
docker compose -f compose.dev.yaml run --rm seed
docker compose -f compose.dev.yaml up --build backend frontend mailpit
```

Use `make dev-docker` when you want the same Docker stack without reseeding data.

---

## Development Workflow

### Docker Development

```bash
# Full hot-reload stack
make dev-docker

# Seed demo data, then start the stack
make dev-docker-seed

# Stop containers
make dev-docker-down

# Stop containers and remove dev volumes
make dev-docker-clean
```

The Docker stack keeps Go modules, Go build cache, npm cache, `node_modules`,
and development data in named Docker volumes. Your source tree is bind-mounted,
so changing Go or Angular files triggers the matching live-reload process.

### Native Development

Use native dev if you already have Go, Node.js, npm, a C toolchain, and Mailpit
installed on your host.

```bash
make dev-seed
```

This runs the backend and frontend in parallel on your host. The backend serves
the API on `:8080`, and the Angular dev server proxies `/api/*` and `/ingest`
to the backend.

### Backend Only

```bash
make dev-backend
```

The `.air.toml` configures Air to watch `*.go`, `*.sql`, `*.html`, `*.tpl`, and `*.tmpl` files. It excludes `frontend/`, `public/`, and `node_modules/`.

When you change a Go file, `go tool air` recompiles and restarts in ~1-2 seconds.

### Frontend Only

```bash
make dev-frontend
```

This starts `ng serve` on `http://localhost:4200`. The Angular proxy config forwards API calls to the backend.

### Full Build (Production Artifacts)

```bash
make build
```

This:
1. Runs `npm ci && ng build --configuration production` for the dashboard (output: `frontend/dashboard/dist/`)
2. Minifies the tracker snippet (`src/tracker/index.ts` → `dist/dashboard/browser/hk.js`) via esbuild
3. Copies the dashboard bundle to `public/`
4. Compiles the Go binary with shared HitKeep build tags: `go build -tags "$(./scripts/go-build-tags.sh)" -o hitkeep ./cmd/hitkeep/main.go`

The binary embeds the `public/` directory, so the build order matters.

### Running Tests

```bash
# Go checks
GOFLAGS="$(./scripts/go-build-tags.sh goflags)" go test ./...
GOFLAGS="$(./scripts/go-build-tags.sh goflags)" go test -race ./...
golangci-lint run "$(./scripts/go-build-tags.sh golangci)"
# Angular checks
cd frontend/dashboard && npm run fmt && npm run fmt:check
cd frontend/dashboard && npm run lint
cd frontend/dashboard && npm run test -- --watch=false --no-progress

# Seeded end-to-end tests
cd frontend/dashboard && npx playwright install --with-deps chromium
cd frontend/dashboard && npm run e2e
```

Notes:

- `npm run e2e` is the canonical entrypoint for browser end-to-end tests locally and in CI.
- The e2e launcher builds the dashboard, builds the Go binary, seeds demo data, starts disposable local HitKeep instances, and also runs the `/hitkeep` subdirectory deployment smoke.
- Angular 22 still supports `ng e2e`, but HitKeep uses Playwright directly so the documented command matches CI exactly.

If you are making a change that touches frontend behavior, try to run the relevant browser coverage before opening a PR:

```bash
# Full seeded suite
cd frontend/dashboard && npm run e2e

# Or a focused spec while iterating
cd frontend/dashboard && npm run e2e -- e2e/auth.seeded.spec.js --workers=1
```

### Suggested Verification Before a PR

```bash
# Backend
GOFLAGS="$(./scripts/go-build-tags.sh goflags)" go test ./...

# Frontend
cd frontend/dashboard && npm run fmt && npm run fmt:check
cd frontend/dashboard && npm run lint
cd frontend/dashboard && npm run test -- --watch=false --no-progress

# Browser coverage for UI changes
cd frontend/dashboard && npm run e2e
```

### Cleanup

```bash
make clean
# Removes: ./hitkeep binary, public/, frontend/*/dist/, frontend/*/node_modules/
```

---

## AI-Assisted Contributions

AI-assisted PRs are welcome when they follow the same privacy, testing, and review bar as any other contribution. Start with the public [HitKeep Agent Guide](./AGENTS.md) before making repo changes.

Do not paste secrets, customer analytics, private deployment notes, local machine paths, or dashboard screenshots with private data into prompts, commits, docs, skills, or issue comments.

### MCP Changes

HitKeep's MCP server is intentionally read-only and aggregate-only.

When changing `internal/mcpserver/`, check that:

- every tool is read-only and sets `ReadOnlyHint`
- analytics tools stay closed-world, while docs lookup tools are the only open-world tools
- API client bearer tokens and explicit site grants remain the auth model
- no tool exposes raw hit exports, write workflows, billing, site administration, token management, takeout, or dashboard sessions
- `internal/mcpserver/audit_test.go`, `tests/scripts/mcp-audit.sh`, public docs, and Agent Skills stay aligned with any tool changes

Run the focused audit:

```bash
GOFLAGS="$(./scripts/go-build-tags.sh goflags)" go test ./internal/mcpserver -run 'TestMCP.*Audit'
tests/scripts/mcp-audit.sh --schema-only
```

### Agent Skills Changes

The `skills/` directory contains public instructions for end-user assistants.

When adding or changing skills:

- keep skill folders as direct children of `skills/`
- do not include credentials, customer data, private URLs, or private screenshots
- update `skills/README.md`
- keep MCP tool names, filters, caveats, and output expectations synchronized with the live server and docs

### Dashboard i18n Changes

Use the public `hitkeep-i18n` skill when changing dashboard UI copy, translation keys, locale JSON files, language switching, PrimeNG locale behavior, or localized formatting.

When changing dashboard text:

- keep user-visible copy in Transloco keys instead of hardcoded component strings
- update all seven locale files: `en`, `de`, `es`, `fr`, `it`, `nl`, and `pt`
- preserve interpolation variables and placeholder syntax
- use existing locale helpers for dates, numbers, percentages, and durations
- keep short labels short enough for buttons, tabs, table columns, and mobile layouts

Run:

```bash
cd frontend/dashboard && npm run i18n:check
cd frontend/dashboard && npm run fmt:check
```

### AI Structured Output Changes

Optional AI features must save validated product data, not raw model traffic.

When changing AI provider or Opportunities behavior, check that:

- raw prompts, raw provider payloads, external error bodies, provider headers, and secrets are not persisted
- final output passes structured validation before storage
- cited evidence IDs refer to evidence supplied to the run
- GoAI-backed Opportunity proposal changes keep the key/param contract deterministic and add validator coverage for new saved fields, message keys, params, action types, or evidence shapes
- deterministic permission checks and analytics queries stay outside the model
- saved copy remains localization-key based where the dashboard expects localized output

Useful focused checks:

```bash
GOFLAGS="$(./scripts/go-build-tags.sh goflags)" go test ./internal/ai ./internal/opportunities ./internal/database ./internal/mcpserver
```

### Docs, OpenAPI, And Privacy Review

Public behavior changes should update docs in the separate `hitkeep-docs` repository. API contract changes should update both the runtime OpenAPI source and the docs OpenAPI file.

Before opening an AI-assisted PR, review:

- which public docs changed, or why docs were not needed
- whether new database tables carry a scope column (`site_id`, `tenant_id`/`team_id`) or are registered in the cleanup specs (see Database Rules in `AGENTS.md`)
- whether `server.json` changed because MCP Registry metadata changed
- whether `public/openapi.yml` in the docs repo changed because public API shape changed
- whether the PR description lists the commands run
- whether the diff avoids secrets, local-only paths, private deployment details, and customer data

---

## Project Structure

```
hitkeep/
├── cmd/
│   ├── hitkeep/           # Main application entry point
│   └── seed/              # Local/demo data seeding
├── internal/
│   ├── database/          # DuckDB store — all SQL queries live here
│   ├── server/            # HTTP server setup, middleware, shared handlers
│   ├── ingest/            # In-process ingest consumers
│   └── worker/            # Background workers (retention, rollups, reports)
├── frontend/
│   └── dashboard/         # Angular v21 SPA + tracker snippet (src/tracker/index.ts)
├── public/                # Embedded static assets (built frontend output)
├── scripts/               # Runtime/development scripts used outside tests
├── tests/
│   ├── e2e/               # E2E launchers and test-only harness scripts
│   ├── fixtures/          # Shared test fixtures, outside app public/embed trees
│   └── scripts/           # Test-only verification scripts, such as MCP audit
├── .github/               # GitHub-native config only: workflows, templates, assets
├── Makefile               # Development and build commands
└── .air.toml              # Air (live-reload) configuration
```

Keep reusable fixtures, browser test harnesses, and test-only audit scripts under
`tests/`. The `.github/` directory should contain GitHub configuration and
presentation assets only, not build or test implementation files.

## Commit Conventions

HitKeep uses [Conventional Commits](https://www.conventionalcommits.org/) and [Release Please](https://github.com/googleapis/release-please) for automated changelog generation and version bumping.

**Format:** `type(scope): description`

| Type       | When to use                                             |
|:-----------|:--------------------------------------------------------|
| `feat`     | New user-facing feature                                 |
| `fix`      | Bug fix                                                 |
| `chore`    | Maintenance, dependency updates, tooling                |
| `docs`     | Documentation changes                                   |
| `refactor` | Code change that neither adds a feature nor fixes a bug |
| `test`     | Adding or updating tests                                |
| `perf`     | Performance improvement                                 |

**Examples:**

```
feat(ingest): add support for custom event properties
fix(auth): correct JWT expiry calculation for TOTP sessions
chore(deps): update duckdb-go to v2.5.5
docs(api): document /api/events endpoint
```

Breaking changes must include `BREAKING CHANGE:` in the commit body or footer:

```
feat(auth)!: remove legacy password-only login endpoint

BREAKING CHANGE: The /api/login endpoint now requires 2FA if enabled.
```

---

## Submitting a Pull Request

1. **Fork** the repository and create a branch from `main`:
   ```bash
   git checkout -b feat/my-new-feature
   ```

2. **Write your code** following the patterns above.

3. **Test your changes:**
   ```bash
   GOFLAGS="$(./scripts/go-build-tags.sh goflags)" go test ./...
   cd frontend/dashboard && npm run fmt && npm run fmt:check
   cd frontend/dashboard && npm run lint
   cd frontend/dashboard && npm run test -- --watch=false --no-progress
   cd frontend/dashboard && npm run e2e
   ```

4. **Commit** using Conventional Commits format.

5. **Push** your branch and open a PR against `main`.

6. **PR description** should include:
   - What problem this solves
   - Any relevant issue numbers (`Closes #123`)
   - Testing steps

---

## Reporting Bugs & Security Issues

- **Bugs and feature requests:** [GitHub Issues](https://github.com/pascalebeier/hitkeep/issues)
- **Security vulnerabilities:** [GitHub Security Advisories](https://github.com/pascalebeier/hitkeep/security/advisories) — do not open a public issue for security vulnerabilities

See [SECURITY.md](./SECURITY.md) for the full vulnerability disclosure policy.
