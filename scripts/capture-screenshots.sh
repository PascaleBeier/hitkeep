#!/usr/bin/env bash
# capture-screenshots.sh — one-command pipeline:
#
#   1. Seed a fresh database with realistic demo data
#   2. Start a local HitKeep instance pointing at that database
#   3. Wait for it to become healthy
#   4. Run screenshot.mjs to capture all dashboard views
#   5. Sync refreshed screenshots into README assets
#   6. Run preview-emails to deliver all report types to Mailpit
#   7. Clean up
#
# Usage:
#   ./scripts/capture-screenshots.sh
#
# Options (env vars or flags):
#   --db <path>           Database file (default: /tmp/hitkeep-demo.db)
#   --port <port>         HTTP port for the local instance (default: 8099)
#   --email <email>       Demo user email (default: demo@example.com)
#   --password <pass>     Demo user password (default: demo1234)
#   --days <n>            Days of demo data to seed (default: 90)
#   --output-dir <dir>    Screenshot output directory (default: ../hitkeep-docs/src/assets/screenshots)
#   --scale <n>           Device pixel ratio for screenshots (default: 2)
#   --data-path <dir>     Base directory for per-tenant data files (default: directory containing --db)
#   --mailpit-host <h>    Mailpit SMTP host (default: localhost)
#   --mailpit-port <p>    Mailpit SMTP port (default: 1025)
#   --mailpit-ui <p>      Mailpit web UI port (default: 8025)
#   --no-seed             Skip seeding — use the --db database as-is
#   --no-build            Skip 'go build' and use an existing binary at ./hitkeep-bin
#   --no-emails           Skip the email preview step even if Mailpit is reachable
#
# Environment-variable equivalents (flags take precedence):
#   DB, PORT, HITKEEP_EMAIL, HITKEEP_PASSWORD, SEED_DAYS,
#   OUTPUT_DIR, SCALE, DATA_PATH, MAILPIT_HOST, MAILPIT_PORT, MAILPIT_UI,
#   SKIP_SEED, SKIP_BUILD, SKIP_EMAILS
#
# Prerequisites:
#   npm install playwright && npx playwright install chromium
#   mailpit (optional — brew install mailpit) for email previews
#
# Example — full rebuild + fresh data + email previews:
#   ./scripts/capture-screenshots.sh
#
# Example — re-shoot existing instance (no seed):
#   HITKEEP_URL=http://localhost:8080 \
#   HITKEEP_EMAIL=admin@example.com \
#   HITKEEP_PASSWORD=secret \
#   node scripts/screenshot.mjs

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# ─── Defaults ────────────────────────────────────────────────────────────────
DB="${DB:-/tmp/hitkeep-demo.db}"
PORT="${PORT:-8099}"
EMAIL="${HITKEEP_EMAIL:-demo@example.com}"
PASSWORD="${HITKEEP_PASSWORD:-demo1234}"
DAYS="${SEED_DAYS:-90}"
OUTPUT_DIR="${OUTPUT_DIR:-${REPO_DIR}/../hitkeep-docs/src/assets/screenshots}"
SCALE="${SCALE:-2}"
DATA_PATH="${DATA_PATH:-$(dirname "$DB")}"
MAILPIT_HOST="${MAILPIT_HOST:-localhost}"
MAILPIT_PORT="${MAILPIT_PORT:-1025}"
MAILPIT_UI="${MAILPIT_UI:-8025}"
SKIP_SEED="${SKIP_SEED:-}"
SKIP_BUILD="${SKIP_BUILD:-}"
SKIP_EMAILS="${SKIP_EMAILS:-}"
BIN_PATH="${REPO_DIR}/.screenshot-hitkeep"   # temp binary; deleted on exit

# ─── Parse flags ─────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --db)            DB="$2";            shift 2 ;;
    --port)          PORT="$2";          shift 2 ;;
    --email)         EMAIL="$2";         shift 2 ;;
    --password)      PASSWORD="$2";      shift 2 ;;
    --days)          DAYS="$2";          shift 2 ;;
    --output-dir)    OUTPUT_DIR="$2";    shift 2 ;;
    --scale)         SCALE="$2";         shift 2 ;;
    --data-path)     DATA_PATH="$2";     shift 2 ;;
    --mailpit-host)  MAILPIT_HOST="$2";  shift 2 ;;
    --mailpit-port)  MAILPIT_PORT="$2";  shift 2 ;;
    --mailpit-ui)    MAILPIT_UI="$2";    shift 2 ;;
    --no-seed)       SKIP_SEED=1;        shift   ;;
    --no-build)      SKIP_BUILD=1;       shift   ;;
    --no-emails)     SKIP_EMAILS=1;      shift   ;;
    *) echo "Unknown flag: $1" >&2; exit 1       ;;
  esac
done

BASE_URL="http://localhost:${PORT}"
NSQ_TCP="127.0.0.1:$((PORT + 50))"    # NSQ TCP: port+50 (e.g. 8149)
NSQ_HTTP="127.0.0.1:$((PORT + 51))"   # NSQ HTTP: port+51 (e.g. 8150)
GOSSIP="127.0.0.1:$((PORT + 52))"     # Memberlist gossip: port+52

# ─── State ───────────────────────────────────────────────────────────────────
HK_PID=""

# ─── Cleanup ─────────────────────────────────────────────────────────────────
cleanup() {
  if [[ -n "$HK_PID" ]] && kill -0 "$HK_PID" 2>/dev/null; then
    echo ""
    echo "  Stopping HitKeep (pid $HK_PID)…"
    kill "$HK_PID" 2>/dev/null || true
    wait "$HK_PID" 2>/dev/null || true
  fi
  rm -f "$BIN_PATH"
}
trap cleanup EXIT INT TERM

