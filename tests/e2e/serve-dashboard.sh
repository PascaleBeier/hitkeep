#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

normalize_public_path() {
  local value="${1:-/}"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  if [[ -z "${value}" || "${value}" == "/" ]]; then
    printf '/'
    return
  fi
  value="/${value#/}"
  value="${value%/}/"
  printf '%s' "${value}"
}

join_public_url() {
  local base="${1%/}"
  local public_path
  public_path="$(normalize_public_path "${2}")"

  if [[ "${public_path}" == "/" ]]; then
    printf '%s' "${base}"
    return
  fi

  printf '%s%s' "${base}" "${public_path%/}"
}

PORT="${HITKEEP_E2E_PORT:-8098}"
PUBLIC_PATH="${HITKEEP_E2E_PUBLIC_PATH:-/}"
BASE_URL="$(join_public_url "${HITKEEP_BASE_URL:-http://127.0.0.1:${PORT}}" "${PUBLIC_PATH}")"
EMAIL="${HITKEEP_E2E_EMAIL:-demo@example.com}"
PASSWORD="${HITKEEP_E2E_PASSWORD:-demo1234}"
DAYS="${HITKEEP_E2E_DAYS:-30}"
LOG_LEVEL="${HITKEEP_E2E_LOG_LEVEL:-warn}"
SHARE_TOKEN="${HITKEEP_E2E_SHARE_TOKEN:-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"
MAIL_HOST="${HITKEEP_E2E_MAIL_HOST:-127.0.0.1}"
MAIL_PORT="${HITKEEP_E2E_MAIL_PORT:-2525}"
MAIL_ENCRYPTION="${HITKEEP_E2E_MAIL_ENCRYPTION:-none}"
SKIP_DASHBOARD_BUILD="${HITKEEP_E2E_SKIP_DASHBOARD_BUILD:-${HITKEEP_E2E_SKIP_BUILD:-0}}"
SKIP_BINARY_BUILD="${HITKEEP_E2E_SKIP_BINARY_BUILD:-${HITKEEP_E2E_SKIP_BUILD:-0}}"

REMOVE_RUN_DIR=false
if [[ -n "${HITKEEP_E2E_RUN_DIR:-}" ]]; then
  RUN_DIR="${HITKEEP_E2E_RUN_DIR}"
else
  RUN_DIR="$(mktemp -d "${TMPDIR:-/tmp}/hitkeep-e2e.XXXXXX")"
  REMOVE_RUN_DIR=true
fi
DATA_PATH="${HITKEEP_E2E_DATA_PATH:-${RUN_DIR}/data}"
DB_PATH="${HITKEEP_E2E_DB_PATH:-${RUN_DIR}/hitkeep-e2e.db}"
BIN_PATH="${HITKEEP_E2E_BIN_PATH:-${RUN_DIR}/hitkeep-e2e}"

NSQ_TCP="${HITKEEP_E2E_NSQ_TCP:-127.0.0.1:$((PORT + 50))}"
NSQ_HTTP="${HITKEEP_E2E_NSQ_HTTP:-127.0.0.1:$((PORT + 51))}"
GOSSIP="${HITKEEP_E2E_GOSSIP:-127.0.0.1:$((PORT + 52))}"

HK_PID=""
OWNER_PID="${HITKEEP_E2E_OWNER_PID:-${PPID}}"
OWNER_WATCH_PID=""

watch_owner() {
  while kill -0 "${OWNER_PID}" 2>/dev/null; do
    sleep 0.5
  done
  kill -TERM "$$" 2>/dev/null || true
}

cleanup() {
  trap - EXIT INT TERM
  if [[ -n "${OWNER_WATCH_PID}" ]] && kill -0 "${OWNER_WATCH_PID}" 2>/dev/null; then
    kill "${OWNER_WATCH_PID}" 2>/dev/null || true
    wait "${OWNER_WATCH_PID}" 2>/dev/null || true
  fi
  if [[ -n "${HK_PID}" ]] && kill -0 "${HK_PID}" 2>/dev/null; then
    kill "${HK_PID}" 2>/dev/null || true
    wait "${HK_PID}" 2>/dev/null || true
  fi
  if [[ "${REMOVE_RUN_DIR}" == "true" ]]; then
    rm -rf "${RUN_DIR}"
  fi
}
trap cleanup EXIT INT TERM

# Playwright starts web servers in their own process groups. If a parent QA
# process is force-cancelled it cannot run Playwright's normal teardown, so
# stop this server when its owning process disappears instead of leaving a
# reusable, mutated fixture on the workspace's e2e port.
watch_owner &
OWNER_WATCH_PID=$!

mkdir -p "${DATA_PATH}"
rm -f "${DB_PATH}" "${DB_PATH}.wal"

echo "[e2e] run dir: ${RUN_DIR}"
if [[ "${SKIP_DASHBOARD_BUILD}" == "1" ]]; then
  if [[ ! -f "${REPO_DIR}/frontend/dashboard/dist/dashboard/browser/index.html" ]]; then
    echo "[e2e] dashboard assets missing; unset HITKEEP_E2E_SKIP_DASHBOARD_BUILD or run npm run build:prod first" >&2
    exit 1
  fi
  echo "[e2e] reusing dashboard assets"
else
  echo "[e2e] building dashboard assets"
  (cd "${REPO_DIR}/frontend/dashboard" && npm run build:prod)
fi

if [[ "${SKIP_BINARY_BUILD}" == "1" ]]; then
  if [[ ! -x "${BIN_PATH}" ]]; then
    echo "[e2e] hitkeep binary missing at ${BIN_PATH}; unset HITKEEP_E2E_SKIP_BINARY_BUILD or build it first" >&2
    exit 1
  fi
  echo "[e2e] reusing hitkeep binary"
else
  echo "[e2e] building hitkeep binary"
  (cd "${REPO_DIR}" && go build -tags "$("${REPO_DIR}/hk" ci go-config tags --output plain)" -o "${BIN_PATH}" ./cmd/hitkeep/)
fi

echo "[e2e] seeding demo data"
(cd "${REPO_DIR}" && go run ./cmd/seed \
  -db "${DB_PATH}" \
  -data-path "${DATA_PATH}" \
  -email "${EMAIL}" \
  -password "${PASSWORD}" \
  -days "${DAYS}" \
  -share-token "${SHARE_TOKEN}")

echo "[e2e] starting hitkeep on ${BASE_URL}"
HITKEEP_MAIL_HOST="${MAIL_HOST}" \
HITKEEP_MAIL_PORT="${MAIL_PORT}" \
HITKEEP_MAIL_ENCRYPTION="${MAIL_ENCRYPTION}" \
HITKEEP_WEBHOOK_ALLOW_DEVELOPMENT_TARGETS="true" \
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
  -auth-rate-limit "${HITKEEP_E2E_AUTH_RATE_LIMIT:-1000}" \
  -auth-burst "${HITKEEP_E2E_AUTH_BURST:-1000}" \
  -jwt-secret "e2e-only-${PORT}" \
  -log-level "${LOG_LEVEL}" \
  > >(sed 's/^/[hitkeep] /') 2>&1 &

HK_PID=$!
wait "${HK_PID}"
