#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"
go run -trimpath ./cmd/data-refresh spam -output internal/blocking/default_spam_filter.json "$@"