# ─── Banner ──────────────────────────────────────────────────────────────────
echo ""
echo "  ╔══════════════════════════════════════════════╗"
echo "  ║   HitKeep Screenshot Pipeline               ║"
echo "  ╚══════════════════════════════════════════════╝"
echo "  DB       : $DB"
echo "  Server   : $BASE_URL"
echo "  Output   : $OUTPUT_DIR"
echo "  Scale    : ${SCALE}x"
echo "  Data     : $DATA_PATH"
echo "  Email    : $EMAIL"
if [[ -z "$SKIP_EMAILS" ]]; then
  echo "  Mailpit  : ${MAILPIT_HOST}:${MAILPIT_PORT} (UI :${MAILPIT_UI})"
fi
echo ""

# ─── Step 1: Seed demo data ───────────────────────────────────────────────────
if [[ -z "$SKIP_SEED" ]]; then
  echo "  [1/6] Seeding demo data (${DAYS} days)…"
  rm -f "$DB"
  (cd "$REPO_DIR" && go run ./cmd/seed \
    -db      "$DB"      \
    -data-path "$DATA_PATH" \
    -email   "$EMAIL"   \
    -password "$PASSWORD" \
    -days    "$DAYS")
  echo "  ✓ Seed complete"
else
  echo "  [1/6] Skipping seed — using existing DB: $DB"
fi

# ─── Step 2: Build frontend + HitKeep binary ─────────────────────────────────
echo ""
echo "  [2/6] Building dashboard assets and HitKeep…"
if [[ -z "$SKIP_BUILD" ]]; then
  (cd "$REPO_DIR/frontend/dashboard" && npm run build:prod)
  (cd "$REPO_DIR" && go build -tags "$("$REPO_DIR/scripts/go-build-tags.sh")" -o "$BIN_PATH" ./cmd/hitkeep/)
  echo "  ✓ Build complete"
else
  if [[ ! -x "$BIN_PATH" ]]; then
    echo "  ✗ No binary found at $BIN_PATH and --no-build was set" >&2
    exit 1
  fi
  echo "  Reusing existing binary: $BIN_PATH"
fi

# ─── Step 3: Start HitKeep ───────────────────────────────────────────────────
echo ""
echo "  [3/6] Starting HitKeep on ${BASE_URL}…"

# Use process substitution so $! is the hitkeep PID, not sed's.
# With a plain pipe (cmd | sed &) $! would be sed's PID and cleanup
# would fail to kill the server.
"$BIN_PATH" \
  -db               "$DB"        \
  -data-path        "$DATA_PATH" \
  -http             ":${PORT}"   \
  -public-url       "$BASE_URL"  \
  -bind             "$GOSSIP"    \
  -nsq-tcp-address  "$NSQ_TCP"   \
  -nsq-http-address "$NSQ_HTTP"  \
  -jwt-secret       "screenshot-only-$(date +%s%N)" \
  -log-level        warn         \
  > >(sed 's/^/    [hitkeep] /') 2>&1 &

HK_PID=$!

# Wait up to 60 s for the health endpoint.
echo "  Waiting for server to be ready…"
READY=0
for i in $(seq 1 60); do
  if curl -sf "${BASE_URL}/healthz" >/dev/null 2>&1; then
    READY=1
    echo "  ✓ Server ready (${i}s)"
    break
  fi
  # Bail early if the process already died.
  if ! kill -0 "$HK_PID" 2>/dev/null; then
    echo "  ✗ HitKeep exited unexpectedly — check logs above" >&2
    exit 1
  fi
  sleep 1
done

if [[ "$READY" -ne 1 ]]; then
  echo "  ✗ Server did not become healthy within 60 s" >&2
  exit 1
fi

# ─── Step 4: Capture screenshots ─────────────────────────────────────────────
echo ""
echo "  [4/6] Capturing screenshots…"
echo ""

HITKEEP_URL="$BASE_URL"      \
HITKEEP_EMAIL="$EMAIL"       \
HITKEEP_PASSWORD="$PASSWORD" \
OUTPUT_DIR="$OUTPUT_DIR"     \
SCALE="$SCALE"               \
  node "$SCRIPT_DIR/screenshot.mjs"

# ─── Step 5: Sync README assets ───────────────────────────────────────────────
echo ""
echo "  [5/6] Syncing README screenshot assets…"
mkdir -p "$REPO_DIR/.github/assets"
find "$OUTPUT_DIR" -maxdepth 1 -type f -name '*.png' -exec cp {} "$REPO_DIR/.github/assets/" \;
echo "  ✓ Synced app screenshots to $REPO_DIR/.github/assets"

# ─── Step 6: Preview emails ───────────────────────────────────────────────────
echo ""
echo "  [6/6] Email previews…"

if [[ -n "$SKIP_EMAILS" ]]; then
  echo "  Skipped (--no-emails)"
elif ! nc -z "$MAILPIT_HOST" "$MAILPIT_PORT" 2>/dev/null; then
  echo "  ⚠  Mailpit not reachable on ${MAILPIT_HOST}:${MAILPIT_PORT} — skipping."
  echo "     Start it with:  mailpit"
  echo "     Then re-run with:  --no-seed --no-build"
else
  echo ""
  (cd "$REPO_DIR" && go run ./cmd/preview-emails \
    -host          "$BASE_URL"     \
    -email         "$EMAIL"        \
    -password      "$PASSWORD"     \
    -mailpit-host  "$MAILPIT_HOST" \
    -mailpit-port  "$MAILPIT_PORT" \
    -mailpit-ui    "$MAILPIT_UI"   \
  2>&1 | sed 's/^/    [preview-emails] /')
  echo ""
  echo "  ✓ Emails delivered — open Mailpit: http://${MAILPIT_HOST}:${MAILPIT_UI}"
fi
