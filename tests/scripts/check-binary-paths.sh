#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

go build -o "$TMP_DIR/hitkeep" "$ROOT_DIR/cmd/hitkeep"

for forbidden in \
  "tests/fixtures" \
  "frontend/dashboard/public/tracker-fixtures" \
  ".github/e2e" \
  ".github/scripts/audit-mcp.sh"; do
  if strings "$TMP_DIR/hitkeep" | grep -Fq "$forbidden"; then
    echo "binary contains test-support path: $forbidden" >&2
    exit 1
  fi
done
