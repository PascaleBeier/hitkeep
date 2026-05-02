#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

PORT="${HITKEEP_E2E_PORT:-8098}"
BASE_URL="${HITKEEP_BASE_URL:-http://127.0.0.1:${PORT}}"
EMAIL="${HITKEEP_E2E_EMAIL:-demo@example.com}"
PASSWORD="${HITKEEP_E2E_PASSWORD:-demo1234}"
DAYS="${HITKEEP_E2E_DAYS:-30}"
LOG_LEVEL="${HITKEEP_E2E_LOG_LEVEL:-warn}"
SHARE_TOKEN="${HITKEEP_E2E_SHARE_TOKEN:-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"

RUN_DIR="${HITKEEP_E2E_RUN_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/hitkeep-e2e.XXXXXX")}"
DATA_PATH="${HITKEEP_E2E_DATA_PATH:-${RUN_DIR}/data}"
DB_PATH="${HITKEEP_E2E_DB_PATH:-${RUN_DIR}/hitkeep-e2e.db}"
BIN_PATH="${HITKEEP_E2E_BIN_PATH:-${RUN_DIR}/hitkeep-e2e}"

NSQ_TCP="${HITKEEP_E2E_NSQ_TCP:-127.0.0.1:$((PORT + 50))}"
NSQ_HTTP="${HITKEEP_E2E_NSQ_HTTP:-127.0.0.1:$((PORT + 51))}"
GOSSIP="${HITKEEP_E2E_GOSSIP:-127.0.0.1:$((PORT + 52))}"

HK_PID=""

cleanup() {
  if [[ -n "${HK_PID}" ]] && kill -0 "${HK_PID}" 2>/dev/null; then
    kill "${HK_PID}" 2>/dev/null || true
    wait "${HK_PID}" 2>/dev/null || true
  fi
  rm -rf "${RUN_DIR}"
}
trap cleanup EXIT INT TERM

mkdir -p "${DATA_PATH}"
rm -f "${DB_PATH}" "${DB_PATH}.wal"

echo "[e2e] run dir: ${RUN_DIR}"
echo "[e2e] building dashboard assets"
(cd "${REPO_DIR}/frontend/dashboard" && npm run build:prod)

echo "[e2e] building hitkeep binary"
(cd "${REPO_DIR}" && go build -o "${BIN_PATH}" ./cmd/hitkeep/)

echo "[e2e] seeding demo data"
(cd "${REPO_DIR}" && go run ./cmd/seed \
  -db "${DB_PATH}" \
  -data-path "${DATA_PATH}" \
  -email "${EMAIL}" \
  -password "${PASSWORD}" \
  -days "${DAYS}" \
  -share-token "${SHARE_TOKEN}")

echo "[e2e] starting hitkeep on ${BASE_URL}"
"${BIN_PATH}" \
  -db "${DB_PATH}" \
  -data-path "${DATA_PATH}" \
  -http ":${PORT}" \
  -public-url "${BASE_URL}" \
  -bind "${GOSSIP}" \
  -nsq-tcp-address "${NSQ_TCP}" \
  -nsq-http-address "${NSQ_HTTP}" \
  -api-rate-limit "${HITKEEP_E2E_API_RATE_LIMIT:-1000}" \
  -api-burst "${HITKEEP_E2E_API_BURST:-1000}" \
  -jwt-secret "e2e-only-${PORT}" \
  -log-level "${LOG_LEVEL}" \
  > >(sed 's/^/[hitkeep] /') 2>&1 &

HK_PID=$!
wait "${HK_PID}"
